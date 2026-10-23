package main

import (
	"log"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
)

// validateHorizon valida a cobertura de conteúdo até este andar.
// A torre é infinita (seções com max_floor == -1 cobrem o resto),
// então um horizonte grande é suficiente para garantir conteúdo válido
// para todo o jogo alcançável.
const validateHorizon = 100_000

func main() {
	registry, err := game.Load("")
	if err != nil {
		log.Fatalf("failed to load assets: %v", err)
	}
	if err := registry.Validate(validateHorizon); err != nil {
		log.Fatalf("invalid game content: %v", err)
	}

	gameEngine := engine.NewEngine(registry)
	// TODO: loop principal — recebe comando, processa e mostra os eventos
	// (a TUI em bubbletea consumirá os Eventos do engine).
	_ = gameEngine
}
