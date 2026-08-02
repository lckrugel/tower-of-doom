package game

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
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
	Damage     int    `json:"bonus_dmg"`
	Speed      int    `json:"spd"`
	Defense    int    `json:"def"`
}

// Define o mostro e seu peso para spawnar em uma seção.
type SpawnEntry struct {
	MonsterID string `json:"monster_id"`
	Weight    int    `json:"weight"`
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
	MinFloor           int          `json:"min_floor"`
	MaxFloor           int          `json:"max_floor"`
	Spawns             []SpawnEntry `json:"spawns"`
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

	r := &Registry{
		Monsters: indexBy(monsters, func(m MonsterAsset) string { return m.ID }),
		Items:    indexBy(items, func(i ItemAsset) string { return i.ID }),
		Sections: sections,
	}
	return r, nil

}

// Lê o JSON com os assets do tipo T no local informado
// ou utiliza os assets pré-compilados caso não encontre.
func loadJSON[T any](externalDir, filename string) ([]T, error) {
	var raw []byte
	var err error

	if externalDir != "" {
		raw, err = os.ReadFile(externalDir + "/" + filename)
	}
	if externalDir == "" || err != nil {
		raw, err = embeddedAssets.ReadFile("assets/" + filename)
	}
	if err != nil {
		return nil, err
	}

	var items []T
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filename, err)
	}
	return items, nil
}

// Cria um map para o tipo T utilizando o campo retornado por `keyFn` como índice.
func indexBy[T any](items []T, keyFn func(T) string) map[string]T {
	out := make(map[string]T, len(items))
	for _, item := range items {
		out[keyFn(item)] = item
	}
	return out
}
