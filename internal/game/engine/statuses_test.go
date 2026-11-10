package engine

import (
	"slices"
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
	damage := func(modifier int) game.MoveEffectAsset {
		return game.MoveEffectAsset{Type: "damage", Target: "enemy", DmgModifier: intPtr(modifier)}
	}
	status := func(id, target string, chance int) game.MoveEffectAsset {
		return game.MoveEffectAsset{Type: "status", Target: target, Status: &game.MoveStatusEffectAsset{ID: id, Chance: chance}}
	}
	return map[string]game.MoveAsset{
		"sure_poison": {ID: "sure_poison", Name: "Envenom", Tier: 1,
			Effects: []game.MoveEffectAsset{damage(100), status("poison", "enemy", 100)}, Cooldown: 0, Weight: 10},
		"never_poison": {ID: "never_poison", Name: "Dust Toss", Tier: 1,
			Effects: []game.MoveEffectAsset{damage(100), status("poison", "enemy", 0)}, Cooldown: 0, Weight: 10},
		"sure_weaken": {ID: "sure_weaken", Name: "Rattle", Tier: 1,
			Effects: []game.MoveEffectAsset{damage(100), status("weaken", "enemy", 100)}, Cooldown: 0, Weight: 10},
		"sure_slow": {ID: "sure_slow", Name: "Trip", Tier: 1,
			Effects: []game.MoveEffectAsset{damage(100), status("slow", "enemy", 100)}, Cooldown: 0, Weight: 10},
		"guard_strike": {ID: "guard_strike", Name: "Guard Strike", Tier: 1,
			Effects: []game.MoveEffectAsset{damage(75), status("guard", "self", 100)}, Cooldown: 2, Weight: 10},
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
		originalName := lore[0].Name
		originalID := lore[0].ID
		lore = append(lore, game.StatusAsset{ID: "injected"})
		lore[0].Name = "mangled"
		if _, ok := e.reg.Statuses["injected"]; ok {
			t.Error("mutating StatusLore's return reached the registry")
		}
		if got := e.reg.Statuses[originalID].Name; got != originalName {
			t.Errorf("mutating StatusLore's returned element changed registry name to %q; want %q", got, originalName)
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

		events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "sure_poison")})

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

		events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "guard_strike")})

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

		events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "never_poison")})

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
		stub := &stubRng{values: []int{1, 50, 0}}
		e.rng = stub

		events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "sure_poison")})

		if got := eventTypes(events); !typesEqual(got,
			EventMoveUsed, EventCombatDamageDealt, EventEnemyDefeated, EventGoldGained) {
			t.Fatalf("events = %v, want no status event on a kill", got)
		}
		if stub.calls != 3 {
			t.Errorf("rng calls = %d, want 3 (spread, crit, status roll on lethal hit)", stub.calls)
		}
	})

	t.Run("multiple effect statuses roll independent chances", func(t *testing.T) {
		e := statusesEngine()
		effects := []game.MoveEffectAsset{
			{Type: "status", Target: "enemy", Status: &game.MoveStatusEffectAsset{ID: "poison", Chance: 50}},
			{Type: "status", Target: "enemy", Status: &game.MoveStatusEffectAsset{ID: "weaken", Chance: 50}},
		}
		move := entities.NewMoveFromAsset(game.MoveAsset{ID: "double_status", Name: "Double Status", Effects: effects})
		enemy := monsterFromFixture("goblin")
		startEncounter(e, enemy)
		stub := &stubRng{values: []int{0, 99}}
		e.rng = stub

		events := e.resolveMoveEffects(&move, e.Player, &enemy, ActorHero)

		if got := eventTypes(events); !typesEqual(got, EventMoveUsed, EventStatusApplied) {
			t.Errorf("events = %v, want move-used and only the successful status application", got)
		}
		if stub.calls != 2 {
			t.Errorf("rng calls = %d, want 2 (one independent chance roll per status effect)", stub.calls)
		}
		if len(e.enemyCombat.statuses) != 1 || e.enemyCombat.statuses[0].status.ID != "poison" {
			t.Errorf("enemy statuses = %+v, want only poison", e.enemyCombat.statuses)
		}
	})
}

// Re-applying a status refreshes it to the LONGEST duration — never a
// stack: the same id replaces with max(turnsLeft, duration).
func TestStatusRefreshesToTheLongestDuration(t *testing.T) {
	// Envenom has a cooldown, so a basic-action round advances it before
	// round 3 reapplies the poison. The status refreshes to 3 before its
	// round-end tick.
	e := statusesEngine(1, 50, 0, 0, 1, 50, 1, 50, 0, 1, 50, 1, 50, 0, 0, 1, 50)
	heroWithDamage(e, 5)
	learn(e, "sure_poison")
	startEncounter(e, monsterFromFixture("goblin"))
	e.Process(Command{Type: CmdFight})
	e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "sure_poison")})
	e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "basic_attack")})

	events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "sure_poison")})

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
	// to 2. Basic-action rounds 2-3 let the status tick and expire while
	// the learned move is cooling down.
	e := statusesEngine(1, 50, 0, 0, 1, 50, 1, 50, 0, 1, 50, 1, 50, 0, 1, 50)
	heroWithDamage(e, 5)
	learn(e, "sure_poison", "never_poison")
	startEncounter(e, monsterFromFixture("goblin"))
	e.Process(Command{Type: CmdFight})

	events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "sure_poison")}) // round 1

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

	otherIndex := combatMoveIndex(e, "basic_attack")
	e.Process(Command{Type: CmdMove, Index: otherIndex})          // round 2: tick → 1
	events = e.Process(Command{Type: CmdMove, Index: otherIndex}) // round 3: tick → expired

	if got := eventTypes(events); !typesEqual(got,
		EventMoveUsed, EventCombatDamageDealt, EventMoveUsed, EventCombatDamageTaken,
		EventStatusDamage, EventStatusExpired) {
		t.Fatalf("round 3 events = %v, want [...taken status-damage status-expired]", got)
	}
	if got := len(e.enemyCombat.statuses); got != 0 {
		t.Errorf("enemy has %d statuses after the expiry, want 0", got)
	}
}

func TestMultipleStatusExpirationAndLethalDoTOrdering(t *testing.T) {
	poison := entities.NewStatusFromAsset(testStatuses()["poison"])
	poison.Duration = 1
	guard := entities.NewStatusFromAsset(testStatuses()["guard"])
	guard.Duration = 1

	t.Run("expiration follows application order", func(t *testing.T) {
		target := entities.NewMonsterFromAsset(game.MonsterAsset{ID: "rat", Name: "Rat", HP: 10})
		state := newCombatantState()
		state.applyStatus(poison)
		state.applyStatus(guard)
		events, dead := tickDots(&state, &target)
		if expired := expireStatStatuses(&state, &target); len(expired) != 0 {
			t.Fatal("stat expired during its application turn")
		}
		state.turn++
		events = append(events, expireStatStatuses(&state, &target)...)
		if dead || !typesEqual(eventTypes(events), EventStatusDamage, EventStatusExpired, EventStatusExpired) {
			t.Fatalf("tick events = %v, dead=%v", eventTypes(events), dead)
		}
		if events[1].Data["status"] != "Poison" || events[2].Data["status"] != "Guard" || len(state.statuses) != 0 {
			t.Errorf("expiration order/state = %v / %+v", []any{events[1].Data["status"], events[2].Data["status"]}, state.statuses)
		}
	})

	t.Run("lethal dot stops later dots but not stat expiration", func(t *testing.T) {
		target := entities.NewMonsterFromAsset(game.MonsterAsset{ID: "rat", Name: "Rat", HP: 2})
		state := newCombatantState()
		state.applyStatus(poison)
		state.applyStatus(guard)
		later := poison
		later.ID = "later"
		state.applyStatus(later)
		state.turn++
		events, dead := tickDots(&state, &target)
		if !dead || !typesEqual(eventTypes(events), EventStatusDamage) {
			t.Fatalf("lethal tick events = %v, dead=%v; want only the first DoT", eventTypes(events), dead)
		}
		if len(state.statuses) != 3 || state.statuses[0].remaining(state.turn) != 0 || state.statuses[2].remaining(state.turn) != 1 {
			t.Errorf("later status was ticked after lethal DoT: %+v", state.statuses)
		}
		if expired := expireStatStatuses(&state, &target); !typesEqual(eventTypes(expired), EventStatusExpired) || len(state.statuses) != 2 {
			t.Fatalf("stat bookkeeping after lethal tick = %v / %+v", eventTypes(expired), state.statuses)
		}
	})
}

func TestStatStatusDeadlines(t *testing.T) {
	for _, duration := range []int{1, 2} {
		t.Run(string(rune('0'+duration)), func(t *testing.T) {
			state := newCombatantState()
			state.turn = 1
			target := monsterFromFixture("goblin")
			status := entities.NewStatusFromAsset(testStatuses()["guard"])
			status.Duration = duration
			if got := state.applyStatus(status); got != duration {
				t.Fatalf("application lifetime = %d, want %d", got, duration)
			}
			for turn := 1; turn <= 1+duration; turn++ {
				state.turn = turn
				if flat, _ := state.statMods(entities.StatDEF); flat != 5 {
					t.Fatalf("turn %d: guard inactive before turn end", turn)
				}
				if events, dead := tickDots(&state, &target); dead || len(events) != 0 {
					t.Fatal("stat status produced a DoT tick")
				}
				events := expireStatStatuses(&state, &target)
				if turn < 1+duration && len(events) != 0 {
					t.Fatal("stat expired before its deadline")
				}
			}
			if len(state.statuses) != 0 {
				t.Fatal("stat remained after deadline")
			}
		})
	}
}

func TestStatRefreshUsesLaterDeadline(t *testing.T) {
	for _, duration := range []int{1, 3, 5} {
		state := newCombatantState()
		state.turn = 1
		status := entities.NewStatusFromAsset(testStatuses()["guard"])
		status.Duration = 4
		state.applyStatus(status)
		state.turn = 2
		status.Duration = duration
		want := max(5, 2+duration)
		if got := state.applyStatus(status); got != want-2 {
			t.Fatalf("refresh duration %d: lifetime = %d, want %d", duration, got, want-2)
		}
		if len(state.statuses) != 1 || state.statuses[0].expiresAt != want {
			t.Fatalf("refresh stacked or changed deadline: %+v", state.statuses)
		}
		flat, _ := state.statMods(entities.StatDEF)
		if flat != 5 {
			t.Fatalf("refresh stacked modifier: %d", flat)
		}
	}
}

func TestLethalDotsFollowCapturedInitiative(t *testing.T) {
	for _, tc := range []struct {
		name     string
		speed    int
		heroDies bool
	}{
		{"hero faster", 5, true},
		{"tie favors hero", 10, true},
		{"monster faster", 15, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := statusesEngine(1, 50, 0, 1, 50)
			if tc.speed > 10 {
				e.rng = &stubRng{values: []int{0, 1, 50, 1, 50}}
			}
			heroWithDamage(e, 0)
			e.Player.CurrentHP = 3
			monster := entities.NewMonsterFromAsset(game.MonsterAsset{ID: "rat", Name: "Rat", Difficulty: 1, HP: 3, Damage: 0, Speed: tc.speed})
			startEncounter(e, monster, monsterFromFixture("orc"))
			e.Process(Command{Type: CmdFight})
			poison := entities.NewStatusFromAsset(testStatuses()["poison"])
			e.heroCombat.applyStatus(poison)
			e.enemyCombat.applyStatus(poison)
			e.heroCombat.applyStatus(entities.Status{ID: "ending_guard", Stat: entities.StatDEF, StatAmount: 1, Duration: 1})
			events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "basic_attack")})
			var targets []any
			for _, event := range events {
				if event.Type == EventStatusDamage {
					targets = append(targets, event.Data["target"])
				}
			}
			wantTarget := "Rat"
			if tc.heroDies {
				wantTarget = "Hero"
			}
			if len(targets) != 1 || targets[0] != wantTarget {
				t.Fatalf("DoT targets = %v, want only %s", targets, wantTarget)
			}
			if (e.State == StateGameOver) != tc.heroDies {
				t.Fatalf("state = %v, hero death expected = %v", e.State, tc.heroDies)
			}
			if !tc.heroDies && e.heroCombat.statuses[0].remaining(e.heroCombat.turn) != poison.Duration {
				t.Fatal("skipped surviving hero DoT consumed a tick")
			}
			if !tc.heroDies && (len(e.heroCombat.statuses) != 1 || e.heroStat(entities.StatDEF) != 0) {
				t.Fatal("skipping the hero DoT also skipped unrelated stat expiration")
			}
		})
	}
}

func TestCombinedPercentageStatusCalculations(t *testing.T) {
	target := entities.NewMonsterFromAsset(game.MonsterAsset{ID: "boss", Name: "Boss", HP: 1000})
	dot := entities.NewStatusFromAsset(game.StatusAsset{ID: "bleed", Name: "Bleed", Kind: "debuff", Duration: 2,
		Effect: game.StatusEffectAsset{Type: "dot", Amount: 5, Percent: 3}})
	if got := statusDotDamage(dot, &target); got != 35 {
		t.Errorf("combined DoT = %d, want 35 (5 flat + 3%% of max HP, uncapped)", got)
	}
	target.TakeDmg(990)
	if got := statusDotDamage(dot, &target); got != 35 {
		t.Errorf("DoT after HP loss = %d, want 35 (uses MaxHP, uncapped)", got)
	}

	state := newCombatantState()
	state.applyStatus(entities.Status{ID: "flat", Stat: entities.StatDMG, StatAmount: -3})
	state.applyStatus(entities.Status{ID: "percent", Stat: entities.StatDMG, StatPercent: 25})
	flat, percent := state.statMods(entities.StatDMG)
	got := 40*max(0, 100+percent)/100 + flat
	if got != 47 {
		t.Errorf("combined stat = %d, want 47 (40 + 25%%, then -3)", got)
	}
}

func TestEffectiveStatAppliesStatusPercentagesToEquipmentInclusiveBaseline(t *testing.T) {
	e := statusesEngine()
	heroWithDamage(e, 40)
	// Equipment contributes +20 DMG, making the status-free baseline 60.
	e.Player.Weapon = &entities.Item{BonusDMG: 20}
	e.heroCombat.applyStatus(entities.Status{ID: "percent", Stat: entities.StatDMG, StatPercent: 25})
	e.heroCombat.applyStatus(entities.Status{ID: "flat", Stat: entities.StatDMG, StatAmount: -3})

	if got := e.heroStat(entities.StatDMG); got != 72 {
		t.Errorf("effective hero DMG = %d, want 72 ((40 + 20) × 125%% - 3)", got)
	}

	// Verify the production path also applies the lower bound after both
	// components, rather than relying only on a test-side formula.
	e.heroCombat = newCombatantState()
	e.heroCombat.applyStatus(entities.Status{ID: "percent", Stat: entities.StatDMG, StatPercent: -200})
	e.heroCombat.applyStatus(entities.Status{ID: "flat", Stat: entities.StatDMG, StatAmount: -3})
	if got := e.heroStat(entities.StatDMG); got != 1 {
		t.Errorf("effective hero DMG after negative percentage = %d, want clamped 1", got)
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

	events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "sure_poison")})

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

// A DoT defeat happens after both sides acted, so it advances existing hero
// cooldowns while still giving the move used that round its full cooldown.
func TestDoTKillAdvancesExistingCooldowns(t *testing.T) {
	e := statusesEngine(0, 99, 0, 0, 0, 99, 0, 99, 0, 0, 0, 99)
	heroWithDamage(e, 0)
	learn(e, "sure_poison", "guard_strike")
	monster := entities.NewMonsterFromAsset(game.MonsterAsset{
		ID: "fleabane", Name: "Fleabane", Difficulty: 1, HP: 7, Damage: 0, Speed: 5,
	})
	startEncounter(e, monster, monsterFromFixture("orc"))
	e.Process(Command{Type: CmdFight})
	e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "sure_poison")}) // poison; tick leaves the monster at 3 HP
	if hp := e.CurrentEncounter.Monsters[0].CurrentHP; hp != 3 {
		t.Fatalf("monster HP after poison setup round = %d, want 3", hp)
	}
	e.heroCombat.cooldowns["prior_move"] = 2

	events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "guard_strike")}) // guard strike + lethal poison tick
	if !slices.Contains(eventTypes(events), EventEnemyDefeated) {
		t.Fatalf("events = %v, want a DoT enemy defeat", eventTypes(events))
	}
	if got := e.heroCombat.cooldowns["prior_move"]; got != 1 {
		t.Errorf("existing cooldown after completed DoT-kill round = %d, want 1", got)
	}
	if got := e.heroCombat.cooldowns["guard_strike"]; got != 2 {
		t.Errorf("move used in DoT-kill round cooldown = %d, want full cooldown 2", got)
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

	events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "basic_attack")})

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
		// The goblin's weaken move applies to the hero; its damage modifier
		// must clamp the hero's zero base DMG to one.
		e := statusesEngine(1, 50, 0, 0, 1, 50)
		heroWithDamage(e, 0) // 0 − 3 → clamped to 1
		startEncounter(e, spitterWithMove("sure_weaken", 1))
		e.Process(Command{Type: CmdFight})
		// The monster applies Weaken, then the hero's turn computes effective
		// DMG with the debuff active.
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
		e := statusesEngine(1, 50, 0, 1, 50, 0, 0, 1, 50, 1, 1, 1)
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
	// kills it. The final turn still ticks poison before encounter cleanup.
	e := statusesEngine(1, 50, 0, 0, 1, 50, 1, 50, 0)
	heroWithDamage(e, 30)
	startEncounter(e, spitterWithPoison(1))
	e.Process(Command{Type: CmdFight})
	e.Process(Command{Type: CmdMove, Index: 0}) // hero poisoned (tick → 2)
	e.Process(Command{Type: CmdMove, Index: 0}) // spitter killed → victory

	if got := len(e.heroCombat.statuses); got != 0 {
		t.Fatalf("hero statuses after the encounter = %d, want cleared", got)
	}

	// A new combat resets everything.
	e.heroCombat.startCooldown(entities.Move{ID: "sure_poison", Cooldown: 2})
	if !e.heroCombat.onCooldown("sure_poison") {
		t.Fatal("test setup: sure_poison cooldown was not started")
	}
	startEncounter(e, monsterFromFixture("orc"))
	e.startCombat()
	if got := len(e.enemyCombat.statuses) + len(e.heroCombat.statuses); got != 0 {
		t.Errorf("a new combat started with %d active statuses, want 0", got)
	}
	if e.heroCombat.onCooldown("sure_poison") {
		t.Error("a new combat started with a locked move")
	}
}

// DoT recipient order follows initiative captured before either action.
func TestTickOrderCapturedInitiative(t *testing.T) {
	for _, tc := range []struct {
		name          string
		speed         int
		first, second string
	}{
		{"hero faster", 5, "Hero", "Spitter"},
		{"tie favors hero", 10, "Hero", "Spitter"},
		{"monster faster", 15, "Spitter", "Hero"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Both strikes carry poison; action and recipient order match
			// the initiative captured at turn start.
			e := statusesEngine(1, 50, 0, 0, 1, 50, 0)
			if tc.speed > 10 {
				e.rng = &stubRng{values: []int{0, 1, 50, 0, 1, 50, 0}}
			}
			heroWithDamage(e, 5)
			learn(e, "sure_poison")
			monster := entities.NewMonsterFromAsset(game.MonsterAsset{ID: "spitter", Name: "Spitter", Difficulty: 2, HP: 60, Damage: 1, Speed: tc.speed}, entities.NewMoveFromAsset(statusMoveAssets()["sure_poison"]))
			startEncounter(e, monster)
			e.Process(Command{Type: CmdFight})

			events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "sure_poison")})

			want := []EventType{EventMoveUsed, EventCombatDamageDealt, EventStatusApplied,
				EventMoveUsed, EventCombatDamageTaken, EventStatusApplied, EventStatusDamage, EventStatusDamage}
			if tc.speed > 10 {
				want[1], want[4] = want[4], want[1]
			}
			if got := eventTypes(events); !typesEqual(got, want...) {
				t.Fatalf("events = %v, want two status-damage ticks", got)
			}
			if target := events[6].Data["target"]; target != tc.first {
				t.Errorf("first tick target = %v, want %s", target, tc.first)
			}
			if target := events[7].Data["target"]; target != tc.second {
				t.Errorf("second tick target = %v, want %s", target, tc.second)
			}
		})
	}
}

func TestSpeedChangeDoesNotReorderCurrentTurn(t *testing.T) {
	e := statusesEngine(1, 50, 0, 0, 1, 50)
	heroWithDamage(e, 1)
	selfSlow := statusMoveAssets()["sure_slow"]
	selfSlow.ID = "self_slow"
	selfSlow.Effects[1].Target = "self"
	e.reg.Moves[selfSlow.ID] = selfSlow
	learn(e, "self_slow")
	monster := entities.NewMonsterFromAsset(game.MonsterAsset{ID: "rat", Name: "Rat", Difficulty: 1, HP: 50, Damage: 1, Speed: 12})
	startEncounter(e, monster)
	e.Process(Command{Type: CmdFight})
	// The hero begins faster but its own first action slows it below the monster.
	e.heroCombat.applyStatus(entities.Status{ID: "haste", Stat: entities.StatSPD, StatAmount: 3, Duration: 3})
	poison := entities.NewStatusFromAsset(testStatuses()["poison"])
	e.heroCombat.applyStatus(poison)
	e.enemyCombat.applyStatus(poison)
	events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "self_slow")})
	if events[0].Data["side"] != ActorHero {
		t.Fatalf("first action = %+v, want hero", events[0])
	}
	var targets []any
	for _, event := range events {
		if event.Type == EventStatusDamage {
			targets = append(targets, event.Data["target"])
		}
	}
	if len(targets) != 2 || targets[0] != "Hero" || targets[1] != "Rat" {
		t.Fatalf("same-turn slow reordered recipients: %v", targets)
	}
	if e.heroStat(entities.StatSPD) >= e.enemyStat(entities.StatSPD) {
		t.Fatal("fixture did not reverse effective initiative")
	}
	e.rng = &stubRng{values: []int{0, 1, 50, 1, 50}}
	events = e.Process(Command{Type: CmdMove, Index: 0})
	if events[0].Data["side"] != ActorEnemy {
		t.Fatalf("next turn ignored new initiative: %+v", events[0])
	}
}

func TestStatLifetimeContinuesAcrossKillingTurns(t *testing.T) {
	e := statusesEngine(1, 50)
	heroWithDamage(e, 100)
	startEncounter(e, monsterFromFixture("goblin"), monsterFromFixture("goblin"), monsterFromFixture("goblin"), monsterFromFixture("orc"))
	e.Process(Command{Type: CmdFight})
	guard := entities.NewStatusFromAsset(testStatuses()["guard"])
	// Apply during turn 1, as a move would; the following two killing
	// turns must count even though neither victim gets to act.
	e.combatTurn = 1
	e.heroCombat.turn = 1
	e.heroCombat.applyStatus(guard)
	for turn := 2; turn <= 3; turn++ {
		if e.heroStat(entities.StatDEF) != 5 {
			t.Fatal("Guard expired before its full turns")
		}
		e.Process(Command{Type: CmdMove, Index: 0})
		if e.combatTurn != turn || e.enemyCombat.turn != turn {
			t.Fatal("queue advancement reset clock")
		}
		if e.currentEnemy().HP() != e.currentEnemy().MaxHP() || len(e.enemyCombat.statuses) != 0 {
			t.Fatal("successor acted or inherited state")
		}
	}
	if e.heroStat(entities.StatDEF) != 0 || len(e.heroCombat.statuses) != 0 {
		t.Fatal("killing turns froze Guard expiration")
	}
}
