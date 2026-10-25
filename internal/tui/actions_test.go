package tui

import (
	"slices"
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

func testEngine() *engine.Engine {
	return engine.NewEngine(&game.Registry{
		Monsters: map[string]game.MonsterAsset{
			"goblin": {ID: "goblin", Name: "Goblin", Difficulty: 3, HP: 30, Damage: 2, Speed: 10, Crit: 5},
		},
		Items: map[string]game.ItemAsset{},
		Sections: []game.SectionAsset{{
			ID: "pits", Name: "Goblin Pits", MinFloor: 1, MaxFloor: -1,
			// A real budget: the flow tests resolve actual rounds, which
			// needs an encounter that actually spawns monsters.
			BaseDifficulty: 9, DifficultyPerFloor: 3,
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

// stashTestItem puts a fresh item directly into the hero's bag, so state
// fixtures can render the bag actions enabled.
func stashTestItem(e *engine.Engine) *entities.Item {
	item := entities.NewItemFromAsset(game.ItemAsset{
		ID: "spare_charm", Name: "Spare Charm", Slot: "ring", BonusDEF: 1, Durability: 10,
	})
	e.Player.StashItem(&item)
	return &item
}

func TestActionsPerState(t *testing.T) {
	tests := []struct {
		name      string
		state     engine.GameState
		withBag   bool
		withStock bool // the shop's offers (BUY needs one to be enabled)
		wantKeys  []string
	}{
		{"exploring", engine.StateExploring, false, false, []string{"e", "u", "g", "i", "q"}},
		{"encounter", engine.StateEncounter, false, false, []string{"f", "s", "q"}},
		{"combat", engine.StateCombat, false, false, []string{"r", "q"}}, // moves live in the grid
		{"loot", engine.StateLoot, false, false, []string{"w", "b", "d", "q"}},
		{"inventory", engine.StateInventory, true, false, []string{"w", "x", "c", "q"}},
		{"merchant", engine.StateMerchant, true, true, []string{"p", "v", "l", "q"}},
		{"learn", engine.StateLearn, false, false, []string{"t", "n", "q"}},
		{"game over", engine.StateGameOver, false, false, []string{"q"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := testEngine()
			if tt.withBag {
				// An empty bag renders WEAR/DROP disabled; the enabled
				// invariant below needs a stocked bag.
				stashTestItem(e)
			}
			if tt.withStock {
				// SELL needs the bag (stashed above); BUY needs offers.
				e.MerchantStock = append(e.MerchantStock,
					shopItem("Offer A"), shopItem("Offer B"), shopItem("Offer C"))
			}
			e.State = tt.state

			actions := actionsFor(e)

			if !keysEqual(keysOf(actions), tt.wantKeys...) {
				t.Errorf("keys = %v, want %v", keysOf(actions), tt.wantKeys)
			}
			for _, a := range actions {
				// ASCEND and BOSS render disabled while locked — every
				// other action in the matrix must be enabled here.
				if a.cmd != engine.CmdAscend && a.cmd != engine.CmdChallengeBoss && !a.enabled {
					t.Errorf("action %q is unexpectedly disabled", a.key)
				}
			}
		})
	}
}

// The BOSS chip mirrors the ASCEND precedent: disabled while the
// challenge is locked, enabled once a final-floor victory unlocked it.
// Dispatching it anyway still reaches the engine, which explains the
// block in the log.
func TestBossChipFollowsBossUnlocked(t *testing.T) {
	e := testEngine()
	e.State = engine.StateExploring

	bossChip := func(a action) bool { return a.cmd == engine.CmdChallengeBoss }
	locked := actionsFor(e)[slices.IndexFunc(actionsFor(e), bossChip)]
	if locked.enabled {
		t.Error("boss chip enabled without an unlock")
	}

	e.BossUnlocked = true
	unlocked := actionsFor(e)[slices.IndexFunc(actionsFor(e), bossChip)]
	if !unlocked.enabled {
		t.Error("boss chip still disabled after the unlock")
	}
}

// An empty bag can't wear or drop anything: both actions render disabled.
// Dispatching them anyway still reaches the engine, which explains the
// rejection as a log message.
func TestBagActionsAreDisabledOnAnEmptyBag(t *testing.T) {
	e := testEngine()
	e.State = engine.StateInventory

	for _, a := range actionsFor(e) {
		switch a.cmd {
		case engine.CmdWear, engine.CmdDrop:
			if a.enabled {
				t.Errorf("action %q enabled with an empty bag", a.key)
			}
		default:
			if !a.enabled {
				t.Errorf("action %q unexpectedly disabled", a.key)
			}
		}
	}
}

// No shortcut may change meaning across states: combat muscle memory
// (spamming attack) must never leak into a different command.
func TestShortcutsAreGloballyUnique(t *testing.T) {
	byKey := make(map[string]engine.CommandType)
	states := []engine.GameState{
		engine.StateExploring, engine.StateEncounter, engine.StateCombat, engine.StateGameOver,
		engine.StateLoot, engine.StateInventory, engine.StateMerchant, engine.StateLearn,
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

// Only the row-indexed commands dispatch with the cursor: the bag
// commands (WEAR/DROP) index the hero bag; at the merchant, BUY indexes
// the stock and SELL the bag. The combat move chips (CmdMove) instead
// carry their own move position — every one of them is an index user.
func TestIndexedCommandsUseTheRowCursor(t *testing.T) {
	states := []engine.GameState{
		engine.StateExploring, engine.StateEncounter, engine.StateCombat,
		engine.StateGameOver, engine.StateLoot, engine.StateInventory,
		engine.StateMerchant,
	}
	for _, state := range states {
		e := testEngine()
		e.State = state
		for _, a := range actionsFor(e) {
			want := a.cmd == engine.CmdWear || a.cmd == engine.CmdDrop ||
				a.cmd == engine.CmdBuy || a.cmd == engine.CmdSell ||
				a.cmd == engine.CmdForget || a.cmd == engine.CmdMove
			if a.usesIndex != want {
				t.Errorf("state %v: action %q usesIndex = %v, want %v", state, a.key, a.usesIndex, want)
			}
		}
	}
}

// The combat bar lists only Flee and Quit — the hero's moves moved into
// the 2×2 move grid in the body (see the grid tests in combat_test.go).
// The two bar-chip tests died with that redesign.

// Single keys 1-4 jump-dispatch the matching grid cell:
// Command{CmdMove, Index} — the cursor jumps along.
func TestNumberKeysDispatchMoves(t *testing.T) {
	e := testEngine()
	learnTestMove(e, "power_slash", "Power Slash", 0)
	learnTestMove(e, "heavy_blow", "Heavy Blow", 2)
	e.Process(engine.Command{Type: engine.CmdExplore})
	e.Process(engine.Command{Type: engine.CmdFight})
	m := New(e)

	m, _ = press(m, "2")

	if m.engine.State != engine.StateCombat {
		t.Fatalf("state after '2' = %v, want StateCombat", m.engine.State)
	}
	if m.gridCursor != 1 {
		t.Errorf("grid cursor after '2' = %d, want 1 (jump-select)", m.gridCursor)
	}
	// The dispatch locked the move (cooldown 2): a second press is a
	// blocked action, proving the cell really dispatched CmdMove.
	m, _ = press(m, "2")
	last := stripANSI(m.log[len(m.log)-1])
	if last != "Move on cooldown." {
		t.Errorf("second '2' logged %q, want the cooldown block", last)
	}
	if m.engine.MoveCooldown("heavy_blow") <= 0 {
		t.Error("heavy_blow was not locked by the dispatched move")
	}
}

// learnTestMove teaches the hero a hand-built move. Learned moves are
// runtime values — the registry is only consulted for monster move refs
// and status resolution, so no registration is needed here.
func learnTestMove(e *engine.Engine, id, name string, cooldown int) {
	e.Player.LearnMove(entities.Move{
		ID: id, Name: name, Tier: 1, DmgModifier: 100, Cooldown: cooldown, Weight: 10,
	})
}
