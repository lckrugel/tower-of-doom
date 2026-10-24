package engine

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// Helper: puts the engine into StateEncounter with the given monster queue
// (spawn order) before choosing FIGHT/SNEAK.
func startEncounter(e *Engine, monsters ...entities.Monster) {
	e.CurrentEncounter = monsters
	e.State = StateEncounter
	e.CanAscend = false
}

// Helper: a hero that can actually deal damage.
func heroWithDamage(e *Engine, dmg int) {
	e.Player.BaseDMG = dmg
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

func TestFightResolvesFirstRound(t *testing.T) {
	t.Run("tie on speed: hero acts first", func(t *testing.T) {
		// stub [1, 50] = average spread (no crit) per strike, cycling.
		e := newTestEngine(1, 50)
		heroWithDamage(e, 5)
		startEncounter(e, monsterFromFixture("goblin")) // SPD 10 == hero SPD 10

		events := e.Process(Command{Type: CmdFight})

		if e.State != StateCombat {
			t.Errorf("state after fight = %v, want StateCombat", e.State)
		}
		// The round resolves immediately: hero hits, then the enemy hits back.
		if got := eventTypes(events); !typesEqual(got,
			EventCombatDamageDealt, EventCombatDamageTaken) {
			t.Errorf("events = %v, want [dealt taken (tie = hero first)]", got)
		}
		if hp := e.CurrentEncounter[0].CurrentHP; hp != 25 {
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

		events := e.Process(Command{Type: CmdFight})

		if got := eventTypes(events); !typesEqual(got,
			EventCombatDamageTaken, EventCombatDamageDealt) {
			t.Errorf("events = %v, want [taken dealt (enemy faster)]", got)
		}
	})

	t.Run("hero is faster: hero acts first", func(t *testing.T) {
		e := newTestEngine()
		heroWithDamage(e, 5)
		startEncounter(e, monsterFromFixture("orc")) // SPD 8 < hero SPD 10

		events := e.Process(Command{Type: CmdFight})

		if got := eventTypes(events); !typesEqual(got,
			EventCombatDamageDealt, EventCombatDamageTaken) {
			t.Errorf("events = %v, want [dealt taken (hero faster)]", got)
		}
	})
}

func TestAttackResolvesOneRoundPerCommand(t *testing.T) {
	e := newTestEngine(1, 50) // average spread (no crit) per strike
	heroWithDamage(e, 5)
	startEncounter(e, monsterFromFixture("goblin"))
	e.State = StateCombat

	events := e.Process(Command{Type: CmdAttack})

	if got := eventTypes(events); !typesEqual(got,
		EventCombatDamageDealt, EventCombatDamageTaken) {
		t.Errorf("events = %v, want [dealt taken]", got)
	}
	if hp := e.CurrentEncounter[0].CurrentHP; hp != 25 {
		t.Errorf("goblin HP = %d, want 25 (one hit per command)", hp)
	}
}

func TestAttackKillsEnemyAndAdvancesQueue(t *testing.T) {
	e := newTestEngine()
	heroWithDamage(e, 40)
	startEncounter(e, monsterFromFixture("goblin"), monsterFromFixture("orc"))
	e.State = StateCombat

	events := e.Process(Command{Type: CmdAttack})

	// Hero kills on its strike: the enemy never gets its turn this round
	// and the next enemy in the queue steps in with normal initiative.
	if got := eventTypes(events); !typesEqual(got, EventCombatDamageDealt, EventEnemyDefeated) {
		t.Errorf("events = %v, want [dealt defeated]", got)
	}
	evt := events[1]
	if name := evt.Data["enemy"]; name != "Goblin" {
		t.Errorf("defeated enemy = %v, want Goblin", name)
	}
	if xp := evt.Data["xp_gained"]; xp != 12 {
		t.Errorf("xp_gained = %v, want 12", xp)
	}
	if e.State != StateCombat || len(e.CurrentEncounter) != 1 || e.CurrentEncounter[0].ID != "orc" {
		t.Errorf("queue did not advance: state = %v, encounter = %v (want combat with [orc])",
			e.State, monsterIDs(e.CurrentEncounter))
	}
}

func TestVictoryOnLastEnemyKill(t *testing.T) {
	e := newTestEngine()
	heroWithDamage(e, 40)
	startEncounter(e, monsterFromFixture("goblin"))
	e.State = StateCombat

	events := e.Process(Command{Type: CmdAttack})

	if got := eventTypes(events); !typesEqual(got, EventCombatDamageDealt, EventEnemyDefeated, EventVictory) {
		t.Errorf("events = %v, want [dealt defeated victory]", got)
	}
	if e.State != StateExploring {
		t.Errorf("state after victory = %v, want StateExploring", e.State)
	}
	if !e.CanAscend {
		t.Error("victory did not unlock ascending")
	}
	if len(e.CurrentEncounter) != 0 {
		t.Errorf("encounter = %v, want empty after victory", monsterIDs(e.CurrentEncounter))
	}
}

func TestMidCombatLevelUp(t *testing.T) {
	e := newTestEngine()
	heroWithDamage(e, 40)
	e.Player.Xp = 95 // goblin kill (diff 3 → 12 XP) crosses the 100 threshold
	startEncounter(e, monsterFromFixture("goblin"), monsterFromFixture("orc"))
	e.State = StateCombat

	events := e.Process(Command{Type: CmdAttack})

	if got := eventTypes(events); !typesEqual(got,
		EventCombatDamageDealt, EventEnemyDefeated, EventLevelUp) {
		t.Errorf("events = %v, want [dealt defeated level-up]", got)
	}
	if e.Player.Lvl != 2 {
		t.Errorf("hero level = %d, want 2 (mid-combat level up)", e.Player.Lvl)
	}
	if lvl := events[2].Data["level"]; lvl != 2 {
		t.Errorf("level event data = %v, want 2", lvl)
	}
	// Combat continues with the next enemy, still in StateCombat.
	if e.State != StateCombat || e.CurrentEncounter[0].ID != "orc" {
		t.Errorf("combat broken after level up: state = %v, encounter = %v",
			e.State, monsterIDs(e.CurrentEncounter))
	}
}

func TestHeroDeathEndsCombatAndGame(t *testing.T) {
	e := newTestEngine()
	heroWithDamage(e, 5)
	e.Player.CurrentHP = 5
	startEncounter(e, monsterFromFixture("orc")) // dmg 8, slower than hero
	e.State = StateCombat

	events := e.Process(Command{Type: CmdAttack})

	// Hero is faster, hits, fails to kill; the orc returns fire and wins.
	if got := eventTypes(events); !typesEqual(got,
		EventCombatDamageDealt, EventCombatDamageTaken, EventHeroDefeated) {
		t.Errorf("events = %v, want [dealt taken hero-defeated]", got)
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
		if len(e.CurrentEncounter) != 0 {
			t.Errorf("encounter = %v, want discarded", monsterIDs(e.CurrentEncounter))
		}
		if e.CanAscend {
			t.Error("sneaking past must not unlock ascending (only victory does)")
		}
	})

	t.Run("failure starts combat with a free enemy turn", func(t *testing.T) {
		// Intn(2) = 1 → failure; then the free strike rolls average spread, no crit.
		e := newTestEngine(1, 1, 50)
		heroWithDamage(e, 5)
		startEncounter(e, monsterFromFixture("goblin"))

		events := e.Process(Command{Type: CmdSneak})

		// No initiative: the Hero does not act, the enemy gets a single attack.
		if got := eventTypes(events); !typesEqual(got, EventSneakFailed, EventCombatDamageTaken) {
			t.Errorf("events = %v, want [sneak-failed taken (free turn)]", got)
		}
		if e.State != StateCombat {
			t.Errorf("state after failed sneak = %v, want StateCombat", e.State)
		}
		if hp := e.Player.CurrentHP; hp != 98 {
			t.Errorf("hero HP = %d, want 98 (one free goblin hit)", hp)
		}
		if len(e.CurrentEncounter) != 1 {
			t.Errorf("encounter = %v, want intact after failed sneak", monsterIDs(e.CurrentEncounter))
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
		if len(e.CurrentEncounter) != 0 {
			t.Errorf("encounter = %v, want discarded", monsterIDs(e.CurrentEncounter))
		}
		if e.Player.Xp != 0 {
			t.Errorf("xp = %d, want 0 (fleeing grants no reward)", e.Player.Xp)
		}
	})

	t.Run("failure gives the enemy a free turn", func(t *testing.T) {
		// Intn(100) = 99 → flee fails; then the free strike rolls average spread, no crit.
		e := newTestEngine(99, 1, 50)
		heroWithDamage(e, 5)
		startEncounter(e, monsterFromFixture("goblin"))
		e.State = StateCombat

		events := e.Process(Command{Type: CmdFlee})

		if got := eventTypes(events); !typesEqual(got, EventMessage, EventCombatDamageTaken) {
			t.Errorf("events = %v, want [message taken (free turn)]", got)
		}
		if e.State != StateCombat {
			t.Errorf("state after failed flee = %v, want StateCombat", e.State)
		}
		if hp := e.Player.CurrentHP; hp != 98 {
			t.Errorf("hero HP = %d, want 98 (one free goblin hit)", hp)
		}
	})

	t.Run("a deadly free turn on failed flee ends the game", func(t *testing.T) {
		e := newTestEngine(99) // failure
		e.Player.CurrentHP = 1
		startEncounter(e, monsterFromFixture("goblin"))
		e.State = StateCombat

		events := e.Process(Command{Type: CmdFlee})

		if got := eventTypes(events); !typesEqual(got, EventMessage, EventCombatDamageTaken, EventHeroDefeated) {
			t.Errorf("events = %v, want [message taken hero-defeated]", got)
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
		e.Process(Command{Type: CmdFight}) // kill goblin → victory

		if !e.CanAscend {
			t.Fatal("victory did not unlock ascending")
		}
		events := e.Process(Command{Type: CmdAscend})

		if got := eventTypes(events); !typesEqual(got, EventFloorEntered) {
			t.Errorf("events = %v, want [floor-entered]", got)
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

func TestQuitAllowedInEveryState(t *testing.T) {
	for _, state := range []GameState{StateExploring, StateEncounter, StateCombat, StateGameOver} {
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

// rollDamage consumes two rng values per strike: spread, then crit roll.
func TestRollDamageSpread(t *testing.T) {
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
			heroWithDamage(e, tt.stat)

			got, crit := e.rollDamage(e.Player)

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
	e := newTestEngine(1, 0, 1, 50)
	heroWithDamage(e, 5)
	startEncounter(e, monsterFromFixture("goblin"))

	events := e.Process(Command{Type: CmdFight})

	dealt, taken := events[0], events[1]
	if dmg := dealt.Data["damage_dealt"]; dmg != 10 {
		t.Errorf("damage_dealt = %v, want 10 (crit doubled)", dmg)
	}
	if crit, _ := dealt.Data["crit"].(bool); !crit {
		t.Error("damage event crit flag = false, want true")
	}
	if hp := e.CurrentEncounter[0].CurrentHP; hp != 20 {
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
	// enemy strike: spread 1 → swing 2, crit roll 0 < 50 → 4 taken.
	e := newTestEngine()
	e.rng = &stubRng{values: []int{1, 50, 1, 0}}
	heroWithDamage(e, 5)
	critter := entities.NewMonsterFromAsset(game.MonsterAsset{
		ID: "critter", Name: "Critter", Difficulty: 2, HP: 30, Damage: 2, Speed: 5, Crit: 50,
	})
	startEncounter(e, critter)

	events := e.Process(Command{Type: CmdFight})

	taken := events[1]
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

// A 0% threshold can never crit, even on a 0 roll.
func TestCritNeverFiresWithZeroThreshold(t *testing.T) {
	e := newTestEngine(1, 0)

	goblin := monsterFromFixture("goblin") // crit asset value absent → 0
	got, crit := e.rollDamage(&goblin)

	if crit {
		t.Error("crit fired with a 0 threshold")
	}
	if got != 2 {
		t.Errorf("damage = %d, want 2", got)
	}
}
