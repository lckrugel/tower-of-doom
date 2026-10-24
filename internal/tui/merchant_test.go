package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// shopItem builds a priced offer for the merchant fixtures: a ring worth
// 100 gold (BuyPrice) that sells for 50 (SellValue).
func shopItem(name string) *entities.Item {
	item := entities.NewItemFromAsset(game.ItemAsset{
		ID: "shop_wares", Name: name, Slot: "ring", BonusDEF: 2, Durability: 40,
	})
	return &item
}

// merchantEngine builds an engine with an open shop: `stock` offers,
// `bag` bag items and `gold` in the purse. The stock is set directly
// (like the chest fixtures set PendingLoot), so no rng is involved and
// every price is deterministic: the offers cost 100 gold, the bag's
// Spare Charms sell for 25.
func merchantEngine(stock, bag, gold int) *engine.Engine {
	e := testEngine()
	for i := range stock {
		e.MerchantStock = append(e.MerchantStock, shopItem(fmt.Sprintf("Offer %c", 'A'+rune(i))))
	}
	for range bag {
		stashTestItem(e)
	}
	e.Player.Gold = gold
	e.State = engine.StateMerchant
	return e
}

// actionWith looks up an action by its engine command in an action bar.
func actionWith(t *testing.T, actions []action, cmd engine.CommandType) action {
	t.Helper()
	for _, a := range actions {
		if a.cmd == cmd {
			return a
		}
	}
	t.Fatalf("no action for command %v in %v", cmd, keysOf(actions))
	return action{}
}

// ←/→ walk the combined merchant list — stock rows first, then the bag
// rows — with wraparound on both ends, and never touch the action-bar
// cursor.
func TestMerchantCursorWalksStockThenBag(t *testing.T) {
	m := New(merchantEngine(3, 1, 0)) // combined list of 4 rows

	m, _ = press(m, "right")
	if m.bagCursor != 1 || m.cursor != 0 {
		t.Fatalf("right: row cursor = %d, bar cursor = %d, want 1 and untouched", m.bagCursor, m.cursor)
	}

	m, _ = press(m, "right")
	m, _ = press(m, "right")
	if m.bagCursor != 3 {
		t.Fatalf("three rights = %d, want the single bag row (after 3 stock rows)", m.bagCursor)
	}

	m, _ = press(m, "right")
	if m.bagCursor != 0 {
		t.Fatalf("right from the last row = %d, want wraparound to 0", m.bagCursor)
	}

	m, _ = press(m, "left")
	if m.bagCursor != 3 {
		t.Fatalf("left from 0 = %d, want wraparound to 3", m.bagCursor)
	}
}

// BUY is enabled only while the cursor sits on a stock row, SELL only on
// a bag row — the bar tells the player which rows each key acts on.
func TestMerchantBuyAndSellFollowTheCursor(t *testing.T) {
	m := New(merchantEngine(2, 2, 1000))

	bar := m.barActions()
	if !actionWith(t, bar, engine.CmdBuy).enabled {
		t.Error("buy disabled while the cursor is on a stock row")
	}
	if actionWith(t, bar, engine.CmdSell).enabled {
		t.Error("sell enabled while the cursor is on a stock row")
	}

	m, _ = press(m, "right")
	m, _ = press(m, "right") // into the bag rows

	bar = m.barActions()
	if actionWith(t, bar, engine.CmdBuy).enabled {
		t.Error("buy enabled while the cursor is on a bag row")
	}
	if !actionWith(t, bar, engine.CmdSell).enabled {
		t.Error("sell disabled while the cursor is on a bag row")
	}

	// The bar renders the disabled state, not just the flag.
	view := stripControls(stripANSI(m.View()))
	if !strings.Contains(view, "[P] Buy ✗") {
		t.Errorf("view must render buy as disabled on a bag row:\n%s", view)
	}
}

// Pressing p buys the highlighted offer through the real engine: the
// price moves to the merchant, the offer leaves the stock, the item
// lands in the bag and the shop stays open.
func TestBuyPurchasesTheSelectedOffer(t *testing.T) {
	e := merchantEngine(2, 0, 500)
	first, second := e.MerchantStock[0], e.MerchantStock[1]
	m := New(e)

	m, _ = press(m, "p")

	if m.engine.State != engine.StateMerchant {
		t.Errorf("state after 'p' = %v, want to stay shopping", m.engine.State)
	}
	if m.engine.Player.Gold != 400 {
		t.Errorf("gold after the buy = %d, want 400", m.engine.Player.Gold)
	}
	if len(m.engine.MerchantStock) != 1 || m.engine.MerchantStock[0] != second {
		t.Error("the bought offer must leave the stock")
	}
	if bag := m.engine.Player.Bag(); len(bag) != 1 || bag[0] != first {
		t.Error("the bought item must land in the bag")
	}
	if m.bagCursor != 0 {
		t.Errorf("row cursor after dispatch = %d, want back on the first row", m.bagCursor)
	}
}

// Pressing v on a bag row sells that item: its SellValue moves to the
// purse and the item is destroyed.
func TestSellSellsTheSelectedBagItem(t *testing.T) {
	e := merchantEngine(1, 2, 0)
	second := e.Player.Bag()[1]
	m := New(e)

	m, _ = press(m, "right") // skips the single stock row
	m, _ = press(m, "right") // selects bag[1]
	m, _ = press(m, "v")

	if m.engine.State != engine.StateMerchant {
		t.Errorf("state after 'v' = %v, want to stay shopping", m.engine.State)
	}
	if m.engine.Player.Gold != 25 {
		t.Errorf("gold after the sale = %d, want 25", m.engine.Player.Gold)
	}
	if bag := m.engine.Player.Bag(); len(bag) != 1 || bag[0] == second {
		t.Error("the sold item must leave the bag")
	}
}

// [L] leaves the shop explicitly: back to exploring, stock discarded.
func TestLeaveReturnsToExploring(t *testing.T) {
	m := New(merchantEngine(2, 1, 0))

	m, _ = press(m, "l")

	if m.engine.State != engine.StateExploring {
		t.Errorf("state after 'l' = %v, want StateExploring", m.engine.State)
	}
	if m.engine.MerchantStock != nil {
		t.Error("leaving must discard the stock")
	}
	last := stripANSI(m.log[len(m.log)-1])
	if last != "You leave the merchant to their wares." {
		t.Errorf("last log line = %q, want the farewell", last)
	}
}

// esc at the merchant is a no-op, like the loot decision: leaving the
// shop is explicit ([L]), never a stray keypress away.
func TestEscapeIsANoOpAtTheMerchant(t *testing.T) {
	m := New(merchantEngine(2, 1, 0))
	logLen := len(m.log)

	m, cmd := press(m, "esc")

	if cmd != nil {
		t.Error("esc at the merchant must not request a quit")
	}
	if m.engine.State != engine.StateMerchant {
		t.Errorf("state after esc = %v, want to stay shopping", m.engine.State)
	}
	if len(m.engine.MerchantStock) != 2 {
		t.Error("esc must not touch the stock")
	}
	if len(m.log) != logLen {
		t.Error("esc wrote to the log at the merchant")
	}
}

// A rejected buy leaves the shop exactly as it was — the engine explains
// the block in the log and the player picks another option.
func TestBuyWithAFullBagIsRejected(t *testing.T) {
	e := merchantEngine(1, entities.InventoryCapacity, 10000)
	offer := e.MerchantStock[0]
	m := New(e)

	m, _ = press(m, "p")

	if m.engine.State != engine.StateMerchant {
		t.Errorf("state after the rejected buy = %v, want to stay shopping", m.engine.State)
	}
	if m.engine.Player.Gold != 10000 {
		t.Errorf("gold after the rejected buy = %d, want untouched", m.engine.Player.Gold)
	}
	if len(m.engine.MerchantStock) != 1 || m.engine.MerchantStock[0] != offer {
		t.Error("a rejected buy must keep the offer stocked")
	}
	if len(m.engine.Player.Bag()) != entities.InventoryCapacity {
		t.Error("a rejected buy must not touch the bag")
	}
	last := stripANSI(m.log[len(m.log)-1])
	if last != "Your bag is full." {
		t.Errorf("last log line = %q, want the engine's explanation", last)
	}
}

// Not enough gold: the same no-state-change rejection, with the engine's
// own message.
func TestBuyWithoutGoldIsRejected(t *testing.T) {
	e := merchantEngine(2, 0, 99) // the offers cost 100
	stock := append([]*entities.Item{}, e.MerchantStock...)
	m := New(e)

	m, _ = press(m, "p")

	if m.engine.State != engine.StateMerchant {
		t.Errorf("state after the rejected buy = %v, want to stay shopping", m.engine.State)
	}
	if m.engine.Player.Gold != 99 {
		t.Errorf("gold after the rejected buy = %d, want untouched", m.engine.Player.Gold)
	}
	if len(m.engine.MerchantStock) != 2 || m.engine.MerchantStock[0] != stock[0] || m.engine.MerchantStock[1] != stock[1] {
		t.Error("a rejected buy must keep the stock")
	}
	last := stripANSI(m.log[len(m.log)-1])
	if last != "Not enough gold." {
		t.Errorf("last log line = %q, want the engine's explanation", last)
	}
}

// A shortcut for the other section reaches the engine anyway (disabled
// dispatch is the existing pattern) and the engine rejects the index
// without any state change.
func TestMismatchedMerchantShortcutIsRejectedByTheEngine(t *testing.T) {
	m := New(merchantEngine(1, 1, 1000))

	// Cursor on the stock row: sell points before the bag.
	m, _ = press(m, "v")
	if m.engine.State != engine.StateMerchant {
		t.Fatalf("state after a mismatched sell = %v, want to stay shopping", m.engine.State)
	}
	if last := stripANSI(m.log[len(m.log)-1]); last != "Invalid command" {
		t.Errorf("last log line = %q, want the engine's rejection", last)
	}

	// Cursor on the bag row: buy points past the stock.
	m, _ = press(m, "right")
	m, _ = press(m, "p")
	if m.engine.State != engine.StateMerchant {
		t.Fatalf("state after a mismatched buy = %v, want to stay shopping", m.engine.State)
	}
	if last := stripANSI(m.log[len(m.log)-1]); last != "Invalid command" {
		t.Errorf("last log line = %q, want the engine's rejection", last)
	}
}

// The merchant screen names the shop, its offers with buy prices, the
// bag section with sell values, and the shopping actions.
func TestViewMerchantPanel(t *testing.T) {
	m := New(merchantEngine(2, 1, 120))

	view := stripControls(stripANSI(m.View()))
	for _, want := range []string{
		"MERCHANT",
		"Offer A (+2 DEF) — 100 gold",
		"Offer B (+2 DEF) — 100 gold",
		"YOUR BAG (1/10)",
		"Spare Charm (+1 DEF) — sells for 25 gold",
		"▶ Offer A", // the cursor opens on the first stock row
		"[P] Buy", "[V] Sell", "[L] Leave", "[Q] Quit",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("merchant view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "▶ Offer B") || strings.Contains(view, "▶ Spare Charm") {
		t.Errorf("merchant view must highlight only the selected row:\n%s", view)
	}

	// The rows live in the left column: no line may lean on slot() to
	// be clipped.
	for i, line := range viewLines(m.merchantPanel()) {
		if w := ansiLen(line); w > mainColWidth {
			t.Errorf("merchant body line %d is %d wide, overflows the %d-col column:\n%q", i, w, mainColWidth, line)
		}
	}
}

// The selection follows the cursor across both sections: on a bag row
// the marker moves there (price suffix included in the row).
func TestViewMerchantHighlightsBagRowsToo(t *testing.T) {
	e := merchantEngine(1, 0, 0)
	stashNamedItem(e, "Charm One")
	stashNamedItem(e, "Charm Two")
	m := New(e)

	m, _ = press(m, "right") // past the single stock row
	m, _ = press(m, "right") // bag[1]: Charm Two

	view := stripControls(stripANSI(m.View()))
	if !strings.Contains(view, "▶ Charm Two") {
		t.Errorf("view must highlight the selected bag row:\n%s", view)
	}
	if strings.Contains(view, "▶ Offer A") || strings.Contains(view, "▶ Charm One") {
		t.Errorf("view must not highlight unselected rows:\n%s", view)
	}
}

// Worst-case layout math: the panel must never exceed the body slot —
// title (1) + stock (≤3) + bag label (1) + bag (≤10) = 15 — and never
// lean on slot() to clip.
func TestMerchantPanelFitsTheBodySlot(t *testing.T) {
	tests := []struct {
		name  string
		stock int
		bag   int
		exact bool // the full shop renders exactly bodyHeight rows
	}{
		{"full shop", 3, entities.InventoryCapacity, true},
		{"bare shelves", 0, entities.InventoryCapacity, false},
		{"empty bag", 3, 0, false},
		{"nothing left", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(merchantEngine(tt.stock, tt.bag, 0))

			lines := viewLines(m.merchantPanel())

			if len(lines) > bodyHeight {
				t.Errorf("merchant panel has %d rows, overflows the %d-row body slot", len(lines), bodyHeight)
			}
			if tt.exact && len(lines) != bodyHeight {
				t.Errorf("full shop renders %d rows, want exactly %d (title + stock + bag label + bag)", len(lines), bodyHeight)
			}
			for i, line := range lines {
				if w := ansiLen(line); w > mainColWidth {
					t.Errorf("merchant body line %d is %d wide, overflows the %d-col column:\n%q", i, w, mainColWidth, line)
				}
			}
		})
	}
}

// The guards name the empty sections instead of rendering ghost rows.
func TestViewMerchantGuards(t *testing.T) {
	t.Run("bought-out stock", func(t *testing.T) {
		m := New(merchantEngine(0, 2, 0))
		view := stripControls(stripANSI(m.View()))
		if !strings.Contains(view, "(nothing left to buy)") {
			t.Errorf("bought-out shop missing the guard line:\n%s", view)
		}
	})
	t.Run("empty bag", func(t *testing.T) {
		m := New(merchantEngine(2, 0, 0))
		view := stripControls(stripANSI(m.View()))
		if !strings.Contains(view, "(nothing to sell)") {
			t.Errorf("empty-bag shop missing the guard line:\n%s", view)
		}
	})
}
