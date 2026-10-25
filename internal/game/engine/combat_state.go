package engine

import (
	"slices"

	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// Combat 2.0 combat-scoped state: active statuses and move cooldowns
// for the two combatants. Everything here is EPHEMERAL — it resets at
// every combat start (FIGHT, failed SNEAK, boss start) and never
// survives the encounter. Learned moves persist at the entity level
// (Hero.learnedMoves); this is per-fight bookkeeping only.

// activeStatus is one status effect afflicting a combatant: the runtime
// status (resolved from the registry at application time) and how many
// rounds it still has.
type activeStatus struct {
	status    entities.Status
	turnsLeft int
}

// combatantState is one combatant's combat-scoped bookkeeping: the
// statuses afflicting them (in application order) and, per move id, how
// many more rounds the move stays locked (absent or 0 = ready).
type combatantState struct {
	statuses  []activeStatus
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

// applyStatus (re)applies a status to the combatant. Effects NEVER
// stack: a status already active refreshes to the LONGEST duration
// (max of the rounds left and the fresh duration); a new one appends in
// application order. Returns the rounds the status now has left.
func (s *combatantState) applyStatus(status entities.Status) int {
	for i := range s.statuses {
		if s.statuses[i].status.ID == status.ID {
			s.statuses[i].turnsLeft = max(s.statuses[i].turnsLeft, status.Duration)
			return s.statuses[i].turnsLeft
		}
	}
	s.statuses = append(s.statuses, activeStatus{status: status, turnsLeft: status.Duration})
	return status.Duration
}

// statMods sums the stat modifier of every active stat-status for one
// stat kind (DoT statuses modify nothing).
func (s *combatantState) statMods(stat entities.StatKind) int {
	total := 0
	for _, a := range s.statuses {
		if !a.status.IsDot() && a.status.Stat == stat {
			total += a.status.StatAmount
		}
	}
	return total
}

// tickStatuses runs the round-end status tick for one combatant, in
// application order: a DoT damages the afflicted (a plain TakeDmg clamp
// — no mitigation, CAN kill), then every status counts a round down;
// reaching 0 expires it. Returns the tick's events and whether the DoT
// killed the afflicted (the caller handles the defeat paths and stops).
func tickStatuses(st *combatantState, target Combatant) (events []Event, dead bool) {
	for i := 0; i < len(st.statuses); {
		a := &st.statuses[i]
		if a.status.IsDot() {
			dmg := target.TakeDmg(a.status.DotAmount)
			events = append(events, NewStatusDamage(target, a.status, dmg))
			if !target.IsAlive() {
				return events, true
			}
		}
		a.turnsLeft--
		if a.turnsLeft <= 0 {
			events = append(events, NewStatusExpired(target, a.status))
			st.statuses = slices.Delete(st.statuses, i, i+1)
			continue
		}
		i++
	}
	return events, false
}
