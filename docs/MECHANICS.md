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
      WEAR / STASH / DISCARD — see "Items"
    - MERCHANT encounters open StateMerchant: BUY / SELL / LEAVE —
      see "Gold & prices" and "Types of encounters"
- PLayer can FIGHT or SNEAK
- FIGHT leads to combat (StateCombat)
    - Entering combat resolves NO round: rounds advance from the hero's
      move choices, so engaging consumes no rng
- SKEAK leads to a roll to determined success
    - 50/50 for now, no stats involved
    - On success combat is avoided
    - On fail combat starts and enemy gets a free turn
- Combat is 1x1
    - An encounter with multiple enemies is tranformed into a queue
- Each turn, whoever has the highest SPD attacks first
    - Total SPD, including items
- During StateCombat, player can MOVE (a move choice) or FLEE
- FLEE leads to a roll weighted by the difference in SPD between the player and the current enemy
    - FLEE chance is never higher than 95% and never lower than 5%
    - On success exits combat with no reward
    - On fail current enemy gets a free turn
- MOVE uses the hero move at the chosen index of the hero's combat move
    set (learned moves, then weapon-granted; a move-less hero fights
    with the canonical basic_attack) — see "Combat 2.0 — decided design"
- When an enemy die mid combat the player gets the XP reward instantly
    - mid combat level ups are possible
    - each level-up also heals 25% of the base max HP (no rng — see
      "Kill XP")
    - every defeat also pays `5 × enemy difficulty` gold (no rng)
- When all enemies die, the encounter ends with a victory for the player
    - Unlocks climbing: ASCEND to the next floor
    - Ascending also rolls a random armor piece (helmet/body/boots) from
      the registry, restricted to the section's `item_tiers`, and opens
      the SAME loot decision as a chest: WEAR / STASH / DISCARD — see
      "Items" (gold and item tiers: see "Gold & prices" / "Item tiers")
- After combat games goes to StateExploring again
    - If combat was won, ASCEND is now permitted
- From StateExploring the bag can be opened (StateInventory): WEAR /
  DROP / CLOSE — see "Items"; no bag access mid-combat
- If the Hero's HP reachs zero, it dies
    - StateGameOver


## Combat calculations

### RNG

All combat randomness flows through the engine's injectable `rng` seam
(`Intn(n)`), so tests stay deterministic. The crypto-based `game.RollD*`
helpers are for non-engine use only (e.g. item durability).

- **Attack resolution (per strike):** no hit/miss roll — every attack
  lands. The strike is made with a MOVE (the hero picks one; the monster
  picks by AI, below):
  1. Damage base: `round(effective DMG × dmg_modifier / 100)` — pure
     math, no rng (the modifier is a percent; 100 = neutral).
  2. Damage spread: `max(1, base − 1 + Intn(3))` (±1 around the base).
  3. Critical hit: crit if
     `Intn(100) < clamp(CritThreshold() + move.crit_bonus, 0, 100)`,
     doubling the swung damage of that hit.
  - The Hero has a fixed 10% crit threshold; each monster has its own
    `crit` asset value (0 = never crits, validated in [0, 100]).
- **Monster move AI (v1):** each monster turn, the engine filters the
  monster's move set to the moves NOT on cooldown and picks ONE `Intn`
  over the filtered total weight (the per-monster ref weights, resolved
  at spawn). Filtering consumes no rng; when every move is locked the
  monster falls back to the canonical basic attack WITHOUT a pick roll —
  there is nothing left to pick.
- **Move cooldowns:** using a move locks it for exactly its `cooldown`
  full rounds after the round of use (0 = spammable). Locks tick at
  round end and never survive the encounter. A fully-cooled hero turn
  resolves as an automatic RECOVER round (a message, the enemy acts,
  ticks run) — never a soft-lock; picking one locked move while another
  is ready is only a blocking message with no state change.
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
- **WEAR — one command, two states** — moves an item into its equipment
  slot; the emission is identical in both states
  (`event-item-equipped{item, swapped_from?}`, `swapped_from` ABSENT when
  nothing was displaced):
    - StateLoot (pending loot) — wears `Engine.PendingLoot` (`ItemIndex`
      ignored); the displaced item goes to the bag when there is room,
      else it is destroyed (`event-item-lost`). Either way back to
      StateExploring with nothing pending. Chests never unlock ASCEND.
    - StateInventory (bag screen) — wears the bag item at `ItemIndex`;
      the displaced item takes the worn item's old bag position (the bag
      length is unchanged — an empty slot shrinks the bag by one). The
      bag stays open.
    - Out-of-range/negative indexes are a generic invalid command with
      no state change, never a panic (player-reachable); in StateLoot the
      index is ignored instead.
- **Chest flow** — after the encounter-kind roll, ONE extra rng roll
  (`Intn(100) < 40`) decides gold-vs-item:
    - GOLD chest — pays `5 × floor difficulty` on the spot
      (`event-gold-gained`, source "chest") and returns to exploring
      directly: NO StateLoot decision for gold.
    - ITEM chest — rolls an item from the section's allowed tiers (sorted
      pool; rng roll unchanged apart from the gold check ahead of it) and
      holds it as `Engine.PendingLoot`, entering StateLoot. The player
      decides:
      - `WEAR` — see above (pending-loot case).
      - `STASH` — stores it in the bag. While the bag is full the attempt
        is rejected (`event-bag-full`, NO state change: the decision
        stays open and the item stays pending).
      - `DISCARD` — destroys it on the spot.
- **Inventory (bag screen)** — reachable ONLY from StateExploring (no
  mid-combat bag access):
    - `WEAR <index>` — see above (bag-screen case).
    - `DROP <index>` — destroys the item on the spot. There is no floor
      layer: destroyed/dropped items are gone for good.
    - `CLOSE` — back to StateExploring; the bag keeps its contents.
- **Ascend flow** — there is NO auto-equip anymore: ascending rolls the
  armor reward from the NEW floor's section and opens the very same
  StateLoot decision (WEAR / STASH / DISCARD — the pending-loot case
  above). An empty armor pool degrades like an empty chest: a message
  event and exploring continues (no loot decision).

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
- **Stock generation** — up to 4 DISTINCT offers rolled from the
  section's allowed item tiers: each pick removes the item's id from the
  pool before the next roll (one `Intn` per offer over the shrinking
  pool — fewer offers only when the pool runs dry; never duplicates),
  then ONE `Intn(100) < 35` check: on fire, ONE `Intn` over the stock
  length picks which offer is replaced by a tier+1 item, capped at the
  registry's max tier and EXCLUDING the ids already in stock (an empty
  filtered pool keeps the original offer). The exact rng order is a
  determinism contract, documented in `merchant.go`: one offer roll per
  offer → `Intn(100)` → on upgrade `Intn(stock length)` →
  replacement-pool roll.

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
    - Stocks up to 4 DISTINCT offers from the section's allowed item
      tiers (the pool shrinks per pick); one offer may be upgraded to a
      tier+1 item (35%, capped at the registry's max tier, never
      duplicating the stock) — see "Gold & prices"
    - BUY / SELL / LEAVE (and QUIT); buying and selling never unlock
      ASCEND
    - Does NOT unlock ASCEND

Only a combat victory clears the floor path and unlocks ASCEND.

## Combat 2.0 — decided design

The decisions below are LOCKED. Implementation lands across the phases
listed at the end; the game-loop and combat sections above will be
rewritten as each phase ships.

### Bosses

- ONE Boss Encounter per section, fought at the END of the section (the
  section's `max_floor`): the goblin pits' boss on floor 3, the
  outskirts' boss on floor 13, the underdark's on 33, wraithfall's on 59.
- Boss groups are PREDEFINED in the section assets: fixed monster
  lists — no weights, no difficulty budget.
- Progression:
    - On any normal floor, winning a combat unlocks ASCEND (unchanged).
    - On a section's final floor, winning a normal combat unlocks the
      BOSS CHALLENGE (a dedicated command) instead of ASCEND; killing
      the boss is what unlocks ASCEND into the next section.
- Boss fights cannot be SNEAKED: the challenge goes straight to combat.
- FLEEING the boss re-locks the challenge (another normal-combat victory
  re-unlocks it) and never unlocks ASCEND.
- Boss loot: an item from tier+1 vs the section (capped at the
  registry's max tier) plus a gold bonus, presented as the standard
  loot decision (WEAR / STASH / DISCARD).
- The infinite dragon_hoard tail (`max_floor: -1`) has NO boss gate.
- Boss challenges skip every encounter-kind roll (zero rng consumed).

### Moves

- A Move is a new asset (`moves.json`): `id`, `name`, `tier`,
  `dmg_modifier` (percent; 100 = neutral — DMG-only scaling),
  `crit_bonus` (percentage points), `cooldown` (turns; 0 = spammable),
  optional `status {id, chance, target: enemy|self}`, and `weight`
  (monster AI).
- Each combatant has AT MOST 4 moves (`MAX_MOVES`).
- Monsters pick moves by weight (AI v1; a decision tree may come later).
  A monster without a `moves` field uses the canonical `basic_attack`
  (cooldown 0), so older assets/mods keep working.
- Hero moves: 2 LEARNED (via shrines, below) plus up to 2 granted by
  the equipped weapon (a `moves` field on weapon assets).
- `CmdMove(index)` replaces ATTACK: every combat turn is a move choice.
- A move on cooldown is locked for its cooldown's turns (ticked at
  round end). A fully-cooled hero turn resolves as an automatic
  RECOVER turn (statuses tick, monsters act) — never a soft-lock.
- Damage: `base = round(effective DMG × dmg_modifier / 100)`, then the
  usual spread and crit math; the crit check gains the move's
  `crit_bonus`.

### Status effects

- A Status is a new asset (`statuses.json`): `id`, `name`,
  `kind` (buff/debuff), `effect` (DoT damage per turn, or a stat
  modifier for DMG/DEF/SPD), `duration` (turns). Moves apply them
  (target: enemy or self, with an application chance).
- Active effects are COMBAT-SCOPED: tracked per combatant with turns
  left, ticked at round end, refreshed to the longest duration (never
  stacked), and cleared when combat ends.
- Effective stats = base + gear + active status modifiers, clamped
  at 0 (DMG at 1). DoT damage CAN kill.
- Starter set: poison (DoT), weaken (−DMG), slow (−SPD), guard
  (+DEF, self).

### Shrines (move learning)

- A fifth encounter kind (`shrine`): offers a random UNLEARNED move
  drawn from the section's `move_tiers` (the `item_tiers` pattern:
  absent = unrestricted).
- Learning is capped at 2 learned moves: at the cap, the shrine asks
  the player to FORGET one first.
- `move_tiers` gate shrine offers; weapon-granted moves are gated by
  the item's own tier gating.

### Moves & statuses — data model

The schemas below are the DATA LAYER of this design (shipped in phase
1); combat-flow documentation lands with the integration phase.

**Move asset** (`moves.json`). Only `tier` has an absent-field default
(`0` → 1, mirroring items — JSON cannot tell them apart); every other
field is required as authored and validated at startup:

- `id`, `name` — identity.
- `tier` — progression ladder (1 = earliest); absent/0 → 1.
- `dmg_modifier` — percent scaling of the striker's DMG (100 = neutral);
  validated to [50, 300].
- `crit_bonus` — percentage points added to the striker's crit
  threshold for this move; may be negative (the effective threshold is
  clamped to [0, 100] at use time).
- `cooldown` — turns the move stays locked after use (0 = spammable);
  validated ≥ 0.
- `status` — optional `{id, chance, target}`: the status applied on
  hit. `chance` is a percentage validated to [0, 100]; `target` is
  `enemy` (the hit target) or `self` (the striker). The `id` must exist
  in statuses.json.
- `weight` — the move's default AI weight (monster picks); ≥ 1.

**Status asset** (`statuses.json`). Every field is required — statuses
have no absent-field defaults:

- `id`, `name` — identity.
- `kind` — `buff` or `debuff`.
- `effect` — one of `{type: dot, amount ≥ 1}` (damage per turn) or
  `{type: stat, stat: dmg|def|spd, amount ≠ 0}`.
- `duration` — turns the status lasts; ≥ 1.

**Where moves attach:**

- `MonsterAsset.moves` — a list of `{id, weight}` refs: at most 4
  entries, weights ≥ 1, ids must exist. The ref's weight is the
  MONSTER's own AI weight for that move — it overrides the move asset's
  `weight` when the engine resolves the set. ABSENT or empty → the
  monster fights with the canonical `basic_attack`, so older assets and
  mods keep working unchanged.
- `ItemAsset.moves` — a list of move ids. ONLY weapons may grant moves;
  at most 2 per weapon (`MaxWeaponMoves`); every id must exist. The
  engine resolves the ids into runtime moves when the item is built, so
  the hero's set is always computed from the EQUIPPED weapon (unwearing
  it removes the grants).
- `SectionAsset.move_tiers` — the exact `item_tiers` pattern: it gates
  which move tiers the section can offer (shrine draws, phase 4).
  Absent or empty = unrestricted; every listed tier must have at least
  one registry move (typo protection).

**Registry queries** (mirroring the item ones): `MovesForTiers(tiers)`
(move assets sorted by id; unrestricted when no tiers are listed; an
empty pool is never an error), `MoveByID`, `StatusByID`, `MaxMoveTier`.

**Hero move set** — `Hero.EffectiveMoves(weapon)`: the LEARNED moves
first (learning order, capped at 2 — `MaxLearnedMoves`), then the
equipped weapon's granted moves, deduplicated by id (the learned copy
wins), capped at 4 (`MaxMoves`) — weapon moves beyond the cap are
dropped, so the set never exceeds 4. The canonical `basic_attack` is
NOT implicitly added to the hero set: a hero with no learned moves and
no weapon-granted moves has an empty set, and what that means in combat
is decided by the combat integration. Learning: `LearnMove` refuses at
the cap (learning never displaces a move on its own — the caller drives
`ForgetMove(moveID)` and retries); learning an already-known id is a
refused no-op.

**Monster move set** — built by the engine from the asset's refs at
spawn time (`NewMonsterFromAsset(asset, moves...)`; no moves passed =
the canonical basic attack, matching the asset-level fallback). The
runtime `Move.Weight` carries the ref's per-monster AI weight.

**Load-time validation** (content bugs fail at startup, house style):

- move: tier < 1 (absent/0 defaults to 1 at Load); `dmg_modifier`
  outside [50, 300]; cooldown < 0; weight < 1; a `status` with chance
  outside [0, 100], an unknown `target` word, or an unknown status id.
- status: kind not buff/debuff; effect type not dot/stat; dot amount
  < 1; stat amount = 0 or an unknown stat word; duration < 1.
- monster: more than 4 moves, an unknown move id, a ref weight < 1.
- weapon: more than 2 moves or an unknown move id; any non-weapon slot
  carrying moves.
- section: `move_tiers` values < 1, or a listed tier with zero registry
  moves of that tier.
- duplicate ids in moves.json / statuses.json fail at Load (indexBy).

### Implementation phases

1. Schema & entities — moves/statuses assets, move sets, validation
2. Combat integration — CmdMove, monster AI, cooldowns, status ticks,
   stopgap UI
3. TUI combat redesign — 2×2 move grid, status chips, cooldown display
4. Bosses & shrines — gate flow, move learning, content, balance retune