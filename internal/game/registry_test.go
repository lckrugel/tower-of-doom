package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

// The first published slice has disjoint boundaries and retains legacy budgets
// after floor 15; validation also checks all new references and weapon grants.
func TestEmbeddedProgression(t *testing.T) {
	r, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(100_000); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		floor, budget int
		section       string
		boss          bool
	}{
		{1, 9, "goblin_pits", false}, {5, 13, "goblin_pits", true},
		{6, 18, "goblin_camp_outskirts", false}, {15, 27, "goblin_camp_outskirts", true},
		{16, 44, "underdark_reaches", false}, {33, 163, "underdark_reaches", true},
		{34, 40, "wraithfall_bastion", false}, {59, 290, "wraithfall_bastion", true},
		{60, 60, "dragon_hoard", false}, {100_000, 1_199_340, "dragon_hoard", false},
	} {
		s, err := r.SectionForFloor(tt.floor)
		if err != nil {
			t.Fatal(err)
		}
		if s.ID != tt.section || s.FloorDifficulty(tt.floor) != tt.budget || r.IsSectionEnd(tt.floor) != tt.boss {
			t.Errorf("floor %d: section=%s budget=%d boss=%v", tt.floor, s.ID, s.FloorDifficulty(tt.floor), r.IsSectionEnd(tt.floor))
		}
	}
	for _, tt := range []struct {
		floor           int
		name            string
		tier            int
		spawns          []SpawnEntry
		learned, bosses []string
		gold            int
	}{
		{1, "Goblin Gateworks", 1,
			[]SpawnEntry{{MonsterID: "goblin", Weight: 100}, {MonsterID: "goblin_scout", Weight: 60}, {MonsterID: "giant_rat", Weight: 25}, {MonsterID: "kobold", Weight: 25}, {MonsterID: "goblin_bomber", Weight: 15}},
			[]string{"field_dressing", "guard_strike", "hamstring", "measured_strike"}, []string{"goblin_chief", "hobgoblin"}, 250},
		{6, "Emberforged Barracks", 2,
			[]SpawnEntry{{MonsterID: "orc", Weight: 40}, {MonsterID: "orc_berserker", Weight: 10}, {MonsterID: "hobgoblin", Weight: 30}, {MonsterID: "ember_stoker", Weight: 20}, {MonsterID: "barracks_sentinel", Weight: 30}},
			[]string{"cinderbite", "heavy_slash", "windstep"}, []string{"orc_warlord", "orc_berserker", "orc"}, 1000},
	} {
		s, _ := r.SectionForFloor(tt.floor)
		if s.Name != tt.name || !slices.Equal(s.ItemTiers, []int{tt.tier}) || !slices.Equal(s.MoveTiers, []int{tt.tier}) || !reflect.DeepEqual(s.Spawns, tt.spawns) {
			t.Errorf("floor %d: unexpected section pools: %+v", tt.floor, s)
		}
		var learned []string
		for _, m := range r.MovesForTiers(s.MoveTiers) {
			learned = append(learned, m.ID)
		}
		if !slices.Equal(learned, tt.learned) {
			t.Errorf("floor %d learned pool = %v, want %v", tt.floor, learned, tt.learned)
		}
		if s.Boss == nil || !slices.Equal(s.Boss.Monsters, tt.bosses) || s.Boss.Gold != tt.gold {
			t.Errorf("floor %d boss = %+v", tt.floor, s.Boss)
		}
		// Both early tiers cover all six slots, including their new accessories.
		slots := map[string]bool{}
		for _, id := range r.ItemIDsForTiers(s.ItemTiers) {
			slots[r.Items[id].Slot] = true
		}
		for _, slot := range []string{"weapon", "helmet", "body_armor", "boots", "ring", "amulet"} {
			if !slots[slot] {
				t.Errorf("tier %d lacks %s", tt.tier, slot)
			}
		}
	}
}

func TestEmbeddedEarlyMonsterKits(t *testing.T) {
	r, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	guardAI := &MonsterAIAsset{
		Priorities: []MonsterPriorityAsset{{When: []MonsterConditionAsset{{Type: "status_absent", Target: "self", StatusID: "guard"}}, Moves: []string{"guard_strike"}}},
		Fallback:   []string{"basic_attack"},
	}
	for _, tt := range []struct {
		id    string
		moves []MoveRef
		ai    *MonsterAIAsset
	}{
		{"goblin_scout", []MoveRef{{ID: "quick_stab", Weight: 100}}, nil},
		{"kobold", []MoveRef{{ID: "guard_strike", Weight: 100}, {ID: "basic_attack", Weight: 100}}, guardAI},
		{"barracks_sentinel", []MoveRef{{ID: "guard_strike", Weight: 100}, {ID: "basic_attack", Weight: 100}}, guardAI},
		{"ember_stoker", []MoveRef{{ID: "cinderbite", Weight: 35}, {ID: "basic_attack", Weight: 65}}, nil},
	} {
		m, ok := r.Monsters[tt.id]
		if !ok || !slices.Equal(m.Moves, tt.moves) || !reflect.DeepEqual(m.AI, tt.ai) {
			t.Errorf("%s: moves=%+v AI=%+v", tt.id, m.Moves, m.AI)
		}
	}
}

func TestMoveEffectsDecodeAndValidate(t *testing.T) {
	var mv MoveAsset
	err := json.Unmarshal([]byte(`{"id":"m","name":"M","type":"both","tier":1,"cooldown":0,"weight":1,"effects":[{"type":"damage","target":"enemy","dmg_modifier":120},{"type":"heal","target":"self","flat":5,"percent":10},{"type":"status","target":"enemy","status":{"id":"poison","chance":50}}]}`), &mv)
	if err != nil {
		t.Fatal(err)
	}
	if len(mv.Effects) != 3 || mv.Effects[1].Type != "heal" {
		t.Fatalf("effects = %#v", mv.Effects)
	}
	if mv.Effects[1].Flat == nil || *mv.Effects[1].Flat != 5 || mv.Effects[1].Percent == nil || *mv.Effects[1].Percent != 10 {
		t.Fatalf("heal payload = %#v", mv.Effects[1])
	}
}

// A percentage DoT at the allowed upper bound and a negative stat percentage
// are valid independently of the shipped balance values.
func TestValidateAcceptsPercentageEffects(t *testing.T) {
	r := combat2Reg()
	poison := r.Statuses["poison"]
	poison.Effect.Percent = 100
	r.Statuses["poison"] = poison
	slow := r.Statuses["slow"]
	slow.Effect.Percent = -10
	r.Statuses["slow"] = slow
	if err := r.Validate(10_000); err != nil {
		t.Errorf("Validate(percentage effects) = %v, want nil", err)
	}
}

func TestMonsterAIValidation(t *testing.T) {
	base := func() *Registry {
		r := makeReg([]MonsterAsset{{ID: "rat", Name: "Rat", Difficulty: 1, HP: 10, Moves: []MoveRef{{ID: "hit", Weight: 1}}, AI: &MonsterAIAsset{Fallback: []string{"hit"}, Priorities: []MonsterPriorityAsset{{When: []MonsterConditionAsset{{Type: "hp_below", Target: "self", Percent: 50}}, Moves: []string{"hit"}}}}}}, []SectionAsset{{ID: "s", MinFloor: 1, MaxFloor: -1, BaseDifficulty: 1, Spawns: []SpawnEntry{{MonsterID: "rat", Weight: 1}}}})
		r.Moves["hit"] = MoveAsset{ID: "hit", Type: "creature", Tier: 1, Weight: 1, Effects: []MoveEffectAsset{{Type: "damage", Target: "enemy", DmgModifier: intPtr(100)}}}
		return r
	}
	if err := base().Validate(10); err != nil {
		t.Fatalf("valid AI: %v", err)
	}
	tests := []struct {
		name   string
		change func(*MonsterAsset)
	}{
		{"empty fallback", func(m *MonsterAsset) { m.AI.Fallback = nil }},
		{"move outside kit", func(m *MonsterAsset) { m.AI.Fallback = []string{"missing"} }},
		{"empty conditions", func(m *MonsterAsset) { m.AI.Priorities[0].When = nil }},
		{"empty rule moves", func(m *MonsterAsset) { m.AI.Priorities[0].Moves = nil }},
		{"invalid condition", func(m *MonsterAsset) { m.AI.Priorities[0].When[0].Type = "wat" }},
		{"hp target", func(m *MonsterAsset) { m.AI.Priorities[0].When[0].Target = "enemy" }},
		{"hp range", func(m *MonsterAsset) { m.AI.Priorities[0].When[0].Percent = 101 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := base()
			m := r.Monsters["rat"]
			tt.change(&m)
			r.Monsters["rat"] = m
			if err := r.Validate(10); err == nil {
				t.Fatal("Validate = nil, want AI validation error")
			}
		})
	}
}

func intPtr(n int) *int { return &n }

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
			"zero monster HP",
			[]MonsterAsset{{ID: "rat", Name: "Rat", Difficulty: 1, HP: 0}},
			[]SectionAsset{{ID: "s", Name: "S", MinFloor: 1, MaxFloor: -1,
				Spawns: []SpawnEntry{{MonsterID: "rat", Weight: 1}}}},
			"hp",
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

func TestValidateChecksBossOnlyMonsterSchema(t *testing.T) {
	for _, tt := range []struct {
		name    string
		monster MonsterAsset
		wantErr string
	}{
		{"difficulty", MonsterAsset{Difficulty: 0, HP: 10}, `has difficulty 0`},
		{"HP", MonsterAsset{Difficulty: 1, HP: 0}, `has hp 0`},
		{"defense", MonsterAsset{Difficulty: 1, HP: 10, Defense: -1}, `has def -1`},
		{"crit", MonsterAsset{Difficulty: 1, HP: 10, Crit: 101}, `has crit 101`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			monster := tt.monster
			monster.ID, monster.Name = "boss_only", "Boss Only"
			monsters := append(validMonsters(), monster)
			sections := validSections()
			sections[0].Boss = &BossAsset{Monsters: []string{"boss_only"}}

			err := makeReg(monsters, sections).Validate(10)
			if err == nil || !strings.Contains(err.Error(), `monster "boss_only" `+tt.wantErr) {
				t.Fatalf("Validate(boss-only monster) = %v, want intrinsic schema error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRejectsUnknownItemSlots(t *testing.T) {
	r := makeReg(validMonsters(), validSections())
	r.Items["misspelled"] = ItemAsset{ID: "misspelled", Name: "Misspelled", Slot: "helment", Tier: 1}

	err := r.Validate(10)
	if err == nil || !strings.Contains(err.Error(), `item "misspelled" has unknown slot "helment"`) {
		t.Fatalf("Validate(unknown item slot) = %v, want unknown slot error", err)
	}
}

func TestLoadJSONRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	contents := `[{"id":"rat","name":"Rat","difficulty":1,"hp":10,"dmg":1,"spd":5,"dificulty":1}]`
	if err := os.WriteFile(filepath.Join(dir, "monsters.json"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := loadJSON[MonsterAsset](dir, "monsters.json")
	if err == nil || !strings.Contains(err.Error(), `unknown field "dificulty"`) {
		t.Fatalf("loadJSON with a misspelled field = %v, want unknown-field error", err)
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

// regWithItems fills the registry's item index on top of the Combat 2.0
// move/status helpers (combat2Reg) — the weapon rules (explicit
// spammables, grant types) need every grant resolvable for Validate to
// pass.
func regWithItems(items []ItemAsset, sections []SectionAsset) *Registry {
	r := combat2Reg()
	r.Sections = sections
	for _, it := range items {
		r.Items[it.ID] = it
	}
	return r
}

// tierItems is a small item registry spanning tiers 1–3. The weapon
// declares its moves explicitly (the explicit-spammable rule) with a
// move the helper registries carry.
func tierItems() []ItemAsset {
	return []ItemAsset{
		{ID: "sword", Name: "Sword", Slot: "weapon", BonusDMG: 5, Durability: 10, Tier: 1,
			Moves: []string{"basic_attack"}},
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
		{"id": "sword", "slot": "weapon", "name": "Sword", "bonus_dmg": 5, "durability": 10,
		 "moves": ["basic_attack"]},
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
	items, source, err := loadJSON[MonsterAsset](t.TempDir(), "monsters.json")
	if err != nil {
		t.Fatalf("loadJSON: %v", err)
	}
	if source != "embedded" {
		t.Errorf("loadJSON source = %q, want \"embedded\"", source)
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

	_, _, err := loadJSON[MonsterAsset](dir, "monsters.json")

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
// basic_attack included. Status wiring and move TYPES mirror the
// embedded content — including one weapon-only move (steel_flurry), so
// the shrine-pool filter has something to exclude.
func validMoves() []MoveAsset {
	ptr := func(v int) *int { return &v }
	damage := func(mod int, crit *int) []MoveEffectAsset {
		e := MoveEffectAsset{Type: "damage", Target: "enemy", DmgModifier: ptr(mod)}
		if crit != nil {
			e.CritBonus = crit
		}
		return []MoveEffectAsset{e}
	}
	status := func(mod int, id, target string, chance int) []MoveEffectAsset {
		e := MoveEffectAsset{Type: "damage", Target: "enemy", DmgModifier: ptr(mod)}
		s := MoveEffectAsset{Type: "status", Target: target, Status: &MoveStatusEffectAsset{ID: id, Chance: chance}}
		return []MoveEffectAsset{e, s}
	}
	moves := []MoveAsset{
		{ID: "basic_attack", Name: "Basic Attack", Tier: 1, Effects: damage(100, nil), Weight: 100, Type: "both"},
		{ID: "quick_stab", Name: "Quick Stab", Tier: 1, Effects: damage(60, ptr(20)), Weight: 100, Type: "both"},
		{ID: "guard_strike", Name: "Guard Strike", Tier: 1, Effects: status(75, "guard", "self", 100), Cooldown: 2, Weight: 50, Type: "both"},
		{ID: "venom_bite", Name: "Venom Bite", Tier: 1, Effects: status(80, "poison", "enemy", 60), Cooldown: 2, Weight: 60, Type: "creature"},
		{ID: "heavy_slash", Name: "Heavy Slash", Tier: 2, Effects: damage(175, nil), Cooldown: 2, Weight: 60, Type: "both"},
		{ID: "crushing_blow", Name: "Crushing Blow", Tier: 2, Effects: status(120, "weaken", "enemy", 75), Cooldown: 3, Weight: 40, Type: "creature"},
		{ID: "steel_flurry", Name: "Steel Flurry", Tier: 3, Effects: damage(110, nil), Weight: 70, Type: "weapon"},
		{ID: "frost_touch", Name: "Frost Touch", Tier: 3, Effects: status(90, "slow", "enemy", 80), Cooldown: 3, Weight: 40, Type: "creature"},
		{ID: "brand_of_the_breach", Name: "Brand of the Breach", Tier: 3, Effects: []MoveEffectAsset{
			{Type: "status", Target: "self", Status: &MoveStatusEffectAsset{ID: "breachfire", Chance: 100}},
			{Type: "status", Target: "self", Status: &MoveStatusEffectAsset{ID: "open_guard", Chance: 100}},
		}, Cooldown: 3, Weight: 25, Type: "creature"},
	}
	learnable := map[string]bool{"guard_strike": true, "heavy_slash": true, "crushing_blow": true, "frost_touch": true, "brand_of_the_breach": true}
	for i := range moves {
		moves[i].Learnable = learnable[moves[i].ID]
	}
	return moves
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
		{ID: "breachfire", Name: "Breachfire", Kind: "buff", Effect: StatusEffectAsset{Type: "stat", Stat: "dmg", Percent: 35}, Duration: 2},
		{ID: "open_guard", Name: "Open Guard", Kind: "debuff", Effect: StatusEffectAsset{Type: "stat", Stat: "def", Percent: -50}, Duration: 2},
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
// nothing matches. It is the SHRINE's pool query, so it is also
// type-filtered: creature- or both-typed moves only — a weapon-only
// move (steel_flurry) never enters the pool a shrine could draw from.
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
		{"nil tiers is unrestricted (minus weapon-only moves)", nil,
			[]string{"brand_of_the_breach", "crushing_blow", "frost_touch", "guard_strike", "heavy_slash"}},
		{"empty tiers is unrestricted", []int{},
			[]string{"brand_of_the_breach", "crushing_blow", "frost_touch", "guard_strike", "heavy_slash"}},
		{"single tier", []int{1}, []string{"guard_strike"}},
		{"union of tiers, sorted", []int{3, 1},
			[]string{"brand_of_the_breach", "frost_touch", "guard_strike"}},
		{"tier 3 creature moves are shrine-available", []int{3}, []string{"brand_of_the_breach", "frost_touch"}},
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

// AllStatuses is the read-only status surface for lore/UI consumers
// (the registry's map is unexported to them): every status, sorted by
// id, as a copy.
func TestAllStatuses(t *testing.T) {
	t.Run("every status, sorted by id", func(t *testing.T) {
		got := combat2Reg().AllStatuses()

		if len(got) != len(validStatuses()) {
			t.Fatalf("AllStatuses = %d entries, want %d", len(got), len(validStatuses()))
		}
		ids := make([]string, len(got))
		for i, s := range got {
			ids[i] = s.ID
		}
		if !slices.IsSorted(ids) {
			t.Errorf("AllStatuses ids = %v, want sorted", ids)
		}
	})

	t.Run("the result is a copy, not an alias", func(t *testing.T) {
		r := combat2Reg()

		got := r.AllStatuses()
		got[0] = StatusAsset{ID: "tampered"}

		if again := r.AllStatuses(); again[0].ID == "tampered" {
			t.Errorf("mutating the returned slice changed the registry: %+v", again[0])
		}
		if _, ok := r.StatusByID("tampered"); ok {
			t.Error("the mutated asset leaked into the registry")
		}
	})

	t.Run("an empty registry is safe", func(t *testing.T) {
		if got := makeReg(validMonsters(), validSections()).AllStatuses(); len(got) != 0 {
			t.Errorf("AllStatuses on a status-less registry = %+v, want empty", got)
		}
	})
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
	// A weapon whose moves reference this registry's own moves isolates
	// the fixture from the embedded items (whose grants reference moves
	// this registry does not carry). Every weapon declares its moves
	// explicitly (the explicit-spammable rule).
	write("items.json", `[
		{"id": "stick", "slot": "weapon", "name": "Stick", "bonus_dmg": 1, "durability": 5,
		 "moves": ["jab"]}
	]`)
	write("sections.json", `[
		{"id": "s", "name": "S", "base_difficulty": 1, "min_floor": 1, "max_floor": -1,
		 "spawns": [{"monster_id": "rat", "weight": 1}]}
	]`)
	write("moves.json", `[
		{"id": "jab", "name": "Jab", "type": "both", "effects":[{"type":"damage","target":"enemy","dmg_modifier":100}], "weight": 1},
		{"id": "hook", "name": "Hook", "type": "creature", "tier": 2, "effects":[{"type":"damage","target":"enemy","dmg_modifier":150}], "cooldown": 1, "weight": 1}
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
			{"id": "jab", "name": "Jab", "effects":[{"type":"damage","target":"enemy","dmg_modifier":100}], "weight": 1},
			{"id": "jab", "name": "Jab Again", "effects":[{"type":"damage","target":"enemy","dmg_modifier":120}], "weight": 1}
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

// ---- Asset provenance: where each loaded file came from ----

// Load records WHERE each asset file came from — "embedded" or
// "external (<dir>)" — per file, respecting the per-file fallback
// rules: an existing external file wins, a missing one falls back to
// the compiled-in assets. Provenance() hands that record out read-only.
func TestLoadProvenance(t *testing.T) {
	files := []string{"monsters.json", "items.json", "moves.json", "statuses.json", "consumables.json", "sections.json"}

	t.Run("no external dir: everything is embedded", func(t *testing.T) {
		r, err := Load("")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}

		prov := r.Provenance()
		if len(prov) != len(files) {
			t.Errorf("Provenance has %d entries, want %d: %v", len(prov), len(files), prov)
		}
		for _, file := range files {
			if got := prov[file]; got != "embedded" {
				t.Errorf("provenance[%q] = %q, want \"embedded\"", file, got)
			}
		}
	})

	t.Run("a mixed mod dir: external files win, missing fall back", func(t *testing.T) {
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

		r, err := Load(dir)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}

		prov := r.Provenance()
		for _, file := range []string{"monsters.json", "sections.json"} {
			if got := prov[file]; got != "external ("+dir+")" {
				t.Errorf("provenance[%q] = %q, want \"external (%s)\"", file, got, dir)
			}
		}
		for _, file := range []string{"items.json", "moves.json", "statuses.json", "consumables.json"} {
			if got := prov[file]; got != "embedded" {
				t.Errorf("provenance[%q] = %q, want \"embedded\" (file missing from the dir)", file, got)
			}
		}
	})

	t.Run("the returned map is a copy, not an alias", func(t *testing.T) {
		r, err := Load("")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}

		prov := r.Provenance()
		prov["items.json"] = "tampered"

		if again := r.Provenance(); again["items.json"] != "embedded" {
			t.Errorf("mutating the returned map changed the registry: provenance[items.json] = %q", again["items.json"])
		}
	})
}

// ---- Distribution: the conventional mods directory ----

// DefaultModsDir is a pure convention table: the caller injects the
// environment, so every platform branch — the XDG absolute-path rule,
// the APPDATA fallback and the missing-home cases — is table-testable
// without touching the process environment.
func TestDefaultModsDir(t *testing.T) {
	hostAbs := func(path string) string {
		abs, err := filepath.Abs(path)
		if err != nil {
			t.Fatal(err)
		}
		return abs
	}
	xdgData, home := hostAbs(filepath.Join("xdg", "data")), hostAbs("hero-home")
	homeHero := filepath.Join(home, "hero")
	env := func(kv ...string) func(string) string {
		vars := make(map[string]string, len(kv)/2)
		for i := 0; i+1 < len(kv); i += 2 {
			vars[kv[i]] = kv[i+1]
		}
		return func(key string) string { return vars[key] }
	}

	tests := []struct {
		name   string
		goos   string
		getenv func(string) string
		want   string
	}{
		{
			"linux honours an absolute XDG_DATA_HOME",
			"linux", env("XDG_DATA_HOME", xdgData, "HOME", homeHero),
			filepath.Join(xdgData, "tower-of-doom", "mods"),
		},
		{
			"linux rejects a relative XDG_DATA_HOME (the XDG spec requires absolute)",
			"linux", env("XDG_DATA_HOME", "relative/data", "HOME", homeHero),
			filepath.Join(homeHero, ".local", "share", "tower-of-doom", "mods"),
		},
		{
			"linux falls back to ~/.local/share without XDG_DATA_HOME",
			"linux", env("HOME", homeHero),
			filepath.Join(homeHero, ".local", "share", "tower-of-doom", "mods"),
		},
		{
			"linux accepts an absolute XDG_DATA_HOME without a home",
			"linux", env("XDG_DATA_HOME", xdgData),
			filepath.Join(xdgData, "tower-of-doom", "mods"),
		},
		{
			"linux gives up without a home or XDG_DATA_HOME",
			"linux", env(),
			"",
		},
		{
			"unknown goos follows the unix convention",
			"freebsd", env("XDG_DATA_HOME", xdgData, "HOME", homeHero),
			filepath.Join(xdgData, "tower-of-doom", "mods"),
		},
		{
			"darwin uses ~/Library/Application Support",
			"darwin", env("HOME", "/Users/hero"),
			filepath.Join("/Users/hero", "Library", "Application Support", "tower-of-doom", "mods"),
		},
		{
			"darwin gives up without a home",
			"darwin", env("XDG_DATA_HOME", "/xdg/data"),
			"",
		},
		{
			"windows uses %APPDATA%",
			"windows", env("APPDATA", "/appdata", "HOME", "/home/hero"),
			filepath.Join("/appdata", "tower-of-doom", "mods"),
		},
		{
			"windows falls back to ~/AppData/Roaming without APPDATA",
			"windows", env("HOME", "/home/hero"),
			filepath.Join("/home/hero", "AppData", "Roaming", "tower-of-doom", "mods"),
		},
		{
			"windows gives up without APPDATA or a home",
			"windows", env(),
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultModsDir(tt.goos, tt.getenv)

			if got != tt.want {
				t.Errorf("DefaultModsDir(%q) = %q, want %q", tt.goos, got, tt.want)
			}
			if got != "" && filepath.Base(got) != "mods" {
				t.Errorf("DefaultModsDir(%q) = %q, want it to end with the \"mods\" element", tt.goos, got)
			}
		})
	}

	// The CLI's environment override name is part of the contract.
	if ModsEnvVar != "TOD_MODS" {
		t.Errorf("ModsEnvVar = %q, want %q", ModsEnvVar, "TOD_MODS")
	}
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
			"damage effect modifier below range",
			func(r *Registry) {
				m := r.Moves["quick_stab"]
				v := 49
				m.Effects[0].DmgModifier = &v
				r.Moves["quick_stab"] = m
			},
			"dmg_modifier",
		},
		{
			"damage effect modifier above range",
			func(r *Registry) {
				m := r.Moves["quick_stab"]
				v := 301
				m.Effects[0].DmgModifier = &v
				r.Moves["quick_stab"] = m
			},
			"dmg_modifier",
		},
		{
			"valid heal effect",
			func(r *Registry) {
				m := r.Moves["quick_stab"]
				flat := 5
				m.Effects = []MoveEffectAsset{{Type: "heal", Target: "self", Flat: &flat}}
				r.Moves["quick_stab"] = m
			},
			"",
		},
		{
			"valid status-only effect",
			func(r *Registry) {
				m := r.Moves["quick_stab"]
				m.Effects = []MoveEffectAsset{{Type: "status", Target: "self", Status: &MoveStatusEffectAsset{ID: "guard", Chance: 100}}}
				r.Moves["quick_stab"] = m
			},
			"",
		},
		{
			"damage effect cannot target self",
			func(r *Registry) { m := r.Moves["quick_stab"]; m.Effects[0].Target = "self"; r.Moves["quick_stab"] = m },
			"damage target",
		},
		{
			"heal effect must target self",
			func(r *Registry) {
				m := r.Moves["quick_stab"]
				flat := 5
				m.Effects = []MoveEffectAsset{{Type: "heal", Target: "enemy", Flat: &flat}}
				r.Moves["quick_stab"] = m
			},
			"heal target",
		},
		{
			"healing effect cannot be zero",
			func(r *Registry) {
				m := r.Moves["quick_stab"]
				zero := 0
				m.Effects = []MoveEffectAsset{{Type: "heal", Target: "self", Flat: &zero}}
				r.Moves["quick_stab"] = m
			},
			"no positive healing component",
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
		// Move types: the word is required (no absent-field default) and
		// gates which consumer may carry the move.
		{
			"move with a missing type",
			func(r *Registry) { m := r.Moves["heavy_slash"]; m.Type = ""; r.Moves["heavy_slash"] = m },
			"has type",
		},
		{
			"move with an unknown type",
			func(r *Registry) { m := r.Moves["heavy_slash"]; m.Type = "psychic"; r.Moves["heavy_slash"] = m },
			"has type",
		},
		{
			"weapon granting a creature-only move",
			func(r *Registry) {
				r.Items["sword"] = ItemAsset{ID: "sword", Name: "Sword", Slot: "weapon", Tier: 1,
					Moves: []string{"venom_bite"}}
			},
			"weapon or both",
		},
		{
			"monster referencing a weapon-only move",
			func(r *Registry) {
				m := r.Monsters["rat"]
				m.Moves = []MoveRef{moveRef("steel_flurry", 1)}
				r.Monsters["rat"] = m
			},
			"creature or both",
		},
		// Move status wiring.
		{
			"move status chance over 100",
			func(r *Registry) {
				m := r.Moves["venom_bite"]
				m.Effects[1].Status.Chance = 101
				r.Moves["venom_bite"] = m
			},
			"invalid status payload",
		},
		{
			"move status negative chance",
			func(r *Registry) {
				m := r.Moves["venom_bite"]
				m.Effects[1].Status.Chance = -1
				r.Moves["venom_bite"] = m
			},
			"invalid status payload",
		},
		{
			"move status missing payload",
			func(r *Registry) { m := r.Moves["venom_bite"]; m.Effects[1].Status = nil; r.Moves["venom_bite"] = m },
			"missing status payload",
		},
		{
			"move status unknown target",
			func(r *Registry) {
				m := r.Moves["guard_strike"]
				m.Effects[1].Target = "foe"
				r.Moves["guard_strike"] = m
			},
			"target",
		},
		{
			"status effect missing payload",
			func(r *Registry) { m := r.Moves["venom_bite"]; m.Effects[1].Status = nil; r.Moves["venom_bite"] = m },
			"missing status payload",
		},
		{
			"move referencing unknown status",
			func(r *Registry) {
				m := r.Moves["venom_bite"]
				m.Effects[1].Status.ID = "nope"
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
			"no dot component",
		},
		{
			"stat amount zero",
			func(r *Registry) { s := r.Statuses["weaken"]; s.Effect.Amount = 0; r.Statuses["weaken"] = s },
			"no stat modifier",
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
			"exactly one",
		},
		{
			"non-weapon slot carrying moves",
			func(r *Registry) {
				r.Items["helm"] = ItemAsset{ID: "helm", Name: "Helm", Slot: "helmet", Tier: 1,
					Moves: []string{"heavy_slash"}}
			},
			"only weapons",
		},
		// The spammable rule: a weapon that grants moves REPLACES the
		// basic_attack fallback, so its list must carry at least one
		// cooldown-0 move.
		{
			"weapon whose only move is on cooldown",
			func(r *Registry) {
				r.Items["sword"] = ItemAsset{ID: "sword", Name: "Sword", Slot: "weapon", Tier: 1,
					Moves: []string{"heavy_slash"}}
			},
			"cooldown 0",
		},
		{
			"weapon whose every move is on cooldown",
			func(r *Registry) {
				r.Items["sword"] = ItemAsset{ID: "sword", Name: "Sword", Slot: "weapon", Tier: 1,
					Moves: []string{"heavy_slash", "crushing_blow"}}
			},
			"exactly one",
		},
		{
			"weapon with a lone spammable",
			func(r *Registry) {
				r.Items["sword"] = ItemAsset{ID: "sword", Name: "Sword", Slot: "weapon", Tier: 1,
					Moves: []string{"quick_stab"}}
			},
			"",
		},
		// Every weapon declares its moves explicitly: an absent or empty
		// grant list no longer keeps the basic_attack fallback — it is a
		// content bug (BREAKING for move-less mods, intended).
		{
			"weapon with no moves list",
			func(r *Registry) {
				r.Items["sword"] = ItemAsset{ID: "sword", Name: "Sword", Slot: "weapon", Tier: 1}
			},
			"exactly one",
		},
		{
			"weapon with an empty moves list",
			func(r *Registry) {
				r.Items["sword"] = ItemAsset{ID: "sword", Name: "Sword", Slot: "weapon", Tier: 1,
					Moves: []string{}}
			},
			"exactly one",
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
			"no learnable moves of that tier",
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
					Moves: []string{"quick_stab"}}
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

func TestMaxWeaponMoves(t *testing.T) {
	if MaxWeaponMoves != 1 {
		t.Fatalf("MaxWeaponMoves = %d, want 1", MaxWeaponMoves)
	}
}

func TestValidateAuthoredPunch(t *testing.T) {
	base := func() *Registry {
		r := combat2Reg()
		punch := r.Moves["quick_stab"]
		punch.ID, punch.Name, punch.Type = "punch", "Punch", "both"
		punch.Cooldown, punch.Learnable = 0, false
		r.Moves["punch"] = punch
		return r
	}
	if err := base().Validate(10); err != nil {
		t.Fatalf("valid authored punch: %v", err)
	}
	for _, tt := range []struct {
		name string
		edit func(*MoveAsset)
		want string
	}{
		{"learnable", func(m *MoveAsset) { m.Learnable = true }, "must not be learnable"},
		{"creature-only", func(m *MoveAsset) { m.Type = "creature" }, "must be weapon-compatible"},
		{"cooldown", func(m *MoveAsset) { m.Cooldown = 1 }, "must have cooldown 0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := base()
			punch := r.Moves["punch"]
			tt.edit(&punch)
			r.Moves["punch"] = punch
			err := r.Validate(10)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

// Validate collects every fault in one pass instead of stopping at the
// first one: a multi-fault registry reports ALL of them, one per line,
// in a deterministic order (assets walked by id, sections in slice
// order, floors ascending). Every category appears — asset schema,
// references, section rules, floor coverage — and every instance of a
// category (two broken sections yield two lines, not one).
func TestValidateCollectsEveryError(t *testing.T) {
	r := combat2Reg()

	// Two faults on the SAME status asset, plus one fault per remaining
	// category: moves (×2, sorted order pinned), monsters, items,
	// sections (×2, one per section) and floor coverage.
	poison := r.Statuses["poison"]
	poison.Kind = "curse"
	poison.Duration = 0
	r.Statuses["poison"] = poison

	stab := r.Moves["quick_stab"]
	v := 10
	stab.Effects[0].DmgModifier = &v
	r.Moves["quick_stab"] = stab

	bite := r.Moves["venom_bite"]
	bite.Effects[1].Status.ID = "nope"
	r.Moves["venom_bite"] = bite

	rat := r.Monsters["rat"]
	rat.Moves = []MoveRef{{ID: "ghost_move", Weight: 1}}
	r.Monsters["rat"] = rat

	r.Items["charm"] = ItemAsset{ID: "charm", Name: "Charm", Slot: "ring", Tier: 1, Moves: []string{"heavy_slash"}}

	r.Sections[0].ItemTiers = []int{9}
	r.Sections[0].BaseDifficulty = 0 // floor 1's budget drops below the rat
	r.Sections[1].ItemTiers = []int{7}

	err := r.Validate(10)
	if err == nil {
		t.Fatal("Validate = nil, want the collected fault report")
	}

	want := []string{
		`status "poison" has kind "curse"`,
		`status "poison" has duration 0`,
		`move "quick_stab" effect 0 has invalid dmg_modifier`,
		`move "venom_bite" effect 1 references unknown status "nope"`,
		`monster "rat" references unknown move "ghost_move"`,
		`item "charm" (slot "ring") grants moves, but only weapons may`,
		`section "cellar" allows item tier 9`,
		`section "depths" allows item tier 7`,
		`floor 1 (section "cellar") has difficulty budget 0`,
	}

	lines := strings.Split(err.Error(), "\n")
	if len(lines) != len(want) {
		t.Fatalf("the report has %d lines, want %d:\n%s", len(lines), len(want), err.Error())
	}
	for i, wantLine := range want {
		if !strings.Contains(lines[i], wantLine) {
			t.Errorf("report line %d = %q, want it to contain %q", i, lines[i], wantLine)
		}
	}

	// The order is stable: Validate only reads, so a second pass over
	// the same registry produces the same report.
	again := r.Validate(10)
	if again == nil || again.Error() != err.Error() {
		t.Errorf("Validate is not deterministic:\nfirst:  %v\nsecond: %v", err, again)
	}
}

func TestValidateRejectsEquipmentThatCanZeroHeroMaxHP(t *testing.T) {
	r := makeReg(validMonsters(), validSections())
	r.Moves["basic_attack"] = MoveAsset{
		ID: "basic_attack", Name: "Basic Attack", Type: "both", Tier: 1,
		Effects: []MoveEffectAsset{{Type: "damage", Target: "enemy", DmgModifier: func() *int { v := 100; return &v }()}}, Weight: 100,
	}
	for i, slot := range []string{"weapon", "helmet", "body_armor", "boots", "ring", "amulet"} {
		id := "penalty_" + slot
		r.Items[id] = ItemAsset{ID: id, Name: id, Slot: slot, Tier: 1, BonusHP: -17}
		if i == 0 {
			// A legitimate negative non-HP bonus remains accepted.
			r.Items[id] = ItemAsset{ID: id, Name: id, Slot: slot, Tier: 1, BonusHP: -16, BonusSPD: -10, Moves: []string{"basic_attack"}}
		}
	}

	err := r.Validate(10)
	if err == nil || !strings.Contains(err.Error(), "could reduce the hero's maximum HP to 0") {
		t.Fatalf("Validate(lethal equipment combination) = %v, want unsafe-max-HP error", err)
	}

	// The same combination leaving exactly one HP is valid.
	r.Items["penalty_amulet"] = ItemAsset{ID: "penalty_amulet", Name: "penalty_amulet", Slot: "amulet", Tier: 1, BonusHP: -15}
	if err := r.Validate(10); err != nil {
		t.Errorf("Validate(equipment combination leaving 1 max HP) = %v, want nil", err)
	}
}
