package engine

import (
	"fmt"
	"math"

	"github.com/lckrugel/tower-of-doom/internal/game"
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

// CombatMoves returns the hero's combat actions in the same order as
// CombatMovesWithSource. The basic action (weapon grant or virtual Unarmed
// Punch) is first, followed by learned cooldown moves.
func (e *Engine) CombatMoves() []entities.Move {
	sourced := e.CombatMovesWithSource()
	moves := make([]entities.Move, len(sourced))
	for i := range sourced {
		moves[i] = sourced[i].Move
	}
	return moves
}

// CombatMovesWithSource is the authoritative hero loadout for combat,
// command indexing, and UI display. A weapon supplies the first basic
// action; without a weapon the engine supplies registry-resolved Punch as
// a virtual weapon-source action. Learned moves follow in learning order.
func (e *Engine) CombatMovesWithSource() []entities.SourcedMove {
	if e.Player.Weapon != nil {
		return e.Player.EffectiveMovesWithSource(e.Player.Weapon)
	}
	moves := make([]entities.SourcedMove, 0, entities.MaxMoves)
	moves = append(moves, entities.SourcedMove{Move: e.unarmedPunch(), Source: entities.SourceWeapon})
	moves = append(moves, e.Player.EffectiveMovesWithSource(nil)...)
	return moves
}

// unarmedPunch resolves the virtual Unarmed action from the active
// registry, with the canonical entity definition as a safe fallback.
func (e *Engine) unarmedPunch() entities.Move {
	if asset, ok := e.reg.MoveByID(entities.Punch().ID); ok {
		return entities.NewMoveFromAsset(asset)
	}
	return entities.Punch()
}

// basicAttack resolves the canonical fallback from the active registry. The
// entity-level value remains a safe fallback for unvalidated/minimal registries.
func (e *Engine) basicAttack() entities.Move {
	if asset, ok := e.reg.MoveByID(entities.BasicAttack().ID); ok {
		return entities.NewMoveFromAsset(asset)
	}
	return entities.BasicAttack()
}

// MoveCooldown reports how many more rounds the hero's move stays
// locked (0 = ready) — a read-only view of the combat-scoped cooldown
// state for the UI's chip rendering.
func (e *Engine) MoveCooldown(moveID string) int {
	return e.heroCombat.cooldowns[moveID]
}

// resolveRound plays one full round of combat from the hero's move
// choice: whoever has the higher EFFECTIVE Speed acts first — the Hero
// wins ties — and both combatants act unless someone dies mid-round.
// `move` is the Hero's chosen move (the CmdMove index into
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
	return e.resolveCombatRound(move, nil, events)
}

// resolveConsumableRound spends the Hero's action on a consumable. The
// consumable is applied only when the Hero reaches their initiative point.
func (e *Engine) resolveConsumableRound(consumable game.ConsumableAsset) []Event {
	return e.resolveCombatRound(nil, &consumable, nil)
}

// resolveCombatRound is the round core shared by the hero's move rounds,
// RECOVER rounds and the enemy's free turns (failed sneak/flee): `move`
// is the hero's chosen move (nil = the hero does not act this round) and
// `events` any event prefix already collected.
func (e *Engine) resolveCombatRound(move *entities.Move, consumable *game.ConsumableAsset, events []Event) []Event {
	// Initiative reads the EFFECTIVE speeds (stat-mod statuses included).
	heroFirst := e.heroStat(entities.StatSPD) >= e.enemyStat(entities.StatSPD)
	heroActs := func() []Event {
		if move != nil {
			return e.heroStrike(move)
		}
		if consumable != nil {
			return e.drinkConsumable(*consumable)
		}
		return nil
	}

	if (move != nil || consumable != nil) && heroFirst {
		events = append(events, heroActs()...)
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

	if (move != nil || consumable != nil) && !heroFirst {
		events = append(events, heroActs()...)
		if !e.currentEnemy().IsAlive() {
			return e.appendEnemyDefeat(events, move)
		}
	}

	// The round completed: the round-end ticks run.
	return e.endRound(events, move, enemyMove)
}

// effectiveStat is a combatant's EFFECTIVE stat: base+gear (the
// Combatant accessor) plus the sum of its active stat-mod statuses,
// clamped — DMG at 1 (a weakened fighter still chips), DEF and SPD at 0.
// Used for damage, mitigation, initiative and the flee formula alike.
// Pure math — no rng.
func (e *Engine) effectiveStat(c Combatant, st *combatantState, stat entities.StatKind) int {
	var total int
	switch stat {
	case entities.StatDMG:
		total = c.Damage()
	case entities.StatDEF:
		total = c.Defense()
	case entities.StatSPD:
		total = c.Speed()
	}
	flat, percent := st.statMods(stat)
	total = total*(max(0, 100+percent))/100 + flat
	if stat == entities.StatDMG {
		return max(1, total)
	}
	return max(0, total)
}

// heroStat is the hero's effective stat in the current combat.
func (e *Engine) heroStat(stat entities.StatKind) int {
	return e.effectiveStat(e.Player, &e.heroCombat, stat)
}

// enemyStat is the current enemy's effective stat in the current combat.
func (e *Engine) enemyStat(stat entities.StatKind) int {
	return e.effectiveStat(e.currentEnemy(), &e.enemyCombat, stat)
}

// stateOf returns the combat-scoped state of a combatant: a fight has
// exactly two sides — the hero and the current front monster.
func (e *Engine) stateOf(c Combatant) *combatantState {
	if _, isHero := c.(*entities.Hero); isHero {
		return &e.heroCombat
	}
	return &e.enemyCombat
}

// rollMoveDamage resolves a strike made with a Combat 2.0 move, in this
// rng order (a determinism contract):
//
//  1. base = round(effective DMG × dmgModifier / 100) — pure math,
//     no rng (the modifier is a percent, 100 = neutral);
//  2. damage spread: max(1, base − 1 + Intn(3)) (±1 around the base,
//     floored at 1, no misses);
//  3. crit: Intn(100) < clamp(critThreshold + critBonus, 0, 100) —
//     doubles the swung damage;
//  4. (a status-carrying move rolls its application chance AFTER the
//     crit — see the status effects.)
func (e *Engine) rollMoveDamage(effDmg, dmgModifier, critBonus, critThreshold int) (swing int, crit bool) {
	base := int(math.Round(float64(effDmg*dmgModifier) / 100))
	swing = max(1, base-1+e.rng.Intn(3))
	crit = e.rng.Intn(100) < min(100, max(0, critThreshold+critBonus))
	if crit {
		swing *= 2
	}
	return swing, crit
}

// heroStrike performs the Hero's strike with the chosen move against the
// current enemy: the move's damage math over the hero's EFFECTIVE DMG,
// mitigated by the enemy's EFFECTIVE DEF, then the plain TakeDmg clamp.
// rng order: spread → crit (→ status chance when the move carries one).
func (e *Engine) heroStrike(move *entities.Move) []Event {
	return e.resolveMoveEffects(move, e.Player, e.currentEnemy(), ActorHero)
}

// resolveMoveEffects executes a move's authored effects in order.
func (e *Engine) resolveMoveEffects(move *entities.Move, attacker, defender Combatant, side ActorSide) []Event {
	events := []Event{NewMoveUsedOnSide(attacker.Name(), move.Name, side)}
	for _, effect := range move.Effects {
		target := defender
		if effect.Target == entities.TargetSelf {
			target = attacker
		}
		if !target.IsAlive() {
			if effect.Type == "status" {
				e.rng.Intn(100)
			}
			continue
		}
		switch effect.Type {
		case "damage":
			if target != defender {
				continue
			}
			var dmg int
			var crit bool
			if side == ActorHero {
				swing, c := e.rollMoveDamage(e.heroStat(entities.StatDMG), effect.DmgModifier, effect.CritBonus, attacker.CritThreshold())
				crit = c
				dmg = target.TakeDmg(effectiveDamage(swing, e.enemyStat(entities.StatDEF)))
			} else {
				swing, c := e.rollMoveDamage(e.enemyStat(entities.StatDMG), effect.DmgModifier, effect.CritBonus, attacker.CritThreshold())
				crit = c
				dmg = target.TakeDmg(effectiveDamage(swing, e.heroStat(entities.StatDEF)))
			}
			if side == ActorHero {
				events = append(events, NewDamageDealtWithMove(target, dmg, crit, move.Name))
			} else {
				events = append(events, NewDamageTakenWithMove(target, dmg, crit, move.Name))
			}
		case "heal":
			amount := effect.Flat + target.MaxHP()*effect.Percent/100
			healed := target.Heal(amount)
			events = append(events, NewCombatHealed(target, side, move.Name, healed))
		case "status":
			fired := e.rng.Intn(100) < effect.Status.Chance
			statusTarget := target
			if effect.Target == entities.TargetSelf {
				statusTarget = attacker
			} else if effect.Target == entities.TargetEnemy {
				statusTarget = defender
			}
			if !fired || !statusTarget.IsAlive() {
				continue
			}
			asset, ok := e.reg.StatusByID(effect.Status.ID)
			if !ok {
				panic(fmt.Sprintf("unreachable: move %v references unknown status %v despite passing content validation", move.ID, effect.Status.ID))
			}
			st := entities.NewStatusFromAsset(asset)
			events = append(events, NewStatusApplied(statusTarget, st, e.stateOf(statusTarget).applyStatus(st)))
		}
	}
	return events
}

// applyStrikeStatus rolls a move's status application. The rng contract
// is ONE Intn(100) per status-carrying move use — the roll happens even
// when the chance is 0 or fails, keeping every rng sequence
// position-stable. On fire the status is applied to the move's target
// (enemy = the defender of the hit, self = the striker), refreshed to
// the longest duration; a strike that KILLED its recipient afflicts
// nothing (dead combatants can't take statuses) — the roll still
// happened.
// enemyTurnMove resolves the monster's move for this turn (AI v1): ONE
// Intn over the cooldown-filtered move set's total weight — the rng
// contract is a single pick roll per monster turn. Locked moves are
// filtered out BEFORE the roll (filtering consumes no rng); when every
// move is locked the monster falls back to the canonical basic attack
// WITHOUT consuming a pick roll — there is nothing left to pick.
func (e *Engine) enemyTurnMove() *entities.Move {
	moves := e.currentEnemy().Moves()
	filtered := make([]entities.Move, 0, len(moves))
	for _, m := range moves {
		if e.enemyCombat.onCooldown(m.ID) {
			continue
		}
		filtered = append(filtered, m)
	}
	if len(filtered) == 0 {
		basic := e.basicAttack()
		return &basic
	}
	asset, ok := e.reg.Monsters[e.currentEnemy().ID]
	if !ok || asset.AI == nil {
		picked := pickMonsterMove(e, filtered)
		return &picked
	}
	for _, rule := range asset.AI.Priorities {
		matches := true
		for _, condition := range rule.When {
			if !e.monsterConditionMatches(condition) {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		candidates := movesByIDs(filtered, rule.Moves)
		if len(candidates) != 0 {
			picked := pickMonsterMove(e, candidates)
			return &picked
		}
	}
	fallback := movesByIDs(filtered, asset.AI.Fallback)
	if len(fallback) == 0 {
		basic := e.basicAttack()
		return &basic
	}
	picked := pickMonsterMove(e, fallback)
	return &picked
}

func pickMonsterMove(e *Engine, moves []entities.Move) entities.Move {
	weights := make([]int, len(moves))
	for i, move := range moves {
		weights[i] = move.Weight
	}
	return weightedPick(e.rng, moves, weights)
}

func movesByIDs(moves []entities.Move, ids []string) []entities.Move {
	var result []entities.Move
	for _, id := range ids {
		for _, move := range moves {
			if move.ID == id {
				result = append(result, move)
				break
			}
		}
	}
	return result
}

func (e *Engine) monsterConditionMatches(c game.MonsterConditionAsset) bool {
	var target Combatant
	state := &e.enemyCombat
	if c.Target == "self" {
		target = e.currentEnemy()
	} else {
		target = e.Player
		state = &e.heroCombat
	}
	switch c.Type {
	case "hp_below":
		return c.Target == "self" && target.HP()*100 < target.MaxHP()*c.Percent
	case "status_present", "status_absent":
		present := false
		for _, status := range state.statuses {
			if status.status.ID == c.StatusID {
				present = true
				break
			}
		}
		if c.Type == "status_present" {
			return present
		}
		return !present
	default:
		return false
	}
}

// enemyStrike performs the current enemy's attack against the Hero with
// the given move — the same move math as the hero's side, over the
// monster's EFFECTIVE stats (its own crit asset plus the move's bonus).
func (e *Engine) enemyStrike(move *entities.Move) []Event {
	return e.resolveMoveEffects(move, e.currentEnemy(), e.Player, ActorEnemy)
}

// enemyFreeTurn is the single enemy attack granted by a failed sneak or a
// failed flee attempt: the Hero does not act this round — no recover
// announcement, just the enemy's turn.
func (e *Engine) enemyFreeTurn() []Event {
	return e.resolveCombatRound(nil, nil, nil)
}

// endRound runs the round-end ticks of a COMPLETED round (both sides
// acted — a round aborted by a death skips them), in this order:
//
//  1. status ticks, the HERO's afflictions first (deterministic order):
//     every DoT damages its afflicted — a plain clamp, CAN kill (a dead
//     monster goes through the normal defeat path, a dead hero ends the
//     game) — then every status counts a round down, expiring at 0 with
//     an event;
//  2. cooldowns tick one round down (both sides);
//  3. the moves used this round lock for their full cooldown — set
//     AFTER the tick so the round of use doesn't eat one turn of the
//     lock: with cooldown C the move stays locked for exactly C full
//     rounds after this one.
func (e *Engine) endRound(events []Event, heroMove, enemyMove *entities.Move) []Event {
	tickEvents, heroDead := tickStatuses(&e.heroCombat, e.Player)
	events = append(events, tickEvents...)
	if heroDead {
		return e.appendHeroDefeat(events)
	}
	tickEvents, enemyDead := tickStatuses(&e.enemyCombat, e.currentEnemy())
	events = append(events, tickEvents...)
	if enemyDead {
		// Both sides completed the round before the poison tick, so existing
		// hero cooldowns advance. appendEnemyDefeat then applies the move used
		// this round at its full cooldown, after that completed-round tick.
		e.heroCombat.tickCooldowns()
		return e.appendEnemyDefeat(events, heroMove)
	}

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
// resolves the encounter's end when the last enemy falls — one of three
// endings: a BOSS victory (the boss rewards and the gate, see
// appendBossVictory), the boss CHALLENGE unlocking (a normal victory on
// the final floor of a section with a boss gate — ASCEND stays locked),
// or the plain victory (ASCEND unlocks, as always). The gold grants
// consume no rng. `heroMove` is the move the Hero used this round (locked
// for its full cooldown — the round ended early, so no tick ran); the
// next monster in the queue steps in with fresh combat-scoped state.
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
		if e.CurrentEncounter.Boss {
			return e.appendBossVictory(events)
		}
		// A normal encounter's ending. On the final floor of a section
		// with a boss gate the victory summons the CHALLENGE instead of
		// opening the gate; sections without a boss block keep the old
		// regime on their final floor (mod compatibility).
		if section := e.currentSection(); section.Boss != nil && e.reg.IsSectionEnd(e.CurrentFloor) {
			e.BossUnlocked = true
			e.State = StateExploring
			e.CurrentEncounter = Encounter{}
			return append(events, NewBossUnlocked())
		}
		e.CanAscend = true
		e.State = StateExploring
		e.CurrentEncounter = Encounter{}
		events = append(events, NewVictory())
	}
	return events
}

// appendHeroDefeat ends the game, reporting the run state at death:
// floor reached, hero level and enemies defeated so far.
func (e *Engine) appendHeroDefeat(events []Event) []Event {
	e.State = StateGameOver
	events = append(events, NewHeroDefeatedWithStats(e.CurrentFloor, e.Player.Lvl, e.kills))
	e.CurrentEncounter = Encounter{}
	return events
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
