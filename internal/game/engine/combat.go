package engine

import (
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

// resolveRound plays one full round of combat: whoever has the higher total
// Speed (items included) acts first — the Hero wins ties — and both
// combatants act unless someone dies mid-round.
//
// `heroAttack` tells whether the Hero acts this round. When false (failed
// sneak or failed flee) the round is a free turn for the enemy: a single
// enemy attack with no Hero action.
func (e *Engine) resolveRound(heroAttack bool) []Event {
	var events []Event
	heroFirst := e.Player.Speed() >= e.currentEnemy().Speed()

	if heroAttack && heroFirst {
		events = append(events, e.heroStrike()...)
		if !e.currentEnemy().IsAlive() {
			return e.appendEnemyDefeat(events)
		}
	}

	// The enemy always acts (first when it is faster than the Hero).
	events = append(events, e.enemyStrike()...)
	if !e.Player.IsAlive() {
		return e.appendHeroDefeat(events)
	}

	if heroAttack && !heroFirst {
		events = append(events, e.heroStrike()...)
		if !e.currentEnemy().IsAlive() {
			return e.appendEnemyDefeat(events)
		}
	}

	return events
}

// rollDamage resolves an attacker's strike: the attack stat with a small
// ±1 spread (floored at 1, no misses), doubled on a critical hit.
// Crit chance is the attacker's CritThreshold read as a percentage
// (rng.Intn(100) < threshold).
func (e *Engine) rollDamage(attacker Combatant) (dmg int, crit bool) {
	dmg = max(1, attacker.Damage()-1+e.rng.Intn(3))
	crit = e.rng.Intn(100) < attacker.CritThreshold()
	if crit {
		dmg *= 2
	}
	return dmg, crit
}

// heroStrike performs the Hero's attack against the current enemy.
func (e *Engine) heroStrike() []Event {
	enemy := e.currentEnemy()
	swing, crit := e.rollDamage(e.Player)
	return []Event{NewDamageDealt(enemy, enemy.TakeDmg(effectiveDamage(swing, enemy.Defense())), crit)}
}

// enemyStrike performs the current enemy's attack against the Hero.
func (e *Engine) enemyStrike() []Event {
	enemy := e.currentEnemy()
	swing, crit := e.rollDamage(enemy)
	return []Event{NewDamageTaken(e.Player, e.Player.TakeDmg(effectiveDamage(swing, e.Player.Defense())), crit)}
}

// enemyFreeTurn is the single enemy attack granted by a failed sneak or a
// failed flee attempt: the Hero does not act this round.
func (e *Engine) enemyFreeTurn() []Event {
	events := e.enemyStrike()
	if !e.Player.IsAlive() {
		return e.appendHeroDefeat(events)
	}
	return events
}

// appendEnemyDefeat grants the kill XP and the kill gold instantly
// (mid-combat level ups are possible), advances the encounter queue and
// transitions back to exploring when the last enemy of the encounter
// falls. The gold grant consumes no rng.
func (e *Engine) appendEnemyDefeat(events []Event) []Event {
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
const defCurveK = 45

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
