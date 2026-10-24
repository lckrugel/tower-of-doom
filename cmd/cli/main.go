package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// validateHorizon validates content coverage up to this floor.
// The tower is infinite (sections with max_floor == -1 cover the rest),
// so a large horizon is enough to guarantee valid content for everything
// reachable in a game.
const validateHorizon = 100_000

// commandWords maps typed words to engine commands, in a stable order.
var commandWords = []struct {
	word string
	cmd  engine.CommandType
}{
	{"explore", engine.CmdExplore},
	{"ascend", engine.CmdAscend},
	{"fight", engine.CmdFight},
	{"sneak", engine.CmdSneak},
	{"attack", engine.CmdAttack},
	{"flee", engine.CmdFlee},
	{"quit", engine.CmdQuit},
}

func lookupCommand(input string) (engine.Command, bool) {
	for _, cw := range commandWords {
		if cw.word == input {
			return engine.Command{Type: cw.cmd}, true
		}
	}
	return engine.Command{}, false
}

func main() {
	registry, err := game.Load("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load assets:", err)
		os.Exit(1)
	}
	if err := registry.Validate(validateHorizon); err != nil {
		fmt.Fprintln(os.Stderr, "invalid game content:", err)
		os.Exit(1)
	}

	runLoop(engine.NewEngine(registry))
}

// runLoop is the game loop: read a command word, process it in the engine
// and print the resulting events. The TUI will consume the same events.
func runLoop(e *engine.Engine) {
	fmt.Println("Welcome to the Tower of Doom! Climb as high as you dare.")

	sc := bufio.NewScanner(os.Stdin)
	for {
		printPrompt(e)
		if !sc.Scan() {
			fmt.Println()
			break // EOF
		}

		input := strings.ToLower(strings.TrimSpace(sc.Text()))
		if input == "" {
			continue
		}
		cmd, ok := lookupCommand(input)
		if !ok {
			fmt.Printf("Unknown command %q.\n", input)
			continue
		}

		for _, ev := range e.Process(cmd) {
			fmt.Println(renderEvent(ev))
			if ev.Type == engine.EventQuit {
				return
			}
		}
	}
}

// printPrompt shows the hero's vital stats and the commands allowed in the
// current state.
func printPrompt(e *engine.Engine) {
	allowed, _ := lookupTable(e.State)
	fmt.Printf("\n[Floor %d | %s | %d HP] (%v) > ",
		e.CurrentFloor, e.State, e.Player.CurrentHP, allowed)
}

// lookupTable returns the command words allowed in the given state.
func lookupTable(state engine.GameState) ([]string, bool) {
	words := make([]string, 0, 4)
	for _, cw := range commandWords {
		for _, c := range engine.AllowedCommands(state) {
			if cw.cmd == c {
				words = append(words, cw.word)
			}
		}
	}
	return words, len(words) > 0
}

func renderEvent(ev engine.Event) string {
	switch ev.Type {
	case engine.EventFloorEntered:
		return fmt.Sprintf("You climb to floor %v: %v.", ev.Data["floor"], ev.Data["section"])
	case engine.EventEncounterFound:
		return "You stumble upon an encounter: " + describeMonsters(ev) + "!"
	case engine.EventSneakSuccess:
		return "You sneak past the enemies unnoticed."
	case engine.EventSneakFailed:
		return "You are spotted! Combat begins and the enemies strike first."
	case engine.EventCombatDamageDealt:
		return renderDamageDealt(ev.Data)
	case engine.EventCombatDamageTaken:
		return renderDamageTaken(ev.Data)
	case engine.EventEnemyDefeated:
		return fmt.Sprintf("The %v goes down! +%v XP.", ev.Data["enemy"], ev.Data["xp_gained"])
	case engine.EventLevelUp:
		return fmt.Sprintf("Level up! You are now level %v.", ev.Data["level"])
	case engine.EventHeroDefeated:
		return "You collapse... Your journey through the tower ends here."
	case engine.EventVictory:
		return "Victory! The floor is clear — ASCEND to continue your climb."
	case engine.EventFled:
		return "You fled the combat successfully."
	case engine.EventQuit:
		return "You persistently close your eyes. Goodbye."
	case engine.EventMessage:
		return fmt.Sprint(ev.Data["message"])
	default:
		return fmt.Sprintf("(unhandled event: %v)", ev.Data)
	}
}

func renderDamageDealt(data map[string]any) string {
	dmg := data["damage_dealt"]
	enemy := data["enemy"]
	if alive, _ := data["enemy_alive"].(bool); alive {
		return fmt.Sprintf("You hit the %v for %v damage (%v%% HP left).", enemy, dmg, data["enemy_hp"])
	}
	return fmt.Sprintf("You hit the %v for %v damage — a killing blow!", enemy, dmg)
}

func renderDamageTaken(data map[string]any) string {
	dmg := data["damage_taken"]
	if alive, _ := data["hero_alive"].(bool); alive {
		return fmt.Sprintf("You take %v damage (%v%% HP left).", dmg, data["hero_hp"])
	}
	return fmt.Sprintf("You take %v damage and your vision fades...", dmg)
}

// describeMonsters renders the spawned encounter, e.g. "Goblin (30 HP), Orc (50 HP)".
func describeMonsters(ev engine.Event) string {
	monsters, ok := ev.Data["monsters"].([]entities.Monster)
	if !ok {
		return "something unidentifiable"
	}
	parts := make([]string, 0, len(monsters))
	for i := range monsters {
		parts = append(parts, fmt.Sprintf("%s (%d HP)", monsters[i].Name(), monsters[i].MaxHP()))
	}
	return strings.Join(parts, ", ")
}
