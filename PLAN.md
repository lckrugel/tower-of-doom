# Tower of Doom — Combat & Game Loop Implementation Plan

Goal: reach a playable state (engine + simple CLI stdin loop, no bubbletea).
No gold/item rewards in this iteration.

## Locked decisions

| Topic | Decision |
|---|---|
| Combat math | Minimal: flat stat damage, no crit rolls, DEF ignored for now |
| Round structure | 1 player command (ATTACK/FLEE) = 1 full round; both act; higher total SPD first; **hero wins ties** |
| Free turn (sneak fail / flee fail) | Single enemy attack; hero doesn't act; then normal rounds resume |
| Monster queue | 1×1 combat; queue in spawn order; next enemy steps in with normal initiative, no free turn |
| Flee chance | `50% + 5%·(heroSPD − enemySPD)` clamped to [5, 95]; roll via `rng.Intn(100)`; success exits combat, no reward; fail → enemy free turn |
| Sneak | 50/50 via `rng.Intn(2)` (0 = success); success discards encounter → StateExploring, CanAscend untouched (only victory unlocks ascend); fail → combat starts + enemy free turn |
| XP | `4 × difficulty` per kill, granted instantly mid-combat; mid-combat level-ups possible |
| CmdFight | Starts combat **and** resolves the first round immediately (hero attacks with standard initiative) |
| Quit | CmdQuit allowed in every state; engine emits EventQuit, CLI loop exits |
| Randomness | All combat rolls through the engine's injectable `rng` seam; `dice.go` crypto rolls untouched |
| Level-up events | New `EventLevelUp` event type (may add other event types as needed) |

## Bug fix (entities)

- `CurrentHPPercent()` in both `hero.go` and `monster.go`: currently `floor(HP/MaxHP) * 100`
  → always 0 except full HP. Fix to `floor(HP/MaxHP × 100)`.
- `TakeDmg` stays exactly as-is (locked by tests). CritThreshold stays unused.

## Changes by file

### 1. `internal/game/engine/states.go`
- Add `CmdQuit` to `AllowedCommands` for every state (including GameOver).

### 2. `internal/game/engine/engine.go`
- `Process`: add `case StateEncounter: return e.handleEncounter(cmd)`.
- `handleExploring` CmdExplore: after `generateEncounter`, set `State = StateEncounter`.
- New `handleEncounter`:
  - `CmdFight` → `State = StateCombat`, then resolve first round with hero attacking.
  - `CmdSneak` → roll success: EventSneakSuccess, clear encounter, State → Exploring;
    fail: EventSneakFailed, State → Combat, enemy free turn (hero may die → GameOver).
- `handleCombat`:
  - `CmdAttack` → full round (hero attacks).
  - `CmdFlee` → fleeChance (extract pure fn) → roll; success: EventFled, clear encounter,
    State → Exploring, no reward; fail: message + enemy free turn.
- `handleGameOver`: CmdQuit works; anything else → message event.
- Quit handling in every state: emit EventQuit, stop processing.

### 3. `internal/game/engine/combat.go`
- Rework `ProcessCombatTurn` into a full round resolver:
  - Hero faster / tie: hero attacks → enemy dead? process death : enemy attacks → hero-death check.
  - Enemy faster: enemy attacks → hero-death check → hero attacks.
  - Free-turn mode: enemy attacks once, no hero action.
  - Enemy death: pop front of queue, grant XP (`Player.GiveXP(m.GetKillXp())`),
    EventEnemyDefeated + EventLevelUp if leveled; queue empty → EventVictory,
    `CanAscend = true`, State → Exploring; else next enemy steps in.
  - Hero death: EventHeroDefeated, `State = StateGameOver`, stop remaining round.

### 4. `internal/game/engine/events.go`
- New event types/constructors: `NewDamageTaken`, `NewSneakSuccess`, `NewSneakFailed`,
  `NewFled`, `NewVictory`, `NewEnemyDefeated`, `NewHeroDefeated`, `NewQuit`, `NewLevelUp`.
- Same `Data map[string]any` style as existing constructors.

### 5. `internal/game/engine/entities/{hero,monster}.go`
- Fix `CurrentHPPercent()` (see bug fix above).

### 6. `cmd/cli/main.go`
- Simple stdin loop: print prompt/allowed commands, read word
  (`explore/ascend/fight/sneak/attack/flee/quit`), map to `Command`,
  call `engine.Process`, print each Event as a text line:
  encounter composition, damage both ways, enemy defeated + xp + level-ups,
  victory, fled, game over, floor entered. Exit on quit/EOF.

### 7. Tests
- Routing/transitions: explore → StateEncounter; fight → StateCombat; ascend
  blocked/unblocked → EventFloorEntered; quit allowed in every state.
- Round resolution (deterministic stubRng): hero-faster order (dealt → taken),
  enemy-faster order (taken → dealt), tie → hero first; kill skips remaining enemy
  attack that round; mid-combat XP grant + mid-combat level-up; queue advance;
  last kill → EventVictory + CanAscend + State Exploring.
- Sneak: success (rng=0) discards encounter; fail (rng=1) → free turn.
- Flee: fleeChance clamp table (diff −10 → 5, 0 → 50, +10 → 95); success → EventFled +
  exploring; fail → free turn; hero can die during free turn.
- GameOver: only quit accepted; other commands → message.
- Entities: `CurrentHPPercent` table (0/33/99/100) for hero and monster.

## Notes
- Comments/doc in English (project convention).
- Commit small complete tasks as they finish.
