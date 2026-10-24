package engine

import "github.com/lckrugel/tower-of-doom/internal/game/engine/entities"

// `EventType` é tudo aquilo que o jogo pode reportar que aconteceu.
type EventType int

const (
	// Hero enters a new floor.
	EventFloorEntered EventType = iota
	// A new encounter was found.
	EventEncounterFound
	// Hero past the encounter successfully.
	EventSneakSuccess
	// Hero failed to sneak past the encounter.
	EventSneakFailed
	// Hero's damage dealt.
	EventCombatDamageDealt
	// Hero's damage taken.
	EventCombatDamageTaken
	EventEnemyHealed
	// Hero healed outside of combat (sanctuary encounters).
	EventHeroHealed
	EventEnemyDefeated
	// Hero died.
	EventHeroDefeated
	// Hero successfully fled the combat.
	EventFled
	// Defeated all enemies in an encounter.
	EventVictory
	// Hero gained a level (mid-combat level ups included).
	EventLevelUp
	// Hero found (and auto-equipped) an item.
	EventItemFound
	// Quit requested the game.
	EventQuit
	// Generic game message.
	EventMessage
	// The Hero moved an item into its equipment slot (from the bag or
	// from a chest's pending loot).
	EventItemEquipped
	// The Hero destroyed an item on purpose (bag drop or loot discard).
	// Destroyed items are gone for good: there is no floor layer.
	EventItemDiscarded
	// A chest yielded an item now held as PendingLoot: the player must
	// choose in StateLoot (equip/stash/discard) — nothing is equipped or
	// stored yet.
	EventLootFound
	// An item was stored in the hero bag.
	EventItemStashed
	// An item was destroyed because it could not be kept — displaced
	// while the hero bag had no room.
	EventItemLost
	// A stash attempt was rejected because the hero bag is full.
	EventBagFull
)

// `Event` é o que o jogo envia após processar um `Command`.
type Event struct {
	Type EventType
	Data map[string]any
}

func NewMessage(msg string) Event {
	return Event{
		Type: EventMessage,
		Data: map[string]any{
			"message": msg,
		}}
}

func NewFloorEntered(floor int, sectionName string) Event {
	return Event{
		Type: EventFloorEntered,
		Data: map[string]any{
			"floor":   floor,
			"section": sectionName,
		},
	}
}

func NewEncounterFound(monsters []entities.Monster) Event {
	return Event{
		Type: EventEncounterFound,
		Data: map[string]any{
			"monsters": monsters,
		},
	}
}

// NewHeroHealed reports a sanctuary-style heal that happened instantly
// outside of combat.
//
// Legacy payload (kept for compatibility):
//   - "healed": HP actually restored (0 when the Hero was already full).
//   - "hero_hp": resulting HP as a floor percentage of max HP.
//
// The engine emits the EventHeroHealedExact payload below.
func NewHeroHealed(healed, heroHPPercent int) Event {
	return Event{
		Type: EventHeroHealed,
		Data: map[string]any{
			"healed":  healed,
			"hero_hp": heroHPPercent,
		},
	}
}

// NewHeroHealedExact is the full EventHeroHealed payload: the legacy
// contract above plus the defender's exact state AFTER the heal:
//   - "hero_hp_exact": current HP as an absolute value.
//   - "hero_max_hp": current max HP.
func NewHeroHealedExact(hero Combatant, healed, heroHPPercent int) Event {
	data := NewHeroHealed(healed, heroHPPercent).Data
	data["hero_hp_exact"] = hero.HP()
	data["hero_max_hp"] = hero.MaxHP()
	return Event{Type: EventHeroHealed, Data: data}
}

// NewDamageDealt reports the Hero's landed hit.
//
// Payload:
//   - "enemy": enemy name.
//   - "enemy_hp": enemy HP as a floor percentage of max HP (legacy).
//   - "enemy_alive": whether the enemy survived the hit.
//   - "damage_dealt": the mitigated damage subtracted from the enemy.
//   - "crit": whether the hit was a critical.
//   - "enemy_hp_exact": enemy current HP as an absolute value, AFTER the hit.
//   - "enemy_max_hp": enemy max HP.
func NewDamageDealt(enemy Combatant, dmgDealt int, crit bool) Event {
	return Event{
		Type: EventCombatDamageDealt,
		Data: map[string]any{
			"enemy":          enemy.Name(),
			"enemy_hp":       enemy.CurrentHPPercent(),
			"enemy_alive":    enemy.IsAlive(),
			"damage_dealt":   dmgDealt,
			"crit":           crit,
			"enemy_hp_exact": enemy.HP(),
			"enemy_max_hp":   enemy.MaxHP(),
		},
	}
}

// NewDamageTaken reports a hit landed on the Hero.
//
// Payload:
//   - "hero": hero name.
//   - "hero_hp": hero HP as a floor percentage of max HP (legacy).
//   - "hero_alive": whether the Hero survived the hit.
//   - "damage_taken": the mitigated damage actually subtracted (clamped
//     to the current HP by TakeDmg, hence possibly lower than the swing).
//   - "crit": whether the hit was a critical.
//   - "hero_hp_exact": hero current HP as an absolute value, AFTER the hit
//     (0 on a lethal blow, never negative).
//   - "hero_max_hp": hero max HP.
func NewDamageTaken(hero Combatant, dmgTaken int, crit bool) Event {
	return Event{
		Type: EventCombatDamageTaken,
		Data: map[string]any{
			"hero":          hero.Name(),
			"hero_hp":       hero.CurrentHPPercent(),
			"hero_alive":    hero.IsAlive(),
			"damage_taken":  dmgTaken,
			"crit":          crit,
			"hero_hp_exact": hero.HP(),
			"hero_max_hp":   hero.MaxHP(),
		},
	}
}

func NewSneakSuccess() Event {
	return Event{Type: EventSneakSuccess}
}

func NewSneakFailed() Event {
	return Event{Type: EventSneakFailed}
}

func NewFled() Event {
	return Event{Type: EventFled}
}

func NewVictory() Event {
	return Event{Type: EventVictory}
}

// NewEnemyDefeated reports a defeated enemy and the instant XP reward
// (mid-combat kills included).
func NewEnemyDefeated(monsterName string, xp int) Event {
	return Event{
		Type: EventEnemyDefeated,
		Data: map[string]any{
			"enemy":     monsterName,
			"xp_gained": xp,
		},
	}
}

// NewHeroDefeated reports the Hero's death without any run statistics
// (kept for compatibility; the engine itself emits the richer payload).
func NewHeroDefeated() Event {
	return Event{Type: EventHeroDefeated}
}

// NewHeroDefeatedWithStats reports the Hero's death together with the run
// state at that moment:
//   - "floor": the floor the Hero died on (CurrentFloor).
//   - "level": the Hero's level at death.
//   - "kills": enemies defeated during the run up to (and including) this
//     round of combat.
//
// No extra data arrives with the legacy NewHeroDefeated payload.
func NewHeroDefeatedWithStats(floor, level, kills int) Event {
	return Event{
		Type: EventHeroDefeated,
		Data: map[string]any{
			"floor": floor,
			"level": level,
			"kills": kills,
		},
	}
}

// NewLevelUp reports that the Hero gained at least one level.
// `lvl` is the level reached after all pending level ups.
func NewLevelUp(lvl int) Event {
	return Event{
		Type: EventLevelUp,
		Data: map[string]any{
			"level": lvl,
		},
	}
}

// NewQuit tells the caller to stop the game loop and exit.
func NewQuit() Event {
	return Event{Type: EventQuit}
}

// NewItemFound reports an item the Hero found and auto-equipped
// (there is no inventory yet — whatever it replaced is gone).
//
// Note: since the hero bag exists, chests do NOT auto-equip anymore (they
// open the StateLoot decision); this event remains for rewards the engine
// still auto-equips, like the ascend armor piece.
func NewItemFound(item *entities.Item) Event {
	return Event{
		Type: EventItemFound,
		Data: map[string]any{
			"item": item,
		},
	}
}

// NewItemEquipped reports an item the Hero moved into its equipment slot —
// from the bag (StateInventory) or the pending chest loot (StateLoot).
//
// Payload:
//   - "item": the item now equipped.
//   - "swapped_from": the displaced item. For bag equips it took the
//     equipped item's old bag position; for loot equips it was sent to
//     the bag when there was room (and is separately reported by
//     EventItemLost when the bag was full and it was destroyed). The key
//     is ABSENT when nothing was displaced — consumers must treat it as
//     optional.
func NewItemEquipped(item, swappedFrom *entities.Item) Event {
	data := map[string]any{
		"item": item,
	}
	if swappedFrom != nil {
		data["swapped_from"] = swappedFrom
	}
	return Event{
		Type: EventItemEquipped,
		Data: data,
	}
}

// NewItemDiscarded reports an item the Hero destroyed on purpose (a bag
// drop or a loot discard). Destroyed items are gone for good: there is no
// floor layer to pick things back up from.
//
// Payload:
//   - "item": the destroyed item.
func NewItemDiscarded(item *entities.Item) Event {
	return Event{
		Type: EventItemDiscarded,
		Data: map[string]any{
			"item": item,
		},
	}
}

// NewLootFound reports the item a chest yielded. The engine holds it as
// PendingLoot while the player decides in StateLoot — nothing is equipped
// or stored yet.
//
// Payload:
//   - "item": the found item.
func NewLootFound(item *entities.Item) Event {
	return Event{
		Type: EventLootFound,
		Data: map[string]any{
			"item": item,
		},
	}
}

// NewItemStashed reports an item stored in the hero bag.
//
// Payload:
//   - "item": the stashed item.
func NewItemStashed(item *entities.Item) Event {
	return Event{
		Type: EventItemStashed,
		Data: map[string]any{
			"item": item,
		},
	}
}

// NewItemLost reports an item the Hero could not keep — displaced while
// the bag had no room — and that was therefore destroyed.
//
// Payload:
//   - "item": the destroyed item.
func NewItemLost(item *entities.Item) Event {
	return Event{
		Type: EventItemLost,
		Data: map[string]any{
			"item": item,
		},
	}
}

// NewBagFull reports a stash attempt rejected because the hero bag is at
// capacity. Carries no payload: the failed command is always a stash
// (the only command that can be blocked by a full bag), and the pending
// item is visible as PendingLoot.
func NewBagFull() Event {
	return Event{Type: EventBagFull}
}
