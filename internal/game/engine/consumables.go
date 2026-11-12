package engine

import (
	"github.com/lckrugel/tower-of-doom/internal/game"
)

// ConsumableAt resolves a raw carried-unit index, not a grouped stack index.
// Unknown definitions and definitions without healing are not usable.
func (e *Engine) ConsumableAt(index int) (game.ConsumableAsset, bool) {
	carried := e.Player.Consumables()
	if index < 0 || index >= len(carried) {
		return game.ConsumableAsset{}, false
	}
	asset, ok := e.ConsumableByID(carried[index])
	return asset, ok && asset.HealPercent > 0
}

// useConsumable handles the turn-free exploring/inventory use path.
func (e *Engine) useConsumable(index int) []Event {
	asset, ok := e.ConsumableAt(index)
	if !ok {
		return []Event{NewMessage("You have no usable consumables.")}
	}
	if e.Player.CurrentHP >= e.Player.MaxHP() {
		return []Event{NewMessage("You are already at full health.")}
	}
	return e.drinkConsumable(asset, index)
}

// drinkConsumable applies the healing potion when its action actually runs.
// Combat calls it at the Hero's initiative point, so an enemy that kills the
// Hero first leaves the potion untouched.
func (e *Engine) drinkConsumable(asset game.ConsumableAsset, index int) []Event {
	if !e.Player.RemoveConsumableAt(index) {
		return []Event{NewMessage("You no longer have that consumable.")}
	}
	amount := e.Player.BaseMaxHP * asset.HealPercent / 100
	healed := e.Player.Heal(amount)
	return []Event{NewConsumableUsed(asset.Name, healed, e.Player)}
}
