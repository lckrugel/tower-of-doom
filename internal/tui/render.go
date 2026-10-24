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
		// The engine's richer payload reports the level-up heal; a legacy
		// event (or a full-HP hero, healed 0) keeps the plain line.
		if healed, _ := ev.Data["healed"].(int); healed > 0 {
			return fmt.Sprintf("Level up! You are now level %v (+%d HP).", ev.Data["level"], healed)
		}
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
	case engine.EventLootFound:
		return lootFoundText(ev.Data)
	case engine.EventItemEquipped:
		return itemEquippedText(ev.Data)
	case engine.EventItemStashed:
		return stashedText(ev.Data)
	case engine.EventItemDiscarded:
		return discardedText(ev.Data)
	case engine.EventItemLost:
		return lostText(ev.Data)
	case engine.EventBagFull:
		return "Your bag is full."
	case engine.EventGoldGained:
		return goldGainedText(ev.Data)
	case engine.EventMerchantFound:
		return fmt.Sprintf("A wandering merchant appears — %v wares for sale.", ev.Data["count"])
	case engine.EventItemBought:
		return boughtText(ev.Data)
	case engine.EventItemSold:
		return soldText(ev.Data)
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
		// Absolute HP wins over the legacy floor percentage: knowing the
		// enemy needs 2 more hits beats doing the math from 27%.
		if cur, max, ok := exactHP(data, "enemy_hp_exact", "enemy_max_hp"); ok {
			fmt.Fprintf(&b, " (%d/%d HP left).", cur, max)
		} else {
			fmt.Fprintf(&b, " (%v%% HP left).", data["enemy_hp"])
		}
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
		// Same absolute-HP preference as the enemy side.
		if cur, max, ok := exactHP(data, "hero_hp_exact", "hero_max_hp"); ok {
			return fmt.Sprintf("%vYou take %v damage (%d/%d HP left).", crit, data["damage_taken"], cur, max)
		}
		return fmt.Sprintf("%vYou take %v damage (%v%% HP left).", crit, data["damage_taken"], data["hero_hp"])
	}
	return fmt.Sprintf("%vYou take %v damage and your vision fades...", crit, data["damage_taken"])
}

// exactHP reads an event's optional absolute HP pair (e.g.
// "enemy_hp_exact" + "enemy_max_hp"). It reports ok=false when either key
// is missing or not an int, so a legacy-only payload keeps rendering the
// floor percentage it does carry.
func exactHP(data map[string]any, curKey, maxKey string) (cur, max int, ok bool) {
	cur, curOk := data[curKey].(int)
	max, maxOk := data[maxKey].(int)
	return cur, max, curOk && maxOk
}

// heroHealedText renders the sanctuary restore, including the
// nothing-to-heal case (a sanctuary clears the floor even at full health).
func heroHealedText(data map[string]any) string {
	healed, _ := data["healed"].(int)
	if healed <= 0 {
		return "An ancient sanctuary embraces you in quiet rest — your wounds are already healed."
	}
	if cur, max, ok := exactHP(data, "hero_hp_exact", "hero_max_hp"); ok {
		return fmt.Sprintf("While exploring, you find an ancient sanctuary and rest. You recover %d HP (%d/%d HP).",
			healed, cur, max)
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

// itemOf reads an event's item payload, treating a missing key and a
// typed nil alike (both mean "no item to name").
func itemOf(data map[string]any) (*entities.Item, bool) {
	item, ok := data["item"].(*entities.Item)
	return item, ok && item != nil
}

// lootFoundText renders the chest's yield, held pending the StateLoot
// decision. A chest without an item (malformed payload) is just dust.
func lootFoundText(data map[string]any) string {
	item, ok := itemOf(data)
	if !ok {
		return "The chest holds nothing but dust."
	}
	return fmt.Sprintf("The chest holds %v%v.", item.Name, itemBonusList(item))
}

// itemEquippedText renders the wear action (from the bag or the chest).
// The displaced item is optional — the swapped_from key is absent when
// nothing was worn in that slot before.
func itemEquippedText(data map[string]any) string {
	item, ok := itemOf(data)
	if !ok {
		return "You equip some gear, but the tower swallowed it."
	}
	if swapped, ok := data["swapped_from"].(*entities.Item); ok && swapped != nil {
		return fmt.Sprintf("You equip %v%v — the %v goes into your bag.",
			item.Name, itemBonusList(item), swapped.Name)
	}
	return fmt.Sprintf("You equip %v%v.", item.Name, itemBonusList(item))
}

func stashedText(data map[string]any) string {
	item, ok := itemOf(data)
	if !ok {
		return "Some gear slips into your bag."
	}
	return fmt.Sprintf("You put the %v%v in your bag.", item.Name, itemBonusList(item))
}

func discardedText(data map[string]any) string {
	item, ok := itemOf(data)
	if !ok {
		return "Some gear crumbles away — gone for good."
	}
	return fmt.Sprintf("The %v%v crumbles away — gone for good.", item.Name, itemBonusList(item))
}

func lostText(data map[string]any) string {
	item, ok := itemOf(data)
	if !ok {
		return "Your bag overflows — some gear is lost to the tower."
	}
	return fmt.Sprintf("Your bag overflows — the %v%v is lost to the tower.", item.Name, itemBonusList(item))
}

// goldGainedText renders a purse reward. The source picks the flavor:
// a kill drops its coin (the kill line right above already names the
// enemy), a chest pays out coins instead of holding an item; anything
// else keeps a generic line — an unknown source must still say
// something truthful.
func goldGainedText(data map[string]any) string {
	amount, _ := data["amount"].(int)
	switch source, _ := data["source"].(string); source {
	case engine.GoldSourceKill:
		return fmt.Sprintf("The fallen enemy drops %d gold.", amount)
	case engine.GoldSourceChest:
		return fmt.Sprintf("The chest holds %d gold coins!", amount)
	default:
		return fmt.Sprintf("You gain %v gold.", data["amount"])
	}
}

// boughtText renders a merchant purchase (the item lands in the hero
// bag). A malformed payload (no item) still owns up to the gold leaving
// the purse.
func boughtText(data map[string]any) string {
	price, _ := data["price"].(int)
	item, ok := itemOf(data)
	if !ok {
		return fmt.Sprintf("You buy the wares for %d gold.", price)
	}
	return fmt.Sprintf("You buy the %v%v for %d gold.", item.Name, itemBonusList(item), price)
}

// soldText renders selling a bag item to the merchant — the item is
// destroyed, so the price is all that comes back.
func soldText(data map[string]any) string {
	price, _ := data["price"].(int)
	item, ok := itemOf(data)
	if !ok {
		return fmt.Sprintf("The merchant pays %d gold for the wares.", price)
	}
	return fmt.Sprintf("The merchant pays %d gold for the %v%v.", price, item.Name, itemBonusList(item))
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
