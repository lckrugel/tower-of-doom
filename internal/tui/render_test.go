package tui

import (
	"maps"
	"strings"
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

func monsterFixture() entities.Monster {
	return entities.NewMonsterFromAsset(game.MonsterAsset{
		ID: "goblin", Name: "Goblin", Difficulty: 3, HP: 30, Damage: 2, Speed: 10, Crit: 5,
	})
}

func testItem() *entities.Item {
	item := entities.NewItemFromAsset(game.ItemAsset{
		ID: "wooden_sword", Name: "Espada de Madeira", Slot: "weapon",
		BonusDMG: 5, Durability: 50,
	})
	return &item
}

// displacedItem is the previously worn gear an equip displaces into the
// bag (the engine's swapped_from payload).
func displacedItem() *entities.Item {
	item := entities.NewItemFromAsset(game.ItemAsset{
		ID: "old_blade", Name: "Old Blade", Slot: "weapon",
		BonusDMG: 2, Durability: 20,
	})
	return &item
}

// damagedHero builds a Hero that took `dmg` damage, mirroring the engine's
// order of operations (damage applied, then the event carries the result).
func damagedHero(dmg int, crit bool) engine.Event {
	hero := entities.NewPlayer()
	hero.TakeDmg(dmg)
	return engine.NewDamageTaken(hero, dmg, crit)
}

// damagedGoblin builds the "enemy was hit" side of the same pattern.
func damagedGoblin(dmg int, crit bool) engine.Event {
	goblin := monsterFixture()
	goblin.TakeDmg(dmg)
	return engine.NewDamageDealt(&goblin, dmg, crit)
}

// The snapshot event map is the renderer's only source of truth, so both
// payload shapes (legacy-only and exact HP) are hand-constructed below.

// withEnemyExact copies a damage-dealt event and adds the exact HP pair
// the engine's richer constructors carry.
func withEnemyExact(ev engine.Event, cur, max int) engine.Event {
	data := maps.Clone(ev.Data)
	data["enemy_hp_exact"], data["enemy_max_hp"] = cur, max
	return engine.Event{Type: ev.Type, Data: data}
}

// withHeroExact copies a hero-side event (hit taken, heal) and adds the
// exact HP pair.
func withHeroExact(ev engine.Event, cur, max int) engine.Event {
	data := maps.Clone(ev.Data)
	data["hero_hp_exact"], data["hero_max_hp"] = cur, max
	return engine.Event{Type: ev.Type, Data: data}
}

// withoutExact copies an event and strips the exact HP pair, rebuilding
// the legacy-only payload an older engine would send. The percentage keys
// are the renderer's fallback and must keep working on their own.
func withoutExact(ev engine.Event) engine.Event {
	data := maps.Clone(ev.Data)
	for _, key := range []string{"enemy_hp_exact", "enemy_max_hp", "hero_hp_exact", "hero_max_hp"} {
		delete(data, key)
	}
	return engine.Event{Type: ev.Type, Data: data}
}

func TestEventText(t *testing.T) {
	goblin := monsterFixture()

	tests := []struct {
		name     string
		event    engine.Event
		want     string
		contains []string // checked instead of exact for composed texts
	}{
		{
			"floor entered",
			engine.NewFloorEntered(2, "Goblin Pits"),
			"You climb to floor 2: Goblin Pits.",
			nil,
		},
		{
			"encounter found lists the monsters",
			engine.NewEncounterFound([]entities.Monster{goblin}),
			"",
			[]string{"You stumble upon an encounter", "Goblin (30 HP)"},
		},
		{
			"plain damage dealt",
			withoutExact(damagedGoblin(5, false)),
			"You hit the Goblin for 5 damage (83% HP left).",
			nil,
		},
		{
			"critical damage dealt",
			withoutExact(damagedGoblin(10, true)),
			"You land a devastating blow on the Goblin — CRIT! 10 damage (66% HP left).",
			nil,
		},
		{
			"killing blow dealt",
			damagedGoblin(30, false),
			"You hit the Goblin for 30 damage — a killing blow!",
			nil,
		},
		{
			"plain damage taken",
			withoutExact(damagedHero(2, false)),
			"You take 2 damage (98% HP left).",
			nil,
		},
		{
			"critical damage taken",
			withoutExact(damagedHero(4, true)),
			"A crushing hit lands on you — CRIT! You take 4 damage (96% HP left).",
			nil,
		},
		// Events carrying the exact HP pair render absolute values
		// instead of the legacy floor percentage.
		{
			"exact HP on a plain hit",
			withEnemyExact(damagedGoblin(12, false), 8, 45),
			"You hit the Goblin for 12 damage (8/45 HP left).",
			nil,
		},
		{
			"exact HP on a critical hit",
			withEnemyExact(damagedGoblin(12, true), 8, 45),
			"You land a devastating blow on the Goblin — CRIT! 12 damage (8/45 HP left).",
			nil,
		},
		{
			"exact HP 0 on a killing blow stays a killing blow",
			withEnemyExact(damagedGoblin(30, false), 0, 30),
			"You hit the Goblin for 30 damage — a killing blow!",
			nil,
		},
		{
			"exact HP on damage taken",
			withHeroExact(damagedHero(12, false), 88, 100),
			"You take 12 damage (88/100 HP left).",
			nil,
		},
		{
			"exact HP 0 on a lethal blow keeps the fading text",
			withHeroExact(damagedHero(100, false), 0, 100),
			"You take 100 damage and your vision fades...",
			nil,
		},
		{
			"malformed exact HP falls back to the legacy percentage",
			engine.Event{Type: engine.EventCombatDamageDealt, Data: map[string]any{
				"enemy": "Goblin", "enemy_hp": 90, "enemy_alive": true,
				"damage_dealt": 3, "crit": false,
				"enemy_hp_exact": "not an int", "enemy_max_hp": 30,
			}},
			"You hit the Goblin for 3 damage (90% HP left).",
			nil,
		},
		{
			"sneak success",
			engine.NewSneakSuccess(),
			"You sneak past the enemies unnoticed.",
			nil,
		},
		{
			"sneak failure",
			engine.NewSneakFailed(),
			"You are spotted! Combat begins and the enemies strike first.",
			nil,
		},
		{
			"enemy defeated",
			engine.NewEnemyDefeated("Goblin", 12),
			"The Goblin goes down! +12 XP.",
			nil,
		},
		{
			"level up",
			engine.NewLevelUp(2),
			"Level up! You are now level 2.",
			nil,
		},
		{
			"level up with heal",
			engine.NewLevelUpWithHeal(2, 27),
			"Level up! You are now level 2 (+27 HP).",
			nil,
		},
		{
			"level up at full health",
			engine.NewLevelUpWithHeal(3, 0),
			"Level up! You are now level 3.",
			nil,
		},
		{
			"hero defeated",
			engine.NewHeroDefeated(),
			"You collapse... Your journey through the tower ends here.",
			nil,
		},
		{
			"victory",
			engine.NewVictory(),
			"Victory! The floor is clear — ASCEND to continue your climb.",
			nil,
		},
		{
			"fled",
			engine.NewFled(),
			"You fled the combat successfully.",
			nil,
		},
		{
			"quit",
			engine.NewQuit(),
			"You persistently close your eyes. Goodbye.",
			nil,
		},
		{
			"sanctuary heal",
			engine.NewHeroHealed(20, 90),
			"While exploring, you find an ancient sanctuary and rest. You recover 20 HP (90% HP left).",
			nil,
		},
		{
			"sanctuary at full health",
			engine.NewHeroHealed(0, 100),
			"An ancient sanctuary embraces you in quiet rest — your wounds are already healed.",
			nil,
		},
		// Exact post-heal HP replaces the percentage when the engine
		// reports it.
		{
			"heal shows post-heal exact HP",
			withHeroExact(engine.NewHeroHealed(20, 45), 45, 100),
			"While exploring, you find an ancient sanctuary and rest. You recover 20 HP (45/100 HP).",
			nil,
		},
		{
			"full heal shows exact HP",
			withHeroExact(engine.NewHeroHealed(20, 100), 100, 100),
			"While exploring, you find an ancient sanctuary and rest. You recover 20 HP (100/100 HP).",
			nil,
		},
		{
			"no-op heal with exact HP stays the already-healed text",
			withHeroExact(engine.NewHeroHealed(0, 100), 100, 100),
			"An ancient sanctuary embraces you in quiet rest — your wounds are already healed.",
			nil,
		},
		{
			"loot found lists bonuses",
			engine.NewLootFound(testItem()),
			"The chest holds Espada de Madeira (+5 DMG).",
			nil,
		},
		{
			"loot found without an item is dust",
			engine.Event{Type: engine.EventLootFound, Data: map[string]any{}},
			"The chest holds nothing but dust.",
			nil,
		},
		{
			"loot found with a typed-nil item is dust",
			engine.Event{Type: engine.EventLootFound, Data: map[string]any{
				"item": (*entities.Item)(nil),
			}},
			"The chest holds nothing but dust.",
			nil,
		},
		{
			"item equipped without a swap",
			engine.NewItemEquipped(testItem(), nil),
			"You equip Espada de Madeira (+5 DMG).",
			nil,
		},
		{
			"item equipped over a displaced one",
			engine.NewItemEquipped(testItem(), displacedItem()),
			"You equip Espada de Madeira (+5 DMG) — the Old Blade goes into your bag.",
			nil,
		},
		{
			"item equipped with a malformed swap falls back to the no-swap text",
			engine.Event{Type: engine.EventItemEquipped, Data: map[string]any{
				"item":         testItem(),
				"swapped_from": "not an item",
			}},
			"You equip Espada de Madeira (+5 DMG).",
			nil,
		},
		{
			"item stashed",
			engine.NewItemStashed(testItem()),
			"You put the Espada de Madeira (+5 DMG) in your bag.",
			nil,
		},
		{
			"item discarded",
			engine.NewItemDiscarded(testItem()),
			"The Espada de Madeira (+5 DMG) crumbles away — gone for good.",
			nil,
		},
		{
			"item lost to the overflow",
			engine.NewItemLost(testItem()),
			"Your bag overflows — the Espada de Madeira (+5 DMG) is lost to the tower.",
			nil,
		},
		{
			"bag full",
			engine.NewBagFull(),
			"Your bag is full.",
			nil,
		},
		{
			"gold from a kill",
			engine.NewGoldGained(15, engine.GoldSourceKill),
			"The fallen enemy drops 15 gold.",
			nil,
		},
		{
			"gold from a chest",
			engine.NewGoldGained(120, engine.GoldSourceChest),
			"The chest holds 120 gold coins!",
			nil,
		},
		{
			"gold from the boss",
			engine.NewGoldGained(250, engine.GoldSourceBoss),
			"The fallen boss drops 250 gold!",
			nil,
		},
		{
			"boss challenge unlocked",
			engine.NewBossUnlocked(),
			"The air trembles — the section's boss awaits!",
			nil,
		},
		{
			"boss challenge start",
			engine.NewBossChallenge([]entities.Monster{goblin}),
			"",
			[]string{"You face the section's boss:", "Goblin (30 HP)"},
		},
		{
			"gold from an unknown source",
			engine.NewGoldGained(7, "mystery"),
			"You gain 7 gold.",
			nil,
		},
		{
			"merchant found",
			engine.NewMerchantFound(3),
			"A wandering merchant appears — 3 wares for sale.",
			nil,
		},
		{
			"item bought",
			engine.NewItemBought(testItem(), 250),
			"You buy the Espada de Madeira (+5 DMG) for 250 gold.",
			nil,
		},
		{
			"item bought without an item payload",
			engine.Event{Type: engine.EventItemBought, Data: map[string]any{"price": 50}},
			"You buy the wares for 50 gold.",
			nil,
		},
		{
			"item sold",
			engine.NewItemSold(testItem(), 125),
			"The merchant pays 125 gold for the Espada de Madeira (+5 DMG).",
			nil,
		},
		{
			"item sold without an item payload",
			engine.Event{Type: engine.EventItemSold, Data: map[string]any{"price": 50}},
			"The merchant pays 50 gold for the wares.",
			nil,
		},
		{
			"hero uses a move",
			engine.NewMoveUsed("Hero", "Power Slash"),
			"You use Power Slash!",
			nil,
		},
		{
			"monster uses a move",
			engine.NewMoveUsed("Goblin", "Venom Bite"),
			"The Goblin uses Venom Bite!",
			nil,
		},
		{
			"debuff lands on the enemy",
			engine.Event{Type: engine.EventStatusApplied, Data: map[string]any{
				"target": "Goblin", "status": "Poison", "turns": 3, "kind": "debuff",
			}},
			"The Goblin suffers Poison (3 turns).",
			nil,
		},
		{
			"buff lands on the hero",
			engine.Event{Type: engine.EventStatusApplied, Data: map[string]any{
				"target": "Hero", "status": "Guard", "turns": 2, "kind": "buff",
			}},
			"You gain Guard (2 turns).",
			nil,
		},
		{
			"debuff lands on the hero",
			engine.Event{Type: engine.EventStatusApplied, Data: map[string]any{
				"target": "Hero", "status": "Poison", "turns": 3, "kind": "debuff",
			}},
			"You suffer Poison (3 turns).",
			nil,
		},
		{
			"buff lands on the enemy",
			engine.Event{Type: engine.EventStatusApplied, Data: map[string]any{
				"target": "Goblin", "status": "Guard", "turns": 2, "kind": "buff",
			}},
			"The Goblin gains Guard (2 turns).",
			nil,
		},
		{
			"status expires",
			engine.Event{Type: engine.EventStatusExpired, Data: map[string]any{
				"target": "Goblin", "status": "Poison", "kind": "debuff",
			}},
			"The Poison wears off.",
			nil,
		},
		{
			"dot tick with exact HP",
			engine.Event{Type: engine.EventStatusDamage, Data: map[string]any{
				"target": "Hero", "status": "Poison", "amount": 3, "kind": "debuff",
				"target_hp_exact": 12, "target_max_hp": 45,
			}},
			"The Poison eats 3 HP (12/45 left).",
			nil,
		},
		{
			"dot tick without exact HP stays truthful",
			engine.Event{Type: engine.EventStatusDamage, Data: map[string]any{
				"target": "Goblin", "status": "Poison", "amount": 3, "kind": "debuff",
			}},
			"The Poison eats 3 HP.",
			nil,
		},
		{
			"shrine found names the offer",
			engine.NewShrineFound(entities.Move{ID: "power_slash", Name: "Power Slash", Tier: 2}),
			"A shrine offers to teach you Power Slash.",
			nil,
		},
		{
			"shrine found without a move payload",
			engine.Event{Type: engine.EventShrineFound, Data: map[string]any{}},
			"A shrine stirs, but its teaching is lost.",
			nil,
		},
		{
			"move learned",
			engine.NewMoveLearned(entities.Move{ID: "power_slash", Name: "Power Slash"}, nil),
			"You learned Power Slash!",
			nil,
		},
		{
			"move learned with a forgotten move",
			engine.NewMoveLearned(entities.Move{ID: "power_slash", Name: "Power Slash"},
				&entities.Move{ID: "guard_strike", Name: "Guard Strike"}),
			"You forget Guard Strike and learn Power Slash!",
			nil,
		},
		{
			"move learned without a move payload",
			engine.Event{Type: engine.EventMoveLearned, Data: map[string]any{}},
			"You learn something, but its name escapes you.",
			nil,
		},
		{
			"message",
			engine.NewMessage("Cannot ascend yet"),
			"Cannot ascend yet",
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eventText(tt.event)
			if tt.contains == nil {
				if got != tt.want {
					t.Errorf("eventText = %q, want %q", got, tt.want)
				}
				return
			}
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("eventText = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

// Status tones follow the status KIND: debuffs render soft-red, buffs
// green — regardless of which side they landed on.
func TestStatusTonesFollowKind(t *testing.T) {
	withKind := func(kind string) engine.Event {
		return engine.Event{Type: engine.EventStatusApplied, Data: map[string]any{"kind": kind}}
	}

	if got, want := toneOf(withKind("debuff")).Render("x"), styleHurt.Render("x"); got != want {
		t.Error("a debuff status must take the soft-red tone")
	}
	if got, want := toneOf(withKind("buff")).Render("x"), styleHeal.Render("x"); got != want {
		t.Error("a buff status must take the green tone")
	}
	if got, want := toneOf(engine.Event{Type: engine.EventStatusDamage, Data: map[string]any{"kind": "debuff"}}).Render("x"), styleHurt.Render("x"); got != want {
		t.Error("a DoT tick must take the soft-red tone")
	}
	if got, want := toneOf(engine.Event{Type: engine.EventMoveUsed}).Render("x"), styleDim.Render("x"); got != want {
		t.Error("a move-used line must stay quiet (dim)")
	}
}
