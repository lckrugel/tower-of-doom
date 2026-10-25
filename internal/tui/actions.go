package tui

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
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
	// (e.g. a locked ASCEND, a cooling move chip). Dispatching a
	// disabled action still reaches the engine, which explains the
	// block as a log message.
	enabled bool
	// usesIndex marks the index-carrying commands: the row-indexed ones
	// (WEAR/DROP/BUY/SELL) dispatch with the selected row, the combat
	// move chips (CmdMove) with their own move position (index).
	usesIndex bool
	// index is the move position a CmdMove chip dispatches (ignored by
	// every other action, which derive their index from the cursor).
	index int
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
	// CmdMove is deliberately absent: the combat bar is built from the
	// hero's move chips (see combatActions), not from this static table.
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
	// Combat swaps the static bar for the hero's move chips.
	if slices.Contains(allowed, engine.CmdMove) {
		return combatActions(e)
	}
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
			index: -1,
		})
	}
	return actions
}

// combatActions builds the combat bar: the hero's combat moves as chips
// keyed 1-4 (in effective-set order — the exact set CmdMove's index
// points into), then [R] Flee and [Q] Quit. A move on cooldown renders
// as a disabled chip carrying its turns-left marker, e.g.
// "[1] Heavy Slash (2)"; dispatching it still reaches the engine, which
// blocks it (or recovers, when every move is locked) in the log.
func combatActions(e *engine.Engine) []action {
	actions := make([]action, 0, entities.MaxMoves+2)
	for i, move := range e.CombatMoves() {
		label := move.Name
		enabled := true
		if cd := e.MoveCooldown(move.ID); cd > 0 {
			label = fmt.Sprintf("%s (%d)", move.Name, cd)
			enabled = false
		}
		actions = append(actions, action{
			key: strconv.Itoa(i + 1), label: label, cmd: engine.CmdMove,
			enabled: enabled, usesIndex: true, index: i,
		})
	}
	actions = append(actions,
		action{key: "r", label: "Flee", cmd: engine.CmdFlee, enabled: true},
		action{key: "q", label: "Quit", cmd: engine.CmdQuit, enabled: true},
	)
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
