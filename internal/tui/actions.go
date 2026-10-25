package tui

import (
	"slices"

	"github.com/lckrugel/tower-of-doom/internal/game/engine"
)

// action is a contextual command option shown in the action bar.
type action struct {
	// key is the single shortcut that triggers this action (case-insensitive).
	key string
	// label is the display name shown in the action bar (e.g. "Explore").
	label string
	// cmd is the engine command this action dispatches.
	cmd engine.CommandType
	// enabled is false for actions the current state can't fulfill yet
	// (e.g. a locked ASCEND). Dispatching a disabled action still reaches
	// the engine, which explains the block as a log message.
	enabled bool
	// usesIndex marks the row-indexed commands (WEAR/DROP/BUY/SELL):
	// dispatch must carry the selected row as the command's Index.
	usesIndex bool
}

// orderedOptions is the canonical display/d Dispatch order for every action
// across all states; each state's bar shows only what AllowedCommands
// permits, in this order.
// Shortcut keys are assigned once per command and never overlap between
// states: the same key must always mean the same action, so combat
// muscle memory (e.g. spamming Attack) can't spill into another command
// once the state changes.
var orderedOptions = []struct {
	key   string
	label string
	cmd   engine.CommandType
}{
	{"e", "Explore", engine.CmdExplore},
	{"u", "Ascend", engine.CmdAscend}, // u for "up"
	{"i", "Inventory", engine.CmdInventory},
	{"f", "Fight", engine.CmdFight},
	{"s", "Sneak", engine.CmdSneak},
	// {"a"} dispatches the hero's first combat move (the combat-integration
	// stopgap: the move-chip bar replaces this label in the TUI phase).
	{"a", "Attack", engine.CmdMove},
	{"r", "Flee", engine.CmdFlee},     // r for "run"
	{"w", "Wear", engine.CmdWear},     // wears the chest loot or the selected bag item
	{"b", "Bag", engine.CmdStashLoot}, // stash the chest loot in the bag
	{"d", "Discard", engine.CmdDiscardLoot},
	{"x", "Drop", engine.CmdDrop}, // destroy the selected bag item
	{"c", "Close", engine.CmdClose},
	{"p", "Buy", engine.CmdBuy},   // buy the offer under the shop cursor
	{"v", "Sell", engine.CmdSell}, // sell the selected bag item
	{"l", "Leave", engine.CmdLeave},
	{"q", "Quit", engine.CmdQuit},
}

// actionsFor builds the action bar for the engine's current state.
func actionsFor(e *engine.Engine) []action {
	allowed := engine.AllowedCommands(e.State)
	actions := make([]action, 0, len(allowed))
	for _, opt := range orderedOptions {
		if !slices.Contains(allowed, opt.cmd) {
			continue
		}
		enabled := true
		switch opt.cmd {
		case engine.CmdAscend:
			enabled = e.CanAscend
		case engine.CmdWear, engine.CmdDrop:
			// Only the bag-state wear/drop act on the bag: an empty bag
			// has nothing to wear or drop there. The loot-state wear (the
			// pending chest item) is never gated — it is only offered
			// while an item pends. Dispatching a disabled action still
			// reaches the engine, which rejects it in the log.
			enabled = e.State == engine.StateLoot || len(e.Player.Bag()) > 0
		case engine.CmdBuy:
			// Static floor (the shop cursor refines these in barActions):
			// a bought-out merchant has nothing to sell.
			enabled = len(e.MerchantStock) > 0
		case engine.CmdSell:
			enabled = len(e.Player.Bag()) > 0
		}
		actions = append(actions, action{
			key: opt.key, label: opt.label, cmd: opt.cmd, enabled: enabled,
			usesIndex: opt.cmd == engine.CmdWear || opt.cmd == engine.CmdDrop ||
				opt.cmd == engine.CmdBuy || opt.cmd == engine.CmdSell,
		})
	}
	return actions
}

// barActions is the state's action bar with cursor-sensitive enablement
// applied. In the shop, BUY and SELL act on the row the cursor walks:
// each is enabled only while the cursor sits on its own section of the
// combined list (stock rows vs bag rows). Dispatching a disabled action
// still reaches the engine, which explains the rejection in the log.
func (m Model) barActions() []action {
	actions := actionsFor(m.engine)
	if m.engine.State != engine.StateMerchant {
		return actions
	}
	onStock := m.bagCursor < len(m.engine.MerchantStock)
	for i := range actions {
		switch actions[i].cmd {
		case engine.CmdBuy:
			actions[i].enabled = onStock
		case engine.CmdSell:
			// onStock is false only when bag rows exist to select — but
			// a bought-out shop with an emptied bag must keep SELL off.
			actions[i].enabled = !onStock && len(m.engine.Player.Bag()) > 0
		}
	}
	return actions
}

func quitAction() action {
	return action{key: "q", label: "Quit", cmd: engine.CmdQuit, enabled: true}
}

func closeAction() action {
	return action{key: "c", label: "Close", cmd: engine.CmdClose, enabled: true}
}
