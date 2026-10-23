# Tower of Doom

This is CLI based roguelike game still under implementation.

Code is divided into core game logic (`internal/game`) and (futurally) display via TUI (using bubbletea).

Assets for the game are described in `json` files on `internal/game/assets`. The assets described in that directory are the base game assets, but the idea is to support user created assets or mods in a custom directory. These assets are loaded into a registry at application launch.

## Design notes (decided)

- **XP curve:** cost to level up from Lvl = `100 · max(Lvl−1, 1)^1.5` (`baseReqXp`, `requiredXpGrowth`). Levels 1→2 and 2→3 cost the same (100) — known consequence of `max()`, kept on purpose ("cheap early levels").
- **%HP preservation:** when the hero's max HP changes (level-up, equip/unequip), `CurrentHP` is rescaled as a fraction of the *pre-change* `MaxHP()` (equipment included), floored and clamped to the new max. Centralized in `rescaleHP`.
- **Damage:** `TakeDmg` never increases HP (negative damage is ignored) and clamps at 0. Hero/Monster `TakeDmg` behavior is identical and locked by tests.
- **Dice:** `RollD*` return values in `[1, n]` inclusive.
- **Engine randomness:** encounter generation goes through the injectable `rng` interface; combat will use the same seam. `weightedPick` panics if no positive weights — content validation guarantees `weight ≥ 1` and `difficulty ≥ 1` (terminating encounter generation); `MAX_ENCOUNTER_SIZE` stays as defense in depth.
- **Duplicate content ids** fail at load time (`indexBy`), not at validation.
- **Engine invariants** (e.g. "content validated at startup") may panic; anything reachable by a player must not. `cmd/cli` calls `Load → Validate → NewEngine` with a large validation horizon (infinite tower).