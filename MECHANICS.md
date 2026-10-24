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
    - Unlocks ascending to the next floor
    - TODO: add gold and items reward
- After combat games goes to StateExploring again
    - If combat was won, ASCEND is now permitted
- If the Hero's HP reachs zero, it dies
    - StateGameOver


## Combat calculations

### RNG

### Defense

## Items

TODO

## Types of encounters

TODO