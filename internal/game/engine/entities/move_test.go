package entities

import (
	"slices"
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
// runtime type) and the validated type word (typed constant).
func TestNewMoveFromAsset(t *testing.T) {
	m := NewMoveFromAsset(game.MoveAsset{
		ID: "venom_bite", Name: "Venom Bite", Tier: 1,
		DmgModifier: 80, CritBonus: 5, Cooldown: 2, Weight: 60, Type: "creature",
		Status: &game.MoveStatusAsset{ID: "poison", Chance: 60, Target: "enemy"},
	})

	if m.ID != "venom_bite" || m.Name != "Venom Bite" || m.Tier != 1 ||
		m.DmgModifier != 80 || m.CritBonus != 5 || m.Cooldown != 2 || m.Weight != 60 {
		t.Errorf("move mapping wrong: %+v", m)
	}
	if m.Type != MoveTypeCreature {
		t.Errorf("Type = %q, want %q", m.Type, MoveTypeCreature)
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

	if basic.ID != "basic_attack" || basic.Name != "Basic Strike" ||
		basic.Tier != 1 || basic.DmgModifier != 100 || basic.CritBonus != 0 ||
		basic.Cooldown != 0 || basic.Weight != 100 {
		t.Errorf("canonical basic attack drifted: %+v", basic)
	}
	if basic.Type != MoveTypeBoth {
		t.Errorf("basic attack type = %q, want %q (every combatant's fallback)", basic.Type, MoveTypeBoth)
	}
	if basic.HasStatus() {
		t.Error("basic attack carries a status, want none")
	}
}

// The usability pair says which CONSUMER may carry a move: a weapon's
// grant list (weapon or both) or a creature's move kit — a monster's
// set or the hero's learned set (creature or both).
func TestMoveUsabilityByType(t *testing.T) {
	tests := []struct {
		name          string
		move          Move
		wantByWeapons bool
		wantByCreats  bool
	}{
		{"weapon-only", Move{ID: "w", Type: MoveTypeWeapon}, true, false},
		{"creature-only", Move{ID: "c", Type: MoveTypeCreature}, false, true},
		{"both", Move{ID: "b", Type: MoveTypeBoth}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.move.UsableByWeapons(); got != tt.wantByWeapons {
				t.Errorf("UsableByWeapons = %v, want %v", got, tt.wantByWeapons)
			}
			if got := tt.move.UsableByCreatures(); got != tt.wantByCreats {
				t.Errorf("UsableByCreatures = %v, want %v", got, tt.wantByCreats)
			}
		})
	}
}

// FilterUnknownMoves is the shrine's offer filter: the tier-filtered
// registry pool minus everything the hero already knows, minus the
// canonical basic_attack (never offered — every combatant already fights
// with it as the fallback).
func TestFilterUnknownMoves(t *testing.T) {
	pool := []game.MoveAsset{
		{ID: "basic_attack", Name: "Basic Strike", Tier: 1, DmgModifier: 100, Weight: 100},
		{ID: "heavy_slash", Name: "Heavy Slash", Tier: 2, DmgModifier: 175, Cooldown: 2, Weight: 60},
		{ID: "quick_stab", Name: "Quick Stab", Tier: 1, DmgModifier: 60, CritBonus: 20, Weight: 100},
		{ID: "venom_bite", Name: "Venom Bite", Tier: 1, DmgModifier: 80, Cooldown: 2, Weight: 60},
	}

	t.Run("drops known moves and the canonical basic_attack", func(t *testing.T) {
		got := FilterUnknownMoves(pool, []Move{
			{ID: "quick_stab"}, {ID: "venom_bite"},
		})

		if len(got) != 1 || got[0].ID != "heavy_slash" {
			t.Errorf("FilterUnknownMoves = %+v, want [heavy_slash] (basic_attack never offered)", got)
		}
	})

	t.Run("dedups the known list by id", func(t *testing.T) {
		got := FilterUnknownMoves(pool, []Move{
			{ID: "quick_stab", Name: "Learned Copy"}, {ID: "quick_stab", Name: "Weapon Copy"},
		})

		for _, asset := range got {
			if asset.ID == "quick_stab" {
				t.Errorf("known move leaked through the filter: %+v", asset)
			}
		}
	})

	t.Run("preserves the pool's order", func(t *testing.T) {
		got := FilterUnknownMoves(pool, nil)

		if len(got) != 3 || got[0].ID != "heavy_slash" || got[1].ID != "quick_stab" || got[2].ID != "venom_bite" {
			t.Errorf("FilterUnknownMoves = %+v, want the pool's order (sorted by id) minus basic_attack", got)
		}
	})

	t.Run("the canonical basic_attack is always dropped", func(t *testing.T) {
		got := FilterUnknownMoves(pool, nil)

		for _, asset := range got {
			if asset.ID == BasicAttack().ID {
				t.Errorf("the canonical basic_attack leaked into the offer pool: %+v", asset)
			}
		}
	})

	t.Run("a known basic_attack is redundant with the always-drop", func(t *testing.T) {
		got := FilterUnknownMoves(pool, nil)

		if len(got) != 3 {
			t.Errorf("FilterUnknownMoves = %+v, want 3 moves (basic_attack still dropped)", got)
		}
	})

	t.Run("an empty pool stays empty", func(t *testing.T) {
		if got := FilterUnknownMoves(nil, nil); len(got) != 0 {
			t.Errorf("FilterUnknownMoves(nil, nil) = %+v, want empty", got)
		}
	})

	t.Run("everything filtered out is an empty pool", func(t *testing.T) {
		allKnown := []Move{
			{ID: "basic_attack"}, {ID: "heavy_slash"},
			{ID: "quick_stab"}, {ID: "venom_bite"},
		}
		if got := FilterUnknownMoves(pool, allKnown); len(got) != 0 {
			t.Errorf("FilterUnknownMoves = %+v, want empty (the shrine's nothing-new case)", got)
		}
	})

	t.Run("pure: the pool is not mutated", func(t *testing.T) {
		before := append([]game.MoveAsset(nil), pool...)
		FilterUnknownMoves(pool, []Move{{ID: "quick_stab"}, {ID: "venom_bite"}, {ID: "heavy_slash"}})

		if !slices.EqualFunc(pool, before, func(a, b game.MoveAsset) bool { return a.ID == b.ID }) {
			t.Errorf("the input pool was mutated: %+v", pool)
		}
	})
}
