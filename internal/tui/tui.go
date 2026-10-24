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
	// width/height track the terminal size from WindowSizeMsg; when both
	// are too small for the fixed canvas, View() falls back to a bare
	// "terminal too small" screen.
	width, height int
	// recap holds the run summary carried by EventHeroDefeated, captured
	// as the event passes through the log. The game-over body quotes it;
	// nil (legacy defeat without stats) falls back to the engine's final
	// player state.
	recap *runRecap
}

// New builds the UI for a fresh engine, greeting the player. The
// greeting is short on purpose: it must fit the log column's width.
func New(e *engine.Engine) Model {
	return Model{
		engine: e,
		log: []string{
			styleDim.Render("Welcome to the Tower of Doom! Climb as you dare."),
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
		m.absorb(ev)
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

// absorb records one engine event's consequences on the model: the
// rendered log line, plus anything a later view needs (the defeat event's
// run stats feed the game-over recap).
func (m *Model) absorb(ev engine.Event) {
	m.log = append(m.log, renderEvent(ev))
	if ev.Type == engine.EventHeroDefeated {
		m.recap = recapFromEvent(ev)
	}
}

// runRecap is the run summary an EventHeroDefeatedWithStats carries:
// the level the hero reached and how many enemies it defeated. The floor
// is deliberately not stored — the recap's "Reached Floor N · Section"
// keeps using the engine's location label, which also names the section.
type runRecap struct {
	level    int
	kills    int
	hasKills bool // an older engine's event may omit the kill count
}

// recapFromEvent captures a defeat event's run statistics. It requires
// the level/floor pair the engine's richer payload sends; anything else
// (legacy events, malformed stats) returns nil, making the recap fall
// back to the engine's final state rather than trusting partial data.
func recapFromEvent(ev engine.Event) *runRecap {
	level, okLevel := ev.Data["level"].(int)
	_, okFloor := ev.Data["floor"].(int)
	if !okLevel || !okFloor {
		return nil
	}
	recap := &runRecap{level: level}
	if kills, ok := ev.Data["kills"].(int); ok {
		recap.kills, recap.hasKills = kills, true
	}
	return recap
}

// trimLog keeps only the newest lines so the panel stays bounded.
func trimLog(log []string) []string {
	if len(log) > maxLogLines {
		return log[len(log)-maxLogLines:]
	}
	return log
}
