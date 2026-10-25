package entities

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
)

// newTestWeapon builds a weapon item carrying the given granted moves
// (resolved by the caller — entities never touch the registry).
func newTestWeapon(granted ...Move) *Item {
	item := NewItemFromAsset(game.ItemAsset{
		ID: "sword", Name: "Sword", Slot: weapon, BonusDMG: 5, Durability: 10,
	}, granted...)
	return &item
}

// NewMoveFromAsset maps every asset field onto the runtime move,
// including the optional status (pointer in the asset, value in the
// runtime type).
func TestNewMoveFromAsset(t *testing.T) {
	m := NewMoveFromAsset(game.MoveAsset{
		ID: "venom_bite", Name: "Venom Bite", Tier: 1,
		DmgModifier: 80, CritBonus: 5, Cooldown: 2, Weight: 60,
		Status: &game.MoveStatusAsset{ID: "poison", Chance: 60, Target: "enemy"},
	})

	if m.ID != "venom_bite" || m.Name != "Venom Bite" || m.Tier != 1 ||
		m.DmgModifier != 80 || m.CritBonus != 5 || m.Cooldown != 2 || m.Weight != 60 {
		t.Errorf("move mapping wrong: %+v", m)
	}
	if !m.HasStatus() {
		t.Fatal("HasStatus = false, want true (asset carries a status)")
	}
	if m.Status.ID != "poison" || m.Status.Chance != 60 || m.Status.Target != TargetEnemy {
		t.Errorf("status mapping wrong: %+v", m.Status)
	}
	if m.TargetSelf() {
		t.Error("TargetSelf = true, want false (enemy-targeted status)")
	}
}

func TestNewMoveFromAssetWithoutStatus(t *testing.T) {
	m := NewMoveFromAsset(game.MoveAsset{
		ID: "heavy_slash", Name: "Heavy Slash", Tier: 2,
		DmgModifier: 175, Cooldown: 2, Weight: 60,
	})

	if m.HasStatus() {
		t.Errorf("HasStatus = true, want false (asset has no status): %+v", m.Status)
	}
	if m.TargetSelf() {
		t.Error("TargetSelf = true, want false (no status at all)")
	}
}

// TargetSelf distinguishes self-buffs (guard) from enemy debuffs.
func TestMoveTargetSelf(t *testing.T) {
	m := NewMoveFromAsset(game.MoveAsset{
		ID: "guard_strike", Name: "Guard Strike", DmgModifier: 75, Weight: 50,
		Status: &game.MoveStatusAsset{ID: "guard", Chance: 100, Target: "self"},
	})

	if !m.TargetSelf() {
		t.Error("TargetSelf = false, want true (self-targeted status)")
	}
}

// NewStatusFromAsset flattens the asset's effect: a DoT fills DotAmount,
// a stat modifier fills Stat + StatAmount.
func TestNewStatusFromAsset(t *testing.T) {
	tests := []struct {
		name       string
		asset      game.StatusAsset
		wantDot    int
		wantStat   StatKind
		wantAmount int
		wantKind   StatusKind
	}{
		{"dot status",
			game.StatusAsset{ID: "poison", Name: "Poison", Kind: "debuff",
				Effect: game.StatusEffectAsset{Type: "dot", Amount: 3}, Duration: 3},
			3, "", 0, StatusDebuff},
		{"stat status",
			game.StatusAsset{ID: "guard", Name: "Guard", Kind: "buff",
				Effect: game.StatusEffectAsset{Type: "stat", Stat: "def", Amount: 5}, Duration: 2},
			0, StatDEF, 5, StatusBuff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStatusFromAsset(tt.asset)

			if s.ID != tt.asset.ID || s.Name != tt.asset.Name || s.Duration != tt.asset.Duration {
				t.Errorf("identity mapping wrong: %+v", s)
			}
			if s.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", s.Kind, tt.wantKind)
			}
			if s.DotAmount != tt.wantDot || s.Stat != tt.wantStat || s.StatAmount != tt.wantAmount {
				t.Errorf("effect = (dot %d, stat %q+%d), want (dot %d, stat %q+%d)",
					s.DotAmount, s.Stat, s.StatAmount, tt.wantDot, tt.wantStat, tt.wantAmount)
			}
			if s.IsDot() != (tt.wantDot > 0) {
				t.Errorf("IsDot = %v, want %v", s.IsDot(), tt.wantDot > 0)
			}
		})
	}
}

// The canonical basic attack: a neutral, spammable, status-less strike.
// Both the entity-level fallback and the embedded asset ship these
// values; they must never drift apart.
func TestBasicAttack(t *testing.T) {
	basic := BasicAttack()

	if basic.ID != "basic_attack" || basic.Name != "Basic Attack" ||
		basic.Tier != 1 || basic.DmgModifier != 100 || basic.CritBonus != 0 ||
		basic.Cooldown != 0 || basic.Weight != 100 {
		t.Errorf("canonical basic attack drifted: %+v", basic)
	}
	if basic.HasStatus() {
		t.Error("basic attack carries a status, want none")
	}
}
