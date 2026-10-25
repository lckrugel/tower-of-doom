package engine

import "github.com/lckrugel/tower-of-doom/internal/game/engine/entities"

// Combat 2.0 combat-scoped state: move cooldowns (and, later, active
// statuses) for the two combatants. Everything here is EPHEMERAL — it
// resets at every combat start (FIGHT, failed SNEAK, boss start) and
// never survives the encounter. Learned moves persist at the entity
// level (Hero.learnedMoves); this is per-fight bookkeeping only.

// combatantState is one combatant's combat-scoped bookkeeping: per move
// id, how many more rounds the move stays locked (absent or 0 = ready).
type combatantState struct {
	cooldowns map[string]int
}

func newCombatantState() combatantState {
	return combatantState{cooldowns: make(map[string]int)}
}

// onCooldown reports whether the move id is locked this round.
func (s *combatantState) onCooldown(moveID string) bool {
	return s.cooldowns[moveID] > 0
}

// startCooldown locks a just-used move for exactly its cooldown's
// rounds. Callers set it AFTER the round-end tick so the round of use
// doesn't eat one turn of the lock: with cooldown C the move is unusable
// for exactly C full rounds after the round it was used in. Cooldown 0
// never locks.
func (s *combatantState) startCooldown(move entities.Move) {
	if move.Cooldown > 0 {
		s.cooldowns[move.ID] = move.Cooldown
	}
}

// tickCooldowns counts one round off every lock, removing the spent ones
// (round end).
func (s *combatantState) tickCooldowns() {
	for id, left := range s.cooldowns {
		if left <= 1 {
			delete(s.cooldowns, id)
			continue
		}
		s.cooldowns[id] = left - 1
	}
}
