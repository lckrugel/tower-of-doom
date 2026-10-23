package engine

import "github.com/lckrugel/tower-of-doom/internal/game/engine/entities"

// `Combatant` é o contrato de alguém que pode participar de combate.
//
// Hero e Monster são implementações — as verificações em tempo de
// compilação abaixo garantem que continuam satisfazendo a interface.
type Combatant interface {
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
