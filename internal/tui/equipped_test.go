package tui

import (
	"strings"
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// dragonWardrobe dresses the hero in the worst-case loadout: every slot
// filled, including the longest asset name with its full bonus list
// (dragon scale armor). The gear goes on directly (the established
// fixture pattern), so no rng is involved.
func dragonWardrobe(state engine.GameState) Model {
	e := testEngine()
	pieces := []game.ItemAsset{
		{ID: "dragon_helm", Name: "Elmo de Dragão", Slot: "helmet", BonusHP: 15, BonusDEF: 10, Durability: 200},
		{ID: "dragon_scale_armor", Name: "Armadura de Escamas de Dragão", Slot: "body_armor", BonusHP: 30, BonusDEF: 20, BonusSPD: -10, Durability: 300},
		{ID: "dragon_boots", Name: "Botas Dracônicas", Slot: "boots", BonusHP: 10, BonusDEF: 15, BonusSPD: 5, Durability: 250},
		{ID: "dragon_sword", Name: "Espada Dracônica", Slot: "weapon", BonusDMG: 25, BonusSPD: 5, Durability: 250},
		{ID: "dragon_ring", Name: "Anel do Dragão", Slot: "ring", BonusDEF: 8, Durability: 100},
		{ID: "dragon_amulet", Name: "Amuleto do Dragão", Slot: "amulet", BonusHP: 20, Durability: 100},
	}
	for _, asset := range pieces {
		item := entities.NewItemFromAsset(asset)
		e.Player.EquipItem(&item)
	}
	e.State = state
	return New(e)
}

// (a) In the three trade states the right column shows the equipped
// panel — on the right half of the frame, where the log usually lives.
func TestEquippedPanelReplacesTheLogInTheTradeStates(t *testing.T) {
	for _, name := range []string{"loot", "inventory", "merchant"} {
		m := viewModels()[name]

		lines := viewLines(m.View())
		found := false
		for _, line := range lines {
			if idx := strings.Index(line, "◈ EQUIPPED"); idx >= 0 {
				if idx < contentWidth/2 {
					t.Errorf("%s: equipped panel starts at column %d, want the right half (≥ %d)", name, idx, contentWidth/2)
				}
				found = true
			}
		}
		if !found {
			t.Errorf("%s: view is missing the equipped panel:\n%s", name, strings.Join(lines, "\n"))
		}
	}
}

// (b) Everywhere else the log keeps the right column: the equipped
// panel must not leak into exploring, encounters, combat or game over.
func TestEquippedPanelStaysInTheTradeStates(t *testing.T) {
	for _, name := range []string{"exploring", "geared", "encounter", "combat", "full queue", "long log", "game over"} {
		m := viewModels()[name]

		view := stripControls(stripANSI(m.View()))
		if strings.Contains(view, "◈ EQUIPPED") {
			t.Errorf("%s: equipped panel leaked outside the trade states:\n%s", name, view)
		}
		if !strings.Contains(view, "Welcome to the Tower of Doom!") {
			t.Errorf("%s: the log greeting vanished from the view:\n%s", name, view)
		}
	}
}

// (c) In the trade states the log is gone from the screen: the right
// column was swapped, so the greeting must not render.
func TestTradeStatesSwapTheLogAway(t *testing.T) {
	for _, name := range []string{"loot", "inventory", "merchant"} {
		m := viewModels()[name]

		view := stripControls(stripANSI(m.View()))
		if strings.Contains(view, "Welcome to the Tower of Doom!") {
			t.Errorf("%s: the log greeting still renders while the equipped panel is up:\n%s", name, view)
		}
	}
}

// The panel lists all six slots in the fixed order, each occupied slot
// showing the item name with its full bonus list on a second line.
func TestEquippedPanelListsTheSlotsInFixedOrder(t *testing.T) {
	m := dragonWardrobe(engine.StateInventory)

	lines := viewLines(m.equippedPanel())

	if title := strings.TrimSpace(lines[0]); title != "◈ EQUIPPED" {
		t.Fatalf("panel title = %q, want %q", title, "◈ EQUIPPED")
	}

	// Fixed order — the same Head, Body, Feet, Wpn, Ring, Amu the hero
	// panel's gear line uses — one name line and one bonus line each.
	names := []string{
		"Elmo de Dragão",
		"Armadura de Escamas de Dragão",
		"Botas Dracônicas",
		"Espada Dracônica",
		"Anel do Dragão",
		"Amuleto do Dragão",
	}
	bonuses := []string{
		"(+15 HP, +10 DEF)",          // head
		"(+30 HP, +20 DEF, -10 SPD)", // body
		"(+10 HP, +15 DEF, +5 SPD)",  // boots
		"(+25 DMG, +5 SPD)",          // weapon
		"(+8 DEF)",                   // ring
		"(+20 HP)",                   // amulet
	}
	labels := []string{"Head", "Body", "Feet", "Wpn", "Ring", "Amu"}
	for i := range labels {
		nameRow, bonusRow := lines[1+2*i], lines[2+2*i]
		for _, want := range []string{labels[i], names[i]} {
			if !strings.Contains(nameRow, want) {
				t.Errorf("%s slot's name row %q missing %q", labels[i], nameRow, want)
			}
		}
		if !strings.Contains(bonusRow, bonuses[i]) {
			t.Errorf("%s slot's bonus row %q missing %q", labels[i], bonusRow, bonuses[i])
		}
	}
}

// An unequipped slot renders one dim line: the slot label and the
// em-dash placeholder. A bare hero shows nothing but that — the panel
// is title + 6 rows.
func TestEquippedPanelEmptySlots(t *testing.T) {
	m := New(testEngine()) // bare hero: nothing equipped

	lines := viewLines(m.equippedPanel())

	if len(lines) != 7 {
		t.Fatalf("bare hero's panel has %d lines, want 7 (title + 6 empty slots)", len(lines))
	}
	for i, label := range []string{"Head", "Body", "Feet", "Wpn", "Ring", "Amu"} {
		line := lines[i+1]
		if !strings.Contains(line, label) || !strings.Contains(line, "— (empty)") {
			t.Errorf("empty %s slot renders %q, want the label and \"— (empty)\"", label, line)
		}
	}
}

// Worst case: every slot occupied, including the longest asset name
// with its full bonus list. Nothing may lean on slot() to be clipped —
// every line fits the column and the panel fits the middle band.
func TestEquippedPanelNeverOverflowsTheLogColumn(t *testing.T) {
	m := dragonWardrobe(engine.StateLoot)

	lines := viewLines(m.equippedPanel())

	if len(lines) != 13 {
		t.Errorf("fully geared panel renders %d lines, want 13 (title + 6 two-line slots)", len(lines))
	}
	if len(lines) > middleHeight {
		t.Errorf("equipped panel has %d rows, overflows the %d-row right column", len(lines), middleHeight)
	}
	for i, line := range lines {
		if w := ansiLen(line); w > logColWidth-1 {
			t.Errorf("equipped panel line %d is %d wide, overflows the %d-col column: %q", i, w, logColWidth-1, line)
		}
	}

	// The dragon armor must appear whole: name line and bonus line.
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"Armadura de Escamas de Dragão",
		"(+30 HP, +20 DEF, -10 SPD)",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("equipped panel clipped %q:\n%s", want, joined)
		}
	}
}

// The swapped right column must keep the frame byte-exact in the trade
// states — including the worst-case gear (the layout invariants over
// the standard fixtures are covered by the layout tests).
func TestEquippedPanelKeepsTheConstantFrame(t *testing.T) {
	for _, state := range []engine.GameState{engine.StateLoot, engine.StateInventory, engine.StateMerchant} {
		m := dragonWardrobe(state)

		lines := viewLines(m.View())
		if len(lines) != contentHeight+2 {
			t.Errorf("state %v: %d lines, want the full frame of %d", state, len(lines), contentHeight+2)
		}
		for i, line := range lines {
			if w := ansiLen(line); w != contentWidth+4 {
				t.Errorf("state %v: line %d is %d wide, want %d: %q", state, i, w, contentWidth+4, line)
			}
		}
	}
}
