package game

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Arquivos .json na pasta `assets/` serão compilados juntamente do executável da aplicação.
//
//go:embed assets/*.json
var embeddedAssets embed.FS

// Define um asset para item.
//
// Usado para spawnar uma instância de `Item` no jogo.
type ItemAsset struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Slot       string `json:"slot"`
	BonusHP    int    `json:"bonus_hp"`
	BonusDMG   int    `json:"bonus_dmg"`
	BonusDEF   int    `json:"bonus_def"`
	BonusSPD   int    `json:"bonus_spd"`
	Durability int    `json:"durability"`
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
	ID                 string       `json:"id"`
	Name               string       `json:"name"`
	BaseDifficulty     int          `json:"base_difficulty"`
	DifficultyPerFloor int          `json:"difficulty_per_floor"`
	MinFloor           int                  `json:"min_floor"`
	MaxFloor           int                  `json:"max_floor"`
	Spawns             []SpawnEntry         `json:"spawns"`
	EncounterTypes     EncounterTypeWeights `json:"encounter_types"`
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
