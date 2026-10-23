package game

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Conteúdo de teste válido: cobre todos os andares de 1 até ∞,
// com spawns possíveis e orçamento suficiente.
func validMonsters() []MonsterAsset {
	return []MonsterAsset{
		{ID: "rat", Name: "Rat", Difficulty: 1, HP: 10, Damage: 1, Speed: 5},
		{ID: "orc", Name: "Orc", Difficulty: 5, HP: 50, Damage: 8, Speed: 8},
	}
}

func validSections() []SectionAsset {
	return []SectionAsset{
		{
			ID: "cellar", Name: "Cellar", BaseDifficulty: 5, DifficultyPerFloor: 2,
			MinFloor: 1, MaxFloor: 5,
			Spawns: []SpawnEntry{{MonsterID: "rat", Weight: 10}},
		},
		{
			ID: "depths", Name: "The Depths", BaseDifficulty: 50, DifficultyPerFloor: 5,
			MinFloor: 6, MaxFloor: -1,
			Spawns: []SpawnEntry{{MonsterID: "rat", Weight: 100}, {MonsterID: "orc", Weight: 5}},
		},
	}
}

func makeReg(monsters []MonsterAsset, sections []SectionAsset) *Registry {
	r := &Registry{
		Monsters: make(map[string]MonsterAsset, len(monsters)),
		Items:    make(map[string]ItemAsset),
		Sections: sections,
	}
	for _, m := range monsters {
		r.Monsters[m.ID] = m
	}
	return r
}

func TestValidateAcceptsValidContent(t *testing.T) {
	if err := makeReg(validMonsters(), validSections()).Validate(10_000); err != nil {
		t.Errorf("Validate(valid content) = %v, want nil", err)
	}
}

// Os assets pré-compilados do jogo também devem validar continuamente.
func TestValidateAcceptsEmbeddedAssets(t *testing.T) {
	r, err := Load("")
	if err != nil {
		t.Fatalf("Load embedded assets: %v", err)
	}
	if err := r.Validate(10_000); err != nil {
		t.Errorf("Validate(embedded assets) = %v, want nil", err)
	}
}

func TestValidateRejectsInvalidContent(t *testing.T) {
	tests := []struct {
		name     string
		monsters []MonsterAsset
		sections []SectionAsset
		wantErr  string
	}{
		{
			"section without spawn table",
			validMonsters(),
			[]SectionAsset{{ID: "s", Name: "S", MinFloor: 1, MaxFloor: -1}},
			"no spawn table",
		},
		{
			"unknown monster reference",
			validMonsters(),
			[]SectionAsset{{ID: "s", Name: "S", MinFloor: 1, MaxFloor: -1,
				Spawns: []SpawnEntry{{MonsterID: "dragon", Weight: 1}}}},
			"unknown monster",
		},
		{
			"zero spawn weight",
			validMonsters(),
			[]SectionAsset{{ID: "s", Name: "S", MinFloor: 1, MaxFloor: -1,
				Spawns: []SpawnEntry{{MonsterID: "rat", Weight: 0}}}},
			"weight",
		},
		{
			"zero monster difficulty",
			[]MonsterAsset{{ID: "rat", Name: "Rat", Difficulty: 0, HP: 10}},
			[]SectionAsset{{ID: "s", Name: "S", MinFloor: 1, MaxFloor: -1,
				Spawns: []SpawnEntry{{MonsterID: "rat", Weight: 1}}}},
			"difficulty",
		},
		{
			"floor budget below cheapest monster",
			[]MonsterAsset{{ID: "rat", Name: "Rat", Difficulty: 5, HP: 10}},
			[]SectionAsset{{ID: "s", Name: "S", BaseDifficulty: 1, MinFloor: 1, MaxFloor: -1,
				Spawns: []SpawnEntry{{MonsterID: "rat", Weight: 1}}}},
			"would always be empty",
		},
		{
			"uncovered floor",
			validMonsters(),
			[]SectionAsset{{ID: "s", Name: "S", MinFloor: 2, MaxFloor: -1,
				Spawns: []SpawnEntry{{MonsterID: "rat", Weight: 1}}}},
			"no section found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := makeReg(tt.monsters, tt.sections).Validate(10)

			if err == nil {
				t.Fatalf("Validate = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestSectionForFloor(t *testing.T) {
	r := makeReg(validMonsters(), validSections())

	tests := []struct {
		name    string
		floor   int
		wantID  string
		wantErr bool
	}{
		{"first floor", 1, "cellar", false},
		{"max floor of finite section", 5, "cellar", false},
		{"first floor of tail section", 6, "depths", false},
		{"deep into infinite tail", 100_000, "depths", false},
		{"floors start at 1", 0, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := r.SectionForFloor(tt.floor)

			if tt.wantErr {
				if err == nil {
					t.Errorf("SectionForFloor(%d) = %q, want error", tt.floor, s.ID)
				}
				return
			}
			if err != nil {
				t.Fatalf("SectionForFloor(%d): %v", tt.floor, err)
			}
			if s.ID != tt.wantID {
				t.Errorf("SectionForFloor(%d) = %q, want %q", tt.floor, s.ID, tt.wantID)
			}
		})
	}
}

func TestFloorDifficulty(t *testing.T) {
	s := SectionAsset{ID: "camp", BaseDifficulty: 35, DifficultyPerFloor: 7, MinFloor: 6, MaxFloor: -1}

	tests := []struct {
		name  string
		floor int
		want  int
	}{
		{"min floor of section", 6, 35},
		{"two floors in", 8, 49},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.FloorDifficulty(tt.floor); got != tt.want {
				t.Errorf("FloorDifficulty(%d) = %d, want %d", tt.floor, got, tt.want)
			}
		})
	}
}

func TestIndexByRejectsDuplicateIDs(t *testing.T) {
	dupes := []MonsterAsset{
		{ID: "goblin", Name: "Goblin One"},
		{ID: "goblin", Name: "Goblin Two"},
	}

	_, err := indexBy(dupes, func(m MonsterAsset) string { return m.ID })

	if err == nil {
		t.Fatal("indexBy(duplicate ids) = nil error, want error")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %v, want it to mention duplicate ids", err)
	}
}

// loadJSON: arquivo externo ausente cai para os assets embutidos;
// arquivo externo quebrado deve ser erro real.
func TestLoadJSONFallsBackToEmbedded(t *testing.T) {
	items, err := loadJSON[MonsterAsset](t.TempDir(), "monsters.json")
	if err != nil {
		t.Fatalf("loadJSON: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("loadJSON returned empty list, want embedded monsters")
	}
}

func TestLoadJSONBrokenExternalFileIsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "monsters.json")
	if err := os.WriteFile(path, []byte("this is not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := loadJSON[MonsterAsset](dir, "monsters.json")

	if err == nil {
		t.Fatal("broken external file silently fell back to embedded assets")
	}
	if !strings.Contains(err.Error(), "parsing") {
		t.Errorf("error = %v, want a parse error mentioning the file", err)
	}
}
