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
// status, remaining DoT ticks, and the end-of-turn deadline for stat effects.
type activeStatus struct {
	status    entities.Status
	turnsLeft int
	expiresAt int
}

// remaining reports DoT ticks or subsequent full turns, never a deadline.
func (a activeStatus) remaining(turn int) int {
	if a.status.IsDot() {
		return max(0, a.turnsLeft)
	}
	return max(0, a.expiresAt-turn)
}

// combatantState is one combatant's combat-scoped bookkeeping: the
// statuses afflicting them (in application order) and, per move id, how
// many more rounds the move stays locked (absent or 0 = ready).
type combatantState struct {
	turn      int
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
// stack: stat effects retain the later deadline, DoTs the larger tick count.
// New effects append in application order. Returns authoritative lifetime.
func (s *combatantState) applyStatus(status entities.Status) int {
	for i := range s.statuses {
		if s.statuses[i].status.ID == status.ID {
			if status.IsDot() {
				s.statuses[i].turnsLeft = max(s.statuses[i].turnsLeft, status.Duration)
			} else {
				s.statuses[i].expiresAt = max(s.statuses[i].expiresAt, s.turn+status.Duration)
			}
			return s.statuses[i].remaining(s.turn)
		}
	}
	s.statuses = append(s.statuses, activeStatus{status: status, turnsLeft: status.Duration, expiresAt: s.turn + status.Duration})
	return status.Duration
}

// statMods sums the stat modifier of every active stat-status for one
// stat kind (DoT statuses modify nothing).
func (s *combatantState) statMods(stat entities.StatKind) (flat, percent int) {
	for _, a := range s.statuses {
		if !a.status.IsDot() && a.status.Stat == stat {
			flat += a.status.StatAmount
			percent += a.status.StatPercent
		}
	}
	return flat, percent
}

func statusDotDamage(status entities.Status, target Combatant) int {
	percent := target.MaxHP() * status.DotPercent / 100
	return max(1, status.DotAmount+percent)
}

// tickDots damages a living recipient in application order, consuming only
// ticks actually run. A lethal tick stops damage immediately; stat expiration
// remains a separate, unconditional end-of-turn bookkeeping phase.
func tickDots(st *combatantState, target Combatant) (events []Event, dead bool) {
	if !target.IsAlive() {
		return nil, true
	}
	for i := 0; i < len(st.statuses); {
		a := &st.statuses[i]
		if a.status.IsDot() {
			dmg := target.TakeDmg(statusDotDamage(a.status, target))
			events = append(events, NewStatusDamage(target, a.status, dmg))
			a.turnsLeft--
			if !target.IsAlive() {
				return events, true
			}
			if a.turnsLeft <= 0 {
				events = append(events, NewStatusExpired(target, a.status))
				st.statuses = slices.Delete(st.statuses, i, i+1)
				continue
			}
		}
		i++
	}
	return events, false
}

// expireStatStatuses removes stat effects at their deadline, after actions and
// DoTs. This runs even when lethal damage stopped the DoT phase early.
func expireStatStatuses(st *combatantState, target Combatant) (events []Event) {
	for i := 0; i < len(st.statuses); {
		a := st.statuses[i]
		if !a.status.IsDot() && a.expiresAt <= st.turn {
			events = append(events, NewStatusExpired(target, a.status))
			st.statuses = slices.Delete(st.statuses, i, i+1)
			continue
		}
		i++
	}
	return events
}
