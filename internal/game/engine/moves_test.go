package engine

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// testMoves mirrors the shipped moves.json entries the combat tests
// exercise (hand-built registry — content validation is not in play).
// Types mirror the embedded content too (all four are creature- or
// both-typed: monster kits and shrine pools can carry them).
func testMoves() map[string]game.MoveAsset {
	damage := func(modifier, crit int) []game.MoveEffectAsset {
		return []game.MoveEffectAsset{{Type: "damage", Target: "enemy", DmgModifier: intPtr(modifier), CritBonus: intPtr(crit)}}
	}
	return map[string]game.MoveAsset{
		"basic_attack":     {ID: "basic_attack", Name: "Basic Attack", Tier: 1, Effects: damage(100, 0), Weight: 100, Type: "both"},
		"guard_strike":     {ID: "guard_strike", Name: "Guard Strike", Tier: 3, Effects: damage(75, 0), Cooldown: 2, Weight: 50, Type: "both", Learnable: true},
		"heavy_slash":      {ID: "heavy_slash", Name: "Heavy Slash", Tier: 2, Effects: damage(175, 0), Cooldown: 2, Weight: 60, Type: "both", Learnable: true},
		"quick_stab":       {ID: "quick_stab", Name: "Quick Stab", Tier: 1, Effects: damage(60, 20), Cooldown: 1, Weight: 100, Type: "both", Learnable: true},
		"assassins_strike": {ID: "assassins_strike", Name: "Assassin's Strike", Tier: 3, Effects: damage(110, 35), Cooldown: 3, Weight: 30, Type: "both"},
	}
}

// movesEngine is a test engine whose registry carries the move fixtures.
func movesEngine(rngValues ...int) *Engine {
	e := newTestEngine(rngValues...)
	e.reg.Moves = testMoves()
	return e
}

func TestRegistryResolvedUnarmedPunchAndMonsterBasicFallback(t *testing.T) {
	e := movesEngine()
	asset := game.MoveAsset{
		ID: "punch", Name: "Modded Punch", Type: "both", Tier: 1,
		Effects: []game.MoveEffectAsset{{Type: "damage", Target: "enemy", DmgModifier: intPtr(135), CritBonus: intPtr(7)}}, Cooldown: 1, Weight: 25,
	}
	e.reg.Moves[asset.ID] = asset

	heroMoves := e.CombatMoves()
	if len(heroMoves) != 1 || heroMoves[0].Name != "Modded Punch" || heroMoves[0].Effects[0].DmgModifier != 135 || heroMoves[0].Cooldown != 1 {
		t.Fatalf("hero unarmed action = %+v, want registry-defined Punch", heroMoves)
	}
	delete(e.reg.Moves, "punch")
	basic := game.MoveAsset{ID: "basic_attack", Name: "Modded Basic", Type: "both", Tier: 1,
		Effects: []game.MoveEffectAsset{{Type: "damage", Target: "enemy", DmgModifier: intPtr(135)}}, Weight: 25}
	e.reg.Moves[basic.ID] = basic
	monsterMoves, err := e.monsterMoves(game.MonsterAsset{ID: "rat"})
	if err != nil || len(monsterMoves) != 1 || monsterMoves[0].Name != "Modded Basic" || monsterMoves[0].Weight != 25 {
		t.Fatalf("monster fallback = %+v, err %v; want registry-defined basic attack", monsterMoves, err)
	}

	locked := entities.NewMoveFromAsset(game.MoveAsset{
		ID: "locked", Name: "Locked", Type: "both", Tier: 1,
		Effects: []game.MoveEffectAsset{{Type: "damage", Target: "enemy", DmgModifier: intPtr(100)}}, Cooldown: 2, Weight: 1,
	})
	startEncounter(e, entities.NewMonsterFromAsset(game.MonsterAsset{
		ID: "rat", Name: "Rat", Difficulty: 1, HP: 20,
	}, locked))
	e.State = StateCombat
	e.enemyCombat = newCombatantState()
	e.enemyCombat.cooldowns[locked.ID] = 2
	if got := e.enemyTurnMove(); got.ID != basic.ID || got.Name != basic.Name || got.Effects[0].DmgModifier != 135 {
		t.Errorf("all-locked enemy fallback = %+v, want registry-defined basic attack", got)
	}
}

func TestUtilityHealMoveResolvesWithoutDamageRollAndEnemyStillActs(t *testing.T) {
	e := movesEngine(0, 1, 50)
	flat, percent := 7, 10
	asset := game.MoveAsset{ID: "mend", Name: "Mend", Type: "both", Learnable: true, Cooldown: 1, Weight: 1, Effects: []game.MoveEffectAsset{{Type: "heal", Target: "self", Flat: &flat, Percent: &percent}}}
	e.reg.Moves[asset.ID] = asset
	learn(e, "mend")
	e.Player.CurrentHP = 80
	startEncounter(e, monsterFromFixture("goblin"))
	e.State = StateCombat
	e.heroCombat = newCombatantState()
	e.enemyCombat = newCombatantState()
	stub := &stubRng{values: []int{0, 1, 50}}
	e.rng = stub
	events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "mend")})
	if got := eventTypes(events); !typesEqual(got, EventMoveUsed, EventCombatHealed, EventMoveUsed, EventCombatDamageTaken) {
		t.Fatalf("events = %v, want utility use/heal followed by enemy action", got)
	}
	if got := e.Player.CurrentHP; got != 95 {
		t.Errorf("hero HP = %d, want 95 (heal 17, then enemy hit for 2)", got)
	}
	if stub.calls != 3 {
		t.Errorf("rng calls = %d, want 3 (no hero damage spread/crit)", stub.calls)
	}
	heal := events[1].Data
	if heal["healed"] != 17 || heal["hp_exact"] != 97 || heal["max_hp"] != 100 || heal["side"] != ActorHero || heal["move"] != "Mend" || heal["target"] != e.Player.Name() {
		t.Errorf("heal payload = %#v", heal)
	}
}

func TestUtilityMoveCooldownTicksOnCompletedRounds(t *testing.T) {
	flat := 7
	e := movesEngine(0, 1, 50, 1, 50, 0, 1, 50, 1, 50, 0, 1, 50)
	e.reg.Moves["mend"] = game.MoveAsset{
		ID: "mend", Name: "Mend", Type: "both", Learnable: true, Weight: 1, Cooldown: 2,
		Effects: []game.MoveEffectAsset{{Type: "heal", Target: "self", Flat: &flat}},
	}
	heroWithDamage(e, 5)
	learn(e, "mend", "quick_stab")
	startEncounter(e, monsterFromFixture("goblin"))
	e.Process(Command{Type: CmdFight})
	e.heroCombat.statuses = []activeStatus{{status: entities.Status{ID: "guard", Name: "Guard", Kind: entities.StatusBuff, Stat: entities.StatDEF, StatAmount: 5}, expiresAt: e.combatTurn + 3}}

	mendIndex := combatMoveIndex(e, "mend")
	events := e.Process(Command{Type: CmdMove, Index: mendIndex}) // Mend starts its full cooldown after its turn.
	if got := eventTypes(events); !typesEqual(got, EventMoveUsed, EventCombatHealed, EventMoveUsed, EventCombatDamageTaken) {
		t.Fatalf("utility round events = %v, want heal followed by enemy action", got)
	}
	if !e.heroCombat.onCooldown("mend") {
		t.Fatal("Mend should still be cooling down after its use round")
	}
	if got := e.heroCombat.cooldowns["mend"]; got != 2 {
		t.Errorf("Mend cooldown after use round = %d, want full value 2", got)
	}
	if got := e.heroCombat.statuses[0].remaining(e.heroCombat.turn); got != 2 {
		t.Errorf("Guard turns after utility round = %d, want 2", got)
	}

	blocked := e.Process(Command{Type: CmdMove, Index: mendIndex})
	if got := eventTypes(blocked); !typesEqual(got, EventMessage) {
		t.Fatalf("cooling utility events = %v, want blocked move only", got)
	}
	if got := e.heroCombat.cooldowns["mend"]; got != 2 {
		t.Errorf("blocked action changed Mend cooldown to %d, want 2", got)
	}

	// Spammable basic actions advance rounds without adding another
	// cooldown that would itself block the next action.
	basicIndex := combatMoveIndex(e, "basic_attack")
	events = e.Process(Command{Type: CmdMove, Index: basicIndex})
	if got := eventTypes(events); !typesEqual(got, EventMoveUsed, EventCombatDamageDealt, EventMoveUsed, EventCombatDamageTaken) {
		t.Fatalf("intervening round events = %v, want hero and enemy actions", got)
	}
	if !e.heroCombat.onCooldown("mend") || e.heroCombat.cooldowns["mend"] != 1 {
		t.Fatal("Mend should remain locked for one more completed round")
	}
	if got := e.heroCombat.statuses[0].remaining(e.heroCombat.turn); got != 1 {
		t.Errorf("Guard turns after second completed round = %d, want 1", got)
	}

	e.Process(Command{Type: CmdMove, Index: basicIndex})
	if e.heroCombat.onCooldown("mend") {
		t.Fatal("Mend should be available after its two locked rounds")
	}
	events = e.Process(Command{Type: CmdMove, Index: mendIndex})
	if got := eventTypes(events); !typesEqual(got, EventMoveUsed, EventCombatHealed, EventMoveUsed, EventCombatDamageTaken) {
		t.Fatalf("ready utility events = %v, want Mend followed by enemy action", got)
	}
}

func TestOrderedDamageAndHealEffects(t *testing.T) {
	t.Run("damage then self heal still resolves after lethal enemy damage", func(t *testing.T) {
		e := movesEngine(1, 50)
		heroWithDamage(e, 100)
		e.Player.CurrentHP = 50
		flat := 10
		move := entities.NewMoveFromAsset(game.MoveAsset{ID: "ordered", Name: "Ordered", Effects: []game.MoveEffectAsset{
			{Type: "damage", Target: "enemy", DmgModifier: intPtr(100)},
			{Type: "heal", Target: "self", Flat: &flat},
		}})
		enemy := monsterFromFixture("goblin")
		startEncounter(e, enemy)
		events := e.resolveMoveEffects(&move, e.Player, &enemy, ActorHero)
		if len(events) != 3 || events[1].Type != EventCombatDamageDealt || events[2].Type != EventCombatHealed {
			t.Fatalf("events = %v, want move-used, damage, heal in authored order", eventTypes(events))
		}
		if enemy.IsAlive() || e.Player.CurrentHP != 60 {
			t.Errorf("enemy alive=%v, hero HP=%d; want dead enemy and hero HP 60", enemy.IsAlive(), e.Player.CurrentHP)
		}
	})
	t.Run("damage and heal apply in their authored order", func(t *testing.T) {
		e := movesEngine(1, 50)
		heroWithDamage(e, 5)
		e.Player.CurrentHP = 50
		flat := 10
		move := entities.NewMoveFromAsset(game.MoveAsset{ID: "ordered", Name: "Ordered", Effects: []game.MoveEffectAsset{
			{Type: "heal", Target: "self", Flat: &flat},
			{Type: "damage", Target: "enemy", DmgModifier: intPtr(100)},
		}})
		enemy := monsterFromFixture("goblin")
		startEncounter(e, enemy)
		events := e.resolveMoveEffects(&move, e.Player, &enemy, ActorHero)
		if len(events) != 3 || events[1].Type != EventCombatHealed || events[2].Type != EventCombatDamageDealt {
			t.Fatalf("events = %v, want move-used, heal, damage", eventTypes(events))
		}
		if e.Player.CurrentHP != 60 || enemy.HP() != 25 {
			t.Errorf("hero HP=%d enemy HP=%d, want 60/25", e.Player.CurrentHP, enemy.HP())
		}
	})
}

func TestMonsterCombatHealEventReportsActualRestoreAndSide(t *testing.T) {
	e := movesEngine()
	flat := 10
	move := entities.NewMoveFromAsset(game.MoveAsset{ID: "mend", Name: "Mend", Effects: []game.MoveEffectAsset{{Type: "heal", Target: "self", Flat: &flat}}})
	enemy := monsterFromFixture("goblin")
	startEncounter(e, enemy)
	actor := e.currentEnemy()
	actor.TakeDmg(5)
	events := e.resolveMoveEffects(&move, actor, e.Player, ActorEnemy)
	if len(events) != 2 || events[1].Type != EventCombatHealed {
		t.Fatalf("events = %v, want move-used and combat-healed", eventTypes(events))
	}
	data := events[1].Data
	if data["target"] != actor.Name() || data["side"] != ActorEnemy || data["move"] != "Mend" || data["healed"] != 5 || data["hp_exact"] != actor.MaxHP() || data["max_hp"] != actor.MaxHP() {
		t.Errorf("monster heal payload = %#v", data)
	}
	full := e.resolveMoveEffects(&move, actor, e.Player, ActorEnemy)[1].Data
	if full["healed"] != 0 || full["hp_exact"] != actor.MaxHP() {
		t.Errorf("at-cap monster heal payload = %#v", full)
	}
}

func TestMonsterPureHealThroughEngineTurn(t *testing.T) {
	flat := 10
	healAsset := game.MoveAsset{
		ID: "mend", Name: "Mend", Type: "both", Weight: 1,
		Effects: []game.MoveEffectAsset{{Type: "heal", Target: "self", Flat: &flat}},
	}
	makeEnemy := func() entities.Monster {
		move := entities.NewMoveFromAsset(healAsset)
		return entities.NewMonsterFromAsset(game.MonsterAsset{
			ID: "healer", Name: "Healer", Difficulty: 1, HP: 20, Damage: 2, Speed: 10,
		}, move)
	}

	t.Run("ordinary hero round", func(t *testing.T) {
		e := movesEngine()
		e.Player.CurrentHP = 80
		startEncounter(e, makeEnemy())
		e.CurrentEncounter.Monsters[0].TakeDmg(5)
		e.Process(Command{Type: CmdFight})
		stub := &stubRng{values: []int{1, 50, 0}}
		e.rng = stub

		events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "punch")})
		if got := eventTypes(events); !typesEqual(got, EventMoveUsed, EventCombatDamageDealt, EventMoveUsed, EventCombatHealed) {
			t.Fatalf("events = %v, want hero action then monster heal", got)
		}
		if events[2].Data["move"] != "Mend" || events[2].Data["user"] != "Healer" {
			t.Errorf("enemy move event = %#v, want Healer using Mend", events[2].Data)
		}
		heal := events[3].Data
		if heal["target"] != "Healer" || heal["side"] != ActorEnemy || heal["move"] != "Mend" || heal["healed"] != 6 || heal["hp_exact"] != 20 || heal["max_hp"] != 20 {
			t.Errorf("monster heal payload = %#v, want actual 6 HP to Healer at 20/20", heal)
		}
		if events[3].Data["damage_dealt"] != nil || events[3].Data["crit"] != nil {
			t.Errorf("pure heal event contains damage fields: %#v", events[3].Data)
		}
		if stub.calls != 3 {
			t.Errorf("rng calls = %d, want 3 (hero strike and monster move pick; no enemy strike)", stub.calls)
		}
	})

	t.Run("failed flee grants a free enemy heal turn", func(t *testing.T) {
		e := movesEngine()
		startEncounter(e, makeEnemy())
		e.CurrentEncounter.Monsters[0].TakeDmg(5)
		e.Process(Command{Type: CmdFight})
		stub := &stubRng{values: []int{99, 0}}
		e.rng = stub

		events := e.Process(Command{Type: CmdFlee})
		if got := eventTypes(events); !typesEqual(got, EventMessage, EventMoveUsed, EventCombatHealed) {
			t.Fatalf("events = %v, want failed flee message then enemy heal", got)
		}
		if events[0].Data["message"] != "You failed to flee!" {
			t.Errorf("flee event = %#v, want failed flee", events[0].Data)
		}
		heal := events[2].Data
		if events[1].Data["move"] != "Mend" || events[1].Data["user"] != "Healer" || heal["target"] != "Healer" || heal["side"] != ActorEnemy || heal["move"] != "Mend" || heal["healed"] != 5 || heal["hp_exact"] != 20 {
			t.Errorf("enemy heal events = %#v / %#v, want Healer's actual 5 HP heal", events[1].Data, heal)
		}
		if stub.calls != 2 {
			t.Errorf("rng calls = %d, want 2 (failed flee and enemy move pick only)", stub.calls)
		}
	})
}

func intPtr(v int) *int { return &v }

// learn teaches the hero the named fixture moves in order (learning
// order = the effective-set order the CmdMove index points into).
func learn(e *Engine, ids ...string) {
	for _, id := range ids {
		asset, ok := e.reg.MoveByID(id)
		if !ok {
			panic("test fixture: unknown move " + id)
		}
		// This helper creates an eligible test move without coupling
		// mechanics tests to the shipped learnability classification.
		asset.Learnable = true
		if asset.Type != "creature" && asset.Type != "both" {
			asset.Type = "creature"
		}
		if asset.Cooldown <= 0 {
			asset.Cooldown = 1
		}
		if _, learned := e.Player.LearnMove(entities.NewMoveFromAsset(asset)); !learned {
			panic("test fixture: could not learn " + id)
		}
	}
}

// monsterWithMoves builds a monster whose move set is resolved from the
// registry like the engine does at spawn time: each ref's weight
// OVERRIDES the move asset's default weight.
func monsterWithMoves(asset game.MonsterAsset, refs ...game.MoveRef) entities.Monster {
	moves := make([]entities.Move, 0, len(refs))
	for _, ref := range refs {
		moveAsset, ok := testMoves()[ref.ID]
		if !ok {
			panic("test fixture: unknown move " + ref.ID)
		}
		m := entities.NewMoveFromAsset(moveAsset)
		m.Weight = ref.Weight // the ref's per-monster AI weight wins
		moves = append(moves, m)
	}
	return entities.NewMonsterFromAsset(asset, moves...)
}

// The move's dmg_modifier scales the striker's DMG before the spread:
// base = round(DMG × modifier / 100).
func TestMoveDamageModifier(t *testing.T) {
	// stub [1, 50] cycling: spread 1 → the base itself, crit roll 50 → no crit.
	e := movesEngine(1, 50, 0, 1, 50)
	heroWithDamage(e, 5)
	learn(e, "heavy_slash") // 175% → round(5 × 1.75) = 9
	startEncounter(e, monsterFromFixture("goblin"))
	e.Process(Command{Type: CmdFight})

	events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "heavy_slash")})

	if got := eventTypes(events); !typesEqual(got,
		EventMoveUsed, EventCombatDamageDealt, EventMoveUsed, EventCombatDamageTaken) {
		t.Fatalf("events = %v, want [move-used dealt move-used taken]", got)
	}
	if dmg := events[1].Data["damage_dealt"]; dmg != 9 {
		t.Errorf("damage_dealt = %v, want 9 (5 DMG × 175%%, rounded)", dmg)
	}
	if move := events[1].Data["move"]; move != "Heavy Slash" {
		t.Errorf("damage event move = %v, want Heavy Slash", move)
	}
}

// The move's crit_bonus adds percentage points to the striker's crit
// threshold for that strike.
func TestMoveCritBonus(t *testing.T) {
	t.Run("the bonus widens the crit window", func(t *testing.T) {
		// quick_stab: 60% dmg, +20 crit → threshold 10 + 20 = 30.
		// stub [1, 29]: spread 1 → base 3 (round(5 × 0.6)); roll 29 < 30 → crit.
		e := movesEngine(1, 29, 0, 1, 50)
		heroWithDamage(e, 5)
		learn(e, "quick_stab")
		startEncounter(e, monsterFromFixture("goblin"))
		e.Process(Command{Type: CmdFight})

		events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "quick_stab")})

		if dmg := events[1].Data["damage_dealt"]; dmg != 6 {
			t.Errorf("damage_dealt = %v, want 6 (3 base, crit doubled)", dmg)
		}
		if crit, _ := events[1].Data["crit"].(bool); !crit {
			t.Error("crit flag = false, want true (29 < 30)")
		}
	})

	t.Run("a roll at the boosted threshold is not a crit", func(t *testing.T) {
		e := movesEngine(1, 30, 0, 1, 50)
		heroWithDamage(e, 5)
		learn(e, "quick_stab")
		startEncounter(e, monsterFromFixture("goblin"))
		e.Process(Command{Type: CmdFight})

		events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "quick_stab")})

		if crit, _ := events[1].Data["crit"].(bool); crit {
			t.Error("roll 30 crit with threshold 30 — must be exclusive")
		}
	})
}

// The effective crit threshold clamps to [0, 100] at use time.
func TestMoveCritThresholdClamps(t *testing.T) {
	t.Run("a huge bonus clamps at 100", func(t *testing.T) {
		e := movesEngine(1, 99, 0, 1, 50) // 99 < 100 → crit on every roll but 100
		e.reg.Moves["overkill"] = game.MoveAsset{ID: "overkill", Name: "Overkill", Effects: []game.MoveEffectAsset{{Type: "damage", Target: "enemy", DmgModifier: intPtr(100), CritBonus: intPtr(200)}}, Weight: 1}
		heroWithDamage(e, 5)
		learn(e, "overkill")
		startEncounter(e, monsterFromFixture("goblin"))
		e.Process(Command{Type: CmdFight})

		events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "overkill")})

		if crit, _ := events[1].Data["crit"].(bool); !crit {
			t.Error("crit roll 99 with a clamped 100 threshold must crit")
		}
	})

	t.Run("a negative bonus clamps at 0", func(t *testing.T) {
		e := movesEngine(1, 0, 0, 1, 50) // roll 0 with threshold 0 must not crit
		e.reg.Moves["dull"] = game.MoveAsset{ID: "dull", Name: "Dull Blow", Effects: []game.MoveEffectAsset{{Type: "damage", Target: "enemy", DmgModifier: intPtr(100), CritBonus: intPtr(-50)}}, Weight: 1}
		heroWithDamage(e, 5)
		learn(e, "dull")
		startEncounter(e, monsterFromFixture("goblin"))
		e.Process(Command{Type: CmdFight})

		events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "dull")})

		if crit, _ := events[1].Data["crit"].(bool); crit {
			t.Error("crit fired with a clamped 0 threshold")
		}
	})
}

func TestMoveCooldown(t *testing.T) {
	t.Run("a used move is blocked on the very next turn", func(t *testing.T) {
		// stub [1, 50]: heavy_slash round → 9 dealt, hero takes 2.
		e := movesEngine(1, 50, 0, 1, 50)
		heroWithDamage(e, 5)
		learn(e, "heavy_slash", "quick_stab")
		startEncounter(e, monsterFromFixture("goblin"))
		e.Process(Command{Type: CmdFight})
		heavyIndex := combatMoveIndex(e, "heavy_slash")
		e.Process(Command{Type: CmdMove, Index: heavyIndex}) // heavy_slash locks for 2

		stub := &stubRng{values: []int{1, 50}}
		e.rng = stub
		events := e.Process(Command{Type: CmdMove, Index: heavyIndex}) // locked

		// A blocked action, not an error state: a message, no change.
		if got := eventTypes(events); !typesEqual(got, EventMessage) {
			t.Fatalf("events = %v, want [message]", got)
		}
		if msg := events[0].Data["message"]; msg != "Move on cooldown." {
			t.Errorf("message = %v, want %q", msg, "Move on cooldown.")
		}
		if stub.calls != 0 {
			t.Errorf("a blocked move consumed %d rng calls, want 0", stub.calls)
		}
		if hp := e.CurrentEncounter.Monsters[0].CurrentHP; hp != 21 {
			t.Errorf("goblin HP = %d, want 21 (nobody swung)", hp)
		}
		if hp := e.Player.CurrentHP; hp != 98 {
			t.Errorf("hero HP = %d, want 98 (nobody swung)", hp)
		}
		if e.State != StateCombat {
			t.Errorf("state = %v, want StateCombat (a blocked move changes nothing)", e.State)
		}
	})

	t.Run("the lock lasts exactly its cooldown's rounds", func(t *testing.T) {
		e := movesEngine(1, 50, 0, 1, 50) // every strike: spread 1, no crit
		heroWithDamage(e, 5)
		learn(e, "heavy_slash", "quick_stab")
		startEncounter(e, monsterFromFixture("goblin"))
		e.Process(Command{Type: CmdFight})

		heavyIndex, quickIndex := combatMoveIndex(e, "heavy_slash"), combatMoveIndex(e, "quick_stab")
		basicIndex := combatMoveIndex(e, "basic_attack")
		e.Process(Command{Type: CmdMove, Index: heavyIndex})            // round 1: heavy_slash → locked 2
		e.Process(Command{Type: CmdMove, Index: quickIndex})            // round 2: quick_stab (tick → 1)
		blocked := e.Process(Command{Type: CmdMove, Index: heavyIndex}) // round 3: still locked
		if got := eventTypes(blocked); !typesEqual(got, EventMessage) {
			t.Fatalf("round 3: heavy_slash unlocked one round early: %v", got)
		}
		e.Process(Command{Type: CmdMove, Index: basicIndex}) // round 3 resolves (tick → 0)

		events := e.Process(Command{Type: CmdMove, Index: heavyIndex}) // round 4: ready again
		if got := eventTypes(events); !typesEqual(got,
			EventMoveUsed, EventCombatDamageDealt, EventMoveUsed, EventCombatDamageTaken) {
			t.Fatalf("round 4 events = %v, want [move-used dealt move-used taken]", got)
		}
		if move := events[0].Data["move"]; move != "Heavy Slash" {
			t.Errorf("round 4 move = %v, want Heavy Slash (cooldown expired)", move)
		}
	})
}

// The spammable basic action remains usable while cooldown moves are locked.
func TestBasicActionRemainsUsableWhileLearnedMoveCools(t *testing.T) {
	e := movesEngine(1, 50, 0, 1, 50, 1, 50, 0, 1, 50, 1, 50, 0, 1, 50, 1, 50, 0, 1, 50)
	heroWithDamage(e, 5)
	learn(e, "heavy_slash")
	startEncounter(e, monsterFromFixture("goblin"))
	e.Process(Command{Type: CmdFight})
	heavyIndex := combatMoveIndex(e, "heavy_slash")
	basicIndex := combatMoveIndex(e, "basic_attack")
	e.Process(Command{Type: CmdMove, Index: heavyIndex}) // round 1: locked for 2

	blocked := e.Process(Command{Type: CmdMove, Index: heavyIndex})
	if got := eventTypes(blocked); !typesEqual(got, EventMessage) {
		t.Fatalf("locked Heavy Slash events = %v, want a blocking message", got)
	}
	if e.heroCombat.cooldowns["heavy_slash"] != 2 {
		t.Fatal("blocked selection advanced the cooldown")
	}

	e.Process(Command{Type: CmdMove, Index: basicIndex}) // tick → 1
	e.Process(Command{Type: CmdMove, Index: basicIndex}) // tick → ready
	if e.heroCombat.onCooldown("heavy_slash") {
		t.Fatal("Heavy Slash should be ready after two completed rounds")
	}
	events := e.Process(Command{Type: CmdMove, Index: heavyIndex})
	if got := eventTypes(events); !typesEqual(got,
		EventMoveUsed, EventCombatDamageDealt, EventMoveUsed, EventCombatDamageTaken) {
		t.Fatalf("ready move events = %v, want [move-used dealt move-used taken]", got)
	}
	if dmg := events[1].Data["damage_dealt"]; dmg != 9 {
		t.Errorf("damage_dealt = %v, want 9", dmg)
	}
}

// Killing turns advance old locks but preserve the used move's full lock.
func TestKillingTurnAdvancesOldCooldownsAndLocksUsedMove(t *testing.T) {
	e := movesEngine()
	heroWithDamage(e, 40) // one-shots the goblin with any move
	learn(e, "heavy_slash", "quick_stab")
	startEncounter(e, monsterFromFixture("goblin"), monsterFromFixture("orc"))
	e.Process(Command{Type: CmdFight})
	e.heroCombat.cooldowns["quick_stab"] = 2

	e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "heavy_slash")})
	if got := e.heroCombat.cooldowns["quick_stab"]; got != 1 {
		t.Errorf("old cooldown = %d, want 1 after killing turn", got)
	}
	if got := e.heroCombat.cooldowns["heavy_slash"]; got != 2 {
		t.Errorf("used cooldown = %d, want full lock 2", got)
	}

	events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "heavy_slash")}) // next enemy
	if got := eventTypes(events); !typesEqual(got, EventMessage) {
		t.Errorf("events = %v, want [message] — heavy_slash must still be locked", got)
	}
}

func TestZeroCooldownKillAdvancesOldLocksWithoutLockingBasicAttack(t *testing.T) {
	e := movesEngine(1, 50)
	heroWithDamage(e, 40)
	startEncounter(e, monsterFromFixture("goblin"), monsterFromFixture("orc"))
	e.Process(Command{Type: CmdFight})
	e.heroCombat.cooldowns["heavy_slash"] = 1
	e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "basic_attack")})
	if e.heroCombat.onCooldown("heavy_slash") || e.heroCombat.onCooldown("basic_attack") {
		t.Errorf("killing turn should expire old lock and leave basic attack ready: %v", e.heroCombat.cooldowns)
	}
	if len(e.CurrentEncounter.Monsters) != 1 || e.CurrentEncounter.Monsters[0].CurrentHP != 50 {
		t.Fatal("queued successor should remain untouched until its own turn")
	}
}

// A fresh hero starts with the equipped wooden sword's basic action and
// no learned moves. Without a weapon, the engine supplies registry-backed
// virtual Punch first, then learned cooldown moves.
func TestFreshHeroHasOneBasicActionAndNoLearnedMoves(t *testing.T) {
	makeRegistry := func(withWeapon, withPunch bool) *game.Registry {
		reg := &game.Registry{Monsters: testMonsters(), Items: map[string]game.ItemAsset{}, Moves: testMoves(), Sections: testSections()}
		if withWeapon {
			reg.Items[starterItemID] = game.ItemAsset{
				ID: "wooden_sword", Name: "Wooden Sword", Slot: "weapon", BonusDMG: 5,
				Durability: 50, Moves: []string{"basic_attack"},
			}
			reg.Moves["basic_attack"] = game.MoveAsset{ID: "basic_attack", Name: "Basic Strike", Type: "both", Effects: []game.MoveEffectAsset{{Type: "damage", Target: "enemy", DmgModifier: intPtr(100)}}}
		}
		if withPunch {
			reg.Moves["punch"] = game.MoveAsset{ID: "punch", Name: "Custom Punch", Type: "both", Effects: []game.MoveEffectAsset{{Type: "damage", Target: "enemy", DmgModifier: intPtr(80)}}}
		}
		return reg
	}

	t.Run("starter weapon is the first action and learned set starts empty", func(t *testing.T) {
		e := NewEngine(makeRegistry(true, false))
		if got := e.Player.LearnedMoves(); len(got) != 0 {
			t.Fatalf("new hero learned moves = %+v, want none", got)
		}
		moves := e.CombatMoves()
		if len(moves) != 1 || moves[0].ID != "basic_attack" {
			t.Errorf("combat actions = %+v, want [basic_attack]", moves)
		}
		sourced := e.CombatMovesWithSource()
		if len(sourced) != 1 || sourced[0].Source != entities.SourceWeapon {
			t.Errorf("sourced actions = %+v, want one weapon-source action", sourced)
		}
	})

	t.Run("weaponless action resolves Punch from registry before learned moves", func(t *testing.T) {
		e := NewEngine(makeRegistry(false, true))
		learn(e, "quick_stab")
		moves := e.CombatMoves()
		if len(moves) != 2 || moves[0].ID != "punch" || moves[0].Name != "Custom Punch" || moves[0].Effects[0].DmgModifier != 80 || moves[1].ID != "quick_stab" {
			t.Errorf("combat actions = %+v, want registry Punch then learned quick_stab", moves)
		}
		sourced := e.CombatMovesWithSource()
		if len(sourced) != 2 || sourced[0].Source != entities.SourceWeapon || sourced[1].Source != entities.SourceLearned {
			t.Errorf("sourced actions = %+v, want basic action then learned action", sourced)
		}
	})

	t.Run("canonical Punch is the fallback when registry definition is absent", func(t *testing.T) {
		e := NewEngine(makeRegistry(false, false))
		moves := e.CombatMoves()
		if len(moves) != 1 || moves[0].ID != entities.Punch().ID || moves[0].Name != entities.Punch().Name {
			t.Errorf("combat actions = %+v, want canonical Punch", moves)
		}
	})
}

// Weapon-granted moves ride the equipped weapon: the engine resolves the
// asset's move ids at item build time and the set reaches CombatMoves.
func TestWeaponGrantedMovesAreUsable(t *testing.T) {
	e := movesEngine(1, 50, 0, 1, 50)
	heroWithDamage(e, 5)
	weapon := e.itemFromAsset(game.ItemAsset{
		ID: "fang_blade", Name: "Fang Blade", Slot: "weapon", BonusDMG: 3,
		Moves: []string{"quick_stab"},
	})
	e.Player.EquipItem(&weapon)

	moves := e.CombatMoves()
	if len(moves) != 1 || moves[0].ID != "quick_stab" {
		t.Fatalf("combat moves = %v, want [quick_stab] (resolved from the weapon)", moves)
	}

	startEncounter(e, monsterFromFixture("goblin"))
	e.Process(Command{Type: CmdFight})

	events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "quick_stab")})

	// quick_stab: 60% of (5 hero + 3 weapon = 8) → round(4.8) = 5.
	if dmg := events[1].Data["damage_dealt"]; dmg != 5 {
		t.Errorf("damage_dealt = %v, want 5 (8 DMG × 60%%)", dmg)
	}
}

// Every strike reports the move it was made with: EventMoveUsed carries
// the user and the move name, and the damage payloads grow the additive
// "move" key.
func TestMoveUsedEventPayload(t *testing.T) {
	e := movesEngine(1, 50, 0, 1, 50)
	heroWithDamage(e, 5)
	learn(e, "heavy_slash")
	startEncounter(e, monsterFromFixture("goblin"))
	e.Process(Command{Type: CmdFight})

	events := e.Process(Command{Type: CmdMove, Index: combatMoveIndex(e, "heavy_slash")})

	used := events[0]
	if used.Type != EventMoveUsed {
		t.Fatalf("first event = %v, want EventMoveUsed", used.Type)
	}
	if user := used.Data["user"]; user != "Hero" {
		t.Errorf("user = %v, want Hero", user)
	}
	if move := used.Data["move"]; move != "Heavy Slash" {
		t.Errorf("move = %v, want Heavy Slash", move)
	}
	// The enemy's strike reports its move too (until the monster AI
	// lands, always the canonical basic attack).
	if move := events[2].Data["move"]; move != "Basic Strike" {
		t.Errorf("enemy move = %v, want Basic Strike", move)
	}
	if user := events[2].Data["user"]; user != "Goblin" {
		t.Errorf("enemy user = %v, want Goblin", user)
	}
}

// Encounter generation resolves each spawned monster's move refs through
// the registry, with the ref's weight OVERRIDING the move asset's
// default AI weight for that monster.
func TestGenerateEncounterResolvesMonsterMoves(t *testing.T) {
	e := newTestEngine(0) // always the cheap goblin
	e.reg.Moves = testMoves()
	e.reg.Monsters["goblin"] = game.MonsterAsset{
		ID: "goblin", Name: "Goblin", Difficulty: 3, HP: 30, Damage: 2, Speed: 10,
		Moves: []game.MoveRef{
			{ID: "heavy_slash", Weight: 7}, // asset weight 60 → overridden by 7
			{ID: "quick_stab", Weight: 3},  // asset weight 100 → overridden by 3
		},
	}

	encounter, err := e.generateEncounter(1)
	if err != nil {
		t.Fatalf("generateEncounter: %v", err)
	}

	moves := encounter.Monsters[0].Moves()
	if len(moves) != 2 || moves[0].ID != "heavy_slash" || moves[1].ID != "quick_stab" {
		t.Fatalf("resolved moves = %v, want [heavy_slash quick_stab] in ref order", moves)
	}
	if moves[0].Weight != 7 || moves[1].Weight != 3 {
		t.Errorf("weights = %d/%d, want 7/3 (the ref weight overrides the asset's)", moves[0].Weight, moves[1].Weight)
	}
}

// A monster referencing an unknown move id fails encounter generation —
// a content bug that load-time validation otherwise catches.
func TestGenerateEncounterRejectsUnknownMoveRef(t *testing.T) {
	e := newTestEngine(0)
	e.reg.Moves = testMoves()
	e.reg.Monsters["goblin"] = game.MonsterAsset{
		ID: "goblin", Name: "Goblin", Difficulty: 3, HP: 30, Damage: 2, Speed: 10,
		Moves: []game.MoveRef{{ID: "no_such_move", Weight: 1}},
	}

	if _, err := e.generateEncounter(1); err == nil {
		t.Error("generateEncounter accepted an unknown move ref")
	}
}

// Monster AI v1: the monster picks by weight among its COOLDOWN-FILTERED
// moves — ONE Intn over the filtered total weight, per turn.
func TestMonsterPicksMovesByWeight(t *testing.T) {
	// A fixed pair: heavy_slash (weight 60, cooldown 2) then quick_stab
	// (weight 100) — total 160.
	buildMonster := func() entities.Monster {
		return monsterWithMoves(
			game.MonsterAsset{ID: "brute", Name: "Brute", Difficulty: 3, HP: 60, Damage: 2, Speed: 10},
			game.MoveRef{ID: "heavy_slash", Weight: 60},
			game.MoveRef{ID: "quick_stab", Weight: 100},
		)
	}

	t.Run("the pick is one Intn over the filtered total", func(t *testing.T) {
		e := movesEngine(1, 50) // hero strike: spread 1, no crit
		heroWithDamage(e, 5)
		startEncounter(e, buildMonster())
		e.Process(Command{Type: CmdFight})
		stub := &stubRng{values: []int{0, 0, 0}}
		e.rng = stub

		events := e.Process(Command{Type: CmdMove, Index: 0}) // hero + monster turn

		// rng 0 lands in the first bucket: heavy_slash.
		if move := events[2].Data["move"]; move != "Heavy Slash" {
			t.Errorf("monster move = %v, want Heavy Slash (rng 0 over weights [60, 100])", move)
		}
		// Hero spread + crit (2), the monster's ONE pick, then the
		// monster strike's spread + crit (2).
		if stub.calls != 5 {
			t.Errorf("rng calls = %d, want 5 (the monster pick is one Intn)", stub.calls)
		}
	})

	t.Run("a lock filters the pick without consuming rng", func(t *testing.T) {
		e := movesEngine(1, 50)
		heroWithDamage(e, 5)
		startEncounter(e, buildMonster())
		e.Process(Command{Type: CmdFight})
		stub := &stubRng{values: []int{0, 0, 0, 0, 0}}
		e.rng = stub

		e.Process(Command{Type: CmdMove, Index: 0}) // monster uses heavy_slash (locks 2)

		events := e.Process(Command{Type: CmdMove, Index: 0}) // heavy_slash filtered out

		// The second pick runs over only quick_stab (total 100): rng 0 →
		// Quick Stab — and NO extra roll was consumed by the filtering.
		if move := events[2].Data["move"]; move != "Quick Stab" {
			t.Errorf("monster move = %v, want Quick Stab (heavy_slash is locked)", move)
		}
	})
}

// When every monster move is locked the monster falls back to the
// canonical basic attack WITHOUT consuming a pick roll — there is
// nothing to pick.
func TestMonsterAllMovesLockedUsesBasicAttackWithoutRng(t *testing.T) {
	// One stub per resolved round: [spread, crit, pick, spread, crit].
	e := movesEngine(1, 50, 0, 1, 50, 1, 50, 0, 1, 50)
	heroWithDamage(e, 5)
	startEncounter(e, monsterWithMoves(
		game.MonsterAsset{ID: "brute", Name: "Brute", Difficulty: 3, HP: 900, Damage: 2, Speed: 10},
		game.MoveRef{ID: "heavy_slash", Weight: 60},      // cooldown 2
		game.MoveRef{ID: "assassins_strike", Weight: 30}, // cooldown 3
	))
	e.Process(Command{Type: CmdFight})

	e.Process(Command{Type: CmdMove, Index: 0}) // monster uses heavy_slash (first bucket)
	e.Process(Command{Type: CmdMove, Index: 0}) // monster uses assassins_strike (heavy_slash filtered)

	stub := &stubRng{values: []int{0, 0}}
	e.rng = stub
	events := e.Process(Command{Type: CmdMove, Index: 0}) // everything locked

	if got := eventTypes(events); !typesEqual(got, EventMoveUsed, EventCombatDamageDealt, EventMoveUsed, EventCombatDamageTaken) {
		t.Fatalf("events = %v, want [move-used dealt move-used taken]", got)
	}
	if move := events[2].Data["move"]; move != "Basic Attack" {
		t.Errorf("monster move = %v, want registry-defined basic_attack name", move)
	}
	// The fallback saves the PICK roll: two strikes' worth of rolls
	// (spread + crit each) — one less than a picked turn would consume.
	if stub.calls != 4 {
		t.Errorf("rng calls = %d, want 4 (two strikes, no pick roll)", stub.calls)
	}
}

// A locked fallback never traps the monster: cooldowns tick at round
// end, so the locked moves free up again.
func TestMonsterCooldownsRecover(t *testing.T) {
	e := movesEngine(1, 2, 0)
	heroWithDamage(e, 5)
	startEncounter(e, monsterWithMoves(
		game.MonsterAsset{ID: "brute", Name: "Brute", Difficulty: 3, HP: 900, Damage: 2, Speed: 10},
		game.MoveRef{ID: "heavy_slash", Weight: 100}, // the only move, cooldown 2
	))
	e.Process(Command{Type: CmdFight})
	e.Process(Command{Type: CmdMove, Index: 0}) // heavy_slash used → locked 2

	if !e.enemyCombat.onCooldown("heavy_slash") {
		t.Fatal("heavy_slash is not locked right after use")
	}

	// Two completed rounds tick the lock down (the monster falls back to
	// the basic attack without a pick in the meantime)...
	e.Process(Command{Type: CmdMove, Index: 0})
	e.Process(Command{Type: CmdMove, Index: 0})
	if e.enemyCombat.onCooldown("heavy_slash") {
		t.Fatal("heavy_slash is still locked after two full rounds")
	}

	// ...and the move is picked again.
	events := e.Process(Command{Type: CmdMove, Index: 0})

	if move := events[2].Data["move"]; move != "Heavy Slash" {
		t.Errorf("monster move after the cooldown = %v, want Heavy Slash again", move)
	}
}

func TestMonsterAIPrioritiesAndFallback(t *testing.T) {
	makeAIEngine := func(t *testing.T, hpDamage int, rules []game.MonsterPriorityAsset, fallback []string, moves ...game.MoveRef) (*Engine, *stubRng) {
		t.Helper()
		e := movesEngine()
		asset := game.MonsterAsset{
			ID: "ai_monster", Name: "AI Monster", Difficulty: 1, HP: 100, Damage: 2, Speed: 10,
			AI: &game.MonsterAIAsset{Priorities: rules, Fallback: fallback},
		}
		e.reg.Monsters[asset.ID] = asset
		enemy := monsterWithMoves(asset, moves...)
		startEncounter(e, enemy)
		e.enemyCombat = newCombatantState()
		if hpDamage > 0 {
			e.currentEnemy().TakeDmg(hpDamage)
		}
		stub := &stubRng{values: []int{0}}
		e.rng = stub
		return e, stub
	}
	condition := func(kind, target string, percent int, status string) game.MonsterConditionAsset {
		return game.MonsterConditionAsset{Type: kind, Target: target, Percent: percent, StatusID: status}
	}

	t.Run("hp threshold is strict and conditions use current exact hp", func(t *testing.T) {
		rule := game.MonsterPriorityAsset{When: []game.MonsterConditionAsset{condition("hp_below", "self", 35, "")}, Moves: []string{"heavy_slash"}}
		for _, tc := range []struct {
			damage int
			want   string
		}{{65, "Quick Stab"}, {66, "Heavy Slash"}} {
			e, stub := makeAIEngine(t, tc.damage, []game.MonsterPriorityAsset{rule}, []string{"quick_stab"}, game.MoveRef{ID: "heavy_slash", Weight: 1}, game.MoveRef{ID: "quick_stab", Weight: 1})
			if got := e.enemyTurnMove().Name; got != tc.want {
				t.Errorf("at %d damage selected %q, want %q", tc.damage, got, tc.want)
			}
			if stub.calls != 1 {
				t.Errorf("pick calls = %d, want one", stub.calls)
			}
		}
	})

	t.Run("all conditions must match and first applicable rule wins", func(t *testing.T) {
		rules := []game.MonsterPriorityAsset{
			{When: []game.MonsterConditionAsset{condition("status_present", "self", 0, "breachfire"), condition("status_absent", "enemy", 0, "poison")}, Moves: []string{"heavy_slash"}},
			{When: []game.MonsterConditionAsset{condition("status_present", "self", 0, "breachfire")}, Moves: []string{"assassins_strike"}},
		}
		e, stub := makeAIEngine(t, 0, rules, []string{"quick_stab"}, game.MoveRef{ID: "heavy_slash", Weight: 1}, game.MoveRef{ID: "assassins_strike", Weight: 1}, game.MoveRef{ID: "quick_stab", Weight: 1})
		e.enemyCombat.applyStatus(entities.Status{ID: "breachfire", Duration: 2})
		if got := e.enemyTurnMove().ID; got != "heavy_slash" {
			t.Errorf("first matching rule selected %q, want heavy_slash", got)
		}
		if stub.calls != 1 {
			t.Errorf("condition evaluation consumed RNG: calls=%d, want 1 pick", stub.calls)
		}
	})

	t.Run("cooldown-blocked priority allows lower rule", func(t *testing.T) {
		rules := []game.MonsterPriorityAsset{
			{When: []game.MonsterConditionAsset{condition("status_present", "self", 0, "breachfire")}, Moves: []string{"heavy_slash"}},
			{When: []game.MonsterConditionAsset{condition("status_present", "self", 0, "breachfire")}, Moves: []string{"assassins_strike"}},
		}
		e, stub := makeAIEngine(t, 0, rules, []string{"quick_stab"}, game.MoveRef{ID: "heavy_slash", Weight: 1}, game.MoveRef{ID: "assassins_strike", Weight: 1}, game.MoveRef{ID: "quick_stab", Weight: 1})
		e.enemyCombat.applyStatus(entities.Status{ID: "breachfire", Duration: 2})
		e.enemyCombat.cooldowns["heavy_slash"] = 1
		if got := e.enemyTurnMove().ID; got != "assassins_strike" {
			t.Errorf("selected %q, want lower-priority ready move", got)
		}
		if stub.calls != 1 {
			t.Errorf("pick calls = %d, want one", stub.calls)
		}
	})

	t.Run("configured fallback only and preserves weighted pick", func(t *testing.T) {
		e, stub := makeAIEngine(t, 0, nil, []string{"quick_stab"}, game.MoveRef{ID: "heavy_slash", Weight: 100}, game.MoveRef{ID: "quick_stab", Weight: 1})
		if got := e.enemyTurnMove().ID; got != "quick_stab" {
			t.Errorf("fallback selected %q, want only configured quick_stab", got)
		}
		if stub.calls != 1 {
			t.Errorf("pick calls = %d, want one", stub.calls)
		}
	})
}
