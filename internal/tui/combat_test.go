package tui

import (
	"fmt"
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

// statusAssets mirrors the shipped statuses.json — the fixture engines'
// registry lore (the '?' overlay and the chip explanations read it;
// strike-time resolution only ever references poison and guard).
var statusAssets = map[string]game.StatusAsset{
	"poison": {ID: "poison", Name: "Poison", Kind: "debuff",
		Effect: game.StatusEffectAsset{Type: "dot", Amount: 3}, Duration: 3},
	"guard": {ID: "guard", Name: "Guard", Kind: "buff",
		Effect: game.StatusEffectAsset{Type: "stat", Stat: "def", Amount: 5}, Duration: 2},
	"weaken": {ID: "weaken", Name: "Weaken", Kind: "debuff",
		Effect: game.StatusEffectAsset{Type: "stat", Stat: "dmg", Amount: -3}, Duration: 3},
	"slow": {ID: "slow", Name: "Slow", Kind: "debuff",
		Effect: game.StatusEffectAsset{Type: "stat", Stat: "spd", Amount: -3}, Duration: 3},
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

// ---- the 2×2 move grid ----

// heavy is the canonical cooldown move fixture (175%, locks 2 rounds).
func heavy() entities.Move {
	return entities.Move{ID: "heavy_slash", Name: "Heavy Slash", Tier: 2, DmgModifier: 175, Cooldown: 2, Weight: 10}
}

// stoneEdge / wildSwing are plain damage moves for the weapon fixture.
func stoneEdge() entities.Move {
	return entities.Move{ID: "stone_edge", Name: "Stone Edge", Tier: 1, DmgModifier: 90, Weight: 10}
}

func wildSwing() entities.Move {
	return entities.Move{ID: "wild_swing", Name: "Wild Swing", Tier: 1, DmgModifier: 110, Weight: 10}
}

// equipWeapon puts a weapon granting the given runtime moves on the hero
// — the second half of a 4-move hero set (MaxLearnedMoves caps learning
// at 2; weapon moves sit on top).
func equipWeapon(e *engine.Engine, moves ...entities.Move) {
	weapon := entities.NewItemFromAsset(game.ItemAsset{
		ID: "fang_blade", Name: "Fang Blade", Slot: "weapon",
	}, moves...)
	e.Player.EquipItem(&weapon)
}

// fourMoveModel is a real combat model with a full grid: two learned
// moves plus two weapon-granted ones.
func fourMoveModel(t *testing.T) Model {
	t.Helper()
	e := combatEngine()
	learnMove(e, envenom(0))
	learnMove(e, jab())
	equipWeapon(e, stoneEdge(), wildSwing())
	return enterCombat(t, e)
}

// A modded move whose name is longer than any shipped one truncates
// inside its cell — the grid's geometry never moves for it.
func TestGridTruncatesModdedOverlongNames(t *testing.T) {
	e := combatEngine()
	learnMove(e, entities.Move{
		ID: "modded", Name: "Super Calibrating Excalibur Strike", Tier: 1, DmgModifier: 100, Weight: 10,
	})
	m := enterCombat(t, e)

	for i, line := range viewLines(m.moveGrid()) {
		if i == 0 {
			continue
		}
		if w := ansiLen(line); w != mainColWidth {
			t.Errorf("grid line %d is %d wide, want exactly %d:\n%q", i, w, mainColWidth, line)
		}
	}
}

// The combat bar shrank to the two combat-only actions; the moves live
// in the grid.
func TestCombatBarIsFleeAndQuit(t *testing.T) {
	m := fourMoveModel(t)

	actions := actionsFor(m.engine)

	if !keysEqual(keysOf(actions), "r", "q") {
		t.Fatalf("combat bar = %v, want [r q] (moves moved into the grid)", keysOf(actions))
	}
}

// The body renders the grid: a cell per effective move, in the exact
// order CmdMove's index points into, each with its tags.
func TestViewCombatShowsTheMoveGrid(t *testing.T) {
	m := fourMoveModel(t)

	view := stripControls(stripANSI(m.View()))

	for _, want := range []string{
		"◈ MOVES",
		"Envenom", "Jab", "Stone Edge", "Wild Swing", // effective-set order
		"100%", "☠poison 100%", // envenom's tags
		"90%", "110%", // the weapon moves' modifiers
		"▶ Envenom", // the cursor opens on the first cell
	} {
		if !strings.Contains(view, want) {
			t.Errorf("combat grid missing %q:\n%s", want, view)
		}
	}
}

// Fewer than 4 moves leaves the remaining cells as dim — placeholders.
func TestViewGridShowsPlaceholdersForMissingMoves(t *testing.T) {
	e := combatEngine()
	learnMove(e, envenom(0))
	m := enterCombat(t, e)

	view := stripControls(stripANSI(m.View()))

	if !strings.Contains(view, "Envenom") {
		t.Errorf("the single move is missing from the grid:\n%s", view)
	}
	if !strings.Contains(view, "—") {
		t.Errorf("the empty cells render no placeholder:\n%s", view)
	}
	if strings.Contains(view, "Basic Attack") {
		t.Errorf("the grid invented a move:\n%s", view)
	}
}

// The 2+2 split made visible: every grid cell trails its move's source
// glyph — ◆ for the learned set (shrine teaching), ⚔ for weapon
// grants — resolved from the hero's effective set
// (Hero.EffectiveMovesWithSource).
func TestGridCellsTagTheMoveSource(t *testing.T) {
	m := fourMoveModel(t) // two learned moves + two weapon-granted

	grid := stripControls(stripANSI(m.moveGrid()))

	for _, want := range []string{
		"☠poison 100% ◆", // Envenom: learned
		"100% ◆",         // Jab: learned
		"90% ⚔",          // Stone Edge: weapon-granted
		"110% ⚔",         // Wild Swing: weapon-granted
	} {
		if !strings.Contains(grid, want) {
			t.Errorf("move grid missing the source tag %q:\n%s", want, grid)
		}
	}
}

// A hero with NO moves at all fights with the canonical basic attack —
// the grid shows it as the single real cell.
func TestViewGridShowsTheBasicAttackFallback(t *testing.T) {
	m := enterCombat(t, combatEngine())

	view := stripControls(stripANSI(m.View()))

	if !strings.Contains(view, "Basic Strike") || !strings.Contains(view, "100%") {
		t.Errorf("the basic-attack fallback is missing from the grid:\n%s", view)
	}
	// The canonical fallback is neither learned nor weapon-granted: its
	// cell carries no source tag.
	if strings.Contains(view, "◆") || strings.Contains(view, "100% ⚔") {
		t.Errorf("the basic-attack fallback grew a source tag:\n%s", view)
	}
}

// A cooling move's cell grays out and shows the turns-left marker —
// the modifier tags yield to it while the move is unusable.
func TestViewGridMarksCoolingMoves(t *testing.T) {
	e := combatEngine()
	learnMove(e, heavy())
	learnMove(e, jab())
	m := enterCombat(t, e)
	m = useMove(t, m, 0) // Heavy Slash locks for 2

	view := stripControls(stripANSI(m.View()))

	if !strings.Contains(view, "(2 turns)") {
		t.Errorf("the cooling cell shows no turns-left marker:\n%s", view)
	}
	if strings.Contains(view, "175%") {
		t.Errorf("the cooling cell kept its modifier tag:\n%s", view)
	}
	if !strings.Contains(view, "Heavy Slash") {
		t.Errorf("the cooling cell lost its name:\n%s", view)
	}
}

// The arrows walk the 2×2 grid with wraparound — the action-bar cursor
// stays untouched (the combat bar is a key legend now, not selectable).
func TestArrowKeysMoveTheGridCursor(t *testing.T) {
	m := fourMoveModel(t) // a perfect 2×2: [0 1 / 2 3]
	if m.gridCursor != 0 {
		t.Fatalf("the grid opens on cell %d, want 0", m.gridCursor)
	}
	logLen := len(m.log)

	steps := []struct {
		key  string
		want int
	}{
		{"right", 1}, {"down", 3}, {"left", 2}, {"up", 0}, // the square
		{"up", 2}, {"up", 0}, // vertical wraparound
		{"down", 2}, {"down", 0},
		{"left", 3}, {"left", 2}, // horizontal wraparound
		{"right", 3}, {"right", 0},
	}
	for _, s := range steps {
		m, _ = press(m, s.key)
		if m.gridCursor != s.want {
			t.Fatalf("%s from the previous cell = %d, want %d", s.key, m.gridCursor, s.want)
		}
		if m.cursor != 0 {
			t.Fatalf("%s moved the action-bar cursor (%d)", s.key, m.cursor)
		}
	}

	// Navigation dispatches nothing: no log, no engine round.
	if len(m.log) != logLen {
		t.Errorf("cursor movement wrote %d log lines, want 0", len(m.log)-logLen)
	}
}

// Three moves leave the bottom-right cell empty: wraparound walks the
// real cells and never rests on a placeholder.
func TestGridCursorWrapsOnThreeMoves(t *testing.T) {
	e := combatEngine()
	learnMove(e, envenom(0))
	learnMove(e, jab())
	equipWeapon(e, stoneEdge()) // 3 moves: [0 1 / 2 —]
	m := enterCombat(t, e)

	m, _ = press(m, "down") // the real cell below 0
	if m.gridCursor != 2 {
		t.Fatalf("down from 0 = %d, want 2", m.gridCursor)
	}
	m, _ = press(m, "down") // wraparound up the column: 2 → 0
	if m.gridCursor != 0 {
		t.Fatalf("down from 2 = %d, want 0", m.gridCursor)
	}
	m, _ = press(m, "right") // 0 → 1
	m, _ = press(m, "down")  // below 1 is the placeholder: stay put
	if m.gridCursor != 1 {
		t.Fatalf("down from 1 with a ragged grid = %d, want 1 (stay put)", m.gridCursor)
	}
}

// Enter dispatches the highlighted cell — the observable engine effect
// is the move's round resolving — and the cursor stays where it was.
func TestEnterDispatchesTheHighlightedMove(t *testing.T) {
	e := combatEngine()
	learnMove(e, heavy())
	learnMove(e, jab())
	m := enterCombat(t, e)

	m, _ = press(m, "right") // highlight Jab (index 1)
	m, _ = press(m, "enter")

	if !strings.Contains(joinedLog(m), "You use Jab!") {
		t.Errorf("enter dispatched the wrong cell:\n%s", joinedLog(m))
	}
	if m.gridCursor != 1 {
		t.Errorf("grid cursor after dispatch = %d, want 1 (stays put)", m.gridCursor)
	}
}

// Enter on a cooling cell dispatches too: the engine answers "Move on
// cooldown." in the log, state intact.
func TestEnterOnACoolingCellStillDispatches(t *testing.T) {
	e := combatEngine()
	learnMove(e, heavy())
	learnMove(e, jab())
	m := enterCombat(t, e)
	m = useMove(t, m, 0) // Heavy Slash locks for 2; the cursor stays on 0

	m, _ = press(m, "enter")

	last := stripANSI(m.log[len(m.log)-1])
	if last != "Move on cooldown." {
		t.Errorf("enter on the cooling cell logged %q, want the cooldown block", last)
	}
	if m.engine.State != engine.StateCombat {
		t.Errorf("state after the blocked dispatch = %v, want StateCombat", m.engine.State)
	}
}

// Digits beyond the move set dispatch anyway — the engine explains the
// rejection as a generic invalid command, and the cursor never rests on
// a placeholder.
func TestDigitsBeyondTheMoveSetStillDispatch(t *testing.T) {
	e := combatEngine()
	learnMove(e, jab())
	m := enterCombat(t, e)

	m, _ = press(m, "3") // a placeholder cell

	last := stripANSI(m.log[len(m.log)-1])
	if last != "Invalid command" {
		t.Errorf("digit on a placeholder logged %q, want the engine's rejection", last)
	}
	if m.engine.State != engine.StateCombat {
		t.Errorf("state after the rejected digit = %v, want StateCombat", m.engine.State)
	}
	if m.gridCursor != 0 {
		t.Errorf("grid cursor after the rejected digit = %d, want 0 (untouched)", m.gridCursor)
	}
}

// esc during combat is a no-op — the arrows are navigation there, so esc
// must neither quit nor arm the confirmation.
func TestEscapeIsANoOpInCombat(t *testing.T) {
	m := fourMoveModel(t)
	logLen := len(m.log)

	m, cmd := press(m, "esc")

	if cmd != nil {
		t.Error("esc in combat must not request a quit")
	}
	if m.engine.State != engine.StateCombat {
		t.Errorf("state after esc = %v, want to stay in StateCombat", m.engine.State)
	}
	if m.confirmQuit {
		t.Error("esc in combat armed the quit confirmation")
	}
	if len(m.log) != logLen {
		t.Error("esc in combat wrote to the log")
	}
}

// A new combat re-opens the grid on the first cell (the cursor resets
// with the mirror).
func TestGridCursorResetsWithTheMirror(t *testing.T) {
	e := combatEngine()
	learnMove(e, jab())
	e.Player.BaseDMG = 40 // one-shots the queue
	e.CurrentEncounter = engine.Encounter{
		Type:     engine.EncounterCombat,
		Monsters: []entities.Monster{spitterMonster(200), goblinMonster()},
	}
	e.State = engine.StateCombat
	m := New(e)

	m = useMove(t, m, 0)
	m.engine.CurrentEncounter.Monsters[0].CurrentHP = 1
	m = useMove(t, m, 0) // the spitter falls → the goblin steps up
	m = useMove(t, m, 0) // the goblin falls → victory
	if m.engine.State != engine.StateExploring {
		t.Fatalf("setup: state = %v, want StateExploring (victory)", m.engine.State)
	}

	m, _ = press(m, "e")
	m, _ = press(m, "f")

	if m.gridCursor != 0 {
		t.Errorf("a new combat opened the grid on cell %d, want 0", m.gridCursor)
	}
}

// Outside combat the vertical arrows mean nothing — the action-bar
// cursor must not move.
func TestVerticalArrowsAreNoOpsOutsideCombat(t *testing.T) {
	m := New(testEngine())

	m, _ = press(m, "up")
	m, _ = press(m, "down")

	if m.cursor != 0 {
		t.Errorf("vertical arrows moved the bar cursor to %d outside combat", m.cursor)
	}
}

// ---- status chips beside the HP bars ----

// The enemy block shows its active statuses as chips next to the fight:
// the poison chip appears as the event lands (already ticked once —
// the round ended).
func TestViewCombatShowsEnemyStatusChips(t *testing.T) {
	e := combatEngine()
	learnMove(e, envenom(0))
	m := enterCombat(t, e)

	m = useMove(t, m, 0)

	view := stripControls(stripANSI(m.View()))
	if !strings.Contains(view, "Poison(2)") {
		t.Errorf("the enemy block shows no poison chip:\n%s", view)
	}
}

// Chips count down as completed rounds pass and vanish on expiry —
// the whole scripted lifecycle, asserted on the rendered view.
func TestStatusChipsCountDownAndVanish(t *testing.T) {
	e := combatEngine()
	learnMove(e, envenom(0))
	learnMove(e, jab())
	m := enterCombat(t, e)

	m = useMove(t, m, 0) // applied: 3, ticked to 2 at round end
	if !strings.Contains(stripControls(stripANSI(m.View())), "Poison(2)") {
		t.Errorf("round 1: the chip should show Poison(2):\n%s", stripControls(stripANSI(m.View())))
	}

	m = useMove(t, m, 1) // round 2
	if !strings.Contains(stripControls(stripANSI(m.View())), "Poison(1)") {
		t.Errorf("round 2: the chip should show Poison(1):\n%s", stripControls(stripANSI(m.View())))
	}

	m = useMove(t, m, 1) // round 3: the last tick expires it
	if strings.Contains(stripControls(stripANSI(m.View())), "Poison(") {
		t.Errorf("round 3: the expired chip still renders:\n%s", stripControls(stripANSI(m.View())))
	}
}

// The hero panel's HP row gains its own chips — without growing a row:
// the panel's four-line shape is the layout invariant.
func TestViewHeroPanelShowsStatusChips(t *testing.T) {
	e := combatEngine()
	learnMove(e, jab())
	e.CurrentEncounter = engine.Encounter{
		Type:     engine.EncounterCombat,
		Monsters: []entities.Monster{spitterMonster(200), goblinMonster()},
	}
	e.State = engine.StateCombat
	m := New(e)

	m = useMove(t, m, 0) // the spitter poisons the hero

	lines := viewLines(m.heroPanel())
	if len(lines) != 4 {
		t.Fatalf("hero panel has %d lines with chips, want 4", len(lines))
	}
	if !strings.Contains(lines[2], "Poison(2)") {
		t.Errorf("hero HP row shows no poison chip: %q", lines[2])
	}
}

// Chips render ONLY while the fight lives: after combat ends the hero
// panel is back to plain vitals.
func TestHeroChipsVanishOutsideCombat(t *testing.T) {
	e := combatEngine()
	learnMove(e, jab())
	e.Player.BaseDMG = 40
	e.CurrentEncounter = engine.Encounter{
		Type:     engine.EncounterCombat,
		Monsters: []entities.Monster{spitterMonster(200), goblinMonster()},
	}
	e.State = engine.StateCombat
	m := New(e)

	m = useMove(t, m, 0) // poisoned
	m.engine.CurrentEncounter.Monsters[0].CurrentHP = 1
	m = useMove(t, m, 0) // the spitter falls
	m = useMove(t, m, 0) // the goblin falls → victory

	if m.engine.State != engine.StateExploring {
		t.Fatalf("setup: state = %v, want StateExploring (victory)", m.engine.State)
	}
	if strings.Contains(stripControls(stripANSI(m.View())), "Poison(") {
		t.Errorf("the hero chip survived the victory:\n%s", stripControls(stripANSI(m.View())))
	}
}

// The hero's chips gain the same explanation, riding the HP row after
// them — the panel's four-line shape is the layout invariant (heroHeight
// feeds bodyHeight and the merchant's exact worst case), so the
// explanation shares the row, cut to the room left: the documented
// display cap; the '?' overlay carries the full lore.
func TestHeroChipsExplanationRidesTheHPRow(t *testing.T) {
	e := combatEngine()
	learnMove(e, jab())
	e.CurrentEncounter = engine.Encounter{
		Type:     engine.EncounterCombat,
		Monsters: []entities.Monster{spitterMonster(200), goblinMonster()},
	}
	e.State = engine.StateCombat
	m := New(e)

	m = useMove(t, m, 0) // the spitter poisons the hero

	lines := viewLines(m.heroPanel())
	if len(lines) != 4 {
		t.Fatalf("hero panel has %d lines with the explanation, want 4", len(lines))
	}
	if w := ansiLen(lines[2]); w > mainColWidth {
		t.Errorf("hero HP row is %d wide, overflows the %d-col column", w, mainColWidth)
	}
	// The chip stays; the explanation rides after it, cut to the ~13
	// runes of room the row has left for this fixture.
	if !strings.Contains(lines[2], "Poison(2)") || !strings.Contains(lines[2], "Poison — 3 d") {
		t.Errorf("hero HP row lost the chip or the explanation: %q", lines[2])
	}
}

// Chip tones follow the status KIND, same rule as the log: debuffs
// soft-red, buffs green.
func TestStatusChipsToneByKind(t *testing.T) {
	chips := statusChips([]combatStatus{
		{name: "Guard", kind: "buff", turns: 2},
		{name: "Poison", kind: "debuff", turns: 3},
	})

	if got, want := chips[0], styleHeal.Render("Guard(2)"); got != want {
		t.Errorf("buff chip = %q, want %q", got, want)
	}
	if got, want := chips[1], styleHurt.Render("Poison(3)"); got != want {
		t.Errorf("debuff chip = %q, want %q", got, want)
	}
}

// The enemy block's chips wrap to at most two rows, plus ONE dim
// explanation line under them; nothing leans on slot() to clip: every
// row fits the column, the rest is dropped.
func TestEnemyStatusRowsCap(t *testing.T) {
	m := New(combatEngine())
	for i := range 6 {
		m.mirror.enemyStatuses = append(m.mirror.enemyStatuses, combatStatus{
			name: fmt.Sprintf("Status %d", i), kind: "debuff", turns: 3,
		})
	}

	rows := strings.Split(m.enemyStatusRows(), "\n")

	if len(rows) > maxStatusRows+1 {
		t.Errorf("enemy statuses occupy %d rows, want at most %d chip rows + 1 explanation",
			len(rows), maxStatusRows)
	}
	for i, row := range rows {
		if w := ansiLen(row); w > mainColWidth {
			t.Errorf("status row %d is %d wide, overflows the %d-col column: %q", i, w, mainColWidth, row)
		}
	}
}

// Under the chips sits one dim explanation line — "Poison — 3 dmg/turn
// · 2 turns left": the live turns from the mirror, the effect from the
// registry's lore.
func TestEnemyChipsCarryAnExplanationLine(t *testing.T) {
	e := combatEngine()
	e.CurrentEncounter = engine.Encounter{
		Type:     engine.EncounterCombat,
		Monsters: []entities.Monster{goblinMonster()},
	}
	m := New(e)
	m.mirror.enemyStatuses = []combatStatus{{name: "Poison", kind: "debuff", turns: 2}}

	found := 0
	for _, line := range viewLines(m.combatPanel()) {
		if strings.Contains(line, "Poison — 3 dmg/turn · 2 turns left") {
			found++
		}
	}
	if found != 1 {
		t.Errorf("the chips' explanation line is missing:\n%s", m.combatPanel())
	}
}

// A status the registry doesn't know degrades to its live count — the
// explanation stays truthful.
func TestChipExplanationDegradesWithoutLore(t *testing.T) {
	e := combatEngine()
	e.CurrentEncounter = engine.Encounter{
		Type:     engine.EncounterCombat,
		Monsters: []entities.Monster{goblinMonster()},
	}
	m := New(e)
	m.mirror.enemyStatuses = []combatStatus{{name: "Mystery", kind: "debuff", turns: 4}}

	view := stripControls(stripANSI(m.combatPanel()))
	if !strings.Contains(view, "Mystery · 4 turns left") {
		t.Errorf("the unknown status lost its explanation:\n%s", view)
	}
}

// The hero's chips fill the HP row while they fit — the row never
// overflows the column and the panel keeps its four lines.
func TestHeroChipsNeverOverflowTheRow(t *testing.T) {
	m := New(combatEngine())
	for i := range 8 {
		m.mirror.heroStatuses = append(m.mirror.heroStatuses, combatStatus{
			name: fmt.Sprintf("Status %d", i), kind: "debuff", turns: 3,
		})
	}

	lines := viewLines(m.heroPanel())

	if len(lines) != 4 {
		t.Fatalf("hero panel has %d lines, want 4", len(lines))
	}
	if w := ansiLen(lines[2]); w > mainColWidth {
		t.Errorf("hero HP row is %d wide, overflows the %d-col column: %q", w, mainColWidth, lines[2])
	}
}
