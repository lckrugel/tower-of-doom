package entities

import "testing"

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

func TestLevelUpStats(t *testing.T) {
	tests := []struct {
		name          string
		currentHP     int
		wantLVL       int
		wantMaxHP     int
		wantCurrentHP int
		wantDMG       int
	}{
		{"level up increases status", baseHp, 2, baseHp + hpBylvl, baseHp + hpBylvl, baseDmg + dmgByLvl},
		{"full health level up", baseHp, 2, baseHp + hpBylvl, baseHp + hpBylvl, baseDmg + dmgByLvl},
		{"half health level up", baseHp / 2, 2, baseHp + hpBylvl, (baseHp + hpBylvl) / 2, baseDmg + dmgByLvl},
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
			}
		})
	}
}
