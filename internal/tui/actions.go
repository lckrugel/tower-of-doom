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
	// usesIndex marks the bag commands (WEAR/DROP): dispatch must carry
	// the bag cursor as the command's ItemIndex.
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
	{"a", "Attack", engine.CmdAttack},
	{"r", "Flee", engine.CmdFlee},      // r for "run"
	{"g", "Grab", engine.CmdEquipLoot}, // g for "grab": wear the chest loot
	{"b", "Bag", engine.CmdStashLoot},  // stash the chest loot in the bag
	{"d", "Discard", engine.CmdDiscardLoot},
	{"w", "Wear", engine.CmdEquip}, // wear the selected bag item
	{"x", "Drop", engine.CmdDrop},  // destroy the selected bag item
	{"c", "Close", engine.CmdClose},
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
		case engine.CmdEquip, engine.CmdDrop:
			// An empty bag has nothing to wear or drop; dispatching
			// anyway still reaches the engine, which rejects it in the log.
			enabled = len(e.Player.Bag()) > 0
		}
		actions = append(actions, action{
			key: opt.key, label: opt.label, cmd: opt.cmd, enabled: enabled,
			usesIndex: opt.cmd == engine.CmdEquip || opt.cmd == engine.CmdDrop,
		})
	}
	return actions
}

func quitAction() action {
	return action{key: "q", label: "Quit", cmd: engine.CmdQuit, enabled: true}
}
