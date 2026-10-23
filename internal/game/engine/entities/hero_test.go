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
		name string

		currentHP int
		baseHP
	}
}
