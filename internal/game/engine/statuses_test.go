package engine

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// testStatuses mirrors the shipped statuses.json starter set.
func testStatuses() map[string]game.StatusAsset {
	return map[string]game.StatusAsset{
		"poison": {ID: "poison", Name: "Poison", Kind: "debuff",
			Effect: game.StatusEffectAsset{Type: "dot", Amount: 3}, Duration: 3},
		"weaken": {ID: "weaken", Name: "Weaken", Kind: "debuff",
			Effect: game.StatusEffectAsset{Type: "stat", Stat: "dmg", Amount: -3}, Duration: 3},
		"slow": {ID: "slow", Name: "Slow", Kind: "debuff",
			Effect: game.StatusEffectAsset{Type: "stat", Stat: "spd", Amount: -3}, Duration: 3},
		"guard": {ID: "guard", Name: "Guard", Kind: "buff",
			Effect: game.StatusEffectAsset{Type: "stat", Stat: "def", Amount: 5}, Duration: 2},
	}
}

// statusMoveAssets are the deterministic application moves layered onto
// the registry by statusesEngine — also usable by monster fixtures.
func statusMoveAssets() map[string]game.MoveAsset {
	return map[string]game.MoveAsset{
		"sure_poison": {ID: "sure_poison", Name: "Envenom", Tier: 1,
			DmgModifier: 100, Cooldown: 0, Weight: 10,
			Status: &game.MoveStatusAsset{ID: "poison", Chance: 100, Target: "enemy"}},
		"never_poison": {ID: "never_poison", Name: "Dust Toss", Tier: 1,
			DmgModifier: 100, Cooldown: 0, Weight: 10,
			Status: &game.MoveStatusAsset{ID: "poison", Chance: 0, Target: "enemy"}},
		"sure_weaken": {ID: "sure_weaken", Name: "Rattle", Tier: 1,
			DmgModifier: 100, Cooldown: 0, Weight: 10,
			Status: &game.MoveStatusAsset{ID: "weaken", Chance: 100, Target: "enemy"}},
		"sure_slow": {ID: "sure_slow", Name: "Trip", Tier: 1,
			DmgModifier: 100, Cooldown: 0, Weight: 10,
			Status: &game.MoveStatusAsset{ID: "slow", Chance: 100, Target: "enemy"}},
		"guard_strike": {ID: "guard_strike", Name: "Guard Strike", Tier: 1,
			DmgModifier: 75, Cooldown: 2, Weight: 10,
			Status: &game.MoveStatusAsset{ID: "guard", Chance: 100, Target: "self"}},
	}
}

// statusesEngine is a move fixture engine that also carries the status
// assets plus deterministic application moves:
//   - sure_poison: neutral damage, poison 100% on the enemy, no cooldown;
//   - never_poison: poison 0% (the roll happens, never fires);
//   - sure_weaken / sure_slow: weaken/slow 100% on the enemy;
//   - guard_strike: 75% damage, cooldown 2, Guard 100% on the striker.
//
// The standard stub pattern is [spread, crit, status-roll] per strike
// plus [pick] before each monster strike: [1, 50, 0, 0, 1, 50].
func statusesEngine(rngValues ...int) *Engine {
	e := movesEngine(rngValues...)
	e.reg.Statuses = testStatuses()
	for id, asset := range statusMoveAssets() {
		e.reg.Moves[id] = asset
	}
	return e
}

// spitterWithPoison is a monster whose only move is sure_poison (chance
// 100, target enemy) — it poisons the HERO.
func spitterWithPoison(dmg int) entities.Monster {
	return spitterWithMove("sure_poison", dmg)
}

// spitterWithMove is a monster whose only move is the named status move
// (chance 100, target enemy = the hero).
func spitterWithMove(moveID string, dmg int) entities.Monster {
	move := entities.NewMoveFromAsset(statusMoveAssets()[moveID])
	move.Weight = 100
	return entities.NewMonsterFromAsset(
		game.MonsterAsset{ID: "spitter", Name: "Spitter", Difficulty: 2, HP: 60, Damage: dmg, Speed: 10},
		move,
	)
}

// StatusLore exposes the registry's statuses for lore/UI consumers (the
// TUI's "?" overlay): the assets sorted by id, as a fresh copy — and an
// empty registry is safe.
func TestStatusLoreReturnsTheRegistryStatuses(t *testing.T) {
	t.Run("the registry's statuses, sorted and copied", func(t *testing.T) {
		e := statusesEngine()

		lore := e.StatusLore()

		if len(lore) != len(e.reg.Statuses) {
			t.Fatalf("StatusLore = %d statuses, want %d", len(lore), len(e.reg.Statuses))
		}
		for i := 1; i < len(lore); i++ {
			if lore[i-1].ID >= lore[i].ID {
				t.Errorf("StatusLore not sorted by id: %v before %v", lore[i-1].ID, lore[i].ID)
			}
		}
		// A fresh slice: growing or mutating the return never reaches the
		// registry.
		lore = append(lore, game.StatusAsset{ID: "injected"})
		lore[0].Name = "mangled"
		if _, ok := e.reg.Statuses["injected"]; ok {
			t.Error("mutating StatusLore's return reached the registry")
		}
	})

	t.Run("an empty registry is safe", func(t *testing.T) {
		e := newTestEngine() // no statuses in the registry

		lore := e.StatusLore()

		if len(lore) != 0 {
			t.Errorf("StatusLore on an empty registry = %+v, want empty", lore)
		}
	})
}

// A landed hit from a status-carrying move applies the status to the
// move's target (enemy = the defender) with its full duration.
func TestStatusApplication(t *testing.T) {
	t.Run("enemy-targeted status lands on the defender", func(t *testing.T) {
		// Hero strike: spread 1 (5 dealt), crit 50 (no crit), status
		// roll 0 < 100 → Poison. Then the round-end tick.
		e := statusesEngine(1, 50, 0, 0, 1, 50)
		heroWithDamage(e, 5)
		learn(e, "sure_poison")
		startEncounter(e, monsterFromFixture("goblin"))
		e.Process(Command{Type: CmdFight})

		events := e.Process(Command{Type: CmdMove, Index: 0})

		if got := eventTypes(events); !typesEqual(got,
			EventMoveUsed, EventCombatDamageDealt, EventStatusApplied,
			EventMoveUsed, EventCombatDamageTaken, EventStatusDamage) {
			t.Fatalf("events = %v, want [move-used dealt status-applied move-used taken status-damage]", got)
		}
		applied := events[2]
		if applied.Data["target"] != "Goblin" {
			t.Errorf("target = %v, want Goblin", applied.Data["target"])
		}
		if applied.Data["status"] != "Poison" {
			t.Errorf("status = %v, want Poison", applied.Data["status"])
		}
		if applied.Data["turns"] != 3 {
			t.Errorf("turns = %v, want 3 (the status's duration)", applied.Data["turns"])
		}
		if got := len(e.enemyCombat.statuses); got != 1 {
			t.Fatalf("enemy has %d active statuses, want 1", got)
		}
	})

	t.Run("self-targeted status lands on the striker", func(t *testing.T) {
		// guard_strike: 75% of 5 → round(3.75) = 4; then Guard on the hero.
		e := statusesEngine(1, 50, 0, 0, 1, 50)
		heroWithDamage(e, 5)
		learn(e, "guard_strike")
		startEncounter(e, monsterFromFixture("goblin"))
		e.Process(Command{Type: CmdFight})

		events := e.Process(Command{Type: CmdMove, Index: 0})

		if got := eventTypes(events); !typesEqual(got,
			EventMoveUsed, EventCombatDamageDealt, EventStatusApplied,
			EventMoveUsed, EventCombatDamageTaken) {
			t.Fatalf("events = %v, want [move-used dealt status-applied move-used taken]", got)
		}
		applied := events[2]
		if applied.Data["target"] != "Hero" || applied.Data["status"] != "Guard" {
			t.Errorf("applied = %v/%v, want Hero/Guard", applied.Data["target"], applied.Data["status"])
		}
		// The guard is active immediately: the goblin's swing this very
		// round is mitigated by DEF 5 (2 → 1).
		if hp := e.Player.CurrentHP; hp != 99 {
			t.Errorf("hero HP = %d, want 99 (guard +5 DEF mitigated the same round)", hp)
		}
	})

	t.Run("a failed chance roll still consumes its rng slot", func(t *testing.T) {
		// never_poison: the status roll (0 < 0) happens and fails.
		e := statusesEngine(1, 50, 0, 0, 1, 50)
		heroWithDamage(e, 5)
		learn(e, "never_poison")
		startEncounter(e, monsterFromFixture("goblin"))
		e.Process(Command{Type: CmdFight})
		stub := &stubRng{values: []int{1, 50, 0, 0, 1, 50}}
		e.rng = stub

		events := e.Process(Command{Type: CmdMove, Index: 0})

		if got := eventTypes(events); !typesEqual(got,
			EventMoveUsed, EventCombatDamageDealt, EventMoveUsed, EventCombatDamageTaken) {
			t.Fatalf("events = %v, want no status event", got)
		}
		if got := len(e.enemyCombat.statuses); got != 0 {
			t.Errorf("enemy has %d statuses, want 0", got)
		}
		// Hero strike (spread, crit, status roll) + monster pick and
		// strike (pick, spread, crit).
		if stub.calls != 6 {
			t.Errorf("rng calls = %d, want 6 (the status roll happened even at 0%%)", stub.calls)
		}
	})

	t.Run("a plain move consumes no status roll", func(t *testing.T) {
		e := statusesEngine(1, 50, 0, 1, 50)
		heroWithDamage(e, 5)
		startEncounter(e, monsterFromFixture("goblin"))
		e.Process(Command{Type: CmdFight})
		stub := &stubRng{values: []int{1, 50, 0, 1, 50}}
		e.rng = stub

		e.Process(Command{Type: CmdMove, Index: 0}) // basic attack

		if stub.calls != 5 {
			t.Errorf("rng calls = %d, want 5 (spread+crit ×2 and the monster pick — no status roll)", stub.calls)
		}
	})

	t.Run("a killing blow cannot afflict the corpse", func(t *testing.T) {
		// The strike kills: the status roll fires (the rng slot is
		// consumed) but a dead target is not afflicted.
		e := statusesEngine(1, 50, 0)
		heroWithDamage(e, 40) // one-shots the goblin
		learn(e, "sure_poison")
		startEncounter(e, monsterFromFixture("goblin"), monsterFromFixture("orc"))
		e.Process(Command{Type: CmdFight})

		events := e.Process(Command{Type: CmdMove, Index: 0})

		if got := eventTypes(events); !typesEqual(got,
			EventMoveUsed, EventCombatDamageDealt, EventEnemyDefeated, EventGoldGained) {
			t.Fatalf("events = %v, want no status event on a kill", got)
		}
	})
}

// Re-applying a status refreshes it to the LONGEST duration — never a
// stack: the same id replaces with max(turnsLeft, duration).
func TestStatusRefreshesToTheLongestDuration(t *testing.T) {
	// Two rounds of Envenom: round 1 applies (3, ticked to 2 at round
	// end), round 2 re-applies → refreshed to 3.
	e := statusesEngine(1, 50, 0, 0, 1, 50, 1, 50, 0, 0, 1, 50)
	heroWithDamage(e, 5)
	learn(e, "sure_poison")
	startEncounter(e, monsterFromFixture("goblin"))
	e.Process(Command{Type: CmdFight})
	e.Process(Command{Type: CmdMove, Index: 0})

	events := e.Process(Command{Type: CmdMove, Index: 0})

	applied := events[2]
	if applied.Data["turns"] != 3 {
		t.Errorf("turns = %v, want 3 (refreshed to the longest duration)", applied.Data["turns"])
	}
	if got := len(e.enemyCombat.statuses); got != 1 {
		t.Errorf("enemy has %d statuses, want 1 (never stacked)", got)
	}
}

// The round-end tick: after both sides acted, every status deals its DoT
// then counts a round down; hitting 0 expires it with an event.
func TestStatusTicksAtRoundEnd(t *testing.T) {
	// Round 1: Envenom applies the poison (3 turns) — the tick drops it
	// to 2. Rounds 2-3: the status-less Dust Toss; the poison ticks at
	// each round end and expires after its third tick.
	e := statusesEngine(1, 50, 0, 0, 1, 50, 1, 50, 0, 0, 1, 50, 1, 50, 0, 0, 1, 50)
	heroWithDamage(e, 5)
	learn(e, "sure_poison", "never_poison")
	startEncounter(e, monsterFromFixture("goblin"))
	e.Process(Command{Type: CmdFight})

	events := e.Process(Command{Type: CmdMove, Index: 0}) // round 1

	dot := events[5]
	if dot.Type != EventStatusDamage {
		t.Fatalf("round-end event = %v, want EventStatusDamage", dot.Type)
	}
	if dot.Data["target"] != "Goblin" || dot.Data["status"] != "Poison" || dot.Data["amount"] != 3 {
		t.Errorf("dot payload = %v, want Goblin/Poison/3", dot.Data)
	}
	if hp, _ := dot.Data["target_hp_exact"].(int); hp != 22 {
		t.Errorf("target_hp_exact = %v, want 22 (25 after the hit, then the tick)", hp)
	}
	if got := len(e.enemyCombat.statuses); got != 1 {
		t.Fatalf("poison lost at round end: %d statuses", got)
	}
	if left := e.enemyCombat.statuses[0].turnsLeft; left != 2 {
		t.Errorf("turns left after the tick = %d, want 2", left)
	}

	e.Process(Command{Type: CmdMove, Index: 1})          // round 2: tick → 1
	events = e.Process(Command{Type: CmdMove, Index: 1}) // round 3: tick → expired

	if got := eventTypes(events); !typesEqual(got,
		EventMoveUsed, EventCombatDamageDealt, EventMoveUsed, EventCombatDamageTaken,
		EventStatusDamage, EventStatusExpired) {
		t.Fatalf("round 3 events = %v, want [...taken status-damage status-expired]", got)
	}
	if got := len(e.enemyCombat.statuses); got != 0 {
		t.Errorf("enemy has %d statuses after the expiry, want 0", got)
	}
}

// DoT damage is a plain clamp (no mitigation) and CAN kill: a monster
// dying to poison goes through the normal defeat path (XP, gold,
// queue/victory).
func TestDoTKillsTheMonster(t *testing.T) {
	e := statusesEngine(1, 50, 0, 0, 1, 50)
	heroWithDamage(e, 0) // chips for 1
	learn(e, "sure_poison")
	fleabane := entities.NewMonsterFromAsset(game.MonsterAsset{
		ID: "fleabane", Name: "Fleabane", Difficulty: 1, HP: 2, Damage: 0, Speed: 5,
	})
	startEncounter(e, fleabane)
	e.Process(Command{Type: CmdFight})

	events := e.Process(Command{Type: CmdMove, Index: 0})

	// Hero chips 1; the monster's 0-DMG swing still chips 1; the
	// round-end poison tick eats the last HP and kills.
	if got := eventTypes(events); !typesEqual(got,
		EventMoveUsed, EventCombatDamageDealt, EventStatusApplied,
		EventMoveUsed, EventCombatDamageTaken, EventStatusDamage,
		EventEnemyDefeated, EventGoldGained, EventVictory) {
		t.Fatalf("events = %v, want the DoT kill through the defeat path", got)
	}
	if dmg, _ := events[5].Data["amount"].(int); dmg != 1 {
		t.Errorf("dot amount = %v, want 1 (clamped to the remaining HP)", dmg)
	}
	if e.State != StateExploring || !e.CanAscend {
		t.Errorf("state = %v, ascend = %v, want a victory (the DoT kill counts)", e.State, e.CanAscend)
	}
	if e.Player.Xp != 4 { // round(4 × 1^1.5) = 4
		t.Errorf("xp = %d, want 4 (the kill XP was granted)", e.Player.Xp)
	}
}

// A hero dying to DoT at round end ends the game.
func TestDoTKillsTheHero(t *testing.T) {
	e := statusesEngine(1, 50, 0, 0, 1, 50)
	heroWithDamage(e, 5)
	e.Player.CurrentHP = 2
	// The spitter's sure_poison targets ITS enemy — the hero.
	startEncounter(e, spitterWithPoison(1))
	e.Process(Command{Type: CmdFight})

	events := e.Process(Command{Type: CmdMove, Index: 0})

	// Hero strikes; the spitter chips 1 (2 → 1) and poisons the hero;
	// the round-end tick eats the last HP.
	if got := eventTypes(events); !typesEqual(got,
		EventMoveUsed, EventCombatDamageDealt,
		EventMoveUsed, EventCombatDamageTaken, EventStatusApplied,
		EventStatusDamage, EventHeroDefeated) {
		t.Fatalf("events = %v, want the hero DoT death", got)
	}
	if e.State != StateGameOver {
		t.Errorf("state = %v, want StateGameOver (DoT kills)", e.State)
	}
}

// Stat-mod statuses feed the effective stats used for damage, mitigation,
// initiative and the flee formula — clamped (DMG ≥ 1, DEF/SPD ≥ 0).
func TestStatModStatuses(t *testing.T) {
	t.Run("weaken cuts the monster's damage", func(t *testing.T) {
		// Round 1: Rattle applies Weaken (−3 DMG); the goblin's
		// same-round swing drops from 2 to 1 (effDMG clamped at 1).
		e := statusesEngine(1, 50, 0, 0, 1, 50)
		heroWithDamage(e, 5)
		learn(e, "sure_weaken")
		startEncounter(e, monsterFromFixture("goblin"))
		e.Process(Command{Type: CmdFight})

		e.Process(Command{Type: CmdMove, Index: 0})

		if hp := e.Player.CurrentHP; hp != 99 {
			t.Errorf("hero HP = %d, want 99 (weakened swing 1)", hp)
		}
	})

	t.Run("weaken cannot floor the hero's damage below 1", func(t *testing.T) {
		e := statusesEngine(1, 50, 0, 0, 1, 50)
		heroWithDamage(e, 0) // 0 − 3 → clamped to 1
		learn(e, "sure_weaken")
		startEncounter(e, monsterFromFixture("goblin"))
		e.Process(Command{Type: CmdFight})
		e.Process(Command{Type: CmdMove, Index: 0})

		if got := e.heroStat(entities.StatDMG); got != 1 {
			t.Errorf("weakened hero DMG = %d, want 1 (clamped)", got)
		}
	})

	t.Run("slow flips the initiative", func(t *testing.T) {
		// Round 1 (tie): the spitter's Trip applies Slow (−3 SPD) to the
		// HERO after its strike. Round 2: the hero's 7 SPD loses to the
		// spitter's 10 — the spitter acts first.
		// r1: hero [spread, crit] + spitter [pick, spread, crit, status];
		// r2: spitter first [pick, spread, crit, status] + hero [spread, crit].
		e := statusesEngine(1, 50, 0, 1, 50, 0, 0, 1, 50, 1, 50, 50)
		heroWithDamage(e, 5)
		startEncounter(e, spitterWithMove("sure_slow", 1))
		e.Process(Command{Type: CmdFight})

		e.Process(Command{Type: CmdMove, Index: 0})           // round 1: the hero gets slowed
		events := e.Process(Command{Type: CmdMove, Index: 0}) // round 2

		if got := eventTypes(events); !typesEqual(got,
			EventMoveUsed, EventCombatDamageTaken, EventStatusApplied, EventMoveUsed, EventCombatDamageDealt) {
			t.Errorf("round 2 events = %v, want [move-used taken status-applied move-used dealt] (monster first)", got)
		}
	})

	t.Run("slow lowers the flee chance", func(t *testing.T) {
		// Round 1: the spitter slows the hero: SPD 7 vs 10 → chance 35.
		// A flee roll of 35 must FAIL (35 is not < 35) — with the base
		// speeds (50) it would have succeeded.
		// r1 slows the hero; the flee roll is 35 (chance 35 → fails) and
		// the free turn re-applies the slow.
		e := statusesEngine(1, 50, 0, 1, 50, 0, 35, 0, 1, 50, 1)
		heroWithDamage(e, 5)
		startEncounter(e, spitterWithMove("sure_slow", 1))
		e.Process(Command{Type: CmdFight})
		e.Process(Command{Type: CmdMove, Index: 0}) // slowed

		events := e.Process(Command{Type: CmdFlee})

		if got := eventTypes(events); !typesEqual(got, EventMessage, EventMoveUsed, EventCombatDamageTaken, EventStatusApplied) {
			t.Errorf("events = %v, want a failed flee (chance 35, roll 35) with the free turn", got)
		}
		if e.State != StateCombat {
			t.Errorf("state = %v, want StateCombat (the flee failed)", e.State)
		}
	})
}

// Statuses are combat-scoped: they never survive the encounter — a new
// combat starts both sides clean.
func TestStatusesResetPerEncounter(t *testing.T) {
	// Round 1: the spitter chips and poisons the hero. Round 2: the hero
	// kills it (the round ends early — the poison stays on the hero).
	e := statusesEngine(1, 50, 0, 0, 1, 50, 1, 50, 0)
	heroWithDamage(e, 30)
	startEncounter(e, spitterWithPoison(1))
	e.Process(Command{Type: CmdFight})
	e.Process(Command{Type: CmdMove, Index: 0}) // hero poisoned (tick → 2)
	e.Process(Command{Type: CmdMove, Index: 0}) // spitter killed → victory

	if got := len(e.heroCombat.statuses); got != 1 {
		t.Fatalf("hero statuses after the fight = %d, want 1 (poison)", got)
	}

	// A new combat resets everything.
	startEncounter(e, monsterFromFixture("orc"))
	e.startCombat()
	if got := len(e.enemyCombat.statuses) + len(e.heroCombat.statuses); got != 0 {
		t.Errorf("a new combat started with %d active statuses, want 0", got)
	}
	if e.heroCombat.onCooldown("sure_poison") {
		t.Error("a new combat started with a locked move")
	}
}

// Both sides afflicted at once: the round-end tick runs the HERO's
// statuses first (deterministic order), then the enemy's.
func TestTickOrderHeroFirst(t *testing.T) {
	// Both strikes are chance-100 poison: each side ends up afflicted,
	// and both ticks fire at round end — hero first.
	e := statusesEngine(1, 50, 0, 0, 1, 50)
	heroWithDamage(e, 5)
	learn(e, "sure_poison")
	startEncounter(e, spitterWithPoison(1))
	e.Process(Command{Type: CmdFight})

	events := e.Process(Command{Type: CmdMove, Index: 0})

	if got := eventTypes(events); !typesEqual(got,
		EventMoveUsed, EventCombatDamageDealt, EventStatusApplied,
		EventMoveUsed, EventCombatDamageTaken, EventStatusApplied,
		EventStatusDamage, EventStatusDamage) {
		t.Fatalf("events = %v, want two status-damage ticks", got)
	}
	if target := events[6].Data["target"]; target != "Hero" {
		t.Errorf("first tick target = %v, want Hero (hero afflictions tick first)", target)
	}
	if target := events[7].Data["target"]; target != "Spitter" {
		t.Errorf("second tick target = %v, want Spitter", target)
	}
}
