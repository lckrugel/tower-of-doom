package engine

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
)

func TestAscendGoldChestUsesDestinationBudget(t *testing.T) {
	e := newKindEngine(39) // Even before the first EXPLORE, ascent is not exploration.
	e.reg.Sections = []game.SectionAsset{
		{ID: "old", Name: "Old", MinFloor: 1, MaxFloor: 5, BaseDifficulty: 1},
		{ID: "next", Name: "Next", MinFloor: 6, MaxFloor: -1, BaseDifficulty: 17, DifficultyPerFloor: 3},
	}
	e.CurrentFloor = 5
	e.CanAscend = true
	e.BossUnlocked = true
	e.Player.Gold = 7
	firstExplore := e.firstExploreDone
	history, misses := e.recentKinds, e.encounterMisses

	events := e.Process(Command{Type: CmdAscend})

	if got := eventTypes(events); !typesEqual(got, EventFloorEntered, EventChestFound) {
		t.Fatalf("events = %v, want [floor-entered chest-found]", got)
	}
	if events[0].Data["floor"] != 6 || events[0].Data["section"] != "Next" {
		t.Errorf("floor event = %v, want destination floor 6 / Next", events[0].Data)
	}
	if e.rng.(*stubRng).calls != 0 || e.Player.Gold != 7 || e.PendingChest == nil || e.PendingChest.Opened {
		t.Fatal("ascent discovery rolled or awarded a reward")
	}
	if e.CurrentFloor != 6 || e.CanAscend || e.BossUnlocked || e.State != StateChest || e.PendingLoot != nil || len(e.CurrentEncounter.Monsters) != 0 {
		t.Errorf("invalid post-ascend state: floor=%d ascend=%v boss=%v state=%v loot=%v encounter=%v", e.CurrentFloor, e.CanAscend, e.BossUnlocked, e.State, e.PendingLoot, e.CurrentEncounter)
	}
	opened := e.Process(Command{Type: CmdOpenChest})
	if len(opened) != 1 || opened[0].Data["amount"] != 85 || opened[0].Data["source"] != GoldSourceChest || e.Player.Gold != 92 {
		t.Fatalf("open = %v, gold = %d", opened, e.Player.Gold)
	}
	e.Process(Command{Type: CmdContinue})
	if e.firstExploreDone != firstExplore || e.recentKinds != history || e.encounterMisses != misses {
		t.Error("ascension chest changed exploration pacing/history")
	}
	stub := e.rng.(*stubRng)
	if stub.calls != 1 || stub.bounds[0] != 100 {
		t.Errorf("rng bounds = %v, want [100] (no appearance or kind roll)", stub.bounds)
	}
}

// Kills pay a flat 5 × enemy-difficulty gold on every defeat (mid-combat
// kills included), alongside the instant XP. The grant consumes no rng.
func TestKillGrantsGold(t *testing.T) {
	t.Run("gold scales with the enemy difficulty", func(t *testing.T) {
		e := newTestEngine(1, 50) // swing 60 — one-shots the orc
		heroWithDamage(e, 60)
		startEncounter(e, monsterFromFixture("orc")) // difficulty 5 → 25 gold
		e.State = StateCombat

		events := e.Process(Command{Type: CmdMove, Index: 0})

		if got := eventTypes(events); !typesEqual(got,
			EventMoveUsed, EventCombatDamageDealt, EventEnemyDefeated, EventGoldGained, EventVictory) {
			t.Fatalf("events = %v, want [move-used dealt defeated gold-gained victory]", got)
		}
		gold := events[3]
		if amount, _ := gold.Data["amount"].(int); amount != 25 {
			t.Errorf("amount = %v, want 25 (5 × difficulty 5)", amount)
		}
		if source, _ := gold.Data["source"].(string); source != GoldSourceKill {
			t.Errorf("source = %v, want %q", source, GoldSourceKill)
		}
		if got := e.Player.Gold; got != 25 {
			t.Errorf("hero gold = %d, want 25", got)
		}
	})

	t.Run("mid-combat kills pay too, without ending the fight", func(t *testing.T) {
		e := newTestEngine(1, 50) // swing 40 — one-shots the goblin
		heroWithDamage(e, 40)
		startEncounter(e, monsterFromFixture("goblin"), monsterFromFixture("orc"))
		e.State = StateCombat

		e.Process(Command{Type: CmdMove, Index: 0}) // goblin (difficulty 3) falls

		if got := e.Player.Gold; got != 15 {
			t.Errorf("gold after the goblin = %d, want 15 (still mid-combat)", got)
		}
		if e.State != StateCombat {
			t.Errorf("state = %v, want StateCombat (combat continues)", e.State)
		}

		heroWithDamage(e, 60) // swing 60 — one-shots the orc
		e.Process(Command{Type: CmdMove, Index: 0})
		if got := e.Player.Gold; got != 40 {
			t.Errorf("gold after the orc = %d, want 40 (15 + 25)", got)
		}
	})

	t.Run("the gold grant consumes no rng", func(t *testing.T) {
		e := newTestEngine(1, 50)
		heroWithDamage(e, 40)
		startEncounter(e, monsterFromFixture("goblin"))
		e.State = StateCombat
		stub := &stubRng{values: []int{1, 50}}
		e.rng = stub

		e.Process(Command{Type: CmdMove, Index: 0}) // one strike, one kill

		// One strike consumes exactly spread + crit roll.
		if stub.calls != 2 {
			t.Errorf("rng calls = %d, want 2 (spread + crit only)", stub.calls)
		}
	})
}

// Opening pays gold instead of an item 40% of the time. Gold skips StateLoot
// but remains on the chest screen until acknowledgment.
func TestChestGold(t *testing.T) {
	t.Run("gold chest pays 5 × floor difficulty and skips StateLoot", func(t *testing.T) {
		e := pastFirstExplore(newKindEngine(20, 0)) // chest, then rng 0 < 40 → gold

		e.Process(Command{Type: CmdExplore})
		events := e.Process(Command{Type: CmdOpenChest})

		if got := eventTypes(events); !typesEqual(got, EventGoldGained) {
			t.Fatalf("events = %v, want [gold-gained]", got)
		}
		// Goblin Pits floor 1: difficulty 9 → 45 gold.
		if amount, _ := events[0].Data["amount"].(int); amount != 45 {
			t.Errorf("amount = %v, want 45 (5 × floor difficulty 9)", amount)
		}
		if source, _ := events[0].Data["source"].(string); source != GoldSourceChest {
			t.Errorf("source = %v, want %q", source, GoldSourceChest)
		}
		if got := e.Player.Gold; got != 45 {
			t.Errorf("hero gold = %d, want 45", got)
		}
		if e.State != StateChest {
			t.Errorf("state = %v, want StateChest (acknowledge gold)", e.State)
		}
		if e.PendingLoot != nil {
			t.Errorf("PendingLoot = %v, want nil", e.PendingLoot)
		}
		if e.CanAscend {
			t.Error("gold chest must not unlock ascending")
		}
	})

	t.Run("the amount scales with the floor difficulty", func(t *testing.T) {
		e := pastFirstExplore(newKindEngine(20, 39)) // still gold (39 < 40), on floor 2
		e.CurrentFloor = 2                           // difficulty 9 + 3 = 12

		e.Process(Command{Type: CmdExplore})
		events := e.Process(Command{Type: CmdOpenChest})

		if amount, _ := events[0].Data["amount"].(int); amount != 60 {
			t.Errorf("amount = %v, want 60 (5 × floor difficulty 12)", amount)
		}
	})

	t.Run("item chest keeps the loot decision (40 boundary excluded)", func(t *testing.T) {
		e := chestEngine(0) // rng [20 chest, 40 is not < 40, 0 pick]
		e.rng = &stubRng{values: []int{20, 40, 0}}

		e.Process(Command{Type: CmdExplore})
		events := e.Process(Command{Type: CmdOpenChest})

		if got := eventTypes(events); !typesEqual(got, EventLootFound) {
			t.Fatalf("events = %v, want [loot-found] (40 is not < 40)", got)
		}
		if e.State != StateLoot {
			t.Errorf("state = %v, want StateLoot", e.State)
		}
	})
}
