# Proposed game mechanics(WIP)


## Game loop

- Game starts (StateExploring)
- Player can EXPLORE or ASCEND
    - ASCEND starts blocked
- EXPLORE generates an encounter
    - The encounter TYPE is rolled from the section's `encounter_types`
      weights (combat/sanctuary/chest; a section without the table is
      combat-only) — see "Types of encounters"
    - COMBAT encounters keep the two-step flow (StateEncounter)
    - SANCTUARY and CHEST encounters resolve instantly, on the spot
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

### Kill XP

Defeating a monster grants `round(4 · difficulty^1.5)` XP instantly
(mid-combat kills can trigger level-ups on the spot). The exponent
mirrors the hero level-cost curve (`100 · max(Lvl−1, 1)^1.5`), which
keeps the time to level flat per floor tier instead of ballooning as
content difficulty rises: a goblin (difficulty 3) pays 21 XP, an orc
warlord (12) pays 166, an ancient red dragon (40) pays 1012.

## Items

TODO

## Types of encounters

Each section defines weights per encounter kind in its `encounter_types`
table (`combat` / `sanctuary` / `chest`). The engine rolls the KIND first
(one `Intn` over the total weight; the roll order is combat, sanctuary,
chest), then fills the section difficulty budget with monsters only when the
kind is combat. A section without the table (or with every weight at 0) is
combat-only and consumes no rng for the kind roll, so old sections/mods keep
their exact spawn behavior.

- **COMBAT** — enemies spawn per the section spawn table:
    - The Hero can FIGHT (StateCombat) or SNEAK past it (50/50)
    - Victory unlocks ASCEND; sneaking/fleeing past does not
- **SANCTUARY** — a healing spot; resolves instantly on EXPLORE:
    - Heals `35%` of the Hero's **base** max HP (equipment excluded, floored)
    - Clamped by `Hero.Heal`: a Hero at full health gains nothing
    - Does NOT unlock ASCEND
- **CHEST OF RICHES** — resolves instantly on EXPLORE:
    - Grants a random item from the registry (any slot, sorted-ID roll),
      auto-equipped on the spot; whatever it replaced is gone (there is no
      inventory yet)
    - Does NOT unlock ASCEND

Only a combat victory clears the floor path and unlocks ASCEND.