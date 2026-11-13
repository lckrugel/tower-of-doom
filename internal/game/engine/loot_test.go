package engine

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// chestEngine returns an engine whose rng rolls a chest (20 → chest in the
// mixed section), then an ITEM chest (50 ≥ chestGoldChance), then
// `wantPick` for the item pool, plus a mixed-section registry with two
// items: sorted pool [rusty_ring, shiny_sword]. The run's forced first
// combat (pacing rule) is already spent, so EXPLORE rolls the kind table
// as usual.
func chestEngine(wantPick int, rngValues ...int) *Engine {
	values := append([]int{20, 50, wantPick}, rngValues...)
	e := newKindEngine(values...)
	e.reg.Sections = []game.SectionAsset{mixedSection()}
	e.reg.Items = map[string]game.ItemAsset{
		"shiny_sword": {ID: "shiny_sword", Name: "Shiny Sword", Slot: "weapon", BonusDMG: 5},
		"rusty_ring":  {ID: "rusty_ring", Name: "Rusty Ring", Slot: "ring", BonusDEF: 1},
	}
	return pastFirstExplore(e)
}

// Chest commands depend on the current step, and reject invalid actions safely.
func TestChestCommandSteps(t *testing.T) {
	for _, tc := range []struct {
		name    string
		state   GameState
		chest   *ChestState
		allowed []CommandType
	}{
		{"unopened", StateChest, &ChestState{}, []CommandType{CmdOpenChest, CmdQuit}},
		{"gold", StateChest, &ChestState{Opened: true, Gold: 45}, []CommandType{CmdContinue, CmdQuit}},
		{"empty", StateChest, &ChestState{Opened: true}, []CommandType{CmdContinue, CmdQuit}},
		{"missing", StateChest, nil, []CommandType{CmdQuit}},
		{"outside", StateExploring, nil, AllowedCommands(StateExploring)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := chestEngine(0)
			e.State, e.PendingChest = tc.state, tc.chest
			commands := e.AvailableCommands()
			if len(commands) != len(tc.allowed) {
				t.Fatalf("commands = %v, want %v", commands, tc.allowed)
			}
			for i := range commands {
				if commands[i] != tc.allowed[i] {
					t.Fatalf("commands = %v, want %v", commands, tc.allowed)
				}
			}
			for _, cmd := range []CommandType{CmdOpenChest, CmdContinue, CmdExplore, CmdAscend, CmdInventory, CmdUseConsumable} {
				allowed := false
				for _, c := range tc.allowed {
					allowed = allowed || c == cmd
				}
				if allowed {
					continue
				}
				events := e.Process(Command{Type: cmd})
				if !typesEqual(eventTypes(events), EventMessage) || e.State != tc.state || e.PendingChest != tc.chest || e.rng.(*stubRng).calls != 0 {
					t.Fatalf("rejected %v changed state or RNG: %v", cmd, events)
				}
			}
		})
	}
}

func TestChestGoldAcknowledgment(t *testing.T) {
	e := pastFirstExplore(newKindEngine(20, 44))
	e.Process(Command{Type: CmdExplore})
	history, misses := e.recentKinds, e.encounterMisses
	if !typesEqual(eventTypes(e.Process(Command{Type: CmdOpenChest})), EventGoldGained) || e.State != StateChest || e.PendingChest == nil || !e.PendingChest.Opened || e.PendingChest.Gold != 63 || e.Player.Gold != 63 {
		t.Fatal("gold reward not retained for acknowledgment")
	}
	calls := e.rng.(*stubRng).calls
	e.Process(Command{Type: CmdOpenChest})
	events := e.Process(Command{Type: CmdContinue})
	if len(events) != 0 || e.State != StateExploring || e.PendingChest != nil || e.Player.Gold != 63 || e.rng.(*stubRng).calls != calls || e.recentKinds != history || e.encounterMisses != misses {
		t.Fatal("repeat open or continue changed rewards, RNG, or exploration history")
	}
}

// Exploration discovers the chest; opening rolls an item without auto-equipping.
func TestExploreChestOpensLootDecision(t *testing.T) {
	// rng 20 → chest; 1 → shiny_sword from the sorted pool.
	e := chestEngine(1)

	discovery := e.Process(Command{Type: CmdExplore})
	if !typesEqual(eventTypes(discovery), EventChestFound) || e.State != StateChest || e.PendingChest == nil || e.PendingChest.Opened {
		t.Fatalf("discovery = %v, state = %v, chest = %v", discovery, e.State, e.PendingChest)
	}
	if e.rng.(*stubRng).calls != 1 || e.PendingLoot != nil || e.Player.Gold != 0 {
		t.Fatal("discovery rolled or awarded a reward")
	}
	events := e.Process(Command{Type: CmdOpenChest})
	if e.PendingChest != nil {
		t.Fatal("item reward retained PendingChest")
	}

	if got := eventTypes(events); !typesEqual(got, EventLootFound) {
		t.Fatalf("events = %v, want [loot-found]", got)
	}
	item, _ := events[0].Data["item"].(*entities.Item)
	if item == nil || item.ID != "shiny_sword" {
		t.Errorf("loot item = %v, want shiny_sword", item)
	}
	if e.State != StateLoot {
		t.Errorf("state after chest = %v, want StateLoot", e.State)
	}
	if e.PendingLoot != item {
		t.Errorf("PendingLoot = %v, want the loot item held as pending", e.PendingLoot)
	}
	if e.Player.Weapon != nil && e.Player.Weapon.ID == "shiny_sword" {
		t.Error("chest item auto-equipped — must wait for the player decision")
	}
	if e.CanAscend {
		t.Error("chest must not unlock ascending (only victory does)")
	}
}

// CmdWear (StateLoot) wears the pending item; the displaced item goes to
// the bag when there is room. Afterwards the engine is back to exploring
// and nothing stays pending.
func TestLootWearDisplacedGoesToBag(t *testing.T) {
	e := chestEngine(1) // shiny_sword displaces the equipped rusty sword
	rusty := testBagItem("rusty_sword", "weapon")
	e.Player.EquipItem(rusty)
	e.Process(Command{Type: CmdExplore})
	e.Process(Command{Type: CmdOpenChest})
	pending := e.PendingLoot

	events := e.Process(Command{Type: CmdWear})

	if got := eventTypes(events); !typesEqual(got, EventItemEquipped) {
		t.Fatalf("events = %v, want [item-equipped]", got)
	}
	if item, _ := events[0].Data["item"].(*entities.Item); item != pending {
		t.Errorf("equipped = %v, want the pending loot", item)
	}
	if swapped, _ := events[0].Data["swapped_from"].(*entities.Item); swapped != rusty {
		t.Errorf("swapped_from = %v, want the displaced rusty sword", swapped)
	}
	if e.Player.Weapon != pending {
		t.Error("pending loot not equipped in the weapon slot")
	}
	bag := e.Player.Bag()
	if len(bag) != 1 || bag[0] != rusty {
		t.Errorf("bag = %v, want the displaced sword stashed", bag)
	}
	if e.State != StateExploring {
		t.Errorf("state after equip = %v, want StateExploring", e.State)
	}
	if e.PendingLoot != nil {
		t.Errorf("PendingLoot = %v, want nil after the decision", e.PendingLoot)
	}
}

// With no bag room left, wearing the loot destroys the displaced item.
func TestLootWearWithFullBagDestroysDisplaced(t *testing.T) {
	e := chestEngine(1) // shiny_sword displaces the equipped rusty sword
	rusty := testBagItem("rusty_sword", "weapon")
	e.Player.EquipItem(rusty)
	fillBag(e)
	e.Process(Command{Type: CmdExplore})
	e.Process(Command{Type: CmdOpenChest})
	pending := e.PendingLoot

	events := e.Process(Command{Type: CmdWear})

	if got := eventTypes(events); !typesEqual(got, EventItemEquipped, EventItemLost) {
		t.Fatalf("events = %v, want [item-equipped item-lost]", got)
	}
	if item, _ := events[1].Data["item"].(*entities.Item); item != rusty {
		t.Errorf("lost = %v, want the displaced rusty sword", item)
	}
	if e.Player.Weapon != pending {
		t.Error("pending loot not equipped in the weapon slot")
	}
	if got := len(e.Player.Bag()); got != entities.InventoryCapacity {
		t.Errorf("bag length = %d, want %d (loot equipped, bag still full)",
			got, entities.InventoryCapacity)
	}
	if e.State != StateExploring || e.PendingLoot != nil {
		t.Errorf("state = %v, pending = %v, want Exploring/nil", e.State, e.PendingLoot)
	}
}

// Wearing loot into an empty slot displaces nothing.
func TestLootWearIntoEmptySlot(t *testing.T) {
	e := chestEngine(0) // rusty_ring, hero has an empty ring slot
	e.Process(Command{Type: CmdExplore})
	e.Process(Command{Type: CmdOpenChest})

	events := e.Process(Command{Type: CmdWear})

	if got := eventTypes(events); !typesEqual(got, EventItemEquipped) {
		t.Fatalf("events = %v, want [item-equipped] only", got)
	}
	if _, ok := events[0].Data["swapped_from"]; ok {
		t.Error("swapped_from present with nothing displaced")
	}
	if e.Player.Ring == nil {
		t.Error("pending loot not equipped in the ring slot")
	}
	if e.State != StateExploring || e.PendingLoot != nil {
		t.Errorf("state = %v, pending = %v, want Exploring/nil", e.State, e.PendingLoot)
	}
}

// In StateLoot the pending item is the only wearer candidate: CmdWear
// ignores Index entirely (garbage indexes included) — the index only
// means something in StateInventory.
func TestLootWearIgnoresIndex(t *testing.T) {
	e := chestEngine(0) // rusty_ring, hero has an empty ring slot
	e.Process(Command{Type: CmdExplore})
	e.Process(Command{Type: CmdOpenChest})
	pending := e.PendingLoot

	events := e.Process(Command{Type: CmdWear, Index: 7})

	if got := eventTypes(events); !typesEqual(got, EventItemEquipped) {
		t.Fatalf("events = %v, want [item-equipped]", got)
	}
	if e.Player.Ring != pending {
		t.Error("pending loot not worn from StateLoot")
	}
	if e.State != StateExploring || e.PendingLoot != nil {
		t.Errorf("state = %v, pending = %v, want Exploring/nil", e.State, e.PendingLoot)
	}
}

// CmdStashLoot stores the pending item in the bag when there is room.
func TestLootStash(t *testing.T) {
	e := chestEngine(0)
	e.Process(Command{Type: CmdExplore})
	e.Process(Command{Type: CmdOpenChest})
	pending := e.PendingLoot

	events := e.Process(Command{Type: CmdStashLoot})

	if got := eventTypes(events); !typesEqual(got, EventItemStashed) {
		t.Fatalf("events = %v, want [item-stashed]", got)
	}
	if item, _ := events[0].Data["item"].(*entities.Item); item != pending {
		t.Errorf("stashed = %v, want the pending loot", item)
	}
	bag := e.Player.Bag()
	if len(bag) != 1 || bag[0] != pending {
		t.Errorf("bag = %v, want the loot stashed", bag)
	}
	if e.State != StateExploring || e.PendingLoot != nil {
		t.Errorf("state = %v, pending = %v, want Exploring/nil", e.State, e.PendingLoot)
	}
}

// A stash attempt with a full bag is rejected: EventBagFull, no state
// change (the decision stays open) and the item stays pending.
func TestLootStashWithFullBagIsRejected(t *testing.T) {
	e := chestEngine(0)
	fillBag(e)
	e.Process(Command{Type: CmdExplore})
	e.Process(Command{Type: CmdOpenChest})
	pending := e.PendingLoot

	events := e.Process(Command{Type: CmdStashLoot})

	if got := eventTypes(events); !typesEqual(got, EventBagFull) {
		t.Fatalf("events = %v, want [bag-full]", got)
	}
	if e.State != StateLoot {
		t.Errorf("state after rejected stash = %v, want StateLoot (decision stays open)", e.State)
	}
	if e.PendingLoot != pending {
		t.Errorf("PendingLoot = %v, want the loot kept pending", e.PendingLoot)
	}
	// The player can still discard the item to get out of the corner.
	events = e.Process(Command{Type: CmdDiscardLoot})
	if got := eventTypes(events); !typesEqual(got, EventItemDiscarded) {
		t.Fatalf("events after discard = %v, want [item-discarded]", got)
	}
	if e.State != StateExploring || e.PendingLoot != nil {
		t.Errorf("state = %v, pending = %v, want Exploring/nil", e.State, e.PendingLoot)
	}
}

// CmdDiscardLoot destroys the item for good: nothing equipped, nothing
// stashed.
func TestLootDiscard(t *testing.T) {
	e := chestEngine(0)
	e.Process(Command{Type: CmdExplore})
	e.Process(Command{Type: CmdOpenChest})
	pending := e.PendingLoot

	events := e.Process(Command{Type: CmdDiscardLoot})

	if got := eventTypes(events); !typesEqual(got, EventItemDiscarded) {
		t.Fatalf("events = %v, want [item-discarded]", got)
	}
	if item, _ := events[0].Data["item"].(*entities.Item); item != pending {
		t.Errorf("discarded = %v, want the pending loot", item)
	}
	if e.Player.Weapon != nil || e.Player.Ring != nil {
		t.Error("discarded loot ended up equipped")
	}
	if got := len(e.Player.Bag()); got != 0 {
		t.Errorf("bag length = %d, want 0", got)
	}
	if e.State != StateExploring || e.PendingLoot != nil {
		t.Errorf("state = %v, pending = %v, want Exploring/nil", e.State, e.PendingLoot)
	}
}
