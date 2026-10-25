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
	// Gold was added to the hero's purse ("source": "kill" or "chest").
	EventGoldGained
	// A merchant was found and opened shop (StateMerchant): the offers
	// are held in Engine.MerchantStock.
	EventMerchantFound
	// An item was bought from a merchant (it lands in the hero bag).
	EventItemBought
	// An item was sold to a merchant (it is destroyed).
	EventItemSold
	// A combatant used a Combat 2.0 move (every strike is a move use).
	EventMoveUsed
	// A status effect was applied to a combatant (refreshes never stack).
	EventStatusApplied
	// A status effect expired (its last round ticked away).
	EventStatusExpired
	// A DoT status damaged its afflicted at the round-end tick (it can
	// kill — the defeat events follow when it does).
	EventStatusDamage
	// A normal-combat victory on the section's final floor summoned the
	// boss challenge (the BOSS action is now offered; ASCEND stays
	// locked until the boss falls).
	EventBossUnlocked
	// The boss challenge was accepted: the predefined boss group is
	// engaged, straight into StateCombat (no FIGHT/SNEAK step).
	EventBossChallenge
	// A shrine offered a move at the learned cap: the offer is held in
	// Engine.PendingMove and the player must forget a learned move or
	// decline (StateLearn).
	EventShrineFound
	// The hero learned a move (a shrine's teaching): the optional
	// additive `forgotten` key carries the move displaced from the
	// learned set when it was at the cap.
	EventMoveLearned
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

// NewDamageDealtWithMove is the full EventCombatDamageDealt payload: the
// legacy keys above plus the ADDITIVE key:
//   - "move": the name of the Combat 2.0 move the strike was made with.
//     Consumers predating the key keep working.
func NewDamageDealtWithMove(enemy Combatant, dmgDealt int, crit bool, move string) Event {
	data := NewDamageDealt(enemy, dmgDealt, crit).Data
	data["move"] = move
	return Event{Type: EventCombatDamageDealt, Data: data}
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

// NewDamageTakenWithMove is the full EventCombatDamageTaken payload: the
// legacy keys above plus the ADDITIVE key:
//   - "move": the name of the Combat 2.0 move the strike was made with.
//     Consumers predating the key keep working.
func NewDamageTakenWithMove(hero Combatant, dmgTaken int, crit bool, move string) Event {
	data := NewDamageTaken(hero, dmgTaken, crit).Data
	data["move"] = move
	return Event{Type: EventCombatDamageTaken, Data: data}
}

// NewMoveUsed reports a combatant using a Combat 2.0 move — every strike
// is a move use now (the canonical basic attack included).
//
// Payload:
//   - "user": the striker's name.
//   - "move": the move's name.
func NewMoveUsed(user, move string) Event {
	return Event{
		Type: EventMoveUsed,
		Data: map[string]any{
			"user": user,
			"move": move,
		},
	}
}

// NewStatusApplied reports a status effect landing on a combatant —
// freshly applied or refreshed (never stacked: refreshed to the longest
// duration).
//
// Payload:
//   - "target": the afflicted combatant's name.
//   - "status": the status's name.
//   - "turns": rounds the status now has left.
//   - "kind": "buff" or "debuff" (additive; lets UIs tone the line).
func NewStatusApplied(target Combatant, status entities.Status, turns int) Event {
	return Event{
		Type: EventStatusApplied,
		Data: map[string]any{
			"target": target.Name(),
			"status": status.Name,
			"turns":  turns,
			"kind":   string(status.Kind),
		},
	}
}

// NewStatusExpired reports a status effect whose last round ticked away.
//
// Payload:
//   - "target": the (formerly) afflicted combatant's name.
//   - "status": the status's name.
//   - "kind": "buff" or "debuff" (additive).
func NewStatusExpired(target Combatant, status entities.Status) Event {
	return Event{
		Type: EventStatusExpired,
		Data: map[string]any{
			"target": target.Name(),
			"status": status.Name,
			"kind":   string(status.Kind),
		},
	}
}

// NewStatusDamage reports a DoT status hurting its afflicted at the
// round-end tick. The damage is a plain clamp (no mitigation) and CAN
// kill — the defeat events follow when it does.
//
// Payload:
//   - "target": the afflicted combatant's name.
//   - "status": the status's name.
//   - "amount": the HP actually subtracted (clamped to the current HP).
//   - "target_hp_exact"/"target_max_hp": the afflicted's exact HP AFTER
//     the tick.
//   - "kind": "buff" or "debuff" (additive).
func NewStatusDamage(target Combatant, status entities.Status, amount int) Event {
	return Event{
		Type: EventStatusDamage,
		Data: map[string]any{
			"target":          target.Name(),
			"status":          status.Name,
			"amount":          amount,
			"target_hp_exact": target.HP(),
			"target_max_hp":   target.MaxHP(),
			"kind":            string(status.Kind),
		},
	}
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
// Kept for compatibility: this payload has no "healed" key, so consumers
// predating the level-up heal keep working unchanged.
func NewLevelUp(lvl int) Event {
	return Event{
		Type: EventLevelUp,
		Data: map[string]any{
			"level": lvl,
		},
	}
}

// NewLevelUpWithHeal is the full EventLevelUp payload: the legacy keys
// above plus the ADDITIVE key:
//   - "healed": total HP the level up(s) actually healed — 25% of the
//     BASE max HP per level (equipment excluded), floored and clamped by
//     Hero.Heal; 0 when the Hero was at full health. Legacy consumers
//     that ignore the key keep working.
func NewLevelUpWithHeal(lvl, healed int) Event {
	data := NewLevelUp(lvl).Data
	data["healed"] = healed
	return Event{Type: EventLevelUp, Data: data}
}

// NewQuit tells the caller to stop the game loop and exit.
func NewQuit() Event {
	return Event{Type: EventQuit}
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

// Gold sources for EventGoldGained's "source" payload.
const (
	// GoldSourceKill — gold granted for defeating a monster.
	GoldSourceKill = "kill"
	// GoldSourceChest — gold granted by a chest that paid out coins
	// instead of holding an item.
	GoldSourceChest = "chest"
	// GoldSourceBoss — the gold bonus a defeated section boss pays.
	GoldSourceBoss = "boss"
)

// NewGoldGained reports gold added to the hero's purse.
//
// Payload:
//   - "amount": the gold gained.
//   - "source": where it came from — GoldSourceKill (enemy defeated),
//     GoldSourceChest (a chest paid out coins instead of an item) or
//     GoldSourceBoss (the section boss's victory bonus).
func NewGoldGained(amount int, source string) Event {
	return Event{
		Type: EventGoldGained,
		Data: map[string]any{
			"amount": amount,
			"source": source,
		},
	}
}

// NewBossUnlocked reports that a normal-combat victory on the section's
// final floor summoned the boss challenge: the BOSS action is now
// offered (the engine's BossUnlocked is set), while ASCEND stays locked
// until the boss falls. No payload.
func NewBossUnlocked() Event {
	return Event{Type: EventBossUnlocked}
}

// NewBossChallenge reports the challenge accepted: the section's
// PREDEFINED boss group is engaged, straight into StateCombat (no
// FIGHT/SNEAK step — a boss challenge cannot be sneaked).
//
// Payload:
//   - "monsters": the boss group, in authored order.
func NewBossChallenge(monsters []entities.Monster) Event {
	return Event{
		Type: EventBossChallenge,
		Data: map[string]any{
			"monsters": monsters,
		},
	}
}

// NewMerchantFound reports a merchant encounter: the shop is open in
// StateMerchant with its offers held in Engine.MerchantStock.
//
// Payload:
//   - "count": how many offers the merchant stocks.
func NewMerchantFound(count int) Event {
	return Event{
		Type: EventMerchantFound,
		Data: map[string]any{
			"count": count,
		},
	}
}

// NewItemBought reports an item bought from a merchant.
//
// Payload:
//   - "item": the bought item (now in the hero bag).
//   - "price": the gold paid (the item's BuyPrice).
func NewItemBought(item *entities.Item, price int) Event {
	return Event{
		Type: EventItemBought,
		Data: map[string]any{
			"item":  item,
			"price": price,
		},
	}
}

// NewItemSold reports an item sold to a merchant.
//
// Payload:
//   - "item": the sold item (destroyed — gone for good).
//   - "price": the gold received (the item's SellValue).
func NewItemSold(item *entities.Item, price int) Event {
	return Event{
		Type: EventItemSold,
		Data: map[string]any{
			"item":  item,
			"price": price,
		},
	}
}

// NewShrineFound reports a shrine offering a move at the learned cap: the
// offer is held in Engine.PendingMove while the player decides in
// StateLearn (forget a learned move or decline) — nothing is learned yet.
//
// Payload:
//   - "move": the offered move.
func NewShrineFound(move entities.Move) Event {
	return Event{
		Type: EventShrineFound,
		Data: map[string]any{
			"move": move,
		},
	}
}

// NewMoveLearned reports a move the hero learned (a shrine's teaching).
//
// Payload:
//   - "move": the learned move.
//   - "forgotten": ADDITIVE optional key — the learned move it displaced
//     (present only when the set was at the cap and the player chose to
//     forget one). Consumers predating the key keep working.
func NewMoveLearned(move entities.Move, forgotten *entities.Move) Event {
	data := map[string]any{
		"move": move,
	}
	if forgotten != nil {
		data["forgotten"] = *forgotten
	}
	return Event{Type: EventMoveLearned, Data: data}
}
