package tui

import (
	"strconv"
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
	// mirror is the combat mirror — the model-side picture of the
	// engine's unexported combat-scoped state (move cooldowns, active
	// statuses), rebuilt from the event stream. See combat.go for the
	// round-tracking and reset rules.
	mirror combatMirror
	// gridCursor is the selected cell of the combat move grid (0-3,
	// row-major: [0 1 / 2 3]). The four arrows walk it with wraparound
	// while in combat; it only ever rests on a real move (never on a
	// placeholder) and resets to the first cell with the mirror, on
	// every combat entry.
	gridCursor int
	// overlay marks the '?' lore overlay: a TUI-internal pure view mode
	// (no engine call) that replaces the canvas while open. Any key
	// closes it before anything else dispatches — ctrl+c's emergency
	// exit excepted, which quits from anywhere.
	overlay bool
}

// New builds the UI for a fresh engine, greeting the player. The
// greeting is short on purpose: it must fit the log column's width.
func New(e *engine.Engine) Model {
	return Model{
		engine: e,
		log: []string{
			styleDim.Render("Welcome to the Tower of Doom! Climb as you dare."),
		},
		mirror: newCombatMirror(),
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
// chest loot is pending, the merchant is open, a fight is on or the
// shrine's forget prompt waits it does nothing: those are explicit-only
// decisions, and in combat the arrows are the move grid's navigation —
// esc there would be one stray finger from ending the run). Arrows move
// the action-bar cursor — or the state's row cursor while the bag, the
// shop list or the shrine's forget rows are open, or the move grid's
// cell cursor in combat —, Enter dispatches the selection (the
// highlighted grid cell during combat), and single-letter
// shortcuts dispatch their action directly (case-insensitive); the
// digits 1-4 jump-dispatch grid cells in combat. The Quit action goes
// through the confirmation everywhere but game over.
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
	if m.overlay {
		// The lore overlay is a pure view mode: any key closes it
		// first — nothing dispatches while it is open.
		m.overlay = false
		return m, nil
	}
	switch msg.String() {
	case "esc":
		switch m.engine.State {
		case engine.StateInventory:
			return m.dispatch(closeAction())
		case engine.StateLoot, engine.StateMerchant, engine.StateCombat, engine.StateLearn:
			return m, nil
		}
		return m.dispatch(quitAction())
	case "left", "right", "up", "down":
		if m.engine.State == engine.StateCombat {
			// The move grid owns all four arrows in combat.
			m.gridCursor = gridCursorMove(m.gridCursor, len(m.engine.CombatMoves()), msg.String())
			return m, nil
		}
		if msg.String() == "up" || msg.String() == "down" {
			return m, nil // nothing vertical outside the grid
		}
		step := 1
		if msg.String() == "left" {
			step = -1
		}
		if n := m.selectableRows(); n > 0 {
			m.bagCursor = (m.bagCursor + step + n) % n
			return m, nil
		}
		actions := actionsFor(m.engine)
		m.cursor = (m.cursor + step + len(actions)) % len(actions)
		return m, nil
	case "enter":
		if m.engine.State == engine.StateCombat {
			return m.dispatchMove()
		}
		return m.confirmOrDispatch(m.selectedAction())
	default:
		// Single-key shortcuts: '?' toggles the lore overlay; digits
		// jump-dispatch grid cells in combat; letters match the current
		// bar (case-insensitive).
		key := strings.ToLower(msg.String())
		if len(key) == 1 {
			if key == "?" {
				m.overlay = !m.overlay
				return m, nil
			}
			if m.engine.State == engine.StateCombat {
				if d, ok := gridDigit(key); ok {
					return m.jumpDispatchMove(d)
				}
			}
			for _, a := range actionsFor(m.engine) {
				if a.key == key {
					return m.confirmOrDispatch(a)
				}
			}
		}
		return m, nil
	}
}

// gridDigit resolves a digit key to a zero-based grid cell index.
func gridDigit(key string) (int, bool) {
	if key[0] < '1' || key[0] > '4' {
		return 0, false
	}
	return int(key[0] - '1'), true
}

// gridCursorMove walks the 2×2 cell cursor with wraparound over the REAL
// moves (the grid's placeholders are never selected): left/right step
// one cell, up/down steps one ROW (two cells). A step landing on or
// beyond the move count wraps within the cursor's column — with a ragged
// grid (2 or 3 moves) a vertical step may have nothing to reach and the
// cursor stays put.
func gridCursorMove(cursor, moves int, dir string) int {
	if moves <= 0 {
		return 0
	}
	c := min(max(cursor, 0), moves-1)
	switch dir {
	case "left":
		return (c - 1 + moves) % moves
	case "right":
		return (c + 1) % moves
	case "up", "down":
		step := 2
		if dir == "up" {
			step = -2
		}
		if cand := c + step; cand >= 0 && cand < moves {
			return cand
		}
		col := c % 2
		bottom := col + 2
		if step < 0 && bottom < moves {
			return bottom // wrap to the column's bottom row
		}
		return col // wrap to the column's top row (or stay put)
	default:
		return c
	}
}

// dispatchMove dispatches the highlighted grid cell: Command{CmdMove,
// Index}. A cooling cell dispatches too — the engine answers "Move on
// cooldown." in the log, state intact.
func (m Model) dispatchMove() (tea.Model, tea.Cmd) {
	index := min(max(m.gridCursor, 0), len(m.engine.CombatMoves())-1)
	return m.dispatch(action{
		key: strconv.Itoa(index + 1), label: "move",
		cmd: engine.CmdMove, enabled: true, usesIndex: true, index: index,
	})
}

// jumpDispatchMove dispatches a digit's cell immediately: the cursor
// jumps along (only onto real moves — a digit beyond the move set
// dispatches anyway and the engine explains the rejection in the log).
func (m Model) jumpDispatchMove(digit int) (tea.Model, tea.Cmd) {
	if digit < len(m.engine.CombatMoves()) {
		m.gridCursor = digit
	}
	return m.dispatch(action{
		key: strconv.Itoa(digit + 1), label: "move",
		cmd: engine.CmdMove, enabled: true, usesIndex: true, index: digit,
	})
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
	// The SHAPED bar — the one the screen renders (the shrine's per
	// step, the shop's per cursor) — not the raw union.
	actions := m.barActions()
	cursor := m.cursor
	if cursor >= len(actions) {
		cursor = max(len(actions)-1, 0)
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
	case engine.StateLearn:
		// The shrine's row cursor walks the live step's list: the
		// pending offers while choosing, the learned moves while
		// forgetting.
		if m.learnForgetStep() {
			return len(m.engine.Player.LearnedMoves())
		}
		return len(m.engine.PendingMoves)
	default:
		return 0
	}
}

// indexFor resolves the row an index-carrying command acts on. The
// bag commands (WEAR/DROP) take the bag cursor directly, as does the
// shrine's FORGET (the selected learned move's row); at the
// merchant the same cursor walks the combined stock-then-bag list, so
// BUY takes it as-is (stock rows come first) and SELL subtracts the
// stock size to reach the bag row. (CmdMove does not come through
// here — the move grid carries its own cell index.) A cursor on the
// wrong section sends an out-of-range index, which the engine rejects
// as a generic message — never a panic, no state change.
func (m Model) indexFor(cmd engine.CommandType) int {
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
		// Indexed commands act on their subject: the move grid (and the
		// legacy chips) carry their own position; the row-indexed
		// commands act on the selected row. An out-of-range index is
		// rejected by the engine as a generic message, no state change.
		if a.cmd == engine.CmdMove {
			cmd.Index = a.index
		} else {
			cmd.Index = m.indexFor(a.cmd)
		}
	}
	prevState := m.engine.State
	events := m.engine.Process(cmd)
	// A combat boundary crossed by this dispatch resets the mirror (and
	// the grid cursor with it): on entry BEFORE the batch is absorbed (a
	// failed sneak's free turn tracks on a clean slate), on exit after it
	// — the reset is idempotent, and nothing outside a live fight renders
	// the mirror.
	if (prevState == engine.StateCombat) != (m.engine.State == engine.StateCombat) {
		m.mirror.reset()
		m.gridCursor = 0
	}
	for _, ev := range events {
		m.absorb(ev)
		if ev.Type == engine.EventQuit {
			quit = true
		}
	}
	m.mirror.endBatch()
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
// rendered log line, the combat mirror's update, plus anything a later
// view needs (the defeat event's run stats feed the game-over recap).
func (m *Model) absorb(ev engine.Event) {
	m.log = append(m.log, renderEvent(ev))
	m.mirror.absorb(ev, m.engine)
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
