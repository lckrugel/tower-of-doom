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
	State            GameState
	Player           *entities.Hero
	CurrentFloor     int
	CanAscend        bool
	CurrentEncounter []entities.Monster
	reg              *game.Registry
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
		e.CurrentEncounter = encounter
		e.State = StateEncounter
		return []Event{NewEncounterFound(encounter)}
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
			e.Player.EquipItem(item)
			events = append(events, NewItemFound(item))
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
		e.CurrentEncounter = nil
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

// resolveFlee attempts to escape the combat. Success exits combat with no
// reward; failure gives the current enemy a free turn.
func (e *Engine) resolveFlee() []Event {
	enemy := e.currentEnemy()
	chance := fleeChance(e.Player.Speed(), enemy.Speed())
	if e.rng.Intn(100) < chance {
		e.CurrentEncounter = nil
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

// generateEncounter decides what actually spawns on a floor: finds the
// section, computes its difficulty budget, then weighted-randomly picks
// monsters from the spawn table until the budget runs out. Registry only
// supplies the data (sections, spawn weights, monster stats) — every
// decision involving randomness happens here, in the engine.
func (e *Engine) generateEncounter(floor int) ([]entities.Monster, error) {
	section, err := e.reg.SectionForFloor(floor)
	if err != nil {
		return nil, err
	}
	budget := section.FloorDifficulty(floor)

	allDefs := make([]game.MonsterAsset, 0, len(section.Spawns))
	allWeights := make([]int, 0, len(section.Spawns))
	for _, spawn := range section.Spawns {
		m, ok := e.reg.Monsters[spawn.MonsterID]
		if !ok {
			return nil, fmt.Errorf("section %v references unknown monster %v", section.ID, spawn.MonsterID)
		}
		allDefs = append(allDefs, m)
		allWeights = append(allWeights, spawn.Weight)
	}
	if len(allDefs) == 0 {
		return nil, fmt.Errorf("section %v has no spawn table", section.ID)
	}

	var encounter []entities.Monster
	for budget > 0 && len(encounter) < MAX_ENCOUNTER_SIZE {
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

		def := e.weightedPick(candidates, weights)
		encounter = append(encounter, entities.NewMonsterFromAsset(def))
		budget -= def.Difficulty
	}
	if len(encounter) == 0 {
		log.Printf("generated empty encounter for floor %v ", floor)
	}
	return encounter, nil
}

// rollArmorReward rolls a random armor piece (helmet, body or boots) from
// the registry as the reward for ascending — a temporary stand-in for the
// full gold/item drops. Returns nil when the registry defines no armor.
// The Hero auto-equips it (there is no inventory); whatever it replaced is
// gone. The armor list is built from sorted IDs so the roll is deterministic
// under a seeded rng.
func (e *Engine) rollArmorReward() *entities.Item {
	ids := slices.Sorted(maps.Keys(e.reg.Items))
	armor := make([]string, 0, len(ids))
	for _, id := range ids {
		if entities.IsArmor(entities.ItemSlot(e.reg.Items[id].Slot)) {
			armor = append(armor, id)
		}
	}
	if len(armor) == 0 {
		return nil
	}
	item := entities.NewItemFromAsset(e.reg.Items[armor[e.rng.Intn(len(armor))]])
	return &item
}

// weightedPick escolhe um item aleatoriamente, proporcional ao seu peso.
//
// Panics se nenhum peso for positivo: a validação de conteúdo garante
// weight >= 1, então isso é defesa em profundidade para chamadores que
// pularem a validação.
func (e *Engine) weightedPick(items []game.MonsterAsset, weights []int) game.MonsterAsset {
	total := 0
	for _, w := range weights {
		total += w
	}
	if total <= 0 {
		panic(fmt.Sprintf("weightedPick: nenhum peso positivo (total=%d)", total))
	}
	r := e.rng.Intn(total)
	cumulative := 0
	for i, w := range weights {
		cumulative += w
		if r < cumulative {
			return items[i]
		}
	}
	return items[len(items)-1] // unreachable
}

func (e *Engine) isValidCommand(cmd Command) bool {
	allowed := AllowedCommands(e.State)
	return slices.Contains(allowed, cmd.Type)
}
