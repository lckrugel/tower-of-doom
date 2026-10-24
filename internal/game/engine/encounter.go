package engine

import (
	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// `EncounterType` is the kind of encounter found while exploring.
type EncounterType int

const (
	// Enemies spawn and the player chooses to fight or sneak past.
	EncounterCombat EncounterType = iota
	// A healing spot: resolves instantly on EXPLORE.
	EncounterSanctuary
	// Riches: opens the loot decision (StateLoot) on EXPLORE with a
	// random item.
	EncounterChest
	// A wandering shop: opens StateMerchant with a stocked offer list.
	// APPENDED LAST in the kind roll on purpose — a zero/absent merchant
	// weight keeps every pre-merchant rng sequence identical.
	EncounterMerchant
)

func (t EncounterType) String() string {
	switch t {
	case EncounterCombat:
		return "Combat"
	case EncounterSanctuary:
		return "Sanctuary"
	case EncounterChest:
		return "Chest"
	case EncounterMerchant:
		return "Merchant"
	default:
		return "Unknown"
	}
}

// `Encounter` is what actually spawns on a floor: combat encounters hold
// the enemy queue; sanctuary encounters carry no monsters and resolve
// instantly the moment they are found, while chest encounters open the
// StateLoot decision over the loot they rolled.
type Encounter struct {
	Type     EncounterType
	Monsters []entities.Monster
}

// sanctuaryHealPercent is how much of the Hero's base max HP a sanctuary
// restores, as a percentage.
const sanctuaryHealPercent = 35

// chestGoldChance is the chance (percentage) that a chest pays out gold
// instead of holding an item.
const chestGoldChance = 40

// goldPerDifficulty is the economy's gold coefficient: kills pay it per
// unit of enemy difficulty, gold chests per unit of floor difficulty.
const goldPerDifficulty = 5

// rollEncounterType sorteia o tipo do encontro a partir dos pesos da seção.
//
// A ordem dentro do sorteio é [combat, sanctuary, chest, merchant], o que
// torna a sequência determinística sob um rng semeado — e é um contrato
// de determinismo: o merchant foi ANEXADO por último, então um peso
// zerado/ausente mantém intactas todas as sequências de rng anteriores
// (os buckets anteriores não se deslocam). Toda a tabela zerada (ou
// ausente no JSON) devolve combate sem consumir rng: assim as seções de
// combate puro não deslocam a sequência usada pelos sorteios de monstros.
func (e *Engine) rollEncounterType(w game.EncounterTypeWeights) EncounterType {
	if w.Combat <= 0 && w.Sanctuary <= 0 && w.Chest <= 0 && w.Merchant <= 0 {
		return EncounterCombat
	}
	return weightedPick(
		e.rng,
		[]EncounterType{EncounterCombat, EncounterSanctuary, EncounterChest, EncounterMerchant},
		[]int{w.Combat, w.Sanctuary, w.Chest, w.Merchant},
	)
}

// decideEncounterKind resolves one encounter-kind decision (the EXPLORE
// path) on top of the raw weighted roll (rollEncounterType): the FIRST
// explore of a run is forced combat, skipping the kind roll entirely —
// zero rng consumed, so a sanctuary, chest or merchant can never open a
// run. The flag is per run: ascension does not reset it. Every later
// explore rolls the kind table exactly as before the rule existed.
func (e *Engine) decideEncounterKind(w game.EncounterTypeWeights) EncounterType {
	if !e.firstExploreDone {
		e.firstExploreDone = true
		return EncounterCombat
	}
	return e.rollEncounterType(w)
}

// resolveEncounter handles the EncounterTypes that resolve on the spot:
// a sanctuary (heal) resolves instantly, while a chest rolls its item and
// holds it as PendingLoot for the StateLoot decision (equip/stash/discard).
// Only combat keeps the flow two-stepped, going through StateEncounter for
// FIGHT/SNEAK.
//
// Instant encounters never unlock ASCEND: only a combat victory clears the
// floor path.
func (e *Engine) resolveEncounter(encounter Encounter) []Event {
	switch encounter.Type {
	case EncounterSanctuary:
		healed := e.restAtSanctuary()
		return []Event{NewHeroHealedExact(e.Player, healed, e.Player.CurrentHPPercent())}
	case EncounterChest:
		section := e.currentSection()
		// One extra roll right after the kind roll decides gold-vs-item: a
		// gold chest pays out on the spot and skips the loot decision
		// entirely — exploring continues directly.
		if e.rng.Intn(100) < chestGoldChance {
			amount := goldPerDifficulty * section.FloorDifficulty(e.CurrentFloor)
			e.Player.Gold += amount
			return []Event{NewGoldGained(amount, GoldSourceChest)}
		}
		// The rng roll for the item stays exactly as before; only the
		// disposition of the rolled item changed (no auto-equip anymore).
		// The pool is gated by the section's item_tiers (empty =
		// unrestricted).
		if item := e.rollItemReward(section.ItemTiers); item != nil {
			e.PendingLoot = item
			e.State = StateLoot
			return []Event{NewLootFound(item)}
		}
		// Registry defines no items at all — the chest is just empty,
		// and exploring continues.
		return []Event{NewMessage("The chest creaks open... but it is empty.")}
	case EncounterMerchant:
		// EXPLORE opens the shop: the stock is rolled from the section's
		// allowed item tiers and held in MerchantStock until the player
		// leaves (or quits). Merchants never unlock ASCEND.
		stock := e.rollMerchantStock(e.currentSection())
		if len(stock) == 0 {
			// Validation keeps every merchant section stocked; this guard
			// only exists for registries that skipped validation.
			return []Event{NewMessage("The merchant spreads empty hands: nothing to sell today.")}
		}
		e.MerchantStock = stock
		e.State = StateMerchant
		return []Event{NewMerchantFound(len(stock))}
	default:
		// Combat encounters are never resolved instantly; the caller must
		// only send sanctuary/chest encounters through this path.
		return []Event{NewMessage("Unhandled encounter type")}
	}
}

// restAtSanctuary heals the Hero by sanctuaryHealPercent of its *base* max
// HP (equipment not included), floored to an integer and clamped by
// `Hero.Heal` — a full-health Hero gains nothing. Consumes no rng.
func (e *Engine) restAtSanctuary() int {
	toHeal := e.Player.BaseMaxHP * sanctuaryHealPercent / 100
	return e.Player.Heal(toHeal)
}
