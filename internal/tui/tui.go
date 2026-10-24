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
	// bagCursor is the selected row of the state's selectable list. While
	// the bag is open (StateInventory) it indexes the hero bag; while
	// shopping (StateMerchant) it indexes the combined stock-then-bag
	// list (stock rows come first, so BUY takes it as the stock index
	// and SELL subtracts the stock size to reach the bag row). ←/→ walk
	// it with wraparound and the indexed commands (WEAR/DROP/BUY/SELL)
	// dispatch with it; every dispatch resets it, so re-entering a list
	// selects the first row again.
	bagCursor int
	// width/height track the terminal size from WindowSizeMsg; when both
	// are too small for the fixed canvas, View() falls back to a bare
	// "terminal too small" screen.
	width, height int
	// recap holds the run summary carried by EventHeroDefeated, captured
	// as the event passes through the log. The game-over body quotes it;
	// nil (legacy defeat without stats) falls back to the engine's final
	// player state.
	recap *runRecap
	// confirmQuit arms the quit-confirmation prompt: while true the
	// action bar is replaced by the Really-quit line and every key but y
	// cancels back. Arming is TUI-internal (no engine call); the flag
	// never survives a dispatch, so any state change restores the bar.
	confirmQuit bool
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

// handleKey resolves a key press: ctrl+c always quits — immediately,
// even out of the quit confirmation. While the confirmation is armed
// every key but y cancels back to the normal bar. Otherwise esc quits —
// except where it means something closer (it closes the bag; while a
// chest loot is pending or the merchant is open it does nothing, since
// both are explicit-only decisions: quitting there would silently
// destroy the pending item, and the shop is left through [L]). Arrows
// move the action-bar cursor — or the state's row cursor while the bag
// or the shop list is open —, Enter dispatches the selection, and
// single-letter shortcuts dispatch their action directly
// (case-insensitive). The Quit action goes through the confirmation
// everywhere but game over.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m.dispatch(quitAction())
	}
	if m.confirmQuit {
		// The prompt is modal: only y confirms, anything else — n, esc,
		// arrows, unknown letters — cancels with no dispatch, no log.
		if strings.ToLower(msg.String()) == "y" {
			return m.dispatch(quitAction())
		}
		m.confirmQuit = false
		return m, nil
	}
	switch msg.String() {
	case "esc":
		switch m.engine.State {
		case engine.StateInventory:
			return m.dispatch(closeAction())
		case engine.StateLoot, engine.StateMerchant:
			return m, nil
		}
		return m.dispatch(quitAction())
	case "left":
		if n := m.selectableRows(); n > 0 {
			m.bagCursor = (m.bagCursor - 1 + n) % n
			return m, nil
		}
		actions := actionsFor(m.engine)
		m.cursor = (m.cursor - 1 + len(actions)) % len(actions)
		return m, nil
	case "right":
		if n := m.selectableRows(); n > 0 {
			m.bagCursor = (m.bagCursor + 1) % n
			return m, nil
		}
		actions := actionsFor(m.engine)
		m.cursor = (m.cursor + 1) % len(actions)
		return m, nil
	case "enter":
		return m.confirmOrDispatch(m.selectedAction())
	default:
		// Single-letter shortcuts: match the key against the current bar.
		key := strings.ToLower(msg.String())
		if len(key) == 1 {
			for _, a := range actionsFor(m.engine) {
				if a.key == key {
					return m.confirmOrDispatch(a)
				}
			}
		}
		return m, nil
	}
}

// confirmOrDispatch routes the Quit action through the TUI-internal
// confirmation — everywhere but game over, where there is nothing left
// to lose — and dispatches every other action directly.
func (m Model) confirmOrDispatch(a action) (tea.Model, tea.Cmd) {
	if a.cmd == engine.CmdQuit && m.engine.State != engine.StateGameOver {
		m.confirmQuit = true
		return m, nil
	}
	return m.dispatch(a)
}

func (m Model) selectedAction() action {
	actions := actionsFor(m.engine)
	cursor := m.cursor
	if cursor >= len(actions) {
		cursor = len(actions) - 1
	}
	return actions[cursor]
}

// selectableRows is the length of the list the row cursor walks: the
// hero bag while it is open, the merchant's combined stock-then-bag
// list while shopping. 0 where there is nothing to select — the arrows
// then move the action-bar cursor instead.
func (m Model) selectableRows() int {
	switch m.engine.State {
	case engine.StateInventory:
		return len(m.engine.Player.Bag())
	case engine.StateMerchant:
		return len(m.engine.MerchantStock) + len(m.engine.Player.Bag())
	default:
		return 0
	}
}

// itemIndexFor resolves the row an index-carrying command acts on. The
// bag commands (WEAR/DROP) take the bag cursor directly; at the
// merchant the same cursor walks the combined stock-then-bag list, so
// BUY takes it as-is (stock rows come first) and SELL subtracts the
// stock size to reach the bag row. A cursor on the wrong section sends
// an out-of-range index, which the engine rejects as a generic message
// — never a panic, no state change.
func (m Model) itemIndexFor(cmd engine.CommandType) int {
	if cmd == engine.CmdSell {
		return m.bagCursor - len(m.engine.MerchantStock)
	}
	return m.bagCursor
}

// dispatch sends an action to the engine, appends every event to the log
// and reports a tea.Quit when the engine ends the session.
func (m Model) dispatch(a action) (tea.Model, tea.Cmd) {
	quit := false
	cmd := engine.Command{Type: a.cmd}
	if a.usesIndex {
		// Indexed commands act on the selected row; an out-of-range
		// index is rejected by the engine as a generic message, no
		// state change.
		cmd.ItemIndex = m.itemIndexFor(a.cmd)
	}
	for _, ev := range m.engine.Process(cmd) {
		m.absorb(ev)
		if ev.Type == engine.EventQuit {
			quit = true
		}
	}
	m.log = trimLog(m.log)
	m.cursor = 0    // the action bar is rebuilt for the (possibly new) state
	m.bagCursor = 0 // re-entering the bag (or the shop) selects the first row again
	m.confirmQuit = false
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
