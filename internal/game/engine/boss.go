package engine

import (
	"fmt"
	"slices"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// ——— The section boss challenge ———
//
// ONE boss per section, fought at the section's END (the floor where
// Registry.IsSectionEnd holds — first-match section resolution, so the
// embedded content's floor 3 is the goblin pits' end). A normal-combat
// victory there unlocks the challenge (EventBossUnlocked) instead of
// ASCEND; killing the boss is what opens the gate. See docs/MECHANICS.md,
// "Combat 2.0 → Bosses".

// challengeBoss runs CmdChallengeBoss (StateExploring): the section's
// PREDEFINED boss group is engaged straight into StateCombat — no
// StateEncounter, no sneak step. The build consumes NO rng: the group is
// authored (no weights, no difficulty budget), no encounter-kind roll is
// made, and nothing feeds the kind history (a challenge is not an
// explore — the pacing rules are untouched by it).
func (e *Engine) challengeBoss() []Event {
	if !e.BossUnlocked {
		// Blocked like a locked ASCEND: a message, state intact.
		return []Event{NewMessage("No boss awaits your challenge.")}
	}
	section := e.currentSection()
	if section.Boss == nil {
		// Invariant: BossUnlocked is only ever set by a victory on the
		// final floor of a section WITH a boss block, and the floor
		// cannot change while the challenge pends (ASCEND stays locked).
		panic(fmt.Sprintf("unreachable: boss challenge accepted on floor %d, whose section %q has no boss",
			e.CurrentFloor, section.ID))
	}
	encounter, err := e.bossEncounter(section)
	if err != nil {
		// Load-time validation rejects unknown boss monsters; this is a
		// registry that skipped validation.
		panic(fmt.Sprintf("unreachable: boss encounter for floor %d failed to build despite passing content validation: %v",
			e.CurrentFloor, err))
	}
	e.CurrentEncounter = encounter
	e.startCombat()
	return []Event{NewBossChallenge(encounter.Monsters)}
}

// bossEncounter builds the predefined encounter from the section's boss
// block: every monster in AUTHORED order, its move refs resolved exactly
// like a regular spawn's (monsterMoves). No rng, no budget, no size cap —
// the authored group is the whole encounter.
func (e *Engine) bossEncounter(section game.SectionAsset) (Encounter, error) {
	encounter := Encounter{Type: EncounterCombat, Boss: true}
	for _, id := range section.Boss.Monsters {
		def, ok := e.reg.Monsters[id]
		if !ok {
			return Encounter{}, fmt.Errorf("section %v boss references unknown monster %v", section.ID, id)
		}
		moves, err := e.monsterMoves(def)
		if err != nil {
			return Encounter{}, err
		}
		encounter.Monsters = append(encounter.Monsters, entities.NewMonsterFromAsset(def, moves...))
	}
	return encounter, nil
}

// appendBossVictory resolves the boss's fall (the last boss monster just
// went through the NORMAL defeat flow — bosses pay kill XP and
// 5×difficulty gold like anything else). The victory's own rewards, in
// this event order (the rng contract is step 1's single roll):
//
//  1. the loot roll — ONE Intn over the tier+1 pool (see rollBossLoot);
//     an empty pool skips the item AND the rng. A rolled item is held as
//     PendingLoot and the engine enters StateLoot (the chest decision).
//  2. the gold bonus (BossAsset.Gold, no rng) — EventGoldGained with the
//     "boss" source; skipped when the bonus is 0.
//  3. EventVictory — the ASCEND unlock finally holds: CanAscend = true,
//     the unlock flag is consumed. With a pending item the engine stays
//     in StateLoot (the decision routes back to exploring); without one
//     it returns to exploring directly.
func (e *Engine) appendBossVictory(events []Event) []Event {
	section := e.currentSection()
	if section.Boss == nil {
		// Same invariant as challengeBoss: a boss encounter implies a
		// boss block on the floor's section.
		panic(fmt.Sprintf("unreachable: boss victory on floor %d, whose section %q has no boss",
			e.CurrentFloor, section.ID))
	}
	if item := e.rollBossLoot(section); item != nil {
		e.PendingLoot = item
		e.State = StateLoot
		events = append(events, NewLootFound(item))
	}
	if section.Boss.Gold > 0 {
		e.Player.Gold += section.Boss.Gold
		events = append(events, NewGoldGained(section.Boss.Gold, GoldSourceBoss))
	}
	e.BossUnlocked = false
	e.CanAscend = true
	events = append(events, NewVictory())
	if e.State != StateLoot {
		e.State = StateExploring
	}
	return events
}

// rollBossLoot rolls the boss's item reward, pickItem-style: the pool is
// every registry item (any slot) of the TARGET tier — the section's top
// allowed tier + 1, capped at the registry's max tier. A section without
// item_tiers is unrestricted: its top allowed tier IS the registry's max,
// so the cap lands the reward on the max tier too. ONE Intn over the
// sorted pool (deterministic under a seeded rng); an empty pool consumes
// no rng and yields nil — the gold bonus is granted either way.
func (e *Engine) rollBossLoot(section game.SectionAsset) *entities.Item {
	topTier := e.reg.MaxItemTier()
	if len(section.ItemTiers) > 0 {
		topTier = slices.Max(section.ItemTiers)
	}
	target := min(topTier+1, e.reg.MaxItemTier())
	return e.pickItem(func(entities.ItemSlot) bool { return true }, []int{target})
}
