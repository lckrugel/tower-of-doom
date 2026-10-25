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
		Moves:    make(map[string]MoveAsset),
		Statuses: make(map[string]StatusAsset),
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
			"negative shrine weight",
			validMonsters(),
			[]SectionAsset{{ID: "s", Name: "S", MinFloor: 1, MaxFloor: -1,
				Spawns:         []SpawnEntry{{MonsterID: "rat", Weight: 1}},
				EncounterTypes: EncounterTypeWeights{Combat: 10, Shrine: -1}}},
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

// A merchant section that could never stock its shop is a content bug:
// offers roll from the allowed tiers only (tier+1 upgrades can only
// REPLACE offers, so they cannot stock an empty shop).
func TestValidateMerchantStockability(t *testing.T) {
	spawns := []SpawnEntry{{MonsterID: "rat", Weight: 1}}
	merchant := func(itemTiers []int) []SectionAsset {
		return []SectionAsset{{ID: "s", Name: "S", BaseDifficulty: 5, MinFloor: 1, MaxFloor: -1,
			Spawns: spawns, ItemTiers: itemTiers,
			EncounterTypes: EncounterTypeWeights{Combat: 10, Merchant: 5}}}
	}

	t.Run("no items at all in an unrestricted section", func(t *testing.T) {
		err := makeReg(validMonsters(), merchant(nil)).Validate(10)
		if err == nil || !strings.Contains(err.Error(), "stock it") {
			t.Errorf("Validate = %v, want a stockability error", err)
		}
	})

	// Note: a RESTRICTED section that lists an unpopulated tier already
	// fails the item_tiers typo check above, so the merchant rule only
	// ever fires when the effective allowed pool is empty — an item-less
	// registry (e.g. a mod shipping merchant sections without items).

	t.Run("stockable merchant section validates", func(t *testing.T) {
		if err := regWithItems(tierItems(), merchant([]int{1})).Validate(10); err != nil {
			t.Errorf("Validate = %v, want nil", err)
		}
	})

	t.Run("negative merchant weight", func(t *testing.T) {
		sections := []SectionAsset{{ID: "s", Name: "S", BaseDifficulty: 5, MinFloor: 1, MaxFloor: -1,
			Spawns:         spawns,
			EncounterTypes: EncounterTypeWeights{Combat: 10, Merchant: -1}}}
		err := makeReg(validMonsters(), sections).Validate(10)
		if err == nil || !strings.Contains(err.Error(), "encounter type weight") {
			t.Errorf("Validate = %v, want a negative-weight error", err)
		}
	})
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

// ---- Section bosses: asset schema, validation and the section-end query ----

// bossSections is a section ladder with boss blocks: cellar 1-5, mid 5-10
// (overlapping on purpose — SectionForFloor resolves first-match, so
// floor 5 belongs to the cellar), tail 10-∞ (no boss — infinite tails
// have no final floor).
func bossSections() []SectionAsset {
	return []SectionAsset{
		{
			ID: "cellar", Name: "Cellar", BaseDifficulty: 5, DifficultyPerFloor: 2,
			MinFloor: 1, MaxFloor: 5,
			Spawns: []SpawnEntry{{MonsterID: "rat", Weight: 10}},
			Boss:   &BossAsset{Monsters: []string{"orc"}, Gold: 50},
		},
		{
			ID: "mid", Name: "The Mid", BaseDifficulty: 20, DifficultyPerFloor: 3,
			MinFloor: 5, MaxFloor: 10,
			Spawns: []SpawnEntry{{MonsterID: "orc", Weight: 10}},
			Boss:   &BossAsset{Monsters: []string{"orc", "rat"}},
		},
		{
			ID: "tail", Name: "The Tail", BaseDifficulty: 60, DifficultyPerFloor: 5,
			MinFloor: 10, MaxFloor: -1,
			Spawns: []SpawnEntry{{MonsterID: "orc", Weight: 10}},
		},
	}
}

// Boss-block content validation: a listed monster must exist (typo
// protection), the group cannot be empty, and the gold bonus cannot be
// negative. A nil boss block (absent field) is the mods' status quo.
func TestValidateSectionBoss(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(r *Registry)
		wantErr string // "" means the content is legal
	}{
		{"valid boss blocks", func(r *Registry) {}, ""},
		{"boss referencing unknown monster", func(r *Registry) {
			r.Sections[0].Boss.Monsters = []string{"dragon"}
		}, "unknown monster"},
		{"boss block with an empty monster list", func(r *Registry) {
			r.Sections[0].Boss.Monsters = nil
		}, "empty monster list"},
		{"negative boss gold", func(r *Registry) {
			r.Sections[0].Boss.Gold = -1
		}, "gold"},
		{"zero boss gold is legal (no bonus)", func(r *Registry) {
			r.Sections[0].Boss.Gold = 0
		}, ""},
		{"nil boss block is legal (no gate)", func(r *Registry) {
			r.Sections[0].Boss = nil
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := makeReg(validMonsters(), bossSections())
			tt.mutate(r)
			err := r.Validate(10)

			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate = %v, want nil (legal content)", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// IsSectionEnd marks the floor where a section's boss gates the
// ascension: the section's max_floor, when it has one.
func TestIsSectionEnd(t *testing.T) {
	r := makeReg(validMonsters(), bossSections())

	tests := []struct {
		name  string
		floor int
		want  bool
	}{
		{"early floor", 1, false},
		{"floor 5 resolves first-match: the cellar's end, not the mid's start", 5, true},
		{"floor 10 is the mid section's end", 10, true},
		{"an infinite tail has no final floor", 11, false},
		{"deep into the infinite tail", 100_000, false},
		{"no section covers floor 0", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.IsSectionEnd(tt.floor); got != tt.want {
				t.Errorf("IsSectionEnd(%d) = %v, want %v", tt.floor, got, tt.want)
			}
		})
	}
}

// ---- Combat 2.0 data layer: moves & statuses ----

// validMoves is a small move registry spanning tiers 1–3, the canonical
// basic_attack included. Status wiring mirrors the embedded content.
func validMoves() []MoveAsset {
	return []MoveAsset{
		{ID: "basic_attack", Name: "Basic Attack", Tier: 1, DmgModifier: 100, Weight: 100},
		{ID: "quick_stab", Name: "Quick Stab", Tier: 1, DmgModifier: 60, CritBonus: 20, Weight: 100},
		{ID: "guard_strike", Name: "Guard Strike", Tier: 1, DmgModifier: 75, Cooldown: 2, Weight: 50,
			Status: &MoveStatusAsset{ID: "guard", Chance: 100, Target: "self"}},
		{ID: "venom_bite", Name: "Venom Bite", Tier: 1, DmgModifier: 80, Cooldown: 2, Weight: 60,
			Status: &MoveStatusAsset{ID: "poison", Chance: 60, Target: "enemy"}},
		{ID: "heavy_slash", Name: "Heavy Slash", Tier: 2, DmgModifier: 175, Cooldown: 2, Weight: 60},
		{ID: "crushing_blow", Name: "Crushing Blow", Tier: 2, DmgModifier: 120, Cooldown: 3, Weight: 40,
			Status: &MoveStatusAsset{ID: "weaken", Chance: 75, Target: "enemy"}},
		{ID: "frost_touch", Name: "Frost Touch", Tier: 3, DmgModifier: 90, Cooldown: 3, Weight: 40,
			Status: &MoveStatusAsset{ID: "slow", Chance: 80, Target: "enemy"}},
	}
}

// validStatuses is the four starter statuses.
func validStatuses() []StatusAsset {
	return []StatusAsset{
		{ID: "poison", Name: "Poison", Kind: "debuff",
			Effect:   StatusEffectAsset{Type: "dot", Amount: 3},
			Duration: 3},
		{ID: "weaken", Name: "Weaken", Kind: "debuff",
			Effect:   StatusEffectAsset{Type: "stat", Stat: "dmg", Amount: -3},
			Duration: 3},
		{ID: "slow", Name: "Slow", Kind: "debuff",
			Effect:   StatusEffectAsset{Type: "stat", Stat: "spd", Amount: -3},
			Duration: 3},
		{ID: "guard", Name: "Guard", Kind: "buff",
			Effect:   StatusEffectAsset{Type: "stat", Stat: "def", Amount: 5},
			Duration: 2},
	}
}

// combat2Reg returns a registry pre-filled with the move/status helpers;
// tests mutate it to break one rule at a time.
func combat2Reg() *Registry {
	r := makeReg(validMonsters(), validSections())
	for _, m := range validMoves() {
		r.Moves[m.ID] = m
	}
	for _, s := range validStatuses() {
		r.Statuses[s.ID] = s
	}
	return r
}

// MovesForTiers mirrors ItemIDsForTiers for moves: assets sorted by id,
// unrestricted when no tier is listed, empty (never an error) pool when
// nothing matches.
func TestMovesForTiers(t *testing.T) {
	r := combat2Reg()

	ids := func(assets []MoveAsset) []string {
		out := make([]string, len(assets))
		for i, a := range assets {
			out[i] = a.ID
		}
		return out
	}

	tests := []struct {
		name  string
		tiers []int
		want  []string
	}{
		{"nil tiers is unrestricted", nil,
			[]string{"basic_attack", "crushing_blow", "frost_touch", "guard_strike", "heavy_slash", "quick_stab", "venom_bite"}},
		{"empty tiers is unrestricted", []int{},
			[]string{"basic_attack", "crushing_blow", "frost_touch", "guard_strike", "heavy_slash", "quick_stab", "venom_bite"}},
		{"single tier", []int{1}, []string{"basic_attack", "guard_strike", "quick_stab", "venom_bite"}},
		{"union of tiers, sorted", []int{3, 1},
			[]string{"basic_attack", "frost_touch", "guard_strike", "quick_stab", "venom_bite"}},
		{"unmatched tier is an empty pool", []int{9}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ids(r.MovesForTiers(tt.tiers))
			if !slices.Equal(got, tt.want) {
				t.Errorf("MovesForTiers(%v) = %v, want %v", tt.tiers, got, tt.want)
			}
		})
	}
}

func TestMaxMoveTier(t *testing.T) {
	if got := combat2Reg().MaxMoveTier(); got != 3 {
		t.Errorf("MaxMoveTier = %d, want 3", got)
	}
	empty := makeReg(validMonsters(), validSections())
	if got := empty.MaxMoveTier(); got != 0 {
		t.Errorf("MaxMoveTier on a move-less registry = %d, want 0", got)
	}
}

func TestMoveByIDAndStatusByID(t *testing.T) {
	r := combat2Reg()

	if m, ok := r.MoveByID("heavy_slash"); !ok || m.Name != "Heavy Slash" {
		t.Errorf("MoveByID(heavy_slash) = (%+v, %v), want the move", m, ok)
	}
	if _, ok := r.MoveByID("nope"); ok {
		t.Error("MoveByID(unknown) reported success")
	}
	if s, ok := r.StatusByID("poison"); !ok || s.Effect.Amount != 3 {
		t.Errorf("StatusByID(poison) = (%+v, %v), want the status", s, ok)
	}
	if _, ok := r.StatusByID("nope"); ok {
		t.Error("StatusByID(unknown) reported success")
	}
}

// The embedded boss groups are table-locked: one boss per finite
// section, none on the infinite tail. Every group references existing
// monsters and is tuned as a considerable challenge for its section's
// LATE floors — see the assets commit for the tuning rationale.
func TestLoadEmbeddedSectionBosses(t *testing.T) {
	r, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := map[string][]string{
		"goblin_pits":           {"goblin_chief", "hobgoblin"},
		"goblin_camp_outskirts": {"orc_warlord", "orc_berserker", "orc"},
		"underdark_reaches":     {"lich", "vampire_spawn", "wraith"},
		"wraithfall_bastion":    {"beholder", "wraith", "wraith"},
		"dragon_hoard":          nil, // the infinite tail: no boss gate, ever
	}
	wantGold := map[string]int{
		"goblin_pits":           250,
		"goblin_camp_outskirts": 1000,
		"underdark_reaches":     2500,
		"wraithfall_bastion":    5000,
		"dragon_hoard":          0,
	}

	for _, s := range r.Sections {
		var got []string
		if s.Boss != nil {
			got = s.Boss.Monsters
		}
		if !slices.Equal(got, want[s.ID]) {
			t.Errorf("section %q boss = %v, want %v", s.ID, got, want[s.ID])
		}
		gold := 0
		if s.Boss != nil {
			gold = s.Boss.Gold
		}
		if gold != wantGold[s.ID] {
			t.Errorf("section %q boss gold = %d, want %d", s.ID, gold, wantGold[s.ID])
		}
	}
}

// The embedded assets ship the canonical basic attack and the four
// starter statuses; this locks their core values.
func TestLoadEmbeddedMovesAndStatuses(t *testing.T) {
	r, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	basic, ok := r.MoveByID("basic_attack")
	if !ok {
		t.Fatal("the canonical basic_attack move is missing from the embedded assets")
	}
	if basic.Tier != 1 || basic.DmgModifier != 100 || basic.CritBonus != 0 ||
		basic.Cooldown != 0 || basic.Weight != 100 || basic.Status != nil {
		t.Errorf("basic_attack drifted: %+v", basic)
	}

	poison, ok := r.StatusByID("poison")
	if !ok {
		t.Fatal("the starter poison status is missing from the embedded assets")
	}
	if poison.Kind != "debuff" || poison.Effect.Type != "dot" ||
		poison.Effect.Amount != 3 || poison.Duration != 3 {
		t.Errorf("poison drifted: %+v", poison)
	}

	guard, ok := r.StatusByID("guard")
	if !ok {
		t.Fatal("the starter guard status is missing from the embedded assets")
	}
	if guard.Kind != "buff" || guard.Effect.Stat != "def" ||
		guard.Effect.Amount != 5 || guard.Duration != 2 {
		t.Errorf("guard drifted: %+v", guard)
	}

	if got, want := len(r.Moves), 8; got != want {
		t.Errorf("embedded moves = %d, want %d (basic attack + 7 starters)", got, want)
	}
	if got, want := len(r.Statuses), 4; got != want {
		t.Errorf("embedded statuses = %d, want %d (the starter set)", got, want)
	}
	if got := r.MaxMoveTier(); got != 3 {
		t.Errorf("embedded MaxMoveTier = %d, want 3", got)
	}
}

// A move without a tier field defaults to tier 1 after Load, mirroring
// items — tier-less moves in mods keep working.
func TestLoadDefaultsAbsentMoveTierToOne(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("monsters.json", `[
		{"id": "rat", "name": "Rat", "difficulty": 1, "hp": 10, "dmg": 1, "spd": 5}
	]`)
	write("sections.json", `[
		{"id": "s", "name": "S", "base_difficulty": 1, "min_floor": 1, "max_floor": -1,
		 "spawns": [{"monster_id": "rat", "weight": 1}]}
	]`)
	write("moves.json", `[
		{"id": "jab", "name": "Jab", "dmg_modifier": 100, "weight": 1},
		{"id": "hook", "name": "Hook", "tier": 2, "dmg_modifier": 150, "cooldown": 1, "weight": 1}
	]`)

	r, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := r.Validate(100); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got := r.Moves["jab"].Tier; got != 1 {
		t.Errorf("tier-less move loaded with tier %d, want 1 (absent = first tier)", got)
	}
	if got := r.Moves["hook"].Tier; got != 2 {
		t.Errorf("explicit tier loaded as %d, want 2", got)
	}
}

// Duplicate ids in the new asset files fail at Load (indexBy pattern),
// never silently hiding content.
func TestLoadRejectsDuplicateMoveAndStatusIDs(t *testing.T) {
	t.Run("duplicate move ids", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "moves.json")
		content := `[
			{"id": "jab", "name": "Jab", "dmg_modifier": 100, "weight": 1},
			{"id": "jab", "name": "Jab Again", "dmg_modifier": 120, "weight": 1}
		]`
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := Load(dir)
		if err == nil || !strings.Contains(err.Error(), "duplicate") {
			t.Errorf("Load(duplicate moves) = %v, want a duplicate id error", err)
		}
	})

	t.Run("duplicate status ids", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "statuses.json")
		content := `[
			{"id": "poison", "name": "Poison", "kind": "debuff",
			 "effect": {"type": "dot", "amount": 3}, "duration": 3},
			{"id": "poison", "name": "Poison II", "kind": "debuff",
			 "effect": {"type": "dot", "amount": 9}, "duration": 5}
		]`
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := Load(dir)
		if err == nil || !strings.Contains(err.Error(), "duplicate") {
			t.Errorf("Load(duplicate statuses) = %v, want a duplicate id error", err)
		}
	})
}

// ---- Combat 2.0 content validation ----

// Combat 2.0 validation: content bugs fail at startup. Each case breaks
// exactly one rule against an otherwise-valid registry; wantErr "" means
// the mutation is legal and must validate.
func TestValidateCombat2Content(t *testing.T) {
	moveRef := func(id string, weight int) MoveRef { return MoveRef{ID: id, Weight: weight} }
	tests := []struct {
		name    string
		mutate  func(r *Registry)
		wantErr string
	}{
		// Move fields.
		{
			"move tier below 1",
			func(r *Registry) { m := r.Moves["heavy_slash"]; m.Tier = 0; r.Moves["heavy_slash"] = m },
			"tier",
		},
		{
			"move dmg_modifier below range",
			func(r *Registry) { m := r.Moves["quick_stab"]; m.DmgModifier = 49; r.Moves["quick_stab"] = m },
			"dmg_modifier",
		},
		{
			"move dmg_modifier above range",
			func(r *Registry) { m := r.Moves["quick_stab"]; m.DmgModifier = 301; r.Moves["quick_stab"] = m },
			"dmg_modifier",
		},
		{
			"negative move cooldown",
			func(r *Registry) { m := r.Moves["heavy_slash"]; m.Cooldown = -1; r.Moves["heavy_slash"] = m },
			"cooldown",
		},
		{
			"move weight below 1",
			func(r *Registry) { m := r.Moves["quick_stab"]; m.Weight = 0; r.Moves["quick_stab"] = m },
			"weight",
		},
		// Move status wiring.
		{
			"move status chance over 100",
			func(r *Registry) {
				m := r.Moves["venom_bite"]
				m.Status.Chance = 101
				r.Moves["venom_bite"] = m
			},
			"chance",
		},
		{
			"move status negative chance",
			func(r *Registry) {
				m := r.Moves["venom_bite"]
				m.Status.Chance = -1
				r.Moves["venom_bite"] = m
			},
			"chance",
		},
		{
			"move status unknown target",
			func(r *Registry) {
				m := r.Moves["guard_strike"]
				m.Status.Target = "foe"
				r.Moves["guard_strike"] = m
			},
			"target",
		},
		{
			"move referencing unknown status",
			func(r *Registry) {
				m := r.Moves["venom_bite"]
				m.Status.ID = "nope"
				r.Moves["venom_bite"] = m
			},
			"unknown status",
		},
		// Status assets.
		{
			"status unknown kind",
			func(r *Registry) { s := r.Statuses["guard"]; s.Kind = "curse"; r.Statuses["guard"] = s },
			"kind",
		},
		{
			"status duration below 1",
			func(r *Registry) { s := r.Statuses["poison"]; s.Duration = 0; r.Statuses["poison"] = s },
			"duration",
		},
		{
			"status unknown effect type",
			func(r *Registry) { s := r.Statuses["poison"]; s.Effect.Type = "aura"; r.Statuses["poison"] = s },
			"effect type",
		},
		{
			"dot amount below 1",
			func(r *Registry) { s := r.Statuses["poison"]; s.Effect.Amount = 0; r.Statuses["poison"] = s },
			"dot amount",
		},
		{
			"stat amount zero",
			func(r *Registry) { s := r.Statuses["weaken"]; s.Effect.Amount = 0; r.Statuses["weaken"] = s },
			"zero stat amount",
		},
		{
			"status unknown stat",
			func(r *Registry) { s := r.Statuses["slow"]; s.Effect.Stat = "chi"; r.Statuses["slow"] = s },
			"unknown stat",
		},
		// Monster move references.
		{
			"monster referencing unknown move",
			func(r *Registry) {
				m := r.Monsters["rat"]
				m.Moves = []MoveRef{moveRef("nope", 1)}
				r.Monsters["rat"] = m
			},
			"unknown move",
		},
		{
			"monster with more than 4 moves",
			func(r *Registry) {
				m := r.Monsters["rat"]
				m.Moves = []MoveRef{
					moveRef("basic_attack", 1), moveRef("quick_stab", 1), moveRef("heavy_slash", 1),
					moveRef("frost_touch", 1), moveRef("crushing_blow", 1),
				}
				r.Monsters["rat"] = m
			},
			"at most 4",
		},
		{
			"monster move ref weight below 1",
			func(r *Registry) {
				m := r.Monsters["rat"]
				m.Moves = []MoveRef{moveRef("basic_attack", 0)}
				r.Monsters["rat"] = m
			},
			"weight",
		},
		// Weapon-granted moves.
		{
			"weapon referencing unknown move",
			func(r *Registry) {
				r.Items["sword"] = ItemAsset{ID: "sword", Name: "Sword", Slot: "weapon", Tier: 1, Moves: []string{"nope"}}
			},
			"unknown move",
		},
		{
			"weapon with more than 2 moves",
			func(r *Registry) {
				r.Items["sword"] = ItemAsset{ID: "sword", Name: "Sword", Slot: "weapon", Tier: 1,
					Moves: []string{"basic_attack", "quick_stab", "heavy_slash"}}
			},
			"at most 2",
		},
		{
			"non-weapon slot carrying moves",
			func(r *Registry) {
				r.Items["helm"] = ItemAsset{ID: "helm", Name: "Helm", Slot: "helmet", Tier: 1,
					Moves: []string{"heavy_slash"}}
			},
			"only weapons",
		},
		// Section move_tiers.
		{
			"move_tiers value below 1",
			func(r *Registry) { r.Sections[0].MoveTiers = []int{1, 0} },
			"move_tiers",
		},
		{
			"move_tiers referencing an empty tier (typo)",
			func(r *Registry) { r.Sections[0].MoveTiers = []int{1, 9} },
			"no moves of that tier",
		},
		// Legal content.
		{
			"monster with a valid move list",
			func(r *Registry) {
				m := r.Monsters["rat"]
				m.Moves = []MoveRef{moveRef("basic_attack", 100), moveRef("venom_bite", 40)}
				r.Monsters["rat"] = m
			},
			"",
		},
		{
			"weapon with valid granted moves",
			func(r *Registry) {
				r.Items["sword"] = ItemAsset{ID: "sword", Name: "Sword", Slot: "weapon", Tier: 1,
					Moves: []string{"heavy_slash", "quick_stab"}}
			},
			"",
		},
		{
			"section with populated move_tiers",
			func(r *Registry) { r.Sections[0].MoveTiers = []int{1, 2} },
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := combat2Reg()
			tt.mutate(r)
			err := r.Validate(10)

			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate = %v, want nil (legal content)", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
