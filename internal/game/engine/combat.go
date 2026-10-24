package engine

import (
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// `Combatant` defines an entity that can join a combat.
type Combatant interface {
	Name() string
	CurrentHPPercent() int
	MaxHP() int
	Damage() int
	Speed() int
	Defense() int
	CritThreshold() int
	TakeDmg(dmg int) int
	Heal(hp int) int
	IsAlive() bool
}

var (
	_ Combatant = (*entities.Hero)(nil)
	_ Combatant = (*entities.Monster)(nil)
)

func ProcessCombatTurn(h *entities.Hero, enemy Combatant, heroAttack bool) []Event {
	var events []Event
	if h.Speed() >= enemy.Speed() {
		if heroAttack {
			dmgDealt := enemy.TakeDmg(h.Damage())
			events = append(events, NewDamageDealt(enemy, dmgDealt))
			if !enemy.IsAlive() {
				events = append(events)
			}
		}
	}
	// TODO: finish implementation
}
