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

// BasicAttack returns the canonical fallback move (asset id
// "basic_attack"): a neutral strike — 100% damage, no crit bonus, no
// cooldown, no status. Monsters whose asset lists no moves fight with
// it. A function (not a package variable) so callers can never mutate a
// shared value; the embedded moves.json ships the same move as an asset.
func BasicAttack() Move {
	return Move{
		ID:          "basic_attack",
		Name:        "Basic Strike",
		Tier:        1,
		DmgModifier: 100,
		CritBonus:   0,
		Cooldown:    0,
		Weight:      100,
	}
}
