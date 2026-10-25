package entities

import "github.com/lckrugel/tower-of-doom/internal/game"

// MaxMoves is the cap on every combatant's move set (Combat 2.0). The
// number lives in the game package — the registry validates asset-level
// caps against it — and is re-exported here as the runtime-facing name.
const MaxMoves = game.MaxMoves

// MaxLearnedMoves is how many moves a hero may have LEARNED (shrine
// learning). Weapon-granted moves sit on top of the learned ones, and
// the union is capped by MaxMoves.
const MaxLearnedMoves = 2

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

// MoveStatus is the optional status a move applies on hit. The zero
// value means "no status" — check HasStatus instead of comparing ids.
type MoveStatus struct {
	// ID references a status asset (validated at load time).
	ID string
	// Chance is the application chance, in percent ([0, 100]).
	Chance int
	// Target says who receives the status.
	Target MoveTarget
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
	// Tier places the move on the progression ladder (1 = earliest).
	Tier int
	// DmgModifier scales the striker's DMG, in percent (100 = neutral).
	DmgModifier int
	// CritBonus adds percentage points to the striker's crit threshold
	// for this move (may be negative; clamped to [0, 100] at use time).
	CritBonus int
	// Cooldown is how many turns the move stays locked after use
	// (0 = spammable).
	Cooldown int
	// Status is the optional status the move applies on hit (zero value
	// = none).
	Status MoveStatus
	// Weight is the AI weight for monster picks. For a monster's moves it
	// carries that monster's own ref weight (the caller overrides it after
	// NewMoveFromAsset); for the canonical basic attack it is the move
	// asset's own weight.
	Weight int
}

// HasStatus reports whether the move applies a status effect on hit.
func (m Move) HasStatus() bool { return m.Status.ID != "" }

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

// TargetSelf reports whether the move's status targets the striker
// instead of the hit target. Always false for status-less moves.
func (m Move) TargetSelf() bool {
	return m.HasStatus() && m.Status.Target == TargetSelf
}

// NewMoveFromAsset builds a runtime move from its asset. The asset must
// come from a validated registry (Load → Validate): unknown target words
// are copied through as-is, never remapped.
func NewMoveFromAsset(asset game.MoveAsset) Move {
	m := Move{
		ID:          asset.ID,
		Name:        asset.Name,
		Type:        MoveType(asset.Type),
		Tier:        max(1, asset.Tier),
		DmgModifier: asset.DmgModifier,
		CritBonus:   asset.CritBonus,
		Cooldown:    asset.Cooldown,
		Weight:      asset.Weight,
	}
	if asset.Status != nil {
		m.Status = MoveStatus{
			ID:     asset.Status.ID,
			Chance: asset.Status.Chance,
			Target: MoveTarget(asset.Status.Target),
		}
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
	// Exactly one effect flavor is set: DotAmount (> 0) deals that much
	// damage per turn; otherwise Stat + StatAmount modify a stat (the
	// amount is never 0 in validated content).
	DotAmount  int
	Stat       StatKind // "" for DoT effects
	StatAmount int
	// Duration is how many turns the status lasts.
	Duration int
}

// IsDot reports whether the status deals damage per turn (instead of
// modifying a stat).
func (s Status) IsDot() bool { return s.DotAmount > 0 }

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
	case "stat":
		s.Stat = StatKind(asset.Effect.Stat)
		s.StatAmount = asset.Effect.Amount
	}
	return s
}

// MoveSource says where a hero's effective move comes from — the
// learned set (shrine teaching) or the equipped weapon's grants. The
// learned-vs-weapon split is the UI's "2+2" view of the move set.
type MoveSource int

const (
	// SourceLearned marks a move learned via LearnMove (a shrine's
	// teaching). A move known through BOTH channels reports as learned —
	// the learned copy wins the dedup.
	SourceLearned MoveSource = iota
	// SourceWeapon marks a move granted by the equipped weapon (it
	// leaves the set when the weapon is unworn).
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

// BasicAttack returns the canonical fallback move (asset id
// "basic_attack"): a neutral strike — 100% damage, no crit bonus, no
// cooldown, no status. Monsters whose asset lists no moves fight with
// it. A function (not a package variable) so callers can never mutate a
// shared value; the embedded moves.json ships the same move as an asset.
func BasicAttack() Move {
	return Move{
		ID:          "basic_attack",
		Name:        "Basic Strike",
		Type:        MoveTypeBoth,
		Tier:        1,
		DmgModifier: 100,
		CritBonus:   0,
		Cooldown:    0,
		Weight:      100,
	}
}
