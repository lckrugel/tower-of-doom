package engine

import "github.com/lckrugel/tower-of-doom/internal/game/engine/entities"

// ActorSide is a stable combat identity, independent of an actor's display name.
type ActorSide string

const (
	ActorHero  ActorSide = "hero"
	ActorEnemy ActorSide = "enemy"
)

// CombatStats contains effective combat values, including active status modifiers.
type CombatStats struct {
	HP, MaxHP           int
	DMG, DEF, SPD, Crit int
}

type CombatMoveView struct {
	ID, Name               string
	Cooldown, CooldownLeft int
}

type CombatStatusView struct {
	ID, Name string
	Kind     entities.StatusKind
	// TurnsLeft is the remaining count in LifetimeUnit (legacy field name).
	TurnsLeft    int
	LifetimeUnit string
	Category     string
}

type CombatActorView struct {
	Side     ActorSide
	ID, Name string
	Stats    CombatStats
	Moves    []CombatMoveView
	Statuses []CombatStatusView
}

// CombatView is a read-only snapshot of the current fight. It contains value
// copies only; mutating its slices cannot change engine state.
type CombatView struct {
	InCombat    bool
	Hero, Enemy CombatActorView
}

// CombatView returns the current combat snapshot; outside combat it reports
// InCombat=false. Actor identity uses side and monster ID, never display names.
func (e *Engine) CombatView() CombatView {
	if e.State != StateCombat || len(e.CurrentEncounter.Monsters) == 0 {
		return CombatView{}
	}
	return CombatView{
		InCombat: true,
		Hero:     e.combatActor(ActorHero, "hero", e.Player, &e.heroCombat, true),
		Enemy:    e.combatActor(ActorEnemy, e.currentEnemy().ID, e.currentEnemy(), &e.enemyCombat, false),
	}
}

func (e *Engine) combatActor(side ActorSide, id string, c Combatant, state *combatantState, hero bool) CombatActorView {
	stat := func(kind entities.StatKind) int {
		if hero {
			return e.heroStat(kind)
		}
		return e.enemyStat(kind)
	}
	v := CombatActorView{
		Side: side, ID: id, Name: c.Name(),
		Stats: CombatStats{HP: c.HP(), MaxHP: c.MaxHP(), DMG: stat(entities.StatDMG), DEF: stat(entities.StatDEF), SPD: stat(entities.StatSPD), Crit: c.CritThreshold()},
	}
	if hero {
		for _, move := range e.CombatMoves() {
			v.Moves = append(v.Moves, CombatMoveView{ID: move.ID, Name: move.Name, Cooldown: move.Cooldown, CooldownLeft: state.cooldowns[move.ID]})
		}
	} else {
		for _, move := range e.currentEnemy().Moves() {
			v.Moves = append(v.Moves, CombatMoveView{ID: move.ID, Name: move.Name, Cooldown: move.Cooldown, CooldownLeft: state.cooldowns[move.ID]})
		}
	}
	for _, active := range state.statuses {
		unit, category := statusLifetime(active.status)
		v.Statuses = append(v.Statuses, CombatStatusView{ID: active.status.ID, Name: active.status.Name, Kind: active.status.Kind, TurnsLeft: active.remaining(state.turn), LifetimeUnit: unit, Category: category})
	}
	return v
}
