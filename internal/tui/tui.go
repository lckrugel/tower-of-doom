package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lckrugel/tower-of-doom/internal/game/engine"
)

// maxLogLines bounds the event log panel to the most recent lines.
const maxLogLines = 8

// Model wraps the game engine for bubbletea. It owns no game logic: every
// action goes through engine.Process and the events drive the log.
type Model struct {
	engine *engine.Engine
	// log holds the rendered event lines, newest last.
	log []string
	// cursor is the selected index of the current state's action bar.
	cursor int
	// width/height are tracked for future layout work.
	width, height int
}

// New builds the UI for a fresh engine, greeting the player.
func New(e *engine.Engine) Model {
	return Model{
		engine: e,
		log: []string{
			styleDim.Render("Welcome to the Tower of Doom! Climb as high as you dare."),
		},
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	default:
		return m, nil
	}
}

// handleKey resolves a key press: ctrl+c/esc always quit, arrows move the
// action-bar cursor, Enter dispatches the selection, and single-letter
// shortcuts dispatch their action directly (case-insensitive).
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m.dispatch(quitAction())
	case "left":
		actions := actionsFor(m.engine)
		m.cursor = (m.cursor - 1 + len(actions)) % len(actions)
		return m, nil
	case "right":
		actions := actionsFor(m.engine)
		m.cursor = (m.cursor + 1) % len(actions)
		return m, nil
	case "enter":
		return m.dispatch(m.selectedAction())
	default:
		// Single-letter shortcuts: match the key against the current bar.
		key := strings.ToLower(msg.String())
		if len(key) == 1 {
			for _, a := range actionsFor(m.engine) {
				if a.key == key {
					return m.dispatch(a)
				}
			}
		}
		return m, nil
	}
}

func (m Model) selectedAction() action {
	actions := actionsFor(m.engine)
	cursor := m.cursor
	if cursor >= len(actions) {
		cursor = len(actions) - 1
	}
	return actions[cursor]
}

// dispatch sends an action to the engine, appends every event to the log
// and reports a tea.Quit when the engine ends the session.
func (m Model) dispatch(a action) (tea.Model, tea.Cmd) {
	quit := false
	for _, ev := range m.engine.Process(engine.Command{Type: a.cmd}) {
		m.log = append(m.log, renderEvent(ev))
		if ev.Type == engine.EventQuit {
			quit = true
		}
	}
	m.log = trimLog(m.log)
	m.cursor = 0 // the action bar is rebuilt for the (possibly new) state
	if quit {
		return m, tea.Quit
	}
	return m, nil
}

// trimLog keeps only the newest lines so the panel stays bounded.
func trimLog(log []string) []string {
	if len(log) > maxLogLines {
		return log[len(log)-maxLogLines:]
	}
	return log
}
