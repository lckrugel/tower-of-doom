package engine

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// merchantSection is the mixed encounter table extended with an equal
// merchant weight: with the roll order [combat, sanctuary, chest,
// merchant] over total 40, the merchant rolls at rng 30. The stock is
// tier-gated to 1 so the base pool is exactly the four t1 items.
func merchantSection() game.SectionAsset {
	s := mixedSection()
	s.EncounterTypes.Merchant = 10
	s.ItemTiers = []int{1}
	return s
}

// merchantItems offers a four-item tier-1 pool (sorted [t1_helm, t1_mail,
// t1_ring, t1_sword]) and one tier-2 piece as the upgrade target.
func merchantItems() map[string]game.ItemAsset {
	return map[string]game.ItemAsset{
		"t1_helm":  {ID: "t1_helm", Name: "T1 Helm", Slot: "helmet", BonusDEF: 2, Tier: 1},
		"t1_mail":  {ID: "t1_mail", Name: "T1 Mail", Slot: "body_armor", BonusDEF: 3, Tier: 1},
		"t1_ring":  {ID: "t1_ring", Name: "T1 Ring", Slot: "ring", BonusDEF: 1, Tier: 1},
		"t1_sword": {ID: "t1_sword", Name: "T1 Sword", Slot: "weapon", BonusDMG: 5, Tier: 1},
		"t2_crown": {ID: "t2_crown", Name: "T2 Crown", Slot: "helmet", BonusDEF: 9, Tier: 2},
	}
}

// merchantEngine is an engine over the merchant section with
// pre-programmed rng values (30 → merchant). The run's forced first
// combat (pacing rule) is already spent, so EXPLORE rolls the kind table
// as usual.
func merchantEngine(rngValues ...int) *Engine {
	e := newTestEngine(rngValues...)
	e.reg.Sections = []game.SectionAsset{merchantSection()}
	e.reg.Items = merchantItems()
	return pastFirstExplore(e)
}

// stockOffers is the rng for finding a merchant with an all-tier-1 stock
// of 4 distinct offers: the picks drain the sorted tier-1 pool
// [t1_helm, t1_mail, t1_ring, t1_sword] as [t1_sword, t1_helm, t1_mail,
// t1_ring] (upgrade roll 99 → none).
var stockOffers = []int{30, 3, 0, 0, 0, 99}

// EXPLORE into a merchant opens the shop: StateMerchant with exactly 4
// DISTINCT offers, reported by EventMerchantFound. ASCEND stays locked.
func TestExploreMerchantOpensShop(t *testing.T) {
	e := merchantEngine(stockOffers...)

	events := e.Process(Command{Type: CmdExplore})

	if got := eventTypes(events); !typesEqual(got, EventMerchantFound) {
		t.Fatalf("events = %v, want [merchant-found]", got)
	}
	if count, _ := events[0].Data["count"].(int); count != 4 {
		t.Errorf("count = %v, want 4", count)
	}
	if len(e.MerchantStock) != 4 {
		t.Fatalf("stock size = %d, want 4", len(e.MerchantStock))
	}
	seen := map[string]bool{}
	for i, item := range e.MerchantStock {
		if item == nil || item.Tier != 1 {
			t.Errorf("stock[%d] = %v, want a tier-1 offer (upgrade roll was 99)", i, item)
		}
		if seen[item.ID] {
			t.Errorf("stock[%d] = %q duplicates an earlier offer", i, item.ID)
		}
		seen[item.ID] = true
	}
	if e.State != StateMerchant {
		t.Errorf("state = %v, want StateMerchant", e.State)
	}
	if e.CanAscend {
		t.Error("merchant must not unlock ascending (only victory does)")
	}
	if e.CurrentEncounter.Type != EncounterCombat {
		t.Errorf("current encounter = %v, want zeroed", e.CurrentEncounter.Type)
	}
}

// The 35% tier-above upgrade replaces exactly one offer — or none.
func TestMerchantStockUpgrade(t *testing.T) {
	t.Run("no upgrade on a roll of 35", func(t *testing.T) {
		e := merchantEngine(30, 3, 0, 0, 0, 35) // 35 is not < 35

		e.Process(Command{Type: CmdExplore})

		for i, item := range e.MerchantStock {
			if item.Tier != 1 {
				t.Errorf("stock[%d] = %q (tier %d), want tier 1", i, item.ID, item.Tier)
			}
		}
	})

	t.Run("upgrade replaces the picked offer with a tier+1 item", func(t *testing.T) {
		// rng: 30 merchant; offers drain the pool as [t1_sword, t1_helm,
		// t1_mail, t1_ring]; upgrade 34 < 35; offer 2; t2 pool pick 0.
		e := merchantEngine(30, 3, 0, 0, 0, 34, 2, 0)

		e.Process(Command{Type: CmdExplore})

		want := []string{"t1_sword", "t1_helm", "t2_crown", "t1_ring"}
		for i, id := range want {
			if got := e.MerchantStock[i].ID; got != id {
				t.Errorf("stock[%d] = %q, want %q", i, got, id)
			}
		}
	})

	t.Run("the replacement never duplicates an offer already in stock", func(t *testing.T) {
		// Unrestricted section, registry with two tier-2 pieces: the
		// stock picks t2_crown first, then three t1 pieces. The upgrade
		// targets stock[1]; the tier-2 replacement pool is [t2_crown,
		// t2_mail], but t2_crown is ALREADY in stock — the filtered pool
		// is [t2_mail] alone (pick 0 would hit the duplicate without the
		// filter).
		e := merchantEngine(30, 4, 0, 0, 0, 34, 1, 0, 0)
		e.reg.Items["t2_mail"] = game.ItemAsset{ID: "t2_mail", Name: "T2 Mail", Slot: "body_armor", BonusDEF: 6, Tier: 2}
		s := merchantSection()
		s.ItemTiers = nil // unrestricted: the base pool holds t1 AND t2 items
		e.reg.Sections = []game.SectionAsset{s}

		e.Process(Command{Type: CmdExplore})

		want := []string{"t2_crown", "t2_mail", "t1_mail", "t1_ring"}
		for i, id := range want {
			if got := e.MerchantStock[i].ID; got != id {
				t.Errorf("stock[%d] = %q, want %q", i, got, id)
			}
		}
	})

	t.Run("the upgrade is capped at the registry max tier and keeps the offer when the filtered pool is empty", func(t *testing.T) {
		// Only tier-2 items are allowed: the "tier+1" target is capped at
		// the registry max (2). The two-item pool drains into 2 offers
		// (pool exhaustion), the upgrade fires on stock[0], but every
		// tier-2 item is already in stock — the filtered replacement pool
		// is empty, so the original offer stays (and consumes no rng).
		e := merchantEngine(30, 0, 0, 0, 0)
		e.reg.Items = map[string]game.ItemAsset{
			"t2_crown": {ID: "t2_crown", Name: "T2 Crown", Slot: "helmet", BonusDEF: 9, Tier: 2},
			"t2_mail":  {ID: "t2_mail", Name: "T2 Mail", Slot: "body_armor", BonusDEF: 6, Tier: 2},
		}
		s := mixedSection()
		s.ItemTiers = []int{2}
		s.EncounterTypes.Merchant = 10
		e.reg.Sections = []game.SectionAsset{s}

		e.Process(Command{Type: CmdExplore})

		want := []string{"t2_crown", "t2_mail"}
		if len(e.MerchantStock) != len(want) {
			t.Fatalf("stock size = %d, want %d (the 2-item pool ran dry)", len(e.MerchantStock), len(want))
		}
		for i, id := range want {
			if got := e.MerchantStock[i].ID; got != id {
				t.Errorf("stock[%d] = %q, want %q (upgrade kept the original)", i, got, id)
			}
		}
	})
}

// A pool smaller than the stock yields FEWER offers: the drain stops when
// the pool runs dry, consuming no extra rng, and the upgrade check still
// runs over whatever stock exists.
func TestMerchantStockPoolExhaustionYieldsFewerOffers(t *testing.T) {
	e := merchantEngine(30, 0, 0, 99)
	e.reg.Items = map[string]game.ItemAsset{
		"t1_ring":  {ID: "t1_ring", Name: "T1 Ring", Slot: "ring", BonusDEF: 1, Tier: 1},
		"t1_sword": {ID: "t1_sword", Name: "T1 Sword", Slot: "weapon", BonusDMG: 5, Tier: 1},
	}
	stub := &stubRng{values: e.rng.(*stubRng).values}
	e.rng = stub

	events := e.Process(Command{Type: CmdExplore})

	if got := eventTypes(events); !typesEqual(got, EventMerchantFound) {
		t.Fatalf("events = %v, want [merchant-found]", got)
	}
	if count, _ := events[0].Data["count"].(int); count != 2 {
		t.Errorf("count = %v, want 2 (pool had only 2 items)", count)
	}
	if len(e.MerchantStock) != 2 {
		t.Fatalf("stock size = %d, want 2", len(e.MerchantStock))
	}
	ids := map[string]bool{e.MerchantStock[0].ID: true, e.MerchantStock[1].ID: true}
	if len(ids) != 2 {
		t.Errorf("stock = %v, want 2 distinct offers", ids)
	}
	// kind roll + 2 offers + upgrade check: the exhausted pool consumed
	// no extra rng.
	if stub.calls != 4 {
		t.Errorf("rng calls = %d, want 4 (kind + 2 offers + upgrade check)", stub.calls)
	}
}

// A zero/absent merchant weight keeps every existing rng sequence: no
// merchant bucket in the kind roll and no extra roll on any path.
func TestZeroMerchantWeightNeverSpawnsMerchant(t *testing.T) {
	t.Run("the kind roll buckets are unchanged", func(t *testing.T) {
		e := newKindEngine(0)
		stub := &stubRng{values: []int{0}}
		e.rng = stub

		weights := game.EncounterTypeWeights{Combat: 10, Sanctuary: 10, Chest: 10}
		if kind := e.rollEncounterType(weights); kind != EncounterCombat {
			t.Errorf("rng 0 rolled %v, want combat (bucket order unchanged)", kind)
		}
		if stub.calls != 1 {
			t.Errorf("kind roll consumed %d rng values, want 1", stub.calls)
		}
	})

	t.Run("exploring a merchantless section consumes no merchant rng", func(t *testing.T) {
		// rng: 20 → chest, 50 ≥ 40 → item, 0 → pick. Nothing else.
		e := chestEngine(0)
		stub := &stubRng{values: []int{20, 50, 0}}
		e.rng = stub

		events := e.Process(Command{Type: CmdExplore})

		if got := eventTypes(events); !typesEqual(got, EventLootFound) {
			t.Fatalf("events = %v, want [loot-found]", got)
		}
		if stub.calls != 3 {
			t.Errorf("rng calls = %d, want 3 (kind, gold check, pick — no merchant roll)", stub.calls)
		}
	})
}

// CmdBuy moves gold to the price and the item to the hero bag, removing
// the offer from the stock. Shopping continues (state stays).
func TestMerchantBuy(t *testing.T) {
	setup := func() *Engine {
		e := merchantEngine(stockOffers...)
		e.Player.Gold = 1000
		e.Process(Command{Type: CmdExplore})
		return e
	}

	t.Run("success buys into the bag at the BuyPrice", func(t *testing.T) {
		e := setup()
		offer := e.MerchantStock[0] // t1_sword

		events := e.Process(Command{Type: CmdBuy, ItemIndex: 0})

		if got := eventTypes(events); !typesEqual(got, EventItemBought) {
			t.Fatalf("events = %v, want [item-bought]", got)
		}
		if item, _ := events[0].Data["item"].(*entities.Item); item != offer {
			t.Errorf("bought = %v, want the stock offer", item)
		}
		if price, _ := events[0].Data["price"].(int); price != 250 {
			t.Errorf("price = %v, want 250 (t1_sword BuyPrice)", price)
		}
		if got := e.Player.Gold; got != 750 {
			t.Errorf("gold = %d, want 750", got)
		}
		bag := e.Player.Bag()
		if len(bag) != 1 || bag[0] != offer {
			t.Errorf("bag = %v, want the bought item", bag)
		}
		if got := len(e.MerchantStock); got != 3 {
			t.Errorf("stock size = %d, want 3 (offer removed)", got)
		}
		if e.State != StateMerchant {
			t.Errorf("state = %v, want StateMerchant (shopping continues)", e.State)
		}
	})

	t.Run("not enough gold is rejected with no change", func(t *testing.T) {
		e := setup()
		e.Player.Gold = 100 // t1_sword costs 250

		events := e.Process(Command{Type: CmdBuy, ItemIndex: 0})

		if got := eventTypes(events); !typesEqual(got, EventMessage) {
			t.Fatalf("events = %v, want [message (not enough gold)]", got)
		}
		if e.Player.Gold != 100 || len(e.Player.Bag()) != 0 || len(e.MerchantStock) != 4 {
			t.Error("a rejected buy changed gold, bag or stock")
		}
		if e.State != StateMerchant {
			t.Errorf("state = %v, want StateMerchant", e.State)
		}
	})

	t.Run("a full bag is rejected with no change", func(t *testing.T) {
		e := setup()
		fillBag(e)

		events := e.Process(Command{Type: CmdBuy, ItemIndex: 0})

		if got := eventTypes(events); !typesEqual(got, EventBagFull) {
			t.Fatalf("events = %v, want [bag-full]", got)
		}
		if e.Player.Gold != 1000 || len(e.MerchantStock) != 4 {
			t.Error("a rejected buy changed gold or stock")
		}
		if e.State != StateMerchant {
			t.Errorf("state = %v, want StateMerchant", e.State)
		}
	})
}

// CmdSell pays the SellValue into the purse and destroys the bag item.
func TestMerchantSell(t *testing.T) {
	e := merchantEngine(stockOffers...)
	e.Process(Command{Type: CmdExplore})
	heirloom := entities.NewItemFromAsset(game.ItemAsset{
		ID: "heirloom", Name: "Heirloom", Slot: "ring", BonusDEF: 2, Tier: 1,
	})
	e.Player.StashItem(&heirloom)

	events := e.Process(Command{Type: CmdSell, ItemIndex: 0})

	if got := eventTypes(events); !typesEqual(got, EventItemSold) {
		t.Fatalf("events = %v, want [item-sold]", got)
	}
	if item, _ := events[0].Data["item"].(*entities.Item); item != &heirloom {
		t.Errorf("sold = %v, want the bag item", item)
	}
	if price, _ := events[0].Data["price"].(int); price != 50 {
		t.Errorf("price = %v, want 50 (SellValue of a DEF 2 item)", price)
	}
	if got := e.Player.Gold; got != 50 {
		t.Errorf("gold = %d, want 50", got)
	}
	if got := len(e.Player.Bag()); got != 0 {
		t.Errorf("bag length = %d, want 0 (sold item destroyed)", got)
	}
	if e.State != StateMerchant {
		t.Errorf("state = %v, want StateMerchant (shopping continues)", e.State)
	}
}

// CmdLeave discards the stock and returns to exploring.
func TestMerchantLeave(t *testing.T) {
	e := merchantEngine(stockOffers...)
	e.Process(Command{Type: CmdExplore})

	events := e.Process(Command{Type: CmdLeave})

	if got := eventTypes(events); len(got) != 1 || got[0] != EventMessage {
		t.Errorf("events = %v, want a single message", got)
	}
	if e.MerchantStock != nil {
		t.Errorf("stock = %v, want discarded", e.MerchantStock)
	}
	if e.State != StateExploring {
		t.Errorf("state = %v, want StateExploring", e.State)
	}
}

// Merchant commands are only valid in StateMerchant, and their indexes
// must point at a real offer/bag slot — otherwise the generic invalid
// command, never a panic.
func TestMerchantCommandGuards(t *testing.T) {
	t.Run("rejected outside StateMerchant", func(t *testing.T) {
		states := []GameState{StateExploring, StateEncounter, StateCombat, StateLoot, StateInventory, StateGameOver}
		for _, state := range states {
			for _, cmdType := range []CommandType{CmdBuy, CmdSell, CmdLeave} {
				e := newTestEngine()
				e.State = state

				events := e.Process(Command{Type: cmdType, ItemIndex: 0})

				if got := eventTypes(events); len(got) != 1 || got[0] != EventMessage {
					t.Errorf("%v in %v: events = %v, want a single [invalid command] message", cmdType, state, got)
				}
				if e.State != state {
					t.Errorf("%v in %v: state changed to %v", cmdType, state, e.State)
				}
			}
		}
	})

	t.Run("out-of-range indexes are invalid, never a panic", func(t *testing.T) {
		e := merchantEngine(stockOffers...)
		e.Process(Command{Type: CmdExplore})

		for _, index := range []int{-1, 4, 100} {
			if got := eventTypes(e.Process(Command{Type: CmdBuy, ItemIndex: index})); len(got) != 1 || got[0] != EventMessage {
				t.Errorf("CmdBuy(%d): events = %v, want a single [invalid command] message", index, got)
			}
		}
		for _, index := range []int{-1, 0, 100} { // 0: the bag is empty
			if got := eventTypes(e.Process(Command{Type: CmdSell, ItemIndex: index})); len(got) != 1 || got[0] != EventMessage {
				t.Errorf("CmdSell(%d): events = %v, want a single [invalid command] message", index, got)
			}
		}
		if e.State != StateMerchant || len(e.MerchantStock) != 4 {
			t.Error("invalid indexes changed the shop")
		}
	})
}
