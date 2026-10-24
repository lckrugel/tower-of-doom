package tui

import (
	"slices"
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
)

func testEngine() *engine.Engine {
	return engine.NewEngine(&game.Registry{
		Monsters: map[string]game.MonsterAsset{
			"goblin": {ID: "goblin", Name: "Goblin", Difficulty: 3, HP: 30, Damage: 2, Speed: 10, Crit: 5},
		},
		Items: map[string]game.ItemAsset{},
		Sections: []game.SectionAsset{{
			ID: "pits", Name: "Goblin Pits", MinFloor: 1, MaxFloor: -1,
			Spawns: []game.SpawnEntry{{MonsterID: "goblin", Weight: 1}},
		}},
	})
}

func keysOf(actions []action) []string {
	keys := make([]string, 0, len(actions))
	for _, a := range actions {
		keys = append(keys, a.key)
	}
	return keys
}

func keysEqual(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestActionsPerState(t *testing.T) {
	tests := []struct {
		name     string
		state    engine.GameState
		wantKeys []string
	}{
		{"exploring", engine.StateExploring, []string{"e", "u", "q"}},
		{"encounter", engine.StateEncounter, []string{"f", "s", "q"}},
		{"combat", engine.StateCombat, []string{"a", "r", "q"}},
		{"game over", engine.StateGameOver, []string{"q"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := testEngine()
			e.State = tt.state

			actions := actionsFor(e)

			if !keysEqual(keysOf(actions), tt.wantKeys...) {
				t.Errorf("keys = %v, want %v", keysOf(actions), tt.wantKeys)
			}
			for _, a := range actions {
				if a.cmd != engine.CmdAscend && !a.enabled {
					t.Errorf("action %q is unexpectedly disabled", a.key)
				}
			}
		})
	}
}

// No shortcut may change meaning across states: combat muscle memory
// (spamming attack) must never leak into a different command.
func TestShortcutsAreGloballyUnique(t *testing.T) {
	byKey := make(map[string]engine.CommandType)
	states := []engine.GameState{
		engine.StateExploring, engine.StateEncounter, engine.StateCombat, engine.StateGameOver,
	}
	for _, state := range states {
		e := testEngine()
		e.State = state
		for _, a := range actionsFor(e) {
			if prev, dup := byKey[a.key]; dup && prev != a.cmd {
				t.Errorf("state %v: key %q maps to both %v and %v", state, a.key, prev, a.cmd)
			}
			byKey[a.key] = a.cmd
		}
	}
}

// Ascending is unlocked by combat victories only; before that the action
// renders disabled. Dispatching it anyway still asks the engine, which
// explains the block in the event log.
func TestAscendEnabledFollowsCanAscend(t *testing.T) {
	e := testEngine()
	e.State = engine.StateExploring

	locked := actionsFor(e)[slices.IndexFunc(actionsFor(e), func(a action) bool { return a.cmd == engine.CmdAscend })]
	if locked.enabled {
		t.Error("ascend enabled without a floor clear")
	}

	e.CanAscend = true
	unlocked := actionsFor(e)[slices.IndexFunc(actionsFor(e), func(a action) bool { return a.cmd == engine.CmdAscend })]
	if !unlocked.enabled {
		t.Error("ascend still disabled after a victory")
	}
}
