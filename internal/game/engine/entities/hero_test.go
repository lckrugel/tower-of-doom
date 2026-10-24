package entities

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
)

// newTestItem cria um item de teste pelo asset informado.
func newTestItem(asset game.ItemAsset) *Item {
	item := NewItemFromAsset(asset)
	return &item
}

// Curva de XP: o custo para subir do nível atual é 100 * max(Lvl-1, 1)^1.5.
// Custos por nível: L1→2: 100, L2→3: 100, L3→4: 282, L4→5: 519, L5→6: 800,
// L6→7: 1118, L7→8: 1469, L8→9: 1852, L9→10: 2262.
//
// Nota (decisão fechada): L2→3 custa o mesmo que L1→2 por causa do
// max(Lvl-1, 1) — "níveis iniciais baratos", mantido de propósito.
func TestHeroGiveXP(t *testing.T) {
	tests := []struct {
		name    string
		givenXP int
		wantLVL int
		wantXP  int
	}{
		{"below level threshold", 50, 1, 50},
		{"exactly reaches level 2", 100, 2, 0},
		{"exactly finishes level 3", 482, 4, 0},
		{"one xp short of finishing level 3", 481, 3, 281},
		{"multiple level ups", 550, 4, 68},
		{"many level ups", 10_000, 10, 1_498},
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

func TestHeroGiveXP_DeadHeroIsNoOp(t *testing.T) {
	h := NewPlayer()
	h.TakeDmg(h.BaseMaxHP)
	h.GiveXP(1_000)

	if h.Lvl != 1 || h.Xp != 0 {
		t.Errorf("dead hero gained XP: level = %d, xp = %d, want 1/0", h.Lvl, h.Xp)
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

// O % de vida atual deve ser preservado quando o HP máximo muda — incluindo
// quando há itens de BonusHP equipados (o máximo muda pelo item também).
func TestHeroLevelUp_RescalesHPWithEquippedItem(t *testing.T) {
	h := NewPlayer()
	h.EquipItem(newTestItem(game.ItemAsset{
		ID:      "collar",
		Name:    "Test Collar",
		Slot:    amulet,
		BonusHP: 50,
	}))
	// Full: 150/150 → sobe de nível → novo máximo 160.
	h.levelUp()

	if h.CurrentHP != 160 {
		t.Errorf("currentHP = %d, want 160 (new MaxHP at full health)", h.CurrentHP)
	}
	if h.MaxHP() != 160 {
		t.Errorf("maxHP = %d, want 160", h.MaxHP())
	}
}

// Table: hpPercent = floor(CurrentHP / MaxHP * 100).
func TestHeroCurrentHPPercent(t *testing.T) {
	tests := []struct {
		name      string
		currentHP int
		itemBonus int
		wantPct   int
	}{
		{"full health", baseHp, 0, 100},
		{"half health", baseHp / 2, 0, 50},
		{"third of health", baseHp / 3, 0, 33},
		{"fraction floors down", 99, 0, 99},
		{"dead", 0, 0, 0},
		{"item bonus hp counts in max", 50, 50, 33}, // 50/150
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewPlayer()
			if tt.itemBonus != 0 {
				h.EquipItem(newTestItem(game.ItemAsset{
					ID:      "collar",
					Name:    "Test Collar",
					Slot:    amulet,
					BonusHP: tt.itemBonus,
				}))
			}
			h.CurrentHP = tt.currentHP

			if got := h.CurrentHPPercent(); got != tt.wantPct {
				t.Errorf("CurrentHPPercent = %d, want %d", got, tt.wantPct)
			}
		})
	}
}

func TestHeroTakeDmg(t *testing.T) {
	tests := []struct {
		name          string
		dmgReceived   int
		wantDMGTaken  int
		wantCurrentHP int
		wantAlive     bool
	}{
		{"takes partial damage", 30, 30, 70, true},
		{"takes exactly max HP", 100, 100, 0, false},
		{"overkill clamps to current HP", 150, 100, 0, false},
		{"negative damage does nothing", -5, 0, 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewPlayer()
			dmgTaken := h.TakeDmg(tt.dmgReceived)

			if dmgTaken != tt.wantDMGTaken {
				t.Errorf("DMGTaken = %d, want = %d", dmgTaken, tt.wantDMGTaken)
			}
			if h.CurrentHP != tt.wantCurrentHP {
				t.Errorf("CurrentHP = %d, want %d", h.CurrentHP, tt.wantCurrentHP)
			}
			if h.IsAlive() != tt.wantAlive {
				t.Errorf("IsAlive = %v, want %v", h.IsAlive(), tt.wantAlive)
			}
		})
	}

	t.Run("dead hero stays dead", func(t *testing.T) {
		h := NewPlayer()
		h.TakeDmg(100)
		dmgTaken := h.TakeDmg(50)

		if dmgTaken != 0 || h.CurrentHP != 0 || h.IsAlive() {
			t.Errorf("damage on dead hero: taken = %d, HP = %d, alive = %v, want 0/0/false",
				dmgTaken, h.CurrentHP, h.IsAlive())
		}
	})
}

// StartersItens... somatório de atributos deve refletir todos os slots equipados.
func TestHeroEquippedItemStats(t *testing.T) {
	h := NewPlayer()
	h.EquipItem(newTestItem(game.ItemAsset{ID: "sword", Name: "Sword", Slot: weapon, BonusDMG: 5, BonusSPD: 1}))
	h.EquipItem(newTestItem(game.ItemAsset{ID: "helm", Name: "Helm", Slot: helmet, BonusDEF: 2}))
	h.EquipItem(newTestItem(game.ItemAsset{ID: "plate", Name: "Plate", Slot: body, BonusHP: 30, BonusDEF: 1}))

	if got := h.Damage(); got != 5 {
		t.Errorf("Damage = %d, want 5", got)
	}
	if got := h.Speed(); got != 11 {
		t.Errorf("Speed = %d, want 11", got)
	}
	if got := h.Defense(); got != 3 {
		t.Errorf("Defense = %d, want 3", got)
	}
	if got := h.MaxHP(); got != 130 {
		t.Errorf("MaxHP = %d, want 130", got)
	}
}

func TestHeroXpToLevel(t *testing.T) {
	h := NewPlayer()

	if got := h.XpToLevel(); got != 100 {
		t.Errorf("XpToLevel at fresh hero = %d, want 100", got)
	}

	h.GiveXP(40)
	if got := h.XpToLevel(); got != 60 {
		t.Errorf("XpToLevel after 40 XP = %d, want 60", got)
	}

	// XP carries over across level ups: 40 banked + 100 gained consumes the
	// 100 cost of Lvl 2 and leaves a 40 carryover toward Lvl 3.
	h.GiveXP(100) // levels up, 40 carry over
	if got := h.XpToLevel(); got != 60 {
		t.Errorf("XpToLevel after level up = %d, want 60 (carryover included)", got)
	}
}

func TestHeroHeal(t *testing.T) {
	tests := []struct {
		name          string
		currentHP     int
		healReceived  int
		itemBonusHP   int
		wantCurrentHP int
		wantHealDone  int
	}{
		{"heals a fixed amount", 50, 20, 0, 70, 20},
		{"heal is greater than HP", 50, 200, 0, 100, 50},
		{"full health heals nothing", 100, 50, 0, 100, 0},
		{"heal considers item bonus HP", 50, 200, 100, 200, 150},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewPlayer()
			if tt.itemBonusHP != 0 {
				h.EquipItem(newTestItem(game.ItemAsset{
					ID:      "collar",
					Name:    "Test Collar",
					Slot:    amulet,
					BonusHP: tt.itemBonusHP,
				}))
			}
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

// Equipar/unequip itens com BonusHP reescalan o CurrentHP para preservar
// o % de vida — relativo ao MaxHP() de antes, não ao BaseMaxHP.
func TestHeroEquipRescalesHP(t *testing.T) {
	tests := []struct {
		name          string
		damageTaken   int
		itemBonusHP   int
		wantCurrentHP int
		wantMaxHP     int
	}{
		{"full health keeps full", 0, 50, 150, 150},
		{"fraction preserved when bonus lands", 75, 50, 37, 150},
		{"dead hero stays dead", 100, 50, 0, 150},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewPlayer()
			h.TakeDmg(tt.damageTaken)

			h.EquipItem(newTestItem(game.ItemAsset{
				ID:      "collar",
				Name:    "Test Collar",
				Slot:    amulet,
				BonusHP: tt.itemBonusHP,
			}))

			if h.CurrentHP != tt.wantCurrentHP {
				t.Errorf("CurrentHP = %d, want %d", h.CurrentHP, tt.wantCurrentHP)
			}
			if h.MaxHP() != tt.wantMaxHP {
				t.Errorf("MaxHP = %d, want %d", h.MaxHP(), tt.wantMaxHP)
			}
		})
	}

	t.Run("equipping item without hp bonus leaves HP untouched", func(t *testing.T) {
		h := NewPlayer()
		h.TakeDmg(40)

		h.EquipItem(newTestItem(game.ItemAsset{ID: "sword", Name: "Test Sword", Slot: weapon}))

		if h.CurrentHP != 60 || h.MaxHP() != 100 {
			t.Errorf("CurrentHP = %d, MaxHP = %d, want 60/100", h.CurrentHP, h.MaxHP())
		}
	})

	t.Run("equipping over an occupied slot returns the replaced item", func(t *testing.T) {
		h := NewPlayer()
		first := newTestItem(game.ItemAsset{ID: "collar_a", Name: "Collar A", Slot: amulet, BonusHP: 50})
		second := newTestItem(game.ItemAsset{ID: "collar_b", Name: "Collar B", Slot: amulet, BonusHP: 100})

		h.EquipItem(first)
		replaced := h.EquipItem(second)

		if replaced != first {
			t.Errorf("replaced item = %v, want the first collar", replaced)
		}
		if h.Amulet != second {
			t.Errorf("unequipped item = %v, want the unequipped collar", h.Amulet)
		}
		if h.MaxHP() != 200 {
			t.Errorf("MaxHP = %d, want 200 (only the second collar equipped)", h.MaxHP())
		}
	})

	t.Run("swapping hp items mid-health preserves the fraction", func(t *testing.T) {
		h := NewPlayer()
		first := newTestItem(game.ItemAsset{ID: "collar_a", Name: "Collar A", Slot: amulet, BonusHP: 50})
		second := newTestItem(game.ItemAsset{ID: "collar_b", Name: "Collar B", Slot: amulet, BonusHP: 100})

		h.EquipItem(first) // 150 no máximo
		h.TakeDmg(75)      // 75/150 = 50%
		h.EquipItem(second)

		if h.CurrentHP != 100 || h.MaxHP() != 200 {
			t.Errorf("CurrentHP = %d, MaxHP = %d, want 100/200 (50%% of new max)", h.CurrentHP, h.MaxHP())
		}
	})
}

func TestHeroUnequipRescalesHP(t *testing.T) {
	t.Run("unequipping an empty slot is a no-op", func(t *testing.T) {
		h := NewPlayer()

		removed := h.UnequipItem(boots)

		if removed != nil {
			t.Errorf("unequipped = %v, want nil", removed)
		}
		if h.CurrentHP != 100 || h.MaxHP() != 100 {
			t.Errorf("CurrentHP = %d, MaxHP = %d, want unchanged 100/100", h.CurrentHP, h.MaxHP())
		}
	})

	t.Run("unequipping hp item preserves the fraction", func(t *testing.T) {
		h := NewPlayer()
		h.EquipItem(newTestItem(game.ItemAsset{ID: "collar", Name: "Test Collar", Slot: amulet, BonusHP: 50}))
		h.TakeDmg(75) // 75/150 = 50%

		removed := h.UnequipItem(amulet)

		if removed == nil {
			t.Fatal("unequipped = nil, want the collar")
		}
		if h.Amulet != nil {
			t.Errorf("unequipped = %v, want the unequipped collar", h.Amulet)
		}
		if h.CurrentHP != 50 || h.MaxHP() != 100 {
			t.Errorf("CurrentHP = %d, MaxHP = %d, want 50/100 (50%% of new max)", h.CurrentHP, h.MaxHP())
		}
	})

	t.Run("unequipping item without hp bonus leaves HP untouched", func(t *testing.T) {
		h := NewPlayer()
		h.EquipItem(newTestItem(game.ItemAsset{ID: "sword", Name: "Test Sword", Slot: weapon}))
		h.TakeDmg(40)

		h.UnequipItem(weapon)

		if h.CurrentHP != 60 || h.MaxHP() != 100 {
			t.Errorf("CurrentHP = %d, MaxHP = %d, want 60/100", h.CurrentHP, h.MaxHP())
		}
	})
}
