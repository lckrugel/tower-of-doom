package engine

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// tieredItems is a small registry spanning tiers 1 and 2. The tier-2 piece
// sorts FIRST ("adamant_*" < "t1_*"), so pick 0 discriminates a gated pool
// from an unrestricted one.
func tieredItems() map[string]game.ItemAsset {
	return map[string]game.ItemAsset{
		"adamant_crown": {ID: "adamant_crown", Name: "Adamant Crown", Slot: "helmet", BonusDEF: 9, Tier: 2},
		"t1_helm":       {ID: "t1_helm", Name: "T1 Helm", Slot: "helmet", BonusDEF: 2, Tier: 1},
		"t1_mail":       {ID: "t1_mail", Name: "T1 Mail", Slot: "body_armor", BonusDEF: 3, Tier: 1},
		"t1_ring":       {ID: "t1_ring", Name: "T1 Ring", Slot: "ring", BonusDEF: 1, Tier: 1},
		"t1_sword":      {ID: "t1_sword", Name: "T1 Sword", Slot: "weapon", BonusDMG: 5, Tier: 1},
	}
}

// tierSection is the mixed encounter table with item tiers restricted to
// `tiers` (nil = unrestricted).
func tierSection(tiers []int) game.SectionAsset {
	s := mixedSection()
	s.ItemTiers = tiers
	return s
}

// tieredEngine is a chest-bearing engine over `tiers` with the tiered
// registry. With the mixed table the chest rolls at rng 20. The run's
// forced first combat (pacing rule) is already spent, so EXPLORE rolls
// the kind table as usual.
func tieredEngine(tiers []int, rngValues ...int) *Engine {
	e := newKindEngine(rngValues...)
	e.reg.Sections = []game.SectionAsset{tierSection(tiers)}
	e.reg.Items = tieredItems()
	return pastFirstExplore(e)
}

// A section restricted by item_tiers can only roll items of those tiers —
// the chest pool excludes everything else (the tier-1 pool holds exactly
// four items here).
func TestChestRollsOnlyAllowedTiers(t *testing.T) {
	for _, pick := range []int{0, 1, 2, 3} {
		e := tieredEngine([]int{1}, 20, 50, pick)

		e.Process(Command{Type: CmdExplore})
		events := e.Process(Command{Type: CmdOpenChest})

		if got := eventTypes(events); !typesEqual(got, EventLootFound) {
			t.Fatalf("pick %d: events = %v, want [loot-found]", pick, got)
		}
		item, _ := events[0].Data["item"].(*entities.Item)
		if item == nil || item.Tier != 1 {
			t.Errorf("pick %d: chest rolled %v (tier %d), want a tier-1 item", pick, item, item.Tier)
		}
	}
}

// Without item_tiers the section rolls the whole registry, tier-2 items
// included — proving the restriction above is what gates the pool.
func TestUnrestrictedSectionRollsEveryTier(t *testing.T) {
	e := tieredEngine(nil, 20, 50, 0) // first sorted id → adamant_crown (tier 2)

	e.Process(Command{Type: CmdExplore})
	events := e.Process(Command{Type: CmdOpenChest})

	item, _ := events[0].Data["item"].(*entities.Item)
	if item == nil || item.ID != "adamant_crown" {
		t.Errorf("chest rolled %v, want adamant_crown (unrestricted pools reach tier 2)", item)
	}
}

// Ascension uses the standard chest's uniform, all-slot tier-filtered pool.
func TestAscendChestRespectsTiersAndAllowsAllSlots(t *testing.T) {
	for pick, wantID := range []string{"t1_helm", "t1_mail", "t1_ring", "t1_sword"} {
		e := tieredEngine([]int{1}, 40, pick)
		e.CanAscend = true

		events := e.Process(Command{Type: CmdAscend})
		events = append(events[:1], e.Process(Command{Type: CmdOpenChest})...)

		if got := eventTypes(events); !typesEqual(got, EventFloorEntered, EventLootFound) {
			t.Fatalf("pick %d: events = %v, want [floor-entered loot-found]", pick, got)
		}
		item := events[1].Data["item"].(*entities.Item)
		if item == nil || item.Tier != 1 || item.ID != wantID {
			t.Fatalf("pick %d: reward = %v, want %s (tier 1)", pick, item, wantID)
		}
		if e.State != StateLoot || e.PendingLoot != item {
			t.Errorf("pick %d: state = %v, pending = %v, want StateLoot holding the reward", pick, e.State, e.PendingLoot)
		}
		stub := e.rng.(*stubRng)
		if stub.calls != 2 || stub.bounds[0] != 100 || stub.bounds[1] != 4 {
			t.Errorf("rng bounds = %v, want [100 4] (chest branch, uniform item pick only)", stub.bounds)
		}
		switch entities.ItemSlot(item.Slot) {
		case entities.ItemSlot("helmet"):
			if e.Player.Head != nil {
				t.Error("armor reward auto-equipped in the helmet slot — must wait for the player decision")
			}
		case entities.ItemSlot("body_armor"):
			if e.Player.Body != nil {
				t.Error("armor reward auto-equipped in the body slot — must wait for the player decision")
			}
		}
	}
}

func TestSectionBoundaryAscendRewardUsesNewSectionTiers(t *testing.T) {
	// Fixture sections cover both the first boundary and legacy higher tiers;
	// this specifies engine behavior without locking tests to asset contents.
	for _, destinationTier := range []int{2, 3, 4} {
		e := tieredEngine(nil, 40, 0)
		oldFloor := 5 * (destinationTier - 1)
		e.reg.Items["next_weapon"] = game.ItemAsset{ID: "next_weapon", Name: "Next Weapon", Slot: "weapon", Tier: destinationTier}
		e.reg.Sections = []game.SectionAsset{
			{ID: "first", Name: "First Fixture Section", BaseDifficulty: 1,
				MinFloor: 1, MaxFloor: oldFloor, ItemTiers: []int{1}},
			{ID: "next", Name: "Next Fixture Section", BaseDifficulty: 1,
				MinFloor: oldFloor + 1, MaxFloor: -1, ItemTiers: []int{destinationTier}},
		}
		// Leave only the new weapon in this destination tier's pool.
		delete(e.reg.Items, "adamant_crown")
		e.CurrentFloor = oldFloor
		e.CanAscend = true
		e.BossUnlocked = true

		events := e.Process(Command{Type: CmdAscend})
		events = append(events[:1], e.Process(Command{Type: CmdOpenChest})...)
		if got := eventTypes(events); !typesEqual(got, EventFloorEntered, EventLootFound) {
			t.Fatalf("tier %d: events = %v, want [floor-entered loot-found]", destinationTier, got)
		}
		if events[0].Data["section"] != "Next Fixture Section" || events[0].Data["floor"] != oldFloor+1 {
			t.Fatalf("floor event = %v, want destination section/floor", events[0].Data)
		}
		item := events[1].Data["item"].(*entities.Item)
		if item.Tier != destinationTier || item.ID != "next_weapon" {
			t.Errorf("section-boundary reward = %v, want destination tier %d weapon", item, destinationTier)
		}
		if e.CanAscend || e.BossUnlocked || e.State != StateLoot || e.PendingLoot != item {
			t.Error("item chest did not clear old floor flags and open the loot decision")
		}
	}
}

// Unrestricted, the ascend chest pool reaches the tier-2 helmet (it is
// sorted first), so the gating above is what kept it out.
func TestAscendChestUnrestrictedReachesEveryTier(t *testing.T) {
	e := tieredEngine(nil, 40, 0)
	e.CanAscend = true

	events := e.Process(Command{Type: CmdAscend})
	events = append(events[:1], e.Process(Command{Type: CmdOpenChest})...)

	if got := eventTypes(events); !typesEqual(got, EventFloorEntered, EventLootFound) {
		t.Fatalf("events = %v, want [floor-entered loot-found]", got)
	}
	item := events[1].Data["item"].(*entities.Item)
	if item == nil || item.ID != "adamant_crown" {
		t.Errorf("chest reward = %v, want adamant_crown (unrestricted pools reach tier 2)", item)
	}
	if e.PendingLoot != item || e.State != StateLoot {
		t.Errorf("state = %v, pending = %v, want StateLoot holding the reward", e.State, e.PendingLoot)
	}
}

// A runtime-empty allowed pool (only possible for chests — merchant
// sections are validated stockable) degrades to the empty-chest message.
func TestChestWithEmptyAllowedTierPoolIsEmpty(t *testing.T) {
	e := tieredEngine([]int{7}, 20, 50, 0) // no tier-7 item exists; registry skips validation

	e.Process(Command{Type: CmdExplore})
	events := e.Process(Command{Type: CmdOpenChest})

	if got := eventTypes(events); !typesEqual(got, EventMessage) {
		t.Fatalf("events = %v, want [message (empty chest)]", got)
	}
	if e.State != StateChest || e.PendingLoot != nil || e.PendingChest == nil || !e.PendingChest.Opened || e.PendingChest.Gold != 0 {
		t.Errorf("state = %v, chest = %v, want opened empty chest", e.State, e.PendingChest)
	}
	e.Process(Command{Type: CmdContinue})
	if e.State != StateExploring || e.PendingChest != nil {
		t.Fatal("empty chest not cleared")
	}
}

// An empty ascent chest waits for acknowledgment, never StateLoot with a nil item.
func TestAscendWithEmptyItemPoolWaitsForContinue(t *testing.T) {
	e := tieredEngine([]int{7}, 40) // no tier-7 item exists; registry skips validation
	e.CanAscend = true

	events := e.Process(Command{Type: CmdAscend})
	events = append(events[:1], e.Process(Command{Type: CmdOpenChest})...)

	if got := eventTypes(events); !typesEqual(got, EventFloorEntered, EventMessage) {
		t.Fatalf("events = %v, want [floor-entered message]", got)
	}
	if e.State != StateChest || e.PendingLoot != nil || e.PendingChest == nil || !e.PendingChest.Opened {
		t.Errorf("state = %v, chest = %v, want opened empty chest", e.State, e.PendingChest)
	}
	if e.CanAscend {
		t.Error("ascend did not consume the CanAscend flag")
	}
	if events[1].Data["message"] != "The chest creaks open... but it is empty." {
		t.Errorf("empty chest message = %v", events[1].Data)
	}
	stub := e.rng.(*stubRng)
	if stub.calls != 1 || stub.bounds[0] != 100 {
		t.Errorf("rng bounds = %v, want [100] (empty pool has no item pick)", stub.bounds)
	}
}
