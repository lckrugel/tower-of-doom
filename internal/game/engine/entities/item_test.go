package entities

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
)

// Item tiers come from the asset; an absent field (0 in JSON) means the
// first tier.
func TestNewItemFromAssetTier(t *testing.T) {
	tests := []struct {
		name     string
		asset    game.ItemAsset
		wantTier int
	}{
		{"absent tier defaults to 1", game.ItemAsset{ID: "a", Name: "A", Slot: "weapon"}, 1},
		{"explicit tier is honored", game.ItemAsset{ID: "b", Name: "B", Slot: "weapon", Tier: 3}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewItemFromAsset(tt.asset).Tier; got != tt.wantTier {
				t.Errorf("Tier = %d, want %d", got, tt.wantTier)
			}
		})
	}
}

// BuyPrice is what a merchant charges for the item: its full value
// (bonuses ×50, durability-scaled; indestructible items are always full).
func TestItemBuyPrice(t *testing.T) {
	tests := []struct {
		name  string
		asset game.ItemAsset
		use   int // durability spent before pricing
		want  int
	}{
		{"full durability is full value",
			game.ItemAsset{ID: "s", Name: "S", Slot: "weapon", BonusDMG: 5, Durability: 50}, 0, 250},
		{"worn durability scales the price",
			game.ItemAsset{ID: "s", Name: "S", Slot: "weapon", BonusDMG: 5, Durability: 50}, 25, 125},
		{"indestructible items never lose value",
			game.ItemAsset{ID: "r", Name: "R", Slot: "ring", BonusHP: 5, Durability: -1}, 0, 250},
		{"a zero-bonus item is free",
			game.ItemAsset{ID: "z", Name: "Z", Slot: "weapon", Durability: 50}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := NewItemFromAsset(tt.asset)
			item.CurrentDurability -= tt.use

			if got := item.BuyPrice(); got != tt.want {
				t.Errorf("BuyPrice = %d, want %d", got, tt.want)
			}
		})
	}
}

// SellValue is what a merchant pays: half the value, floored at 1 — even
// junk sells for something.
func TestItemSellValue(t *testing.T) {
	tests := []struct {
		name  string
		asset game.ItemAsset
		use   int
		want  int
	}{
		{"full durability sells for half",
			game.ItemAsset{ID: "s", Name: "S", Slot: "weapon", BonusDMG: 5, Durability: 50}, 0, 125},
		{"worn durability scales the offer",
			game.ItemAsset{ID: "s", Name: "S", Slot: "weapon", BonusDMG: 5, Durability: 50}, 25, 62},
		{"indestructible items sell high",
			game.ItemAsset{ID: "r", Name: "R", Slot: "ring", BonusHP: 5, Durability: -1}, 0, 125},
		{"worthless items still sell for 1",
			game.ItemAsset{ID: "z", Name: "Z", Slot: "weapon", Durability: 50}, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := NewItemFromAsset(tt.asset)
			item.CurrentDurability -= tt.use

			if got := item.SellValue(); got != tt.want {
				t.Errorf("SellValue = %d, want %d", got, tt.want)
			}
		})
	}
}
