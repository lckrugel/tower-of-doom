# Proposed game mechanics(WIP)


## Game loop

- Game starts (StateExploring)
- Player can EXPLORE, ASCEND or INVENTORY
    - ASCEND starts blocked
- EXPLORE generates an encounter
    - The FIRST EXPLORE of a run is always COMBAT: the kind roll is
      skipped entirely (no rng consumed), so a run can never open with
      a sanctuary, chest or merchant
    - The encounter TYPE is rolled from the section's `encounter_types`
      weights (combat/sanctuary/chest/merchant; a section without the
      table is combat-only) — see "Types of encounters"
    - A third identical NON-combat kind in a row never happens: when the
      roll lands on the same non-combat kind as the previous two, it is
      overridden to combat (an override, NOT a reroll — the roll's rng
      consumption is unchanged) — see "Types of encounters"
    - COMBAT encounters keep the two-step flow (StateEncounter)
    - SANCTUARY encounters resolve instantly, on the spot (heal)
    - CHEST encounters roll gold-vs-item first (one extra rng roll):
      a GOLD chest pays out and exploring continues directly, an ITEM
      chest rolls an item and opens StateLoot: decide
      EQUIP LOOT / STASH / DISCARD — see "Items"
    - MERCHANT encounters open StateMerchant: BUY / SELL / LEAVE —
      see "Gold & prices" and "Types of encounters"
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
    - each level-up also heals 25% of the base max HP (no rng — see
      "Kill XP")
    - every defeat also pays `5 × enemy difficulty` gold (no rng)
- When all enemies die, the encounter ends with a victory for the player
    - Unlocks climbing: ASCEND to the next floor
    - Ascending also grants a random armor piece (helmet/body/boots) from
      the registry, restricted to the section's `item_tiers`, auto-equipped
      on the spot; whatever it displaced goes to the hero bag when there
      is room, else it is destroyed (gold and item tiers: see "Gold &
      prices" / "Item tiers")
- After combat games goes to StateExploring again
    - If combat was won, ASCEND is now permitted
- From StateExploring the bag can be opened (StateInventory): EQUIP /
  DROP / CLOSE — see "Items"; no bag access mid-combat
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
- `K` is the tunable `defCurveK` constant in `combat.go`, currently **35**:
  a single leather piece (DEF 5) mitigates ~12.5%, the full leather set
  (DEF 15) ~30%, and DEF == K exactly halves incoming damage. Lower `K`
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

Every level-up ALSO heals the hero by `25%` of the **base** max HP
(equipment excluded, floored, clamped by `Hero.Heal` — a full-health
hero gains nothing, and HP never exceeds the max), on top of the
unchanged %HP rescale that preserves the current HP as a fraction of
the pre-change max. The heal consumes no rng and rides the level-up
event as the ADDITIVE `healed` key (`event-level-up{level, healed}`):
consumers predating the key keep working.

## Items

Gear lives in six equipment slots (head, body, feet, weapon, ring,
amulet). Equipping/unequipping an item with an HP bonus preserves the
current HP as a fraction of the previous max (`rescaleHP`).

- **Hero bag** — bounded by `InventoryCapacity` = 10. `Hero.StashItem`
  appends at the end (insertion order preserved) and refuses when the bag
  is full: "bag full" is a player-facing condition, never an overflow.
- **Chest flow** — after the encounter-kind roll, ONE extra rng roll
  (`Intn(100) < 40`) decides gold-vs-item:
    - GOLD chest — pays `5 × floor difficulty` on the spot
      (`event-gold-gained`, source "chest") and returns to exploring
      directly: NO StateLoot decision for gold.
    - ITEM chest — rolls an item from the section's allowed tiers (sorted
      pool; rng roll unchanged apart from the gold check ahead of it) and
      holds it as `Engine.PendingLoot`, entering StateLoot. The player
      decides:
    - `EQUIP LOOT` — equips it; the displaced item goes to the bag when
      there is room, else it is destroyed (`event-item-lost`). Either
      way back to StateExploring with nothing pending. Chests still never
      unlock ASCEND.
    - `STASH` — stores it in the bag. While the bag is full the attempt
      is rejected (`event-bag-full`, NO state change: the decision stays
      open and the item stays pending).
    - `DISCARD` — destroys it on the spot.
- **Inventory (bag screen)** — reachable ONLY from StateExploring (no
  mid-combat bag access):
    - `EQUIP <index>` — swaps the bag item into its slot; the displaced
      item takes the swapped item's old bag position (the bag length is
      unchanged — an empty slot shrinks the bag by one).
    - `DROP <index>` — destroys the item on the spot. There is no floor
      layer: destroyed/dropped items are gone for good.
    - `CLOSE` — back to StateExploring; the bag keeps its contents.
    - Out-of-range/negative indexes are a generic invalid command with
      no state change, never a panic (player-reachable).
- **Ascend exception** — ascending still auto-equips the rolled armor
  reward (fast pacing by design); its displaced gear goes to the bag when
  there is room, else it is destroyed (`event-item-lost`).

New events (additive; no existing key changed meaning):
`event-loot-found{item}`, `event-item-equipped{item, swapped_from?}`
(`swapped_from` is absent when nothing was displaced),
`event-item-stashed{item}`, `event-item-discarded{item}`,
`event-item-lost{item}`, `event-bag-full{}` (no payload — the only
blocked command is the stash, and the pending item is visible as
`Engine.PendingLoot`).

## Item tiers

Every item sits on a progression ladder: `Item.Tier`, taken from the
asset's `"tier"` field. An absent field means tier 1 (JSON cannot tell
`"tier": 0` from an absent field), so the tier-less embedded assets and
older mods keep working.

- **Section gating** — a section may list `"item_tiers": [..]`; every
  item roll it makes (chest loot, ascend armor reward, merchant stock)
  draws from a pool restricted to those tiers. Absent/empty = NO
  restriction (mods keep rolling the whole registry). The pool is the
  registry's `ItemIDsForTiers` lookup: sorted ids, unrestricted when no
  tiers are listed, and an error-free empty result when nothing matches
  (an item chest then degrades to the "empty chest" message).
- **Merchant reach** — the merchant's tier+1 upgrade (below) is the one
  mechanism that deliberately EXCEEDS the section's tiers: the upgraded
  offer rolls registry-wide at the target tier.
- **Validation** (content bugs fail at startup): item tier < 1 is an
  error (absent/0 defaults to 1 at Load); `item_tiers` values < 1 are an
  error; an `item_tiers` entry with zero registry items of that tier is
  an error (typo protection); a section with a merchant weight but no
  stockable item in its allowed tiers is an error (tier+1 upgrades only
  REPLACE offers, so they cannot stock an empty shop).

## Gold & prices

- **Kill gold** — every defeat (mid-combat kills included) pays
  `5 × enemy difficulty` gold alongside the instant XP. The grant
  consumes no rng; reported as `event-gold-gained{amount, source:
  "kill"}`.
- **Chest gold** — a gold chest pays `5 × floor difficulty` (floor
  difficulty = `base_difficulty + difficulty_per_floor × floors into the
  section`); reported as `event-gold-gained{amount, source: "chest"}`.
- **Prices** — both derive from the item's `Value()` (bonuses ×50,
  durability-scaled; indestructible items are always at full value):
    - `BuyPrice` = full value — what a merchant charges.
    - `SellValue` = half the value, floored at 1 — what a merchant pays
      (even junk sells for something).
- **Merchant flow** — EXPLORE opens the shop (StateMerchant) with the
  offers held in `Engine.MerchantStock`:
    - `BUY <stock index>` — not enough gold: `event-message`, no change;
      full bag: `event-bag-full`, no change; success: the item goes to
      the bag at its `BuyPrice` and the offer leaves the stock
      (`event-item-bought{item, price}`). Shopping continues.
    - `SELL <bag index>` — the item is destroyed (the hero's existing
      bag drop) and its `SellValue` added to the purse
      (`event-item-sold{item, price}`). Shopping continues.
    - `LEAVE` — the remaining stock is discarded, back to exploring.
    - `QUIT` works as everywhere else. Merchants do NOT unlock ASCEND —
      only combat victories do.
- **Stock generation** — 4 offers rolled from the section's allowed item
  tiers (sorted pool; duplicates possible when the pool is smaller),
  then ONE `Intn(100) < 35` check: on fire, ONE `Intn(4)` picks which
  offer is replaced by a tier+1 item, capped at the registry's max tier
  (an empty replacement pool keeps the original offer). The exact rng
  order is a determinism contract, documented in `merchant.go`:
  4 offer rolls → `Intn(100)` → on upgrade `Intn(4)` → replacement-pool
  roll.

## Types of encounters

Each section defines weights per encounter kind in its `encounter_types`
table (`combat` / `sanctuary` / `chest` / `merchant`). The engine rolls
the KIND first (one `Intn` over the total weight; the roll order is
combat, sanctuary, chest, merchant — the merchant bucket was APPENDED
last, so a zero/absent merchant weight keeps every pre-merchant rng
sequence identical), then fills the section difficulty budget with
monsters only when the kind is combat. A section without the table (or
with every weight at 0) is combat-only and consumes no rng for the kind
roll, so old sections/mods keep their exact spawn behavior.

Two engine-level PACING rules shape the kind decision (they hold
regardless of frontend, and both are rng-neutral or rng-saving — never
an extra roll):

- **The run's first EXPLORE is always COMBAT.** The very first kind
  decision of a run skips the kind roll entirely (zero rng consumed), so
  a sanctuary, chest or merchant can never open a run. The flag is per
  run — ascending does not reset it — and every later explore rolls the
  kind table exactly as before the rule existed. The forced combat is
  the first entry of the history below.
- **No three identical non-combat kinds in a row.** The engine records
  the kinds of the last two resolved kind decisions; when a roll lands
  on a non-combat kind equal to BOTH of them, the result is overridden
  to combat. The override is not a reroll: the original roll's single
  `Intn` stays the only rng consumed, and a pair (two identical
  non-combat kinds in a row) remains possible. Only the rolled/resolved
  kinds feed this history — monster picks, chest gold rolls and merchant
  stock are never affected — and an override is recorded as what
  actually happened (combat), so it naturally breaks the streak.

- **COMBAT** — enemies spawn per the section spawn table:
    - The Hero can FIGHT (StateCombat) or SNEAK past it (50/50)
    - Victory unlocks ASCEND; sneaking/fleeing past does not
- **SANCTUARY** — a healing spot; resolves instantly on EXPLORE:
    - Heals `50%` of the Hero's **base** max HP (equipment excluded, floored)
    - Clamped by `Hero.Heal`: a Hero at full health gains nothing
    - Does NOT unlock ASCEND
- **CHEST OF RICHES** — resolves on EXPLORE:
    - ONE extra rng roll (`Intn(100) < 40`) decides gold-vs-item
    - GOLD — pays `5 × floor difficulty` (`event-gold-gained`, source
      "chest") and exploring continues directly: no StateLoot decision
    - ITEM — rolls a random item from the section's allowed tiers (any
      slot, sorted-ID pool; the same roll as before, now tier-gated) and
      holds it as `Engine.PendingLoot`, entering StateLoot — see "Items"
    - Does NOT unlock ASCEND
- **MERCHANT** — a wandering shop; opens StateMerchant on EXPLORE:
    - Stocks 4 offers from the section's allowed item tiers; one offer
      may be upgraded to a tier+1 item (35%, capped at the registry's
      max tier) — see "Gold & prices"
    - BUY / SELL / LEAVE (and QUIT); buying and selling never unlock
      ASCEND
    - Does NOT unlock ASCEND

Only a combat victory clears the floor path and unlocks ASCEND.