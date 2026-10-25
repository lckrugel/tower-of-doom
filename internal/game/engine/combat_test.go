package engine

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// Helper: puts the engine into StateEncounter with the given monster queue
// (spawn order) before choosing FIGHT/SNEAK.
func startEncounter(e *Engine, monsters ...entities.Monster) {
	e.CurrentEncounter = Encounter{Type: EncounterCombat, Monsters: monsters}
	e.State = StateEncounter
	e.CanAscend = false
}

// Helper: a hero that can actually deal damage.
func heroWithDamage(e *Engine, dmg int) {
	e.Player.BaseDMG = dmg
}

// fightFirstRound engages the encounter (FIGHT) and resolves the first
// round with the hero's first move — the Combat 2.0 flow: FIGHT only
// opens the state, rounds advance from move choices.
func fightFirstRound(e *Engine) []Event {
	e.Process(Command{Type: CmdFight})
	return e.Process(Command{Type: CmdMove, Index: 0})
}

func monsterFromFixture(id string) entities.Monster {
	return entities.NewMonsterFromAsset(testMonsters()[id])
}

// fastRat is a monster faster than the base hero SPD (10), for initiative tests.
func fastRat() entities.Monster {
	return entities.NewMonsterFromAsset(game.MonsterAsset{
		ID: "rat", Name: "Fast Rat", Difficulty: 2, HP: 30, Damage: 2, Speed: 12,
	})
}

func eventTypes(events []Event) []EventType {
	types := make([]EventType, 0, len(events))
	for _, ev := range events {
		types = append(types, ev.Type)
	}
	return types
}

func typesEqual(got []EventType, want ...EventType) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestExploreTransitionsToEncounter(t *testing.T) {
	e := newTestEngine(0)

	e.Process(Command{Type: CmdExplore})

	if e.State != StateEncounter {
		t.Errorf("state after explore = %v, want StateEncounter", e.State)
	}
}

func TestFightEntersCombatWithoutResolvingARound(t *testing.T) {
	// FIGHT only opens the combat state: rounds advance from the hero's
	// move choices (CmdMove), so engaging must consume no rng at all.
	e := newTestEngine(0)
	stub := &stubRng{values: []int{0}}
	e.rng = stub
	heroWithDamage(e, 5)
	startEncounter(e, monsterFromFixture("goblin"))

	events := e.Process(Command{Type: CmdFight})

	if len(events) != 0 {
		t.Errorf("events = %v, want none (FIGHT enters, it does not resolve)", events)
	}
	if e.State != StateCombat {
		t.Errorf("state after fight = %v, want StateCombat", e.State)
	}
	if stub.calls != 0 {
		t.Errorf("FIGHT consumed %d rng calls, want 0", stub.calls)
	}
	if hp := e.CurrentEncounter.Monsters[0].CurrentHP; hp != 30 {
		t.Errorf("goblin HP = %d, want 30 (no round resolved)", hp)
	}
}

func TestMoveResolvesOneRoundPerCommand(t *testing.T) {
	t.Run("tie on speed: hero acts first", func(t *testing.T) {
		// stub [1, 50, 0, 1, 50]: hero strike (spread 1, no crit), the
		// monster's pick, then the enemy strike (spread 1, no crit).
		e := newTestEngine(1, 50, 0, 1, 50)
		heroWithDamage(e, 5)
		startEncounter(e, monsterFromFixture("goblin")) // SPD 10 == hero SPD 10
		e.Process(Command{Type: CmdFight})

		events := e.Process(Command{Type: CmdMove, Index: 0})

		// The round resolves on the move choice: hero hits, then the
		// enemy hits back (each strike announces its move first).
		if got := eventTypes(events); !typesEqual(got,
			EventMoveUsed, EventCombatDamageDealt, EventMoveUsed, EventCombatDamageTaken) {
			t.Errorf("events = %v, want [move-used dealt move-used taken (tie = hero first)]", got)
		}
		if hp := e.CurrentEncounter.Monsters[0].CurrentHP; hp != 25 {
			t.Errorf("goblin HP = %d, want 25", hp)
		}
		if hp := e.Player.CurrentHP; hp != 98 {
			t.Errorf("hero HP = %d, want 98", hp)
		}
	})

	t.Run("enemy is faster: enemy acts first", func(t *testing.T) {
		e := newTestEngine()
		heroWithDamage(e, 5)
		startEncounter(e, fastRat()) // SPD 12 > hero SPD 10
		e.Process(Command{Type: CmdFight})

		events := e.Process(Command{Type: CmdMove, Index: 0})

		if got := eventTypes(events); !typesEqual(got,
			EventMoveUsed, EventCombatDamageTaken, EventMoveUsed, EventCombatDamageDealt) {
			t.Errorf("events = %v, want [move-used taken move-used dealt (enemy faster)]", got)
		}
	})

	t.Run("hero is faster: hero acts first", func(t *testing.T) {
		e := newTestEngine()
		heroWithDamage(e, 5)
		startEncounter(e, monsterFromFixture("orc")) // SPD 8 < hero SPD 10
		e.Process(Command{Type: CmdFight})

		events := e.Process(Command{Type: CmdMove, Index: 0})

		if got := eventTypes(events); !typesEqual(got,
			EventMoveUsed, EventCombatDamageDealt, EventMoveUsed, EventCombatDamageTaken) {
			t.Errorf("events = %v, want [move-used dealt move-used taken (hero faster)]", got)
		}
	})
}

// The move index points into the hero's combat move set; anything out of
// range is a generic invalid command with no state change and no rng.
func TestMoveIndexOutOfRangeIsInvalid(t *testing.T) {
	e := newTestEngine(0)
	heroWithDamage(e, 5)
	startEncounter(e, monsterFromFixture("goblin"))
	e.State = StateCombat
	stub := &stubRng{values: []int{0}}
	e.rng = stub

	for _, index := range []int{-1, 1, 99} { // the fallback set has one move
		events := e.Process(Command{Type: CmdMove, Index: index})

		if got := eventTypes(events); !typesEqual(got, EventMessage) {
			t.Errorf("index %d: events = %v, want [message (invalid command)]", index, got)
		}
		if e.State != StateCombat {
			t.Errorf("index %d: state = %v, want StateCombat (rejected command must not transition)", index, e.State)
		}
	}
	if stub.calls != 0 {
		t.Errorf("rejected moves consumed %d rng calls, want 0", stub.calls)
	}
	if hp := e.CurrentEncounter.Monsters[0].CurrentHP; hp != 30 {
		t.Errorf("goblin HP = %d, want 30 (no strike on a rejected move)", hp)
	}
}

func TestMoveKillsEnemyAndAdvancesQueue(t *testing.T) {
	e := newTestEngine()
	heroWithDamage(e, 40)
	startEncounter(e, monsterFromFixture("goblin"), monsterFromFixture("orc"))
	e.State = StateCombat

	events := e.Process(Command{Type: CmdMove, Index: 0})

	// Hero kills on its strike: the enemy never gets its turn this round
	// and the next enemy in the queue steps in with normal initiative.
	if got := eventTypes(events); !typesEqual(got, EventMoveUsed, EventCombatDamageDealt, EventEnemyDefeated, EventGoldGained) {
		t.Errorf("events = %v, want [move-used dealt defeated gold-gained]", got)
	}
	evt := events[2]
	if name := evt.Data["enemy"]; name != "Goblin" {
		t.Errorf("defeated enemy = %v, want Goblin", name)
	}
	if xp := evt.Data["xp_gained"]; xp != 21 {
		t.Errorf("xp_gained = %v, want 21", xp)
	}
	if e.State != StateCombat || len(e.CurrentEncounter.Monsters) != 1 || e.CurrentEncounter.Monsters[0].ID != "orc" {
		t.Errorf("queue did not advance: state = %v, encounter = %v (want combat with [orc])",
			e.State, monsterIDs(e.CurrentEncounter.Monsters))
	}
}

func TestVictoryOnLastEnemyKill(t *testing.T) {
	e := newTestEngine()
	heroWithDamage(e, 40)
	startEncounter(e, monsterFromFixture("goblin"))
	e.State = StateCombat

	events := e.Process(Command{Type: CmdMove, Index: 0})

	if got := eventTypes(events); !typesEqual(got, EventMoveUsed, EventCombatDamageDealt, EventEnemyDefeated, EventGoldGained, EventVictory) {
		t.Errorf("events = %v, want [move-used dealt defeated gold-gained victory]", got)
	}
	if e.State != StateExploring {
		t.Errorf("state after victory = %v, want StateExploring", e.State)
	}
	if !e.CanAscend {
		t.Error("victory did not unlock ascending")
	}
	if len(e.CurrentEncounter.Monsters) != 0 {
		t.Errorf("encounter = %v, want empty after victory", monsterIDs(e.CurrentEncounter.Monsters))
	}
}

func TestMidCombatLevelUp(t *testing.T) {
	e := newTestEngine()
	heroWithDamage(e, 40)
	e.Player.Xp = 95 // goblin kill (diff 3 → 21 XP) crosses the 100 threshold
	startEncounter(e, monsterFromFixture("goblin"), monsterFromFixture("orc"))
	e.State = StateCombat

	events := e.Process(Command{Type: CmdMove, Index: 0})

	if got := eventTypes(events); !typesEqual(got,
		EventMoveUsed, EventCombatDamageDealt, EventEnemyDefeated, EventGoldGained, EventLevelUp) {
		t.Errorf("events = %v, want [move-used dealt defeated gold-gained level-up]", got)
	}
	if e.Player.Lvl != 2 {
		t.Errorf("hero level = %d, want 2 (mid-combat level up)", e.Player.Lvl)
	}
	if lvl := events[4].Data["level"]; lvl != 2 {
		t.Errorf("level event data = %v, want 2", lvl)
	}
	// A full-health hero gains nothing from the level-up heal — the
	// additive "healed" key reports 0.
	if healed := events[4].Data["healed"]; healed != 0 {
		t.Errorf("healed event data = %v, want 0 (already at full health)", healed)
	}
	// Combat continues with the next enemy, still in StateCombat.
	if e.State != StateCombat || e.CurrentEncounter.Monsters[0].ID != "orc" {
		t.Errorf("combat broken after level up: state = %v, encounter = %v",
			e.State, monsterIDs(e.CurrentEncounter.Monsters))
	}
}

// A mid-combat level up also heals: the hero crossing the XP threshold
// during appendEnemyDefeat ends the round with the level-up heal applied
// (a quarter of the base max HP, clamped), reported on the event.
func TestMidCombatLevelUpHeals(t *testing.T) {
	e := newTestEngine()
	heroWithDamage(e, 40) // one-hit kill: the goblin never strikes back
	e.Player.Xp = 95      // goblin kill (diff 3 → 21 XP) crosses the 100 threshold
	e.Player.CurrentHP = 50
	startEncounter(e, monsterFromFixture("goblin"), monsterFromFixture("orc"))
	e.State = StateCombat

	events := e.Process(Command{Type: CmdMove, Index: 0})

	if got := eventTypes(events); !typesEqual(got,
		EventMoveUsed, EventCombatDamageDealt, EventEnemyDefeated, EventGoldGained, EventLevelUp) {
		t.Fatalf("events = %v, want [move-used dealt defeated gold-gained level-up]", got)
	}
	// 50/100 → rescaled to 55/110, then healed 25% of the new base max
	// HP (110 × 25% = 27.5, floored to 27).
	if healed := events[4].Data["healed"]; healed != 27 {
		t.Errorf("healed event data = %v, want 27 (25%% of base max HP 110)", healed)
	}
	if hp := e.Player.CurrentHP; hp != 82 {
		t.Errorf("hero HP = %d, want 82 (55 after rescale + 27 healed)", hp)
	}
	if e.Player.HP() > e.Player.MaxHP() {
		t.Errorf("hero HP %d exceeds max %d", e.Player.HP(), e.Player.MaxHP())
	}
}

func TestHeroDeathEndsCombatAndGame(t *testing.T) {
	e := newTestEngine()
	heroWithDamage(e, 5)
	e.Player.CurrentHP = 5
	startEncounter(e, monsterFromFixture("orc")) // dmg 8, slower than hero
	e.State = StateCombat

	events := e.Process(Command{Type: CmdMove, Index: 0})

	// Hero is faster, hits, fails to kill; the orc returns fire and wins.
	if got := eventTypes(events); !typesEqual(got,
		EventMoveUsed, EventCombatDamageDealt, EventMoveUsed, EventCombatDamageTaken, EventHeroDefeated) {
		t.Errorf("events = %v, want [move-used dealt move-used taken hero-defeated]", got)
	}
	if e.State != StateGameOver {
		t.Errorf("state after death = %v, want StateGameOver", e.State)
	}
}

func TestSneak(t *testing.T) {
	t.Run("success discards the encounter and unlocks nothing", func(t *testing.T) {
		e := newTestEngine(0) // Intn(2) = 0 → success
		startEncounter(e, monsterFromFixture("goblin"))

		events := e.Process(Command{Type: CmdSneak})

		if got := eventTypes(events); !typesEqual(got, EventSneakSuccess) {
			t.Errorf("events = %v, want [sneak-success]", got)
		}
		if e.State != StateExploring {
			t.Errorf("state after successful sneak = %v, want StateExploring", e.State)
		}
		if len(e.CurrentEncounter.Monsters) != 0 {
			t.Errorf("encounter = %v, want discarded", monsterIDs(e.CurrentEncounter.Monsters))
		}
		if e.CanAscend {
			t.Error("sneaking past must not unlock ascending (only victory does)")
		}
	})

	t.Run("failure starts combat with a free enemy turn", func(t *testing.T) {
		// Intn(2) = 1 → failure; then the free strike rolls average spread, no crit.
		e := newTestEngine(1, 0, 1, 50)
		heroWithDamage(e, 5)
		startEncounter(e, monsterFromFixture("goblin"))

		events := e.Process(Command{Type: CmdSneak})

		// No initiative: the Hero does not act, the enemy gets a single attack.
		if got := eventTypes(events); !typesEqual(got, EventSneakFailed, EventMoveUsed, EventCombatDamageTaken) {
			t.Errorf("events = %v, want [sneak-failed move-used taken (free turn)]", got)
		}
		if e.State != StateCombat {
			t.Errorf("state after failed sneak = %v, want StateCombat", e.State)
		}
		if hp := e.Player.CurrentHP; hp != 98 {
			t.Errorf("hero HP = %d, want 98 (one free goblin hit)", hp)
		}
		if len(e.CurrentEncounter.Monsters) != 1 {
			t.Errorf("encounter = %v, want intact after failed sneak", monsterIDs(e.CurrentEncounter.Monsters))
		}
	})
}

func TestFleeChance(t *testing.T) {
	tests := []struct {
		name       string
		heroSpeed  int
		enemySpeed int
		want       int
	}{
		{"10 points behind clamps to floor", 10, 20, 5},
		{"10 points ahead clamps to cap", 20, 10, 95},
		{"even speeds", 10, 10, 50},
		{"3 points ahead", 12, 9, 65},
		{"huge disadvantage still respects the floor", 0, 100, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fleeChance(tt.heroSpeed, tt.enemySpeed); got != tt.want {
				t.Errorf("fleeChance(%d, %d) = %d, want %d",
					tt.heroSpeed, tt.enemySpeed, got, tt.want)
			}
		})
	}
}

func TestFlee(t *testing.T) {
	t.Run("success exits combat with no reward", func(t *testing.T) {
		e := newTestEngine(0) // Intn(100) = 0 < chance 50 → success
		heroWithDamage(e, 5)
		startEncounter(e, monsterFromFixture("goblin"))
		e.State = StateCombat

		events := e.Process(Command{Type: CmdFlee})

		if got := eventTypes(events); !typesEqual(got, EventFled) {
			t.Errorf("events = %v, want [fled]", got)
		}
		if e.State != StateExploring {
			t.Errorf("state after fleeing = %v, want StateExploring", e.State)
		}
		if len(e.CurrentEncounter.Monsters) != 0 {
			t.Errorf("encounter = %v, want discarded", monsterIDs(e.CurrentEncounter.Monsters))
		}
		if e.Player.Xp != 0 {
			t.Errorf("xp = %d, want 0 (fleeing grants no reward)", e.Player.Xp)
		}
	})

	t.Run("failure gives the enemy a free turn", func(t *testing.T) {
		// Intn(100) = 99 → flee fails; then the free strike rolls average spread, no crit.
		e := newTestEngine(99, 0, 1, 50)
		heroWithDamage(e, 5)
		startEncounter(e, monsterFromFixture("goblin"))
		e.State = StateCombat

		events := e.Process(Command{Type: CmdFlee})

		if got := eventTypes(events); !typesEqual(got, EventMessage, EventMoveUsed, EventCombatDamageTaken) {
			t.Errorf("events = %v, want [message move-used taken (free turn)]", got)
		}
		if e.State != StateCombat {
			t.Errorf("state after failed flee = %v, want StateCombat", e.State)
		}
		if hp := e.Player.CurrentHP; hp != 98 {
			t.Errorf("hero HP = %d, want 98 (one free goblin hit)", hp)
		}
	})

	t.Run("a deadly free turn on failed flee ends the game", func(t *testing.T) {
		e := newTestEngine(99, 0, 1, 0) // failure
		e.Player.CurrentHP = 1
		startEncounter(e, monsterFromFixture("goblin"))
		e.State = StateCombat

		events := e.Process(Command{Type: CmdFlee})

		if got := eventTypes(events); !typesEqual(got, EventMessage, EventMoveUsed, EventCombatDamageTaken, EventHeroDefeated) {
			t.Errorf("events = %v, want [message move-used taken hero-defeated]", got)
		}
		if e.State != StateGameOver {
			t.Errorf("state = %v, want StateGameOver", e.State)
		}
	})
}

func TestAscendUnlocking(t *testing.T) {
	t.Run("blocked before a victory", func(t *testing.T) {
		e := newTestEngine(0)

		events := e.Process(Command{Type: CmdAscend})

		if got := eventTypes(events); !typesEqual(got, EventMessage) {
			t.Errorf("events = %v, want [message (cannot ascend)]", got)
		}
		if e.CurrentFloor != 1 {
			t.Errorf("floor = %d, want 1", e.CurrentFloor)
		}
	})

	t.Run("victory unlocks ascending to the next floor", func(t *testing.T) {
		e := newTestEngine()
		heroWithDamage(e, 40)
		startEncounter(e, monsterFromFixture("goblin"))
		fightFirstRound(e) // enters combat; the move kills → victory

		if !e.CanAscend {
			t.Fatal("victory did not unlock ascending")
		}
		events := e.Process(Command{Type: CmdAscend})

		// The test registry has no armor pieces: the ascend emits the
		// floor event plus the empty-pool message and stays exploring.
		if got := eventTypes(events); !typesEqual(got, EventFloorEntered, EventMessage) {
			t.Errorf("events = %v, want [floor-entered message]", got)
		}
		if e.CurrentFloor != 2 {
			t.Errorf("floor = %d, want 2", e.CurrentFloor)
		}
		if e.CanAscend {
			t.Error("ascending must re-lock until the next victory")
		}
		if e.State != StateExploring {
			t.Errorf("state after ascend = %v, want StateExploring", e.State)
		}
	})
}

// Mitigation per hit is DEF/(DEF+K) applied over the swung damage.
func TestEffectiveDamage(t *testing.T) {
	tests := []struct {
		name  string
		swing int
		def   int
		want  int
	}{
		{"no defense is a clean hit", 5, 0, 5},
		{"single leather armor mitigates 12.5%", 5, 5, 4}, // 5×35/40
		{"full leather set mitigates 30%", 5, 15, 3},      // 5×35/50
		{"DEF 21 mitigates 37.5%", 5, 21, 3},              // 5×35/56
		{"half mitigation exactly at DEF == K", 10, defCurveK, 5},
		{"armor bites harder at low K", 9, 5, 7}, // 9×35/40 = 7.875 (8 under K 45)
		{"armor cannot fully nullify weak hits", 1, 100, 1},
		{"heavy monsters take less from the same swing", 5, 3, 4}, // 5×35/38
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveDamage(tt.swing, tt.def); got != tt.want {
				t.Errorf("effectiveDamage(%d, %d) = %d, want %d", tt.swing, tt.def, got, tt.want)
			}
		})
	}
}

// The defense of each side is applied to incoming damage during real rounds.
func TestDefenseMitigationInCombat(t *testing.T) {
	t.Run("hero wearing armor takes less damage", func(t *testing.T) {
		// stub [1, 50, 1, 50]: each strike rolls nothing special.
		e := newTestEngine(1, 50, 0, 1, 50)
		heroWithDamage(e, 5)
		helm := entities.NewItemFromAsset(game.ItemAsset{
			ID: "helm", Name: "Helm", Slot: "helmet", BonusDEF: 5,
		})
		e.Player.EquipItem(&helm)
		startEncounter(e, monsterFromFixture("goblin"))

		fightFirstRound(e)

		// goblin swing 2 mitigated by DEF 5: 2×35/40 = 1.
		if hp := e.Player.CurrentHP; hp != 99 {
			t.Errorf("hero HP = %d, want 99 (2 dmg mitigated to 1)", hp)
		}
	})

	t.Run("armored monsters take less damage", func(t *testing.T) {
		e := newTestEngine(1, 50, 0, 1, 50)
		heroWithDamage(e, 5)
		armored := entities.NewMonsterFromAsset(game.MonsterAsset{
			ID: "armored", Name: "Armored Rat", Difficulty: 2, HP: 30, Damage: 2, Speed: 5, Defense: 5,
		})
		startEncounter(e, armored)

		fightFirstRound(e)

		// hero swing 5 mitigated by monster DEF 5: 5×35/40 = 4.
		if hp := e.CurrentEncounter.Monsters[0].CurrentHP; hp != 26 {
			t.Errorf("monster HP = %d, want 26 (5 dmg mitigated to 4)", hp)
		}
	})
}

// ascendItems is the armor-bearing registry shared by the ascend tests:
// the sorted armor pool is [chain_mail, helm_cap].
func ascendItems() map[string]game.ItemAsset {
	return map[string]game.ItemAsset{
		"wooden_sword": {ID: "wooden_sword", Name: "Sword", Slot: "weapon", BonusDMG: 5},
		"chain_mail":   {ID: "chain_mail", Name: "Mail", Slot: "body_armor", BonusDEF: 3},
		"helm_cap":     {ID: "helm_cap", Name: "Cap", Slot: "helmet", BonusDEF: 5},
	}
}

// Ascending to the next floor rolls a random armor piece from the NEW
// floor's section and opens the loot decision over it (no auto-equip).
func TestAscendGrantsRandomArmor(t *testing.T) {
	tests := []struct {
		name    string
		rngPick int
		wantID  string
	}{
		{"first armor in sorted order", 0, "chain_mail"},
		{"second armor in sorted order", 1, "helm_cap"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(tt.rngPick)
			e.reg.Items = ascendItems()
			weaponBefore := e.Player.Weapon
			e.CanAscend = true

			events := e.Process(Command{Type: CmdAscend})

			// Floor event first, loot decision second.
			if got := eventTypes(events); !typesEqual(got, EventFloorEntered, EventLootFound) {
				t.Fatalf("events = %v, want [floor-entered loot-found]", got)
			}
			item := events[1].Data["item"].(*entities.Item)
			if item.ID != tt.wantID {
				t.Errorf("looted item = %q, want %q (weapons must never roll)", item.ID, tt.wantID)
			}
			if e.State != StateLoot || e.PendingLoot != item {
				t.Errorf("state = %v, pending = %v, want StateLoot holding the reward", e.State, e.PendingLoot)
			}
			// Nothing is equipped until the player decides.
			if e.Player.Head != nil || e.Player.Body != nil || e.Player.Feet != nil {
				t.Error("armor reward auto-equipped — must wait for the player decision")
			}
			if e.Player.Weapon != weaponBefore {
				t.Error("the worn gear changed on ascend")
			}
		})
	}
}

// A registry without armor pieces makes ascending work with just the floor
// event and a message: the empty-pool guard behaves like an empty chest,
// staying in StateExploring with nothing pending.
func TestAscendWithoutArmorRegistryGrantsNothing(t *testing.T) {
	e := newTestEngine()
	e.CanAscend = true

	events := e.Process(Command{Type: CmdAscend})

	if got := eventTypes(events); !typesEqual(got, EventFloorEntered, EventMessage) {
		t.Fatalf("events = %v, want [floor-entered message]", got)
	}
	if e.State != StateExploring || e.PendingLoot != nil {
		t.Errorf("state = %v, pending = %v, want Exploring/nil", e.State, e.PendingLoot)
	}
}

func TestQuitAllowedInEveryState(t *testing.T) {
	for _, state := range []GameState{
		StateExploring, StateEncounter, StateCombat, StateGameOver,
		StateLoot, StateInventory, StateMerchant,
	} {
		e := newTestEngine()
		e.State = state

		events := e.Process(Command{Type: CmdQuit})

		if got := eventTypes(events); !typesEqual(got, EventQuit) {
			t.Errorf("state %v: events = %v, want [quit]", state, got)
		}
	}
}

func TestGameOverOnlyAcceptsQuit(t *testing.T) {
	e := newTestEngine()
	e.State = StateGameOver

	if got := eventTypes(e.Process(Command{Type: CmdExplore})); !typesEqual(got, EventMessage) {
		t.Errorf("explore in game over: events = %v, want [message]", got)
	}
	if got := eventTypes(e.Process(Command{Type: CmdQuit})); !typesEqual(got, EventQuit) {
		t.Errorf("quit in game over: events = %v, want [quit]", got)
	}
}

func TestInvalidCommandInWrongState(t *testing.T) {
	e := newTestEngine()
	e.Player.BaseDMG = 5
	startEncounter(e, monsterFromFixture("goblin"))
	e.State = StateCombat

	if got := eventTypes(e.Process(Command{Type: CmdExplore})); !typesEqual(got, EventMessage) {
		t.Errorf("events = %v, want [message (invalid command)]", got)
	}
	// State must be untouched by rejected commands.
	if e.State != StateCombat {
		t.Errorf("state = %v, want StateCombat (rejected command must not transition)", e.State)
	}
}

// rollMoveDamage consumes two rng values per strike: spread, then crit
// roll. With a neutral move (100% modifier, no crit bonus) the base is
// the attack stat itself, so the table pins the raw spread/crit math.
func TestRollDamageSpread(t *testing.T) {
	basic := entities.BasicAttack()
	tests := []struct {
		name     string
		stat     int
		spread   int // Intn(3) result
		critRoll int // Intn(100) result
		want     int
		wantCrit bool
	}{
		{"spread 0 rolls below the stat", 5, 0, 50, 4, false},
		{"spread 1 rolls the exact stat", 5, 1, 50, 5, false},
		{"spread 2 rolls above the stat", 5, 2, 50, 6, false},
		{"damage is floored at 1", 1, 0, 50, 1, false},
		{"a weaponless fighter still chips for 1", 0, 0, 50, 1, false},
		{"crit doubles the swung damage", 5, 1, 0, 10, true},
		{"roll equal to the threshold is not a crit", 5, 1, 10, 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(tt.spread, tt.critRoll)

			got, crit := e.rollMoveDamage(tt.stat, &basic, 10)

			if got != tt.want {
				t.Errorf("damage = %d, want %d", got, tt.want)
			}
			if crit != tt.wantCrit {
				t.Errorf("crit = %v, want %v", crit, tt.wantCrit)
			}
		})
	}
}

// The hero's fixed 10% threshold fires in an actual round.
func TestHeroCritInCombat(t *testing.T) {
	// hero strike: spread 1 → swing 5, crit roll 0 < 10 → 10 dealt;
	// enemy strike: spread 1 → swing 2, crit roll 50 → no crit.
	e := newTestEngine(1, 0, 0, 1, 50)
	heroWithDamage(e, 5)
	startEncounter(e, monsterFromFixture("goblin"))

	events := fightFirstRound(e)

	dealt, taken := events[1], events[3]
	if dmg := dealt.Data["damage_dealt"]; dmg != 10 {
		t.Errorf("damage_dealt = %v, want 10 (crit doubled)", dmg)
	}
	if crit, _ := dealt.Data["crit"].(bool); !crit {
		t.Error("damage event crit flag = false, want true")
	}
	if hp := e.CurrentEncounter.Monsters[0].CurrentHP; hp != 20 {
		t.Errorf("goblin HP = %d, want 20", hp)
	}
	if crit, _ := taken.Data["crit"].(bool); crit {
		t.Error("hero took a crit, want none")
	}
	if hp := e.Player.CurrentHP; hp != 98 {
		t.Errorf("hero HP = %d, want 98", hp)
	}
}

// Monsters use their own crit attribute, from the spawn asset.
func TestMonsterCritInCombat(t *testing.T) {
	// hero strike: spread 1, crit 50 → plain 5 dealt;
	// monster pick, then enemy strike: spread 1 → swing 2, crit roll 0 < 50 → 4 taken.
	e := newTestEngine()
	e.rng = &stubRng{values: []int{1, 50, 0, 1, 0}}
	heroWithDamage(e, 5)
	critter := entities.NewMonsterFromAsset(game.MonsterAsset{
		ID: "critter", Name: "Critter", Difficulty: 2, HP: 30, Damage: 2, Speed: 5, Crit: 50,
	})
	startEncounter(e, critter)

	events := fightFirstRound(e)

	taken := events[3]
	if dmg := taken.Data["damage_taken"]; dmg != 4 {
		t.Errorf("damage_taken = %v, want 4 (monster crit doubled)", dmg)
	}
	if crit, _ := taken.Data["crit"].(bool); !crit {
		t.Error("damage event crit flag = false, want true")
	}
	if hp := e.Player.CurrentHP; hp != 96 {
		t.Errorf("hero HP = %d, want 96", hp)
	}
}

// Damage events carry the defender's exact HP in addition to the legacy
// percentage keys, reflecting the defender's state AFTER the hit
// (mitigation, clamp and mid-round death included).
func TestDamageEventsCarryExactHP(t *testing.T) {
	e := newTestEngine(1, 50, 0, 1, 50) // average spread (no crit) per strike, pick between
	heroWithDamage(e, 5)
	startEncounter(e, monsterFromFixture("goblin")) // 30 HP, DEF 0

	events := fightFirstRound(e)

	dealt := events[1]
	if dmg := dealt.Data["damage_dealt"]; dmg != 5 {
		t.Errorf("damage_dealt = %v, want 5", dmg)
	}
	if hp := dealt.Data["enemy_hp_exact"]; hp != 25 {
		t.Errorf("enemy_hp_exact = %v, want 25 (defender state after the hit)", hp)
	}
	if mhp := dealt.Data["enemy_max_hp"]; mhp != 30 {
		t.Errorf("enemy_max_hp = %v, want 30", mhp)
	}
	// The legacy percentage key keeps its meaning: 25/30, floored.
	if pct := dealt.Data["enemy_hp"]; pct != 83 {
		t.Errorf("enemy_hp = %v, want 83 (legacy percentage key untouched)", pct)
	}

	taken := events[3]
	if hp := taken.Data["hero_hp_exact"]; hp != 98 {
		t.Errorf("hero_hp_exact = %v, want 98", hp)
	}
	if mhp := taken.Data["hero_max_hp"]; mhp != 100 {
		t.Errorf("hero_max_hp = %v, want 100", mhp)
	}
	if pct := taken.Data["hero_hp"]; pct != 98 {
		t.Errorf("hero_hp = %v, want 98 (legacy percentage key untouched)", pct)
	}
}

// A lethal blow clamps the exact HP at 0 before the defeat event rides:
// taken damage exceeding the current HP must not surface as a negative HP.
func TestDamageEventsExactHPClampsAtZeroOnDeath(t *testing.T) {
	// Hero strike: spread 1 → swing 5 (orc 50 HP survives);
	// orc strike: spread 1 → swing 8, which exceeds the hero's 5 HP.
	e := newTestEngine(1, 50, 0, 1, 50)
	heroWithDamage(e, 5)
	e.Player.CurrentHP = 5
	startEncounter(e, monsterFromFixture("orc")) // dmg 8, slower than hero
	e.State = StateCombat

	events := e.Process(Command{Type: CmdMove, Index: 0})

	dealt := events[1]
	if hp := dealt.Data["enemy_hp_exact"]; hp != 45 {
		t.Errorf("enemy_hp_exact = %v, want 45", hp)
	}
	if pct := dealt.Data["enemy_hp"]; pct != 90 {
		t.Errorf("enemy_hp = %v, want 90 (legacy percentage)", pct)
	}

	taken := events[3]
	if dmg := taken.Data["damage_taken"]; dmg != 5 {
		t.Errorf("damage_taken = %v, want 5 (the swing was 8, but only the hero's remaining 5 HP got subtracted)", dmg)
	}
	if hp := taken.Data["hero_hp_exact"]; hp != 0 {
		t.Errorf("hero_hp_exact = %v, want 0 (clamped by TakeDmg, never negative)", hp)
	}
	if mhp := taken.Data["hero_max_hp"]; mhp != 100 {
		t.Errorf("hero_max_hp = %v, want 100", mhp)
	}
	if pct := taken.Data["hero_hp"]; pct != 0 {
		t.Errorf("hero_hp = %v, want 0 (legacy percentage)", pct)
	}
	if alive := taken.Data["hero_alive"]; alive != false {
		t.Errorf("hero_alive = %v, want false (mid-round death)", alive)
	}
	if got := eventTypes(events); !typesEqual(got,
		EventMoveUsed, EventCombatDamageDealt, EventMoveUsed, EventCombatDamageTaken, EventHeroDefeated) {
		t.Errorf("events = %v, want [move-used dealt move-used taken hero-defeated]", got)
	}
}

// Run statistics: the kills counter increments on every EventEnemyDefeated
// path (mid-combat kills included) and never on a player defeat.
func TestKillsCounter(t *testing.T) {
	t.Run("fresh engine has zero kills", func(t *testing.T) {
		if got := newTestEngine().kills; got != 0 {
			t.Errorf("fresh engine kills = %d, want 0", got)
		}
	})

	t.Run("increments across a multi-monster encounter", func(t *testing.T) {
		e := newTestEngine(1, 50) // average spread (no crit) per strike, cycling
		heroWithDamage(e, 60)     // one-hit kills each foe
		startEncounter(e, monsterFromFixture("goblin"), monsterFromFixture("orc"))
		e.State = StateCombat

		e.Process(Command{Type: CmdMove, Index: 0}) // goblin falls
		if got := e.kills; got != 1 {
			t.Errorf("kills after the first kill = %d, want 1", got)
		}

		e.Process(Command{Type: CmdMove, Index: 0}) // orc falls → victory
		if got := e.kills; got != 2 {
			t.Errorf("kills after the second kill = %d, want 2", got)
		}
	})

	t.Run("does not increment when the player loses", func(t *testing.T) {
		e := newTestEngine(1, 50, 0, 1, 50)
		heroWithDamage(e, 5)
		e.Player.CurrentHP = 1
		startEncounter(e, monsterFromFixture("orc")) // dmg 8, slower than hero
		e.State = StateCombat

		e.Process(Command{Type: CmdMove, Index: 0}) // hero strikes, then the orc wins

		if got := e.kills; got != 0 {
			t.Errorf("kills after a defeat = %d, want 0", got)
		}
	})
}

// EventHeroDefeated reports the run state at the moment of death.
func TestHeroDefeatedEventCarriesRunStats(t *testing.T) {
	e := newTestEngine(1, 50) // average spread (no crit) per strike, cycling
	e.CurrentFloor = 3
	e.Player.Lvl = 7
	heroWithDamage(e, 40)

	// First kill of the run: kills → 1, then a scripted death.
	startEncounter(e, monsterFromFixture("goblin"))
	e.State = StateCombat
	e.Process(Command{Type: CmdMove, Index: 0})
	if got := e.kills; got != 1 {
		t.Fatalf("kills after the goblin = %d, want 1", got)
	}

	heroWithDamage(e, 5) // weak hits never kill the orc
	e.Player.CurrentHP = 1
	startEncounter(e, monsterFromFixture("orc")) // dmg 8, slower than hero
	e.State = StateCombat
	events := e.Process(Command{Type: CmdMove, Index: 0})

	if got := eventTypes(events); !typesEqual(got,
		EventMoveUsed, EventCombatDamageDealt, EventMoveUsed, EventCombatDamageTaken, EventHeroDefeated) {
		t.Fatalf("events = %v, want [move-used dealt move-used taken hero-defeated]", got)
	}
	death := events[4]
	if floor := death.Data["floor"]; floor != 3 {
		t.Errorf("floor = %v, want 3 (CurrentFloor at death)", floor)
	}
	if level := death.Data["level"]; level != 7 {
		t.Errorf("level = %v, want 7 (Lvl at death)", level)
	}
	if kills := death.Data["kills"]; kills != 1 {
		t.Errorf("kills = %v, want 1 (the earlier goblin kill)", kills)
	}
}

// A 0% threshold can never crit, even on a 0 roll.
func TestCritNeverFiresWithZeroThreshold(t *testing.T) {
	e := newTestEngine(1, 0)

	goblin := monsterFromFixture("goblin") // crit asset value absent → 0
	basic := entities.BasicAttack()
	got, crit := e.rollMoveDamage(2, &basic, goblin.CritThreshold())

	if crit {
		t.Error("crit fired with a 0 threshold")
	}
	if got != 2 {
		t.Errorf("damage = %d, want 2", got)
	}
}

// The ascend reward is a loot decision: wearing it from StateLoot sends
// the displaced gear to the bag when there is room.
func TestAscendLootWearDisplacesGearToBag(t *testing.T) {
	e := newTestEngine(1) // helm_cap from the sorted armor pool
	e.reg.Items = ascendItems()
	oldHelm := entities.NewItemFromAsset(game.ItemAsset{
		ID: "rusty_helm", Name: "Rusty Helm", Slot: "helmet", BonusDEF: 1,
	})
	e.Player.EquipItem(&oldHelm)
	e.CanAscend = true

	events := e.Process(Command{Type: CmdAscend})

	// Floor, then the reward held as pending loot.
	if got := eventTypes(events); !typesEqual(got, EventFloorEntered, EventLootFound) {
		t.Fatalf("events = %v, want [floor-entered loot-found]", got)
	}
	pending := e.PendingLoot

	// Wearing the reward displaces the rusty helm into the bag (reported
	// by swapped_from — the same emission contract as the chest flow).
	events = e.Process(Command{Type: CmdWear})

	if got := eventTypes(events); !typesEqual(got, EventItemEquipped) {
		t.Fatalf("wear events = %v, want [item-equipped]", got)
	}
	if equipped, _ := events[0].Data["item"].(*entities.Item); equipped != pending {
		t.Errorf("equipped = %v, want the ascend reward", equipped)
	}
	if swapped, _ := events[0].Data["swapped_from"].(*entities.Item); swapped == nil || swapped.ID != "rusty_helm" {
		t.Errorf("swapped_from = %v, want the displaced rusty helm", swapped)
	}
	if e.Player.Head == nil || e.Player.Head.ID != "helm_cap" {
		t.Error("reward not worn in the helmet slot")
	}
	bag := e.Player.Bag()
	if len(bag) != 1 || bag[0].ID != "rusty_helm" {
		t.Errorf("bag = %v, want the displaced rusty helm stashed", bag)
	}
	if e.State != StateExploring || e.PendingLoot != nil {
		t.Errorf("state = %v, pending = %v, want Exploring/nil", e.State, e.PendingLoot)
	}
}

// With the bag full, wearing the ascend reward from StateLoot destroys the
// displaced gear (EventItemLost) — same as the chest flow.
func TestAscendLootWearWithFullBagDestroysDisplaced(t *testing.T) {
	e := newTestEngine(1) // helm_cap
	e.reg.Items = ascendItems()
	oldHelm := entities.NewItemFromAsset(game.ItemAsset{
		ID: "rusty_helm", Name: "Rusty Helm", Slot: "helmet", BonusDEF: 1,
	})
	e.Player.EquipItem(&oldHelm)
	for e.Player.HasBagRoom() {
		e.Player.StashItem(testBagItem("filler", "amulet"))
	}
	e.CanAscend = true

	events := e.Process(Command{Type: CmdAscend})

	if got := eventTypes(events); !typesEqual(got, EventFloorEntered, EventLootFound) {
		t.Fatalf("events = %v, want [floor-entered loot-found]", got)
	}
	pending := e.PendingLoot

	events = e.Process(Command{Type: CmdWear})

	if got := eventTypes(events); !typesEqual(got, EventItemEquipped, EventItemLost) {
		t.Fatalf("wear events = %v, want [item-equipped item-lost]", got)
	}
	lost, _ := events[1].Data["item"].(*entities.Item)
	if lost == nil || lost.ID != "rusty_helm" {
		t.Errorf("lost = %v, want the displaced rusty helm", lost)
	}
	if e.Player.Head != pending {
		t.Error("reward not worn in the helmet slot")
	}
	if got := len(e.Player.Bag()); got != entities.InventoryCapacity {
		t.Errorf("bag length = %d, want %d (reward worn, bag still full)",
			got, entities.InventoryCapacity)
	}
}

// From the ascend loot decision the reward can also be stashed or
// discarded — exactly the chest options.
func TestAscendLootStashAndDiscard(t *testing.T) {
	setup := func(rngPick int) (*Engine, *entities.Item) {
		e := newTestEngine(rngPick)
		e.reg.Items = ascendItems()
		e.CanAscend = true
		e.Process(Command{Type: CmdAscend})
		return e, e.PendingLoot
	}

	t.Run("stash puts the reward in the bag", func(t *testing.T) {
		e, pending := setup(0) // chain_mail

		events := e.Process(Command{Type: CmdStashLoot})

		if got := eventTypes(events); !typesEqual(got, EventItemStashed) {
			t.Fatalf("events = %v, want [item-stashed]", got)
		}
		if item, _ := events[0].Data["item"].(*entities.Item); item != pending {
			t.Errorf("stashed = %v, want the ascend reward", item)
		}
		bag := e.Player.Bag()
		if len(bag) != 1 || bag[0] != pending {
			t.Errorf("bag = %v, want the ascend reward stashed", bag)
		}
		if e.State != StateExploring || e.PendingLoot != nil {
			t.Errorf("state = %v, pending = %v, want Exploring/nil", e.State, e.PendingLoot)
		}
	})

	t.Run("discard destroys the reward", func(t *testing.T) {
		e, pending := setup(0)

		events := e.Process(Command{Type: CmdDiscardLoot})

		if got := eventTypes(events); !typesEqual(got, EventItemDiscarded) {
			t.Fatalf("events = %v, want [item-discarded]", got)
		}
		if item, _ := events[0].Data["item"].(*entities.Item); item != pending {
			t.Errorf("discarded = %v, want the ascend reward", item)
		}
		if e.Player.Head != nil || e.Player.Body != nil || e.Player.Feet != nil || len(e.Player.Bag()) != 0 {
			t.Error("discarded reward survived somewhere")
		}
		if e.State != StateExploring || e.PendingLoot != nil {
			t.Errorf("state = %v, pending = %v, want Exploring/nil", e.State, e.PendingLoot)
		}
	})
}
