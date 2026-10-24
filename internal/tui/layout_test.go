package tui

import (
	"fmt"
	"strings"
	"testing"

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

	gameOver := func() Model {
		e := testEngine()
		e.State = engine.StateGameOver
		return New(e)
	}

	// Item on the hero, so the gear line reaches its realistic max width.
	geared := func() Model {
		e := testEngine()
		item := entities.NewItemFromAsset(game.ItemAsset{
			ID: "wooden_sword", Name: "Espada de Madeira", Slot: "weapon",
			BonusDMG: 5, Durability: 50,
		})
		e.Player.EquipItem(&item)
		return New(e)
	}

	return map[string]Model{
		"exploring":  New(testEngine()),
		"geared":     geared(),
		"encounter":  encounter(engine.StateEncounter),
		"combat":     combat,
		"game over":  gameOver(),
		"full queue": fullQueue(),
		"long log":   longLog(),
	}
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
			// heroHeight is the panel's line count: HERO + Lv + HP + stats + gear.
			t.Errorf("%s: hero panel spans %d rows above the bar, want %d", name, got, want)
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
