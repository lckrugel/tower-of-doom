package tui

import (
	"strings"
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game/engine"
)

// confirmableStates lists every state where quitting asks for
// confirmation — all but game over, where nothing is left to lose.
var confirmableStates = []engine.GameState{
	engine.StateExploring, engine.StateEncounter, engine.StateCombat,
	engine.StateLoot, engine.StateInventory, engine.StateMerchant,
}

// Pressing q never dispatches right away: the model arms the quit
// confirmation instead — no engine call, no log line, and the bar turns
// into the Really-quit prompt.
func TestQuitKeyAsksForConfirmation(t *testing.T) {
	for _, state := range confirmableStates {
		m := New(testEngine())
		m.engine.State = state
		logLen := len(m.log)

		m, cmd := press(m, "q")

		if cmd != nil {
			t.Errorf("state %v: 'q' requested a quit without confirmation", state)
		}
		if !m.confirmQuit {
			t.Errorf("state %v: 'q' did not arm the quit confirmation", state)
		}
		if m.engine.State != state {
			t.Errorf("state %v: 'q' changed the state to %v", state, m.engine.State)
		}
		if len(m.log) != logLen {
			t.Errorf("state %v: 'q' wrote to the log", state)
		}

		view := stripControls(stripANSI(m.View()))
		if !strings.Contains(view, "Really quit? [Y] Yes / [N] No") {
			t.Errorf("state %v: confirm prompt missing from the bar:\n%s", state, view)
		}
		if strings.Contains(view, "[Q] Quit") {
			t.Errorf("state %v: the normal bar still renders while confirming:\n%s", state, view)
		}
		if strings.Contains(view, "← → select") {
			t.Errorf("state %v: the select hint still renders while confirming:\n%s", state, view)
		}
	}
}

// Reaching the Quit entry through the bar's cursor and Enter is the same
// Quit action: it must confirm too, not silently end the run.
func TestEnterOnTheQuitActionAsksForConfirmation(t *testing.T) {
	m := New(testEngine()) // exploring: [E] [U] [I] [Q]
	m, _ = press(m, "right")
	m, _ = press(m, "right")
	m, _ = press(m, "right") // cursor on Quit

	m, cmd := press(m, "enter")

	if cmd != nil {
		t.Error("enter on Quit requested a quit without confirmation")
	}
	if !m.confirmQuit {
		t.Error("enter on Quit did not arm the quit confirmation")
	}
}

// y confirms: the engine's quit event comes back and the model reports
// tea.Quit, exactly like the old immediate quit.
func TestConfirmQuitYesQuits(t *testing.T) {
	m := New(testEngine())
	m, _ = press(m, "q")

	m, cmd := press(m, "y")

	if cmd == nil {
		t.Fatal("'y' after 'q' did not request a quit")
	}
	last := stripANSI(m.log[len(m.log)-1])
	if last != "You persistently close your eyes. Goodbye." {
		t.Errorf("last log line = %q, want the engine's farewell", last)
	}
}

// In confirm mode only y confirms; every other key — n, esc, arrows,
// unknown letters — cancels back to the normal bar without dispatching
// anything or writing to the log.
func TestConfirmQuitCancels(t *testing.T) {
	for _, key := range []string{"n", "esc", "left", "right", "z", "q", "enter"} {
		m := New(testEngine())
		m, _ = press(m, "q")
		logLen := len(m.log)

		m, cmd := press(m, key)

		if cmd != nil {
			t.Errorf("cancel key %q requested a quit", key)
		}
		if m.confirmQuit {
			t.Errorf("cancel key %q left the confirmation armed", key)
		}
		if m.engine.State != engine.StateExploring {
			t.Errorf("cancel key %q changed the state to %v", key, m.engine.State)
		}
		if len(m.log) != logLen {
			t.Errorf("cancel key %q wrote to the log", key)
		}
		view := stripControls(stripANSI(m.View()))
		if strings.Contains(view, "Really quit?") {
			t.Errorf("cancel key %q left the confirm prompt up:\n%s", key, view)
		}
		if !strings.Contains(view, "[Q] Quit") {
			t.Errorf("cancel key %q did not restore the normal bar:\n%s", key, view)
		}
	}
}

// Game over is the one state where q quits immediately: the run is
// already lost, there is nothing the confirmation could protect.
func TestGameOverQuitsImmediately(t *testing.T) {
	e := testEngine()
	e.State = engine.StateGameOver
	m := New(e)

	m, cmd := press(m, "q")

	if cmd == nil {
		t.Error("game-over 'q' did not request a quit")
	}
	if m.confirmQuit {
		t.Error("game-over 'q' armed the confirmation instead of quitting")
	}
}

// ctrl+c keeps quitting immediately, everywhere — including out of an
// armed confirmation: the emergency exit is never gated.
func TestCtrlCQuitsImmediately(t *testing.T) {
	t.Run("from a fresh model", func(t *testing.T) {
		m := New(testEngine())

		m, cmd := press(m, "ctrl+c")

		if cmd == nil {
			t.Error("ctrl+c did not request a quit")
		}
	})
	t.Run("from the confirm prompt", func(t *testing.T) {
		m := New(testEngine())
		m, _ = press(m, "q")

		m, cmd := press(m, "ctrl+c")

		if cmd == nil {
			t.Error("ctrl+c from the confirm prompt did not request a quit")
		}
	})
}
