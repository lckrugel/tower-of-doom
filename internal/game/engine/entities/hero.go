package entities

import (
	"math"
)

// Status base do Herói
const (
	baseHp           = 100
	baseDmg          = 0
	baseSpd          = 10
	baseDef          = 0
	hpBylvl          = 10
	dmgByLvl         = 2
	baseReqXp        = 100
	requiredXpGrowth = 1.5
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
	// Guarda o HP máximo de antes para reescalar a vida atual proporcionalmente.
	oldMax := h.MaxHP()
	// Raises base stats
	h.BaseMaxHP += hpBylvl
	h.BaseDMG += dmgByLvl
	h.rescaleHP(oldMax, h.MaxHP())
	h.Lvl++
}

func (h *Hero) requiredXp() int {
	return int(baseReqXp * math.Pow(float64(max(h.Lvl-1, 1)), requiredXpGrowth))
}

// Heals the Hero with a set amount.
// Returns the amount that was actually healed.
func (h *Hero) Heal(heal int) int {
	healDone := min(heal, h.MaxHP()-h.CurrentHP)
	h.CurrentHP += healDone
	return healDone
}

func (h *Hero) TakeDmg(dmg int) int {
	// TODO: apply damage reduction formula
	dmgReceived := max(0, min(dmg, h.CurrentHP))
	h.CurrentHP -= dmgReceived
	return dmgReceived
}

// rescaleHP reaplica o % de vida atual após uma mudança no HP máximo.
//
// oldMax deve ser o HP máximo ANTES da mudança; a vida atual é preservada
// como fração do máximo (floor), limitada ao novo máximo.
func (h *Hero) rescaleHP(oldMax, newMax int) {
	if oldMax <= 0 || newMax <= 0 {
		// Sem máximo válido para escalar — mantém CurrentHP como está.
		return
	}
	scaled := h.CurrentHP * newMax / oldMax
	if scaled < 0 {
		scaled = 0
	}
	h.CurrentHP = min(scaled, newMax)
}

func (h *Hero) EquipItem(i *Item) *Item {
	if i == nil {
		return nil
	}
	oldItem := h.UnequipItem(i.Slot)

	// Máximo antes do novo item entrar no slot.
	oldMax := h.MaxHP()

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

	// Preserva o % de vida quando o novo item muda o HP máximo.
	if i.BonusHP != 0 {
		h.rescaleHP(oldMax, h.MaxHP())
	}
	return oldItem
}

func (h *Hero) UnequipItem(slot ItemSlot) *Item {
	// Máximo antes do item sair do slot.
	oldMax := h.MaxHP()

	var item *Item
	switch slot {
	case weapon:
		item = h.Weapon
		h.Weapon = nil
	case helmet:
		item = h.Head
		h.Head = nil
	case body:
		item = h.Body
		h.Body = nil
	case boots:
		item = h.Feet
		h.Feet = nil
	case amulet:
		item = h.Amulet
		h.Amulet = nil
	case ring:
		item = h.Ring
		h.Ring = nil
	}

	// Preserva o % de vida quando o item que saiu muda o HP máximo.
	if item != nil && item.BonusHP != 0 {
		h.rescaleHP(oldMax, h.MaxHP())
	}
	return item
}
