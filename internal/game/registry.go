package game

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// Arquivos .json na pasta `assets/` serão compilados juntamente do executável da aplicação.
//
//go:embed assets/*.json
var embeddedAssets embed.FS

// Define um asset para item.
//
// Usado para spawnar uma instância de `Item` no jogo.
type ItemAsset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slot string `json:"slot"`
	// Tier places the item on the progression ladder (1 = earliest gear).
	// An absent field means tier 1, so older assets/mods keep working.
	Tier int `json:"tier"`

	BonusHP    int `json:"bonus_hp"`
	BonusDMG   int `json:"bonus_dmg"`
	BonusDEF   int `json:"bonus_def"`
	BonusSPD   int `json:"bonus_spd"`
	Durability int `json:"durability"`
}

// Definição de um asset para monstro.
//
// Usado para spawnar uma instância de `Monster` no jogo.
type MonsterAsset struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Difficulty int    `json:"difficulty"`
	HP         int    `json:"hp"`
	Damage     int    `json:"dmg"`
	Speed      int    `json:"spd"`
	Defense    int    `json:"def"`
	// Critical hit chance as a percentage; 0 means the monster never crits.
	Crit int `json:"crit"`
}

// Define o mostro e seu peso para spawnar em uma seção.
type SpawnEntry struct {
	MonsterID string `json:"monster_id"`
	Weight    int    `json:"weight"`
}

// Pesos por tipo de encontro da seção: o engine sorteia o TIPO do
// encontro antes de preencher o orçamento de monstros.
//
// Uma tabela zerada (ou o campo ausente no JSON) significa que a seção
// sorteia apenas encontros de combate — seções/mods antigos continuam válidos.
type EncounterTypeWeights struct {
	Combat    int `json:"combat"`
	Sanctuary int `json:"sanctuary"`
	Chest     int `json:"chest"`
}

// Define um asset para uma seção da torre.
// Usado para gerar um `Floor`.
//
// `SectionAsset` com `MaxFloor == -1` não tem um limite máximo de andar.
type SectionAsset struct {
	ID                 string               `json:"id"`
	Name               string               `json:"name"`
	BaseDifficulty     int                  `json:"base_difficulty"`
	DifficultyPerFloor int                  `json:"difficulty_per_floor"`
	MinFloor           int                  `json:"min_floor"`
	MaxFloor           int                  `json:"max_floor"`
	Spawns             []SpawnEntry         `json:"spawns"`
	EncounterTypes     EncounterTypeWeights `json:"encounter_types"`
	// ItemTiers restricts which item tiers this section can roll as
	// rewards or merchant stock. Absent/empty means NO restriction, so
	// older sections/mods keep rolling the whole registry.
	ItemTiers []int `json:"item_tiers"`
}

// Registro de todos os assets carregados em memória.
//
// Estes assets são acessados para spawnar as instâncias no jogo.
type Registry struct {
	Monsters map[string]MonsterAsset
	Items    map[string]ItemAsset
	Sections []SectionAsset
}

// Busca a seção à qual este andar pertence.
func (r *Registry) SectionForFloor(floor int) (SectionAsset, error) {
	for _, s := range r.Sections {
		if floor >= s.MinFloor && (s.MaxFloor == -1 || floor <= s.MaxFloor) {
			return s, nil
		}
	}
	return SectionAsset{}, fmt.Errorf("no section found for floor %d", floor)
}

// ItemIDsForTiers returns the sorted ids of every registry item whose tier
// is one of `tiers`. Empty/nil `tiers` lifts the restriction: every item
// id is returned. A pool with no matches comes back empty — never an
// error; the caller decides what an empty pool means (empty chest,
// unstockable shop, ...).
func (r *Registry) ItemIDsForTiers(tiers []int) []string {
	restricted := len(tiers) > 0
	allowed := make(map[int]bool, len(tiers))
	for _, tier := range tiers {
		allowed[tier] = true
	}
	ids := make([]string, 0, len(r.Items))
	for id, asset := range r.Items {
		if !restricted || allowed[asset.Tier] {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// Calcula a dificuldade para o andar com base na seção atual.
func (s SectionAsset) FloorDifficulty(floor int) int {
	// O quão avançado se está dentro desta seção.
	floorIntoSection := floor - s.MinFloor
	return s.BaseDifficulty + s.DifficultyPerFloor*floorIntoSection
}

// Carrega para memória os assets do jogo.
//
// Se `externalDir` estiver definido e contem arquivos correspondetes, ele terá precedência
// sobre os valores pré-compilados do jogo.
func Load(externalDir string) (*Registry, error) {
	monsters, err := loadJSON[MonsterAsset](externalDir, "monsters.json")
	if err != nil {
		return nil, fmt.Errorf("loading monsters: %w", err)
	}
	items, err := loadJSON[ItemAsset](externalDir, "items.json")
	if err != nil {
		return nil, fmt.Errorf("loading items: %w", err)
	}
	sections, err := loadJSON[SectionAsset](externalDir, "sections.json")
	if err != nil {
		return nil, fmt.Errorf("loading sections: %w", err)
	}

	monstersMap, err := indexBy(monsters, func(m MonsterAsset) string { return m.ID })
	if err != nil {
		return nil, fmt.Errorf("loading monsters: %w", err)
	}
	itemsMap, err := indexBy(items, func(i ItemAsset) string { return i.ID })
	if err != nil {
		return nil, fmt.Errorf("loading items: %w", err)
	}
	defaultAbsentItemTiers(itemsMap)

	r := &Registry{
		Monsters: monstersMap,
		Items:    itemsMap,
		Sections: sections,
	}
	return r, nil

}

// Validate checks that the loaded content is internally consistent —
// every floor up to topFloor is covered by some section, and every
// section's spawn table only references monsters that actually exist.
// Call this once right after Load(); if it returns an error, that's a
// content bug (bad JSON), not something a player can hit by playing —
// so the caller should treat it as fatal and refuse to start.
func (r *Registry) Validate(topFloor int) error {
	// Item tiers must be real. Load defaults the absent field to 1, so a
	// non-positive tier here is a content bug, not a missing field.
	for _, item := range r.Items {
		if item.Tier < 1 {
			return fmt.Errorf(
				"content validation: item %q has tier %d, want >= 1 (absent/0 defaults to tier 1)",
				item.ID, item.Tier,
			)
		}
	}

	for _, s := range r.Sections {
		if len(s.Spawns) == 0 {
			return fmt.Errorf("content validation: section %q has no spawn table", s.ID)
		}
		// Negativos distorceriam o sorteio do tipo de encontro. Pesos zerados
		// (ou o campo ausente) são válidos: significam "apenas combate".
		if s.EncounterTypes.Combat < 0 || s.EncounterTypes.Sanctuary < 0 || s.EncounterTypes.Chest < 0 {
			return fmt.Errorf(
				"content validation: section %q has a negative encounter type weight, want >= 0",
				s.ID,
			)
		}
		// item_tiers: every listed value must be a real tier, and each
		// listed tier must have at least one item in the registry — a
		// listed tier with no items is a typo.
		listed := make(map[int]bool, len(s.ItemTiers))
		for _, tier := range s.ItemTiers {
			if tier < 1 {
				return fmt.Errorf(
					"content validation: section %q has item_tiers value %d, want >= 1",
					s.ID, tier,
				)
			}
			listed[tier] = true
		}
		for tier := range listed {
			if len(r.ItemIDsForTiers([]int{tier})) == 0 {
				return fmt.Errorf(
					"content validation: section %q allows item tier %d, but the registry has no items of that tier",
					s.ID, tier,
				)
			}
		}
		for _, spawn := range s.Spawns {
			m, ok := r.Monsters[spawn.MonsterID]
			if !ok {
				return fmt.Errorf("content validation: section %q references unknown monster %q", s.ID, spawn.MonsterID)
			}
			if spawn.Weight < 1 {
				return fmt.Errorf(
					"content validation: section %q spawn for monster %q has weight %d, want >= 1",
					s.ID, spawn.MonsterID, spawn.Weight,
				)
			}
			if m.Difficulty < 1 {
				return fmt.Errorf(
					"content validation: monster %q has difficulty %d, want >= 1 "+
						"(garante que geração de encontros termina)",
					m.ID, m.Difficulty,
				)
			}
			if m.Defense < 0 {
				return fmt.Errorf(
					"content validation: monster %q has def %d, want >= 0 "+
						"(negative defense would amplify incoming damage)",
					m.ID, m.Defense,
				)
			}
			if m.Crit < 0 || m.Crit > 100 {
				return fmt.Errorf(
					"content validation: monster %q has crit %d, want within [0, 100] (percentage)",
					m.ID, m.Crit,
				)
			}
		}
	}

	for floor := 1; floor <= topFloor; floor++ {
		section, err := r.SectionForFloor(floor)
		if err != nil {
			return fmt.Errorf("content validation: %w", err)
		}

		cheapest, err := r.cheapestSpawn(section)
		if err != nil {
			return fmt.Errorf("content validation: %w", err)
		}
		budget := section.FloorDifficulty(floor)
		if budget < cheapest {
			return fmt.Errorf(
				"content validation: floor %d (section %q) has difficulty budget %d, "+
					"but the cheapest monster it can spawn (%s) costs %d — this floor would always be empty",
				floor, section.ID, budget, r.cheapestSpawnName(section), cheapest,
			)
		}
	}
	return nil
}

// cheapestSpawn returns the lowest Difficulty among a section's spawn table.
func (r *Registry) cheapestSpawn(s SectionAsset) (int, error) {
	cheapest := -1
	for _, spawn := range s.Spawns {
		m, ok := r.Monsters[spawn.MonsterID]
		if !ok {
			return 0, fmt.Errorf("section %q references unknown monster %q", s.ID, spawn.MonsterID)
		}
		if cheapest == -1 || m.Difficulty < cheapest {
			cheapest = m.Difficulty
		}
	}
	return cheapest, nil
}

// cheapestSpawnName is a small helper purely for a more useful error message.
func (r *Registry) cheapestSpawnName(s SectionAsset) string {
	cheapest, name := -1, ""
	for _, spawn := range s.Spawns {
		m, ok := r.Monsters[spawn.MonsterID]
		if ok && (cheapest == -1 || m.Difficulty < cheapest) {
			cheapest, name = m.Difficulty, m.Name
		}
	}
	return name
}

// Lê o JSON com os assets do tipo T no local informado
// ou utiliza os assets pré-compilados caso não encontre.
//
// Um arquivo externo que não existe faz fallback para os assets embutidos;
// um arquivo externo que existe mas não pode ser lido é tratado como erro
// real (mods quebrados não devem falhar silenciosamente).
func loadJSON[T any](externalDir, filename string) ([]T, error) {
	var raw []byte

	if externalDir != "" {
		path := filepath.Join(externalDir, filename)
		r, err := os.ReadFile(path)
		switch {
		case err == nil:
			raw = r
		case errors.Is(err, os.ErrNotExist):
			// arquivo externo ausente: cai para os assets embutidos
		default:
			return nil, fmt.Errorf("reading external asset %s: %w", path, err)
		}
	}
	if raw == nil {
		r, err := embeddedAssets.ReadFile("assets/" + filename)
		if err != nil {
			return nil, fmt.Errorf("reading embedded asset %s: %w", filename, err)
		}
		raw = r
	}

	var items []T
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filename, err)
	}
	return items, nil
}

// Cria um map para o tipo T utilizando o campo retornado por `keyFn` como índice.
// Retorna erro se dois itens tiverem o mesmo id: duplicatas seriam
// silenciosamente escondidas pelo map, escondendo conteúdo.
func indexBy[T any](items []T, keyFn func(T) string) (map[string]T, error) {
	out := make(map[string]T, len(items))
	for _, item := range items {
		key := keyFn(item)
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("duplicate id %q", key)
		}
		out[key] = item
	}
	return out, nil
}

// defaultAbsentItemTiers applies the item tier default in place: JSON
// cannot tell an explicit "tier": 0 from an absent field, and both mean
// tier 1. After this, every registry item carries a real tier and
// Validate's "tier >= 1" rule only rejects genuine content bugs.
func defaultAbsentItemTiers(items map[string]ItemAsset) {
	for id, asset := range items {
		if asset.Tier == 0 {
			asset.Tier = 1
			items[id] = asset
		}
	}
}
