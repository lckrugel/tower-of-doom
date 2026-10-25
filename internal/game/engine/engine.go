package engine

import (
	"fmt"
	"log"
	"math/rand"
	"slices"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

const MAX_ENCOUNTER_SIZE = 6

// starterItemID identifies the item the Hero starts the game equipped with,
// loaded from the registry (so mods can redefine the starter gear).
const starterItemID = "wooden_sword"

type Engine struct {
	State        GameState
	Player       *entities.Hero
	CurrentFloor int
	CanAscend    bool
	// BossUnlocked mirrors CanAscend's style: set when a normal combat is
	// won on the final floor of a section with a boss gate, it offers the
	// boss challenge (CmdChallengeBoss) — ASCEND then stays locked there
	// until the boss falls. Reset on ASCEND (new floor), on fleeing the
	// boss, and on a boss victory; another normal-combat victory on that
	// floor re-unlocks it.
	BossUnlocked bool
	// kills counts the enemies defeated during the current run. It is
	// incremented at the same spot where the kill XP is granted (every
	// EventEnemyDefeated, mid-combat kills included) and reported on
	// EventHeroDefeated. Unexported on purpose: the UI consumes the
	// enriched event, not an accessor.
	kills int
	// CurrentEncounter is the encounter currently being dealt with: the
	// enemy queue while in StateEncounter/StateCombat, zeroed otherwise.
	CurrentEncounter Encounter
	// PendingLoot is the item a chest yielded, waiting for the player's
	// decision in StateLoot (equip/stash/discard). It is set when the
	// chest opens and cleared on every resolution path; it is meaningful
	// only while State == StateLoot.
	PendingLoot *entities.Item
	// MerchantStock holds the merchant's offers while State ==
	// StateMerchant (like PendingLoot for chests). Buying removes an
	// offer; leaving discards them all; it is meaningful only while the
	// shop is open.
	MerchantStock []*entities.Item
	// PendingMove is the move a shrine offered, waiting for the player's
	// forget-or-decline decision in StateLearn (like PendingLoot for
	// chests). It is set when the shrine is found and cleared on every
	// resolution path; it is meaningful only while State == StateLearn.
	PendingMove *entities.Move
	reg         *game.Registry
	// rng é a fonte de aleatoriedade para decisões do engine
	// (geração de encontros hoje; combates no futuro).
	// Testes injetam um rng determinístico.
	rng rng
	// firstExploreDone tracks whether the run's first EXPLORE already
	// happened. The first encounter-kind decision of a run is forced
	// combat without consuming rng (pacing rule — see
	// decideEncounterKind), so a sanctuary/chest/merchant can never open
	// a run. Per run: ascension does not reset it.
	firstExploreDone bool
	// recentKinds holds the encounter kinds resolved by the last two kind
	// decisions (recentKinds[1] is the most recent), feeding the "no
	// three identical non-combat kinds in a row" pacing rule. The zero
	// value is combat, so a fresh engine's empty history can never
	// satisfy the non-combat streak and false-trigger the override.
	recentKinds [2]EncounterType
	// heroCombat / enemyCombat are the combat-scoped states (move
	// cooldowns, active statuses) of the two combatants. Reset at every
	// combat start (startCombat); the enemy state also resets whenever
	// the queue advances (the next monster steps in fresh). Never
	// survives the encounter — see combat_state.go.
	heroCombat  combatantState
	enemyCombat combatantState
}

// `rng` é a fonte de aleatoriedade usada pelo engine.
//
// A implementação padrão usa `math/rand` global (auto-seedada);
// testes substituem por uma versão determinística.
type rng interface {
	Intn(n int) int
}

type mathRandRng struct{}

func (mathRandRng) Intn(n int) int { return rand.Intn(n) }

func NewEngine(reg *game.Registry) *Engine {
	player := entities.NewPlayer()
	e := &Engine{
		Player:       player,
		CurrentFloor: 1,
		State:        StateExploring,
		CanAscend:    false,
		reg:          reg,
		rng:          mathRandRng{},
	}
	if asset, ok := reg.Items[starterItemID]; ok {
		// The starter item is built through the engine's resolver so its
		// granted moves (if any) are resolved from the registry.
		starter := e.itemFromAsset(asset)
		player.EquipItem(&starter)
	}
	return e
}

func (e *Engine) Process(cmd Command) []Event {
	if !e.isValidCommand(cmd) {
		return []Event{NewMessage("Invalid command")}
	}
	if cmd.Type == CmdQuit {
		return []Event{NewQuit()}
	}
	switch e.State {
	case StateExploring:
		return e.handleExploring(cmd)
	case StateEncounter:
		return e.handleEncounter(cmd)
	case StateCombat:
		return e.handleCombat(cmd)
	case StateLoot:
		return e.handleLoot(cmd)
	case StateInventory:
		return e.handleInventory(cmd)
	case StateMerchant:
		return e.handleMerchant(cmd)
	case StateLearn:
		return e.handleLearn(cmd)
	case StateGameOver:
		return e.handleGameOver(cmd)
	default:
		return []Event{NewMessage("Unhandled state")}
	}
}

func (e *Engine) handleExploring(cmd Command) []Event {
	switch cmd.Type {
	case CmdExplore:
		// Gera um encontro
		encounter, err := e.generateEncounter(e.CurrentFloor)
		if err != nil {
			// Deve ser inacessivel.
			// O conteúdo deve ser validado durante a inicialização e os casos onde
			// `generateEncounter` poderia dar erro devem ser
			// previstos e evitados durante esta etapa.
			panic(fmt.Sprintf("unreachable: GenerateEncounter failed for floor %d despite passing content validation: %v",
				e.CurrentFloor, err))
		}
		if encounter.Type != EncounterCombat {
			// Sanctuary resolves instantly, chest opens the StateLoot
			// decision over its item: no StateEncounter, no FIGHT/SNEAK
			// choice — `CurrentEncounter` only ever holds the combat
			// queue.
			e.CurrentEncounter = Encounter{}
			return e.resolveEncounter(encounter)
		}
		e.CurrentEncounter = encounter
		e.State = StateEncounter
		return []Event{NewEncounterFound(encounter.Monsters)}
	case CmdChallengeBoss:
		return e.challengeBoss()
	case CmdInventory:
		// Opening the bag is a pure inspection pause: the bag stays open
		// until CmdClose (or CmdQuit). No combat state is disturbed —
		// this command only exists in StateExploring.
		e.State = StateInventory
		return []Event{NewMessage("You open your bag.")}
	case CmdAscend:
		if !e.CanAscend {
			return []Event{NewMessage("Cannot ascend yet")}
		}
		e.CurrentFloor++
		e.CanAscend = false
		// The boss unlock belongs to the old floor: the new floor starts
		// fresh (its own final floor will re-gate the climb).
		e.BossUnlocked = false
		sectionDef := e.currentSection()
		events := []Event{NewFloorEntered(e.CurrentFloor, sectionDef.Name)}
		// The ascend reward is a loot decision, not an auto-equip: the
		// armor piece rolled from the NEW floor's section is held as
		// PendingLoot and the player chooses in StateLoot (wear/stash/
		// discard) — the very same flow as a chest.
		if item := e.rollArmorReward(sectionDef.ItemTiers); item != nil {
			e.PendingLoot = item
			e.State = StateLoot
			events = append(events, NewLootFound(item))
		} else {
			// Empty armor pool (only possible for registries that skipped
			// validation): behave like an empty chest — a message and no
			// loot decision, never StateLoot holding a nil item.
			events = append(events, NewMessage("No armor piece awaits on this floor."))
		}
		return events

	default:
		return []Event{NewMessage("Unhandled command")}
	}
}

func (e *Engine) handleEncounter(cmd Command) []Event {
	switch cmd.Type {
	case CmdFight:
		// Entering combat resolves NO round: rounds advance from the
		// hero's move choices (CmdMove in StateCombat). FIGHT only opens
		// the state — no rng consumed, no strike forced.
		e.startCombat()
		return nil
	case CmdSneak:
		return e.resolveSneak()
	default:
		return []Event{NewMessage("Unhandled command")}
	}
}

// startCombat opens a fresh combat: the combat-scoped state (move
// cooldowns, active statuses) resets — every FIGHT / failed-SNEAK / boss
// start begins a clean slate. Nothing here survives the encounter.
func (e *Engine) startCombat() {
	e.heroCombat = newCombatantState()
	e.enemyCombat = newCombatantState()
	e.State = StateCombat
}

// resolveSneak attempts to slip past the encounter. Success avoids combat
// entirely (no reward, and ascending stays locked until a victory);
// failure starts combat with a free turn for the enemies.
func (e *Engine) resolveSneak() []Event {
	// 50/50 for now, no stats involved.
	if e.rng.Intn(2) == 0 {
		e.CurrentEncounter = Encounter{}
		e.State = StateExploring
		return []Event{NewSneakSuccess()}
	}
	// A failed sneak is a fresh combat with a free turn for the enemy.
	e.startCombat()
	return append([]Event{NewSneakFailed()}, e.enemyFreeTurn()...)
}

func (e *Engine) handleCombat(cmd Command) []Event {
	switch cmd.Type {
	case CmdMove:
		// The index was range-checked by isValidCommand against the
		// hero's combat move set, so this indexes safely.
		move := e.CombatMoves()[cmd.Index]
		if e.heroCombat.onCooldown(move.ID) {
			// A locked move is a blocked action, not an error: a message,
			// no state change. But a FULLY cooled hero must still be able
			// to act — never a soft-lock — so when every move is locked
			// the turn resolves as a RECOVER round (the hero doesn't act,
			// the enemy does, everything ticks).
			if !e.heroHasReadyMove() {
				return e.resolveRound(nil)
			}
			return []Event{NewMessage("Move on cooldown.")}
		}
		return e.resolveRound(&move)
	case CmdFlee:
		return e.resolveFlee()
	default:
		return []Event{NewMessage("Unhandled command")}
	}
}

// heroHasReadyMove reports whether any of the hero's combat moves is off
// cooldown (a fully-cooled hero turn resolves as a RECOVER round instead
// of a blocked command).
func (e *Engine) heroHasReadyMove() bool {
	for _, m := range e.CombatMoves() {
		if !e.heroCombat.onCooldown(m.ID) {
			return true
		}
	}
	return false
}

// handleLoot resolves the pending loot decision (StateLoot — chests and,
// since the ascend rework, ascensions too). Every branch consumes the
// pending item (clears PendingLoot) and returns to exploring, except a
// stash rejected by a full bag, which keeps the decision open.
func (e *Engine) handleLoot(cmd Command) []Event {
	switch cmd.Type {
	case CmdWear:
		// The pending item is the only subject here: Index is ignored.
		item := e.PendingLoot
		displaced := e.Player.EquipItem(item)
		events := []Event{NewItemEquipped(item, displaced)}
		if displaced != nil && !e.Player.StashItem(displaced) {
			// No bag room: the displaced item is destroyed right here.
			events = append(events, NewItemLost(displaced))
		}
		e.PendingLoot = nil
		e.State = StateExploring
		return events
	case CmdStashLoot:
		if e.Player.StashItem(e.PendingLoot) {
			item := e.PendingLoot
			e.PendingLoot = nil
			e.State = StateExploring
			return []Event{NewItemStashed(item)}
		}
		// Staying in StateLoot: the player must choose another option
		// (the bag-full branch also keeps the item pending).
		return []Event{NewBagFull()}
	case CmdDiscardLoot:
		item := e.PendingLoot
		e.PendingLoot = nil
		e.State = StateExploring
		return []Event{NewItemDiscarded(item)}
	default:
		return []Event{NewMessage("Unhandled command")}
	}
}

// handleInventory manages the hero bag (StateInventory, reachable only
// from StateExploring — no mid-combat bag access). Wear and drop index
// the bag; `isValidCommand` already guarantees a live bag slot, so the
// handlers below index safely.
func (e *Engine) handleInventory(cmd Command) []Event {
	switch cmd.Type {
	case CmdWear:
		item := e.Player.Bag()[cmd.Index]
		displaced := e.Player.EquipFromBag(cmd.Index)
		return []Event{NewItemEquipped(item, displaced)}
	case CmdDrop:
		// Destroyed on the spot: there is no floor layer to drop things
		// on, so the item is gone for good.
		item := e.Player.DropFromBag(cmd.Index)
		return []Event{NewItemDiscarded(item)}
	case CmdClose:
		e.State = StateExploring
		return []Event{NewMessage("You close your bag.")}
	default:
		return []Event{NewMessage("Unhandled command")}
	}
}

// handleLearn resolves the shrine's forget-or-decline decision
// (StateLearn). Every branch consumes the pending offer (clears
// PendingMove) and returns to exploring; learning and forgetting consume
// no rng.
func (e *Engine) handleLearn(cmd Command) []Event {
	switch cmd.Type {
	case CmdForget:
		// The index was range-checked by isValidCommand against the
		// learned set, so this indexes safely.
		forgotten := e.Player.LearnedMoves()[cmd.Index]
		offered := e.PendingMove
		e.Player.ForgetMove(forgotten.ID)
		// Unreachable failure: the offer was filtered to unlearned and a
		// slot just freed. The safe fallback never strands the state.
		if _, learned := e.Player.LearnMove(*offered); !learned {
			e.PendingMove = nil
			e.State = StateExploring
			return []Event{NewMessage(shrineNothingNewMessage)}
		}
		e.PendingMove = nil
		e.State = StateExploring
		return []Event{NewMoveLearned(*offered, &forgotten)}
	case CmdDecline:
		e.PendingMove = nil
		e.State = StateExploring
		return []Event{NewMessage("You leave the shrine's teaching behind.")}
	default:
		return []Event{NewMessage("Unhandled command")}
	}
}

// resolveFlee attempts to escape the combat. The chance reads the
// EFFECTIVE speeds (stat-mod statuses included) — a boss's SPD applies
// like any enemy's. Success exits combat with no reward; failure gives
// the current enemy a free turn. Fleeing a BOSS re-locks the challenge
// (another normal-combat victory on this floor re-unlocks it); fleeing a
// normal combat leaves the unlock alone.
func (e *Engine) resolveFlee() []Event {
	chance := fleeChance(e.heroStat(entities.StatSPD), e.enemyStat(entities.StatSPD))
	if e.rng.Intn(100) < chance {
		boss := e.CurrentEncounter.Boss
		e.CurrentEncounter = Encounter{}
		e.State = StateExploring
		if boss {
			e.BossUnlocked = false
		}
		return []Event{NewFled()}
	}
	return append([]Event{NewMessage("You failed to flee!")}, e.enemyFreeTurn()...)
}

func (e *Engine) handleGameOver(cmd Command) []Event {
	switch cmd.Type {
	default:
		return []Event{NewMessage("Unhandled command")}
	}
}

// generateEncounter decides what actually spawns on a floor: first it
// resolves the encounter kind through the pacing rules (decideEncounterKind)
// over the section's `encounter_types` weights (an absent/zeroed table means
// combat-only — see rollEncounterType), then,
// for combat, computes the section difficulty budget and weighted-randomly
// picks monsters from the spawn table until the budget runs out. Registry
// only supplies the data (sections, spawn weights, monster stats) — every
// decision involving randomness happens here, in the engine.
func (e *Engine) generateEncounter(floor int) (Encounter, error) {
	section, err := e.reg.SectionForFloor(floor)
	if err != nil {
		return Encounter{}, err
	}

	kind := e.decideEncounterKind(section.EncounterTypes)
	if kind != EncounterCombat {
		return Encounter{Type: kind}, nil
	}
	budget := section.FloorDifficulty(floor)

	allDefs := make([]game.MonsterAsset, 0, len(section.Spawns))
	allWeights := make([]int, 0, len(section.Spawns))
	for _, spawn := range section.Spawns {
		m, ok := e.reg.Monsters[spawn.MonsterID]
		if !ok {
			return Encounter{}, fmt.Errorf("section %v references unknown monster %v", section.ID, spawn.MonsterID)
		}
		allDefs = append(allDefs, m)
		allWeights = append(allWeights, spawn.Weight)
	}
	if len(allDefs) == 0 {
		return Encounter{}, fmt.Errorf("section %v has no spawn table", section.ID)
	}

	var encounter Encounter
	encounter.Type = EncounterCombat
	for budget > 0 && len(encounter.Monsters) < MAX_ENCOUNTER_SIZE {
		// Considera apenas monstros com difficulty >= 1: cada iteração ou
		// adiciona um monstro (reduzindo o budget) ou não acha candidato e
		// para. Na prática a garantia real vem da validação de conteúdo
		// (difficulty >= 1); MAX_ENCOUNTER_SIZE fica como defesa em
		// profundidade.
		candidates := make([]game.MonsterAsset, 0, len(allDefs))
		weights := make([]int, 0, len(allDefs))
		for i, def := range allDefs {
			if def.Difficulty <= budget {
				candidates = append(candidates, def)
				weights = append(weights, allWeights[i])
			}
		}
		if len(candidates) == 0 {
			break // nothing left affordable — stop here
		}

		def := weightedPick(e.rng, candidates, weights)
		moves, err := e.monsterMoves(def)
		if err != nil {
			return Encounter{}, err
		}
		encounter.Monsters = append(encounter.Monsters, entities.NewMonsterFromAsset(def, moves...))
		budget -= def.Difficulty
	}
	if len(encounter.Monsters) == 0 {
		log.Printf("generated empty encounter for floor %v ", floor)
	}
	return encounter, nil
}

// monsterMoves resolves a monster asset's move refs into its runtime
// move set: each ref's move is looked up in the registry and the REF's
// weight OVERRIDES the move asset's default AI weight for that monster.
// A monster without refs returns nil — NewMonsterFromAsset then equips
// the canonical basic attack. Unknown refs are an error: a content bug
// that load-time validation otherwise catches.
func (e *Engine) monsterMoves(asset game.MonsterAsset) ([]entities.Move, error) {
	if len(asset.Moves) == 0 {
		return nil, nil
	}
	moves := make([]entities.Move, 0, len(asset.Moves))
	for _, ref := range asset.Moves {
		moveAsset, ok := e.reg.MoveByID(ref.ID)
		if !ok {
			return nil, fmt.Errorf("monster %v references unknown move %v", asset.ID, ref.ID)
		}
		m := entities.NewMoveFromAsset(moveAsset)
		m.Weight = ref.Weight
		moves = append(moves, m)
	}
	return moves, nil
}

// pickItem rolls a random item from the registry whose slot passes the
// `accept` filter and whose tier is one of `tiers` (empty/nil tiers =
// unrestricted). The candidate pool is built from sorted ids, so the roll
// is deterministic under a seeded rng. Returns nil when no registry entry
// matches the filters (an empty pool consumes no rng).
func (e *Engine) pickItem(accept func(entities.ItemSlot) bool, tiers []int) *entities.Item {
	pool := make([]string, 0, len(e.reg.Items))
	for _, id := range e.reg.ItemIDsForTiers(tiers) {
		if accept(entities.ItemSlot(e.reg.Items[id].Slot)) {
			pool = append(pool, id)
		}
	}
	if len(pool) == 0 {
		return nil
	}
	item := e.itemFromAsset(e.reg.Items[pool[e.rng.Intn(len(pool))]])
	return &item
}

// rollArmorReward rolls a random armor piece (helmet, body or boots) from
// the registry as the reward for ascending, restricted to the section's
// allowed item tiers (empty = unrestricted). The piece is held as
// PendingLoot: the player decides its fate in StateLoot, like any chest.
func (e *Engine) rollArmorReward(tiers []int) *entities.Item {
	return e.pickItem(entities.IsArmor, tiers)
}

// rollItemReward rolls a random item from the whole registry (any slot) as
// the reward for a chest of riches, restricted to the section's allowed
// item tiers (empty = unrestricted). The engine holds the item as
// PendingLoot, letting the player decide in StateLoot.
func (e *Engine) rollItemReward(tiers []int) *entities.Item {
	return e.pickItem(func(entities.ItemSlot) bool { return true }, tiers)
}

// weightedPick escolhe um item aleatoriamente, proporcional ao seu peso.
//
// Panics se nenhum peso for positivo: a validação de conteúdo garante
// weight >= 1, então isso é defesa em profundidade para chamadores que
// pularem a validação.
func weightedPick[T any](r rng, items []T, weights []int) T {
	total := 0
	for _, w := range weights {
		total += w
	}
	if total <= 0 {
		panic(fmt.Sprintf("weightedPick: nenhum peso positivo (total=%d)", total))
	}
	cumulative := 0
	rand := r.Intn(total)
	for i, w := range weights {
		cumulative += w
		if rand < cumulative {
			return items[i]
		}
	}
	return items[len(items)-1] // unreachable
}

// SectionName reports the name of the section covering the floor for UI
// display, or "" when no section covers it (validation-timed content bug).
func (e *Engine) SectionName(floor int) string {
	section, err := e.reg.SectionForFloor(floor)
	if err != nil {
		return ""
	}
	return section.Name
}

// currentSection returns the section covering the current floor. The error
// is unreachable: the floor was just resolved or validated at startup, and
// Validate guarantees full coverage up to the infinite top floor.
func (e *Engine) currentSection() game.SectionAsset {
	section, err := e.reg.SectionForFloor(e.CurrentFloor)
	if err != nil {
		panic(fmt.Sprintf("unreachable: no section covers floor %d despite passing content validation: %v", e.CurrentFloor, err))
	}
	return section
}

func (e *Engine) isValidCommand(cmd Command) bool {
	allowed := AllowedCommands(e.State)
	if !slices.Contains(allowed, cmd.Type) {
		return false
	}
	// Index-carrying commands must point at a real slot, otherwise it is
	// a generic invalid command with no state change (never a panic —
	// the index is player-reachable): bag commands and selling at a bag
	// item, buying at a stock offer, and CmdMove at the hero's combat
	// move set.
	if cmd.Type == CmdDrop || cmd.Type == CmdSell {
		if cmd.Index < 0 || cmd.Index >= len(e.Player.Bag()) {
			return false
		}
	}
	// CmdWear indexes the bag only from StateInventory: the StateLoot
	// wear takes the pending item and ignores the index.
	if cmd.Type == CmdWear && e.State == StateInventory {
		if cmd.Index < 0 || cmd.Index >= len(e.Player.Bag()) {
			return false
		}
	}
	if cmd.Type == CmdBuy {
		if cmd.Index < 0 || cmd.Index >= len(e.MerchantStock) {
			return false
		}
	}
	// CmdForget indexes the learned moves (the shrine's forget
	// selection).
	if cmd.Type == CmdForget {
		if cmd.Index < 0 || cmd.Index >= len(e.Player.LearnedMoves()) {
			return false
		}
	}
	if cmd.Type == CmdMove {
		if cmd.Index < 0 || cmd.Index >= len(e.CombatMoves()) {
			return false
		}
	}
	return true
}
