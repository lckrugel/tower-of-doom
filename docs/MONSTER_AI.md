# Monster move selection

Monsters choose a move when their turn arrives. The engine does not show or
store a preview of that choice, so the player never sees an exact intention.
Some monsters can still create recognizable setup turns through visible status
effects.

## Move selection

Each monster has a move kit. Its `moves` entries define the available move IDs
and their weights; these per-monster weights are used for selection. Moves on
cooldown are removed before the AI rules are considered.

Monsters without an `ai` section preserve the original behavior: choose
randomly by weight from all ready moves in their kit. For a monster with `ai`,
the engine:

1. Checks `priorities` in the order written in the asset.
2. Requires every condition in a priority's `when` list to match.
3. Chooses by weight from that priority's `moves` that are ready.
4. If the conditions match but all of that rule's moves are cooling down,
   continues to the next priority.
5. If no priority supplies a ready move, chooses by weight from `fallback`
   moves that are ready.

If every move in the monster's kit is cooling down, the engine uses the
canonical `basic_attack` without a selection roll. The same basic-attack
fallback is used defensively if a configured fallback has no ready candidate;
validated game content is expected to provide a usable fallback.

Each nonempty candidate pool gets one weighted random pick, including a pool
with a single move. Condition checks and cooldown filtering consume no
randomness. AI selection does not predict damage or inspect the player's
pending action.

## Conditions

Conditions are deliberately small and explicit:

| `type` | `target` | Meaning |
|---|---|---|
| `hp_below` | `self` | The monster's current HP is strictly below `percent` of its max HP. |
| `status_present` | `self` or `enemy` | The target currently has the status named by `status_id`. |
| `status_absent` | `self` or `enemy` | The target does not currently have the status named by `status_id`. |

In the current one-on-one combat system, `enemy` means the hero. HP checks use
the exact HP ratio, not a rounded display percentage. For example, `hp_below`
at 35 does not match at exactly 35% HP. Multiple conditions in one rule are
conjunctive (all must match). Conditions are evaluated using the current
combat state at the time the monster chooses its move.

## Asset configuration

AI configuration is optional on a monster. An `ai` object has an ordered
`priorities` list and an explicit `fallback` list. Each rule has a `when`
condition list and a `moves` candidate list. Rule and fallback move IDs must
belong to that monster's move kit; weights remain defined by the kit entries.

For example, this rule uses `guard_strike` only below 60% HP and while the
monster lacks Guard. Otherwise, it falls back to `heavy_slash`:

```json
{
  "moves": [
    { "id": "heavy_slash", "weight": 60 },
    { "id": "guard_strike", "weight": 40 }
  ],
  "ai": {
    "priorities": [
      {
        "when": [
          { "type": "hp_below", "target": "self", "percent": 60 },
          { "type": "status_absent", "target": "self", "status_id": "guard" }
        ],
        "moves": ["guard_strike"]
      }
    ],
    "fallback": ["heavy_slash"]
  }
}
```

The registry validates condition types and targets, percentages and status
references, and verifies that rules and fallbacks are nonempty and refer only
to moves in the monster's kit. Older assets without `ai` remain valid.

## Current examples

- **Vampire Spawn:** below 35% HP, prioritizes `rallying_cut`; otherwise uses
  `assassins_strike` as its fallback.
- **Goblin Chief:** below 60% HP and without Guard, prioritizes `guard_strike`;
  otherwise uses `heavy_slash`.
- **Dragonling:** its fallback is `cinderbite`. When it already has
  `breachfire`, it prioritizes `skull_crusher`. Its move kit also includes
  `brand_of_the_breach`, which can apply Breachfire and expose the Dragonling's
  guard. However, Brand is currently in neither its priority list nor its
  fallback list, so this AI selector does not choose it. The Breachfire
  follow-up rule is therefore dormant unless another effect applies that
  status. This is a content/configuration mismatch with the intended setup
  behavior, not a hidden move-selection feature.

These are opt-in examples, not a global behavior applied to every monster.
The system does not currently support exact intention displays, damage
prediction, scripted multi-turn plans, or special combo damage beyond the
effects of the selected moves.
