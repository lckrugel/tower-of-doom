package engine

type Combatent interface {
	MaxHP() int
	Damage() int
	TakeDmg() int
	Heal() int
	IsAlive() bool
}
