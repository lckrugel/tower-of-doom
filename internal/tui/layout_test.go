package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lckrugel/tower-of-doom/internal/game"

	// MAX_ENCOUNTER_SIZE drives the body slot height in the layout tests.
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// viewModels builds a Model per game state, as close to real play as
// possible (encounters populated, partial HP damage in combat).
func viewModels() map[string]Model {
	encounter := func(state engine.GameState) Model {
		e := testEngine()
		e.Player = entities.NewPlayer()
		e.CurrentEncounter = engine.Encounter{
			Type: engine.EncounterCombat,
			Monsters: []entities.Monster{
				entities.NewMonsterFromAsset(game.MonsterAsset{ID: "goblin", Name: "Goblin", HP: 30, Damage: 2, Speed: 10}),
				entities.NewMonsterFromAsset(game.MonsterAsset{ID: "orc", Name: "Orc", HP: 50, Damage: 8, Speed: 8}),
			},
		}
		e.State = state
		return New(e)
	}

	combat := encounter(engine.StateCombat)
	combat.engine.CurrentEncounter.Monsters[0].TakeDmg(10)
	// The combat screen mid-fight: the mirror holds statuses on both
	// sides and a cooling move, as the event replay would have left it.
	combat.mirror.heroCooldowns["basic_attack"] = 1
	combat.mirror.heroStatuses = []combatStatus{
		{name: "Guard", kind: "buff", turns: 2},
		{name: "Poison", kind: "debuff", turns: 1},
	}
	combat.mirror.enemyStatuses = []combatStatus{
		{name: "Poison", kind: "debuff", turns: 2},
		{name: "Weaken", kind: "debuff", turns: 3},
	}
	combat.gridCursor = 1

	gameOver := func() Model {
		e := testEngine()
		e.State = engine.StateGameOver
		return New(e)
	}

	// Item on the hero, so the stats line reflects equipped gear (and a
	// worn item never disturbs the frame).
	geared := func() Model {
		e := testEngine()
		item := entities.NewItemFromAsset(game.ItemAsset{
			ID: "wooden_sword", Name: "Espada de Madeira", Slot: "weapon",
			BonusDMG: 5, Durability: 50,
		})
		e.Player.EquipItem(&item)
		return New(e)
	}

	// Chest loot pending: the decision screen over a real item.
	loot := func() Model {
		e := testEngine()
		item := entities.NewItemFromAsset(game.ItemAsset{
			ID: "wooden_sword", Name: "Espada de Madeira", Slot: "weapon",
			BonusDMG: 5, Durability: 50,
		})
		e.PendingLoot = &item
		e.State = engine.StateLoot
		return New(e)
	}

	// Bag open with a few stashed items: the selection screen.
	inventory := func() Model {
		e := testEngine()
		stone := entities.NewItemFromAsset(game.ItemAsset{
			ID: "power_stone", Name: "Pedra do Poder", Slot: "ring", BonusDEF: 2, Durability: 30,
		})
		blade := entities.NewItemFromAsset(game.ItemAsset{
			ID: "oak_staff", Name: "Cajado de Carvalho", Slot: "weapon", BonusDMG: 7, Durability: 40,
		})
		cape := entities.NewItemFromAsset(game.ItemAsset{
			ID: "travel_cloak", Name: "Capa de Viagem", Slot: "body_armor", BonusHP: 5, Durability: 60,
		})
		e.Player.StashItem(&stone)
		e.Player.StashItem(&blade)
		e.Player.StashItem(&cape)
		e.State = engine.StateInventory
		return New(e)
	}

	// The merchant's shop: three offers and a couple of bag items over a
	// real purse — the widest selectable list the game shows (stock rows
	// first, then the bag; the cursor opens on the first offer).
	merchant := func() Model {
		e := testEngine()
		e.Player.Gold = 120
		e.MerchantStock = append(e.MerchantStock,
			shopItem("Offer A"), shopItem("Offer B"), shopItem("Offer C"))
		stone := entities.NewItemFromAsset(game.ItemAsset{
			ID: "power_stone", Name: "Pedra do Poder", Slot: "ring", BonusDEF: 2, Durability: 30,
		})
		cape := entities.NewItemFromAsset(game.ItemAsset{
			ID: "travel_cloak", Name: "Capa de Viagem", Slot: "body_armor", BonusHP: 5, Durability: 60,
		})
		e.Player.StashItem(&stone)
		e.Player.StashItem(&cape)
		e.State = engine.StateMerchant
		return New(e)
	}

	// The boss challenge unlocked on the exploring bar: the [G] chip lit.
	bossUnlocked := func() Model {
		e := testEngine()
		e.BossUnlocked = true
		return New(e)
	}

	// The boss challenge mid-fight: the same combat screen over a
	// boss-flagged encounter — the ☠ BOSS badge joins the title row.
	bossCombat := encounter(engine.StateCombat)
	bossCombat.engine.CurrentEncounter.Boss = true

	// The shrine's choosing step (StateLearn): BOTH offers pend as
	// dossiers over the hero's two learned moves (at the cap). The
	// cursor picks the offer [L] Learn acts on.
	shrineAt := func(cursor int) Model {
		e := testEngine()
		learnTestMove(e, "guard_strike", "Guard Strike", 2)
		learnTestMove(e, "heavy_slash", "Heavy Slash", 2)
		e.PendingMoves = []entities.Move{
			{ID: "power_slash", Name: "Power Slash", Tier: 2, DmgModifier: 175, Weight: 10},
			{ID: "venom_shot", Name: "Venom Shot", Tier: 1, DmgModifier: 75, Weight: 10,
				Status: entities.MoveStatus{ID: "poison", Chance: 60, Target: entities.TargetEnemy}},
		}
		e.State = engine.StateLearn
		m := New(e)
		m.bagCursor = cursor
		return m
	}

	// The shrine's forgetting step: the choice recorded in ChosenMove,
	// the two learned moves as selectable rows (the forget cursor walked
	// to the second row).
	forgetAt := func(cursor int) Model {
		m := shrineAt(0)
		chosen := m.engine.PendingMoves[0]
		m.engine.ChosenMove = &chosen
		m.bagCursor = cursor
		return m
	}

	return map[string]Model{
		"exploring":     New(testEngine()),
		"geared":        geared(),
		"encounter":     encounter(engine.StateEncounter),
		"combat":        combat,
		"full grid":     fullGrid(),
		"loot":          loot(),
		"inventory":     inventory(),
		"merchant":      merchant(),
		"learn":         learnModel(),
		"game over":     gameOver(),
		"full queue":    fullQueue(),
		"long log":      longLog(),
		"boss unlocked": bossUnlocked(),
		"boss combat":   bossCombat,
		"shrine":        shrineAt(0),
		"forget prompt": forgetAt(1),
	}
}

// learnModel is the choosing screen's under-cap case the frame must
// survive: ONE offer with a modded move name wider than any shipped one
// and every tag flavor — the dossier truncates inside the column.
func learnModel() Model {
	e := testEngine()
	learnTestMove(e, "guard_strike", "Guard Strike", 2)
	e.PendingMoves = []entities.Move{{
		ID: "super_calibrating_excalibur_strike", Name: "Super Calibrating Excalibur Strike",
		Tier: 3, DmgModifier: 300, CritBonus: 25, Cooldown: 4, Weight: 10,
		Status: entities.MoveStatus{ID: "poison", Chance: 60, Target: entities.TargetEnemy},
	}}
	e.State = engine.StateLearn
	return New(e)
}

// fullGrid is the combat screen's worst case: a 4-move set with the
// longest shipped move name and every tag flavor, a cooling cell, the
// cursor resting on a later cell and statuses up on both sides.
func fullGrid() Model {
	e := combatEngine()
	learnMove(e, envenom(0)) // modifier + status tag
	learnMove(e, heavy())    // modifier only — cooling below
	equipWeapon(e,
		entities.Move{ID: "assassins_strike", Name: "Assassin's Strike", Tier: 3, DmgModifier: 110, CritBonus: 35, Cooldown: 3, Weight: 10},
		entities.Move{ID: "guard_strike", Name: "Guard Strike", Tier: 1, DmgModifier: 75, Weight: 10,
			Status: entities.MoveStatus{ID: "guard", Chance: 100, Target: entities.TargetSelf}},
	)
	m := New(e)
	e.State = engine.StateCombat
	e.CurrentEncounter = engine.Encounter{
		Type:     engine.EncounterCombat,
		Monsters: []entities.Monster{goblinMonster()},
	}
	m.mirror.heroCooldowns["heavy_slash"] = 2
	m.mirror.heroStatuses = []combatStatus{{name: "Guard", kind: "buff", turns: 2}}
	m.mirror.enemyStatuses = []combatStatus{
		{name: "Poison", kind: "debuff", turns: 2},
		{name: "Weaken", kind: "debuff", turns: 3},
	}
	m.gridCursor = 3
	return m
}

// fullQueue stacks the maximum allowed enemies — the fattest body the
// game can produce.
func fullQueue() Model {
	e := testEngine()
	e.Player = entities.NewPlayer()
	monsters := make([]entities.Monster, engine.MAX_ENCOUNTER_SIZE)
	for i := range monsters {
		monsters[i] = entities.NewMonsterFromAsset(game.MonsterAsset{
			ID: "goblin", Name: "Goblin", Difficulty: 3, HP: 30, Damage: 2, Speed: 10,
		})
	}
	e.CurrentEncounter = engine.Encounter{Type: engine.EncounterCombat, Monsters: monsters}
	e.State = engine.StateCombat
	return New(e)
}

// The combat screen always fits the body slot — never leaning on slot()
// to clip. Worst case: the enemy block's 6 rows (title, name/HP, stats,
// the TWO capped status rows, the waiting counter) plus the grid's 5
// (title + four cell lines) = 11 of the 16 body rows, and every line
// inside the column. The fixtures cover the fattest screens: statuses
// on both sides over a full 2×2 grid ("full grid"), the deepest queue
// ("full queue"), and the enriched mid-fight screen ("combat").
func TestCombatBodyFitsTheBodySlot(t *testing.T) {
	for _, name := range []string{"combat", "full grid", "full queue"} {
		m := viewModels()[name]

		lines := viewLines(m.combatPanel())

		if len(lines) > bodyHeight {
			t.Errorf("%s: combat body has %d lines, want at most %d", name, len(lines), bodyHeight)
		}
		for i, line := range lines {
			if w := ansiLen(line); w > mainColWidth {
				t.Errorf("%s: combat body line %d is %d wide, overflows the %d-col column:\n%q",
					name, i, w, mainColWidth, line)
			}
		}
	}
}

// The grid's cells are exactly half the column wide, so the two cells
// of a row always total the column — a modded move with a longer name
// than any shipped one ("Assassin's Strike", 17 runes) truncates inside
// its cell instead of stretching the frame.
func TestGridCellsAreExactlyHalfTheColumn(t *testing.T) {
	m := viewModels()["full grid"]

	for i, line := range viewLines(m.moveGrid()) {
		if i == 0 {
			continue // the title row is not a cell row
		}
		if w := ansiLen(line); w != mainColWidth {
			t.Errorf("grid line %d is %d wide, want exactly %d:\n%q", i, w, mainColWidth, line)
		}
	}
}

// The combat screen shows the whole picture: the enemy block with its
// status chips, the grid with every tag flavor and the cooling marker,
// and the bar as the two-key legend.
func TestViewFullGridShowsEveryElement(t *testing.T) {
	m := viewModels()["full grid"]

	view := stripControls(stripANSI(m.View()))

	for _, want := range []string{
		"◈ MOVES",
		"Envenom", "Heavy Slash", "Assassin's Strike", "Guard Strike",
		"100% ☠poison 100%", // envenom's tags
		"(2 turns)",         // the cooling Heavy Slash
		"110% ✦+35",         // the Assassin's Strike crit tag
		"75% ☠guard 100%",   // Guard Strike's self-buff tag
		"Poison(2)",         // the enemy's chips
		"Weaken(3)",
		"Guard(2)", // the hero panel's chip
		"[R] Flee", "[Q] Quit",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("full-grid view missing %q:\n%s", want, view)
		}
	}
}

// The shrine panel always fits the body slot — never leaning on slot()
// to clip. Worst case: the dossier over the two forget rows, with a
// modded move name wider than any shipped one ("learn" fixture).
func TestShrineBodyFitsTheBodySlot(t *testing.T) {
	for _, name := range []string{"shrine", "forget prompt", "learn"} {
		m := viewModels()[name]

		lines := viewLines(m.shrinePanel())

		if len(lines) > bodyHeight {
			t.Errorf("%s: shrine body has %d lines, want at most %d", name, len(lines), bodyHeight)
		}
		for i, line := range lines {
			if w := ansiLen(line); w > mainColWidth {
				t.Errorf("%s: shrine body line %d is %d wide, overflows the %d-col column:\n%q",
					name, i, w, mainColWidth, line)
			}
		}
	}
}

// longLog fills the event log to its cap.
func longLog() Model {
	m := New(testEngine())
	for i := range maxLogLines {
		m.log = append(m.log, fmt.Sprintf("log line %d", i))
	}
	return m
}

// viewLines is the plain-text view split into lines, ready for
// width/height measuring.
func viewLines(v string) []string {
	return strings.Split(stripControls(stripANSI(v)), "\n")
}

// The window must occupy exactly one size in every state; content-driven
// resize is jarring mid-fight.
func TestViewHasAConstantFixedSize(t *testing.T) {
	models := viewModels()

	var height, width int
	for name, m := range models {
		lines := viewLines(m.View())

		if height == 0 {
			height, width = len(lines), ansiLen(lines[0])
		}
		if len(lines) != height {
			t.Errorf("%s: %d lines, want %d", name, len(lines), height)
		}
		for i, line := range lines {
			if l := ansiLen(line); l != width {
				t.Errorf("%s: line %d has width %d, want %d (%q)", name, i, l, width, line)
			}
		}
	}
}

// ansiLen measures the rune width of a line that may contain ANSI codes
// (after stripANSI there should be none left).
func ansiLen(s string) int {
	return len([]rune(s))
}

// The vitals must sit right above the shortcut bar in every state —
// that anchor is what stops the hero panel from wandering between
// encounter and combat as the enemy list changes.
func TestHeroStatsAnchorAboveTheActionBar(t *testing.T) {
	for name, m := range viewModels() {
		lines := viewLines(m.View())

		barRow, heroRow := -1, -1
		for i, line := range lines {
			if strings.Contains(line, "[Q] Quit") {
				barRow = i
			}
			// First row of the hero panel; the log readout shares that row
			// on the right, so exclude it.
			if strings.Contains(line, "HERO") && !strings.Contains(line, "Welcome") {
				heroRow = i
			}
		}
		if barRow < 0 || heroRow < 0 {
			t.Fatalf("%s: missing hero panel or bar:\n%s", name, strings.Join(lines, "\n"))
		}
		if got, want := barRow-heroRow, heroHeight; got != want {
			// heroHeight is the panel's line count: HERO + Lv + HP + stats.
			t.Errorf("%s: hero panel spans %d rows above the bar, want %d", name, got, want)
		}
	}
}

// The '?' overlay replaces the canvas but keeps the window frame's
// exact geometry: same fixed size in every dimension — the terminal
// never reflows for it.
func TestOverlayKeepsTheConstantFrame(t *testing.T) {
	m := New(testEngine())
	m.overlay = true

	lines := viewLines(m.View())

	if len(lines) != contentHeight+2 {
		t.Errorf("overlay renders %d lines, want the full frame of %d", len(lines), contentHeight+2)
	}
	for i, line := range lines {
		if w := ansiLen(line); w != contentWidth+4 {
			t.Errorf("overlay line %d is %d wide, want %d", i, w, contentWidth+4)
		}
	}
}

// The event log lives in the right half: its lines must start beyond the
// middle of the frame, clear of the game area on the left.
func TestLogPanelOccupiesTheRightHalf(t *testing.T) {
	m := New(testEngine())
	lines := viewLines(m.View())

	for _, line := range lines {
		if idx := strings.Index(line, "Welcome to the"); idx >= 0 {
			if idx < contentWidth/2 {
				t.Errorf("log line starts at column %d, want ≥ %d:\n%s", idx, contentWidth/2, line)
			}
			return
		}
	}
	t.Fatal("log greeting not found in the view")
}

// Without a WindowSizeMsg the model can't know the terminal size: the
// view must render the full canvas regardless (first frame / tests).
func TestViewAtUnknownSizeRendersTheFullCanvas(t *testing.T) {
	models := viewModels()

	for name, m := range models {
		lines := viewLines(m.View())
		if len(lines) != contentHeight+2 {
			t.Fatalf("%s: %d lines, want the full frame of %d", name, len(lines), contentHeight+2)
		}
		for i, line := range lines {
			if w := ansiLen(line); w != contentWidth+4 {
				t.Errorf("%s: line %d is %d wide, want %d", name, i, w, contentWidth+4)
			}
		}
	}
}

// Sending the size must record it without changing anything the view
// renders: a roomy terminal gets exactly the same fixed frame.
func TestWindowSizeMsgIsStoredWithoutRedrawing(t *testing.T) {
	models := viewModels()

	for name, m := range models {
		next, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
		sized := modelOf(next, t)

		if sized.width != 200 || sized.height != 60 {
			t.Errorf("%s: size stored as %dx%d, want 200x60", name, sized.width, sized.height)
		}
		if got, want := sized.View(), m.View(); got != want {
			t.Errorf("%s: view changed after the size message:\n%q", name, got)
		}
	}
}

// A terminal that can't fit the canvas gets a bare fallback screen: the
// warning plus nothing else from the game UI (no frame, no action bar).
func TestViewOnATooSmallTerminalIsABareFallback(t *testing.T) {
	tests := []struct {
		name  string
		width int
		// 0 means "no height known yet" is impossible via WindowSizeMsg;
		// every case below has both dimensions.
		height int
	}{
		{"narrow", 80, 40},
		{"short", 120, 20},
		{"way too small", 40, 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(testEngine())
			next, _ := m.Update(tea.WindowSizeMsg{Width: tt.width, Height: tt.height})
			model := modelOf(next, t)
			if !model.tooSmall() {
				t.Fatalf("%dx%d not detected as too small", tt.width, tt.height)
			}

			view := stripANSI(model.View())
			want := fmt.Sprintf("Terminal too small — needs %dx%d", minWidth, minHeight)
			if !strings.Contains(view, want) {
				t.Errorf("small-terminal view missing %q:\n%s", want, view)
			}
			for _, leaked := range []string{"[Q] Quit", "HERO", "Tower of Doom", "⚔ ENEMIES"} {
				if strings.Contains(view, leaked) {
					t.Errorf("small-terminal view must not contain %q:\n%s", leaked, view)
				}
			}
			for i, line := range strings.Split(view, "\n") {
				if w := ansiLen(line); w > tt.width {
					t.Errorf("fallback line %d is %d wide, overflows the %d-col terminal: %q", i, w, tt.width, line)
				}
			}
		})
	}
}

// Exactly at the threshold the canvas must still fit.
func TestMinimumSizeRendersTheFullCanvas(t *testing.T) {
	m := New(testEngine())
	next, _ := m.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})
	model := modelOf(next, t)
	if model.tooSmall() {
		t.Errorf("%dx%d wrongly flagged as too small", minWidth, minHeight)
	}
	if got, want := model.View(), New(testEngine()).View(); got != want {
		t.Errorf("view at the minimum size differs from the unsized canvas:\n%q", got)
	}
}

// The action bar carries the instructions; it must sit on the bottom row
// of the frame in every state — growing logs or queues must push blank
// space, never the controls.
func TestActionBarStaysOnTheBottomRow(t *testing.T) {
	bottomRows := make(map[string]int)
	for name, m := range viewModels() {
		lines := viewLines(m.View())

		bottomRow := -1
		for i, line := range lines {
			if strings.Contains(line, "[Q] Quit") {
				bottomRow = i
				break
			}
		}
		if bottomRow < 0 {
			t.Fatalf("%s: no action bar in view:\n%s", name, strings.Join(lines, "\n"))
		}
		if want := len(lines) - 2; bottomRow != want { // above the bottom border
			t.Errorf("%s: action bar on row %d, want the bottom row %d", name, bottomRow, want)
		}

		// All states share the same fixed height, so every bar must land
		// on the same index.
		bottomRows[name] = bottomRow
	}
	for name, row := range bottomRows {
		if row != bottomRows["exploring"] {
			t.Errorf("%s: action bar on row %d, want %d (exploring's row)", name, row, bottomRows["exploring"])
		}
	}
}
