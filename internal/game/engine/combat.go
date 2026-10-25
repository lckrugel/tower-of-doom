package engine

import (
	"math"

	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// `Combatant` defines an entity that can join a combat.
type Combatant interface {
	Name() string
	CurrentHPPercent() int
	// HP is the exact current HP value; CurrentHPPercent is its
	// percentage view.
	HP() int
	MaxHP() int
	Damage() int
	Speed() int
	Defense() int
	CritThreshold() int
	TakeDmg(dmg int) int
	Heal(hp int) int
	IsAlive() bool
}

var (
	_ Combatant = (*entities.Hero)(nil)
	_ Combatant = (*entities.Monster)(nil)
)

// `currentEnemy` returns the enemy currently engaged in combat —
// the front of the encounter queue.
func (e *Engine) currentEnemy() *entities.Monster {
	return &e.CurrentEncounter.Monsters[0]
}

// CombatMoves returns the move set the Hero fights with in StateCombat:
// the hero's effective set (learned moves first, then weapon-granted —
// see Hero.EffectiveMoves), or the canonical basic attack when that set
// is empty. The fallback is a deliberate combat-integration decision
// (the entity layer leaves the empty set meaning open): a hero without
// moves must still be able to act, mirroring the monster-side rule that
// a monster without a move set fights with basic_attack — move-less
// mods and starter gear keep working. The set is rebuilt per call; the
// CmdMove index points into exactly this list.
func (e *Engine) CombatMoves() []entities.Move {
	moves := e.Player.EffectiveMoves(e.Player.Weapon)
	if len(moves) == 0 {
		return []entities.Move{entities.BasicAttack()}
	}
	return moves
}

// resolveRound plays one full round of combat from the hero's move
// choice: whoever has the higher total Speed (items included) acts first
// — the Hero wins ties — and both combatants act unless someone dies
// mid-round. `move` is the Hero's chosen move (the CmdMove index into
// CombatMoves); nil means a RECOVER round — every hero move is locked,
// so the turn advances with an announced idle round instead of a
// soft-lock.
func (e *Engine) resolveRound(move *entities.Move) []Event {
	var events []Event
	if move == nil {
		// RECOVER round: announced, then the enemy's turn and the
		// round-end ticks run as usual.
		events = append(events, NewMessage("Every move is on cooldown — you recover this turn."))
	}
	return e.resolveCombatRound(move, events)
}

// resolveCombatRound is the round core shared by the hero's move rounds,
// RECOVER rounds and the enemy's free turns (failed sneak/flee): `move`
// is the hero's chosen move (nil = the hero does not act this round) and
// `events` any event prefix already collected.
func (e *Engine) resolveCombatRound(move *entities.Move, events []Event) []Event {
	heroFirst := e.Player.Speed() >= e.currentEnemy().Speed()

	if move != nil && heroFirst {
		events = append(events, e.heroStrike(move)...)
		if !e.currentEnemy().IsAlive() {
			return e.appendEnemyDefeat(events, move)
		}
	}

	// The enemy always acts (first when it is faster than the Hero).
	enemyMove := e.enemyTurnMove()
	events = append(events, e.enemyStrike(enemyMove)...)
	if !e.Player.IsAlive() {
		return e.appendHeroDefeat(events)
	}

	if move != nil && !heroFirst {
		events = append(events, e.heroStrike(move)...)
		if !e.currentEnemy().IsAlive() {
			return e.appendEnemyDefeat(events, move)
		}
	}

	// The round completed: the round-end ticks run.
	return e.endRound(events, move, enemyMove)
}

// rollMoveDamage resolves a strike made with a Combat 2.0 move, in this
// rng order (a determinism contract):
//
//  1. base = round(effective DMG × move.DmgModifier / 100) — pure math,
//     no rng (the modifier is a percent, 100 = neutral);
//  2. damage spread: max(1, base − 1 + Intn(3)) (±1 around the base,
//     floored at 1, no misses);
//  3. crit: Intn(100) < clamp(critThreshold + move.CritBonus, 0, 100) —
//     doubles the swung damage;
//  4. (a status-carrying move rolls its application chance AFTER the
//     crit — see the status effects.)
func (e *Engine) rollMoveDamage(effDmg int, move *entities.Move, critThreshold int) (swing int, crit bool) {
	base := int(math.Round(float64(effDmg*move.DmgModifier) / 100))
	swing = max(1, base-1+e.rng.Intn(3))
	crit = e.rng.Intn(100) < min(100, max(0, critThreshold+move.CritBonus))
	if crit {
		swing *= 2
	}
	return swing, crit
}

// heroStrike performs the Hero's strike with the chosen move against the
// current enemy: the move's damage math, then mitigation, then the plain
// TakeDmg clamp. rng order: spread → crit (→ status chance, once the
// move carries a status).
func (e *Engine) heroStrike(move *entities.Move) []Event {
	enemy := e.currentEnemy()
	swing, crit := e.rollMoveDamage(e.Player.Damage(), move, e.Player.CritThreshold())
	events := []Event{NewMoveUsed(e.Player.Name(), move.Name)}
	events = append(events, NewDamageDealtWithMove(enemy,
		enemy.TakeDmg(effectiveDamage(swing, enemy.Defense())), crit, move.Name))
	return events
}

// enemyTurnMove resolves the monster's move for this turn (AI v1): ONE
// Intn over the cooldown-filtered move set's total weight — the rng
// contract is a single pick roll per monster turn. Locked moves are
// filtered out BEFORE the roll (filtering consumes no rng); when every
// move is locked the monster falls back to the canonical basic attack
// WITHOUT consuming a pick roll — there is nothing left to pick.
func (e *Engine) enemyTurnMove() *entities.Move {
	moves := e.currentEnemy().Moves()
	filtered := make([]entities.Move, 0, len(moves))
	weights := make([]int, 0, len(moves))
	for _, m := range moves {
		if e.enemyCombat.onCooldown(m.ID) {
			continue
		}
		filtered = append(filtered, m)
		weights = append(weights, m.Weight)
	}
	if len(filtered) == 0 {
		basic := entities.BasicAttack()
		return &basic
	}
	picked := weightedPick(e.rng, filtered, weights)
	return &picked
}

// enemyStrike performs the current enemy's attack against the Hero with
// the given move.
func (e *Engine) enemyStrike(move *entities.Move) []Event {
	enemy := e.currentEnemy()
	swing, crit := e.rollMoveDamage(enemy.Damage(), move, enemy.CritThreshold())
	events := []Event{NewMoveUsed(enemy.Name(), move.Name)}
	events = append(events, NewDamageTakenWithMove(e.Player,
		e.Player.TakeDmg(effectiveDamage(swing, e.Player.Defense())), crit, move.Name))
	return events
}

// enemyFreeTurn is the single enemy attack granted by a failed sneak or a
// failed flee attempt: the Hero does not act this round — no recover
// announcement, just the enemy's turn.
func (e *Engine) enemyFreeTurn() []Event {
	return e.resolveCombatRound(nil, nil)
}

// endRound runs the round-end ticks of a COMPLETED round (both sides
// acted — a round aborted by a death skips them):
//
//  1. cooldowns tick one round down (both sides; statuses will tick
//     here too, once they exist);
//  2. the moves used this round lock for their full cooldown — set
//     AFTER the tick so the round of use doesn't eat one turn of the
//     lock: with cooldown C the move stays locked for exactly C full
//     rounds after this one.
func (e *Engine) endRound(events []Event, heroMove, enemyMove *entities.Move) []Event {
	e.heroCombat.tickCooldowns()
	e.enemyCombat.tickCooldowns()
	if heroMove != nil {
		e.heroCombat.startCooldown(*heroMove)
	}
	if enemyMove != nil {
		e.enemyCombat.startCooldown(*enemyMove)
	}
	return events
}

// appendEnemyDefeat grants the kill XP and the kill gold instantly
// (mid-combat level ups are possible), advances the encounter queue and
// transitions back to exploring when the last enemy of the encounter
// falls. The gold grant consumes no rng. `heroMove` is the move the Hero
// used this round (locked for its full cooldown — the round ended early,
// so no tick ran); the next monster in the queue steps in with fresh
// combat-scoped state.
func (e *Engine) appendEnemyDefeat(events []Event, heroMove *entities.Move) []Event {
	enemy := e.currentEnemy()
	xp := enemy.GetKillXp()
	gold := goldPerDifficulty * enemy.Difficulty
	oldLvl := e.Player.Lvl

	healed := e.Player.GiveXP(xp)
	e.Player.Gold += gold
	e.kills++
	events = append(events, NewEnemyDefeated(enemy.Name(), xp))
	events = append(events, NewGoldGained(gold, GoldSourceKill))
	if e.Player.Lvl > oldLvl {
		// The heal rides the level-up event (giveXP returns the total the
		// level ups actually healed; the heal itself consumes no rng).
		events = append(events, NewLevelUpWithHeal(e.Player.Lvl, healed))
	}

	if heroMove != nil {
		e.heroCombat.startCooldown(*heroMove)
	}
	e.enemyCombat = newCombatantState()
	e.CurrentEncounter.Monsters = e.CurrentEncounter.Monsters[1:]
	if len(e.CurrentEncounter.Monsters) == 0 {
		e.CanAscend = true
		e.State = StateExploring
		events = append(events, NewVictory())
	}
	return events
}

// appendHeroDefeat ends the game, reporting the run state at death:
// floor reached, hero level and enemies defeated so far.
func (e *Engine) appendHeroDefeat(events []Event) []Event {
	e.State = StateGameOver
	return append(events, NewHeroDefeatedWithStats(e.CurrentFloor, e.Player.Lvl, e.kills))
}

// defCurveK tunes the defense mitigation curve — mitigation per hit is
// DEF/(DEF+K), so a defender with DEF == K halves incoming damage.
// Easily tweakable: lower K makes armor stronger, higher K weaker.
const defCurveK = 35

// effectiveDamage applies the defender's mitigation to a swung hit:
// swing × K/(DEF+K), floored at 1 so no landed hit is fully nullified.
// Pure math — consumes no rng.
func effectiveDamage(swing, def int) int {
	return max(1, swing*defCurveK/(def+defCurveK))
}

// fleeChance returns the flee probability in percent, weighted by the speed
// difference between the Hero and the enemy, always clamped to [5, 95].
func fleeChance(heroSpeed, enemySpeed int) int {
	chance := 50 + 5*(heroSpeed-enemySpeed)
	return min(95, max(5, chance))
}
