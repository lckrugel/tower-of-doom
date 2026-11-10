package entities

import "github.com/lckrugel/tower-of-doom/internal/game"

// MaxMoves is the cap on every combatant's move set (Combat 2.0). The
// number lives in the game package — the registry validates asset-level
// caps against it — and is re-exported here as the runtime-facing name.
const MaxMoves = game.MaxMoves

// MaxLearnedMoves is how many cooldown moves a hero may learn at shrines.
// The weapon/Unarmed basic action sits before them; together they fit
// within MaxMoves.
const MaxLearnedMoves = 3

// MoveTarget says who receives a move's status effect.
type MoveTarget string

const (
	TargetEnemy MoveTarget = "enemy"
	TargetSelf  MoveTarget = "self"
)

// MoveType says which consumers may carry a move: a weapon's grant
// list, a creature's move kit (a monster's set or the hero's learned
// set), or both. Mirrors the validated asset word (moves.json "type").
type MoveType string

const (
	// MoveTypeWeapon marks a weapon-only move: it reaches a hero through
	// the equipped weapon's grants, and a monster's kit can never carry
	// it (nor can it be learned — the shrine pool filters it out).
	MoveTypeWeapon MoveType = "weapon"
	// MoveTypeCreature marks a creature-only move: monsters fight with
	// it natively and heroes learn it at shrines; a weapon can never
	// grant it.
	MoveTypeCreature MoveType = "creature"
	// MoveTypeBoth marks a move any consumer may carry.
	MoveTypeBoth MoveType = "both"
)

// MoveStatus is the payload for a status effect.
type MoveStatus struct {
	// ID references a status asset (validated at load time).
	ID string
	// Chance is the application chance, in percent ([0, 100]).
	Chance int
}

// MoveEffect is an immutable-by-convention runtime copy of an authored effect.
type MoveEffect struct {
	Type                   string
	Target                 MoveTarget
	DmgModifier, CritBonus int
	Flat, Percent          int
	Status                 MoveStatus
}

// Move is a Combat 2.0 move: one entry of a combatant's move set, built
// from a validated MoveAsset. It is a plain value type — copies are safe
// and intended (move sets are rebuilt, never mutated in place).
type Move struct {
	ID   string
	Name string
	// Type says which consumers may carry the move (a weapon's grant
	// list, a creature's kit, or both — see MoveType). The zero value is
	// untyped: the usability pair reports false for it. Runtime moves
	// always come from validated assets, so the zero value only ever
	// appears in hand-built values.
	Type MoveType
	// Learnable indicates whether this move may be learned by a hero.
	Learnable bool
	// Tier places the move on the progression ladder (1 = earliest).
	Tier int
	// Cooldown is how many rounds the move stays locked after use
	// (0 = spammable).
	Cooldown int
	// Weight is the AI weight for monster picks. For a monster's moves it
	// carries that monster's own ref weight (the caller overrides it after
	// NewMoveFromAsset); for the canonical basic attack it is the move
	// asset's own weight.
	Weight  int
	Effects []MoveEffect
}

// HasStatus reports whether the move applies a status effect on hit.
func (m Move) HasStatus() bool {
	for _, effect := range m.Effects {
		if effect.Type == "status" {
			return true
		}
	}
	return false
}

// UsableByWeapons reports whether a WEAPON's grant list may carry the
// move (weapon- or both-typed). A creature-only move is learned or
// monster-fought — never granted.
func (m Move) UsableByWeapons() bool {
	return m.Type == MoveTypeWeapon || m.Type == MoveTypeBoth
}

// UsableByCreatures reports whether a CREATURE's move kit — a monster's
// set or the hero's learned set — may carry the move (creature- or
// both-typed). A weapon-only move reaches the hero through the equipped
// weapon, never through learning.
func (m Move) UsableByCreatures() bool {
	return m.Type == MoveTypeCreature || m.Type == MoveTypeBoth
}

// TargetSelf reports whether any status effect targets the striker.
func (m Move) TargetSelf() bool {
	for _, effect := range m.Effects {
		if effect.Type == "status" && effect.Target == TargetSelf {
			return true
		}
	}
	return false
}

// NewMoveFromAsset builds a runtime move from its asset. The asset must
// come from a validated registry (Load → Validate): unknown target words
// are copied through as-is, never remapped.
func NewMoveFromAsset(asset game.MoveAsset) Move {
	m := Move{
		ID:        asset.ID,
		Name:      asset.Name,
		Type:      MoveType(asset.Type),
		Learnable: asset.Learnable,
		Tier:      max(1, asset.Tier),
		Cooldown:  asset.Cooldown,
		Weight:    asset.Weight,
	}
	for _, effect := range asset.Effects {
		e := MoveEffect{Type: effect.Type, Target: MoveTarget(effect.Target)}
		if effect.DmgModifier != nil {
			e.DmgModifier = *effect.DmgModifier
		}
		if effect.CritBonus != nil {
			e.CritBonus = *effect.CritBonus
		}
		if effect.Flat != nil {
			e.Flat = *effect.Flat
		}
		if effect.Percent != nil {
			e.Percent = *effect.Percent
		}
		if effect.Status != nil {
			e.Status = MoveStatus{ID: effect.Status.ID, Chance: effect.Status.Chance}
		}
		m.Effects = append(m.Effects, e)
	}
	return m
}

// StatusKind says which side a status helps.
type StatusKind string

const (
	StatusBuff   StatusKind = "buff"
	StatusDebuff StatusKind = "debuff"
)

// StatKind names one of the modifiable combat stats.
type StatKind string

const (
	StatDMG StatKind = "dmg"
	StatDEF StatKind = "def"
	StatSPD StatKind = "spd"
)

// Status is a Combat 2.0 status effect built from a validated
// StatusAsset.
type Status struct {
	ID   string
	Name string
	Kind StatusKind
	// DoT components are damage each round; percentage damage is based on
	// target MaxHP at each tick.
	DotAmount   int
	DotPercent  int
	Stat        StatKind // "" for DoT effects
	StatAmount  int
	StatPercent int
	// Duration is how many rounds the status lasts.
	Duration int
}

// IsDot reports whether the status deals damage each round (instead of
// modifying a stat).
func (s Status) IsDot() bool { return s.Stat == "" && (s.DotAmount != 0 || s.DotPercent != 0) }

// NewStatusFromAsset builds a runtime status from its asset. The asset
// must come from a validated registry (Load → Validate): an unknown
// effect type leaves the runtime effect zeroed, never a panic.
func NewStatusFromAsset(asset game.StatusAsset) Status {
	s := Status{
		ID:       asset.ID,
		Name:     asset.Name,
		Kind:     StatusKind(asset.Kind),
		Duration: asset.Duration,
	}
	switch asset.Effect.Type {
	case "dot":
		s.DotAmount = asset.Effect.Amount
		s.DotPercent = asset.Effect.Percent
	case "stat":
		s.Stat = StatKind(asset.Effect.Stat)
		s.StatAmount = asset.Effect.Amount
		s.StatPercent = asset.Effect.Percent
	}
	return s
}

// MoveSource says where a hero's effective move comes from — the learned
// set (shrine teaching) or the weapon/basic-action slot. Virtual Unarmed
// Punch uses SourceWeapon because it occupies that same slot.
type MoveSource int

const (
	// SourceLearned marks a move learned via LearnMove (a shrine's teaching).
	SourceLearned MoveSource = iota
	// SourceWeapon marks an equipped weapon grant or the engine's virtual
	// Unarmed basic action.
	SourceWeapon
)

// SourcedMove pairs an effective move with its origin, for consumers
// that must distinguish learned from weapon-granted (the TUI's move
// grid tagging). Plain value type — copies are safe and intended.
type SourcedMove struct {
	Move   Move
	Source MoveSource
}

// FilterUnknownMoves narrows a registry move pool to the moves the hero
// does NOT already know — the shrine offer's filter step. `pool` is a
// tier-filtered pool (Registry.MovesForTiers; the filter preserves its
// order, sorted by id). `known` is any move list representing what the
// hero knows — the hero's effective set (EffectiveMoves) covers learned
// AND weapon-granted in one dedup-by-id pass; the filter dedups `known`
// by id itself, so raw sources work too. The canonical basic_attack is
// ALWAYS dropped: it is the move-less fallback every combatant already
// fights with, and teaching it would waste one of the two learned
// slots — it is never offerable. The result is a fresh slice, empty
// (never an error) when every move filters out: an empty pool is the
// shrine's "nothing new" case, for the caller to degrade as it sees
// fit. Pure: neither input is mutated.
func FilterUnknownMoves(pool []game.MoveAsset, known []Move) []game.MoveAsset {
	knownIDs := make(map[string]bool, len(known))
	for _, m := range known {
		knownIDs[m.ID] = true
	}
	filtered := make([]game.MoveAsset, 0, len(pool))
	for _, asset := range pool {
		if asset.ID == BasicAttack().ID || knownIDs[asset.ID] {
			continue
		}
		filtered = append(filtered, asset)
	}
	return filtered
}

// BasicAttack returns the safe built-in fallback value for callers without
// an engine registry. Engine combat resolves the canonical basic_attack ID
// from its active registry when available. A function (not a package
// variable) prevents callers from mutating a shared value.
func BasicAttack() Move {
	return Move{
		ID:       "basic_attack",
		Name:     "Basic Strike",
		Type:     MoveTypeBoth,
		Tier:     1,
		Cooldown: 0,
		Weight:   100,
		Effects:  []MoveEffect{{Type: "damage", Target: TargetEnemy, DmgModifier: 100}},
	}
}

// Punch returns the canonical opening move (asset id "punch"): a weak,
// spammable, creature-only strike — the move every hero is BORN knowing
// (NewPlayer pre-learns it into learned slot 1). Creature-typed on
// purpose: a fist is no weapon grant, so no weapon can ever carry it,
// and it is exactly what a shrine offers a hero who forgot it. A
// function (not a package variable) so callers can never mutate a
// shared value; the embedded moves.json ships the same move as an asset.
func Punch() Move {
	return Move{
		ID:       "punch",
		Name:     "Punch",
		Type:     MoveTypeBoth,
		Tier:     1,
		Cooldown: 0,
		Weight:   50,
		Effects:  []MoveEffect{{Type: "damage", Target: TargetEnemy, DmgModifier: 60}},
	}
}
