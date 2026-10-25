package entities

import (
	"math"

	"github.com/lckrugel/tower-of-doom/internal/game"
)

type Monster struct {
	ID         string
	name       string
	Difficulty int
	// Status base do mosntro
	BaseMaxHP int
	BaseDMG   int
	BaseSPD   int
	BaseDEF   int
	BaseCRIT  int
	// Vida atual, persistida
	CurrentHP int
	// moves is the monster's Combat 2.0 move set, resolved by the CALLER
	// from the asset's move refs (registry lookups stay outside
	// entities). Never empty: monsters without moves fall back to the
	// canonical basic attack.
	moves []Move
}

// NewMonsterFromAsset builds a monster from its asset. `moves` carries
// the monster's move set, resolved by the caller from the asset's move
// refs (per-move Weight = the ref's per-monster AI weight). Passing none
// — what every current caller does — gives the monster the canonical
// basic attack, matching the asset-level rule that a monster without a
// moves field uses basic_attack.
func NewMonsterFromAsset(asset game.MonsterAsset, moves ...Move) Monster {
	m := Monster{
		ID:         asset.ID,
		name:       asset.Name,
		Difficulty: asset.Difficulty,
		BaseMaxHP:  asset.HP,
		CurrentHP:  asset.HP,
		BaseDMG:    asset.Damage,
		BaseSPD:    asset.Speed,
		BaseDEF:    asset.Defense,
		BaseCRIT:   asset.Crit,
	}
	if len(moves) > 0 {
		m.moves = append([]Move(nil), moves...)
	} else {
		m.moves = []Move{BasicAttack()}
	}
	return m
}

func (m *Monster) Damage() int        { return m.BaseDMG }
func (m *Monster) Speed() int         { return m.BaseSPD }
func (m *Monster) MaxHP() int         { return m.BaseMaxHP }
func (m *Monster) Defense() int       { return m.BaseDEF }
func (m *Monster) CritThreshold() int { return m.BaseCRIT }
func (m *Monster) Name() string       { return m.name }

func (m *Monster) CurrentHPPercent() int {
	return int(math.Floor(float64(m.CurrentHP) / float64(m.MaxHP()) * 100))
}

// HP reports the exact current HP value (Combatant interface;
// CurrentHPPercent is its percentage view).
func (m *Monster) HP() int { return m.CurrentHP }

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

// Moves returns the monster's move set in asset order — never empty (the
// fallback is the canonical basic attack). Each move's Weight carries the
// monster's own AI weight for that move. The returned slice is a copy.
func (m *Monster) Moves() []Move {
	moves := make([]Move, len(m.moves))
	copy(moves, m.moves)
	return moves
}

// GetKillXp reports the instant XP reward for defeating the monster.
// The reward grows with difficulty^1.5 — the same exponent as the hero's
// XP-cost curve (`requiredXpGrowth`) — so the time to level stays flat
// per floor tier instead of ballooning as content difficulty rises.
func (m *Monster) GetKillXp() int {
	return int(math.Round(4 * math.Pow(float64(m.Difficulty), 1.5)))
}
