package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/tui"
)

// validateHorizon validates content coverage up to this floor.
// The tower is infinite (sections with max_floor == -1 cover the rest),
// so a large horizon is enough to guarantee valid content for everything
// reachable in a game.
const validateHorizon = 100_000

func main() {
	registry, err := game.Load("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load assets:", err)
		os.Exit(1)
	}
	if err := registry.Validate(validateHorizon); err != nil {
		fmt.Fprintln(os.Stderr, "invalid game content:", err)
		os.Exit(1)
	}

	gameEngine := engine.NewEngine(registry)

	// WithAltScreen keeps the frame off the terminal scrollback: the game
	// screen is redrawn in place, and quitting restores the shell as it
	// was.
	if _, err := tea.NewProgram(tui.New(gameEngine), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error running the TUI:", err)
		os.Exit(1)
	}
}
