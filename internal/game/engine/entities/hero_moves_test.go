package entities

import (
	"reflect"
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
)

// newBareHero returns a fresh hero with an empty learned set.
func newBareHero() *Hero {
	return NewPlayer()
}

func learnableMove(id string) Move {
	return Move{ID: id, Learnable: true, Type: MoveTypeCreature, Cooldown: 1}
}

// learnBoth fills a hero's learned set to the cap (MaxLearnedMoves).
func learnBoth(h *Hero) {
	if _, ok := h.LearnMove(Move{ID: "a", Name: "A", Learnable: true, Type: MoveTypeCreature, Cooldown: 1}); !ok {
		panic("test setup: learning 'a' failed")
	}
	if _, ok := h.LearnMove(Move{ID: "b", Name: "B", Learnable: true, Type: MoveTypeCreature, Cooldown: 1}); !ok {
		panic("test setup: learning 'b' failed")
	}
}

// NewPlayer starts with no learned moves; basic actions are composed by the engine.
func TestNewPlayerStartsWithNoLearnedMoves(t *testing.T) {
	if got := NewPlayer().LearnedMoves(); len(got) != 0 {
		t.Fatalf("fresh learned moves = %+v, want empty", got)
	}
}

// LearnMove appends under the cap, in learning order.
func TestHeroLearnMove(t *testing.T) {
	t.Run("learns under the cap, in learning order", func(t *testing.T) {
		h := newBareHero()

		forgotten, learned := h.LearnMove(Move{ID: "a", Name: "A", Learnable: true, Type: MoveTypeCreature, Cooldown: 1})
		if !learned || forgotten != nil {
			t.Errorf("LearnMove(a) = (%v, %v), want (nil, true)", forgotten, learned)
		}
		if _, learned := h.LearnMove(Move{ID: "b", Name: "B", Learnable: true, Type: MoveTypeCreature, Cooldown: 1}); !learned {
			t.Error("LearnMove(b) refused under the cap")
		}

		learnedMoves := h.LearnedMoves()
		if len(learnedMoves) != 2 || learnedMoves[0].ID != "a" || learnedMoves[1].ID != "b" {
			t.Errorf("learned = %+v, want [a b] in learning order", learnedMoves)
		}
	})

	t.Run("refuses at the cap — forgetting is the caller's decision", func(t *testing.T) {
		h := newBareHero()
		learnBoth(h)
		h.LearnMove(Move{ID: "c", Learnable: true, Type: MoveTypeCreature, Cooldown: 1})

		forgotten, learned := h.LearnMove(Move{ID: "d", Learnable: true, Type: MoveTypeCreature, Cooldown: 1})
		if learned || forgotten != nil {
			t.Errorf("LearnMove at cap = (%v, %v), want (nil, false)", forgotten, learned)
		}
		got := h.LearnedMoves()
		if len(got) != 3 || got[0].ID != "a" || got[1].ID != "b" || got[2].ID != "c" {
			t.Errorf("learned set changed at cap: %+v", got)
		}
	})

	t.Run("rejects ineligible moves without changing learned state", func(t *testing.T) {
		h := newBareHero()
		h.LearnMove(learnableMove("a"))
		before := h.LearnedMoves()
		if _, learned := h.LearnMove(Move{ID: "forbidden"}); learned {
			t.Fatal("learned a non-learnable move")
		}
		if !reflect.DeepEqual(h.LearnedMoves(), before) {
			t.Fatalf("ineligible move changed set: %+v", h.LearnedMoves())
		}
	})

	t.Run("refuses an already-known move id", func(t *testing.T) {
		h := newBareHero()
		h.LearnMove(Move{ID: "a", Name: "A", Learnable: true, Type: MoveTypeCreature, Cooldown: 1})

		if _, learned := h.LearnMove(Move{ID: "a", Name: "A Again", Learnable: true, Type: MoveTypeCreature, Cooldown: 1}); learned {
			t.Error("LearnMove re-learned a known id")
		}
		got := h.LearnedMoves()
		if len(got) != 1 || got[0].Name != "A" {
			t.Errorf("duplicate learn mutated the set: %+v", got)
		}
	})

	t.Run("learning again after forgetting works", func(t *testing.T) {
		h := newBareHero()
		learnBoth(h)

		if !h.ForgetMove("a") {
			t.Fatal("ForgetMove(a) = false, want true")
		}
		if _, learned := h.LearnMove(Move{ID: "c", Learnable: true, Type: MoveTypeCreature, Cooldown: 1}); !learned {
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
		h := newBareHero()
		h.LearnMove(Move{ID: "a", Learnable: true, Type: MoveTypeCreature, Cooldown: 1})
		h.LearnMove(Move{ID: "b", Learnable: true, Type: MoveTypeCreature, Cooldown: 1})

		if !h.ForgetMove("a") {
			t.Error("ForgetMove(a) = false, want true")
		}
		got := h.LearnedMoves()
		if len(got) != 1 || got[0].ID != "b" {
			t.Errorf("learned = %+v, want [b]", got)
		}
	})

	t.Run("unknown id is a no-op", func(t *testing.T) {
		h := newBareHero()
		h.LearnMove(Move{ID: "a", Learnable: true, Type: MoveTypeCreature, Cooldown: 1})

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
	a := Move{ID: "a", Name: "A", Learnable: true, Type: MoveTypeCreature, Cooldown: 1}
	b := Move{ID: "b", Name: "B", Learnable: true, Type: MoveTypeCreature, Cooldown: 1}
	w1 := Move{ID: "w1", Name: "W1"}
	w2 := Move{ID: "w2", Name: "W2"}

	t.Run("weapon moves first, then learned", func(t *testing.T) {
		h := newBareHero()
		h.LearnMove(a)
		h.LearnMove(b)

		got := h.EffectiveMoves(newTestWeapon(w1, w2))

		if len(got) != 4 || got[0].ID != "w1" || got[1].ID != "w2" || got[2].ID != "a" || got[3].ID != "b" {
			t.Errorf("EffectiveMoves = %+v, want [w1 w2 a b]", got)
		}
	})

	t.Run("nil weapon leaves just the learned moves", func(t *testing.T) {
		h := newBareHero()
		h.LearnMove(a)

		got := h.EffectiveMoves(nil)

		if len(got) != 1 || got[0].ID != "a" {
			t.Errorf("EffectiveMoves(nil) = %+v, want [a]", got)
		}
	})

	t.Run("an empty set stays empty — basic attack is not implicit", func(t *testing.T) {
		h := newBareHero()

		got := h.EffectiveMoves(newTestWeapon())

		if len(got) != 0 {
			t.Errorf("EffectiveMoves = %+v, want empty (the combat integration owns the ATTACK fallback)", got)
		}
	})

	t.Run("deduplicates by id, weapon copy wins", func(t *testing.T) {
		h := newBareHero()
		h.LearnMove(a)

		got := h.EffectiveMoves(newTestWeapon(Move{ID: "a", Name: "Weapon A"}, w1))

		if len(got) != 2 || got[0].ID != "a" || got[0].Name != "Weapon A" || got[1].ID != "w1" {
			t.Errorf("EffectiveMoves = %+v, want [a(Weapon A) w1] — weapon copy wins the dedup", got)
		}
	})

	t.Run("never exceeds MaxMoves — weapon moves drop first", func(t *testing.T) {
		h := newBareHero()
		learnBoth(h) // 2 learned

		got := h.EffectiveMoves(newTestWeapon(w1, w2, Move{ID: "w3"})) // 3 granted

		if len(got) != MaxMoves {
			t.Fatalf("EffectiveMoves length = %d, want %d", len(got), MaxMoves)
		}
		for i, want := range []string{"w1", "w2", "w3", "a"} {
			if got[i].ID != want {
				t.Errorf("EffectiveMoves[%d] = %q, want %q (w3 dropped beyond the cap)", i, got[i].ID, want)
			}
		}
	})

	t.Run("learned moves also respect the cap", func(t *testing.T) {
		// Normal learning enforces its own smaller cap. Corrupt the internal
		// slice deliberately to exercise EffectiveMoves' independent total cap.
		h := newBareHero()
		learnBoth(h)
		h.learnedMoves = append(h.learnedMoves, Move{ID: "c", Learnable: true, Type: MoveTypeCreature, Cooldown: 1}, Move{ID: "d"})

		got := h.EffectiveMoves(nil)

		if len(got) != MaxMoves {
			t.Errorf("EffectiveMoves length = %d, want total cap %d", len(got), MaxMoves)
		}
		for i, want := range []string{"a", "b", "c", "d"} {
			if i < len(got) && got[i].ID != want {
				t.Errorf("EffectiveMoves[%d] = %q, want %q", i, got[i].ID, want)
			}
		}
	})

	t.Run("returned slice is a copy", func(t *testing.T) {
		h := newBareHero()
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

// EffectiveMovesWithSource tags every effective move with where it came
// from — the learned set first, then the weapon's grants, the SAME order
// and dedup as EffectiveMoves.
func TestHeroEffectiveMovesWithSource(t *testing.T) {
	a := Move{ID: "a", Name: "A", Learnable: true, Type: MoveTypeCreature, Cooldown: 1}
	b := Move{ID: "b", Name: "B", Learnable: true, Type: MoveTypeCreature, Cooldown: 1}
	w1 := Move{ID: "w1", Name: "W1"}
	w2 := Move{ID: "w2", Name: "W2"}

	t.Run("weapon moves first, learned moves after", func(t *testing.T) {
		h := newBareHero()
		h.LearnMove(a)
		h.LearnMove(b)

		got := h.EffectiveMovesWithSource(newTestWeapon(w1, w2))

		if len(got) != 4 {
			t.Fatalf("EffectiveMovesWithSource = %+v, want 4 entries", got)
		}
		wantSources := []MoveSource{SourceWeapon, SourceWeapon, SourceLearned, SourceLearned}
		for i, want := range []string{"w1", "w2", "a", "b"} {
			if got[i].Move.ID != want || got[i].Source != wantSources[i] {
				t.Errorf("entry %d = {%q %v}, want {%q %v}", i,
					got[i].Move.ID, got[i].Source, want, wantSources[i])
			}
		}
	})

	t.Run("a move known via both reports as LEARNED", func(t *testing.T) {
		h := newBareHero()
		h.LearnMove(a)

		got := h.EffectiveMovesWithSource(newTestWeapon(Move{ID: "a", Name: "Weapon A"}, w1))

		if len(got) != 2 {
			t.Fatalf("EffectiveMovesWithSource = %+v, want 2 entries (dedup by id)", got)
		}
		if got[0].Move.ID != "a" || got[0].Source != SourceWeapon {
			t.Errorf("entry 0 = {%q %v}, want {a WEAPON} — weapon copy wins the dedup",
				got[0].Move.ID, got[0].Source)
		}
		if got[0].Move.Name != "Weapon A" {
			t.Errorf("dedup kept the weapon copy's name %q, want the learned %q", got[0].Move.Name, "A")
		}
	})

	t.Run("nil weapon leaves only learned sources", func(t *testing.T) {
		h := newBareHero()
		h.LearnMove(a)

		got := h.EffectiveMovesWithSource(nil)

		if len(got) != 1 || got[0].Move.ID != "a" || got[0].Source != SourceLearned {
			t.Errorf("EffectiveMovesWithSource(nil) = %+v, want [{a LEARNED}]", got)
		}
	})

	t.Run("the cap drops weapon moves, sources stay consistent", func(t *testing.T) {
		h := newBareHero()
		learnBoth(h) // 2 learned

		got := h.EffectiveMovesWithSource(newTestWeapon(w1, w2, Move{ID: "w3"}))

		if len(got) != MaxMoves {
			t.Fatalf("EffectiveMovesWithSource length = %d, want %d", len(got), MaxMoves)
		}
		for i, want := range []struct {
			id  string
			src MoveSource
		}{{"w1", SourceWeapon}, {"w2", SourceWeapon}, {"w3", SourceWeapon}, {"a", SourceLearned}} {
			if got[i].Move.ID != want.id || got[i].Source != want.src {
				t.Errorf("entry %d = {%q %v}, want {%q %v} (w3 dropped beyond the cap)",
					i, got[i].Move.ID, got[i].Source, want.id, want.src)
			}
		}
	})

	t.Run("EffectiveMoves mirrors the sourced set", func(t *testing.T) {
		h := newBareHero()
		h.LearnMove(a)
		weapon := newTestWeapon(Move{ID: "a", Name: "Weapon A"}, w1, w2)

		sourced := h.EffectiveMovesWithSource(weapon)
		plain := h.EffectiveMoves(weapon)

		if len(plain) != len(sourced) {
			t.Fatalf("EffectiveMoves length = %d, sourced = %d", len(plain), len(sourced))
		}
		for i := range sourced {
			if !reflect.DeepEqual(plain[i], sourced[i].Move) {
				t.Errorf("EffectiveMoves[%d] = %+v, want the sourced %+v", i, plain[i], sourced[i].Move)
			}
		}
	})

	t.Run("returned slice is a copy", func(t *testing.T) {
		h := newBareHero()
		h.LearnMove(a)

		got := h.EffectiveMovesWithSource(nil)
		got[0] = SourcedMove{Move: Move{ID: "tampered"}}

		if again := h.EffectiveMovesWithSource(nil); again[0].Move.ID != "a" {
			t.Errorf("mutating the returned slice changed the set: %q", again[0].Move.ID)
		}
	})
}
