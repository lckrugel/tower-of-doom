package engine

import (
	"fmt"
	"log"
	"math/rand"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

const MAX_ENCOUNTER_SIZE = 6

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
	return &Engine{
		Player:       entities.NewPlayer(),
		CurrentFloor: 1,
		State:        StateExploring,
		CanAscend:    false,
		reg:          reg,
		rng:          mathRandRng{},
	}
}

func (e *Engine) Process(cmd Command) []Event {
	switch e.State {
	case StateExploring:
		return e.handleExploring(cmd)
	case StateCombat:
		return e.handleCombat(cmd)
	case StateGameOver:
		return e.handleGameOver(cmd)
	default:
		return []Event{NewMessage("Nothing more to do")}
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
		return []Event{NewEncounterFound(encounter)}
	case CmdAscend:
		if !e.CanAscend {
			return []Event{NewMessage("Cannot ascend yet")}
		}
		e.CurrentFloor++
		sectionDef, err := e.reg.SectionForFloor(e.CurrentFloor)
		if err != nil {
			// Deve ser inacessível enquanto a torre for infinita:
			// existe uma seção com max_floor == -1 cobrindo todos os andares
			// acima do último trecho finito, e a inicialização valida isso.
			// (TODO: quando a torre tiver um topo, este caso vira uma
			// condição de vitória, não um panic.)
			panic(fmt.Sprintf("unreachable: no section covers floor %d despite passing content validation: %v", e.CurrentFloor, err))
		}
		return []Event{NewFloorEntered(e.CurrentFloor, sectionDef.Name)}
	default:
		return []Event{NewMessage("Invalid command")}
	}
}

func (e *Engine) handleCombat(cmd Command) []Event {
	switch cmd.Type {
	default:
		return []Event{NewMessage("Invalid command")}
	}
}

func (e *Engine) handleGameOver(cmd Command) []Event {
	switch cmd.Type {
	default:
		return []Event{NewMessage("Invalid command")}
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
	return items[len(items)-1] // inalcançável
}
