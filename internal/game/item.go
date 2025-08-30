package game

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
	Id                int      `json:"id"`
	Slot              ItemSlot `json:"slot"`
	Tag               string   `json:"tag"`
	Name              string   `json:"name"`
	Value             int      `json:"value"`
	BonusHp           int      `json:"bonusHp"`
	BonusDmg          int      `json:"bonusDmg"`
	BonusDef          int      `json:"bonusDef"`
	BonusSpd          int      `json:"bonusSpd"`
	MaxDurability     int      `json:"durability"`
	CurrentDurability int
}
