package game

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Arquivos .json na pasta `assets/` serão compilados juntamente do executável da aplicação.
//
//go:embed assets/*.json
var embeddedAssets embed.FS

// weaponSlot is the only equipment slot allowed to grant Combat 2.0
// moves (the entities package owns the typed slot constants).
const weaponSlot = "weapon"

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

	// Moves lists the ids of the Combat 2.0 moves this item grants while
	// equipped. Only weapons may grant moves (validated), at most
	// MaxWeaponMoves per weapon, and every id must exist.
	Moves []string `json:"moves"`
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

	// Moves lists the monster's Combat 2.0 moves with their AI weights.
	// Absent/empty falls back to the canonical basic_attack, so older
	// assets/mods keep working. Validated: known ids, at most MaxMoves
	// entries, weights >= 1.
	Moves []MoveRef `json:"moves"`
}

// Combat 2.0 move-set caps, enforced at load time (content bugs fail at
// startup) and shared with the entities package (the runtime side).
const (
	// MaxMoves is the cap on every combatant's move set.
	MaxMoves = 4
	// MaxWeaponMoves is how many moves a single weapon may grant.
	MaxWeaponMoves = 2
)

// MoveRef references a move in a monster's move list. The weight is the
// MONSTER's own AI weight for that move — it overrides the move asset's
// default weight when the engine resolves the set.
type MoveRef struct {
	ID     string `json:"id"`
	Weight int    `json:"weight"`
}

// MoveStatusAsset is the optional status a move applies on hit.
type MoveStatusAsset struct {
	// ID references a status asset (unknown ids are a content bug).
	ID string `json:"id"`
	// Chance is the application chance, in percent ([0, 100]).
	Chance int `json:"chance"`
	// Target receives the status: "enemy" (the hit target) or "self"
	// (the striker).
	Target string `json:"target"`
}

// MoveAsset defines a Combat 2.0 move (an entry of moves.json). Only the
// tier has an absent-field default (0 → 1, mirroring items): every other
// field is required as listed and validated at startup.
type MoveAsset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Type says which consumers may carry the move: "weapon" (only a
	// weapon's grant list), "creature" (only a creature's kit — monster
	// sets and the hero's learned set) or "both". Required: there is no
	// absent-field default, and an unknown word fails validation.
	Type string `json:"type"`
	// Tier places the move on the progression ladder (1 = earliest).
	Tier int `json:"tier"`
	// DmgModifier scales the striker's DMG, in percent (100 = neutral).
	DmgModifier int `json:"dmg_modifier"`
	// CritBonus adds percentage points to the striker's crit threshold
	// for this move (may be negative; clamped to [0, 100] at use time).
	CritBonus int `json:"crit_bonus"`
	// Cooldown is how many turns the move stays locked after use
	// (0 = spammable).
	Cooldown int `json:"cooldown"`
	// Status is the optional status the move applies on hit.
	Status *MoveStatusAsset `json:"status,omitempty"`
	// Weight is the move's default AI weight (monster picks); a monster's
	// move refs carry their own overriding weight.
	Weight int `json:"weight"`
}

// StatusEffectAsset is what a status does on each of its turns: either a
// DoT (damage per turn) or a stat modifier.
type StatusEffectAsset struct {
	// Type is "dot" or "stat".
	Type string `json:"type"`
	// Stat names the modified stat ("dmg", "def" or "spd") — for type
	// "stat" only.
	Stat string `json:"stat"`
	// Amount is the DoT damage per turn (>= 1) or the stat modifier (!= 0)
	// — matching the type.
	Amount int `json:"amount"`
}

// StatusAsset defines a Combat 2.0 status effect (an entry of
// statuses.json). Every field is required — statuses have no
// absent-field defaults.
type StatusAsset struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Kind     string            `json:"kind"` // "buff" or "debuff"
	Effect   StatusEffectAsset `json:"effect"`
	Duration int               `json:"duration"`
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
	// Merchant weights the wandering-shop encounter (StateMerchant).
	// Absent/zero keeps every existing rng sequence identical: the
	// merchant bucket only exists when the weight is positive.
	Merchant int `json:"merchant"`
	// Shrine weights the move-teaching encounter (StateLearn's forget
	// prompt). APPENDED LAST in the roll order, mirroring the merchant:
	// absent/zero keeps every pre-shrine rng sequence identical.
	Shrine int `json:"shrine"`
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
	// MoveTiers restricts which move tiers this section can offer (the
	// shrine draws, phase 4). Same contract as item_tiers: absent/empty
	// means NO restriction.
	MoveTiers []int `json:"move_tiers"`
	// Boss is the section's boss challenge (nil = no boss gate): fought
	// on the section's FINAL floor (see Registry.IsSectionEnd) after a
	// normal-combat victory there unlocks the challenge. A section
	// without the block keeps the old final-floor behavior (normal
	// victory → ascend), so older sections/mods keep working.
	Boss *BossAsset `json:"boss"`
}

// BossAsset defines a section's boss challenge: a PREDEFINED monster
// group (fought in the authored order — no spawn weights, no difficulty
// budget) and the gold bonus paid when the boss falls. The group
// references existing monster ids (validated at load time).
type BossAsset struct {
	// Monsters are the boss group's monster ids, engaged in this order.
	Monsters []string `json:"monsters"`
	// Gold is the bonus added to the hero's purse on the boss's defeat
	// (absent/0 = no bonus).
	Gold int `json:"gold"`
}

// Registro de todos os assets carregados em memória.
//
// Estes assets são acessados para spawnar as instâncias no jogo.
type Registry struct {
	Monsters map[string]MonsterAsset
	Items    map[string]ItemAsset
	Sections []SectionAsset
	// Combat 2.0 content: moves and the statuses they apply.
	Moves    map[string]MoveAsset
	Statuses map[string]StatusAsset

	// provenance records where each asset file came from, keyed by file
	// name (see Provenance). Filled by Load; a hand-built Registry has
	// none.
	provenance map[string]string
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

// IsSectionEnd reports whether the floor is the FINAL floor of the
// section covering it — the floor where a section's boss gates the
// ascension. Resolution is first-match (SectionForFloor): a floor two
// sections claim belongs to the EARLIER one, and its end is the
// earlier's (the embedded content's floor 3 is the goblin pits' end, not
// the outskirts' first floor — the documented behavior). An infinite
// tail section (max_floor == -1) has no final floor, and a floor no
// section covers is not a section end either.
func (r *Registry) IsSectionEnd(floor int) bool {
	s, err := r.SectionForFloor(floor)
	if err != nil {
		return false
	}
	return s.MaxFloor != -1 && s.MaxFloor == floor
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

// MaxItemTier reports the highest item tier in the registry (0 when the
// registry holds no items). The merchant's tier+1 upgrade caps at it.
func (r *Registry) MaxItemTier() int {
	maxTier := 0
	for _, asset := range r.Items {
		maxTier = max(maxTier, asset.Tier)
	}
	return maxTier
}

// MoveByID looks up a move asset by id.
func (r *Registry) MoveByID(id string) (MoveAsset, bool) {
	m, ok := r.Moves[id]
	return m, ok
}

// StatusByID looks up a status asset by id.
func (r *Registry) StatusByID(id string) (StatusAsset, bool) {
	s, ok := r.Statuses[id]
	return s, ok
}

// AllStatuses returns every registry status, sorted by id — the
// read-only surface for lore/UI consumers the engine may expose later
// (e.g. a status " ? " overlay). The result is a fresh slice: the
// assets are value copies, so mutating the return never reaches the
// registry. An empty registry returns an empty slice.
func (r *Registry) AllStatuses() []StatusAsset {
	out := make([]StatusAsset, 0, len(r.Statuses))
	for _, status := range r.Statuses {
		out = append(out, status)
	}
	slices.SortFunc(out, func(a, b StatusAsset) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// moveUsableByWeapons reports whether a move of this asset type may
// appear on a weapon's grant list ("weapon" or "both" — a creature-only
// move is learned or monster-fought, never granted).
func moveUsableByWeapons(moveType string) bool {
	return moveType == "weapon" || moveType == "both"
}

// moveUsableByCreatures reports whether a move of this asset type may
// appear on a creature's move kit — a monster's set or the hero's
// learned set ("creature" or "both"; a weapon-only move reaches the hero
// only through the equipped weapon).
func moveUsableByCreatures(moveType string) bool {
	return moveType == "creature" || moveType == "both"
}

// MovesForTiers returns every registry move a HERO's shrine may teach —
// creature- or both-typed (a weapon-only move reaches the hero through
// the equipped weapon, never a shrine) — whose tier is one of `tiers`,
// sorted by id (assets, not just ids — moves carry their own display data
// and AI weight). Empty/nil `tiers` lifts the tier restriction: every
// shrine-offerable move is returned. A pool with no matches comes back
// empty — never an error; the caller decides what an empty pool means.
// The engine's shrine flow draws its offers from exactly this query, so
// the type rule lives HERE and no call site can forget it (the section
// move_tiers fillability check consumes it too: a listed tier must hold
// at least one offerable move). Mirrors ItemIDsForTiers.
func (r *Registry) MovesForTiers(tiers []int) []MoveAsset {
	restricted := len(tiers) > 0
	allowed := make(map[int]bool, len(tiers))
	for _, tier := range tiers {
		allowed[tier] = true
	}
	moves := make([]MoveAsset, 0, len(r.Moves))
	for _, move := range r.Moves {
		if !moveUsableByCreatures(move.Type) {
			continue // weapon-only: never offerable at a shrine
		}
		if !restricted || allowed[move.Tier] {
			moves = append(moves, move)
		}
	}
	slices.SortFunc(moves, func(a, b MoveAsset) int { return strings.Compare(a.ID, b.ID) })
	return moves
}

// MaxMoveTier reports the highest move tier in the registry (0 when the
// registry holds no moves).
func (r *Registry) MaxMoveTier() int {
	maxTier := 0
	for _, move := range r.Moves {
		maxTier = max(maxTier, move.Tier)
	}
	return maxTier
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
	monsters, monstersSrc, err := loadJSON[MonsterAsset](externalDir, "monsters.json")
	if err != nil {
		return nil, fmt.Errorf("loading monsters: %w", err)
	}
	items, itemsSrc, err := loadJSON[ItemAsset](externalDir, "items.json")
	if err != nil {
		return nil, fmt.Errorf("loading items: %w", err)
	}
	sections, sectionsSrc, err := loadJSON[SectionAsset](externalDir, "sections.json")
	if err != nil {
		return nil, fmt.Errorf("loading sections: %w", err)
	}
	moves, movesSrc, err := loadJSON[MoveAsset](externalDir, "moves.json")
	if err != nil {
		return nil, fmt.Errorf("loading moves: %w", err)
	}
	statuses, statusesSrc, err := loadJSON[StatusAsset](externalDir, "statuses.json")
	if err != nil {
		return nil, fmt.Errorf("loading statuses: %w", err)
	}

	monstersMap, err := indexBy(monsters, func(m MonsterAsset) string { return m.ID })
	if err != nil {
		return nil, fmt.Errorf("loading monsters: %w", err)
	}
	itemsMap, err := indexBy(items, func(i ItemAsset) string { return i.ID })
	if err != nil {
		return nil, fmt.Errorf("loading items: %w", err)
	}
	movesMap, err := indexBy(moves, func(m MoveAsset) string { return m.ID })
	if err != nil {
		return nil, fmt.Errorf("loading moves: %w", err)
	}
	statusesMap, err := indexBy(statuses, func(s StatusAsset) string { return s.ID })
	if err != nil {
		return nil, fmt.Errorf("loading statuses: %w", err)
	}
	defaultAbsentItemTiers(itemsMap)
	defaultAbsentMoveTiers(movesMap)

	r := &Registry{
		Monsters: monstersMap,
		Items:    itemsMap,
		Sections: sections,
		Moves:    movesMap,
		Statuses: statusesMap,
		provenance: map[string]string{
			"monsters.json": monstersSrc,
			"items.json":    itemsSrc,
			"moves.json":    movesSrc,
			"statuses.json": statusesSrc,
			"sections.json": sectionsSrc,
		},
	}
	return r, nil

}

// Provenance reports where each asset file was loaded from, keyed by
// file name ("monsters.json", "items.json", "moves.json",
// "statuses.json", "sections.json"). The value is "embedded" when the
// compiled-in assets were used, or "external (<dir>)" when the mod
// directory's file won the per-file fallback. Diagnostics surface —
// e.g. the validate command's digest prints one line per file.
//
// The returned map is a copy: mutating it never reaches the registry.
// A Registry not built by Load (hand-assembled in tests) has an empty
// provenance.
func (r *Registry) Provenance() map[string]string {
	out := make(map[string]string, len(r.provenance))
	for file, source := range r.provenance {
		out[file] = source
	}
	return out
}

// Validate checks that the loaded content is internally consistent and
// COLLECTS every fault it finds instead of stopping at the first one:
// asset schema rules (tiers, damage modifiers, cooldowns, status
// effects, weapon slot/cap rules, the spammable rule), reference rules
// (unknown monster/move/status ids), duplicate ids (caught by Load),
// section rules (spawn tables, weights, tier fillability, merchant
// stockability, boss blocks) and per-floor coverage up to topFloor.
//
// It returns nil when the content is valid, or a single error whose
// message lists every fault on its own line (errors.Join semantics) —
// callers should print the report whole, not just its first line.
// The report is deterministic: assets are walked in id order, sections
// in slice order, floors ascending.
//
// Call this once right after Load(); if it returns an error, that's a
// content bug (bad JSON), not something a player can hit by playing —
// so the caller should treat it as fatal and refuse to start.
func (r *Registry) Validate(topFloor int) error {
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("content validation: "+format, args...))
	}

	// Item tiers must be real. Load defaults the absent field to 1, so a
	// non-positive tier here is a content bug, not a missing field.
	for _, id := range sortedIDs(r.Items) {
		item := r.Items[id]
		if item.Tier < 1 {
			fail("item %q has tier %d, want >= 1 (absent/0 defaults to tier 1)",
				item.ID, item.Tier)
		}
	}

	// Combat 2.0: statuses are checked before moves (moves reference
	// them), and moves before monsters/weapons (those reference moves).
	for _, id := range sortedIDs(r.Statuses) {
		st := r.Statuses[id]
		if st.Kind != "buff" && st.Kind != "debuff" {
			fail("status %q has kind %q, want \"buff\" or \"debuff\"", st.ID, st.Kind)
		}
		if st.Duration < 1 {
			fail("status %q has duration %d, want >= 1", st.ID, st.Duration)
		}
		switch st.Effect.Type {
		case "dot":
			if st.Effect.Amount < 1 {
				fail("status %q has dot amount %d, want >= 1", st.ID, st.Effect.Amount)
			}
		case "stat":
			if !slices.Contains([]string{"dmg", "def", "spd"}, st.Effect.Stat) {
				fail("status %q modifies unknown stat %q, want one of dmg, def, spd",
					st.ID, st.Effect.Stat)
			}
			if st.Effect.Amount == 0 {
				fail("status %q has a zero stat amount, want a non-zero modifier", st.ID)
			}
		default:
			fail("status %q has effect type %q, want \"dot\" or \"stat\"", st.ID, st.Effect.Type)
		}
	}

	for _, id := range sortedIDs(r.Moves) {
		mv := r.Moves[id]
		// Load defaults the absent tier to 1, so a non-positive tier here
		// is a content bug, not a missing field.
		if mv.Tier < 1 {
			fail("move %q has tier %d, want >= 1 (absent/0 defaults to tier 1)",
				mv.ID, mv.Tier)
		}
		if mv.DmgModifier < 50 || mv.DmgModifier > 300 {
			fail("move %q has dmg_modifier %d, want within [50, 300] (percent, 100 = neutral)",
				mv.ID, mv.DmgModifier)
		}
		if mv.Cooldown < 0 {
			fail("move %q has cooldown %d, want >= 0 (0 = spammable)", mv.ID, mv.Cooldown)
		}
		// The type word is required (no absent-field default): it gates
		// which consumer may carry the move.
		switch mv.Type {
		case "weapon", "creature", "both":
		default:
			fail("move %q has type %q, want \"weapon\", \"creature\" or \"both\"", mv.ID, mv.Type)
		}
		if mv.Weight < 1 {
			fail("move %q has weight %d, want >= 1 (AI pick weight)", mv.ID, mv.Weight)
		}
		if mv.Status == nil {
			continue
		}
		if mv.Status.Chance < 0 || mv.Status.Chance > 100 {
			fail("move %q has status chance %d, want within [0, 100] (percentage)",
				mv.ID, mv.Status.Chance)
		}
		if mv.Status.Target != "enemy" && mv.Status.Target != "self" {
			fail("move %q has status target %q, want \"enemy\" or \"self\"", mv.ID, mv.Status.Target)
		}
		if _, ok := r.StatusByID(mv.Status.ID); !ok {
			fail("move %q references unknown status %q", mv.ID, mv.Status.ID)
		}
	}

	// Move references on monsters are checked registry-wide (a monster
	// absent from every spawn table still cannot carry broken moves).
	for _, id := range sortedIDs(r.Monsters) {
		m := r.Monsters[id]
		if len(m.Moves) > MaxMoves {
			// An over-cap move list is one fault: its entries are not
			// re-checked (the cap is the bug a modder fixes first).
			fail("monster %q has %d moves, want at most %d (MAX_MOVES)",
				m.ID, len(m.Moves), MaxMoves)
			continue
		}
		for _, ref := range m.Moves {
			if ref.Weight < 1 {
				fail("monster %q move %q has weight %d, want >= 1 (AI pick weight)",
					m.ID, ref.ID, ref.Weight)
			}
			move, ok := r.MoveByID(ref.ID)
			if !ok {
				fail("monster %q references unknown move %q", m.ID, ref.ID)
				continue // the type rule needs the move resolved
			}
			// A monster's kit is a creature channel: weapon-only moves
			// (hero weapon grants) can never ride it.
			if !moveUsableByCreatures(move.Type) {
				fail("monster %q references move %q of type %q, want a creature or both move (weapon-only moves are never monster moves)",
					m.ID, ref.ID, move.Type)
			}
		}
	}

	// Only weapons may grant moves, at most MaxWeaponMoves each, and
	// every granted id must exist.
	for _, id := range sortedIDs(r.Items) {
		item := r.Items[id]
		if len(item.Moves) == 0 {
			continue
		}
		if item.Slot != weaponSlot {
			fail("item %q (slot %q) grants moves, but only weapons may", item.ID, item.Slot)
			continue // the weapon-only rules below make no sense for other slots
		}
		if len(item.Moves) > MaxWeaponMoves {
			// An over-cap grant list is one fault: its entries are not
			// re-checked (the cap is the bug a modder fixes first).
			fail("weapon %q grants %d moves, want at most %d",
				item.ID, len(item.Moves), MaxWeaponMoves)
			continue
		}
		unknownRef := false
		for _, moveID := range item.Moves {
			move, ok := r.MoveByID(moveID)
			if !ok {
				fail("weapon %q references unknown move %q", item.ID, moveID)
				unknownRef = true
				continue
			}
			// A weapon's grant list is the weapon channel: creature-only
			// moves (monster kits / hero learning) can never ride it.
			if !moveUsableByWeapons(move.Type) {
				fail("weapon %q grants move %q of type %q, want a weapon or both move (creature-only moves are never weapon-grantable)",
					item.ID, moveID, move.Type)
			}
		}
		if unknownRef {
			// The spammable rule needs every grant resolved; the unknown
			// refs are already reported above.
			continue
		}
		// The spammable rule: granting moves REPLACES the canonical
		// basic_attack fallback, so the grant list must carry at least
		// one cooldown-0 move — a weapon whose every move is locked can
		// never act. A weapon with NO moves list keeps the fallback
		// (spammable by definition) and skips this rule.
		hasSpammable := false
		for _, moveID := range item.Moves {
			if move, _ := r.MoveByID(moveID); move.Cooldown == 0 {
				hasSpammable = true
			}
		}
		if !hasSpammable {
			fail("weapon %q grants moves, but none has cooldown 0 — "+
				"want at least one spammable (the grants replace the basic_attack fallback)",
				item.ID)
		}
	}

	// Monster schema rules (difficulty/defense/crit) are checked through
	// the spawn-table walk below — the same walk the encounter generator
	// uses — at most once per monster id: the same monster referenced by
	// many sections is one fault, not one per spawn entry.
	schemaChecked := make(map[string]bool)

	for _, s := range r.Sections {
		if len(s.Spawns) == 0 {
			fail("section %q has no spawn table", s.ID)
			continue // the spawn checks below need a table
		}
		// Negativos distorceriam o sorteio do tipo de encontro. Pesos zerados
		// (ou o campo ausente) são válidos: significam "apenas combate".
		if s.EncounterTypes.Combat < 0 || s.EncounterTypes.Sanctuary < 0 ||
			s.EncounterTypes.Chest < 0 || s.EncounterTypes.Merchant < 0 ||
			s.EncounterTypes.Shrine < 0 {
			fail("section %q has a negative encounter type weight, want >= 0", s.ID)
		}
		// item_tiers: every listed value must be a real tier, and each
		// listed tier must have at least one item in the registry — a
		// listed tier with no items is a typo.
		listed := make(map[int]bool, len(s.ItemTiers))
		for _, tier := range s.ItemTiers {
			if tier < 1 {
				fail("section %q has item_tiers value %d, want >= 1", s.ID, tier)
				continue
			}
			listed[tier] = true
		}
		for _, tier := range slices.Sorted(maps.Keys(listed)) {
			if len(r.ItemIDsForTiers([]int{tier})) == 0 {
				fail("section %q allows item tier %d, but the registry has no items of that tier",
					s.ID, tier)
			}
		}
		// move_tiers: the same contract as item_tiers — every listed
		// value must be a real tier with at least one registry move
		// (typo protection). Absent/empty = unrestricted.
		listedMoves := make(map[int]bool, len(s.MoveTiers))
		for _, tier := range s.MoveTiers {
			if tier < 1 {
				fail("section %q has move_tiers value %d, want >= 1", s.ID, tier)
				continue
			}
			listedMoves[tier] = true
		}
		for _, tier := range slices.Sorted(maps.Keys(listedMoves)) {
			if len(r.MovesForTiers([]int{tier})) == 0 {
				fail("section %q allows move tier %d, but the registry has no moves of that tier",
					s.ID, tier)
			}
		}
		// A merchant section must be able to stock its shop: at least one
		// registry item in the allowed tiers (unrestricted when no
		// item_tiers). Tier+1 upgrades only REPLACE offers, so they can
		// never stock an otherwise-empty shop.
		if s.EncounterTypes.Merchant > 0 && len(r.ItemIDsForTiers(s.ItemTiers)) == 0 {
			fail("section %q has a merchant weight but no items to stock it (allowed tiers: %v)",
				s.ID, s.ItemTiers)
		}
		// The boss block (absent = no gate): the group cannot be empty,
		// every listed monster must exist (typo protection), and the gold
		// bonus cannot be negative.
		if s.Boss != nil {
			if len(s.Boss.Monsters) == 0 {
				fail("section %q has a boss block with an empty monster list", s.ID)
			}
			for _, id := range s.Boss.Monsters {
				if _, ok := r.Monsters[id]; !ok {
					fail("section %q boss references unknown monster %q", s.ID, id)
				}
			}
			if s.Boss.Gold < 0 {
				fail("section %q boss has gold %d, want >= 0 (absent/0 = no bonus)",
					s.ID, s.Boss.Gold)
			}
		}
		for _, spawn := range s.Spawns {
			m, ok := r.Monsters[spawn.MonsterID]
			if !ok {
				fail("section %q references unknown monster %q", s.ID, spawn.MonsterID)
				continue
			}
			if spawn.Weight < 1 {
				fail("section %q spawn for monster %q has weight %d, want >= 1",
					s.ID, spawn.MonsterID, spawn.Weight)
			}
			if schemaChecked[spawn.MonsterID] {
				continue
			}
			schemaChecked[spawn.MonsterID] = true
			if m.Difficulty < 1 {
				fail("monster %q has difficulty %d, want >= 1 "+
					"(garante que geração de encontros termina)",
					m.ID, m.Difficulty)
			}
			if m.Defense < 0 {
				fail("monster %q has def %d, want >= 0 "+
					"(negative defense would amplify incoming damage)",
					m.ID, m.Defense)
			}
			if m.Crit < 0 || m.Crit > 100 {
				fail("monster %q has crit %d, want within [0, 100] (percentage)",
					m.ID, m.Crit)
			}
		}
	}

	// Floor coverage: every floor up to topFloor must be covered by a
	// section whose spawn table fits its difficulty budget. The first
	// uncovered floor ends the walk — a missing section fails every
	// later floor identically, so the report already says where the
	// coverage ends. A budget failure is reported once per section: the
	// budget is linear in the floor, so a section's failures share one
	// tuning bug (its first failing floor points at it).
	budgetReported := make(map[string]bool)
	for floor := 1; floor <= topFloor; floor++ {
		section, err := r.SectionForFloor(floor)
		if err != nil {
			errs = append(errs, fmt.Errorf("content validation: %w", err))
			break
		}

		cheapest, err := r.cheapestSpawn(section)
		if err != nil {
			// An unloadable spawn table is already reported per section
			// above; there is nothing new to say about this floor.
			break
		}
		budget := section.FloorDifficulty(floor)
		if budget < cheapest && !budgetReported[section.ID] {
			budgetReported[section.ID] = true
			fail("floor %d (section %q) has difficulty budget %d, "+
				"but the cheapest monster it can spawn (%s) costs %d — this floor would always be empty",
				floor, section.ID, budget, r.cheapestSpawnName(section), cheapest)
		}
	}

	return errors.Join(errs...)
}

// sortedIDs returns the map's ids in sorted order: validation walks the
// assets by id so the collected report is deterministic run to run.
func sortedIDs[T any](m map[string]T) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
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
//
// Returns the assets and WHERE they came from: "embedded", or
// "external (<dir>)" when the mod directory's file won the per-file
// fallback.
func loadJSON[T any](externalDir, filename string) ([]T, string, error) {
	var (
		raw    []byte
		source = "embedded"
	)

	if externalDir != "" {
		path := filepath.Join(externalDir, filename)
		r, err := os.ReadFile(path)
		switch {
		case err == nil:
			raw = r
			source = fmt.Sprintf("external (%s)", externalDir)
		case errors.Is(err, os.ErrNotExist):
			// arquivo externo ausente: cai para os assets embutidos
		default:
			return nil, "", fmt.Errorf("reading external asset %s: %w", path, err)
		}
	}
	if raw == nil {
		r, err := embeddedAssets.ReadFile("assets/" + filename)
		if err != nil {
			return nil, "", fmt.Errorf("reading embedded asset %s: %w", filename, err)
		}
		raw = r
	}

	var items []T
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, "", fmt.Errorf("parsing %s: %w", filename, err)
	}
	return items, source, nil
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

// defaultAbsentMoveTiers applies the move tier default in place (same
// rule as items: JSON cannot tell "tier": 0 from an absent field, and
// both mean tier 1). Every other move field has no default — it is
// required as authored and validated at startup.
func defaultAbsentMoveTiers(moves map[string]MoveAsset) {
	for id, move := range moves {
		if move.Tier == 0 {
			move.Tier = 1
			moves[id] = move
		}
	}
}
