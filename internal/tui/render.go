package tui

import (
	"fmt"
	"strings"

	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// renderEvent returns a log line for an engine event, flavor-colored.
func renderEvent(ev engine.Event) string {
	return toneOf(ev).Render(eventText(ev))
}

// eventText converts an engine event into its plain log line. Kept
// style-free so tests can assert exact texts.
func eventText(ev engine.Event) string {
	switch ev.Type {
	case engine.EventFloorEntered:
		return fmt.Sprintf("You climb to floor %v: %v.", ev.Data["floor"], ev.Data["section"])
	case engine.EventEncounterFound:
		return "You stumble upon an encounter: " + monsterList(ev) + "!"
	case engine.EventSneakSuccess:
		return "You sneak past the enemies unnoticed."
	case engine.EventSneakFailed:
		return "You are spotted! Combat begins and the enemies strike first."
	case engine.EventCombatDamageDealt:
		return damageDealtText(ev.Data)
	case engine.EventCombatDamageTaken:
		return damageTakenText(ev.Data)
	case engine.EventEnemyHealed:
		return fmt.Sprintf("The %v is healed.", ev.Data["enemy"])
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
	case engine.EventHeroHealed:
		return heroHealedText(ev.Data)
	case engine.EventItemFound:
		return itemFoundText(ev.Data)
	case engine.EventQuit:
		return "You persistently close your eyes. Goodbye."
	case engine.EventMessage:
		return fmt.Sprint(ev.Data["message"])
	default:
		return fmt.Sprintf("(unhandled event: %v)", ev.Data)
	}
}

func damageDealtText(data map[string]any) string {
	var b strings.Builder
	if crit, _ := data["crit"].(bool); crit {
		fmt.Fprintf(&b, "You land a devastating blow on the %v — CRIT! %v damage", data["enemy"], data["damage_dealt"])
	} else {
		fmt.Fprintf(&b, "You hit the %v for %v damage", data["enemy"], data["damage_dealt"])
	}
	if alive, _ := data["enemy_alive"].(bool); alive {
		fmt.Fprintf(&b, " (%v%% HP left).", data["enemy_hp"])
	} else {
		b.WriteString(" — a killing blow!")
	}
	return b.String()
}

func damageTakenText(data map[string]any) string {
	crit := ""
	if critFlag, _ := data["crit"].(bool); critFlag {
		crit = "A crushing hit lands on you — CRIT! "
	}
	if alive, _ := data["hero_alive"].(bool); alive {
		return fmt.Sprintf("%vYou take %v damage (%v%% HP left).", crit, data["damage_taken"], data["hero_hp"])
	}
	return fmt.Sprintf("%vYou take %v damage and your vision fades...", crit, data["damage_taken"])
}

// heroHealedText renders the sanctuary restore, including the
// nothing-to-heal case (a sanctuary clears the floor even at full health).
func heroHealedText(data map[string]any) string {
	healed, _ := data["healed"].(int)
	if healed <= 0 {
		return "An ancient sanctuary embraces you in quiet rest — your wounds are already healed."
	}
	return fmt.Sprintf("While exploring, you find an ancient sanctuary and rest. You recover %v HP (%v%% HP left).",
		healed, data["hero_hp"])
}

func itemFoundText(data map[string]any) string {
	item, ok := data["item"].(*entities.Item)
	if !ok {
		return "You found some gear, but the tower swallowed it."
	}
	// No indefinite article: "You find a Espada..." shows the article
	// bug for vowels, and English articles can't be derived from a
	// multilingual item name. Sentence-style naming is article-free and
	// reads the same for every asset.
	return fmt.Sprintf("You find %v%s and put it on.", item.Name, itemBonusList(item))
}

// itemBonusList renders an item's bonuses, e.g. " (+5 HP, +5 DEF)".
func itemBonusList(item *entities.Item) string {
	var parts []string
	if item.BonusHP != 0 {
		parts = append(parts, fmt.Sprintf("%+d HP", item.BonusHP))
	}
	if item.BonusDEF != 0 {
		parts = append(parts, fmt.Sprintf("%+d DEF", item.BonusDEF))
	}
	if item.BonusDMG != 0 {
		parts = append(parts, fmt.Sprintf("%+d DMG", item.BonusDMG))
	}
	if item.BonusSPD != 0 {
		parts = append(parts, fmt.Sprintf("%+d SPD", item.BonusSPD))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// monsterList renders the spawned encounter, e.g. "Goblin (30 HP), Orc (50 HP)".
func monsterList(ev engine.Event) string {
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
