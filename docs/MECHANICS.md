# Proposed game mechanics(WIP)


## Game loop

- Game starts (StateExploring)
- Player can EXPLORE, ASCEND or INVENTORY
    - ASCEND starts blocked
    - While the BOSS CHALLENGE is unlocked (see "Bosses"), the player
      can also CHALLENGE the section's boss
- EXPLORE generates an encounter
    - The FIRST EXPLORE of a run is always COMBAT: the kind roll is
      skipped entirely (no rng consumed), so a run can never open with
      a sanctuary, chest, merchant or shrine
    - The encounter TYPE is rolled from the section's `encounter_types`
      weights (combat/sanctuary/chest/merchant/shrine; a section
      without the table is combat-only) — see "Types of encounters"
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
    - SHRINE encounters offer ONE or TWO moves and open StateLearn:
      CHOOSE one of the offers — under the learned cap it is learned on
      the spot; at the cap the forgetting step follows: FORGET one of
      one learned move or DECLINE — see "Shrines"
- PLayer can FIGHT or SNEAK
- FIGHT leads to combat (StateCombat)
    - Entering combat resolves NO turn: turns advance from the hero's
      move choices, so engaging consumes no rng
- SKEAK leads to a roll to determined success
    - 50/50 for now, no stats involved
    - On success combat is avoided immediately, with no combat ticks
    - On fail a fresh encounter starts and consumes one enemy-only action turn
- Combat is 1x1
    - An encounter with multiple enemies is tranformed into a queue
- An **action** is one combatant using a move, drinking a potion, or
  forfeiting its action opportunity. A **turn** contains both action
  opportunities and end-of-turn processing.
- At turn start, capture the original hero/current-monster pair and their
  initiative using EFFECTIVE SPD (base + gear + active stat modifiers;
  hero wins ties). This order governs actions AND DoT recipients for the
  whole turn; same-turn speed changes cannot reorder either.
    - A combatant killed by an action cannot act. A queued replacement
      enters only after finalization and cannot act or tick until next turn.
    - Killing the monster never skips a surviving hero's turn-end processing.
- During StateCombat, player can MOVE (a move choice) or FLEE
- FLEE leads to a roll weighted by the difference in SPD between the player and the current enemy
    - FLEE chance is never higher than 95% and never lower than 5%
    - On success exits combat immediately with no reward or combat ticks
    - On fail consumes one turn with only the current enemy's action
- MOVE uses the hero move at the chosen index of the hero's combat move
  set (weapon's basic action first, then learned cooldown moves in
  learning order; a weaponless hero gets virtual Unarmed Punch)
    - A move on cooldown is a blocked action: a message, no state
      change; when EVERY move is locked the turn resolves as an
      automatic RECOVER turn (never a soft-lock)
- Statuses are ENCOUNTER-SCOPED: applied by moves (with an application
    chance), never stacked, and cleared when the encounter ends or is
    exited. Hero statuses persist across its monster queue — see "Status effects".
- Enemy kills grant XP and gold only after DoTs and turn bookkeeping,
  provided the hero survives; only then can the queue advance or victory occur
    - mid-combat level ups are possible, but cannot rescue lethal turn-end DoTs
    - each level-up also heals 25% of the base max HP (no rng — see
      "Kill XP")
    - every defeat also pays `5 × enemy difficulty` gold (no rng)
- When all enemies die, the encounter ends with a victory for the player
    - On a NORMAL floor: unlocks climbing — ASCEND to the next floor
    - On a section's FINAL floor (a section with a boss): unlocks the
      BOSS CHALLENGE instead — the gate opens only when the boss falls
      (see "Bosses")
    - Ascending also rolls a random armor piece (helmet/body/boots) from
      the registry, restricted to the section's `item_tiers`, and opens
      the SAME loot decision as a chest: WEAR / STASH / DISCARD — see
      "Items" (gold and item tiers: see "Gold & prices" / "Item tiers")
- After combat games goes to StateExploring again
    - If combat was won, ASCEND is now permitted — EXCEPT on the final
      floor of a section with a boss, where the challenge unlocks and
      ASCEND stays locked until the boss falls (the two-regime rule)
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
- **Status application (per status effect):** ONE `Intn(100)` in authored
  effect order — fires when the roll is < the effect's status chance. The
  roll happens even when it fails (and even at chance 0), and even when
  the recipient is dead; dead recipients are not afflicted. The per-strike
  order is [monster move pick] → [spread] → [crit] → [status chances in
  authored order].
- **End-of-turn processing:** every consumed turn, including killing
  turns, follows the timing contract below. DoTs use captured initiative
  recipient order, not a fixed hero-first order. Stat expiration and
  cooldown bookkeeping run once even if lethal damage stops DoTs early.
- **Effective stats:** every stat read in combat (strike damage,
  mitigation, initiative, the flee formula) starts from base+gear. Active
  status percentages are summed and applied once (floored) to that baseline;
  summed flat modifiers are then added. Stat percentages do not compound.
  Results clamp — DMG at 1, DEF/SPD at 0. Pure math — no rng.
- **Move cooldowns:** using a move locks it for exactly its `cooldown`
  subsequent full turns after the turn of use (0 = spammable). Existing
  locks advance once at every consumed turn end, including killing turns;
  moves actually used then receive their full lock (never skipped actions).
  A cooldown-2 move used in turn 1 is locked in turns 2–3 and ready in
  turn 4. Locks never survive the encounter. A fully-cooled hero turn
  resolves as an automatic RECOVER turn (a message, the enemy acts,
  ticks run) — never a soft-lock; picking one locked move while another
  is ready is only a blocking message with no state change.
- **Read-only combat view:** `Engine.CombatView()` is a value-copy snapshot
  for rendering. Actor side and monster ID are identity; display names are
  narrative only. Moves/statuses carry registry IDs and live cooldown or
  duration values, and actor stats are effective combat stats. Events remain
  responsible for narrative and logs, not reconstructing combat timing.
- **Event payload ownership:** `Event.Data` is a fresh map, but pointer and
  slice entries intentionally reference live engine objects/storage. Treat
  them as read-only notifications, not immutable history or replay records.
- **SNEAK:** 50/50 (`Intn(2)`), no stats involved.
- **FLEE:** chance = `50% + 5% × (hero SPD − enemy SPD)`, clamped to [5, 95].

### Turn timing and outcomes

1. Capture the original combatants and effective-SPD initiative once.
2. Resolve eligible actions in that order. Finish a selected move's effects
   in authored order before deciding whether the other combatant can act:
   a killing move's self-heal still happens here, before DoTs. A monster
   unable to act consumes neither move-pick nor attack RNG.
3. Process living recipients' DoTs in captured initiative order, each
   recipient's statuses in application order. Skip recipients already
   killed by actions. The **first lethal tick settles defeat and stops all
   further DoT damage**; pending ticks cannot reverse the outcome.
4. Perform non-damage status expiration and cooldown bookkeeping exactly
   once, even on killing turns or after an early lethal-tick stop.
5. If the hero survives, settle each enemy defeat once: grant XP, kill
   gold and level-up healing, then any final boss rewards/progression in
   their existing order. Only then advance the queue or declare victory.

DoT order is **recipient order**, not caster action order. If both actors
survive the actions but both face lethal ticks, hero-first initiative
means the hero dies and the monster's tick is skipped; monster-first means
the monster dies and the hero's tick is skipped. **At equal SPD, the hero
recipient is processed first and loses this lethal-DoT tie.** If the first
recipient survives, process the second normally. Skipped DoTs on a
surviving hero retain their unconsumed ticks across the queued enemies.

Direct hero death is irreversible: no later action or DoT damage runs,
and no pending monster DoT or XP can rescue the hero. If an action kills
the monster but a subsequent hero DoT is lethal, the outcome is game over:
no kill rewards, victory, boss loot, or progression unlock. This also
applies to the final monster and boss.

Move use, combat potion use, automatic recover, failed flee, and failed
sneak all consume one turn through this same finalization path. Failed
sneak first starts a fresh encounter. Fight/challenge entry, rejected
commands (including blocked moves or potions), inspection, UI navigation,
and out-of-combat commands do not advance combat time. Successful sneak
or flee is the deliberate exception: immediate encounter exit, without
actions or end-of-turn ticks; escape chances and costs are unchanged.

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

### Status effects

- Status effects support flat `amount`, percentage `percent`, or both.
  Stat percentages use the equipment-inclusive baseline before statuses;
  multiple percentages add, then flat modifiers apply afterward.
- DoT is evaluated at each end-of-turn tick from the target's current
  equipment-inclusive MaxHP: `amount + floor(MaxHP × percent / 100)`.
  Percentage DoTs are uncapped and keep scaling with target MaxHP; flat
  damage remains additive. A positive DoT deals at least 1 damage before
  the remaining-HP clamp. DoT bypasses defense and can kill.
- Stat buffs AND debuffs apply immediately and last through `duration`
  **subsequent full turns**. The partial application turn does not consume
  one. Immediate modifiers can affect the remaining action, but not the
  initiative already captured for actions and DoTs.
- DoT `duration` instead counts **damage ticks**, starting at application
  turn end if the recipient is reached. Only an actual tick consumes a
  tick; stopping at an earlier lethal recipient does not consume skipped
  ticks. DoTs gain no extra tick from stat-effect lifetime rules.
- Same-ID effects never stack: stat reapplication keeps the later expiration
  deadline (a shorter refresh cannot postpone an already-later deadline);
  DoT reapplication keeps the larger of remaining and fresh tick counts.
  Different IDs combine independently. Hero statuses persist between
  queued monsters, but all statuses clear at encounter end or exit.
- Live status displays distinguish future full turns from DoT ticks;
  rendering and inspection never advance either lifetime.

Duration-2 timeline (assuming the recipient survives and its DoTs are processed):

| Turn | Stat buff/debuff, duration 2 | DoT, duration 2 |
| --- | --- | --- |
| 1: application | Active immediately; two future full turns remain | First tick; one remains |
| 2 | Active; one future full turn remains | Second tick; expires |
| 3 | Active; expires at turn end | Absent |
| 4 | Absent | Absent |

Balance impact (no asset retuning): Guard's duration 2 now includes two
future full turns; Breachfire's duration 3 supports three future attacking
turns if combat continues. Other stat buffs/debuffs follow the same rule;
DoTs retain their authored tick counts. Any compensation is a separate
balance change.

### Kill XP

Defeating a monster grants `round(4 · difficulty^1.5)` XP after turn-end
DoTs and bookkeeping, only if the hero survives (mid-combat level-ups
are still possible before the next queued monster). The exponent
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

For example, a hero at 3 HP kills a monster whose XP would level them up,
but has a pending 3-damage poison tick. Poison kills the hero before XP
is granted: no level-up heal or reward occurs. A non-lethal tick instead
resolves before XP and healing. Percentage DoTs use the current MaxHP
before any deferred level-up changes it.

## Items

Gear lives in six equipment slots (head, body, feet, weapon, ring,
amulet). Equipping/unequipping an item with an HP bonus preserves the
current HP as a fraction of the previous max (`rescaleHP`), floored and
clamped to the new maximum. Equipment changes, including replacements,
keep a previously living hero at least at 1 HP (when the new max is positive),
so rounding cannot kill them. Dead heroes are not resurrected. This minimum
is equipment-specific; level-up rescaling and its subsequent heal are unchanged.

- **Hero bag** — bounded by `InventoryCapacity` = 10. `Hero.StashItem`
  appends at the end (insertion order preserved) and refuses when the bag
  is full: "bag full" is a player-facing condition, never an overflow.
- **WEAR — one command, two states** — moves an item into its equipment
  slot; the emission is identical in both states
  (`event-item-equipped{item, swapped_from?}`, `swapped_from` ABSENT when
  nothing was displaced):
    - StateLoot (pending loot) — wears `Engine.PendingLoot` (`Index`
      ignored); the displaced item goes to the bag when there is room,
      else it is destroyed (`event-item-lost`). Either way back to
      StateExploring with nothing pending. Chests never unlock ASCEND.
    - StateInventory (bag screen) — wears the bag item at `Index`;
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
- **Consumables** — separate from the equipment bag and learned moves:
    - The shared pouch holds at most 3 units across all consumable types.
    - The Healing Potion restores `25%` of the Hero's base max HP (equipment
      excluded), floored and clamped. It cannot be used at full HP.
    - `USE CONSUMABLE <index>` selects a raw carried-unit index into
      `Hero.Consumables()` (default 0 for existing callers).
      `Engine.ConsumableAt(index)` queries that unit's usable registry
      definition. The TUI groups equal IDs into stacks in first-carried order
      and sends the first raw matching index, not the stack row index.
      The selected definition determines healing; no particular ID is required.
      Removing a unit preserves the order of all other carried units.
      Invalid indexes/definitions and full-health attempts spend no item,
      turn, or RNG. In StateExploring or
      StateInventory it is turn-free; in combat it consumes the Hero's
      action and resolves at normal initiative. The enemy still acts, and
      end-of-turn DoTs and bookkeeping run as usual. If the enemy kills the
      Hero before their initiative, the potion is not consumed.
    - The TUI shows the pouch count and potion effect; its action is
      available in combat and exploration/inventory only when relevant.
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
  `5 × enemy difficulty` gold alongside XP after turn-end processing,
  provided the hero survives. The grant
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
- **Durability status — unfinished:** items have durability fields and
  `Item.Use()` contains a dormant wear calculation, but the engine does not
  call it. Generated items start at maximum durability and gameplay does not
  currently consume durability. Do not activate wear by adding incidental
  calls; it needs an intentional mechanic design.
- **Durability status — unfinished:** items have durability fields and
  `Item.Use()` contains a dormant wear calculation, but the engine does not
  call it. Generated items start at maximum durability and gameplay does not
  currently consume durability. Do not activate wear by adding incidental
  calls; it needs an intentional mechanic design.
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
table (`combat` / `sanctuary` / `chest` / `merchant` / `shrine`). The
engine rolls the KIND first (one `Intn` over the total weight; the roll
order is combat, sanctuary, chest, merchant, shrine — the merchant
bucket was APPENDED last in its day and the shrine bucket followed it
the same way, so a zero/absent merchant or shrine weight keeps every
earlier rng sequence identical), then fills the section difficulty
budget with monsters only when the kind is combat. A section without
the table (or with every weight at 0) is combat-only and consumes no
rng for the kind roll, so old sections/mods keep their exact spawn
behavior.

Two engine-level PACING rules shape the kind decision (they hold
regardless of frontend, and both are rng-neutral or rng-saving — never
an extra roll):

- **The run's first EXPLORE is always COMBAT.** The very first kind
  decision of a run skips the kind roll entirely (zero rng consumed), so
  a sanctuary, chest or merchant can never open a run. The flag is per
  run — ascending does not reset it — and every later explore rolls the
  kind table, adjusted by the drought protection below. The forced combat
  counts as a missed visit for each available non-combat kind and is the
  first entry of the history below.
- **No three identical non-combat kinds in a row.** The engine records
  the kinds of the last two resolved kind decisions; when a roll lands
  on a non-combat kind equal to BOTH of them, the result is overridden
  to combat. The override is not a reroll: the original roll's single
  `Intn` stays the only rng consumed, and a pair (two identical
  non-combat kinds in a row) remains possible. Only the rolled/resolved
  kinds feed this history — monster picks, chest gold rolls and merchant
  stock are never affected — and an override is recorded as what
  actually happened (combat), so it naturally breaks the streak.
- **Unlucky protection gradually raises encounter weights.** For each
  non-combat kind with a positive weight in the current section, the engine
  tracks eligible exploration encounters since that kind was last visited.
  The first four misses leave its authored weight unchanged; after that,
  each additional miss adds 10% of its authored weight (rounded up). There
  is no gameplay cap or forced selection: even a long drought remains
  possible, but increasingly unlikely. Visiting a kind resets its counter;
  a sanctuary counts even at full health. Kinds with zero weight do not
  accrue misses, and counters are preserved across ascension. Counters use
  the encounter that actually happened after the no-three-identical rule,
  not an overridden roll. Ordinary selection still consumes one kind-roll
  `Intn`.

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
    - Also offers at most one Healing Potion for 100 gold per visit; buying
      removes that offer until the next merchant. Pouch capacity is shared
      across consumables and is not replenished automatically.
    - BUY / SELL / LEAVE (and QUIT); buying and selling never unlock
      ASCEND
    - Does NOT unlock ASCEND
- **SHRINE** — a teacher of moves; resolves on EXPLORE (see "Shrines"
    in the Combat 2.0 section for the full flow):
    - Draws ONE or TWO random unlearned moves from the section's
      `move_tiers` (ONE `Intn` PER offer over the pool SHRINKING per
      pick — the offers never duplicate) and opens StateLearn's
      choosing step: CHOOSE one — under the 3-learned cap it is learned
      on the spot; at the cap the forgetting step asks which learned
      move to forget
    - Does NOT unlock ASCEND

Only a combat victory clears the floor path and unlocks ASCEND.

## Combat 2.0 — implemented design

Moves, statuses and the combat flow below are IMPLEMENTED (phases 1-2
of the plan at the end are shipped; the stopgap TUI ships with them and
the game-loop/combat sections above describe the implemented reality).
**Bosses** are IMPLEMENTED too (the gate flow, the asset block, the
embedded groups and the TUI) — their section below documents the shipped
behavior. **Shrines** are IMPLEMENTED (the fifth encounter kind, the
TWO-offer flow with its two-step learning — CHOOSE, then FORGET at the
cap — the learning/forget rules and the TUI screen) — their section
below documents the shipped behavior.

### Bosses (IMPLEMENTED)

- ONE Boss Encounter per section, fought at the END of the section: the
  floor where `Registry.IsSectionEnd(floor)` holds (`max_floor == floor`
  of the section covering the floor). Sections resolve FIRST-MATCH
  (`SectionForFloor`): a floor two sections claim belongs to the
  EARLIER one — the embedded content's floor 3 is the goblin pits' end
  (its boss is the pits'), not the outskirts' first floor.
- Boss groups are PREDEFINED in the section assets (`"boss": {monsters,
  gold}`): fixed monster lists — no weights, no difficulty budget —
  fought in the authored order, each monster's move refs resolved
  exactly like a regular spawn's. A group of 1-3 monsters referencing
  existing ids; content validation rejects an empty list, an unknown
  monster id and a negative gold bonus (absent/0 = no bonus).
- Progression (the two-regime rule):
    - On any normal floor, winning a combat unlocks ASCEND (unchanged).
    - On a section's final floor, winning a normal combat unlocks the
      BOSS CHALLENGE (`Engine.BossUnlocked`, `event-boss-unlocked`)
      instead of ASCEND — the flag stays locked there; killing the boss
      is what unlocks ASCEND into the next section.
- The challenge: `CmdChallengeBoss` (StateExploring, only while
  `BossUnlocked` — otherwise a blocking message, state intact, like a
  locked ASCEND). It goes STRAIGHT to StateCombat: no StateEncounter, no
  SNEAK step (`Encounter.Boss` flags the encounter for the TUI's styling
  and the no-sneak guarantee). Consumes ZERO rng — no kind roll, no
  budget — and feeds nothing into the kind history (a challenge is not
  an explore; the forced-combat opener and the streak override are
  untouched).
- FLEEING the boss uses the unchanged flee formula (the boss's SPD
  applies): success returns to exploring and RE-LOCKS the challenge —
  another normal-combat victory on that floor re-unlocks it — and never
  unlocks ASCEND. Fleeing a NORMAL combat never touches the unlock;
  ascending resets it with the floor change.
- Boss victory (the last boss monster dies AND the hero survives turn-end
  DoTs and bookkeeping): the NORMAL per-kill flow
  first (kill XP; bosses pay `5 × difficulty` gold like anything else;
  mid-combat level ups included), then the boss's own rewards, in this
  rng/event order:
    1. the loot roll — ONE `Intn` over the tier+1 pool: every registry
       item (any slot) of the section's top allowed tier + 1, CAPPED at
       the registry's max tier (a section without `item_tiers` counts
       its top tier as the max, so the cap lands there too). An empty
       pool skips the item AND the rng — the gold is kept either way.
       A rolled item is held as `PendingLoot` and opens StateLoot (the
       chest decision: WEAR / STASH / DISCARD).
    2. the gold bonus (`boss.gold`, no rng) —
       `event-gold-gained{amount, source: "boss"}`; skipped when 0.
    3. `event-victory` — the ASCEND unlock finally holds
       (`CanAscend = true`, `BossUnlocked` consumed). With a pending
       item the engine stays in StateLoot; without one, back to
       exploring.
- Embedded groups (tuned as a considerable challenge for the section's
  LATE floors; gold ≈ 3-5× the boss floor's gold): goblin pits floor 3
  — goblin chief + hobgoblin, 250 gold; outskirts floor 13 — orc
  warlord + berserker + orc, 1000; underdark floor 33 — lich +
  vampire spawn + wraith, 2500; wraithfall floor 59 — beholder + two
  wraiths, 5000.
- Sections WITHOUT a boss block keep the old behavior on their final
  floor (normal victory → ASCEND): mod compatibility. The infinite
  dragon_hoard tail (`max_floor: -1`) has NO final floor — no boss
  gate, ever.
- Known consequence: a re-unlocked challenge can be re-fought (nothing
  latches "boss defeated") — the unlock follows the same
  lock/unlock/re-lock lifecycle as `CanAscend`.

### Moves

- A Move is an asset (`moves.json`) with `id`, `name`, `type`, `tier`,
  `cooldown`, `weight`, and an ordered `effects` list. Effects can damage
  an enemy, heal the user, or apply a status to self or enemy, and may be
  combined in one move. Effects resolve in authored order. If an effect
  kills a target, later effects continue for living recipients and skip
  dead recipients; status chance rolls still occur in order even for dead
  recipients. Healing is `flat + floor(user MaxHP × percent / 100)` (either
  component may be omitted), applied through `Heal` and clamped to missing
  HP. Healing has no resource or use limit: combat recovery by stalling is
  intentionally unlimited.
- Each combatant has AT MOST 4 moves (`MAX_MOVES`).
- Monsters pick moves by weight (AI v1; a decision tree may come later):
  each monster turn, ONE `Intn` over the cooldown-filtered set's total
  weight; an all-locked set falls back to the canonical `basic_attack`
  WITHOUT consuming a pick roll. A monster without a `moves` field uses
  `basic_attack` (cooldown 0), so older assets/mods keep working.
- **Move types** (content validation, startup-fatal): every move asset
  carries `type`, one of three words with no absent-field default:
    - `weapon` — the move reaches a hero ONLY through a weapon's grant
      list; a monster kit can never carry it and a shrine never offers
      it (the offer pool filters to creature/both).
    - `creature` — the move rides a creature's kit only: monster move
      sets and the hero's LEARNED set (the canonical `punch` is the
      shape case). A weapon can never grant it.
    - `both` — any consumer.
  Validation enforces the consumer contract: a weapon grant list must
  hold weapon/both moves only; a monster's move set must hold
  creature/both moves only. (`entities.Move` carries the type and the
  `UsableByWeapons()` / `UsableByCreatures()` pair.)
- Hero moves: exactly one explicit weapon-compatible, cooldown-0 basic
  action supplied by the equipped weapon, followed by up to 3 learned
  cooldown moves (shrine teaching, in learning order). A fresh hero has
  no learned moves and still starts with the wooden sword equipped.
- **The spammable rule** (content validation, startup-fatal): a weapon
  that grants moves REPLACES the `basic_attack` fallback, so its grant
  list must contain exactly one explicit basic action with
  `cooldown == 0` — a weapon whose
  every move is locked could otherwise never act. **Every weapon
  declares its moves explicitly: an absent or EMPTY `moves` list on a
  weapon is a validation error** (BREAKING for mods — move-less weapons
  no longer load; they no longer keep the fallback). Non-weapon slots
  still may not carry moves. The embedded fallback move's NAME is
  **Basic Strike** (its id stays `basic_attack`); mods may replace the
  definition under that canonical ID. With the rule in place
  every reachable hero turn has a ready move, so the automatic RECOVER
  turn below is content-unreachable — kept as defense in depth (a mod
  could still ship it; validation would catch it at startup).
- **Weaponless action**: a hero without a weapon gets virtual Unarmed
  Punch, resolved from the registry first and the canonical fallback
  second. It is not a learned move, item, or reward. Monsters without
  moves continue to use `basic_attack`.
- `CmdMove(index)` replaces ATTACK: every combat turn is a move choice,
  indexing the hero's combat move set (CombatMoves: the effective set,
  or the canonical `basic_attack` when that set is empty — a move-less
  hero must still be able to act).
- A move is learnable only when its asset explicitly opts in; omitted or
  false means it cannot be learned. Learnable moves must be
  creature-compatible and have positive cooldown. Older mods missing
  the field cannot teach that move.
- The weapon supplies exactly one explicit weapon-compatible basic
  action with cooldown 0. It is at combat index 0, followed by up to
  three learned cooldown moves in learning order. Weapon swaps never
  change the learned set; placeholder entries are not selectable.
- A move on cooldown is locked for exactly its cooldown's subsequent FULL
  turns after the turn of use. Existing locks advance at every consumed
  turn end, including killing turns, then actually used moves get their
  full cooldown; skipped actions never lock a move. Cooldowns never
  survive the encounter. Picking one locked move while
  another is ready is a blocked action (a message, no state change); a
  FULLY-cooled hero turn resolves as an automatic RECOVER turn (a
  message, the enemy acts, the ticks run) — never a soft-lock. With the
  spammable rule validated, every shipped
  weapon set is content-able to always have a ready move, so the
  RECOVER path is now content-unreachable — it stays as depth for
  hand-built states.
- Damage effects use `dmg_modifier` (percent; 100 = neutral) and optional
  `crit_bonus` (percentage points), then the usual spread and crit math. Monster AI
  still selects one available move by weighted random choice and may
  choose healing moves even at full HP.

### Status effects

- A Status is an asset (`statuses.json`): `id`, `name`,
  `kind` (buff/debuff), `effect` (DoT damage per tick, or a stat
  modifier for DMG/DEF/SPD), `duration` (future full turns for stat
  effects; damage ticks for DoTs). Moves apply them
  (target: enemy or self, with an application chance).
- Active effects are ENCOUNTER-SCOPED, never stacked, and cleared on
  encounter end or exit. Hero effects persist across queued monsters.
  Stat expiration deadlines and remaining DoT ticks follow "Combat
  calculations → Status effects", including the duration-2 timeline.
- Effective stats = base + gear + active status modifiers, clamped
  at 0 (DMG at 1). DoT damage CAN kill — through the normal defeat path
  (kill XP/gold for monsters, game over for the hero).
- End-of-turn DoTs follow captured initiative recipient order (hero
  recipient first on speed ties), then status application order within
  each recipient. The first lethal tick stops further damage, not
  non-damage bookkeeping. Rewards require the hero to survive that phase.
- Events: `event-move-used{user, move, side}`, `event-status-applied{target,
  status, turns, lifetime_unit, category, kind, side}` (the legacy `turns` count
  is measured in `ticks` or `turns`, as specified by `lifetime_unit`),
  `event-status-expired{target, status, kind, side}`,
  `event-status-damage{target, status, amount, target_hp_exact,
  target_max_hp, kind}`; the damage-dealt/taken payloads also carry the
  additive `move` key.
- Starter set: poison (DoT), weaken (−DMG), slow (−SPD), guard
  (+DEF, self).

### Shrines (IMPLEMENTED)

- A fifth encounter kind (`shrine`): on EXPLORE it draws ONE or TWO
  random UNLEARNED moves from the section's `move_tiers` (the
  `item_tiers` pattern: absent = unrestricted) and opens
  **StateLearn** in its CHOOSING step — even under the learned cap the
  player picks between the offers:
    - The offers are held in `Engine.PendingMoves` (length 1–2; the
      `PendingLoot` discipline: cleared on every exit path) and
      announced by `event-shrine-offered{count}` (`count` is 1 or 2 —
      the TUI reads `PendingMoves` for the dossiers).
    - **CHOOSE `<offer-index>`** (`CmdChooseOffer`; `Index` into
      `PendingMoves`):
        - **Under the 3-learned cap** — the chosen move is learned on
          the spot (`event-move-learned{move}`) and exploring
          continues: everything cleared.
        - **At the cap** — the choice is recorded in
          `Engine.ChosenMove` (cleared on every exit path like the
          offers) and StateLearn advances to the FORGETTING step (an
          `event-message` prompts the forget): the learned set is
          untouched until the player decides.
    - **FORGET `<learned-index>`** (the forgetting step only;
      `CmdForget`; `Index` into `LearnedMoves()`) — replaces that learned
      move in place with the CHOSEN offer. Other learned slots do not
      shift, so the combat action at each index remains stable
      (`event-move-learned{move, forgotten?}` — the additive optional
      `forgotten` key carries the displaced move, present only when a
      forget happened). Exploring continues, everything cleared.
    - **DECLINE** (`CmdDecline`, EITHER step) — keep everything as it
      was: both learned moves, no choice (`event-message`); exploring
      continues, everything cleared.
    - Out-of-range/negative indexes are a generic invalid command, no
      state change, never a panic; so is a command from the OTHER step
      (below); QUIT works as everywhere else.
- **The step-dependent command surface**: the CHOOSING step allows
  `[CHOOSE, DECLINE, QUIT]`, the FORGETTING step
  `[FORGET, DECLINE, QUIT]` — the engine enforces the per-step matrix
  (`isValidCommand` over `allowedCommands`), and
  `Engine.AvailableCommands()` returns that instance-specific set.
  Package-level `AllowedCommands(state)` is state-only and returns the
  union for StateLearn.
- **The offer rng contract**: ONE `Intn` PER offer (1–2 total) at
  shrine resolution, over the filtered pool SHRINKING per pick (each
  pick removes its move's id — the offers never duplicate; fewer offers
  only when the pool runs dry). An EMPTY filtered pool resolves as the
  nothing-new message with NO state change and NO offer roll; choosing,
  forgetting and declining consume NO rng. Shrines remain covered by
  the no-triple override (below) and never unlock ASCEND.
- The offer pool/filter contract (the DATA layer):
  `MovesForTiers(move_tiers)` (sorted by id; ALREADY type-filtered to
  creature/both — a weapon-only move is never offerable) filtered
  through `entities.FilterUnknownMoves(pool, known)` — the moves the
  hero does NOT already know, where `known` is the hero's effective set
  (learned AND weapon action alike, dedup by id; virtual unarmed punch
  filters out through the same rule), and minus the canonical
  `basic_attack`, which is NEVER offerable: it is the move-less
  fallback every combatant already fights with, and teaching it would
  waste one of the three learned slots. The filter is pure and
  order-preserving.
- `Engine.StatusLore()` exposes every registry status (sorted by id, a
  fresh copy of value assets — mutating the return never reaches the
  registry; empty-registry safe) for lore/UI consumers such as the
  TUI's `?` overlay.
- `Engine.PendingMoves` and `Engine.ChosenMove` are the sole shrine
  workflow state. `Engine.AvailableCommands()` exposes the active step's
  exact command set; the TUI does not infer choosing/forgetting from
  compatibility state. The old `event-shrine-found` constructor remains
  dormant; runtime encounters emit `event-shrine-offered`.
- Shrines are non-combat kinds: the no-triple pacing rule covers them
  (three shrines in a row is impossible) and they never unlock ASCEND.
- Embedded content: every section carries a `shrine` weight (11-13% of
  its kind roll) and `move_tiers`; six new tier 2-4 moves keep every
  section's pool supplied mid-run; 19 of the 24 monsters fight with
  authored move sets (early trash keeps the basic_attack fallback) and
  the iron/steel/draconic swords grant tier-appropriate moves — see the
  "Moves & statuses — data model" tables below.

Learning is capped at 3 learned moves (`MaxLearnedMoves`); the weapon's
single basic action occupies index 0 and learned moves follow it.
Weapon swaps do not alter learned moves.

### Moves & statuses — data model

The schemas below are the DATA LAYER of this design (shipped in phase
1); the combat flow consuming them is documented above and in
"Combat calculations → RNG".

**Move asset** (`moves.json`). Only `tier` has an absent-field default
(`0` → 1, mirroring items — JSON cannot tell them apart); every other
field is required as authored and validated at startup:

- `id`, `name` — identity.
- `type` — which consumers may carry the move: `weapon` (grant lists
  only), `creature` (monster kits and hero learning only) or `both`.
  Required — there is NO absent-field default; a missing or unknown
  word fails validation (startup-fatal; BREAKING for untyped mod moves).
- `tier` — progression ladder (1 = earliest); absent/0 → 1.
- `cooldown` — subsequent full turns the move stays locked after use (0 = spammable);
  validated ≥ 0.
- `effects` — an ordered list; effect order is behaviorally significant.
  Damage effects have `type: damage`, `target: enemy`, required
  `dmg_modifier` and optional `crit_bonus`. Healing effects have
  `type: heal`, `target: self`, and optional `flat` and/or `percent`.
  Status effects have `type: status`, `target: self|enemy`, and nested
  `status: {id, chance}`; the status id must exist and chance is [0, 100].
  A single move may combine any of these effects. Healing uses flat plus
  floored percent of the user's equipment-inclusive MaxHP, then `Heal`
  clamps it to missing HP. There is no resource/use limit.
- `weight` — the move's default AI weight (monster picks); ≥ 1.

**Status asset** (`statuses.json`). Identity, kind, effect type, and
duration are required; the effect's numeric components are optional and
omitted components mean zero:

- `id`, `name` — identity.
- `kind` — `buff` or `debuff`.
- `effect` — `{type: dot, amount?, percent?}` or
  `{type: stat, stat: dmg|def|spd, amount?, percent?}`. At least one of
  `amount` or `percent` must be nonzero. DoT components must be
  nonnegative; a present DoT percentage must be in [1, 100] and is
  uncapped. Stat components may be positive or negative. Stat percentages
  are summed, applied to the equipment-inclusive baseline with flooring,
  then flat amounts are summed and added; effective DMG/DEF/SPD retain
  their existing minimums.
- `duration` — ≥ 1; subsequent full turns for stat effects, damage ticks
  for DoTs. Stat effects apply immediately without consuming a full turn;
  DoTs begin ticking at application turn end if reached. Same-ID refreshes
  retain the later stat expiration deadline or larger DoT tick count,
  without stacking; different IDs combine independently. See the timing
  and lifetime contracts above.

**Where moves attach:**

- `MonsterAsset.moves` — a list of `{id, weight}` refs: at most 4
  entries, weights ≥ 1, ids must exist, and every referenced move must
  be creature- or both-typed (a weapon-only move on a monster is a
  validation error). The ref's weight is the MONSTER's own AI weight
  for that move — it overrides the move asset's `weight` when the
  engine resolves the set. ABSENT or empty → the monster fights with
  the canonical `basic_attack` (the monster-side fallback rule, the
  only one left for monsters), so older assets and mods keep working
  unchanged.
- `ItemAsset.moves` — a list of move ids. ONLY weapons may grant moves;
  **every weapon must declare them: an absent or empty list is a
  validation error now** (BREAKING for mods — the old "move-less weapon
  keeps the fallback" rule is gone; the weapon must grant its own
  spammable, and the starter sword grants `basic_attack` explicitly).
  At most 2 per weapon (`MaxWeaponMoves`); every id must exist and must
  be weapon- or both-typed (a creature-only move can never be granted);
  and the list must carry at least one `cooldown == 0` move (the
  spammable rule: grants replace the `basic_attack` fallback, so the
  weapon must always have a ready move). The engine resolves the ids
  into runtime moves when the item is built, so the hero's set is
  always computed from the EQUIPPED weapon (unwearing it removes the
  grants).
- `SectionAsset.move_tiers` — the exact `item_tiers` pattern: it gates
  which move tiers the section can offer (the shrine draws). Absent or
  empty = unrestricted; every listed tier must have at least one
  shrine-offerable (creature/both) registry move (typo protection).

**Registry queries** (mirroring the item ones): `MovesForTiers(tiers)`
— the SHRINE's pool query: creature- or both-typed moves only (the type
rule lives IN the query, so the engine's offer draw cannot forget it) —
whose tier is one of `tiers`, sorted by id; unrestricted when no tiers
are listed; an empty pool is never an error. Plus `MoveByID`,
`StatusByID`, `MaxMoveTier`, and `AllStatuses` (every status asset,
sorted by id, as a fresh slice — the read-only surface for lore/UI
consumers the engine exposes without sharing the unexported map).

**Hero move set** — the weapon's single explicit basic action is index 0,
followed by LEARNED cooldown moves in learning order (capped at 3 —
`MaxLearnedMoves`). A fresh hero starts with no learned moves and the
wooden sword equipped. Weapon changes do not change learned moves.
Weaponless combat uses virtual Unarmed Punch, resolved registry-first
with canonical fallback; it is not learned, an item, or a reward.
Learning:
`LearnMove` refuses at the cap (learning never displaces a move on its
own — the caller drives `ForgetMove(moveID)` and retries); learning an
already-known id is a refused no-op. For UI source tagging,
`Hero.EffectiveMovesWithSource(weapon)` returns the same set as
`[]entities.SourcedMove` (`Move` + `Source`: `SourceLearned` /
`SourceWeapon`) — the move grid renders the weapon action followed by
learned moves; the shrine offer pool is the same set
viewed from the other side:
`entities.FilterUnknownMoves(pool, hero.EffectiveMoves(weapon))` —
see "Shrines".

**The hero's opening move set** (as shipped): Basic Strike (the starter
wooden sword's explicit cooldown-0 action); there are no learned moves.

**Monster move set** — built by the engine from the asset's refs at
spawn time (`NewMonsterFromAsset(asset, moves...)`; no moves passed =
the canonical basic attack, matching the asset-level fallback). The
runtime `Move.Weight` carries the ref's per-monster AI weight.

**Embedded moves and their types** (the consumer contract applied):

| Type | Moves |
| --- | --- |
| `creature` | Venom Bite, Crushing Blow (ogre smash), Frost Touch, Soul Rot, Skull Crusher, Bloodletter, Punch |
| `both` | Basic Strike, Quick Stab, Guard Strike, Heavy Slash, Assassin's Strike, Venomous Blade, Shield Wall, Dragon's Might |
| `weapon` | Steel Flurry |

Typing notes: Quick Stab, Heavy Slash and Assassin's Strike are
`both` on purpose — each is granted by a shipped sword AND referenced
by shipped monster kits (ghoul/vampire/orc-berserker stabs, the orc
line's slashes, the mimic/vampire's strikes), so a single type keeps
every existing kit and grant coherent with no content edit. Steel
Flurry is the one weapon-only move (no monster fights with it);
Venomous Blade is `both` so the shrines can still offer it.

**Embedded weapons** (each weapon declares exactly one explicit
cooldown-0 basic action):

| Weapon (tier) | Grants | Spammable |
| --- | --- | --- |
| Espada de Madeira (1) | Basic Strike (explicit) | Basic Strike (100%) |
| Espada de Ferro (2) | Quick Stab | Quick Stab (60%, +20 crit) |
| Espada de Aço (3) | Steel Flurry, Heavy Slash | Steel Flurry (110%) |
| Espada Dracônica (4) | Assassin's Strike, Dragon's Might | Assassin's Strike (110%, +35 crit) |

The starter's explicit `basic_attack` grant is the weapon's basic action,
not an implicit fallback. The drake
blade's spammable is the retuned Assassin's Strike (tier 3→4, cooldown
3→0); the two monster kits referencing it keep a weighted second move,
so the spam stays a weighted choice on both sides.

**Load-time validation** (content bugs fail at startup, house style):

- move: tier < 1 (absent/0 defaults to 1 at Load); `type` missing or
  not one of weapon/creature/both (no default — untyped mod moves stop
  loading); cooldown < 0; weight < 1; malformed effects, invalid effect
  targets or parameters, a status chance outside [0, 100], or an unknown
  status id. This schema is breaking for mods: migrate former root-level
  `dmg_modifier`, `crit_bonus`, and `status` fields into `effects`;
  there is no automatic legacy translation.
- status: kind not buff/debuff; effect type not dot/stat; dot amount
  < 1; stat amount = 0 or an unknown stat word; duration < 1.
- monster: more than 4 moves, an unknown move id, a ref weight < 1, or
  a referenced move that is weapon-typed (kits take creature/only
  moves).
- weapon: an absent or EMPTY moves list (every weapon declares its
  moves — move-less weapons no longer load; BREAKING), more than 2
  moves, an unknown move id, a granted move that is creature-typed
  (grants take weapon/only moves), or no `cooldown == 0` move in the
  grant list (the spammable rule — grants replace the fallback); any
  non-weapon slot carrying moves.
- section: `move_tiers` values < 1, or a listed tier with zero
  shrine-offerable (creature/both) registry moves of that tier.
- duplicate ids in moves.json / statuses.json fail at Load (indexBy).

### Implementation phases

1. ~~Schema & entities — moves/statuses assets, move sets, validation~~
   (shipped)
2. ~~Combat integration — CmdMove, monster AI, cooldowns, status ticks,
   stopgap UI~~ (shipped: the engine flow plus the stopgap move-chip
   bar and status log lines; the rng contracts live in
   "Combat calculations → RNG")
3. ~~TUI combat redesign — 2×2 move grid, status chips, cooldown
   display~~ (shipped)
4. ~~Bosses & shrines — the gate flow, the asset block, the embedded
   groups; the fifth encounter kind and move learning~~ (shipped: the
   groups reference existing monsters — their members fight with
   authored move sets since the shrine content pass; the shrines ship
   with the flow documented in "Shrines")
5. ~~Combat view and screen accuracy~~ (shipped: combat snapshots are
   engine-owned; TUI rendering no longer reconstructs timing from events)
6. ~~Contract and ownership simplification~~ (shipped: active commands,
   event payload ownership, canonical fallback resolution, and shared
   item construction are documented above)
