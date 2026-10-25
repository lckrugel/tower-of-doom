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
	return map[string]game.MoveAsset{
		"basic_attack":     {ID: "basic_attack", Name: "Basic Attack", Tier: 1, DmgModifier: 100, Weight: 100, Type: "both"},
		"heavy_slash":      {ID: "heavy_slash", Name: "Heavy Slash", Tier: 2, DmgModifier: 175, Cooldown: 2, Weight: 60, Type: "both"},
		"quick_stab":       {ID: "quick_stab", Name: "Quick Stab", Tier: 1, DmgModifier: 60, CritBonus: 20, Weight: 100, Type: "both"},
		"assassins_strike": {ID: "assassins_strike", Name: "Assassin's Strike", Tier: 3, DmgModifier: 110, CritBonus: 35, Cooldown: 3, Weight: 30, Type: "both"},
	}
}

// movesEngine is a test engine whose registry carries the move fixtures.
func movesEngine(rngValues ...int) *Engine {
	e := newTestEngine(rngValues...)
	e.reg.Moves = testMoves()
	return e
}

// learn teaches the hero the named fixture moves in order (learning
// order = the effective-set order the CmdMove index points into).
func learn(e *Engine, ids ...string) {
	for _, id := range ids {
		asset, ok := e.reg.MoveByID(id)
		if !ok {
			panic("test fixture: unknown move " + id)
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

	events := e.Process(Command{Type: CmdMove, Index: 0})

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

		events := e.Process(Command{Type: CmdMove, Index: 0})

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

		events := e.Process(Command{Type: CmdMove, Index: 0})

		if crit, _ := events[1].Data["crit"].(bool); crit {
			t.Error("roll 30 crit with threshold 30 — must be exclusive")
		}
	})
}

// The effective crit threshold clamps to [0, 100] at use time.
func TestMoveCritThresholdClamps(t *testing.T) {
	t.Run("a huge bonus clamps at 100", func(t *testing.T) {
		e := movesEngine(1, 99, 0, 1, 50) // 99 < 100 → crit on every roll but 100
		e.reg.Moves["overkill"] = game.MoveAsset{ID: "overkill", Name: "Overkill", DmgModifier: 100, CritBonus: 200, Weight: 1}
		heroWithDamage(e, 5)
		learn(e, "overkill")
		startEncounter(e, monsterFromFixture("goblin"))
		e.Process(Command{Type: CmdFight})

		events := e.Process(Command{Type: CmdMove, Index: 0})

		if crit, _ := events[1].Data["crit"].(bool); !crit {
			t.Error("crit roll 99 with a clamped 100 threshold must crit")
		}
	})

	t.Run("a negative bonus clamps at 0", func(t *testing.T) {
		e := movesEngine(1, 0, 0, 1, 50) // roll 0 with threshold 0 must not crit
		e.reg.Moves["dull"] = game.MoveAsset{ID: "dull", Name: "Dull Blow", DmgModifier: 100, CritBonus: -50, Weight: 1}
		heroWithDamage(e, 5)
		learn(e, "dull")
		startEncounter(e, monsterFromFixture("goblin"))
		e.Process(Command{Type: CmdFight})

		events := e.Process(Command{Type: CmdMove, Index: 0})

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
		e.Process(Command{Type: CmdMove, Index: 0}) // heavy_slash locks for 2

		stub := &stubRng{values: []int{1, 50}}
		e.rng = stub
		events := e.Process(Command{Type: CmdMove, Index: 0}) // locked

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

		e.Process(Command{Type: CmdMove, Index: 0})            // round 1: heavy_slash → locked 2
		e.Process(Command{Type: CmdMove, Index: 1})            // round 2: quick_stab (tick → 1)
		blocked := e.Process(Command{Type: CmdMove, Index: 0}) // round 3: still locked
		if got := eventTypes(blocked); !typesEqual(got, EventMessage) {
			t.Fatalf("round 3: heavy_slash unlocked one round early: %v", got)
		}
		e.Process(Command{Type: CmdMove, Index: 1}) // round 3 resolves (tick → 0)

		events := e.Process(Command{Type: CmdMove, Index: 0}) // round 4: ready again
		if got := eventTypes(events); !typesEqual(got,
			EventMoveUsed, EventCombatDamageDealt, EventMoveUsed, EventCombatDamageTaken) {
			t.Fatalf("round 4 events = %v, want [move-used dealt move-used taken]", got)
		}
		if move := events[0].Data["move"]; move != "Heavy Slash" {
			t.Errorf("round 4 move = %v, want Heavy Slash (cooldown expired)", move)
		}
	})
}

// A fully-cooled hero turn resolves as an automatic RECOVER round —
// never a soft-lock: the hero does not act, the enemy does, and the
// round-end ticks run as usual.
func TestFullyCooledHeroRecovers(t *testing.T) {
	// Crafted per round — round 1 (hero strike + monster pick + strike):
	// [1,50, 0, 1,50]; recover rounds (pick + monster strike):
	// [0, 1,50]; round 4 (hero strike again): [1,50, 0, 1,50].
	e := movesEngine(1, 50, 0, 1, 50, 0, 1, 50, 0, 1, 50, 1, 50, 0, 1, 50)
	heroWithDamage(e, 5)
	learn(e, "heavy_slash") // the ONLY move, cooldown 2
	startEncounter(e, monsterFromFixture("goblin"))
	e.Process(Command{Type: CmdFight})
	e.Process(Command{Type: CmdMove, Index: 0}) // round 1: locked for 2

	events := e.Process(Command{Type: CmdMove, Index: 0}) // round 2: recover

	if got := eventTypes(events); !typesEqual(got, EventMessage, EventMoveUsed, EventCombatDamageTaken) {
		t.Fatalf("round 2 events = %v, want [message move-used taken (recover)]", got)
	}
	if hp := e.CurrentEncounter.Monsters[0].CurrentHP; hp != 21 {
		t.Errorf("goblin HP = %d, want 21 (the hero did not strike)", hp)
	}

	// Round 3: still locked (2 locked rounds: 2 and 3) — recover again.
	if got := eventTypes(e.Process(Command{Type: CmdMove, Index: 0})); !typesEqual(got, EventMessage, EventMoveUsed, EventCombatDamageTaken) {
		t.Errorf("round 3 events = %v, want another recover round", got)
	}

	// Round 4: the cooldown has expired — the move strikes again.
	events = e.Process(Command{Type: CmdMove, Index: 0})
	if got := eventTypes(events); !typesEqual(got,
		EventMoveUsed, EventCombatDamageDealt, EventMoveUsed, EventCombatDamageTaken) {
		t.Fatalf("round 4 events = %v, want [move-used dealt move-used taken]", got)
	}
	if dmg := events[1].Data["damage_dealt"]; dmg != 9 {
		t.Errorf("round 4 damage_dealt = %v, want 9", dmg)
	}
}

// A killing blow ends the round early (the dead enemy never acts): the
// used move still locks, but the round-end tick does not run — the lock
// carries its full value into the next round.
func TestKillRoundLocksWithoutTicking(t *testing.T) {
	e := movesEngine()
	heroWithDamage(e, 40) // one-shots the goblin with any move
	learn(e, "heavy_slash", "quick_stab")
	startEncounter(e, monsterFromFixture("goblin"), monsterFromFixture("orc"))
	e.Process(Command{Type: CmdFight})

	e.Process(Command{Type: CmdMove, Index: 0}) // kill → the round ends early

	events := e.Process(Command{Type: CmdMove, Index: 0}) // next enemy
	if got := eventTypes(events); !typesEqual(got, EventMessage) {
		t.Errorf("events = %v, want [message] — heavy_slash must still be locked", got)
	}
}

// The hero's opening move set: Punch PRE-LEARNED (learned slot 1 of 2)
// plus the starter sword's explicit Basic Strike grant. With the punch
// in the learned set the effective set is never empty for a fresh hero —
// the basic_attack fallback below stays depth-only.
func TestFreshHeroStartsWithThePunch(t *testing.T) {
	// starterEngine builds an engine whose registry carries the starter
	// sword (equipped by id) and the move its grant references.
	starterEngine := func() *Engine {
		return NewEngine(&game.Registry{
			Monsters: testMonsters(),
			Items: map[string]game.ItemAsset{
				starterItemID: {
					ID: "wooden_sword", Name: "Espada de Madeira", Slot: "weapon",
					BonusDMG: 5, Durability: 50, Moves: []string{"basic_attack"},
				},
			},
			Moves: map[string]game.MoveAsset{
				"basic_attack": {ID: "basic_attack", Name: "Basic Strike", Tier: 1,
					DmgModifier: 100, Weight: 100, Type: "both"},
			},
			Sections: testSections(),
		})
	}

	t.Run("the engine hero's learned set is exactly [Punch]", func(t *testing.T) {
		got := starterEngine().Player.LearnedMoves()

		if len(got) != 1 || got[0].ID != entities.Punch().ID {
			t.Errorf("learned set = %+v, want [Punch] (pre-learned)", got)
		}
	})

	t.Run("effective set with the starter = [Punch, Basic Strike]", func(t *testing.T) {
		moves := starterEngine().CombatMoves()

		if len(moves) != 2 || moves[0].ID != entities.Punch().ID || moves[1].ID != "basic_attack" {
			t.Errorf("combat moves = %+v, want [Punch Basic Strike] (learned first, weapon grant after)", moves)
		}
	})

	t.Run("a weaponless fresh hero still has the punch — the fallback never fires", func(t *testing.T) {
		// No starter item in the registry: the hero fights bare-fisted.
		e := NewEngine(&game.Registry{
			Monsters: testMonsters(),
			Items:    map[string]game.ItemAsset{},
			Sections: testSections(),
		})

		moves := e.CombatMoves()
		if len(moves) != 1 || moves[0].ID != entities.Punch().ID {
			t.Errorf("combat moves = %+v, want [Punch] (the set is never empty)", moves)
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

	events := e.Process(Command{Type: CmdMove, Index: 0})

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

	events := e.Process(Command{Type: CmdMove, Index: 0})

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
	if move := events[2].Data["move"]; move != "Basic Strike" {
		t.Errorf("monster move = %v, want the basic_attack fallback", move)
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
	e := movesEngine(1, 50, 0, 1, 50)
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
