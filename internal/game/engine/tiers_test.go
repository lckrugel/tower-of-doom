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

		events := e.Process(Command{Type: CmdExplore})

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

	events := e.Process(Command{Type: CmdExplore})

	item, _ := events[0].Data["item"].(*entities.Item)
	if item == nil || item.ID != "adamant_crown" {
		t.Errorf("chest rolled %v, want adamant_crown (unrestricted pools reach tier 2)", item)
	}
}

// The ascend armor reward respects the section's item_tiers too — and,
// like a chest, it opens the StateLoot decision instead of auto-equipping.
func TestAscendArmorRespectsTiers(t *testing.T) {
	for _, pick := range []int{0, 1} { // the tier-1 armor pool holds exactly 2 pieces
		e := tieredEngine([]int{1}, pick)
		e.CanAscend = true

		events := e.Process(Command{Type: CmdAscend})

		if got := eventTypes(events); !typesEqual(got, EventFloorEntered, EventLootFound) {
			t.Fatalf("pick %d: events = %v, want [floor-entered loot-found]", pick, got)
		}
		item := events[1].Data["item"].(*entities.Item)
		if item == nil || item.Tier != 1 {
			t.Errorf("pick %d: armor reward = %v (tier %d), want a tier-1 armor piece", pick, item, item.Tier)
		}
		if e.State != StateLoot || e.PendingLoot != item {
			t.Errorf("pick %d: state = %v, pending = %v, want StateLoot holding the reward", pick, e.State, e.PendingLoot)
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

// Unrestricted, the ascend armor pool reaches the tier-2 helmet (it is
// sorted first), so the gating above is what kept it out.
func TestAscendArmorUnrestrictedReachesEveryTier(t *testing.T) {
	e := tieredEngine(nil, 0)
	e.CanAscend = true

	events := e.Process(Command{Type: CmdAscend})

	if got := eventTypes(events); !typesEqual(got, EventFloorEntered, EventLootFound) {
		t.Fatalf("events = %v, want [floor-entered loot-found]", got)
	}
	item := events[1].Data["item"].(*entities.Item)
	if item == nil || item.ID != "adamant_crown" {
		t.Errorf("armor reward = %v, want adamant_crown (unrestricted pools reach tier 2)", item)
	}
	if e.PendingLoot != item || e.State != StateLoot {
		t.Errorf("state = %v, pending = %v, want StateLoot holding the reward", e.State, e.PendingLoot)
	}
}

// A runtime-empty allowed pool (only possible for chests — merchant
// sections are validated stockable) degrades to the empty-chest message.
func TestChestWithEmptyAllowedTierPoolIsEmpty(t *testing.T) {
	e := tieredEngine([]int{7}, 20, 50, 0) // no tier-7 item exists; registry skips validation

	events := e.Process(Command{Type: CmdExplore})

	if got := eventTypes(events); !typesEqual(got, EventMessage) {
		t.Fatalf("events = %v, want [message (empty chest)]", got)
	}
	if e.State != StateExploring || e.PendingLoot != nil {
		t.Errorf("state = %v, pending = %v, want Exploring/nil", e.State, e.PendingLoot)
	}
}

// An empty armor pool on ascend behaves like an empty chest too: the floor
// is entered, a message is emitted and the run stays exploring with
// nothing pending — never StateLoot holding a nil item.
func TestAscendWithEmptyArmorPoolStaysExploring(t *testing.T) {
	e := tieredEngine([]int{7}, 0) // no tier-7 item exists; registry skips validation
	e.CanAscend = true

	events := e.Process(Command{Type: CmdAscend})

	if got := eventTypes(events); !typesEqual(got, EventFloorEntered, EventMessage) {
		t.Fatalf("events = %v, want [floor-entered message]", got)
	}
	if e.State != StateExploring || e.PendingLoot != nil {
		t.Errorf("state = %v, pending = %v, want Exploring/nil", e.State, e.PendingLoot)
	}
	if e.CanAscend {
		t.Error("ascend did not consume the CanAscend flag")
	}
}
