package engine

import (
	"fmt"
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// The full state × command matrix: each state allows exactly its decision
// commands plus CmdQuit.
func TestAllowedCommandsMatrix(t *testing.T) {
	tests := []struct {
		name  string
		state GameState
		want  []CommandType
	}{
		{"exploring", StateExploring, []CommandType{CmdExplore, CmdAscend, CmdInventory, CmdQuit}},
		{"encounter", StateEncounter, []CommandType{CmdFight, CmdSneak, CmdQuit}},
		{"combat", StateCombat, []CommandType{CmdAttack, CmdFlee, CmdQuit}},
		{"loot decision", StateLoot, []CommandType{CmdWear, CmdStashLoot, CmdDiscardLoot, CmdQuit}},
		{"inventory", StateInventory, []CommandType{CmdWear, CmdDrop, CmdClose, CmdQuit}},
		{"merchant", StateMerchant, []CommandType{CmdBuy, CmdSell, CmdLeave, CmdQuit}},
		{"game over", StateGameOver, []CommandType{CmdQuit}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AllowedCommands(tt.state)
			if len(got) != len(tt.want) {
				t.Fatalf("AllowedCommands(%v) = %v, want %v", tt.state, got, tt.want)
			}
			for _, c := range tt.want {
				if !containsCommand(got, c) {
					t.Errorf("AllowedCommands(%v) is missing %v", tt.state, c)
				}
			}
		})
	}
}

func containsCommand(cmds []CommandType, c CommandType) bool {
	for _, got := range cmds {
		if got == c {
			return true
		}
	}
	return false
}

// testBagItem builds a bag-ready item of the given slot name.
func testBagItem(id, slot string) *entities.Item {
	item := entities.NewItemFromAsset(game.ItemAsset{ID: id, Name: id, Slot: slot})
	return &item
}

// fillBag fills the hero's bag to the brim with filler items.
func fillBag(e *Engine) {
	for e.Player.HasBagRoom() {
		e.Player.StashItem(testBagItem("filler", "amulet"))
	}
}

// CmdInventory (only from StateExploring) opens the bag.
func TestInventoryOpensFromExploring(t *testing.T) {
	e := newTestEngine()

	events := e.Process(Command{Type: CmdInventory})

	if e.State != StateInventory {
		t.Fatalf("state after CmdInventory = %v, want StateInventory", e.State)
	}
	if got := eventTypes(events); len(got) != 1 || got[0] != EventMessage {
		t.Errorf("events = %v, want a single message", got)
	}
}

// CmdWear swaps a bag item into its slot; the displaced item takes the bag
// item's old position. Wearing does NOT close the bag.
func TestInventoryWear(t *testing.T) {
	t.Run("swap sends the displaced item to the bag position", func(t *testing.T) {
		e := newTestEngine()
		oldSword := testBagItem("rusty_sword", "weapon")
		e.Player.EquipItem(oldSword)
		newSword := testBagItem("shiny_sword", "weapon")
		e.Player.StashItem(newSword)
		e.State = StateInventory

		events := e.Process(Command{Type: CmdWear, ItemIndex: 0})

		if got := eventTypes(events); !typesEqual(got, EventItemEquipped) {
			t.Fatalf("events = %v, want [item-equipped]", got)
		}
		if item, _ := events[0].Data["item"].(*entities.Item); item != newSword {
			t.Errorf("equipped = %v, want the new sword", item)
		}
		if swapped, _ := events[0].Data["swapped_from"].(*entities.Item); swapped != oldSword {
			t.Errorf("swapped_from = %v, want the rusty sword", swapped)
		}
		if e.Player.Weapon != newSword {
			t.Error("bag item not equipped in the weapon slot")
		}
		bag := e.Player.Bag()
		if len(bag) != 1 || bag[0] != oldSword {
			t.Errorf("bag = %v, want the displaced sword in the old position", bag)
		}
		if e.State != StateInventory {
			t.Errorf("state after equip = %v, want StateInventory (bag stays open)", e.State)
		}
	})

	t.Run("wearing over an empty slot just shrinks the bag", func(t *testing.T) {
		e := newTestEngine()
		helm := testBagItem("helm", "helmet")
		e.Player.StashItem(helm)
		e.State = StateInventory

		events := e.Process(Command{Type: CmdWear, ItemIndex: 0})

		if got := eventTypes(events); !typesEqual(got, EventItemEquipped) {
			t.Fatalf("events = %v, want [item-equipped]", got)
		}
		if _, ok := events[0].Data["swapped_from"]; ok {
			t.Error("swapped_from present with nothing displaced")
		}
		if e.Player.Head != helm {
			t.Error("bag item not equipped in the helmet slot")
		}
		if got := len(e.Player.Bag()); got != 0 {
			t.Errorf("bag length = %d, want 0", got)
		}
	})
}

// CmdDrop destroys the bag item on the spot (there is no floor layer).
func TestInventoryDrop(t *testing.T) {
	e := newTestEngine()
	helm := testBagItem("helm", "helmet")
	e.Player.StashItem(helm)
	e.State = StateInventory

	events := e.Process(Command{Type: CmdDrop, ItemIndex: 0})

	if got := eventTypes(events); !typesEqual(got, EventItemDiscarded) {
		t.Fatalf("events = %v, want [item-discarded]", got)
	}
	if item, _ := events[0].Data["item"].(*entities.Item); item != helm {
		t.Errorf("discarded = %v, want the helm", item)
	}
	if got := len(e.Player.Bag()); got != 0 {
		t.Errorf("bag length = %d, want 0 after drop", got)
	}
	if e.State != StateInventory {
		t.Errorf("state after drop = %v, want StateInventory (bag stays open)", e.State)
	}
}

// CmdClose returns to exploring; the bag contents survive.
func TestInventoryClose(t *testing.T) {
	e := newTestEngine()
	helm := testBagItem("helm", "helmet")
	e.Player.StashItem(helm)
	e.State = StateInventory

	e.Process(Command{Type: CmdClose})

	if e.State != StateExploring {
		t.Errorf("state after close = %v, want StateExploring", e.State)
	}
	if got := len(e.Player.Bag()); got != 1 {
		t.Errorf("bag length = %d, want 1 (close keeps the bag)", got)
	}
}

// Out-of-range/negative indexes must hit the generic invalid-command path
// with NO state change and NO bag change — they are player-reachable.
func TestInventoryInvalidIndex(t *testing.T) {
	tests := []struct {
		name      string
		bagItems  int
		itemIndex int
	}{
		{"negative index on a filled bag", 2, -1},
		{"index past the end", 2, 2},
		{"index into an empty bag", 0, 0},
	}
	for _, cmdType := range []CommandType{CmdWear, CmdDrop} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%v %s", cmdType, tt.name), func(t *testing.T) {
				e := newTestEngine()
				for i := 0; i < tt.bagItems; i++ {
					e.Player.StashItem(testBagItem("filler", "amulet"))
				}
				e.State = StateInventory

				events := e.Process(Command{Type: cmdType, ItemIndex: tt.itemIndex})

				if got := eventTypes(events); len(got) != 1 || got[0] != EventMessage {
					t.Errorf("events = %v, want a single [invalid command] message", got)
				}
				if msg, _ := events[0].Data["message"].(string); msg == "" {
					t.Error("invalid command carried no message")
				}
				if e.State != StateInventory {
					t.Errorf("state changed on invalid index: %v", e.State)
				}
				if bag := e.Player.Bag(); len(bag) != tt.bagItems {
					t.Errorf("bag changed on invalid index: %d items", len(bag))
				}
			})
		}
	}
}

// Bag commands are only valid in StateInventory; anywhere else they are the
// generic invalid-command event, whatever the index. CmdWear is also valid
// in StateLoot, so it is covered by TestWearRejectedOutsideLootAndInventory
// (below) instead of this matrix.
func TestBagCommandsRejectedOutsideInventory(t *testing.T) {
	states := []GameState{StateExploring, StateEncounter, StateCombat, StateLoot, StateGameOver}
	for _, state := range states {
		for _, cmdType := range []CommandType{CmdDrop} {
			t.Run(fmt.Sprintf("%v %v", state, cmdType), func(t *testing.T) {
				e := newTestEngine()
				e.Player.StashItem(testBagItem("filler", "amulet"))
				e.State = state

				events := e.Process(Command{Type: cmdType, ItemIndex: 0})

				if got := eventTypes(events); len(got) != 1 || got[0] != EventMessage {
					t.Errorf("events = %v, want a single [invalid command] message", got)
				}
				if e.State != state {
					t.Errorf("state changed on a rejected command: %v", e.State)
				}
			})
		}
	}
}

// CmdWear belongs to StateLoot and StateInventory only: anywhere else it is
// the generic invalid-command event, whatever the index.
func TestWearRejectedOutsideLootAndInventory(t *testing.T) {
	states := []GameState{StateExploring, StateEncounter, StateCombat, StateMerchant, StateGameOver}
	for _, state := range states {
		t.Run(state.String(), func(t *testing.T) {
			e := newTestEngine()
			e.Player.StashItem(testBagItem("filler", "amulet"))
			e.State = state

			events := e.Process(Command{Type: CmdWear, ItemIndex: 0})

			if got := eventTypes(events); len(got) != 1 || got[0] != EventMessage {
				t.Errorf("events = %v, want a single [invalid command] message", got)
			}
			if e.State != state {
				t.Errorf("state changed on a rejected command: %v", e.State)
			}
		})
	}
}

// The stash/discard loot commands are only valid in StateLoot; anywhere
// else — the open bag included — they are the generic invalid-command
// event.
func TestLootCommandsRejectedOutsideLootState(t *testing.T) {
	states := []GameState{StateExploring, StateEncounter, StateCombat, StateInventory, StateGameOver}
	for _, state := range states {
		for _, cmdType := range []CommandType{CmdStashLoot, CmdDiscardLoot} {
			t.Run(fmt.Sprintf("%v %v", state, cmdType), func(t *testing.T) {
				e := newTestEngine()
				e.State = state

				events := e.Process(Command{Type: cmdType})

				if got := eventTypes(events); len(got) != 1 || got[0] != EventMessage {
					t.Errorf("events = %v, want a single [invalid command] message", got)
				}
				if e.State != state {
					t.Errorf("state changed on a rejected command: %v", e.State)
				}
			})
		}
	}
}
