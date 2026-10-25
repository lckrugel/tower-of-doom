package engine

import (
	"slices"
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// ——— Boss challenge fixtures ———

// bossTestSections is the pits (floors 1-5) with a boss block: the final
// floor 5 gates the ascension behind the boss challenge.
func bossTestSections() []game.SectionAsset {
	s := testSections()[0] // pits, floors 1-5, budget 9 + 3·(floor-1)
	s.Boss = &game.BossAsset{Monsters: []string{"chief", "goblin"}, Gold: 100}
	return []game.SectionAsset{s}
}

// newBossEngine is newTestEngine over the boss section, with the chief's
// move refs resolvable (a move in the registry for it to reference).
func newBossEngine(rngValues ...int) *Engine {
	e := newTestEngine(rngValues...)
	e.reg.Sections = bossTestSections()
	e.reg.Moves = map[string]game.MoveAsset{
		"smash": {ID: "smash", Name: "Smash", Tier: 1, DmgModifier: 120, Weight: 10},
	}
	e.reg.Monsters["chief"] = game.MonsterAsset{
		ID: "chief", Name: "Goblin Chief", Difficulty: 9, HP: 55, Damage: 6, Speed: 10,
		Defense: 3, Crit: 10,
		Moves: []game.MoveRef{{ID: "smash", Weight: 10}},
	}
	return e
}

// bossTierItems spans tiers 1-3; the sorted tier-2 pool is
// [t2_helm, t2_ring].
func bossTierItems() map[string]game.ItemAsset {
	return map[string]game.ItemAsset{
		"t1_sword": {ID: "t1_sword", Name: "T1 Sword", Slot: "weapon", BonusDMG: 5, Tier: 1},
		"t2_ring":  {ID: "t2_ring", Name: "T2 Ring", Slot: "ring", BonusDEF: 2, Tier: 2},
		"t2_helm":  {ID: "t2_helm", Name: "T2 Helm", Slot: "helmet", BonusDEF: 3, Tier: 2},
		"t3_mail":  {ID: "t3_mail", Name: "T3 Mail", Slot: "body_armor", BonusDEF: 9, Tier: 3},
	}
}

// ——— The two regimes of a combat victory ———

// A normal-combat victory on a NON-final floor keeps the old regime:
// ASCEND unlocks, the boss challenge does not.
func TestNormalVictoryOnARegularFloorUnlocksAscend(t *testing.T) {
	e := newBossEngine()
	e.CurrentFloor = 2 // not the section's final floor (5)
	heroWithDamage(e, 40)
	startEncounter(e, monsterFromFixture("goblin"))
	e.State = StateCombat

	events := e.Process(Command{Type: CmdMove, Index: 0})

	if got := eventTypes(events); !typesEqual(got,
		EventMoveUsed, EventCombatDamageDealt, EventEnemyDefeated, EventGoldGained, EventVictory) {
		t.Fatalf("events = %v, want the plain victory flow", got)
	}
	if !e.CanAscend {
		t.Error("regular-floor victory did not unlock ascending")
	}
	if e.BossUnlocked {
		t.Error("regular-floor victory unlocked the boss challenge")
	}
}

// On the section's FINAL floor a normal victory does NOT open the gate:
// it unlocks the boss challenge instead.
func TestFinalFloorVictoryUnlocksTheBossChallenge(t *testing.T) {
	e := newBossEngine()
	e.CurrentFloor = 5 // the pits' final floor
	heroWithDamage(e, 40)
	startEncounter(e, monsterFromFixture("goblin"))
	e.State = StateCombat

	events := e.Process(Command{Type: CmdMove, Index: 0})

	if got := eventTypes(events); !typesEqual(got,
		EventMoveUsed, EventCombatDamageDealt, EventEnemyDefeated, EventGoldGained, EventBossUnlocked) {
		t.Fatalf("events = %v, want [...gold-gained boss-unlocked]", got)
	}
	if !e.BossUnlocked {
		t.Error("final-floor victory did not unlock the boss challenge")
	}
	if e.CanAscend {
		t.Error("final-floor victory must NOT unlock ascending (the boss gates it)")
	}
	if e.State != StateExploring {
		t.Errorf("state = %v, want StateExploring", e.State)
	}
}

// A section without a boss block keeps the old behavior on its final
// floor: the victory opens the gate (mod compatibility).
func TestFinalFloorWithoutBossKeepsTheOldRegime(t *testing.T) {
	e := newBossEngine()
	e.reg.Sections = testSections() // no boss block
	e.CurrentFloor = 5
	heroWithDamage(e, 40)
	startEncounter(e, monsterFromFixture("goblin"))
	e.State = StateCombat

	events := e.Process(Command{Type: CmdMove, Index: 0})

	if got := eventTypes(events); !typesEqual(got,
		EventMoveUsed, EventCombatDamageDealt, EventEnemyDefeated, EventGoldGained, EventVictory) {
		t.Fatalf("events = %v, want the plain victory flow", got)
	}
	if !e.CanAscend || e.BossUnlocked {
		t.Errorf("CanAscend = %v, BossUnlocked = %v, want the no-boss regime (true/false)",
			e.CanAscend, e.BossUnlocked)
	}
}

// ——— The challenge command ———

// The challenge command exists in StateExploring only.
func TestChallengeBossAllowedCommands(t *testing.T) {
	if got := AllowedCommands(StateExploring); !slices.Contains(got, CmdChallengeBoss) {
		t.Error("StateExploring does not allow CmdChallengeBoss")
	}
	for _, state := range []GameState{
		StateEncounter, StateCombat, StateLoot, StateInventory, StateMerchant, StateGameOver,
	} {
		if slices.Contains(AllowedCommands(state), CmdChallengeBoss) {
			t.Errorf("state %v allows CmdChallengeBoss", state)
		}
	}
}

// CHALLENGE while locked is a blocked action: a message, state intact,
// no rng — like a locked ASCEND.
func TestChallengeBossWhileLockedIsBlocked(t *testing.T) {
	e := newBossEngine()
	stub := &stubRng{values: []int{0}}
	e.rng = stub

	events := e.Process(Command{Type: CmdChallengeBoss})

	if got := eventTypes(events); !typesEqual(got, EventMessage) {
		t.Fatalf("events = %v, want [message]", got)
	}
	if e.State != StateExploring {
		t.Errorf("state = %v, want StateExploring (blocked command)", e.State)
	}
	if stub.calls != 0 {
		t.Errorf("blocked challenge consumed %d rng calls, want 0", stub.calls)
	}
}

// CHALLENGE engages the PREDEFINED boss group straight into combat:
// authored order, boss flag set, zero rng consumed (no kind roll, no
// budget), and the kind history is untouched (a challenge is not an
// explore).
func TestChallengeBossEngagesThePredefinedGroup(t *testing.T) {
	e := newBossEngine()
	e.CurrentFloor = 5
	e.BossUnlocked = true
	stub := &stubRng{values: []int{0}}
	e.rng = stub
	e.recentKinds = [2]EncounterType{EncounterChest, EncounterChest}

	events := e.Process(Command{Type: CmdChallengeBoss})

	if got := eventTypes(events); !typesEqual(got, EventBossChallenge) {
		t.Fatalf("events = %v, want [boss-challenge]", got)
	}
	if e.State != StateCombat {
		t.Errorf("state = %v, want StateCombat (no StateEncounter, no sneak step)", e.State)
	}
	if !e.CurrentEncounter.Boss {
		t.Error("the challenge encounter is not boss-flagged")
	}
	if !idsEqual(monsterIDs(e.CurrentEncounter.Monsters), "chief", "goblin") {
		t.Errorf("encounter = %v, want the authored order [chief goblin]",
			monsterIDs(e.CurrentEncounter.Monsters))
	}
	// The boss resolves its move refs exactly like a regular spawn.
	moves := e.CurrentEncounter.Monsters[0].Moves()
	if len(moves) != 1 || moves[0].Name != "Smash" {
		t.Errorf("chief moves = %v, want the resolved [Smash]", moves)
	}
	if stub.calls != 0 {
		t.Errorf("the challenge consumed %d rng calls, want 0", stub.calls)
	}
	if e.recentKinds != [2]EncounterType{EncounterChest, EncounterChest} {
		t.Error("the challenge fed the encounter-kind history")
	}
	if e.CanAscend {
		t.Error("accepting the challenge must not unlock ascending")
	}
}

// ——— Fleeing ———

// Fleeing the BOSS re-locks the challenge (another normal-combat victory
// re-unlocks it) and never opens the gate; fleeing a normal combat never
// touches the unlock.
func TestFleeAndTheBossChallenge(t *testing.T) {
	t.Run("fleeing the boss re-locks the challenge", func(t *testing.T) {
		e := newBossEngine(0) // Intn(100) = 0 < chance 50 → the flee succeeds
		e.BossUnlocked = true
		startEncounter(e, monsterFromFixture("goblin"))
		e.CurrentEncounter.Boss = true
		e.State = StateCombat

		events := e.Process(Command{Type: CmdFlee})

		if got := eventTypes(events); !typesEqual(got, EventFled) {
			t.Fatalf("events = %v, want [fled]", got)
		}
		if e.BossUnlocked {
			t.Error("fleeing the boss left the challenge unlocked")
		}
		if e.CanAscend {
			t.Error("fleeing the boss must not unlock ascending")
		}
		if e.Player.Xp != 0 || e.Player.Gold != 0 {
			t.Errorf("xp = %d, gold = %d, want 0/0 (fleeing grants no reward)",
				e.Player.Xp, e.Player.Gold)
		}
	})

	t.Run("fleeing a normal combat leaves the unlock alone", func(t *testing.T) {
		e := newBossEngine(0)
		e.BossUnlocked = true
		startEncounter(e, monsterFromFixture("goblin")) // no boss flag
		e.State = StateCombat

		e.Process(Command{Type: CmdFlee})

		if !e.BossUnlocked {
			t.Error("fleeing a normal combat reset the boss unlock")
		}
	})

	t.Run("a failed flee keeps fighting the boss", func(t *testing.T) {
		// Intn(100) = 99 → the flee fails; the free strike rolls average
		// spread, no crit.
		e := newBossEngine(99, 0, 1, 50)
		heroWithDamage(e, 5)
		e.BossUnlocked = true
		startEncounter(e, monsterFromFixture("goblin"))
		e.CurrentEncounter.Boss = true
		e.State = StateCombat

		events := e.Process(Command{Type: CmdFlee})

		if got := eventTypes(events); !typesEqual(got, EventMessage, EventMoveUsed, EventCombatDamageTaken) {
			t.Fatalf("events = %v, want [message move-used taken (free turn)]", got)
		}
		if !e.BossUnlocked {
			t.Error("a failed flee re-locked the challenge")
		}
		if e.State != StateCombat {
			t.Errorf("state = %v, want StateCombat (the fight goes on)", e.State)
		}
	})
}

// ——— The boss victory ———

// The BOSS victory: the per-kill flow first (XP + kill gold), then the
// tier+1 loot roll (ONE Intn over the pool), the gold bonus and the
// victory. The engine ends in StateLoot with the reward pending, the
// gate finally open.
func TestBossVictoryRewards(t *testing.T) {
	// The stub feeds the hero's strike (spread 1, crit 50 → a plain
	// one-shot) and then the loot pick.
	setup := func(rngPick int) *Engine {
		e := newBossEngine()
		e.reg.Items = bossTierItems()
		e.reg.Sections[0].ItemTiers = []int{1} // boss loot target: tier 2
		e.CurrentFloor = 5
		heroWithDamage(e, 40)
		startEncounter(e, monsterFromFixture("goblin")) // the last boss monster
		e.CurrentEncounter.Boss = true
		e.State = StateCombat
		e.rng = &stubRng{values: []int{1, 50, rngPick}}
		return e
	}

	t.Run("event order, rewards and the loot decision", func(t *testing.T) {
		e := setup(1) // sorted tier-2 pool [t2_helm, t2_ring] → t2_ring

		events := e.Process(Command{Type: CmdMove, Index: 0})

		if got := eventTypes(events); !typesEqual(got,
			EventMoveUsed, EventCombatDamageDealt, EventEnemyDefeated, EventGoldGained,
			EventLootFound, EventGoldGained, EventVictory) {
			t.Fatalf("events = %v, want the full boss victory flow", got)
		}
		// Per-kill gold first (5 × difficulty 3), the boss bonus second.
		if amount, _ := events[3].Data["amount"].(int); amount != 15 {
			t.Errorf("kill gold = %v, want 15", events[3].Data["amount"])
		}
		if source, _ := events[3].Data["source"].(string); source != GoldSourceKill {
			t.Errorf("kill gold source = %v, want %q", source, GoldSourceKill)
		}
		if amount, _ := events[5].Data["amount"].(int); amount != 100 {
			t.Errorf("boss gold = %v, want 100 (the asset's bonus)", events[5].Data["amount"])
		}
		if source, _ := events[5].Data["source"].(string); source != GoldSourceBoss {
			t.Errorf("boss gold source = %v, want %q", source, GoldSourceBoss)
		}
		if got := e.Player.Gold; got != 115 {
			t.Errorf("hero gold = %d, want 115 (kill 15 + bonus 100)", got)
		}
		// The rolled item is held as PendingLoot: the chest decision.
		item, _ := events[4].Data["item"].(*entities.Item)
		if item == nil || item.ID != "t2_ring" {
			t.Errorf("loot = %v, want t2_ring (rng pick 1 over [t2_helm, t2_ring])", item)
		}
		if e.PendingLoot != item || e.State != StateLoot {
			t.Errorf("state = %v, pending = %v, want StateLoot holding the reward",
				e.State, e.PendingLoot)
		}
		// The gate opens; the unlock is consumed.
		if !e.CanAscend {
			t.Error("the boss victory did not unlock ascending")
		}
		if e.BossUnlocked {
			t.Error("the boss victory left the challenge unlocked")
		}
		// Exactly the strike (spread + crit) plus the ONE loot roll.
		if got := e.rng.(*stubRng).calls; got != 3 {
			t.Errorf("rng calls = %d, want 3 (spread + crit + one loot pick)", got)
		}
	})

	t.Run("the pick follows the rng over the sorted pool", func(t *testing.T) {
		e := setup(0) // first of [t2_helm, t2_ring]

		events := e.Process(Command{Type: CmdMove, Index: 0})

		item, _ := events[4].Data["item"].(*entities.Item)
		if item == nil || item.ID != "t2_helm" {
			t.Errorf("loot = %v, want t2_helm (rng pick 0)", item)
		}
	})

	t.Run("an empty tier+1 pool skips the item (and the rng), keeps the gold", func(t *testing.T) {
		e := setup(0)
		e.reg.Items = map[string]game.ItemAsset{
			"t1_sword": {ID: "t1_sword", Name: "T1 Sword", Slot: "weapon", BonusDMG: 5, Tier: 1},
			"t3_mail":  {ID: "t3_mail", Name: "T3 Mail", Slot: "body_armor", BonusDEF: 9, Tier: 3},
		} // no tier-2 item exists: the tier+1 pool (target 2) is empty

		events := e.Process(Command{Type: CmdMove, Index: 0})

		if got := eventTypes(events); !typesEqual(got,
			EventMoveUsed, EventCombatDamageDealt, EventEnemyDefeated, EventGoldGained,
			EventGoldGained, EventVictory) {
			t.Fatalf("events = %v, want the no-loot victory flow", got)
		}
		if e.State != StateExploring || e.PendingLoot != nil {
			t.Errorf("state = %v, pending = %v, want Exploring/nil", e.State, e.PendingLoot)
		}
		if got := e.Player.Gold; got != 115 {
			t.Errorf("gold = %d, want 115 (the gold bonus is kept)", got)
		}
		if got := e.rng.(*stubRng).calls; got != 2 {
			t.Errorf("rng calls = %d, want 2 (an empty pool never rolls)", got)
		}
		if !e.CanAscend || e.BossUnlocked {
			t.Errorf("CanAscend = %v, BossUnlocked = %v, want true/false", e.CanAscend, e.BossUnlocked)
		}
	})

	t.Run("the tier+1 target caps at the registry's max tier", func(t *testing.T) {
		e := setup(0)
		e.reg.Sections[0].ItemTiers = []int{3} // tier 4 wanted; the registry stops at 3

		events := e.Process(Command{Type: CmdMove, Index: 0})

		item, _ := events[4].Data["item"].(*entities.Item)
		if item == nil || item.ID != "t3_mail" {
			t.Errorf("loot = %v, want t3_mail (capped at the max tier)", item)
		}
	})

	t.Run("an unrestricted section rolls at the registry's max tier", func(t *testing.T) {
		e := setup(0)
		e.reg.Sections[0].ItemTiers = nil // unrestricted: the top allowed tier IS the max

		events := e.Process(Command{Type: CmdMove, Index: 0})

		item, _ := events[4].Data["item"].(*entities.Item)
		if item == nil || item.ID != "t3_mail" {
			t.Errorf("loot = %v, want t3_mail (registry max tier)", item)
		}
	})
}

// A boss with no gold bonus (absent/0) skips the boss-gold event but
// keeps everything else — the loot, the victory, the gate.
func TestBossVictoryWithoutGoldBonus(t *testing.T) {
	e := newBossEngine()
	e.reg.Sections[0].Boss = &game.BossAsset{Monsters: []string{"chief", "goblin"}} // Gold 0
	e.CurrentFloor = 5
	heroWithDamage(e, 40)
	startEncounter(e, monsterFromFixture("goblin"))
	e.CurrentEncounter.Boss = true
	e.State = StateCombat
	e.rng = &stubRng{values: []int{1, 50}} // the strike; the empty item registry never rolls

	events := e.Process(Command{Type: CmdMove, Index: 0})

	if got := eventTypes(events); !typesEqual(got,
		EventMoveUsed, EventCombatDamageDealt, EventEnemyDefeated, EventGoldGained, EventVictory) {
		t.Fatalf("events = %v, want the kill gold only", got)
	}
	if got := e.Player.Gold; got != 15 {
		t.Errorf("gold = %d, want 15 (kill gold only)", got)
	}
	if !e.CanAscend {
		t.Error("the boss victory must unlock ascending")
	}
}

// ——— The unlock's lifecycle ———

// ASCEND consumes the unlock: the new floor starts fresh.
func TestAscendResetsTheBossUnlock(t *testing.T) {
	e := newBossEngine()
	e.BossUnlocked = true
	e.CanAscend = true

	e.Process(Command{Type: CmdAscend})

	if e.BossUnlocked {
		t.Error("ascending left the boss unlock set")
	}
}

// The full final-floor loop: victory unlocks the challenge, the challenge
// is accepted, the boss is fled (re-lock), another normal victory
// re-unlocks it — and ASCEND stays locked throughout until the boss falls.
func TestBossChallengeRelockLoop(t *testing.T) {
	e := newBossEngine()
	e.CurrentFloor = 5
	heroWithDamage(e, 40)

	startEncounter(e, monsterFromFixture("goblin"))
	e.State = StateCombat
	e.Process(Command{Type: CmdMove, Index: 0}) // victory #1 → the challenge unlocks
	if !e.BossUnlocked || e.CanAscend {
		t.Fatalf("first victory: BossUnlocked = %v, CanAscend = %v, want true/false",
			e.BossUnlocked, e.CanAscend)
	}

	e.Process(Command{Type: CmdChallengeBoss})
	if e.State != StateCombat {
		t.Fatalf("challenge state = %v, want StateCombat", e.State)
	}
	e.rng = &stubRng{values: []int{0}} // flee chance 50 → 0 succeeds
	e.Process(Command{Type: CmdFlee})
	if e.BossUnlocked || e.CanAscend {
		t.Fatalf("after the flee: BossUnlocked = %v, CanAscend = %v, want false/false",
			e.BossUnlocked, e.CanAscend)
	}

	startEncounter(e, monsterFromFixture("goblin"))
	e.State = StateCombat
	e.Process(Command{Type: CmdMove, Index: 0}) // victory #2 re-unlocks
	if !e.BossUnlocked {
		t.Error("the second victory did not re-unlock the challenge")
	}
	if e.CanAscend {
		t.Error("the second victory unlocked ascending on a boss final floor")
	}
}
