package tui

import (
	"strings"
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

func monsterFixture() entities.Monster {
	return entities.NewMonsterFromAsset(game.MonsterAsset{
		ID: "goblin", Name: "Goblin", Difficulty: 3, HP: 30, Damage: 2, Speed: 10, Crit: 5,
	})
}

func testItem() *entities.Item {
	item := entities.NewItemFromAsset(game.ItemAsset{
		ID: "wooden_sword", Name: "Espada de Madeira", Slot: "weapon",
		BonusDMG: 5, Durability: 50,
	})
	return &item
}

// damagedHero builds a Hero that took `dmg` damage, mirroring the engine's
// order of operations (damage applied, then the event carries the result).
func damagedHero(dmg int, crit bool) engine.Event {
	hero := entities.NewPlayer()
	hero.TakeDmg(dmg)
	return engine.NewDamageTaken(hero, dmg, crit)
}

// damagedGoblin builds the "enemy was hit" side of the same pattern.
func damagedGoblin(dmg int, crit bool) engine.Event {
	goblin := monsterFixture()
	goblin.TakeDmg(dmg)
	return engine.NewDamageDealt(&goblin, dmg, crit)
}

func TestEventText(t *testing.T) {
	goblin := monsterFixture()

	tests := []struct {
		name     string
		event    engine.Event
		want     string
		contains []string // checked instead of exact for composed texts
	}{
		{
			"floor entered",
			engine.NewFloorEntered(2, "Goblin Pits"),
			"You climb to floor 2: Goblin Pits.",
			nil,
		},
		{
			"encounter found lists the monsters",
			engine.NewEncounterFound([]entities.Monster{goblin}),
			"",
			[]string{"You stumble upon an encounter", "Goblin (30 HP)"},
		},
		{
			"plain damage dealt",
			damagedGoblin(5, false),
			"You hit the Goblin for 5 damage (83% HP left).",
			nil,
		},
		{
			"critical damage dealt",
			damagedGoblin(10, true),
			"You land a devastating blow on the Goblin — CRIT! 10 damage (66% HP left).",
			nil,
		},
		{
			"killing blow dealt",
			damagedGoblin(30, false),
			"You hit the Goblin for 30 damage — a killing blow!",
			nil,
		},
		{
			"plain damage taken",
			damagedHero(2, false),
			"You take 2 damage (98% HP left).",
			nil,
		},
		{
			"critical damage taken",
			damagedHero(4, true),
			"A crushing hit lands on you — CRIT! You take 4 damage (96% HP left).",
			nil,
		},
		{
			"sneak success",
			engine.NewSneakSuccess(),
			"You sneak past the enemies unnoticed.",
			nil,
		},
		{
			"sneak failure",
			engine.NewSneakFailed(),
			"You are spotted! Combat begins and the enemies strike first.",
			nil,
		},
		{
			"enemy defeated",
			engine.NewEnemyDefeated("Goblin", 12),
			"The Goblin goes down! +12 XP.",
			nil,
		},
		{
			"level up",
			engine.NewLevelUp(2),
			"Level up! You are now level 2.",
			nil,
		},
		{
			"hero defeated",
			engine.NewHeroDefeated(),
			"You collapse... Your journey through the tower ends here.",
			nil,
		},
		{
			"victory",
			engine.NewVictory(),
			"Victory! The floor is clear — ASCEND to continue your climb.",
			nil,
		},
		{
			"fled",
			engine.NewFled(),
			"You fled the combat successfully.",
			nil,
		},
		{
			"quit",
			engine.NewQuit(),
			"You persistently close your eyes. Goodbye.",
			nil,
		},
		{
			"sanctuary heal",
			engine.NewHeroHealed(20, 90),
			"While exploring, you find an ancient sanctuary and rest. You recover 20 HP (90% HP left).",
			nil,
		},
		{
			"sanctuary at full health",
			engine.NewHeroHealed(0, 100),
			"An ancient sanctuary embraces you in quiet rest — your wounds are already healed.",
			nil,
		},
		{
			"item found lists bonuses",
			engine.NewItemFound(testItem()),
			"",
			[]string{"You find Espada de Madeira", "+5 DMG", "and put it on."},
		},
		{
			"message",
			engine.NewMessage("Cannot ascend yet"),
			"Cannot ascend yet",
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eventText(tt.event)
			if tt.contains == nil {
				if got != tt.want {
					t.Errorf("eventText = %q, want %q", got, tt.want)
				}
				return
			}
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("eventText = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}
