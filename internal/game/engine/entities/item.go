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
	ID                string
	Slot              ItemSlot
	Name              string
	BonusHP           int
	BonusDMG          int
	BonusDEF          int
	BonusSPD          int
	MaxDurability     int
	CurrentDurability int
}

func NewItemFromAsset(asset game.ItemAsset) Item {
	return Item{
		ID:                asset.ID,
		Name:              asset.Name,
		Slot:              ItemSlot(asset.Slot),
		BonusHP:           asset.BonusHP,
		BonusDMG:          asset.BonusDMG,
		BonusDEF:          asset.BonusDEF,
		BonusSPD:          asset.BonusSPD,
		MaxDurability:     asset.Durability,
		CurrentDurability: asset.Durability,
	}
}

// Retorna o valor atual do Item.
//
// O valor varia conforme os status do item e a sua durabilidade atual.
func (i *Item) Value() float32 {
	maxValue := float32((i.BonusHP + i.BonusDMG + i.BonusDEF + i.BonusSPD) * 50)
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
