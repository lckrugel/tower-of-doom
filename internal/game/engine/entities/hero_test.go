package entities

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
)

func TestHeroGiveXP(t *testing.T) {
	tests := []struct {
		name    string
		givenXP int
		wantLVL int
		wantXP  int
	}{
		{"below level threshold", 50, 1, 50},
		{"exact level up", 100, 2, 0},
		{"multiple level ups", 550, 3, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewPlayer()
			h.GiveXP(tt.givenXP)
			if h.Lvl != tt.wantLVL {
				t.Errorf("level = %d, want %d", h.Lvl, tt.wantLVL)
			}
			if h.Xp != tt.wantXP {
				t.Errorf("xp = %d, want %d", h.Xp, tt.wantXP)
			}
		})
	}
}

func TestHeroLevelUp_CurrentHP(t *testing.T) {
	tests := []struct {
		name          string
		currentHP     int
		wantLVL       int
		wantMaxHP     int
		wantCurrentHP int
	}{
		{"full health level up", baseHp, 2, baseHp + hpBylvl, baseHp + hpBylvl},
		{"half health level up", baseHp / 2, 2, baseHp + hpBylvl, (baseHp + hpBylvl) / 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewPlayer()
			h.CurrentHP = tt.currentHP
			h.levelUp()
			if h.Lvl != tt.wantLVL {
				t.Errorf("level = %d, want %d", h.Lvl, tt.wantLVL)
			}
			if h.BaseMaxHP != tt.wantMaxHP {
				t.Errorf("baseMaxHP = %d, want = %d", h.BaseMaxHP, tt.wantMaxHP)
			}
			if h.CurrentHP != tt.wantCurrentHP {
				t.Errorf("currentHP = %d, want = %d", h.CurrentHP, tt.wantCurrentHP)
			}
		})
	}
}

func TestHeroTakeDMG(t *testing.T) {
	tests := []struct {
		name         string
		dmgReceived  int
		wantDMGTaken int
	}{
		{"receives exact HP as dmg", 100, 100},
		{"receives dmg greater then HP", 150, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewPlayer()
			dmgTaken := h.TakeDmg(tt.dmgReceived)
			if h.CurrentHP < 0 {
				t.Errorf("CurrentHP = %d, want 0", h.CurrentHP)
			}
			if dmgTaken != tt.wantDMGTaken {
				t.Errorf("DMGTaken = %d, want = %d", dmgTaken, tt.wantDMGTaken)
			}
		})
	}
}

func TestHeroHeal(t *testing.T) {
	item := NewItemFromAsset(game.ItemAsset{
		ID:      "collar",
		Name:    "Test Collar",
		Slot:    "amulet",
		BonusHP: 100,
	})
	tests := []struct {
		name          string
		currentHP     int
		healReceived  int
		item          *Item
		wantCurrentHP int
		wantHealDone  int
	}{
		{"heals a fixed amount", 50, 20, nil, 70, 20},
		{"heal is greater than HP", 50, 200, nil, 100, 50},
		{"heal considers item bonus HP", 50, 200, &item, 200, 150},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewPlayer()
			h.EquipItem(tt.item)
			t.Logf("MaxHP = %d | CurrentHP = %d", h.MaxHP(), h.CurrentHP)
			h.CurrentHP = tt.currentHP
			actualHeal := h.Heal(tt.healReceived)
			if h.CurrentHP != tt.wantCurrentHP {
				t.Errorf("CurrentHP = %d, want = %d", h.CurrentHP, tt.wantCurrentHP)
			}
			if actualHeal != tt.wantHealDone {
				t.Errorf("HealDone = %d, want = %d", actualHeal, tt.wantHealDone)
			}
		})
	}
}
