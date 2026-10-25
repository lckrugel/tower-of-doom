package engine

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// testMoves mirrors the shipped moves.json entries the combat tests
// exercise (hand-built registry — content validation is not in play).
func testMoves() map[string]game.MoveAsset {
	return map[string]game.MoveAsset{
		"basic_attack": {ID: "basic_attack", Name: "Basic Attack", Tier: 1, DmgModifier: 100, Weight: 100},
		"heavy_slash":  {ID: "heavy_slash", Name: "Heavy Slash", Tier: 2, DmgModifier: 175, Cooldown: 2, Weight: 60},
		"quick_stab":   {ID: "quick_stab", Name: "Quick Stab", Tier: 1, DmgModifier: 60, CritBonus: 20, Weight: 100},
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

// The move's dmg_modifier scales the striker's DMG before the spread:
// base = round(DMG × modifier / 100).
func TestMoveDamageModifier(t *testing.T) {
	// stub [1, 50] cycling: spread 1 → the base itself, crit roll 50 → no crit.
	e := movesEngine(1, 50)
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
		e := movesEngine(1, 29)
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
		e := movesEngine(1, 30)
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
		e := movesEngine(1, 99) // 99 < 100 → crit on every roll but 100
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
		e := movesEngine(1, 0) // roll 0 with threshold 0 must not crit
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
		e := movesEngine(1, 50)
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
		e := movesEngine(1, 50) // every strike: spread 1, no crit
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
	e := movesEngine(1, 50)
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

// Weapon-granted moves ride the equipped weapon: the engine resolves the
// asset's move ids at item build time and the set reaches CombatMoves.
func TestWeaponGrantedMovesAreUsable(t *testing.T) {
	e := movesEngine(1, 50)
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
	e := movesEngine(1, 50)
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
	if move := events[2].Data["move"]; move != "Basic Attack" {
		t.Errorf("enemy move = %v, want Basic Attack", move)
	}
	if user := events[2].Data["user"]; user != "Goblin" {
		t.Errorf("enemy user = %v, want Goblin", user)
	}
}
