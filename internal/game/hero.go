package game

import "fmt"

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
	Lvl        int
	Xp         int
	MaxHp      int
	BaseDmg    int
	Spd        int
	Def        int
	CurrentHp  int
	CurrentDmg int
	CurrentSpd int
	CurrentDef int
	head       *Item
	body       *Item
	feeet      *Item
	weapon     *Item
	ring       *Item
	amulet     *Item
}

func NewPlayer(starterItems ...*Item) *Hero {
	hero := &Hero{
		Lvl:        1,
		Xp:         0.0,
		MaxHp:      baseHp,
		BaseDmg:    baseDmg,
		Spd:        baseSpd,
		Def:        baseDef,
		CurrentHp:  baseHp,
		CurrentDmg: baseDmg,
		CurrentSpd: baseSpd,
		CurrentDef: baseDef,
	}

	for _, i := range starterItems {
		hero.EquipItem(i)
	}

	return hero
}

func (h *Hero) GiveXP(xp int) {
	if h.isDead() {
		return
	}
	h.Xp += xp
	nextLevelXp := h.requiredXp()
	if h.Xp >= nextLevelXp {
		mod := h.Xp % nextLevelXp
		for range mod {
			h.Xp = h.Xp - nextLevelXp
			h.levelUp()
		}
	}
}

func (h *Hero) levelUp() {
	if h.isDead() {
		return
	}
	h.MaxHp += hpBylvl
	h.BaseDmg += dmgByLvl
	h.Lvl++
}

func (h *Hero) requiredXp() int {
	return (h.Lvl * h.Lvl) * baseReqXp
}

func (h *Hero) heal(hp int) *GameMessage {
	healDone := hp
	if h.CurrentHp+hp > h.MaxHp {
		healDone = h.MaxHp - h.CurrentHp
		h.CurrentHp = h.MaxHp
	} else {
		h.CurrentHp += hp
	}
	return &GameMessage{
		Type:    InfoMessage,
		Content: fmt.Sprintf("Herói curado em %d", healDone),
		Data: map[string]any{
			"healed":    healDone,
			"maxHp":     h.MaxHp,
			"currentHp": h.CurrentHp,
		},
	}
}

func (h *Hero) takeDmg(dmg int) int {
	actualDmg := dmg
	if actualDmg >= h.CurrentHp {
		h.CurrentHp = 0
	}
	h.CurrentHp -= actualDmg
	return actualDmg
}

func (h *Hero) attack(target Entity) *GameMessage {
	roll := RollD10()
	dmg := h.CurrentDmg + roll
	// Crítico!
	if roll >= h.critThreshold() {
		dmg = dmg * 2
		dmgDone := target.takeDmg(dmg)
		return &GameMessage{
			Type:    CombatMessage,
			Content: fmt.Sprintf("Herói causou %d de dano em um crítico!\n", dmgDone),
			Data: map[string]any{
				"dmg":    dmgDone,
				"isCrit": true,
			},
		}
	}
	dmgDone := target.takeDmg(dmg)
	return &GameMessage{
		Type:    CombatMessage,
		Content: fmt.Sprintf("Herói causou %d de dano\n", dmgDone),
		Data: map[string]any{
			"dmg":    dmgDone,
			"isCrit": false,
		},
	}
}

func (h *Hero) isDead() bool {
	return h.CurrentHp <= 0
}

func (h *Hero) critThreshold() int {
	return 10
}

func (h *Hero) String() []string {
	str := make([]string, 0)
	str = append(str, "Herói\n")
	str = append(str, fmt.Sprintf("Vida: %d/%d | Defesa: %d, | Ataque: %d\n", h.CurrentHp, h.MaxHp, h.CurrentDef, h.CurrentDmg))
	return str
}

func (h *Hero) EquipItem(i *Item) {
	if i == nil {
		return
	}
	h.UnequipItem(i.Slot)

	switch i.Slot {
	case weapon:
		h.weapon = i
	case helmet:
		h.head = i
	case body:
		h.body = i
	case boots:
		h.feeet = i
	case amulet:
		h.amulet = i
	case ring:
		h.ring = i
	}

	h.CurrentDef += i.BonusDef
	h.CurrentDmg += i.BonusDmg
	h.CurrentHp += i.BonusHp
	h.CurrentSpd += i.BonusSpd
}

func (h *Hero) UnequipItem(slot ItemSlot) {
	var item *Item

	switch slot {
	case weapon:
		if h.weapon == nil {
			return
		}
		item = h.weapon
	case helmet:
		if h.head == nil {
			return
		}
		item = h.head
	case body:
		if h.body == nil {
			return
		}
		item = h.body
	case boots:
		if h.feeet == nil {
			return
		}
		item = h.feeet
	case amulet:
		if h.amulet == nil {
			return
		}
		item = h.amulet
	case ring:
		if h.ring == nil {
			return
		}
		item = h.ring
	}

	h.CurrentDef -= item.BonusDef
	h.CurrentDmg -= item.BonusDmg
	h.CurrentHp -= item.BonusHp
	h.CurrentSpd -= item.BonusSpd
}

func (h *Hero) EquipedItems() []string {
	str := make([]string, 0)
	var weapon string
	if h.weapon == nil {
		weapon = "Nenhuma"
	} else {
		weapon = h.weapon.Name
	}
	var head string
	if h.head == nil {
		head = "Nenhum"
	} else {
		head = h.head.Name
	}
	var body string
	if h.body == nil {
		body = "Nenhuma"
	} else {
		body = h.body.Name
	}
	var feet string
	if h.feeet == nil {
		feet = "Nenhuma"
	} else {
		feet = h.feeet.Name
	}
	var ring string
	if h.ring == nil {
		ring = "Nenhum"
	} else {
		ring = h.ring.Name
	}
	var amulet string
	if h.amulet == nil {
		amulet = "Nenhuma"
	} else {
		amulet = h.amulet.Name
	}
	str = append(str, fmt.Sprintf("Arma: %v\n", weapon))
	str = append(str, fmt.Sprintf("Cabeça: %v\n", head))
	str = append(str, fmt.Sprintf("Armadura: %v\n", body))
	str = append(str, fmt.Sprintf("Botas: %v\n", feet))
	str = append(str, fmt.Sprintf("Anel: %v\n", ring))
	str = append(str, fmt.Sprintf("Amuleto: %v\n", amulet))
	return str
}
