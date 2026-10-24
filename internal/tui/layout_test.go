package tui

import (
	"strings"
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
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
		"exploring": New(testEngine()),
		"geared":    geared(),
		"encounter": encounter(engine.StateEncounter),
		"combat":    combat,
		"game over": gameOver(),
	}
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
