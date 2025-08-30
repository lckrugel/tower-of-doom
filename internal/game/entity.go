package game

type Entity interface {
	attack(target Entity) *GameMessage
	takeDmg(dmg int) int
	heal(hp int) *GameMessage
	isDead() bool
	critThreshold() int
}
