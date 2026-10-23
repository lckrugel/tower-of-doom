package engine

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// stubRng devolve os valores pré-programados em sequência, voltando ao
// início quando acaba. Os valores devem ser válidos para cada chamada
// (os testes abaixo usam valores menores que o total de cada sorteio).
type stubRng struct {
	values []int
	calls  int
}

func (s *stubRng) Intn(n int) int {
	v := s.values[s.calls%len(s.values)]
	s.calls++
	return v
}

func testMonsters() map[string]game.MonsterAsset {
	return map[string]game.MonsterAsset{
		"goblin": {ID: "goblin", Name: "Goblin", Difficulty: 3, HP: 30, Damage: 2, Speed: 10},
		"orc":    {ID: "orc", Name: "Orc", Difficulty: 5, HP: 50, Damage: 8, Speed: 8},
	}
}

func testSections() []game.SectionAsset {
	return []game.SectionAsset{
		{
			ID: "pits", Name: "Goblin Pits", BaseDifficulty: 9, DifficultyPerFloor: 3,
			MinFloor: 1, MaxFloor: 5,
			Spawns: []game.SpawnEntry{{MonsterID: "goblin", Weight: 100}, {MonsterID: "orc", Weight: 5}},
		},
	}
}

func newTestEngine(rngValues ...int) *Engine {
	e := NewEngine(&game.Registry{
		Monsters: testMonsters(),
		Items:    map[string]game.ItemAsset{},
		Sections: testSections(),
	})
	if len(rngValues) > 0 {
		e.rng = &stubRng{values: rngValues}
	}
	return e
}

func monsterIDs(monsters []entities.Monster) []string {
	ids := make([]string, len(monsters))
	for i, m := range monsters {
		ids[i] = m.ID
	}
	return ids
}

func idsEqual(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestWeightedPickHonorsRngSequence(t *testing.T) {
	e := newTestEngine()
	e.rng = &stubRng{values: []int{0}} // sempre o primeiro candidato

	items := []game.MonsterAsset{testMonsters()["goblin"], testMonsters()["orc"]}
	if got := e.weightedPick(items, []int{100, 5}); got.ID != "goblin" {
		t.Errorf("weightedPick(rng=0) = %q, want goblin", got.ID)
	}
}

func TestWeightedPickHonorsCumulativeWeights(t *testing.T) {
	e := newTestEngine()
	// total = 105; r=104 só cumpre r < 105 (a cumulativa do orc não é alcançada antes).
	e.rng = &stubRng{values: []int{104}}

	items := []game.MonsterAsset{testMonsters()["goblin"], testMonsters()["orc"]}
	if got := e.weightedPick(items, []int{100, 5}); got.ID != "orc" {
		t.Errorf("weightedPick(rng=104, weights=[100,5]) = %q, want orc", got.ID)
	}
}

func TestWeightedPickPanicsWithoutPositiveWeights(t *testing.T) {
	e := newTestEngine()
	e.rng = &stubRng{values: []int{0}}

	defer func() {
		if recover() == nil {
			t.Fatal("weightedPick(weights all zero) did not panic")
		}
	}()
	e.weightedPick([]game.MonsterAsset{testMonsters()["goblin"]}, []int{0})
}

func TestGenerateEncounter(t *testing.T) {
	t.Run("budget is respected", func(t *testing.T) {
		e := newTestEngine()
		encounter, err := e.generateEncounter(1) // budget 9
		if err != nil {
			t.Fatalf("generateEncounter: %v", err)
		}

		total := 0
		for _, m := range encounter {
			total += m.Difficulty
		}
		if total > 9 {
			t.Errorf("encounter with total difficulty %d exceeded budget 9", total)
		}
		if len(encounter) == 0 {
			t.Fatal("encounter was empty for a floor with valid content")
		}
	})

	t.Run("rng sequence drives the picks", func(t *testing.T) {
		// Orçamento 9: rng=104 → orc (5), resta 4 → rng=0 → goblin (3),
		// resta 1 → só o orc não cabe mais.
		e := newTestEngine(104, 0, 0)
		encounter, err := e.generateEncounter(1)
		if err != nil {
			t.Fatalf("generateEncounter: %v", err)
		}

		got := monsterIDs(encounter)
		if !idsEqual(got, "orc", "goblin") {
			t.Errorf("encounter = %v, want [orc goblin]", got)
		}
	})

	t.Run("size cap with a huge budget", func(t *testing.T) {
		e := newTestEngine(0) // pega sempre o goblin barato
		e.reg.Sections[0].BaseDifficulty = 999_999

		encounter, err := e.generateEncounter(5)
		if err != nil {
			t.Fatalf("generateEncounter: %v", err)
		}

		if len(encounter) != MAX_ENCOUNTER_SIZE {
			t.Errorf("encounter size = %d, want capped at %d", len(encounter), MAX_ENCOUNTER_SIZE)
		}
	})

	t.Run("only spawns from the section spawn table", func(t *testing.T) {
		e := newTestEngine(0, 0, 0)
		encounter, err := e.generateEncounter(1)
		if err != nil {
			t.Fatalf("generateEncounter: %v", err)
		}

		for _, m := range encounter {
			if m.ID != "goblin" && m.ID != "orc" {
				t.Errorf("encounter contains monster %q, which is not in the section's spawn table", m.ID)
			}
		}
	})

	t.Run("stops when nothing is affordable", func(t *testing.T) {
		// Orçamento 1: nem o goblin (3) cabe.
		e := newTestEngine(0)
		e.reg.Sections[0].BaseDifficulty = 1

		encounter, err := e.generateEncounter(1)
		if err != nil {
			t.Fatalf("generateEncounter: %v", err)
		}

		if len(encounter) != 0 {
			t.Errorf("encounter = %v, want empty (nothing affordable)", monsterIDs(encounter))
		}
	})
}

func TestExploringFlow(t *testing.T) {
	e := newTestEngine(0)

	events := e.Process(Command{Type: CmdExplore})
	if len(events) == 0 || events[0].Type != EventEncounterFound {
		t.Fatalf("exploring produced %v, want an encounter event", events)
	}
	if len(e.CurrentEncounter) == 0 {
		t.Error("engine state holds no encounter after explore")
	}
}
