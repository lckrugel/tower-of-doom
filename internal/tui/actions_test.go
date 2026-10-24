package tui

import (
	"slices"
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

func testEngine() *engine.Engine {
	return engine.NewEngine(&game.Registry{
		Monsters: map[string]game.MonsterAsset{
			"goblin": {ID: "goblin", Name: "Goblin", Difficulty: 3, HP: 30, Damage: 2, Speed: 10, Crit: 5},
		},
		Items: map[string]game.ItemAsset{},
		Sections: []game.SectionAsset{{
			ID: "pits", Name: "Goblin Pits", MinFloor: 1, MaxFloor: -1,
			Spawns: []game.SpawnEntry{{MonsterID: "goblin", Weight: 1}},
		}},
	})
}

func keysOf(actions []action) []string {
	keys := make([]string, 0, len(actions))
	for _, a := range actions {
		keys = append(keys, a.key)
	}
	return keys
}

func keysEqual(got []string, want ...string) bool {
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

// stashTestItem puts a fresh item directly into the hero's bag, so state
// fixtures can render the bag actions enabled.
func stashTestItem(e *engine.Engine) *entities.Item {
	item := entities.NewItemFromAsset(game.ItemAsset{
		ID: "spare_charm", Name: "Spare Charm", Slot: "ring", BonusDEF: 1, Durability: 10,
	})
	e.Player.StashItem(&item)
	return &item
}

func TestActionsPerState(t *testing.T) {
	tests := []struct {
		name      string
		state     engine.GameState
		withBag   bool
		withStock bool // the shop's offers (BUY needs one to be enabled)
		wantKeys  []string
	}{
		{"exploring", engine.StateExploring, false, false, []string{"e", "u", "i", "q"}},
		{"encounter", engine.StateEncounter, false, false, []string{"f", "s", "q"}},
		{"combat", engine.StateCombat, false, false, []string{"a", "r", "q"}},
		{"loot", engine.StateLoot, false, false, []string{"g", "b", "d", "q"}},
		{"inventory", engine.StateInventory, true, false, []string{"w", "x", "c", "q"}},
		{"merchant", engine.StateMerchant, true, true, []string{"p", "v", "l", "q"}},
		{"game over", engine.StateGameOver, false, false, []string{"q"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := testEngine()
			if tt.withBag {
				// An empty bag renders WEAR/DROP disabled; the enabled
				// invariant below needs a stocked bag.
				stashTestItem(e)
			}
			if tt.withStock {
				// SELL needs the bag (stashed above); BUY needs offers.
				e.MerchantStock = append(e.MerchantStock,
					shopItem("Offer A"), shopItem("Offer B"), shopItem("Offer C"))
			}
			e.State = tt.state

			actions := actionsFor(e)

			if !keysEqual(keysOf(actions), tt.wantKeys...) {
				t.Errorf("keys = %v, want %v", keysOf(actions), tt.wantKeys)
			}
			for _, a := range actions {
				if a.cmd != engine.CmdAscend && !a.enabled {
					t.Errorf("action %q is unexpectedly disabled", a.key)
				}
			}
		})
	}
}

// An empty bag can't wear or drop anything: both actions render disabled.
// Dispatching them anyway still reaches the engine, which explains the
// rejection as a log message.
func TestBagActionsAreDisabledOnAnEmptyBag(t *testing.T) {
	e := testEngine()
	e.State = engine.StateInventory

	for _, a := range actionsFor(e) {
		switch a.cmd {
		case engine.CmdEquip, engine.CmdDrop:
			if a.enabled {
				t.Errorf("action %q enabled with an empty bag", a.key)
			}
		default:
			if !a.enabled {
				t.Errorf("action %q unexpectedly disabled", a.key)
			}
		}
	}
}

// No shortcut may change meaning across states: combat muscle memory
// (spamming attack) must never leak into a different command.
func TestShortcutsAreGloballyUnique(t *testing.T) {
	byKey := make(map[string]engine.CommandType)
	states := []engine.GameState{
		engine.StateExploring, engine.StateEncounter, engine.StateCombat, engine.StateGameOver,
		engine.StateLoot, engine.StateInventory, engine.StateMerchant,
	}
	for _, state := range states {
		e := testEngine()
		e.State = state
		for _, a := range actionsFor(e) {
			if prev, dup := byKey[a.key]; dup && prev != a.cmd {
				t.Errorf("state %v: key %q maps to both %v and %v", state, a.key, prev, a.cmd)
			}
			byKey[a.key] = a.cmd
		}
	}
}

// Ascending is unlocked by combat victories only; before that the action
// renders disabled. Dispatching it anyway still asks the engine, which
// explains the block in the event log.
func TestAscendEnabledFollowsCanAscend(t *testing.T) {
	e := testEngine()
	e.State = engine.StateExploring

	locked := actionsFor(e)[slices.IndexFunc(actionsFor(e), func(a action) bool { return a.cmd == engine.CmdAscend })]
	if locked.enabled {
		t.Error("ascend enabled without a floor clear")
	}

	e.CanAscend = true
	unlocked := actionsFor(e)[slices.IndexFunc(actionsFor(e), func(a action) bool { return a.cmd == engine.CmdAscend })]
	if !unlocked.enabled {
		t.Error("ascend still disabled after a victory")
	}
}

// Only the row-indexed commands dispatch with the cursor: the bag
// commands (WEAR/DROP) index the hero bag; at the merchant, BUY indexes
// the stock and SELL the bag. Every other action never sends an index.
func TestIndexedCommandsUseTheRowCursor(t *testing.T) {
	states := []engine.GameState{
		engine.StateExploring, engine.StateEncounter, engine.StateCombat,
		engine.StateGameOver, engine.StateLoot, engine.StateInventory,
		engine.StateMerchant,
	}
	for _, state := range states {
		e := testEngine()
		e.State = state
		for _, a := range actionsFor(e) {
			want := a.cmd == engine.CmdEquip || a.cmd == engine.CmdDrop ||
				a.cmd == engine.CmdBuy || a.cmd == engine.CmdSell
			if a.usesIndex != want {
				t.Errorf("state %v: action %q usesIndex = %v, want %v", state, a.key, a.usesIndex, want)
			}
		}
	}
}
