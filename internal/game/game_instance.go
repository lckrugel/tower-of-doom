package game

import (
	"github.com/google/uuid"
)

type GameInstance struct {
	id          uuid.UUID
	playerName  string
	engine      *Engine
	hero        *Hero
	floorNumber int
	gold        float32
}

func NewGameInstance(engine *Engine, name string) (*GameInstance, error) {
	uuid := uuid.New()

	hero := NewPlayer()

	return &GameInstance{
		id:          uuid,
		engine:      engine,
		playerName:  name,
		hero:        hero,
		gold:        0.0,
		floorNumber: 2,
	}, nil
}

func LoadGameInstance(id uuid.UUID) *GameInstance {
	return &GameInstance{}
}

func (gi *GameInstance) BattleFloorRoom() *GameLog {
	log := NewGameLog()

	monsters := gi.spawnMonsters()
	gi.battle(log, monsters)

	return log
}

func (gi *GameInstance) BattleFloorBoss() *GameLog {
	log := NewGameLog()

	return log
}

func (gi *GameInstance) spawnMonsters() []Monster {
	floor := gi.engine.findFloor(gi.floorNumber)
	templates := gi.engine.PickMonstersForFloor(floor)
	monsters := make([]Monster, 0, len(templates))
	for _, t := range templates {
		m := *t
		m.Init()
		monsters = append(monsters, m)
	}
	return monsters
}

func (gi *GameInstance) battle(log *GameLog, monsters []Monster) *GameLog {
	for _, m := range monsters {
		log.AddMessages(m.Status())
		isPlayerFirst := gi.hero.CurrentSpd >= m.CurrentSpd
		if isPlayerFirst {
			log.AddMessages(gi.hero.attack(&m))
			if m.isDead() {
				continue
			}
		}

		for {
			// Monstro ataca
			log.AddMessages(m.attack(gi.hero))
			if gi.hero.isDead() {
				log.AddMessages(gi.gameOver())
				return log
			}
			// Jogador ataca
			log.AddMessages(gi.hero.attack(&m))
			if m.isDead() {

				log.AddNewMessage(InfoMessage, "", map[string]any{})
				break
			}
		}
	}
	return log
}

func (gi *GameInstance) battleWon() *GameMessage {
	return &GameMessage{}
}

func (gi *GameInstance) battleLost() *GameMessage {
	return &GameMessage{}
}

func (gi *GameInstance) gameOver() *GameMessage {
	return &GameMessage{}
}
