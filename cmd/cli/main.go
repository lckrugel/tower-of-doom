package main

import (
	"log"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
)

func main() {
	registry, err := game.Load("")
	if err != nil {
		log.Fatalf("failed to load assets: %v", err)
	}
	engine.NewEngine(registry)
	// Apresenta menu inicial
	for {
		// Recebe comando
		// engine.Process() // Processa comando
		// Mostra eventos/estado atual
	}
}
