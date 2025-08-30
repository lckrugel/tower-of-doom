package game

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
)

type Engine struct {
	monsters []*Monster
	items    []*Item
	floors   []*Floor
}

func NewEngine() (*Engine, error) {
	items, err := loadItems()
	if err != nil || len(items) == 0 {
		return nil, fmt.Errorf("failed to load items: %w", err)
	}

	monsters, err := loadMonsters()
	if err != nil || len(monsters) == 0 {
		return nil, fmt.Errorf("failed to load monsters: %w", err)
	}

	floors, err := loadFloors()
	if err != nil || len(floors) == 0 {
		return nil, fmt.Errorf("failed to load floors: %w", err)
	}

	return &Engine{
		items:    items,
		monsters: monsters,
		floors:   floors,
	}, nil
}

func (e *Engine) findFloor(floorNumber int) *Floor {
	for _, floor := range e.floors {
		if floor.Number == floorNumber {
			return floor
		}
	}
	log.Printf("não encontrado andar %d", floorNumber)
	return e.getFallbackFloor()
}

func (e *Engine) findMonster(tag string) *Monster {
	for _, monster := range e.monsters {
		if monster.Tag == tag {
			return monster
		}
	}
	log.Printf("não encontrado monstro com a tag %s", tag)
	return e.getFallbackMonster()
}

func (e *Engine) findItem(tag string) (*Item, error) {
	for _, item := range e.items {
		if item.Tag == tag {
			return item, nil
		}
	}
	return nil, fmt.Errorf("item with tag %s not found", tag)
}

func (e *Engine) PickMonstersForFloor(floor *Floor) []*Monster {
	if len(floor.MonstersAllowed) == 0 {
		log.Printf("floor %d has no allowed monsters", floor.Number)
		return nil
	}
	monsters := make([]*Monster, 0)
	remainingDifficulty := floor.Difficulty
	maxMonsters := 15 // Hard limit for monsters

	for remainingDifficulty > 0 && len(monsters) < maxMonsters {
		validMonsters := make([]*FloorMonster, 0)
		for _, floorMonster := range floor.MonstersAllowed {
			monster := e.findMonster(floorMonster.MonsterTag)
			if monster.Difficulty <= remainingDifficulty {
				validMonsters = append(validMonsters, floorMonster)
			}
		}

		if len(validMonsters) == 0 {
			break
		}

		selectedMonster := e.selectWeightedRandom(validMonsters)
		monster := e.findMonster(selectedMonster.MonsterTag)
		remainingDifficulty -= monster.Difficulty

		monsters = append(monsters, monster)
	}

	return monsters
}

func (e *Engine) selectWeightedRandom(monsters []*FloorMonster) *FloorMonster {
	totalWeight := float32(0)
	for _, monster := range monsters {
		totalWeight += monster.SpawnChance
	}

	randomValue := rand.Float32() * totalWeight
	currentWeight := float32(0)

	for _, monster := range monsters {
		currentWeight += monster.SpawnChance
		if randomValue <= currentWeight {
			return monster
		}
	}

	return monsters[0] // Fallback
}

func (e *Engine) getFallbackMonster() *Monster {
	return e.monsters[0]
}

func (e *Engine) getFallbackFloor() *Floor {
	last := len(e.floors) - 1
	return e.floors[last]
}

func loadItems() ([]*Item, error) {
	content, err := os.ReadFile("./assets/items.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read items file: %w", err)
	}

	var items []*Item
	err = json.Unmarshal(content, &items)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal items file: %w", err)
	}

	return items, nil
}

func loadMonsters() ([]*Monster, error) {
	content, err := os.ReadFile("./assets/monsters.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read monsters file: %w", err)
	}

	var monsters []*Monster
	err = json.Unmarshal(content, &monsters)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal monsters file: %w", err)
	}

	return monsters, nil
}

func loadFloors() ([]*Floor, error) {
	content, err := os.ReadFile("./assets/floors.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read floors file: %w", err)
	}

	var floors []*Floor
	err = json.Unmarshal(content, &floors)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal floors file: %w", err)
	}

	return floors, nil
}
