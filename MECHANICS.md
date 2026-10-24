# Proposed game mechanics(WIP)


## Game loop

- Game starts (StateExploring)
- Player can EXPLORE or ASCEND
    - ASCEND starts blocked
- EXPLORE generates an encounter (StateEncounter)
    - TODO: adds different types of encounters
- PLayer can FIGHT or SNEAK
- FIGHT leads to combat (StateCombat)
- SKEAK leads to a roll to determined success
    - 50/50 for now, no stats involved
    - On success combat is avoided
    - On fail combat starts and enemy gets a free turn
- Combat is 1x1
    - An encounter with multiple enemies is tranformed into a queue
- Each turn, whoever has the highest SPD attacks first
    - Total SPD, including items
- During StateCombat, player can ATTACK or FLEE
- FLEE leads to a roll weighted by the difference in SPD between the player and the current enemy
    - FLEE chance is never higher than 95% and never lower than 5%
    - On success exits combat with no reward
    - On fail current enemy gets a free turn
- ATTACK trigger a normal attack
- When an enemy die mid combat the player gets the XP reward instantly
    - mid combat level ups are possible
- When all enemies die, the encounter ends with a victory for the player
    - Unlocks climbing: ASCEND to the next floor
    - Ascending also grants a random armor piece (helmet/body/boots) from
      the registry, auto-equipped on the spot (temporary reward; gold and
      proper item drops TODO)
- After combat games goes to StateExploring again
    - If combat was won, ASCEND is now permitted
- If the Hero's HP reachs zero, it dies
    - StateGameOver


## Combat calculations

### RNG

All combat randomness flows through the engine's injectable `rng` seam
(`Intn(n)`), so tests stay deterministic. The crypto-based `game.RollD*`
helpers are for non-engine use only (e.g. item durability).

- **Attack resolution (per strike):** no hit/miss roll — every attack lands.
    1. Damage spread: `max(1, DMG − 1 + Intn(3))` (±1 around the attack stat).
    2. Critical hit: crit if `Intn(100) < CritThreshold()` (a percentage),
       doubling the swung damage of that hit.
    - The Hero has a fixed 10% crit threshold; each monster has its own
      `crit` asset value (0 = never crits, validated in [0, 100]).
- **SNEAK:** 50/50 (`Intn(2)`), no stats involved.
- **FLEE:** chance = `50% + 5% × (hero SPD − enemy SPD)`, clamped to [5, 95].

### Defense

- Per hit, the defender mitigates the swung damage (spread + crit included)
  with a smooth percentage curve: mitigation = `DEF/(DEF+K)`, so the hit
  deals `swing × K/(DEF+K)`.
- `K` is the tunable `defCurveK` constant in `combat.go`, currently **45**:
  a single leather piece (DEF 5) mitigates ~10%, the full leather set
  (DEF 15) ~25%, and DEF == K exactly halves incoming damage. Lower `K`
  makes armor stronger, higher `K` weaker.
- Floored at 1: a landed hit is never fully nullified by armor.
- Applied engine-side on both directions (hero taking damage and monsters
  taking damage); `TakeDmg` itself stays a plain clamp. Pure math —
  consumes no rng. Content validation rejects negative DEF.

## Items

TODO

## Types of encounters

TODO