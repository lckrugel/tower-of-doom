package entities

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
)

// learnBoth fills a hero's learned set to the cap (MaxLearnedMoves).
func learnBoth(h *Hero) {
	if _, ok := h.LearnMove(Move{ID: "a", Name: "A"}); !ok {
		panic("test setup: learning 'a' failed")
	}
	if _, ok := h.LearnMove(Move{ID: "b", Name: "B"}); !ok {
		panic("test setup: learning 'b' failed")
	}
}

// LearnMove appends under the cap, in learning order.
func TestHeroLearnMove(t *testing.T) {
	t.Run("learns under the cap, in learning order", func(t *testing.T) {
		h := NewPlayer()

		forgotten, learned := h.LearnMove(Move{ID: "a", Name: "A"})
		if !learned || forgotten != nil {
			t.Errorf("LearnMove(a) = (%v, %v), want (nil, true)", forgotten, learned)
		}
		if _, learned := h.LearnMove(Move{ID: "b", Name: "B"}); !learned {
			t.Error("LearnMove(b) refused under the cap")
		}

		learnedMoves := h.LearnedMoves()
		if len(learnedMoves) != 2 || learnedMoves[0].ID != "a" || learnedMoves[1].ID != "b" {
			t.Errorf("learned = %+v, want [a b] in learning order", learnedMoves)
		}
	})

	t.Run("refuses at the cap — forgetting is the caller's decision", func(t *testing.T) {
		h := NewPlayer()
		learnBoth(h)

		forgotten, learned := h.LearnMove(Move{ID: "c"})
		if learned || forgotten != nil {
			t.Errorf("LearnMove at cap = (%v, %v), want (nil, false)", forgotten, learned)
		}
		got := h.LearnedMoves()
		if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
			t.Errorf("learned set changed at cap: %+v", got)
		}
	})

	t.Run("refuses an already-known move id", func(t *testing.T) {
		h := NewPlayer()
		h.LearnMove(Move{ID: "a", Name: "A"})

		if _, learned := h.LearnMove(Move{ID: "a", Name: "A Again"}); learned {
			t.Error("LearnMove re-learned a known id")
		}
		got := h.LearnedMoves()
		if len(got) != 1 || got[0].Name != "A" {
			t.Errorf("duplicate learn mutated the set: %+v", got)
		}
	})

	t.Run("learning again after forgetting works", func(t *testing.T) {
		h := NewPlayer()
		learnBoth(h)

		if !h.ForgetMove("a") {
			t.Fatal("ForgetMove(a) = false, want true")
		}
		if _, learned := h.LearnMove(Move{ID: "c"}); !learned {
			t.Error("LearnMove(c) refused with room freed by ForgetMove")
		}
		got := h.LearnedMoves()
		if len(got) != 2 || got[0].ID != "b" || got[1].ID != "c" {
			t.Errorf("learned = %+v, want [b c]", got)
		}
	})
}

// ForgetMove removes the learned move with the given id; unknown ids and
// empty sets are no-ops (player-reachable — never a panic).
func TestHeroForgetMove(t *testing.T) {
	t.Run("removes the named move, keeping order", func(t *testing.T) {
		h := NewPlayer()
		h.LearnMove(Move{ID: "a"})
		h.LearnMove(Move{ID: "b"})

		if !h.ForgetMove("a") {
			t.Error("ForgetMove(a) = false, want true")
		}
		got := h.LearnedMoves()
		if len(got) != 1 || got[0].ID != "b" {
			t.Errorf("learned = %+v, want [b]", got)
		}
	})

	t.Run("unknown id is a no-op", func(t *testing.T) {
		h := NewPlayer()
		h.LearnMove(Move{ID: "a"})

		if h.ForgetMove("nope") {
			t.Error("ForgetMove(unknown) = true, want false")
		}
		if got := h.LearnedMoves(); len(got) != 1 {
			t.Errorf("forgetting an unknown id changed the set: %+v", got)
		}
	})
}

// EffectiveMoves composes the combat move set: learned first (learning
// order), then weapon-granted; deduplicated by id (the learned copy
// wins) and capped at MaxMoves, dropping weapon moves beyond the cap.
func TestHeroEffectiveMoves(t *testing.T) {
	a := Move{ID: "a", Name: "A"}
	b := Move{ID: "b", Name: "B"}
	w1 := Move{ID: "w1", Name: "W1"}
	w2 := Move{ID: "w2", Name: "W2"}

	t.Run("learned first, then weapon moves", func(t *testing.T) {
		h := NewPlayer()
		h.LearnMove(a)
		h.LearnMove(b)

		got := h.EffectiveMoves(newTestWeapon(w1, w2))

		if len(got) != 4 || got[0].ID != "a" || got[1].ID != "b" || got[2].ID != "w1" || got[3].ID != "w2" {
			t.Errorf("EffectiveMoves = %+v, want [a b w1 w2]", got)
		}
	})

	t.Run("nil weapon leaves just the learned moves", func(t *testing.T) {
		h := NewPlayer()
		h.LearnMove(a)

		got := h.EffectiveMoves(nil)

		if len(got) != 1 || got[0].ID != "a" {
			t.Errorf("EffectiveMoves(nil) = %+v, want [a]", got)
		}
	})

	t.Run("an empty set stays empty — basic attack is not implicit", func(t *testing.T) {
		h := NewPlayer()

		got := h.EffectiveMoves(newTestWeapon())

		if len(got) != 0 {
			t.Errorf("EffectiveMoves = %+v, want empty (the combat integration owns the ATTACK fallback)", got)
		}
	})

	t.Run("deduplicates by id, learned copy wins", func(t *testing.T) {
		h := NewPlayer()
		h.LearnMove(a)

		got := h.EffectiveMoves(newTestWeapon(Move{ID: "a", Name: "Weapon A"}, w1))

		if len(got) != 2 || got[0].ID != "a" || got[0].Name != "A" || got[1].ID != "w1" {
			t.Errorf("EffectiveMoves = %+v, want [a(A) w1] — learned copy wins the dedup", got)
		}
	})

	t.Run("never exceeds MaxMoves — weapon moves drop first", func(t *testing.T) {
		h := NewPlayer()
		learnBoth(h) // 2 learned

		got := h.EffectiveMoves(newTestWeapon(w1, w2, Move{ID: "w3"})) // 3 granted

		if len(got) != MaxMoves {
			t.Fatalf("EffectiveMoves length = %d, want %d", len(got), MaxMoves)
		}
		for i, want := range []string{"a", "b", "w1", "w2"} {
			if got[i].ID != want {
				t.Errorf("EffectiveMoves[%d] = %q, want %q (w3 dropped beyond the cap)", i, got[i].ID, want)
			}
		}
	})

	t.Run("learned moves also respect the cap", func(t *testing.T) {
		// learnBoth + ForgetMove-shaped flows can never exceed the cap,
		// but the effective set guards it anyway (defense in depth).
		h := NewPlayer()
		learnBoth(h)

		got := h.EffectiveMoves(nil)

		if len(got) != MaxLearnedMoves {
			t.Errorf("EffectiveMoves length = %d, want %d", len(got), MaxLearnedMoves)
		}
	})

	t.Run("returned slice is a copy", func(t *testing.T) {
		h := NewPlayer()
		h.LearnMove(a)

		got := h.EffectiveMoves(nil)
		got[0] = Move{ID: "tampered"}

		if again := h.EffectiveMoves(nil); again[0].ID != "a" {
			t.Errorf("mutating the returned slice changed the set: %q", again[0].ID)
		}
	})
}

// The runtime Item carries its granted moves (resolved by the caller).
func TestItemGrantedMoves(t *testing.T) {
	heavy := Move{ID: "heavy_slash", Name: "Heavy Slash"}
	stab := Move{ID: "quick_stab", Name: "Quick Stab"}

	t.Run("move-less items grant nothing", func(t *testing.T) {
		item := NewItemFromAsset(game.ItemAsset{ID: "s", Name: "S", Slot: "weapon"})

		if got := item.GrantedMoves(); len(got) != 0 {
			t.Errorf("GrantedMoves = %+v, want empty", got)
		}
	})

	t.Run("granted moves ride the item in order", func(t *testing.T) {
		item := NewItemFromAsset(game.ItemAsset{ID: "s", Name: "S", Slot: "weapon"}, heavy, stab)

		got := item.GrantedMoves()
		if len(got) != 2 || got[0].ID != "heavy_slash" || got[1].ID != "quick_stab" {
			t.Errorf("GrantedMoves = %+v, want [heavy_slash quick_stab]", got)
		}
	})

	t.Run("returned slice is a copy", func(t *testing.T) {
		item := NewItemFromAsset(game.ItemAsset{ID: "s", Name: "S", Slot: "weapon"}, heavy)

		got := item.GrantedMoves()
		got[0] = Move{ID: "tampered"}

		if again := item.GrantedMoves(); again[0].ID != "heavy_slash" {
			t.Errorf("mutating the returned slice changed the item's moves: %q", again[0].ID)
		}
	})
}
