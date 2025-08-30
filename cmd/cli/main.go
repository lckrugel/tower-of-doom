package main

import (
	"fmt"

	"github.com/lckrugel/tower_of_doom/internal/game"
)

func main() {
	engine, err := game.NewEngine()
	if err != nil {
		panic("failed to load game")
	}

	g, err := game.NewGameInstance(engine, "Leo")
	if err != nil {
		panic("failed to load game")
	}
	log := g.BattleFloorRoom()
	fmt.Printf("%v\n", log.ToStrings())
}
