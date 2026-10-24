package engine

import (
	"fmt"
	"log"
	"maps"
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
	reg         *game.Registry
	// rng é a fonte de aleatoriedade para decisões do engine
	// (geração de encontros hoje; combates no futuro).
	// Testes injetam um rng determinístico.
	rng rng
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
	if asset, ok := reg.Items[starterItemID]; ok {
		starter := entities.NewItemFromAsset(asset)
		player.EquipItem(&starter)
	}
	return &Engine{
		Player:       player,
		CurrentFloor: 1,
		State:        StateExploring,
		CanAscend:    false,
		reg:          reg,
		rng:          mathRandRng{},
	}
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
		sectionDef, err := e.reg.SectionForFloor(e.CurrentFloor)
		if err != nil {
			// Must be unreachable while the tower is infinite:
			// there is a section with max_floor == -1 covering all floors
			// above the last finite one, and initialization validates that.
			panic(fmt.Sprintf("unreachable: no section covers floor %d despite passing content validation: %v", e.CurrentFloor, err))
		}
		events := []Event{NewFloorEntered(e.CurrentFloor, sectionDef.Name)}
		if item := e.rollArmorReward(); item != nil {
			// Ascend keeps auto-equipping (fast pacing by design), but the
			// displaced gear is not lost anymore: it goes to the bag when
			// there is room, else it is destroyed.
			events = append(events, NewItemFound(item))
			if displaced := e.Player.EquipItem(item); displaced != nil {
				if e.Player.StashItem(displaced) {
					events = append(events, NewItemStashed(displaced))
				} else {
					events = append(events, NewItemLost(displaced))
				}
			}
		}
		return events

	default:
		return []Event{NewMessage("Unhandled command")}
	}
}

func (e *Engine) handleEncounter(cmd Command) []Event {
	switch cmd.Type {
	case CmdFight:
		// Entering combat resolves the first round immediately: the Hero
		// engages the enemy with standard initiative.
		e.State = StateCombat
		return e.resolveRound(true)
	case CmdSneak:
		return e.resolveSneak()
	default:
		return []Event{NewMessage("Unhandled command")}
	}
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
	e.State = StateCombat
	return append([]Event{NewSneakFailed()}, e.enemyFreeTurn()...)
}

func (e *Engine) handleCombat(cmd Command) []Event {
	switch cmd.Type {
	case CmdAttack:
		return e.resolveRound(true)
	case CmdFlee:
		return e.resolveFlee()
	default:
		return []Event{NewMessage("Unhandled command")}
	}
}

// handleLoot resolves the pending chest loot (StateLoot). Every branch
// consumes the pending item (clears PendingLoot) and returns to exploring,
// except a stash rejected by a full bag, which keeps the decision open.
func (e *Engine) handleLoot(cmd Command) []Event {
	switch cmd.Type {
	case CmdEquipLoot:
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
// from StateExploring — no mid-combat bag access). Equip and drop index
// the bag; `isValidCommand` already guarantees a live bag slot, so the
// handlers below index safely.
func (e *Engine) handleInventory(cmd Command) []Event {
	switch cmd.Type {
	case CmdEquip:
		item := e.Player.Bag()[cmd.ItemIndex]
		displaced := e.Player.EquipFromBag(cmd.ItemIndex)
		return []Event{NewItemEquipped(item, displaced)}
	case CmdDrop:
		// Destroyed on the spot: there is no floor layer to drop things
		// on, so the item is gone for good.
		item := e.Player.DropFromBag(cmd.ItemIndex)
		return []Event{NewItemDiscarded(item)}
	case CmdClose:
		e.State = StateExploring
		return []Event{NewMessage("You close your bag.")}
	default:
		return []Event{NewMessage("Unhandled command")}
	}
}

// resolveFlee attempts to escape the combat. Success exits combat with no
// reward; failure gives the current enemy a free turn.
func (e *Engine) resolveFlee() []Event {
	enemy := e.currentEnemy()
	chance := fleeChance(e.Player.Speed(), enemy.Speed())
	if e.rng.Intn(100) < chance {
		e.CurrentEncounter = Encounter{}
		e.State = StateExploring
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

// generateEncounter decides what actually spawns on a floor: first it rolls
// the encounter kind from the section's `encounter_types` weights
// (an absent/zeroed table means combat-only — see rollEncounterType), then,
// for combat, computes the section difficulty budget and weighted-randomly
// picks monsters from the spawn table until the budget runs out. Registry
// only supplies the data (sections, spawn weights, monster stats) — every
// decision involving randomness happens here, in the engine.
func (e *Engine) generateEncounter(floor int) (Encounter, error) {
	section, err := e.reg.SectionForFloor(floor)
	if err != nil {
		return Encounter{}, err
	}

	kind := e.rollEncounterType(section.EncounterTypes)
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
		encounter.Monsters = append(encounter.Monsters, entities.NewMonsterFromAsset(def))
		budget -= def.Difficulty
	}
	if len(encounter.Monsters) == 0 {
		log.Printf("generated empty encounter for floor %v ", floor)
	}
	return encounter, nil
}

// pickItem rolls a random item from the registry whose slot passes the
// `accept` filter. The candidate pool is built from sorted IDs, so the roll
// is deterministic under a seeded rng. Returns nil when no registry entry
// matches the filter.
func (e *Engine) pickItem(accept func(entities.ItemSlot) bool) *entities.Item {
	ids := slices.Sorted(maps.Keys(e.reg.Items))
	pool := make([]string, 0, len(ids))
	for _, id := range ids {
		if accept(entities.ItemSlot(e.reg.Items[id].Slot)) {
			pool = append(pool, id)
		}
	}
	if len(pool) == 0 {
		return nil
	}
	item := entities.NewItemFromAsset(e.reg.Items[pool[e.rng.Intn(len(pool))]])
	return &item
}

// rollArmorReward rolls a random armor piece (helmet, body or boots) from
// the registry as the reward for ascending — a temporary stand-in for the
// full gold/item drops. The Hero auto-equips it (fast pacing by design);
// whatever it displaced flows to the hero bag when there is room, else it
// is destroyed.
func (e *Engine) rollArmorReward() *entities.Item {
	return e.pickItem(entities.IsArmor)
}

// rollItemReward rolls a random item from the whole registry (any slot) as
// the reward for a chest of riches. The roll is the same as it ever was —
// the engine now holds the item as PendingLoot instead of auto-equipping
// it, letting the player decide in StateLoot.
func (e *Engine) rollItemReward() *entities.Item {
	return e.pickItem(func(entities.ItemSlot) bool { return true })
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

func (e *Engine) isValidCommand(cmd Command) bool {
	allowed := AllowedCommands(e.State)
	if !slices.Contains(allowed, cmd.Type) {
		return false
	}
	// Bag commands carry a bag index: it must point at a real bag slot,
	// otherwise it is a generic invalid command with no state change
	// (never a panic — the index is player-reachable).
	if cmd.Type == CmdEquip || cmd.Type == CmdDrop {
		if cmd.ItemIndex < 0 || cmd.ItemIndex >= len(e.Player.Bag()) {
			return false
		}
	}
	return true
}
