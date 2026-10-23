package entities

import (
	"math"
)

// Status base do Herói
const (
	baseHp    = 100
	baseDmg   = 0
	baseSpd   = 10
	baseDef   = 0
	hpBylvl   = 10
	dmgByLvl  = 2
	baseReqXp = 100
)

type Hero struct {
	Lvl int
	Xp  int
	// Status base recomputados por nível
	BaseMaxHP int
	BaseDMG   int
	BaseSPD   int
	BaseDEF   int
	// Vida atual, persistida entre combates
	CurrentHP int
	// Itens equipados atualmente
	Head   *Item
	Body   *Item
	Feet   *Item
	Weapon *Item
	Ring   *Item
	Amulet *Item
}

func NewPlayer(starterItems ...*Item) *Hero {
	hero := &Hero{
		Lvl:       1,
		Xp:        0,
		BaseMaxHP: baseHp,
		CurrentHP: baseHp,
		BaseDMG:   baseDmg,
		BaseSPD:   baseSpd,
		BaseDEF:   baseDef,
	}

	for _, i := range starterItems {
		hero.EquipItem(i)
	}

	return hero
}

func (h *Hero) equippedItems() []*Item {
	items := []*Item{h.Head, h.Body, h.Feet, h.Weapon, h.Ring, h.Amulet}
	result := make([]*Item, 0, len(items))
	for _, i := range items {
		if i != nil {
			result = append(result, i)
		}
	}
	return result
}

func (h *Hero) Damage() int {
	total := h.BaseDMG
	for _, i := range h.equippedItems() {
		total += i.BonusDMG
	}
	return total
}

func (h *Hero) Speed() int {
	total := h.BaseSPD
	for _, i := range h.equippedItems() {
		total += i.BonusSPD
	}
	return total
}

func (h *Hero) MaxHP() int {
	total := h.BaseMaxHP
	for _, i := range h.equippedItems() {
		total += i.BonusHP
	}
	return total
}

func (h *Hero) Defense() int {
	total := h.BaseDEF
	for _, i := range h.equippedItems() {
		total += i.BonusDEF
	}
	return total
}

func (h *Hero) CritThreshold() int {
	return 10
}

func (h *Hero) IsAlive() bool {
	return h.CurrentHP > 0
}

func (h *Hero) GiveXP(xp int) {
	if !h.IsAlive() {
		return
	}
	h.Xp += xp
	for h.Xp >= h.requiredXp() {
		h.Xp -= h.requiredXp()
		h.levelUp()
	}
}

func (h *Hero) levelUp() {
	if !h.IsAlive() {
		return
	}
	// Snapshots current %HP
	percentHP := float64(h.CurrentHP) / float64(h.BaseMaxHP)
	// Raises base stats
	h.BaseMaxHP += hpBylvl
	h.BaseDMG += dmgByLvl
	// Heals proportional to current %HP
	h.CurrentHP = int(math.Floor(float64(h.BaseMaxHP) * percentHP))
	h.Lvl++
}

func (h *Hero) requiredXp() int {
	return (h.Lvl * h.Lvl) * baseReqXp
}

func (h *Hero) Heal(heal int) int {
	healDone := min(heal, h.MaxHP())
	if h.CurrentHP+healDone > h.MaxHP() {
		healDone = h.MaxHP() - h.CurrentHP
		h.CurrentHP = h.MaxHP()
	} else {
		h.CurrentHP += healDone
	}
	return healDone
}

func (h *Hero) TakeDmg(dmg int) int {
	// TODO: apply damage reduction formula
	dmgReceived := min(dmg, h.CurrentHP)
	h.CurrentHP -= dmgReceived
	return dmgReceived
}

func (h *Hero) EquipItem(i *Item) *Item {
	if i == nil {
		return nil
	}
	oldItem := h.UnequipItem(i.Slot)

	switch i.Slot {
	case weapon:
		h.Weapon = i
	case helmet:
		h.Head = i
	case body:
		h.Body = i
	case boots:
		h.Feet = i
	case amulet:
		h.Amulet = i
	case ring:
		h.Ring = i
	}

	// Updates CurrentHP when equiping and item with BonusHP
	if i.BonusHP != 0 {
		// Snapshots current %HP
		percentHP := float64(h.CurrentHP) / float64(h.BaseMaxHP)
		h.CurrentHP = int(math.Floor(float64(h.MaxHP()) * percentHP))
	}
	return oldItem
}

func (h *Hero) UnequipItem(slot ItemSlot) *Item {
	var item *Item
	switch slot {
	case weapon:
		item = h.Weapon
	case helmet:
		item = h.Head
	case body:
		item = h.Body
	case boots:
		item = h.Feet
	case amulet:
		item = h.Amulet
	case ring:
		item = h.Ring
	}

	// Updates CurrentHP when unequiping and item with BonusHP
	if item != nil && item.BonusHP != 0 {
		// Snapshots current %HP
		percentHP := float64(h.CurrentHP) / float64(h.BaseMaxHP)
		h.CurrentHP = int(math.Floor(float64(h.MaxHP()) * percentHP))
	}
	return item
}
