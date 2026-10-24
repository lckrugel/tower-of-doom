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
}

// orderedOptions is the canonical display/d Dispatch order for every action
// across all states; each state's bar shows only what AllowedCommands
// permits, in this order.
var orderedOptions = []struct {
	key   string
	label string
	cmd   engine.CommandType
}{
	{"e", "Explore", engine.CmdExplore},
	{"a", "Ascend", engine.CmdAscend},
	{"f", "Fight", engine.CmdFight},
	{"s", "Sneak", engine.CmdSneak},
	{"a", "Attack", engine.CmdAttack},
	{"f", "Flee", engine.CmdFlee},
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
		if opt.cmd == engine.CmdAscend {
			enabled = e.CanAscend
		}
		actions = append(actions, action{
			key: opt.key, label: opt.label, cmd: opt.cmd, enabled: enabled,
		})
	}
	return actions
}

func quitAction() action {
	return action{key: "q", label: "Quit", cmd: engine.CmdQuit, enabled: true}
}
