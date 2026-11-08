package engine

import (
	"github.com/lckrugel/tower-of-doom/internal/game"
)

// nextConsumableAsset resolves the next carried consumable to the registry.
// The first implementation supports the healing-potion definition only.
func (e *Engine) nextConsumableAsset() (game.ConsumableAsset, bool) {
	carried := e.Player.Consumables()
	if len(carried) == 0 {
		return game.ConsumableAsset{}, false
	}
	asset, ok := e.reg.ConsumableByID(carried[0])
	return asset, ok && asset.HealPercent > 0
}

// useConsumable handles the turn-free exploring/inventory use path.
func (e *Engine) useConsumable() []Event {
	asset, ok := e.nextConsumableAsset()
	if !ok {
		return []Event{NewMessage("You have no usable consumables.")}
	}
	if e.Player.CurrentHP >= e.Player.MaxHP() {
		return []Event{NewMessage("You are already at full health.")}
	}
	return e.drinkConsumable(asset)
}

// drinkConsumable applies the healing potion when its action actually runs.
// Combat calls it at the Hero's initiative point, so an enemy that kills the
// Hero first leaves the potion untouched.
func (e *Engine) drinkConsumable(asset game.ConsumableAsset) []Event {
	if !e.Player.RemoveConsumable(asset.ID) {
		return []Event{NewMessage("You no longer have that consumable.")}
	}
	amount := e.Player.BaseMaxHP * asset.HealPercent / 100
	healed := e.Player.Heal(amount)
	return []Event{NewConsumableUsed(asset.Name, healed, e.Player)}
}
