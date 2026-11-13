package engine

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
)

func TestAscendGoldChestUsesDestinationBudget(t *testing.T) {
	e := newKindEngine(44) // Even before the first EXPLORE, ascent is not exploration.
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
	if !typesEqual(eventTypes(opened), EventGoldGained) || opened[0].Data["amount"] != 119 || opened[0].Data["source"] != GoldSourceChest || e.Player.Gold != 126 {
		t.Fatalf("open = %v, gold = %d", opened, e.Player.Gold)
	}
	if e.State != StateChest || e.PendingChest == nil || !e.PendingChest.Opened || e.PendingChest.Gold != 119 || e.PendingLoot != nil {
		t.Fatal("gold chest did not retain its reward for acknowledgment")
	}
	if repeated := e.Process(Command{Type: CmdOpenChest}); !typesEqual(eventTypes(repeated), EventMessage) || e.Player.Gold != 126 {
		t.Fatal("repeat open awarded another reward")
	}
	continued := e.Process(Command{Type: CmdContinue})
	if len(continued) != 0 || e.State != StateExploring || e.PendingChest != nil || e.PendingLoot != nil || e.Player.Gold != 126 {
		t.Fatal("continue changed gold or failed to clear the chest")
	}
	if e.firstExploreDone != firstExplore || e.recentKinds != history || e.encounterMisses != misses {
		t.Error("ascension chest changed exploration pacing/history")
	}
	stub := e.rng.(*stubRng)
	if stub.calls != 1 || stub.bounds[0] != 100 {
		t.Errorf("rng bounds = %v, want [100] (no appearance or kind roll)", stub.bounds)
	}
}

// Kills grant XP but never gold, including between queued enemies.
func TestKillsGrantXPWithoutGold(t *testing.T) {
	t.Run("ordinary kill preserves the purse", func(t *testing.T) {
		e := newTestEngine(1, 50) // swing 60 — one-shots the orc
		heroWithDamage(e, 60)
		startEncounter(e, monsterFromFixture("orc"))
		e.State = StateCombat
		e.Player.Gold = 7

		events := e.Process(Command{Type: CmdMove, Index: 0})

		if got := eventTypes(events); !typesEqual(got,
			EventMoveUsed, EventCombatDamageDealt, EventEnemyDefeated, EventVictory) {
			t.Fatalf("events = %v, want [move-used dealt defeated victory]", got)
		}
		if e.Player.Gold != 7 || e.Player.Xp != 45 {
			t.Errorf("gold/XP = %d/%d, want 7/45", e.Player.Gold, e.Player.Xp)
		}
	})

	t.Run("mid-combat kills grant XP without gold or ending the fight", func(t *testing.T) {
		e := newTestEngine(1, 50) // swing 40 — one-shots the goblin
		heroWithDamage(e, 40)
		startEncounter(e, monsterFromFixture("goblin"), monsterFromFixture("orc"))
		e.State = StateCombat
		e.Player.Gold = 7

		events := e.Process(Command{Type: CmdMove, Index: 0})

		if !typesEqual(eventTypes(events), EventMoveUsed, EventCombatDamageDealt, EventEnemyDefeated) || e.Player.Gold != 7 || e.Player.Xp != 21 {
			t.Errorf("mid-combat events=%v gold/XP=%d/%d, want defeat and 7/21", eventTypes(events), e.Player.Gold, e.Player.Xp)
		}
		if e.State != StateCombat {
			t.Errorf("state = %v, want StateCombat (combat continues)", e.State)
		}

		heroWithDamage(e, 60) // swing 60 — one-shots the orc
		e.Process(Command{Type: CmdMove, Index: 0})
		if e.Player.Gold != 7 || e.Player.Xp != 66 {
			t.Errorf("final gold/XP = %d/%d, want 7/66", e.Player.Gold, e.Player.Xp)
		}
	})

	t.Run("kill settlement consumes no rng", func(t *testing.T) {
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

// Opening pays gold instead of an item 45% of the time. Gold skips StateLoot
// but remains on the chest screen until acknowledgment.
func TestChestGold(t *testing.T) {
	t.Run("gold chest pays 7 × floor difficulty and skips StateLoot", func(t *testing.T) {
		e := pastFirstExplore(newKindEngine(20, 0)) // chest, then rng 0 < 45 → gold

		e.Process(Command{Type: CmdExplore})
		events := e.Process(Command{Type: CmdOpenChest})

		if got := eventTypes(events); !typesEqual(got, EventGoldGained) {
			t.Fatalf("events = %v, want [gold-gained]", got)
		}
		// Goblin Pits floor 1: difficulty 9 → 63 gold.
		if amount, _ := events[0].Data["amount"].(int); amount != 63 {
			t.Errorf("amount = %v, want 63 (7 × floor difficulty 9)", amount)
		}
		if source, _ := events[0].Data["source"].(string); source != GoldSourceChest {
			t.Errorf("source = %v, want %q", source, GoldSourceChest)
		}
		if got := e.Player.Gold; got != 63 {
			t.Errorf("hero gold = %d, want 63", got)
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
		e := pastFirstExplore(newKindEngine(20, 44)) // still gold (44 < 45), on floor 2
		e.CurrentFloor = 2                           // difficulty 9 + 3 = 12

		e.Process(Command{Type: CmdExplore})
		events := e.Process(Command{Type: CmdOpenChest})

		if amount, _ := events[0].Data["amount"].(int); amount != 84 {
			t.Errorf("amount = %v, want 84 (7 × floor difficulty 12)", amount)
		}
		if stub := e.rng.(*stubRng); stub.calls != 2 || stub.bounds[0] != 30 || stub.bounds[1] != 100 {
			t.Errorf("RNG bounds = %v, want [30 100] (kind then branch)", stub.bounds)
		}
	})

	t.Run("item chest keeps the loot decision (45 boundary excluded)", func(t *testing.T) {
		e := chestEngine(0) // rng [20 chest, 45 is not < 45, 0 pick]
		e.rng = &stubRng{values: []int{20, 45, 0}}

		e.Process(Command{Type: CmdExplore})
		events := e.Process(Command{Type: CmdOpenChest})

		if got := eventTypes(events); !typesEqual(got, EventLootFound) {
			t.Fatalf("events = %v, want [loot-found] (45 is not < 45)", got)
		}
		if e.State != StateLoot {
			t.Errorf("state = %v, want StateLoot", e.State)
		}
		if e.PendingChest != nil || e.PendingLoot == nil || e.Player.Gold != 0 {
			t.Fatal("item branch changed gold or left invalid pending rewards")
		}
		if stub := e.rng.(*stubRng); stub.calls != 3 || stub.bounds[0] != 30 || stub.bounds[1] != 100 || stub.bounds[2] != 2 {
			t.Errorf("RNG bounds = %v, want [30 100 2]", stub.bounds)
		}
	})
}
