package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// wideEnough is a terminal size comfortably above minWidth/minHeight, for
// tests that want the full game canvas on screen.
var wideEnough = tea.WindowSizeMsg{Width: 160, Height: 50}

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
	case "up":
		keyMsg = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		keyMsg = tea.KeyMsg{Type: tea.KeyDown}
	case "ctrl+c":
		keyMsg = tea.KeyMsg{Type: tea.KeyCtrlC}
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

// The quit-confirmation tests live in quit_test.go.

// esc quits everywhere except where it has a closer meaning: it closes
// the bag (StateInventory) and is a deliberate no-op while a chest loot
// decision is pending — quitting there would silently destroy the item —
// and during combat, where the arrows are the move grid's navigation and
// esc would be one stray finger from ending the run.
func TestEscapeAlsoQuits(t *testing.T) {
	for _, state := range []engine.GameState{
		engine.StateExploring, engine.StateEncounter, engine.StateGameOver,
	} {
		m := New(testEngine())
		m.engine.State = state

		_, cmd := press(m, "esc")

		if cmd == nil {
			t.Errorf("state %v: esc did not request a quit", state)
		}
	}
}

func TestEscapeClosesTheBag(t *testing.T) {
	e := testEngine()
	stashTestItem(e)
	e.State = engine.StateInventory
	m := New(e)

	m, cmd := press(m, "esc")

	if cmd != nil {
		t.Error("esc in the bag must not request a quit")
	}
	if m.engine.State != engine.StateExploring {
		t.Errorf("state after esc = %v, want StateExploring", m.engine.State)
	}
	last := stripANSI(m.log[len(m.log)-1])
	if last != "You close your bag." {
		t.Errorf("last log line = %q, want the close-bag message", last)
	}
}

// The loot decision is mandatory: esc must neither quit nor resolve the
// pending item — the model stays exactly where it was.
func TestEscapeIsANoOpWhileLooting(t *testing.T) {
	m := chestModel(t)

	logLen := len(m.log)
	m, cmd := press(m, "esc")

	if cmd != nil {
		t.Error("esc in StateLoot must not request a quit")
	}
	if m.engine.State != engine.StateLoot {
		t.Errorf("state after esc = %v, want to stay in StateLoot", m.engine.State)
	}
	if m.engine.PendingLoot == nil {
		t.Error("esc consumed the pending loot")
	}
	if len(m.log) != logLen {
		t.Error("esc wrote to the log in StateLoot")
	}
}

// An unknown state must keep the badge visible and name itself: a
// silently empty chip would hide a broken FSM from the player.
func TestBadgeFallbackForUnknownStates(t *testing.T) {
	e := testEngine()
	e.State = engine.GameState(99) // not part of the FSM
	m := New(e)

	view := stripControls(stripANSI(m.View()))

	if !strings.Contains(view, "UNKNOWN") {
		t.Errorf("unknown state rendered without a visible badge:\n%s", view)
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
	m := New(testEngine()) // exploring: 4 actions
	if got := len(actionsFor(m.engine)); got != 4 {
		t.Fatalf("setup: exploring actions = %d, want 4", got)
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
	if m.cursor != 3 {
		t.Fatalf("left from cursor 0 = %d, want wraparound to 3", m.cursor)
	}
}

// In the bag, ←/→ walk the item rows with wraparound over the bag's
// size — the action bar cursor must stay untouched.
func TestBagCursorMovesWithWraparound(t *testing.T) {
	e := testEngine()
	stashTestItem(e)
	stashTestItem(e)
	e.State = engine.StateInventory
	m := New(e)

	m, _ = press(m, "right")
	if m.bagCursor != 1 || m.cursor != 0 {
		t.Fatalf("right: bagCursor = %d, bar cursor = %d, want 1 and untouched", m.bagCursor, m.cursor)
	}

	m, _ = press(m, "right")
	if m.bagCursor != 0 {
		t.Fatalf("right from the last item = %d, want wraparound to 0", m.bagCursor)
	}

	m, _ = press(m, "left")
	if m.bagCursor != 1 {
		t.Fatalf("left from 0 = %d, want wraparound to 1", m.bagCursor)
	}
}

// With an empty bag there is no row to select: the arrows are harmless.
func TestBagCursorStaysPutOnAnEmptyBag(t *testing.T) {
	e := testEngine()
	e.State = engine.StateInventory
	m := New(e)

	m, _ = press(m, "right")
	m, _ = press(m, "left")

	if m.bagCursor != 0 {
		t.Errorf("bag cursor = %d on an empty bag, want 0", m.bagCursor)
	}
}

// WEAR acts on the row the bag cursor highlights: selecting the second
// slot equips that item, and the displaced gear takes its old place in
// the bag. Dispatch also re-enters the bag on its first item.
func TestWearEquipsTheSelectedBagItem(t *testing.T) {
	e := testEngine()
	starter := entities.NewItemFromAsset(game.ItemAsset{
		ID: "rusty_blade", Name: "Rusty Blade", Slot: "weapon", BonusDMG: 1, Durability: 10,
	})
	e.Player.EquipItem(&starter)
	oak := entities.NewItemFromAsset(game.ItemAsset{
		ID: "oak_staff", Name: "Oak Staff", Slot: "weapon", BonusDMG: 3, Durability: 20,
	})
	iron := entities.NewItemFromAsset(game.ItemAsset{
		ID: "iron_staff", Name: "Iron Staff", Slot: "weapon", BonusDMG: 7, Durability: 30,
	})
	e.Player.StashItem(&oak)
	e.Player.StashItem(&iron)
	e.State = engine.StateInventory
	m := New(e)

	m, _ = press(m, "right") // selects bag[1]: Iron Staff
	if m.bagCursor != 1 {
		t.Fatalf("bag cursor after right = %d, want 1", m.bagCursor)
	}

	m, _ = press(m, "w")

	if m.engine.State != engine.StateInventory {
		t.Fatalf("state after wear = %v, want the bag to stay open", m.engine.State)
	}
	if m.engine.Player.Weapon != &iron {
		t.Errorf("weapon = %q, want the selected Iron Staff", m.engine.Player.Weapon.Name)
	}
	bag := m.engine.Player.Bag()
	if len(bag) != 2 {
		t.Fatalf("bag holds %d items after the swap, want 2", len(bag))
	}
	if bag[1] != &starter {
		t.Errorf("bag[1] = %q, want the displaced Rusty Blade", bag[1].Name)
	}
	if m.bagCursor != 0 {
		t.Errorf("bag cursor after dispatch = %d, want back on the first item", m.bagCursor)
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
	next, _ := m.Update(wideEnough)
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

// The purse sits on the hero panel's stats line: DMG/DEF/SPD grow a
// GOLD segment, and the panel keeps its four rows — the layout
// invariant (vitals directly above the shortcut bar) depends on that
// count. The gear list is not part of the panel: the equipped panel in
// the trade states is the only gear view.
func TestHeroPanelShowsGold(t *testing.T) {
	e := testEngine()
	e.Player.Gold = 123
	m := New(e)

	lines := viewLines(m.heroPanel())

	if len(lines) != 4 {
		t.Fatalf("hero panel has %d lines, want 4", len(lines))
	}
	stats := lines[3]
	for _, want := range []string{"DMG", "DEF", "SPD", "GOLD 123"} {
		if !strings.Contains(stats, want) {
			t.Errorf("stats line %q missing %q", stats, want)
		}
	}
	if joined := strings.Join(lines, "\n"); strings.Contains(joined, "Gear:") {
		t.Error("hero panel still carries the gear line")
	}
	if w := ansiLen(stats); w > mainColWidth {
		t.Errorf("stats line is %d wide, overflows the %d-col column: %q", w, mainColWidth, stats)
	}
}

// A chest payout (the engine's gold roll) reaches the log through the
// event's render path: source-aware, coins phrased.
func TestGoldGainedRendersInTheLog(t *testing.T) {
	m := New(testEngine())

	m.absorb(engine.NewGoldGained(120, engine.GoldSourceChest))

	last := stripANSI(m.log[len(m.log)-1])
	if last != "The chest holds 120 gold coins!" {
		t.Errorf("last log line = %q, want the chest payout", last)
	}
}

func encounterWithQueue(state engine.GameState) Model {
	e := testEngine()
	e.Player = entities.NewPlayer()
	e.CurrentEncounter = engine.Encounter{
		Type: engine.EncounterCombat,
		Monsters: []entities.Monster{
			entities.NewMonsterFromAsset(game.MonsterAsset{
				ID: "goblin", Name: "Goblin", HP: 30, Damage: 2, Speed: 10, Crit: 5, Difficulty: 3,
			}),
			entities.NewMonsterFromAsset(game.MonsterAsset{
				ID: "orc", Name: "Orc", HP: 50, Damage: 8, Speed: 8, Crit: 10, Difficulty: 5,
			}),
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

	// Every queued monster carries its stat line: the planning screen
	// should tell the player what it is up against, per enemy.
	for _, want := range []string{
		"⚔ 2 · ⛨ 0 · » 10 · ✦ 5% · ⬆ 3", // goblin
		"⚔ 8 · ⛨ 0 · » 8 · ✦ 10% · ⬆ 5", // orc
	} {
		if !strings.Contains(view, want) {
			t.Errorf("encounter view missing stat line %q:\n%s", want, view)
		}
	}

	// The stat lines are drawn in the left column: they must fit the
	// column's width instead of silently leaning on slot() to clip them.
	for i, line := range viewLines(m.encounterPanel()) {
		if w := ansiLen(line); w > mainColWidth {
			t.Errorf("encounter body line %d is %d wide, overflows the %d-col column:\n%q", i, w, mainColWidth, line)
		}
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

	// The engaged enemy gets its stat line too — crit chance and defense
	// are worth watching mid-fight — but only the front enemy's.
	front := "⚔ 2 · ⛨ 0 · » 10 · ✦ 5% · ⬆ 3"
	if !strings.Contains(view, front) {
		t.Errorf("combat view missing the front enemy's stat line %q:\n%s", front, view)
	}
	if strings.Contains(view, "⚔ 8") {
		t.Errorf("combat view leaked a queued enemy's stats:\n%s", view)
	}
	// Same column-width discipline as the encounter panel.
	for i, line := range viewLines(m.combatPanel()) {
		if w := ansiLen(line); w > mainColWidth {
			t.Errorf("combat body line %d is %d wide, overflows the %d-col column:\n%q", i, w, mainColWidth, line)
		}
	}
	// The hero's moves live in the 2×2 grid in the body now; the bar
	// shrank to the two combat-only actions.
	if !strings.Contains(view, "Basic Attack") || !strings.Contains(view, "◈ MOVES") {
		t.Errorf("combat view missing the move grid:\n%s", view)
	}
	if !strings.Contains(view, "[R] Flee") || !strings.Contains(view, "[Q] Quit") {
		t.Errorf("combat action bar incomplete:\n%s", view)
	}
	if strings.Contains(view, "[1] Basic Attack") {
		t.Errorf("the move chips still render on the bar:\n%s", view)
	}
}

// After death the body must recap the run: the floor/section where the
// climb ended and who the hero was. The header badge alone says nothing
// about the run.
func TestViewGameOverRecap(t *testing.T) {
	e := testEngine()
	e.State = engine.StateGameOver
	e.CurrentFloor = 7
	e.Player.Lvl = 3
	e.Player.Xp = 40
	m := New(e)

	view := stripControls(stripANSI(m.View()))

	for _, want := range []string{
		"YOU DIED",       // the banner stays
		"Floor 7",        // where it ended
		"Goblin Pits",    // the section, via SectionName
		"Level 3",        // hero level
		"40 XP gathered", // xp accumulated before death
	} {
		if !strings.Contains(view, want) {
			t.Errorf("game over view missing %q:\n%s", want, view)
		}
	}

	// The recap lives in the left column: no line may lean on slot()
	// to be clipped.
	for i, line := range viewLines(m.gameOverPanel()) {
		if w := ansiLen(line); w > mainColWidth {
			t.Errorf("game over body line %d is %d wide, overflows the %d-col column:\n%q", i, w, mainColWidth, line)
		}
	}
}

// The defeat event carries the run's final stats on its way through the
// log: dispatch must capture them so the recap can quote what the engine
// recorded, not guess.
func TestRecapIsCapturedFromTheDefeatEvent(t *testing.T) {
	m := New(testEngine())
	m.absorb(engine.NewHeroDefeatedWithStats(7, 3, 7))

	if m.recap == nil {
		t.Fatal("defeat event with stats stored no recap")
	}
	if m.recap.level != 3 {
		t.Errorf("recap level = %d, want 3", m.recap.level)
	}
	if m.recap.kills != 7 || !m.recap.hasKills {
		t.Errorf("recap kills = %d/%v, want 7/captured", m.recap.kills, m.recap.hasKills)
	}
}

// A legacy defeat event has no stats at all: capturing it must not
// fabricate a recap — the panel falls back to the engine's final state.
func TestLegacyDefeatStoresNoRecap(t *testing.T) {
	m := New(testEngine())
	m.absorb(engine.NewHeroDefeated())

	if m.recap != nil {
		t.Errorf("legacy defeat stored a recap %+v, want nil", m.recap)
	}
}

// The game-over recap must quote the captured defeat stats, kills
// included, e.g. "Level 3 · 7 kills · 40 XP gathered".
func TestViewGameOverRecapShowsTheKillCount(t *testing.T) {
	e := testEngine()
	e.State = engine.StateGameOver
	e.CurrentFloor = 7
	e.Player.Lvl = 2 // stale on purpose: the event is the source of truth
	e.Player.Xp = 40
	m := New(e)
	m.absorb(engine.NewHeroDefeatedWithStats(7, 3, 7))

	view := stripControls(stripANSI(m.View()))

	if !strings.Contains(view, "Level 3 · 7 kills · 40 XP gathered") {
		t.Errorf("recap missing the kills line:\n%s", view)
	}

	// Same column discipline as the rest of the panel.
	for i, line := range viewLines(m.gameOverPanel()) {
		if w := ansiLen(line); w > mainColWidth {
			t.Errorf("game over body line %d is %d wide, overflows the %d-col column:\n%q", i, w, mainColWidth, line)
		}
	}
}

// The recap must stay correct for a hero that died without ever killing
// anything (kills=0 shows 0 kills), and an older engine's event without
// a kills key must omit the segment rather than print a zero.
func TestViewGameOverRecapHandlesMissingAndZeroKills(t *testing.T) {
	withKills := func(ev engine.Event) Model {
		e := testEngine()
		e.State = engine.StateGameOver
		e.Player.Xp = 40
		m := New(e)
		m.absorb(ev)
		return m
	}

	t.Run("kills 0 is shown", func(t *testing.T) {
		view := stripControls(stripANSI(withKills(
			engine.NewHeroDefeatedWithStats(1, 1, 0),
		).View()))
		if !strings.Contains(view, "Level 1 · 0 kills · 40 XP gathered") {
			t.Errorf("zero kills recapped wrong:\n%s", view)
		}
	})

	t.Run("event without a kills key omits the segment", func(t *testing.T) {
		view := stripControls(stripANSI(withKills(
			engine.Event{Type: engine.EventHeroDefeated, Data: map[string]any{
				"floor": 1, "level": 2,
			}},
		).View()))
		if strings.Contains(view, "kills") {
			t.Errorf("recap printed a kills segment with no kills data:\n%s", view)
		}
		if !strings.Contains(view, "Level 2 · 40 XP gathered") {
			t.Errorf("recap without kills data lost the level line:\n%s", view)
		}
	})

	t.Run("event with malformed stats falls back to the engine", func(t *testing.T) {
		view := stripControls(stripANSI(withKills(
			engine.Event{Type: engine.EventHeroDefeated, Data: map[string]any{
				"level": "two", "floor": 1, "kills": 5,
			}},
		).View()))
		// level typed wrong means no trustable stats: fall back entirely.
		if strings.Contains(view, "kills") {
			t.Errorf("recap trusted a malformed event:\n%s", view)
		}
	})
}
