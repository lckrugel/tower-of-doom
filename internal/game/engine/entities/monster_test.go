package entities

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
)

func newTestMonster() Monster {
	return NewMonsterFromAsset(game.MonsterAsset{
		ID:         "rat",
		Name:       "Test Rat",
		Difficulty: 3,
		HP:         30,
		Damage:     3,
		Speed:      10,
		Defense:    0,
	})
}

func TestMonsterTakeDmg(t *testing.T) {
	tests := []struct {
		name          string
		damage        int
		wantDMGTaken  int
		wantCurrentHP int
		wantAlive     bool
	}{
		{"takes partial damage", 10, 10, 20, true},
		{"takes exactly remaining HP", 30, 30, 0, false},
		{"overkill clamps to current HP", 99, 30, 0, false},
		{"negative damage does nothing", -5, 0, 30, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestMonster()
			dmgTaken := m.TakeDmg(tt.damage)

			if dmgTaken != tt.wantDMGTaken {
				t.Errorf("DMGTaken = %d, want = %d", dmgTaken, tt.wantDMGTaken)
			}
			if m.CurrentHP != tt.wantCurrentHP {
				t.Errorf("CurrentHP = %d, want %d", m.CurrentHP, tt.wantCurrentHP)
			}
			if m.IsAlive() != tt.wantAlive {
				t.Errorf("IsAlive = %v, want %v", m.IsAlive(), tt.wantAlive)
			}
		})
	}

	t.Run("dead monster stays dead", func(t *testing.T) {
		m := newTestMonster()
		m.TakeDmg(30)
		dmgTaken := m.TakeDmg(10)

		if dmgTaken != 0 || m.CurrentHP != 0 || m.IsAlive() {
			t.Errorf("damage on dead monster: taken = %d, HP = %d, alive = %v, want 0/0/false",
				dmgTaken, m.CurrentHP, m.IsAlive())
		}
	})
}

func TestMonsterHeal(t *testing.T) {
	tests := []struct {
		name          string
		currentHP     int
		healReceived  int
		wantCurrentHP int
		wantHealDone  int
	}{
		{"heals within max", 10, 5, 15, 5},
		{"caps at max", 10, 100, 30, 20},
		{"full health heals nothing", 30, 7, 30, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestMonster()
			m.CurrentHP = tt.currentHP

			healed := m.Heal(tt.healReceived)

			if m.CurrentHP != tt.wantCurrentHP {
				t.Errorf("CurrentHP = %d, want %d", m.CurrentHP, tt.wantCurrentHP)
			}
			if healed != tt.wantHealDone {
				t.Errorf("healDone = %d, want %d", healed, tt.wantHealDone)
			}
		})
	}
}

func TestMonsterGetKillXp(t *testing.T) {
	tests := []struct {
		name       string
		difficulty int
		wantXp     int
	}{
		{"pi-point kill xp", 1, 4},
		{"standard goblin", 3, 12},
		{"orc", 5, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMonsterFromAsset(game.MonsterAsset{
				ID:         "m",
				Name:       "Test Monstro",
				Difficulty: tt.difficulty,
			})

			if xp := m.GetKillXp(); xp != tt.wantXp {
				t.Errorf("killXp = %d, want %d", xp, tt.wantXp)
			}
		})
	}
}
