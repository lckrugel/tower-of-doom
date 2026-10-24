package game

import (
	"os"
	"path/filepath"
	"slices"
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
			Spawns:         []SpawnEntry{{MonsterID: "rat", Weight: 100}, {MonsterID: "orc", Weight: 5}},
			EncounterTypes: EncounterTypeWeights{Combat: 100, Sanctuary: 10, Chest: 5},
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
			"negative monster defense",
			[]MonsterAsset{{ID: "rat", Name: "Rat", Difficulty: 1, HP: 10, Defense: -2}},
			[]SectionAsset{{ID: "s", Name: "S", MinFloor: 1, MaxFloor: -1,
				Spawns: []SpawnEntry{{MonsterID: "rat", Weight: 1}}}},
			"def",
		},
		{
			"negative monster crit",
			[]MonsterAsset{{ID: "rat", Name: "Rat", Difficulty: 1, HP: 10, Crit: -1}},
			[]SectionAsset{{ID: "s", Name: "S", MinFloor: 1, MaxFloor: -1,
				Spawns: []SpawnEntry{{MonsterID: "rat", Weight: 1}}}},
			"crit",
		},
		{
			"crit over 100 percent",
			[]MonsterAsset{{ID: "rat", Name: "Rat", Difficulty: 1, HP: 10, Crit: 101}},
			[]SectionAsset{{ID: "s", Name: "S", MinFloor: 1, MaxFloor: -1,
				Spawns: []SpawnEntry{{MonsterID: "rat", Weight: 1}}}},
			"crit",
		},
		{
			"negative encounter type weight",
			validMonsters(),
			[]SectionAsset{{ID: "s", Name: "S", MinFloor: 1, MaxFloor: -1,
				Spawns:         []SpawnEntry{{MonsterID: "rat", Weight: 1}},
				EncounterTypes: EncounterTypeWeights{Combat: 10, Chest: -1}}},
			"encounter type weight",
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

// regWithItems fills the registry's item index (makeReg leaves it empty).
func regWithItems(items []ItemAsset, sections []SectionAsset) *Registry {
	r := makeReg(validMonsters(), sections)
	for _, it := range items {
		r.Items[it.ID] = it
	}
	return r
}

// tierItems is a small item registry spanning tiers 1–3.
func tierItems() []ItemAsset {
	return []ItemAsset{
		{ID: "sword", Name: "Sword", Slot: "weapon", BonusDMG: 5, Durability: 10, Tier: 1},
		{ID: "ring", Name: "Ring", Slot: "ring", BonusDEF: 1, Durability: -1, Tier: 1},
		{ID: "crown", Name: "Crown", Slot: "helmet", BonusDEF: 9, Durability: 10, Tier: 2},
		{ID: "dragon_mail", Name: "Dragon Mail", Slot: "body_armor", BonusDEF: 20, Durability: 10, Tier: 3},
	}
}

// ItemIDsForTiers is the lookup the engine rolls rewards and merchant
// stock from: sorted ids of every item in the allowed tiers, unrestricted
// when no tier is listed, and an empty (never an error) result when
// nothing matches.
func TestItemIDsForTiers(t *testing.T) {
	r := regWithItems(tierItems(), validSections())

	tests := []struct {
		name  string
		tiers []int
		want  []string
	}{
		{"nil tiers is unrestricted", nil, []string{"crown", "dragon_mail", "ring", "sword"}},
		{"empty tiers is unrestricted", []int{}, []string{"crown", "dragon_mail", "ring", "sword"}},
		{"single tier", []int{1}, []string{"ring", "sword"}},
		{"union of tiers, sorted", []int{3, 1}, []string{"dragon_mail", "ring", "sword"}},
		{"unmatched tier is an empty pool", []int{9}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.ItemIDsForTiers(tt.tiers)
			if !slices.Equal(got, tt.want) {
				t.Errorf("ItemIDsForTiers(%v) = %v, want %v", tt.tiers, got, tt.want)
			}
		})
	}
}

// Tier-related content validation: bad item tiers, bad item_tiers values
// and sections listing tiers the registry cannot fill (typo protection).
func TestValidateItemTiers(t *testing.T) {
	spawns := []SpawnEntry{{MonsterID: "rat", Weight: 1}}
	tests := []struct {
		name     string
		items    []ItemAsset
		sections []SectionAsset
		wantErr  string
	}{
		{
			"negative item tier",
			[]ItemAsset{{ID: "broken", Name: "B", Slot: "weapon", Tier: -1, Durability: 1}},
			validSections(),
			"tier",
		},
		{
			"item_tiers value below 1",
			tierItems(),
			[]SectionAsset{{ID: "s", Name: "S", MinFloor: 1, MaxFloor: -1,
				Spawns: spawns, ItemTiers: []int{1, 0}}},
			"item_tiers",
		},
		{
			"item_tiers referencing an empty tier (typo)",
			tierItems(), // no tier-9 item exists
			[]SectionAsset{{ID: "s", Name: "S", MinFloor: 1, MaxFloor: -1,
				Spawns: spawns, ItemTiers: []int{1, 9}}},
			"no items of that tier",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := regWithItems(tt.items, tt.sections).Validate(10)

			if err == nil {
				t.Fatalf("Validate = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// item_tiers listing populated tiers — or absent entirely — validates fine.
func TestValidateAcceptsTieredContent(t *testing.T) {
	restricted := []SectionAsset{{ID: "s", Name: "S", BaseDifficulty: 5, MinFloor: 1, MaxFloor: -1,
		Spawns:    []SpawnEntry{{MonsterID: "rat", Weight: 1}},
		ItemTiers: []int{1, 2}}}
	if err := regWithItems(tierItems(), restricted).Validate(10); err != nil {
		t.Errorf("Validate(item_tiers [1 2]) = %v, want nil", err)
	}

	// No item_tiers field: unrestricted, the mods' status quo.
	if err := regWithItems(tierItems(), validSections()).Validate(10); err != nil {
		t.Errorf("Validate(without item_tiers) = %v, want nil", err)
	}
}

// An asset without a tier field defaults to tier 1 after Load — the
// tier-less embedded assets and older mods must keep working.
func TestLoadDefaultsAbsentTierToOne(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("monsters.json", `[
		{"id": "rat", "name": "Rat", "difficulty": 1, "hp": 10, "dmg": 1, "spd": 5}
	]`)
	write("items.json", `[
		{"id": "sword", "slot": "weapon", "name": "Sword", "bonus_dmg": 5, "durability": 10},
		{"id": "crown", "slot": "helmet", "name": "Crown", "bonus_def": 9, "durability": 10, "tier": 2}
	]`)
	write("sections.json", `[
		{"id": "s", "name": "S", "base_difficulty": 1, "min_floor": 1, "max_floor": -1,
		 "spawns": [{"monster_id": "rat", "weight": 1}]}
	]`)

	r, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := r.Validate(100); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got := r.Items["sword"].Tier; got != 1 {
		t.Errorf("tier-less asset loaded with tier %d, want 1 (absent = first tier)", got)
	}
	if got := r.Items["crown"].Tier; got != 2 {
		t.Errorf("explicit tier loaded as %d, want 2", got)
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
