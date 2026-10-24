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
)

func (t EncounterType) String() string {
	switch t {
	case EncounterCombat:
		return "Combat"
	case EncounterSanctuary:
		return "Sanctuary"
	case EncounterChest:
		return "Chest"
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
const sanctuaryHealPercent = 20

// rollEncounterType sorteia o tipo do encontro a partir dos pesos da seção.
//
// A ordem dentro do sorteio é [combat, sanctuary, chest], o que torna a
// sequência determinística sob um rng semeado. Toda a tabela zerada
// (ou ausente no JSON) devolve combate sem consumir rng: assim as seções
// de combate puro não deslocam a sequência usada pelos sorteios de monstros.
func (e *Engine) rollEncounterType(w game.EncounterTypeWeights) EncounterType {
	if w.Combat <= 0 && w.Sanctuary <= 0 && w.Chest <= 0 {
		return EncounterCombat
	}
	return weightedPick(
		e.rng,
		[]EncounterType{EncounterCombat, EncounterSanctuary, EncounterChest},
		[]int{w.Combat, w.Sanctuary, w.Chest},
	)
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
		// The rng roll stays exactly as before; only the disposition of
		// the rolled item changed (no auto-equip anymore).
		if item := e.rollItemReward(); item != nil {
			e.PendingLoot = item
			e.State = StateLoot
			return []Event{NewLootFound(item)}
		}
		// Registry defines no items at all — the chest is just empty,
		// and exploring continues.
		return []Event{NewMessage("The chest creaks open... but it is empty.")}
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
