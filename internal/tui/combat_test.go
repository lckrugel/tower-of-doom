package tui

import (
	"strings"
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// The combat mirror's tests. The engine keeps combat-scoped state (move
// cooldowns, active statuses) unexported — the mirror rebuilds it from
// the event stream. Most tests below drive a REAL engine through real
// rounds and assert the mirror against the engine's own observable truth
// (its exported MoveCooldown accessor and its log lines).

// statusAssets mirrors the shipped statuses.json subset the fixtures use.
var statusAssets = map[string]game.StatusAsset{
	"poison": {ID: "poison", Name: "Poison", Kind: "debuff",
		Effect: game.StatusEffectAsset{Type: "dot", Amount: 3}, Duration: 3},
	"guard": {ID: "guard", Name: "Guard", Kind: "buff",
		Effect: game.StatusEffectAsset{Type: "stat", Stat: "def", Amount: 5}, Duration: 2},
}

// combatEngine is a real engine whose registry carries the status assets,
// so status-carrying runtime moves resolve at strike time. The section's
// budget spawns four goblins — a deep enough queue for kill rounds.
func combatEngine() *engine.Engine {
	return engine.NewEngine(&game.Registry{
		Monsters: map[string]game.MonsterAsset{
			"goblin": {ID: "goblin", Name: "Goblin", Difficulty: 3, HP: 30, Damage: 2, Speed: 10, Crit: 5},
		},
		Items:    map[string]game.ItemAsset{},
		Statuses: statusAssets,
		Sections: []game.SectionAsset{{
			ID: "pits", Name: "Goblin Pits", MinFloor: 1, MaxFloor: -1,
			BaseDifficulty: 12, DifficultyPerFloor: 3,
			Spawns: []game.SpawnEntry{{MonsterID: "goblin", Weight: 1}},
		}},
	})
}

// envenom is a deterministic status move: 100% poison on the enemy.
func envenom(cooldown int) entities.Move {
	return entities.Move{
		ID: "envenom", Name: "Envenom", Tier: 1, DmgModifier: 100,
		Cooldown: cooldown, Weight: 10,
		Status: entities.MoveStatus{ID: "poison", Chance: 100, Target: entities.TargetEnemy},
	}
}

// jab is a plain spamming move: neutral damage, no strings attached.
func jab() entities.Move {
	return entities.Move{ID: "jab", Name: "Jab", Tier: 1, DmgModifier: 100, Weight: 10}
}

// learnMove teaches the hero a hand-built runtime move (the registry is
// only consulted for status resolution at strike time).
func learnMove(e *engine.Engine, move entities.Move) {
	if _, ok := e.Player.LearnMove(move); !ok {
		panic("test fixture: could not learn " + move.ID)
	}
}

// enterCombat drives a fresh model into a real fight through the keys:
// explore (a run's first explore is a forced combat encounter), fight.
func enterCombat(t *testing.T, e *engine.Engine) Model {
	t.Helper()
	m := New(e)
	m, _ = press(m, "e")
	if m.engine.State != engine.StateEncounter {
		t.Fatalf("setup: state after explore = %v, want StateEncounter", m.engine.State)
	}
	m, _ = press(m, "f")
	if m.engine.State != engine.StateCombat {
		t.Fatalf("setup: state after fight = %v, want StateCombat", m.engine.State)
	}
	return m
}

// useMove dispatches CmdMove through the model's real dispatch path —
// the same route the move grid's keys take.
func useMove(t *testing.T, m Model, index int) Model {
	t.Helper()
	next, _ := m.dispatch(action{key: "1", label: "move", cmd: engine.CmdMove, enabled: true, usesIndex: true, index: index})
	return modelOf(next, t)
}

// spitterMonster is a monster whose only move is a sure poison aimed at
// ITS enemy — the hero (mirrors the engine's spitter fixture). The HP is
// a parameter: a DMG-boosted fixture hero must never crit-kill it in the
// poison round (tests set CurrentHP = 1 to engineer the kill instead).
func spitterMonster(hp int) entities.Monster {
	move := envenom(0)
	move.ID, move.Weight = "spit_poison", 100
	return entities.NewMonsterFromAsset(
		game.MonsterAsset{ID: "spitter", Name: "Spitter", Difficulty: 2, HP: hp, Damage: 1, Speed: 10},
		move,
	)
}

// goblinMonster is the fixture goblin, built directly for hand-made queues.
func goblinMonster() entities.Monster {
	return entities.NewMonsterFromAsset(game.MonsterAsset{
		ID: "goblin", Name: "Goblin", Difficulty: 3, HP: 30, Damage: 2, Speed: 10, Crit: 5,
	})
}

// joinedLog is the full rendered log as plain text.
func joinedLog(m Model) string {
	lines := make([]string, 0, len(m.log))
	for _, l := range m.log {
		lines = append(lines, stripANSI(l))
	}
	return strings.Join(lines, "\n")
}

// assertCooldownsEqual fails unless the mirror's picture of every combat
// move's cooldown matches the engine's own read-only accessor — the two
// must never drift apart across real rounds.
func assertCooldownsEqual(t *testing.T, m Model, when string) {
	t.Helper()
	for _, mv := range m.engine.CombatMoves() {
		if got, want := m.mirror.cooldownLeft(mv.ID), m.engine.MoveCooldown(mv.ID); got != want {
			t.Errorf("%s: mirror cooldown of %s = %d, engine reports %d", when, mv.ID, got, want)
		}
	}
}

// Cooldowns lock on use and tick down as completed rounds pass — the
// mirror must agree with the engine's accessor after every single round.
func TestMirrorCooldownsFollowTheEngineAcrossRounds(t *testing.T) {
	e := combatEngine()
	learnMove(e, envenom(2)) // locks for 2 rounds
	learnMove(e, jab())
	m := enterCombat(t, e)

	m = useMove(t, m, 0) // round 1: Envenom locks
	assertCooldownsEqual(t, m, "after round 1")
	if got := m.mirror.cooldownLeft("envenom"); got != 2 {
		t.Errorf("cooldown after round 1 = %d, want 2 (the move's cooldown)", got)
	}

	m = useMove(t, m, 1) // round 2: a completed round ticks the lock down
	assertCooldownsEqual(t, m, "after round 2")
	if got := m.mirror.cooldownLeft("envenom"); got != 1 {
		t.Errorf("cooldown after round 2 = %d, want 1", got)
	}

	m = useMove(t, m, 1) // round 3
	m = useMove(t, m, 1) // round 4: ready again
	assertCooldownsEqual(t, m, "after round 4")
	if got := m.mirror.cooldownLeft("envenom"); got != 0 {
		t.Errorf("cooldown after round 4 = %d, want 0 (expired)", got)
	}
}

// A blocked move (dispatched while cooling) is a message, not a round:
// neither the mirror nor the engine may tick.
func TestMirrorBlockedMoveDoesNotTick(t *testing.T) {
	e := combatEngine()
	learnMove(e, envenom(2))
	learnMove(e, jab())
	m := enterCombat(t, e)
	m = useMove(t, m, 0)

	m = useMove(t, m, 0) // locked — the engine blocks it

	last := stripANSI(m.log[len(m.log)-1])
	if last != "Move on cooldown." {
		t.Fatalf("last log line = %q, want the cooldown block", last)
	}
	if got := m.mirror.cooldownLeft("envenom"); got != 2 {
		t.Errorf("a blocked move ticked the mirror: %d, want 2", got)
	}
	assertCooldownsEqual(t, m, "after the blocked move")
}

// A RECOVER round (every move locked) still ticks: the enemy acts, the
// round completes, cooldowns count down without a hero lock being set.
func TestMirrorRecoverRoundTicksCooldowns(t *testing.T) {
	e := combatEngine()
	learnMove(e, envenom(2)) // the ONLY move
	m := enterCombat(t, e)
	m = useMove(t, m, 0)

	m = useMove(t, m, 0) // fully cooled → RECOVER round

	if !strings.Contains(joinedLog(m), "you recover this turn") {
		t.Fatalf("setup: no recover round happened:\n%s", joinedLog(m))
	}
	if got := m.mirror.cooldownLeft("envenom"); got != 1 {
		t.Errorf("cooldown after the recover round = %d, want 1", got)
	}
	assertCooldownsEqual(t, m, "after the recover round")
}

// A killing blow ends the round early: no tick runs, but the used move
// still locks at its full cooldown (engine appendEnemyDefeat).
func TestMirrorKillRoundLocksWithoutTicking(t *testing.T) {
	e := combatEngine()
	e.Player.BaseDMG = 40 // one-shots the fixture goblins
	learnMove(e, envenom(2))
	learnMove(e, jab())
	m := enterCombat(t, e)

	m = useMove(t, m, 0) // kill → round aborted, lock applied at full value
	if got := m.mirror.cooldownLeft("envenom"); got != 2 {
		t.Errorf("kill-round lock = %d, want 2 (full cooldown, no tick)", got)
	}
	assertCooldownsEqual(t, m, "after the kill round")

	m = useMove(t, m, 1) // another kill (Jab: no cooldown of its own)
	if m.engine.State != engine.StateCombat {
		t.Fatalf("setup: combat ended early (state %v)", m.engine.State)
	}
	if got := m.mirror.cooldownLeft("envenom"); got != 2 {
		t.Errorf("the second kill round ticked the lock: %d, want 2", got)
	}
	assertCooldownsEqual(t, m, "after the second kill round")
}

// Status chips appear when the event lands, count down as completed
// rounds pass, and vanish when the engine reports the expiry.
func TestMirrorTracksStatusChipsAcrossRounds(t *testing.T) {
	e := combatEngine()
	learnMove(e, envenom(0)) // spammable poison
	learnMove(e, jab())
	m := enterCombat(t, e)

	m = useMove(t, m, 0) // the poison lands (chance 100)

	chips := m.mirror.enemyStatuses
	if len(chips) != 1 || chips[0].name != "Poison" || chips[0].turns != 2 {
		t.Fatalf("after round 1 the enemy chips are %+v, want Poison(2) — applied at 3, ticked at round end", chips)
	}

	m = useMove(t, m, 1) // round 2
	if chips = m.mirror.enemyStatuses; len(chips) != 1 || chips[0].turns != 1 {
		t.Errorf("after round 2 the enemy chips are %+v, want Poison(1)", chips)
	}

	m = useMove(t, m, 1) // round 3: the last tick expires it
	if chips = m.mirror.enemyStatuses; len(chips) != 0 {
		t.Errorf("the expired poison still shows: %+v", chips)
	}
	if !strings.Contains(joinedLog(m), "The Poison wears off.") {
		t.Error("the engine's expiry line is missing — the mirror must agree with the log")
	}
}

// Re-applying a status refreshes it to the event's turns payload (the
// engine's max-of rule) — never a stack, never a sum.
func TestMirrorRefreshesAStatusToTheEventTurns(t *testing.T) {
	e := combatEngine()
	learnMove(e, envenom(0))
	m := enterCombat(t, e)

	m = useMove(t, m, 0) // applied at 3 → ticked to 2
	m = useMove(t, m, 0) // re-applied: refreshed to 3 → ticked to 2

	chips := m.mirror.enemyStatuses
	if len(chips) != 1 || chips[0].turns != 2 {
		t.Errorf("refreshed chips = %+v, want a single Poison(2)", chips)
	}
}

// A queue advance clears the ENEMY side only: the next monster steps in
// fresh, and the killing move locks at its full cooldown (no tick ran).
func TestMirrorClearsEnemyStatusesWhenTheQueueAdvances(t *testing.T) {
	e := combatEngine()
	learnMove(e, envenom(2))
	learnMove(e, jab())
	m := enterCombat(t, e)
	m = useMove(t, m, 0) // the goblin is poisoned; Envenom locks for 2

	m.engine.CurrentEncounter.Monsters[0].CurrentHP = 1
	m = useMove(t, m, 1) // Jab kills → the poison dies with the goblin

	if chips := m.mirror.enemyStatuses; len(chips) != 0 {
		t.Errorf("the dead goblin kept its statuses: %+v", chips)
	}
	if got := m.mirror.cooldownLeft("envenom"); got != 2 {
		t.Errorf("kill-round lock = %d, want 2 (full value, no tick)", got)
	}
	assertCooldownsEqual(t, m, "after the queue advance")
	if len(m.engine.CurrentEncounter.Monsters) != 3 {
		t.Errorf("queue = %d monsters, want 3 (a fresh goblin stepped up)", len(m.engine.CurrentEncounter.Monsters))
	}
}

// The HERO side survives a queue advance: the engine keeps heroCombat
// when the next monster steps in, and so must the mirror.
func TestMirrorKeepsTheHeroSideAcrossAQueueAdvance(t *testing.T) {
	e := combatEngine()
	learnMove(e, jab())
	e.CurrentEncounter = engine.Encounter{
		Type:     engine.EncounterCombat,
		Monsters: []entities.Monster{spitterMonster(200), goblinMonster()},
	}
	e.State = engine.StateCombat
	m := New(e)

	m = useMove(t, m, 0) // the spitter chips and poisons the hero
	if chips := m.mirror.heroStatuses; len(chips) != 1 || chips[0].turns != 2 {
		t.Fatalf("hero chips after round 1 = %+v, want Poison(2)", chips)
	}

	m.engine.CurrentEncounter.Monsters[0].CurrentHP = 1
	m = useMove(t, m, 0) // kill → the queue advances

	chips := m.mirror.heroStatuses
	if len(chips) != 1 || chips[0].turns != 2 {
		t.Errorf("hero chips after the queue advance = %+v, want Poison(2) kept", chips)
	}
}

// Leaving combat (victory here) resets everything: the hero panel must
// not carry chips into exploring. A NEW fight then tracks from zero —
// the engine's startCombat reset, mirrored at the state transition.
func TestMirrorResetsWhenANewCombatStarts(t *testing.T) {
	e := combatEngine()
	learnMove(e, jab())
	e.Player.BaseDMG = 40 // one-shots the queue
	e.CurrentEncounter = engine.Encounter{
		Type:     engine.EncounterCombat,
		Monsters: []entities.Monster{spitterMonster(200), goblinMonster()},
	}
	e.State = engine.StateCombat
	m := New(e)

	m = useMove(t, m, 0) // the spitter poisons the hero before dying
	if len(m.mirror.heroStatuses) == 0 {
		t.Fatal("setup: the hero was never poisoned")
	}
	m.engine.CurrentEncounter.Monsters[0].CurrentHP = 1
	m = useMove(t, m, 0) // the spitter falls; the goblin steps up
	m = useMove(t, m, 0) // the goblin falls → victory
	if m.engine.State != engine.StateExploring {
		t.Fatalf("setup: state after the last kill = %v, want StateExploring (victory)", m.engine.State)
	}
	if chips := m.mirror.heroStatuses; len(chips) != 0 {
		t.Errorf("the mirror survived the victory: %+v", chips)
	}

	// A new fight starts clean and tracks from zero again.
	m, _ = press(m, "e")
	m, _ = press(m, "f")
	if len(m.mirror.heroStatuses) != 0 {
		t.Errorf("a new combat started with stale chips: %+v", m.mirror.heroStatuses)
	}
	m = useMove(t, m, 0)
	assertCooldownsEqual(t, m, "in the new combat")
}

// EventEncounterFound resets the mirror preemptively: a found encounter
// may become a fight, and nothing stale may leak into it.
func TestMirrorResetsOnEncounterFound(t *testing.T) {
	m := New(combatEngine())
	m.absorb(engine.Event{Type: engine.EventStatusApplied, Data: map[string]any{
		"target": "Hero", "status": "Poison", "turns": 3, "kind": "debuff",
	}})

	m.absorb(engine.NewEncounterFound(nil))

	if chips := m.mirror.heroStatuses; len(chips) != 0 {
		t.Errorf("encounter found kept stale chips: %+v", chips)
	}
}

// A round is not over until the enemy answers — and one dispatch batch is
// one round: a batch holding only the hero's strike (a killing blow is
// the only real case, covered against the real engine above) locks
// nothing on its own.
func TestMirrorRoundNeedsTheEnemysResponse(t *testing.T) {
	newMirror := func() Model {
		m := New(combatEngine())
		learnMove(m.engine, envenom(2))
		return m
	}

	t.Run("a hero strike without the enemy's response locks nothing", func(t *testing.T) {
		m := newMirror()
		m.absorb(engine.NewMoveUsed("Hero", "Envenom"))
		m.mirror.endBatch() // no enemy response in this batch

		if got := m.mirror.cooldownLeft("envenom"); got != 0 {
			t.Errorf("a lock applied without the enemy's response: %d, want 0", got)
		}
	})

	t.Run("the enemy's answer completes the round and locks the move", func(t *testing.T) {
		m := newMirror()
		m.absorb(engine.NewMoveUsed("Hero", "Envenom"))
		m.absorb(engine.NewMoveUsed("Goblin", "Basic Attack"))
		m.absorb(damagedHero(2, false))
		m.mirror.endBatch()

		if got := m.mirror.cooldownLeft("envenom"); got != 2 {
			t.Errorf("cooldown after the completed round = %d, want 2", got)
		}
	})
}

// A failed flee grants the enemy a free turn: the round completes and
// ticks even though the hero never acted (engine enemyFreeTurn).
func TestMirrorFreeTurnCountsAsARound(t *testing.T) {
	m := New(combatEngine())
	m.mirror.heroCooldowns["envenom"] = 2 // locked from an earlier round

	m.absorb(engine.NewMessage("You failed to flee!"))
	m.absorb(engine.NewMoveUsed("Goblin", "Basic Attack"))
	m.absorb(damagedHero(2, false))
	m.mirror.endBatch()

	if got := m.mirror.cooldownLeft("envenom"); got != 1 {
		t.Errorf("the free turn did not tick the lock: %d, want 1", got)
	}
}

// A DoT kill at round end: the enemy's killing tick proves the round-end
// tick ran (the hero's afflictions tick FIRST) even though the defeat
// aborted the cooldown bookkeeping — the mirror reads the tick events as
// proof and ticks the hero side, while the lock still applies at full
// value and the enemy side clears.
func TestMirrorDoTKillAtRoundEndStillTicksHeroStatuses(t *testing.T) {
	m := New(combatEngine())
	learnMove(m.engine, envenom(2))

	m.absorb(engine.Event{Type: engine.EventStatusApplied, Data: map[string]any{
		"target": "Hero", "status": "Guard", "turns": 2, "kind": "buff",
	}})
	m.absorb(engine.Event{Type: engine.EventStatusApplied, Data: map[string]any{
		"target": "Goblin", "status": "Poison", "turns": 1, "kind": "debuff",
	}})
	m.absorb(engine.NewMoveUsed("Hero", "Envenom"))
	m.absorb(engine.NewMoveUsed("Goblin", "Basic Attack"))
	m.absorb(engine.Event{Type: engine.EventStatusDamage, Data: map[string]any{
		"target": "Goblin", "status": "Poison", "amount": 3, "kind": "debuff",
	}})
	m.absorb(engine.NewEnemyDefeated("Goblin", 4))
	m.mirror.endBatch()

	chips := m.mirror.heroStatuses
	if len(chips) != 1 || chips[0].name != "Guard" || chips[0].turns != 1 {
		t.Errorf("hero chips after the DoT kill = %+v, want Guard(1) — the hero's tick ran first", chips)
	}
	if got := m.mirror.cooldownLeft("envenom"); got != 2 {
		t.Errorf("lock after the DoT kill = %d, want 2 (the defeat path locks, never ticks)", got)
	}
	if chips := m.mirror.enemyStatuses; len(chips) != 0 {
		t.Errorf("the dead enemy kept statuses: %+v", chips)
	}
}

// A plain kill (no round-end tick) must NOT tick the hero's statuses:
// only tick events prove the round end ran.
func TestMirrorPlainKillSkipsTheStatusTick(t *testing.T) {
	m := New(combatEngine())
	learnMove(m.engine, envenom(2))

	m.absorb(engine.Event{Type: engine.EventStatusApplied, Data: map[string]any{
		"target": "Hero", "status": "Guard", "turns": 2, "kind": "buff",
	}})
	m.absorb(engine.NewMoveUsed("Goblin", "Basic Attack"))
	m.absorb(damagedHero(2, false))
	m.absorb(engine.NewMoveUsed("Hero", "Envenom"))
	m.absorb(engine.NewEnemyDefeated("Goblin", 4))
	m.mirror.endBatch()

	chips := m.mirror.heroStatuses
	if len(chips) != 1 || chips[0].turns != 2 {
		t.Errorf("hero chips after the plain kill = %+v, want Guard(2) untouched (no tick ran)", chips)
	}
	if got := m.mirror.cooldownLeft("envenom"); got != 2 {
		t.Errorf("lock after the plain kill = %d, want 2", got)
	}
}
