package game

import "fmt"

type Monster struct {
	Id         int    `json:"id"`
	Tag        string `json:"tag"`
	Name       string `json:"name"`
	Difficulty int    `json:"difficulty"`
	MaxHp      int    `json:"hp"`
	BaseDmg    int    `json:"bonusDmg"`
	Spd        int    `json:"spd"`
	Def        int    `json:"def"`
	CurrentHp  int
	CurrentDmg int
	CurrentSpd int
	CurrentDef int
}

func (m *Monster) Init() {
	m.CurrentDef = m.Def
	m.CurrentDmg = m.BaseDmg
	m.CurrentHp = m.MaxHp
	m.CurrentSpd = m.Spd
}

func (m *Monster) attack(target Entity) *GameMessage {
	roll := RollD10()
	dmg := m.CurrentDmg + roll
	if roll >= m.critThreshold() {
		dmg = dmg * 2
	}
	dmgDone := target.takeDmg(dmg)
	return &GameMessage{
		Type:    CombatMessage,
		Content: fmt.Sprintf("%s causou %d de dano\n", m.Name, dmgDone),
		Data: map[string]any{
			"dmg":    dmgDone,
			"isCrit": false,
		},
	}
}

func (m *Monster) takeDmg(dmg int) int {
	dmgTaken := dmg
	if dmgTaken >= m.CurrentHp {
		m.CurrentHp = 0
	}
	m.CurrentHp -= dmgTaken
	return dmgTaken
}

func (m *Monster) heal(hp int) *GameMessage {
	healDone := hp
	if m.CurrentHp+hp > m.MaxHp {
		healDone = m.MaxHp - m.CurrentHp
		m.CurrentHp = m.MaxHp
	} else {
		m.CurrentHp += hp
	}

	return &GameMessage{
		Type:    InfoMessage,
		Content: fmt.Sprintf("%s curou em %d", m.Name, healDone),
		Data: map[string]any{
			"healed":    healDone,
			"maxHp":     m.MaxHp,
			"currentHp": m.CurrentHp,
		},
	}
}

func (m *Monster) isDead() bool {
	return m.CurrentHp <= 0
}

func (m *Monster) critThreshold() int {
	return 10
}

func (m *Monster) getKillXp() int {
	return 4 * m.Difficulty
}

func (m *Monster) String() []string {
	str := make([]string, 0)
	str = append(str, fmt.Sprintf("%s\n", m.Name))
	str = append(str, fmt.Sprintf("Vida: %d | Defesa: %d, | Ataque: %d\n", m.MaxHp, m.CurrentDef, m.CurrentDmg))
	return str
}

func (m *Monster) Status() *GameMessage {
	return &GameMessage{
		Type:    StatusMessage,
		Content: fmt.Sprintf("%s [DR:%d]\nVida: %d/%d | Defesa: %d, | Ataque: %d\n", m.Name, m.Difficulty, m.CurrentHp, m.MaxHp, m.CurrentDef, m.CurrentDmg),
		Data: map[string]any{
			"name":       m.Name,
			"difficulty": m.Difficulty,
			"maxHP":      m.MaxHp,
			"currentHp":  m.CurrentHp,
			"dmg":        fmt.Sprintf("1d10+%d", m.CurrentDmg),
			"def":        m.CurrentDef,
			"spd":        m.CurrentSpd,
		},
	}
}
