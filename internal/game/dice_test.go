package game

import (
	"testing"
)

// Cada dado deve sortear valores em [1, n] — inclusive ambos os extremos.
func TestDiceWithinBounds(t *testing.T) {
	tests := []struct {
		name  string
		sides int
		roll  func() int
	}{
		{"d4", 4, RollD4},
		{"d6", 6, RollD6},
		{"d8", 8, RollD8},
		{"d10", 10, RollD10},
		{"d20", 20, RollD20},
		{"d100", 100, RollD100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for range 10_000 {
				roll := tt.roll()
				if roll < 1 || roll > tt.sides {
					t.Fatalf("roll = %d, want in [1, %d]", roll, tt.sides)
				}
			}
		})
	}
}
