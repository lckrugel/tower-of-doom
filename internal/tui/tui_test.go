package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// press sends a key string through the model as a bubbletea KeyMsg.
func press(m Model, raw string) (Model, tea.Cmd) {
	var keyMsg tea.KeyMsg
	switch raw {
	case "enter":
		keyMsg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		keyMsg = tea.KeyMsg{Type: tea.KeyEsc}
	case "left":
		keyMsg = tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		keyMsg = tea.KeyMsg{Type: tea.KeyRight}
	default:
		keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(raw)}
	}
	next, cmd := m.Update(keyMsg)
	model, ok := next.(Model)
	if !ok {
		panic("Update returned a non-Model")
	}
	return model, cmd
}

func TestExploreShortcutStartsEncounter(t *testing.T) {
	m := New(testEngine())

	m, _ = press(m, "e")

	if m.engine.State != engine.StateEncounter {
		t.Errorf("state after 'e' = %v, want StateEncounter", m.engine.State)
	}
	if len(m.log) < 2 {
		t.Fatal("explore produced no log lines")
	}
	last := stripANSI(m.log[len(m.log)-1])
	if !strings.Contains(last, "You stumble upon an encounter") {
		t.Errorf("last log line = %q, want an encounter discovery", last)
	}
}

func TestQuitShortcutWorksInEveryState(t *testing.T) {
	for _, state := range []engine.GameState{
		engine.StateExploring, engine.StateEncounter, engine.StateCombat, engine.StateGameOver,
	} {
		m := New(testEngine())
		m.engine.State = state

		_, cmd := press(m, "q")

		if cmd == nil {
			t.Errorf("state %v: 'q' did not request a quit", state)
		}
	}
}

func TestEscapeAlsoQuits(t *testing.T) {
	m := New(testEngine())

	_, cmd := press(m, "esc")

	if cmd == nil {
		t.Error("esc did not request a quit")
	}
}

func TestUnknownKeyIsANoOp(t *testing.T) {
	m := New(testEngine())
	logLen := len(m.log)

	m, cmd := press(m, "z")

	if cmd != nil {
		t.Error("'z' returned a command, want none")
	}
	if m.engine.State != engine.StateExploring {
		t.Errorf("state changed to %v on an unbound key", m.engine.State)
	}
	if len(m.log) != logLen {
		t.Error("unbound key wrote to the log")
	}
}

func TestArrowKeysMoveTheCursorWithWraparound(t *testing.T) {
	m := New(testEngine()) // exploring: 3 actions
	if got := len(actionsFor(m.engine)); got != 3 {
		t.Fatalf("setup: exploring actions = %d, want 3", got)
	}

	m, _ = press(m, "right")
	if m.cursor != 1 {
		t.Fatalf("right from cursor 0 = %d, want 1", m.cursor)
	}

	m, _ = press(m, "left")
	if m.cursor != 0 {
		t.Fatalf("left back = %d, want 0", m.cursor)
	}

	m, _ = press(m, "left")
	if m.cursor != 2 {
		t.Fatalf("left from cursor 0 = %d, want wraparound to 2", m.cursor)
	}
}

// Enter dispatches whatever the cursor highlights — including disabled
// actions, letting the engine explain the block through the log.
func TestEnterDispatchesTheSelectedAction(t *testing.T) {
	m := New(testEngine())
	m, _ = press(m, "right") // selects ASCEND, which starts blocked

	m, cmd := press(m, "enter")

	if cmd != nil {
		t.Fatal("ASCEND while blocked must not quit or emit commands")
	}
	if m.engine.State != engine.StateExploring {
		t.Errorf("state = %v, want StateExploring (ascend was blocked)", m.engine.State)
	}
	last := stripANSI(m.log[len(m.log)-1])
	if last != "Cannot ascend yet" {
		t.Errorf("last log line = %q, want the engine's block message", last)
	}
}

// recast maps a tea.Model result back to Model; Update and dispatch both
// return the tea.Model interface.
func modelOf(next tea.Model, t *testing.T) Model {
	t.Helper()
	model, ok := next.(Model)
	if !ok {
		t.Fatal("Update returned a non-Model")
	}
	return model
}

func TestEventLogIsTrimmedToTheNewestLines(t *testing.T) {
	m := New(testEngine())
	for range maxLogLines * 3 {
		next, _ := m.dispatch(action{key: "e", label: "Explore", cmd: engine.CmdExplore})
		m = modelOf(next, t)
	}

	if len(m.log) != maxLogLines {
		t.Fatalf("log length = %d, want %d", len(m.log), maxLogLines)
	}
}

func TestNavKeysDoNotDispatchAnything(t *testing.T) {
	m := New(testEngine())
	logLen := len(m.log)

	m, _ = press(m, "right")
	m, _ = press(m, "left")

	if m.engine.State != engine.StateExploring {
		t.Errorf("state = %v, want StateExploring", m.engine.State)
	}
	if len(m.log) != logLen {
		t.Error("cursor movement wrote to the log")
	}
}

// stripANSI removes ANSI escape sequences so tests can assert readable text.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			// Skip escape sequences: CSI (ESC [ params letter) or similar.
			i++
			if i < len(s) && (s[i] == '[' || s[i] == ']') {
				i++
				for i < len(s) && !isTerminator(s[i]) {
					i++
				}
				if i < len(s) {
					i++ // consume the terminator
				}
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func isTerminator(r byte) bool {
	// CSI sequences end with a letter between @ and ~.
	return r >= '@' && r <= '~'
}

// stripControls removes control characters so printed views stay readable in
// test failures (lipgloss borders/strings are diffed as-is otherwise).
func stripControls(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' {
			return -1
		}
		return r
	}, s)
}

// View tests (kept in the same file — they exercise the same state helpers).
func TestViewExploring(t *testing.T) {
	m := New(testEngine())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model := modelOf(next, t)

	view := stripControls(stripANSI(model.View()))

	for _, want := range []string{
		"Tower of Doom", "Floor 1", "Goblin Pits", "EXPLORING",
		"HP", "DMG", "[E] Explore", "[U] Ascend", "[Q] Quit",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q in:\n%s", want, view)
		}
	}
}

func encounterWithQueue(state engine.GameState) Model {
	e := testEngine()
	e.Player = entities.NewPlayer()
	e.CurrentEncounter = engine.Encounter{
		Type: engine.EncounterCombat,
		Monsters: []entities.Monster{
			entities.NewMonsterFromAsset(game.MonsterAsset{ID: "goblin", Name: "Goblin", HP: 30}),
			entities.NewMonsterFromAsset(game.MonsterAsset{ID: "orc", Name: "Orc", HP: 50}),
		},
	}
	e.State = state
	return New(e)
}

// At an encounter the whole queue is shown: the player needs it to weigh
// FIGHT against SNEAK.
func TestViewEncounterShowsTheFullQueue(t *testing.T) {
	m := encounterWithQueue(engine.StateEncounter)

	view := stripControls(stripANSI(m.View()))

	if !strings.Contains(view, "Goblin") || !strings.Contains(view, "Orc") {
		t.Errorf("encounter view must show every enemy:\n%s", view)
	}
	if !strings.Contains(view, "[F] Fight") || !strings.Contains(view, "[S] Sneak") {
		t.Errorf("encounter action bar incomplete:\n%s", view)
	}
}

// Combat focuses on the front enemy; the rest of the queue is only a
// counter, distilled so the eye doesn't track dead weight mid-fight.
func TestViewCombatShowsOnlyTheCurrentEnemy(t *testing.T) {
	m := encounterWithQueue(engine.StateCombat)

	view := stripControls(stripANSI(m.View()))

	if !strings.Contains(view, "▶") || !strings.Contains(view, "Goblin") {
		t.Errorf("combat view must highlight the current enemy:\n%s", view)
	}
	if strings.Contains(view, "Orc") {
		t.Errorf("combat view must not list queued enemies:\n%s", view)
	}
	if !strings.Contains(view, "1 more") {
		t.Errorf("combat view should hint at the waiting enemies:\n%s", view)
	}
	if !strings.Contains(view, "[A] Attack") || !strings.Contains(view, "[R] Flee") {
		t.Errorf("combat action bar incomplete:\n%s", view)
	}
}

func TestViewGameOverBanner(t *testing.T) {
	e := testEngine()
	e.State = engine.StateGameOver
	m := New(e)

	if view := stripControls(stripANSI(m.View())); !strings.Contains(view, "YOU DIED") {
		t.Errorf("game over view missing a death banner:\n%s", view)
	}
}
