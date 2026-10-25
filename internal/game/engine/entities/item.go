package entities

import "github.com/lckrugel/tower-of-doom/internal/game"

type ItemSlot string

const (
	weapon = "weapon"
	helmet = "helmet"
	body   = "body_armor"
	boots  = "boots"
	ring   = "ring"
	amulet = "amulet"
)

type Item struct {
	ID   string
	Slot ItemSlot
	Name string
	// Tier places the item on the progression ladder (1 = earliest). It
	// gates which sections can roll the item as a reward/stock and what a
	// merchant may upgrade an offer to.
	Tier              int
	BonusHP           int
	BonusDMG          int
	BonusDEF          int
	BonusSPD          int
	MaxDurability     int
	CurrentDurability int
	// grantedMoves holds the Combat 2.0 moves this WEAPON grants while
	// equipped, resolved from the asset's "moves" ids by the caller (the
	// engine owns the registry). Empty for every other slot and for
	// move-less weapons.
	grantedMoves []Move
}

// IsArmor reports whether the slot is defensive gear (helmet, body or boots) —
// the slots considered armor for drop/reward purposes.
func IsArmor(slot ItemSlot) bool {
	return slot == helmet || slot == body || slot == boots
}

// NewItemFromAsset builds an item from its asset. `granted` carries the
// item's granted Combat 2.0 moves, resolved by the caller from the
// asset's move ids (the engine owns the registry) — pass nothing for
// move-less items, the common case and the only valid one for non-weapon
// slots.
func NewItemFromAsset(asset game.ItemAsset, granted ...Move) Item {
	return Item{
		ID:                asset.ID,
		Name:              asset.Name,
		Slot:              ItemSlot(asset.Slot),
		Tier:              max(1, asset.Tier),
		BonusHP:           asset.BonusHP,
		BonusDMG:          asset.BonusDMG,
		BonusDEF:          asset.BonusDEF,
		BonusSPD:          asset.BonusSPD,
		MaxDurability:     asset.Durability,
		CurrentDurability: asset.Durability,
		grantedMoves:      append([]Move(nil), granted...),
	}
}

// GrantedMoves returns the moves the item grants while equipped (Combat
// 2.0; only weapons grant moves). The returned slice is a copy.
func (i *Item) GrantedMoves() []Move {
	moves := make([]Move, len(i.grantedMoves))
	copy(moves, i.grantedMoves)
	return moves
}

// Retorna o valor atual do Item.
//
// O valor varia conforme os status do item e a sua durabilidade atual.
func (i *Item) Value() float32 {
	maxValue := float32((i.BonusHP + i.BonusDMG + i.BonusDEF + i.BonusSPD) * 50)
	// Items with non-positive max durability (e.g. indestructible rings/amulets)
	// are always worth their full value.
	if i.MaxDurability <= 0 {
		return maxValue
	}
	return maxValue * (float32(i.CurrentDurability) / float32(i.MaxDurability))
}

func (i *Item) Use() {
	if i.Slot != amulet && i.Slot != ring {
		roll := game.RollD20()
		if roll == 1 {
			i.CurrentDurability -= 2
		} else if roll < 10 {
			i.CurrentDurability--
		}
	}
}

// BuyPrice is what a merchant charges for the item: its full value.
func (i *Item) BuyPrice() int {
	return int(i.Value())
}

// SellValue is what a merchant pays for the item: half its value, floored
// at 1 so even junk sells for something.
func (i *Item) SellValue() int {
	return max(1, int(i.Value())/2)
}
