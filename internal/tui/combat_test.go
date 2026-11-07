package tui

import (
	"strings"
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// The combat mirror's tests. The engine keeps combat-scoped state (move
// cooldowns, active statuses) unexported — the mirror rebuilds it from
// the event stream. Most tests below drive a REAL engine through real
// rounds and assert the mirror against the engine's own observable truth
// (its exported MoveCooldown accessor and its log lines).

// statusAssets mirrors the shipped statuses.json — the fixture engines'
// registry lore (the '?' overlay and the chip explanations read it;
// strike-time resolution only ever references poison and guard).
var statusAssets = map[string]game.StatusAsset{
	"poison": {ID: "poison", Name: "Poison", Kind: "debuff",
		Effect: game.StatusEffectAsset{Type: "dot", Amount: 3}, Duration: 3},
	"guard": {ID: "guard", Name: "Guard", Kind: "buff",
		Effect: game.StatusEffectAsset{Type: "stat", Stat: "def", Amount: 5}, Duration: 2},
	"weaken": {ID: "weaken", Name: "Weaken", Kind: "debuff",
		Effect: game.StatusEffectAsset{Type: "stat", Stat: "dmg", Amount: -3}, Duration: 3},
	"slow": {ID: "slow", Name: "Slow", Kind: "debuff",
		Effect: game.StatusEffectAsset{Type: "stat", Stat: "spd", Amount: -3}, Duration: 3},
}

// combatEngine is a real engine whose registry carries the status assets,
// so status-carrying runtime moves resolve at strike time. The section's
// budget spawns four goblins — a deep enough queue for kill rounds.
func combatEngine() *engine.Engine {
	e := engine.NewEngine(&game.Registry{
		Monsters: map[string]game.MonsterAsset{
			"goblin": {ID: "goblin", Name: "Goblin", Difficulty: 3, HP: 30, Damage: 2, Speed: 10, Crit: 5},
		},
		Items:    map[string]game.ItemAsset{},
		Statuses: statusAssets,
		Sections: []game.SectionAsset{{
			ID: "pits", Name: "Goblin Pits", MinFloor: 1, MaxFloor: -1,
			BaseDifficulty: 12, DifficultyPerFloor: 3,
			Spawns: []game.SpawnEntry{{MonsterID: "goblin", Weight: 1}},
		}},
	})
	// The punch start (NewPlayer pre-learns it) would eat the learned
	// slot every fixture below assumes — the fixture hero is bare.
	e.Player.ForgetMove(entities.Punch().ID)
	return e
}

// envenom is a deterministic status move: 100% poison on the enemy.
func envenom(cooldown int) entities.Move {
	return entities.Move{
		ID: "envenom", Name: "Envenom", Tier: 1, DmgModifier: 100,
		Cooldown: cooldown, Weight: 10,
		Status: entities.MoveStatus{ID: "poison", Chance: 100, Target: entities.TargetEnemy},
	}
}

// jab is a plain spamming move: neutral damage, no strings attached.
func jab() entities.Move {
	return entities.Move{ID: "jab", Name: "Jab", Tier: 1, DmgModifier: 100, Weight: 10}
}

// learnMove teaches the hero a hand-built runtime move (the registry is
// only consulted for status resolution at strike time).
func learnMove(e *engine.Engine, move entities.Move) {
	if _, ok := e.Player.LearnMove(move); !ok {
		panic("test fixture: could not learn " + move.ID)
	}
}

// enterCombat drives a fresh model into a real fight through the keys:
// explore (a run's first explore is a forced combat encounter), fight.
func enterCombat(t *testing.T, e *engine.Engine) Model {
	t.Helper()
	m := New(e)
	m, _ = press(m, "e")
	if m.engine.State != engine.StateEncounter {
		t.Fatalf("setup: state after explore = %v, want StateEncounter", m.engine.State)
	}
	m, _ = press(m, "f")
	if m.engine.State != engine.StateCombat {
		t.Fatalf("setup: state after fight = %v, want StateCombat", m.engine.State)
	}
	return m
}

// useMove dispatches CmdMove through the model's real dispatch path —
// the same route the move grid's keys take.
func useMove(t *testing.T, m Model, index int) Model {
	t.Helper()
	next, _ := m.dispatch(action{key: "1", label: "move", cmd: engine.CmdMove, enabled: true, usesIndex: true, index: index})
	return modelOf(next, t)
}

// spitterMonster is a monster whose only move is a sure poison aimed at
// ITS enemy — the hero (mirrors the engine's spitter fixture). The HP is
// a parameter: a DMG-boosted fixture hero must never crit-kill it in the
// poison round (tests set CurrentHP = 1 to engineer the kill instead).
func spitterMonster(hp int) entities.Monster {
	move := envenom(0)
	move.ID, move.Weight = "spit_poison", 100
	return entities.NewMonsterFromAsset(
		game.MonsterAsset{ID: "spitter", Name: "Spitter", Difficulty: 2, HP: hp, Damage: 1, Speed: 10},
		move,
	)
}

// goblinMonster is the fixture goblin, built directly for hand-made queues.
func goblinMonster() entities.Monster {
	return entities.NewMonsterFromAsset(game.MonsterAsset{
		ID: "goblin", Name: "Goblin", Difficulty: 3, HP: 30, Damage: 2, Speed: 10, Crit: 5,
	})
}

// joinedLog is the full rendered log as plain text.
func joinedLog(m Model) string {
	lines := make([]string, 0, len(m.log))
	for _, l := range m.log {
		lines = append(lines, stripANSI(l))
	}
	return strings.Join(lines, "\n")
}

func TestCombatRenderingReadsLiveEngineView(t *testing.T) {
	e := combatEngine()
	learnMove(e, envenom(2))
	m := enterCombat(t, e)
	m = useMove(t, m, 0)
	view := stripANSI(m.View())
	for _, want := range []string{"Poison(2)", "(2 turns)", "DMG 1 · DEF 0 · SPD 10"} {
		if !strings.Contains(view, want) {
			t.Errorf("combat view missing engine snapshot value %q:\n%s", want, view)
		}
	}
}

// assertCooldownsEqual fails unless the mirror's picture of every combat
// move's cooldown matches the engine's own read-only accessor — the two
// must never drift apart across real rounds.

// Cooldowns lock on use and tick down as completed rounds pass — the
// mirror must agree with the engine's accessor after every single round.

// A blocked move (dispatched while cooling) is a message, not a round:
// neither the mirror nor the engine may tick.

// A RECOVER round (every move locked) still ticks: the enemy acts, the
// round completes, cooldowns count down without a hero lock being set.

// A killing blow ends the round early: no tick runs, but the used move
// still locks at its full cooldown (engine appendEnemyDefeat).

// Status chips appear when the event lands, count down as completed
// rounds pass, and vanish when the engine reports the expiry.

// Re-applying a status refreshes it to the event's turns payload (the
// engine's max-of rule) — never a stack, never a sum.

// A queue advance clears the ENEMY side only: the next monster steps in
// fresh, and the killing move locks at its full cooldown (no tick ran).

// The HERO side survives a queue advance: the engine keeps heroCombat
// when the next monster steps in, and so must the mirror.

// Leaving combat (victory here) resets everything: the hero panel must
// not carry chips into exploring. A NEW fight then tracks from zero —
// the engine's startCombat reset, mirrored at the state transition.

// EventEncounterFound resets the mirror preemptively: a found encounter
// may become a fight, and nothing stale may leak into it.

// A round is not over until the enemy answers — and one dispatch batch is
// one round: a batch holding only the hero's strike (a killing blow is
// the only real case, covered against the real engine above) locks
// nothing on its own.

// A failed flee grants the enemy a free turn: the round completes and
// ticks even though the hero never acted (engine enemyFreeTurn).

// A DoT kill at round end: the enemy's killing tick proves the round-end
// tick ran (the hero's afflictions tick FIRST) even though the defeat
// aborted the cooldown bookkeeping — the mirror reads the tick events as
// proof and ticks the hero side, while the lock still applies at full
// value and the enemy side clears.

// A plain kill (no round-end tick) must NOT tick the hero's statuses:
// only tick events prove the round end ran.

// ---- the 2×2 move grid ----

// heavy is the canonical cooldown move fixture (175%, locks 2 rounds).
func heavy() entities.Move {
	return entities.Move{ID: "heavy_slash", Name: "Heavy Slash", Tier: 2, DmgModifier: 175, Cooldown: 2, Weight: 10}
}

// stoneEdge / wildSwing are plain damage moves for the weapon fixture.
func stoneEdge() entities.Move {
	return entities.Move{ID: "stone_edge", Name: "Stone Edge", Tier: 1, DmgModifier: 90, Weight: 10}
}

func wildSwing() entities.Move {
	return entities.Move{ID: "wild_swing", Name: "Wild Swing", Tier: 1, DmgModifier: 110, Weight: 10}
}

// equipWeapon puts a weapon granting the given runtime moves on the hero
// — the second half of a 4-move hero set (MaxLearnedMoves caps learning
// at 2; weapon moves sit on top).
func equipWeapon(e *engine.Engine, moves ...entities.Move) {
	weapon := entities.NewItemFromAsset(game.ItemAsset{
		ID: "fang_blade", Name: "Fang Blade", Slot: "weapon",
	}, moves...)
	e.Player.EquipItem(&weapon)
}

// fourMoveModel is a real combat model with a full grid: two learned
// moves plus two weapon-granted ones.
func fourMoveModel(t *testing.T) Model {
	t.Helper()
	e := combatEngine()
	learnMove(e, envenom(0))
	learnMove(e, jab())
	equipWeapon(e, stoneEdge(), wildSwing())
	return enterCombat(t, e)
}

// A modded move whose name is longer than any shipped one truncates
// inside its cell — the grid's geometry never moves for it.
func TestGridTruncatesModdedOverlongNames(t *testing.T) {
	e := combatEngine()
	learnMove(e, entities.Move{
		ID: "modded", Name: "Super Calibrating Excalibur Strike", Tier: 1, DmgModifier: 100, Weight: 10,
	})
	m := enterCombat(t, e)

	for i, line := range viewLines(m.moveGrid()) {
		if i == 0 {
			continue
		}
		if w := ansiLen(line); w != mainColWidth {
			t.Errorf("grid line %d is %d wide, want exactly %d:\n%q", i, w, mainColWidth, line)
		}
	}
}

// The combat bar shrank to the two combat-only actions; the moves live
// in the grid.
func TestCombatBarIsFleeAndQuit(t *testing.T) {
	m := fourMoveModel(t)

	actions := actionsFor(m.engine)

	if !keysEqual(keysOf(actions), "r", "q") {
		t.Fatalf("combat bar = %v, want [r q] (moves moved into the grid)", keysOf(actions))
	}
}

// The body renders the grid: a cell per effective move, in the exact
// order CmdMove's index points into, each with its tags.
func TestViewCombatShowsTheMoveGrid(t *testing.T) {
	m := fourMoveModel(t)

	view := stripControls(stripANSI(m.View()))

	for _, want := range []string{
		"◈ MOVES",
		"Envenom", "Jab", "Stone Edge", "Wild Swing", // effective-set order
		"100%", "☠poison 100%", // envenom's tags
		"90%", "110%", // the weapon moves' modifiers
		"▶ Envenom", // the cursor opens on the first cell
	} {
		if !strings.Contains(view, want) {
			t.Errorf("combat grid missing %q:\n%s", want, view)
		}
	}
}

// Fewer than 4 moves leaves the remaining cells as dim — placeholders.
func TestViewGridShowsPlaceholdersForMissingMoves(t *testing.T) {
	e := combatEngine()
	learnMove(e, envenom(0))
	m := enterCombat(t, e)

	view := stripControls(stripANSI(m.View()))
	grid := stripControls(stripANSI(m.moveGrid()))

	if !strings.Contains(view, "Envenom") {
		t.Errorf("the single move is missing from the grid:\n%s", view)
	}
	if !strings.Contains(grid, "—") {
		t.Errorf("the move grid's empty cells render no placeholder:\n%s", grid)
	}
	if strings.Contains(view, "Basic Attack") {
		t.Errorf("the grid invented a move:\n%s", view)
	}
}

// The 2+2 split made visible: every grid cell trails its move's source
// glyph — ◆ for the learned set (shrine teaching), ⚔ for weapon
// grants — resolved from the hero's effective set
// (Hero.EffectiveMovesWithSource).
func TestGridCellsTagTheMoveSource(t *testing.T) {
	m := fourMoveModel(t) // two learned moves + two weapon-granted

	grid := stripControls(stripANSI(m.moveGrid()))

	for _, want := range []string{
		"☠poison 100% ◆", // Envenom: learned
		"100% ◆",         // Jab: learned
		"90% ⚔",          // Stone Edge: weapon-granted
		"110% ⚔",         // Wild Swing: weapon-granted
	} {
		if !strings.Contains(grid, want) {
			t.Errorf("move grid missing the source tag %q:\n%s", want, grid)
		}
	}
}

// A hero with NO moves at all fights with the canonical basic attack —
// the grid shows it as the single real cell. Depth-only today (the
// punch start means a fresh hero's set is never empty); kept as the
// combat-integration guarantee for hand-built states.
func TestViewGridShowsTheBasicAttackFallback(t *testing.T) {
	m := enterCombat(t, combatEngine())

	view := stripControls(stripANSI(m.View()))

	if !strings.Contains(view, "Basic Strike") || !strings.Contains(view, "100%") {
		t.Errorf("the basic-attack fallback is missing from the grid:\n%s", view)
	}
	// The canonical fallback is neither learned nor weapon-granted: its
	// cell carries no source tag.
	if strings.Contains(view, "◆") || strings.Contains(view, "100% ⚔") {
		t.Errorf("the basic-attack fallback grew a source tag:\n%s", view)
	}
}

// A fresh hero's combat grid IS the punch start: the pre-learned Punch
// as a ◆ cell (learned slot 1) over the starter sword's explicitly
// granted Basic Strike as a ⚔ cell.
func TestViewFreshHeroGridShowsThePunchStart(t *testing.T) {
	e := engine.NewEngine(&game.Registry{
		Monsters: map[string]game.MonsterAsset{
			"goblin": {ID: "goblin", Name: "Goblin", Difficulty: 3, HP: 30, Damage: 2, Speed: 10, Crit: 5},
		},
		Items: map[string]game.ItemAsset{
			"wooden_sword": {ID: "wooden_sword", Name: "Espada de Madeira", Slot: "weapon",
				BonusDMG: 5, Durability: 50, Moves: []string{"basic_attack"}},
		},
		Moves: map[string]game.MoveAsset{
			"basic_attack": {ID: "basic_attack", Name: "Basic Strike", Tier: 1,
				DmgModifier: 100, Weight: 100, Type: "both"},
		},
		Sections: []game.SectionAsset{{
			ID: "pits", Name: "Goblin Pits", MinFloor: 1, MaxFloor: -1,
			BaseDifficulty: 12, DifficultyPerFloor: 3,
			Spawns: []game.SpawnEntry{{MonsterID: "goblin", Weight: 1}},
		}},
	})
	m := enterCombat(t, e)

	grid := stripControls(stripANSI(m.moveGrid()))

	for _, want := range []string{
		"Punch", // learned slot 1: the punch start
		"60% ◆", // the punch's tags: modifier + learned glyph
		"Basic Strike",
		"100% ⚔", // the starter's explicit grant, weapon-tagged
	} {
		if !strings.Contains(grid, want) {
			t.Errorf("fresh-hero move grid missing %q:\n%s", want, grid)
		}
	}
}

// A cooling move's cell grays out and shows the turns-left marker —
// the modifier tags yield to it while the move is unusable.
func TestViewGridMarksCoolingMoves(t *testing.T) {
	e := combatEngine()
	learnMove(e, heavy())
	learnMove(e, jab())
	m := enterCombat(t, e)
	m = useMove(t, m, 0) // Heavy Slash locks for 2

	view := stripControls(stripANSI(m.View()))

	if !strings.Contains(view, "(2 turns)") {
		t.Errorf("the cooling cell shows no turns-left marker:\n%s", view)
	}
	if strings.Contains(view, "175%") {
		t.Errorf("the cooling cell kept its modifier tag:\n%s", view)
	}
	if !strings.Contains(view, "Heavy Slash") {
		t.Errorf("the cooling cell lost its name:\n%s", view)
	}
}

// The arrows walk the 2×2 grid with wraparound — the action-bar cursor
// stays untouched (the combat bar is a key legend now, not selectable).
func TestArrowKeysMoveTheGridCursor(t *testing.T) {
	m := fourMoveModel(t) // a perfect 2×2: [0 1 / 2 3]
	if m.gridCursor != 0 {
		t.Fatalf("the grid opens on cell %d, want 0", m.gridCursor)
	}
	logLen := len(m.log)

	steps := []struct {
		key  string
		want int
	}{
		{"right", 1}, {"down", 3}, {"left", 2}, {"up", 0}, // the square
		{"up", 2}, {"up", 0}, // vertical wraparound
		{"down", 2}, {"down", 0},
		{"left", 3}, {"left", 2}, // horizontal wraparound
		{"right", 3}, {"right", 0},
	}
	for _, s := range steps {
		m, _ = press(m, s.key)
		if m.gridCursor != s.want {
			t.Fatalf("%s from the previous cell = %d, want %d", s.key, m.gridCursor, s.want)
		}
		if m.cursor != 0 {
			t.Fatalf("%s moved the action-bar cursor (%d)", s.key, m.cursor)
		}
	}

	// Navigation dispatches nothing: no log, no engine round.
	if len(m.log) != logLen {
		t.Errorf("cursor movement wrote %d log lines, want 0", len(m.log)-logLen)
	}
}

// Three moves leave the bottom-right cell empty: wraparound walks the
// real cells and never rests on a placeholder.
func TestGridCursorWrapsOnThreeMoves(t *testing.T) {
	e := combatEngine()
	learnMove(e, envenom(0))
	learnMove(e, jab())
	equipWeapon(e, stoneEdge()) // 3 moves: [0 1 / 2 —]
	m := enterCombat(t, e)

	m, _ = press(m, "down") // the real cell below 0
	if m.gridCursor != 2 {
		t.Fatalf("down from 0 = %d, want 2", m.gridCursor)
	}
	m, _ = press(m, "down") // wraparound up the column: 2 → 0
	if m.gridCursor != 0 {
		t.Fatalf("down from 2 = %d, want 0", m.gridCursor)
	}
	m, _ = press(m, "right") // 0 → 1
	m, _ = press(m, "down")  // below 1 is the placeholder: stay put
	if m.gridCursor != 1 {
		t.Fatalf("down from 1 with a ragged grid = %d, want 1 (stay put)", m.gridCursor)
	}
}

// Enter dispatches the highlighted cell — the observable engine effect
// is the move's round resolving — and the cursor stays where it was.
func TestEnterDispatchesTheHighlightedMove(t *testing.T) {
	e := combatEngine()
	learnMove(e, heavy())
	learnMove(e, jab())
	m := enterCombat(t, e)

	m, _ = press(m, "right") // highlight Jab (index 1)
	m, _ = press(m, "enter")

	if !strings.Contains(joinedLog(m), "You use Jab!") {
		t.Errorf("enter dispatched the wrong cell:\n%s", joinedLog(m))
	}
	if m.gridCursor != 1 {
		t.Errorf("grid cursor after dispatch = %d, want 1 (stays put)", m.gridCursor)
	}
}

// Enter on a cooling cell dispatches too: the engine answers "Move on
// cooldown." in the log, state intact.
func TestEnterOnACoolingCellStillDispatches(t *testing.T) {
	e := combatEngine()
	learnMove(e, heavy())
	learnMove(e, jab())
	m := enterCombat(t, e)
	m = useMove(t, m, 0) // Heavy Slash locks for 2; the cursor stays on 0

	m, _ = press(m, "enter")

	last := stripANSI(m.log[len(m.log)-1])
	if last != "Move on cooldown." {
		t.Errorf("enter on the cooling cell logged %q, want the cooldown block", last)
	}
	if m.engine.State != engine.StateCombat {
		t.Errorf("state after the blocked dispatch = %v, want StateCombat", m.engine.State)
	}
}

// Digits beyond the move set dispatch anyway — the engine explains the
// rejection as a generic invalid command, and the cursor never rests on
// a placeholder.
func TestDigitsBeyondTheMoveSetStillDispatch(t *testing.T) {
	e := combatEngine()
	learnMove(e, jab())
	m := enterCombat(t, e)

	m, _ = press(m, "3") // a placeholder cell

	last := stripANSI(m.log[len(m.log)-1])
	if last != "Invalid command" {
		t.Errorf("digit on a placeholder logged %q, want the engine's rejection", last)
	}
	if m.engine.State != engine.StateCombat {
		t.Errorf("state after the rejected digit = %v, want StateCombat", m.engine.State)
	}
	if m.gridCursor != 0 {
		t.Errorf("grid cursor after the rejected digit = %d, want 0 (untouched)", m.gridCursor)
	}
}

// esc during combat is a no-op — the arrows are navigation there, so esc
// must neither quit nor arm the confirmation.
func TestEscapeIsANoOpInCombat(t *testing.T) {
	m := fourMoveModel(t)
	logLen := len(m.log)

	m, cmd := press(m, "esc")

	if cmd != nil {
		t.Error("esc in combat must not request a quit")
	}
	if m.engine.State != engine.StateCombat {
		t.Errorf("state after esc = %v, want to stay in StateCombat", m.engine.State)
	}
	if m.confirmQuit {
		t.Error("esc in combat armed the quit confirmation")
	}
	if len(m.log) != logLen {
		t.Error("esc in combat wrote to the log")
	}
}

// A new combat re-opens the grid on the first cell (the cursor resets
// with the mirror).
func TestGridCursorResetsWithTheMirror(t *testing.T) {
	e := combatEngine()
	learnMove(e, jab())
	e.Player.BaseDMG = 40 // one-shots the queue
	e.CurrentEncounter = engine.Encounter{
		Type:     engine.EncounterCombat,
		Monsters: []entities.Monster{spitterMonster(200), goblinMonster()},
	}
	e.State = engine.StateCombat
	m := New(e)

	m = useMove(t, m, 0)
	m.engine.CurrentEncounter.Monsters[0].CurrentHP = 1
	m = useMove(t, m, 0)     // the spitter falls → the goblin steps up
	m, _ = press(m, "right") // move away from the first cell before victory
	m = useMove(t, m, 0)     // the goblin falls → victory
	if m.engine.State != engine.StateExploring {
		t.Fatalf("setup: state = %v, want StateExploring (victory)", m.engine.State)
	}

	m, _ = press(m, "e")
	m, _ = press(m, "f")

	if m.gridCursor != 0 {
		t.Errorf("a new combat opened the grid on cell %d, want 0", m.gridCursor)
	}
}

// Outside combat the vertical arrows mean nothing — the action-bar
// cursor must not move.
func TestVerticalArrowsAreNoOpsOutsideCombat(t *testing.T) {
	m := New(testEngine())

	m, _ = press(m, "up")
	m, _ = press(m, "down")

	if m.cursor != 0 {
		t.Errorf("vertical arrows moved the bar cursor to %d outside combat", m.cursor)
	}
}

// ---- status chips beside the HP bars ----

// The enemy block shows its active statuses as chips next to the fight:
// the poison chip appears as the event lands (already ticked once —
// the round ended).
func TestViewCombatShowsEnemyStatusChips(t *testing.T) {
	e := combatEngine()
	learnMove(e, envenom(0))
	m := enterCombat(t, e)

	m = useMove(t, m, 0)

	view := stripControls(stripANSI(m.View()))
	if !strings.Contains(view, "Poison(2)") {
		t.Errorf("the enemy block shows no poison chip:\n%s", view)
	}
}

// Chips count down as completed rounds pass and vanish on expiry —
// the whole scripted lifecycle, asserted on the rendered view.
func TestStatusChipsCountDownAndVanish(t *testing.T) {
	e := combatEngine()
	learnMove(e, envenom(0))
	learnMove(e, jab())
	m := enterCombat(t, e)

	m = useMove(t, m, 0) // applied: 3, ticked to 2 at round end
	if !strings.Contains(stripControls(stripANSI(m.View())), "Poison(2)") {
		t.Errorf("round 1: the chip should show Poison(2):\n%s", stripControls(stripANSI(m.View())))
	}

	m = useMove(t, m, 1) // round 2
	if !strings.Contains(stripControls(stripANSI(m.View())), "Poison(1)") {
		t.Errorf("round 2: the chip should show Poison(1):\n%s", stripControls(stripANSI(m.View())))
	}

	m = useMove(t, m, 1) // round 3: the last tick expires it
	if strings.Contains(stripControls(stripANSI(m.View())), "Poison(") {
		t.Errorf("round 3: the expired chip still renders:\n%s", stripControls(stripANSI(m.View())))
	}
}

// The hero panel's HP row gains its own chips — without growing a row:
// the panel's four-line shape is the layout invariant.
func TestViewHeroPanelShowsStatusChips(t *testing.T) {
	e := combatEngine()
	learnMove(e, jab())
	e.CurrentEncounter = engine.Encounter{
		Type:     engine.EncounterCombat,
		Monsters: []entities.Monster{spitterMonster(200), goblinMonster()},
	}
	e.State = engine.StateCombat
	m := New(e)

	m = useMove(t, m, 0) // the spitter poisons the hero

	lines := viewLines(m.heroPanel())
	if len(lines) != 4 {
		t.Fatalf("hero panel has %d lines with chips, want 4", len(lines))
	}
	if !strings.Contains(lines[2], "Poison(2)") {
		t.Errorf("hero HP row shows no poison chip: %q", lines[2])
	}
}

// Chips render ONLY while the fight lives: after combat ends the hero
// panel is back to plain vitals.
func TestHeroChipsVanishOutsideCombat(t *testing.T) {
	e := combatEngine()
	learnMove(e, jab())
	e.Player.BaseDMG = 40
	e.CurrentEncounter = engine.Encounter{
		Type:     engine.EncounterCombat,
		Monsters: []entities.Monster{spitterMonster(200), goblinMonster()},
	}
	e.State = engine.StateCombat
	m := New(e)

	m = useMove(t, m, 0) // poisoned
	m.engine.CurrentEncounter.Monsters[0].CurrentHP = 1
	m = useMove(t, m, 0) // the spitter falls
	m = useMove(t, m, 0) // the goblin falls → victory

	if m.engine.State != engine.StateExploring {
		t.Fatalf("setup: state = %v, want StateExploring (victory)", m.engine.State)
	}
	if strings.Contains(stripControls(stripANSI(m.View())), "Poison(") {
		t.Errorf("the hero chip survived the victory:\n%s", stripControls(stripANSI(m.View())))
	}
}

// The hero's chips gain the same explanation, riding the HP row after
// them — the panel's four-line shape is the layout invariant (heroHeight
// feeds bodyHeight and the merchant's exact worst case), so the
// explanation shares the row, cut to the room left: the documented
// display cap; the '?' overlay carries the full lore.
func TestHeroChipsExplanationRidesTheHPRow(t *testing.T) {
	e := combatEngine()
	learnMove(e, jab())
	e.CurrentEncounter = engine.Encounter{
		Type:     engine.EncounterCombat,
		Monsters: []entities.Monster{spitterMonster(200), goblinMonster()},
	}
	e.State = engine.StateCombat
	m := New(e)

	m = useMove(t, m, 0) // the spitter poisons the hero

	lines := viewLines(m.heroPanel())
	if len(lines) != 4 {
		t.Fatalf("hero panel has %d lines with the explanation, want 4", len(lines))
	}
	if w := ansiLen(lines[2]); w > mainColWidth {
		t.Errorf("hero HP row is %d wide, overflows the %d-col column", w, mainColWidth)
	}
	// The chip stays; the explanation rides after it, cut to the ~13
	// runes of room the row has left for this fixture.
	if !strings.Contains(lines[2], "Poison(2)") || !strings.Contains(lines[2], "Poison — 3 d") {
		t.Errorf("hero HP row lost the chip or the explanation: %q", lines[2])
	}
}

// Chip tones follow the status KIND, same rule as the log: debuffs
// soft-red, buffs green.
func TestStatusChipsToneByKind(t *testing.T) {
	chips := statusChips([]combatStatus{
		{name: "Guard", kind: "buff", turns: 2},
		{name: "Poison", kind: "debuff", turns: 3},
	})

	if got, want := chips[0], styleHeal.Render("Guard(2)"); got != want {
		t.Errorf("buff chip = %q, want %q", got, want)
	}
	if got, want := chips[1], styleHurt.Render("Poison(3)"); got != want {
		t.Errorf("debuff chip = %q, want %q", got, want)
	}
}

// The enemy block's chips wrap to at most two rows, plus ONE dim
// explanation line under them; nothing leans on slot() to clip: every
// row fits the column, the rest is dropped.

// Under the chips sits one dim explanation line — "Poison — 3 dmg/turn
// · 2 turns left": the live turns from the mirror, the effect from the
// registry's lore.

// A status the registry doesn't know degrades to its live count — the
// explanation stays truthful.

// The hero's chips fill the HP row while they fit — the row never
// overflows the column and the panel keeps its four lines.
