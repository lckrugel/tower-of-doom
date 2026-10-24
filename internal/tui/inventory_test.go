package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// chestEngine builds an engine whose only encounter type is a chest: an
// all-zero combat/sanctuary table rolls CHEST deterministically (see the
// engine's rollEncounterType), so a post-opener EXPLORE opens the loot
// decision without any rng seeding. BaseDifficulty gives the forced
// opening combat (encounter pacing: a run always opens with combat)
// something to spawn.
func chestEngine() *engine.Engine {
	return engine.NewEngine(&game.Registry{
		Monsters: map[string]game.MonsterAsset{
			"goblin": {ID: "goblin", Name: "Goblin", Difficulty: 3, HP: 30, Damage: 2, Speed: 10, Crit: 5},
		},
		Items: map[string]game.ItemAsset{
			"wooden_sword": {ID: "wooden_sword", Name: "Espada de Madeira", Slot: "weapon", BonusDMG: 5, Durability: 50},
			"iron_ring":    {ID: "iron_ring", Name: "Iron Ring", Slot: "ring", BonusDEF: 2, Durability: 40},
		},
		Sections: []game.SectionAsset{{
			ID:             "vault",
			Name:           "Silent Vault",
			BaseDifficulty: 10,
			MinFloor:       1,
			MaxFloor:       -1,
			Spawns:         []game.SpawnEntry{{MonsterID: "goblin", Weight: 1}},
			EncounterTypes: game.EncounterTypeWeights{Combat: 0, Sanctuary: 0, Chest: 1},
		}},
	})
}

// fightThroughTheOpener resolves a forced combat (the encounter-pacing
// rules: a run always opens with combat, and every third same-kind
// non-combat roll is overridden to combat) by fighting it out. The
// fixture hero one-shots the opener's goblins, so the fight is short;
// the iteration bound keeps a freak seed from hanging the suite instead
// of failing it.
func fightThroughTheOpener(t *testing.T, m Model) Model {
	t.Helper()
	m.engine.Player.BaseDMG = 50 // one-shot the opener's goblins
	for i := 0; i < 100; i++ {
		switch m.engine.State {
		case engine.StateEncounter:
			m, _ = press(m, "f")
		case engine.StateCombat:
			m, _ = press(m, "a")
		case engine.StateExploring:
			return m
		default:
			t.Fatalf("setup: unexpected state %v during the opener", m.engine.State)
		}
	}
	t.Fatal("setup: the forced opener combat never ended")
	return m
}

// exploreUntilChest presses EXPLORE until a chest yields a pending
// item. Chests may pay out gold instead (engine economy, 40%), and the
// no-triple-streak override periodically forces a combat in a
// chest-only section — both are fought through; the bound keeps a freak
// losing streak from hanging the suite instead of failing it.
func exploreUntilChest(t *testing.T, m Model) Model {
	t.Helper()
	for i := 0; i < 100; i++ {
		switch m.engine.State {
		case engine.StateExploring:
			m, _ = press(m, "e")
		case engine.StateEncounter:
			m, _ = press(m, "f")
		case engine.StateCombat:
			m, _ = press(m, "a")
		case engine.StateLoot:
			return m
		default:
			t.Fatalf("setup: unexpected state %v while hunting a chest", m.engine.State)
		}
	}
	t.Fatal("setup: no chest yielded an item")
	return m
}

// chestModel plays the forced opener, then explores until the model
// lands in StateLoot with the chest's pending item — all through the
// real key path.
func chestModel(t *testing.T) Model {
	t.Helper()
	m := New(chestEngine())
	m = fightThroughTheOpener(t, m)
	return exploreUntilChest(t, m)
}

// bagModel opens the bag through the real key path with n stashed items.
func bagModel(n int) Model {
	e := testEngine()
	for range n {
		stashTestItem(e)
	}
	m := New(e)
	m, _ = press(m, "i")
	return m
}

// stashNamedItem puts a distinguishable item into the hero's bag and
// returns its pointer, so tests can follow it through the bag's slots.
func stashNamedItem(e *engine.Engine, name string) *entities.Item {
	item := entities.NewItemFromAsset(game.ItemAsset{
		ID: "spare_charm", Name: name, Slot: "ring", BonusDEF: 1, Durability: 10,
	})
	e.Player.StashItem(&item)
	return &item
}

// isWorn reports whether the hero currently wears the item.
func isWorn(h *entities.Hero, item *entities.Item) bool {
	worn := []*entities.Item{h.Head, h.Body, h.Feet, h.Weapon, h.Ring, h.Amulet}
	return slices.Contains(worn, item)
}

// Exploring on a chest opens the loot decision: the item is held as
// pending, nothing equipped or stored yet.
func TestExploreOnAChestOpensTheLootDecision(t *testing.T) {
	m := chestModel(t)

	if m.engine.State != engine.StateLoot {
		t.Fatalf("state after explore = %v, want StateLoot", m.engine.State)
	}
	if m.engine.PendingLoot == nil {
		t.Fatal("the chest left no pending loot")
	}
}

// WEAR (loot-state key g until the bar is rekeyed) equips the pending
// loot and returns to exploring.
func TestWearEquipsThePendingLoot(t *testing.T) {
	m := chestModel(t)
	item := m.engine.PendingLoot

	m, _ = press(m, "g")

	if m.engine.State != engine.StateExploring {
		t.Errorf("state after wear = %v, want StateExploring", m.engine.State)
	}
	if m.engine.PendingLoot != nil {
		t.Error("wear left the loot pending")
	}
	if !isWorn(m.engine.Player, item) {
		t.Errorf("the pending item %q is not worn after 'g'", item.Name)
	}
}

// BAG stashes the pending loot and returns to exploring.
func TestBagActionStashesThePendingLoot(t *testing.T) {
	m := chestModel(t)
	item := m.engine.PendingLoot

	m, _ = press(m, "b")

	if m.engine.State != engine.StateExploring {
		t.Errorf("state after 'b' = %v, want StateExploring", m.engine.State)
	}
	if m.engine.PendingLoot != nil {
		t.Error("'b' left the loot pending")
	}
	bag := m.engine.Player.Bag()
	if len(bag) != 1 || bag[0] != item {
		t.Errorf("bag after 'b' holds %d items, want the pending one", len(bag))
	}
}

// DISCARD destroys the pending loot and returns to exploring.
func TestDiscardDestroysThePendingLoot(t *testing.T) {
	m := chestModel(t)
	item := m.engine.PendingLoot

	m, _ = press(m, "d")

	if m.engine.State != engine.StateExploring {
		t.Errorf("state after 'd' = %v, want StateExploring", m.engine.State)
	}
	if m.engine.PendingLoot != nil {
		t.Error("'d' left the loot pending")
	}
	if len(m.engine.Player.Bag()) != 0 {
		t.Error("'d' stashed the loot instead of destroying it")
	}
	if isWorn(m.engine.Player, item) {
		t.Error("'d' equipped the discarded loot")
	}
}

// A full bag can't take the loot: the stash is rejected, the decision
// stays open and the engine explains itself in the log.
func TestBagFullStashKeepsTheDecisionOpen(t *testing.T) {
	e := chestEngine()
	for range entities.InventoryCapacity {
		stashNamedItem(e, "Junk Trinket")
	}
	if len(e.Player.Bag()) != entities.InventoryCapacity {
		t.Fatal("setup: the bag is not at capacity")
	}
	m := New(e)
	m = fightThroughTheOpener(t, m)
	m = exploreUntilChest(t, m)
	if m.engine.State != engine.StateLoot {
		t.Fatal("setup: the chest did not open")
	}
	pending := m.engine.PendingLoot

	m, _ = press(m, "b")

	if m.engine.State != engine.StateLoot {
		t.Errorf("state after a rejected stash = %v, want to stay in StateLoot", m.engine.State)
	}
	if m.engine.PendingLoot != pending {
		t.Error("a rejected stash must keep the pending item")
	}
}

// The bag screen lists every stashed item, the capacity counter and the
// bag actions.
func TestViewInventoryListsTheBag(t *testing.T) {
	m := bagModel(3)

	if m.engine.State != engine.StateInventory {
		t.Fatalf("state after 'i' = %v, want StateInventory", m.engine.State)
	}
	view := stripControls(stripANSI(m.View()))
	for _, want := range []string{
		"◈ BAG (3/10)",
		"Spare Charm",
		"[G] Wear", "[X] Drop", "[C] Close", "[Q] Quit",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("bag view missing %q:\n%s", want, view)
		}
	}
}

// The selected row is highlighted with ▶; ←/→ wrap around the bag.
func TestViewInventoryHighlightsTheSelectedRow(t *testing.T) {
	e := testEngine()
	stashNamedItem(e, "Charm One")
	stashNamedItem(e, "Charm Two")
	m := New(e)
	m, _ = press(m, "i")

	m, _ = press(m, "right") // selects bag[1]
	view := stripControls(stripANSI(m.View()))
	if !strings.Contains(view, "▶ Charm Two") {
		t.Errorf("view must highlight the selected row:\n%s", view)
	}
	if strings.Contains(view, "▶ Charm One") {
		t.Errorf("view must not highlight unselected rows:\n%s", view)
	}

	m, _ = press(m, "right") // wraps back to bag[0]
	view = stripControls(stripANSI(m.View()))
	if !strings.Contains(view, "▶ Charm One") {
		t.Errorf("selection must wrap around the bag:\n%s", view)
	}
}

// DROP destroys the selected row: the item is gone for good.
func TestDropDestroysTheSelectedBagItem(t *testing.T) {
	e := testEngine()
	first := stashNamedItem(e, "Charm One")
	second := stashNamedItem(e, "Charm Two")
	third := stashNamedItem(e, "Charm Three")
	m := New(e)

	m, _ = press(m, "i")
	m, _ = press(m, "right")
	m, _ = press(m, "right") // selects bag[2]: Charm Three
	m, _ = press(m, "x")

	if m.engine.State != engine.StateInventory {
		t.Fatalf("state after 'x' = %v, want the bag to stay open", m.engine.State)
	}
	bag := m.engine.Player.Bag()
	if len(bag) != 2 {
		t.Fatalf("bag holds %d items after 'x', want 2", len(bag))
	}
	if bag[0] != first || bag[1] != second {
		t.Errorf("bag after drop = [%q, %q], want the survivors", bag[0].Name, bag[1].Name)
	}
	if isWorn(m.engine.Player, third) {
		t.Error("the dropped item is still worn")
	}
}

// WEAR from the open bag equips the highlighted row.
func TestWearFromTheOpenBagEquipsTheSelection(t *testing.T) {
	e := testEngine()
	starter := entities.NewItemFromAsset(game.ItemAsset{
		ID: "rusty_blade", Name: "Rusty Blade", Slot: "weapon", BonusDMG: 1, Durability: 10,
	})
	e.Player.EquipItem(&starter)
	stashNamedItem(e, "Charm One")
	blade := stashNamedItem(e, "Hero's Blade")

	m := New(e)
	m, _ = press(m, "i")
	m, _ = press(m, "right") // selects bag[1]: Hero's Blade
	m, _ = press(m, "g")

	if m.engine.State != engine.StateInventory {
		t.Fatalf("state after wear = %v, want the bag to stay open", m.engine.State)
	}
	if !isWorn(m.engine.Player, blade) {
		t.Errorf("weapon = %q, want the selected Hero's Blade", m.engine.Player.Weapon.Name)
	}
}

// CLOSE returns to exploring.
func TestCloseReturnsToExploring(t *testing.T) {
	m := bagModel(1)

	m, _ = press(m, "c")

	if m.engine.State != engine.StateExploring {
		t.Errorf("state after 'c' = %v, want StateExploring", m.engine.State)
	}
}

// The chest screen names the pending item, its bonuses, the fate hint
// and the decision actions — and every line fits the column.
func TestViewLootPanel(t *testing.T) {
	m := chestModel(t)

	view := stripControls(stripANSI(m.View()))
	for _, want := range []string{
		"◈ CHEST LOOT",
		m.engine.PendingLoot.Name,
		"decide — wear it, bag it, or let it go.",
		"[G] Wear", "[B] Bag", "[D] Discard",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("loot view missing %q:\n%s", want, view)
		}
	}
	for i, line := range viewLines(m.lootPanel()) {
		if w := ansiLen(line); w > mainColWidth {
			t.Errorf("loot body line %d is %d wide, overflows the %d-col column:\n%q", i, w, mainColWidth, line)
		}
	}
}

// A nil pending loot (defensive: the engine always holds one while in
// StateLoot) renders the guard line instead of panicking.
func TestViewLootPanelWithNoPendingItem(t *testing.T) {
	e := chestEngine()
	e.State = engine.StateLoot
	m := New(e)

	view := stripControls(stripANSI(m.View()))

	if !strings.Contains(view, "(the chest is empty)") {
		t.Errorf("loot view missing the empty-chest guard:\n%s", view)
	}
}

// An empty bag renders its guard line instead of rows.
func TestViewEmptyBag(t *testing.T) {
	m := bagModel(0)

	view := stripControls(stripANSI(m.View()))

	if !strings.Contains(view, "(the bag is empty)") {
		t.Errorf("empty bag view missing the guard line:\n%s", view)
	}
}

// The bag rows live in the left column: no line may lean on slot() to
// be clipped.
func TestViewInventoryRowsFitTheColumn(t *testing.T) {
	m := bagModel(entities.InventoryCapacity)

	for i, line := range viewLines(m.inventoryPanel()) {
		if w := ansiLen(line); w > mainColWidth {
			t.Errorf("bag body line %d is %d wide, overflows the %d-col column:\n%q", i, w, mainColWidth, line)
		}
	}
}
