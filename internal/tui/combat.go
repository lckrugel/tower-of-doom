package tui

import (
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
)

// heroName is the striker name the engine puts on the hero's event
// payloads (entities.Hero.Name() is fixed).
const heroName = "Hero"

// The combat mirror: the TUI-side picture of the combat-scoped state the
// engine keeps unexported (hero move cooldowns, both sides' active
// statuses). The engine reports that state only through the event stream,
// so the model mirrors it here — absorbing events as they fly through the
// log and replaying the engine's round bookkeeping on them. Nothing here
// is game logic: every rule below mirrors engine behavior (combat.go /
// combat_state.go), and the mirror is display state only.
//
// Round tracking (one engine.Process batch = at most one round):
//
//   - A round COMPLETES when the enemy answers the hero's action: an
//     enemy-side EventMoveUsed in the batch. A completed round ticks —
//     cooldowns count down, the used move locks, statuses count down and
//     expire — exactly once, at the end of the batch (endBatch).
//   - A round ABORTS early when someone dies: a killing blow skips the
//     round-end tick entirely. The hero's used move still locks at its
//     FULL cooldown on a kill (engine appendEnemyDefeat) — the mirror
//     applies the lock the moment EventEnemyDefeated arrives.
//   - A DoT kill at round end is the subtle case: the enemy's killing
//     EventStatusDamage proves the round-end tick RAN (the hero's
//     afflictions tick first, engine endRound), so the hero's statuses
//     tick too — detected by any status tick event in the batch. The
//     cooldowns still don't tick (the defeat path skips tickCooldowns).
//
// Reset (mirrors startCombat / appendEnemyDefeat / the flee exit):
//
//   - Entering StateCombat (FIGHT, failed SNEAK) resets everything; the
//     reset happens BEFORE the batch's events are absorbed, so a failed
//     sneak's free turn tracks on a clean slate.
//   - Leaving combat (victory, successful flee, defeat) resets too: the
//     mirror only ever reflects a live fight — and the hero panel renders
//     in every state, so stale chips must never outlive the fight.
//   - A queue advance (EventEnemyDefeated while still in combat) clears
//     the ENEMY side only: the next monster steps in fresh while the
//     hero's statuses and cooldowns persist (the engine keeps heroCombat).
//   - EventEncounterFound resets preemptively: a found encounter may
//     become a fight.
type combatMirror struct {
	// heroCooldowns maps move id → rounds the move stays locked. Enemy
	// cooldowns are deliberately not mirrored: they shape the monster's
	// AI picks, which nothing in the UI displays.
	heroCooldowns map[string]int
	// heroStatuses / enemyStatuses are the active status chips, in
	// application order.
	heroStatuses  []combatStatus
	enemyStatuses []combatStatus

	// Per-batch round bookkeeping — consumed by endBatch:
	enemyActed bool // an enemy EventMoveUsed was seen (the enemy answered)
	enemyFell  bool // an EventEnemyDefeated was seen (the round aborted)
	ended      bool // victory or hero defeat (the combat is over)
	tickEvents bool // a status tick event was seen (the round end ran)
	// pendingLock is the hero move used this batch (resolved to its id
	// and cooldown at MoveUsed time), waiting for the round end to lock.
	pendingLockID    string
	pendingLockTurns int
}

// combatStatus is one active status on one side of the fight, as the
// chips display it: the status NAME (event payloads carry names), its
// kind (buff/debuff — the chip's color) and the rounds still left.
type combatStatus struct {
	name  string
	kind  string
	turns int
}

func newCombatMirror() combatMirror {
	var m combatMirror
	m.reset()
	return m
}

// reset clears the mirror to a fresh fight's state.
func (m *combatMirror) reset() {
	m.heroCooldowns = make(map[string]int)
	m.heroStatuses = nil
	m.enemyStatuses = nil
	m.clearRoundFlags()
}

// clearRoundFlags consumes one batch's round bookkeeping.
func (m *combatMirror) clearRoundFlags() {
	m.enemyActed, m.enemyFell, m.ended, m.tickEvents = false, false, false, false
	m.pendingLockID, m.pendingLockTurns = "", 0
}

// absorb folds one engine event into the mirror. Called for every event
// dispatched through the model (the same stream the log renders).
func (m *combatMirror) absorb(ev engine.Event, e *engine.Engine) {
	if m.heroCooldowns == nil {
		m.reset() // a zero-valued mirror (not built by New) is still safe
	}
	switch ev.Type {
	case engine.EventEncounterFound, engine.EventBossChallenge:
		// A found encounter may become a fight; a boss challenge is
		// already one. Either way the mirror starts clean.
		m.reset()
	case engine.EventMoveUsed:
		user, _ := ev.Data["user"].(string)
		name, _ := ev.Data["move"].(string)
		if user == heroName {
			m.trackHeroLock(name, e)
			break
		}
		m.enemyActed = true
	case engine.EventStatusApplied:
		target, _ := ev.Data["target"].(string)
		name, _ := ev.Data["status"].(string)
		kind, _ := ev.Data["kind"].(string)
		turns, _ := ev.Data["turns"].(int)
		m.applyStatus(target, name, kind, turns)
	case engine.EventStatusExpired:
		// Authoritative removal — the engine's own tick said so.
		target, _ := ev.Data["target"].(string)
		name, _ := ev.Data["status"].(string)
		m.removeStatus(target, name)
		m.tickEvents = true
	case engine.EventStatusDamage:
		// A DoT tick — proof the round-end tick ran (see endBatch).
		m.tickEvents = true
	case engine.EventEnemyDefeated:
		// The front enemy fell: its state resets (the next monster steps
		// in fresh) and the hero's move locks even though the round ended
		// early. Whether the queue advanced (combat continues) or the
		// victory ended the fight, the batch end settles the rest.
		m.enemyFell = true
		m.enemyStatuses = nil
		m.startPendingLock()
	case engine.EventVictory, engine.EventHeroDefeated:
		m.ended = true
	}
}

// endBatch closes one engine.Process batch: when the batch resolved a
// round, the mirror replays the engine's round-end bookkeeping. Called
// once per dispatch, after every event of the batch was absorbed.
func (m *combatMirror) endBatch() {
	defer m.clearRoundFlags()
	switch {
	case m.ended:
		// The fight is over (victory or defeat): nothing survives it.
		m.reset()
	case m.enemyFell:
		// A kill round never ticks cooldowns (the lock was applied at the
		// defeat). The hero's statuses ticked only when the round end ran
		// — a DoT kill at round end, proven by the batch's tick events.
		if m.tickEvents {
			m.heroStatuses = tickStatuses(m.heroStatuses)
		}
	case m.enemyActed:
		// A completed round: cooldowns tick, the used move locks, both
		// sides' statuses tick (the engine ticks hero first, but the
		// mirror's sides are independent — the order is unobservable).
		for id, left := range m.heroCooldowns {
			if left <= 1 {
				delete(m.heroCooldowns, id)
				continue
			}
			m.heroCooldowns[id] = left - 1
		}
		m.startPendingLock()
		m.heroStatuses = tickStatuses(m.heroStatuses)
		m.enemyStatuses = tickStatuses(m.enemyStatuses)
	}
}

// cooldownLeft reports how many more rounds the hero's move stays locked
// (0 = ready) — the grid's disabled marker reads this.
func (m *combatMirror) cooldownLeft(moveID string) int {
	return m.heroCooldowns[moveID]
}

// trackHeroLock remembers the hero move used this batch, resolved from
// the event's move NAME into the hero's combat set (the event payloads
// carry names, the cooldowns are keyed by id). The first name match wins
// — a modded set with two same-named moves locks the first. An unknown
// name tracks nothing (nothing to lock).
func (m *combatMirror) trackHeroLock(name string, e *engine.Engine) {
	m.pendingLockID, m.pendingLockTurns = "", 0
	for _, mv := range e.CombatMoves() {
		if mv.Name == name {
			m.pendingLockID = mv.ID
			m.pendingLockTurns = mv.Cooldown
			return
		}
	}
}

// startPendingLock locks the batch's used move for its full cooldown —
// the engine sets locks AFTER the round-end tick, so the round of use
// never eats one turn of the lock. Cooldown 0 never locks.
func (m *combatMirror) startPendingLock() {
	if m.pendingLockID == "" || m.pendingLockTurns <= 0 {
		return
	}
	m.heroCooldowns[m.pendingLockID] = m.pendingLockTurns
}

// applyStatus records a status landing on one side — fresh or refreshed
// (the event's turns payload is the resulting count, already maxed by the
// engine's never-stack rule).
func (m *combatMirror) applyStatus(target, name, kind string, turns int) {
	side := &m.enemyStatuses
	if target == heroName {
		side = &m.heroStatuses
	}
	for i := range *side {
		if (*side)[i].name == name {
			(*side)[i].turns = turns
			return
		}
	}
	*side = append(*side, combatStatus{name: name, kind: kind, turns: turns})
}

// removeStatus drops one status from one side (an unknown target or name
// is a no-op — the mirror only mirrors what the engine reported).
func (m *combatMirror) removeStatus(target, name string) {
	side := &m.enemyStatuses
	if target == heroName {
		side = &m.heroStatuses
	}
	for i := range *side {
		if (*side)[i].name == name {
			*side = append((*side)[:i], (*side)[i+1:]...)
			return
		}
	}
}

// tickStatuses counts one round off every status, dropping the spent ones
// (the engine's round-end tick; the StatusExpired events already removed
// theirs from the mirror — a kept status is a survivor to decrement).
func tickStatuses(statuses []combatStatus) []combatStatus {
	kept := make([]combatStatus, 0, len(statuses))
	for _, s := range statuses {
		s.turns--
		if s.turns > 0 {
			kept = append(kept, s)
		}
	}
	return kept
}
