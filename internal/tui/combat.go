package tui

import "github.com/lckrugel/tower-of-doom/internal/game/engine"

// combatStatus is one active status on one side of the fight, as the
// chips display it: the status NAME (event payloads carry names), its
// kind (buff/debuff — the chip's color) and remaining lifetime with its unit.
// combatStatuses adapts engine snapshot values to the TUI's presentation model.
func combatStatuses(actor engine.CombatActorView) []combatStatus {
	statuses := make([]combatStatus, 0, len(actor.Statuses))
	for _, status := range actor.Statuses {
		statuses = append(statuses, combatStatus{id: status.ID, name: status.Name, kind: string(status.Kind), turns: status.TurnsLeft, unit: status.LifetimeUnit})
	}
	return statuses
}

type combatStatus struct {
	id, name, kind string
	turns          int
	unit           string
}

func (m Model) cooldownLeft(id string) int {
	for _, move := range m.engine.CombatView().Hero.Moves {
		if move.ID == id {
			return move.CooldownLeft
		}
	}
	return 0
}
