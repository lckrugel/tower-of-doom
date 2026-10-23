package entities

import (
	"github.com/lckrugel/tower-of-doom/internal/game"
)

type Monster struct {
	ID         string
	Name       string
	Difficulty int
	// Status base do mosntro
	BaseMaxHP int
	BaseDMG   int
	BaseSPD   int
	BaseDEF   int
	// Vida atual, persistida
	CurrentHP int
}

func NewMonsterFromAsset(asset game.MonsterAsset) Monster {
	return Monster{
		ID:         asset.ID,
		Name:       asset.Name,
		Difficulty: asset.Difficulty,
		BaseMaxHP:  asset.HP,
		CurrentHP:  asset.HP,
		BaseDMG:    asset.Damage,
		BaseSPD:    asset.Speed,
		BaseDEF:    asset.Defense,
	}
}

func (m *Monster) Damage() int        { return m.BaseDMG }
func (m *Monster) Speed() int         { return m.BaseSPD }
func (m *Monster) MaxHP() int         { return m.BaseMaxHP }
func (m *Monster) Defense() int       { return m.BaseDEF }
func (m *Monster) CritThreshold() int { return 10 }

func (m *Monster) TakeDmg(dmg int) int {
	dmgTaken := max(0, min(dmg, m.CurrentHP))
	m.CurrentHP -= dmgTaken
	return dmgTaken
}

func (m *Monster) Heal(hp int) int {
	healDone := hp
	if m.CurrentHP+hp > m.BaseMaxHP {
		healDone = m.BaseMaxHP - m.CurrentHP
		m.CurrentHP = m.BaseMaxHP
	} else {
		m.CurrentHP += hp
	}

	return healDone
}

func (m *Monster) IsAlive() bool {
	return m.CurrentHP > 0
}

func (m *Monster) GetKillXp() int {
	return 4 * m.Difficulty
}
