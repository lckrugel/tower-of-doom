package engine

import (
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// mixedSection is the standard test section with a full encounter-type
// table: equal weights for combat, sanctuary and chest.
func mixedSection() game.SectionAsset {
	s := testSections()[0]
	s.EncounterTypes = game.EncounterTypeWeights{Combat: 10, Sanctuary: 10, Chest: 10}
	return s
}

// newKindEngine is a test engine over the mixed section, with pre-programmed
// rng values. With equal weights the kinds roll as: 0–9 combat, 10–19
// sanctuary, 20–29 chest.
func newKindEngine(rngValues ...int) *Engine {
	e := newTestEngine(rngValues...)
	e.reg.Sections = []game.SectionAsset{mixedSection()}
	return e
}

// Equal weights order the roll as [combat, sanctuary, chest].
func TestRollEncounterTypeHonorsRngSequence(t *testing.T) {
	tests := []struct {
		name string
		rng  int
		want EncounterType
	}{
		{"first weight is combat", 0, EncounterCombat},
		{"still combat just under sanctuary", 9, EncounterCombat},
		{"sanctuary starts at 10", 10, EncounterSanctuary},
		{"chest at 20", 20, EncounterChest},
		{"last bucket is chest", 29, EncounterChest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newKindEngine(tt.rng)

			kind := e.rollEncounterType(mixedSection().EncounterTypes)

			if kind != tt.want {
				t.Errorf("rollEncounterType(rng=%d) = %v, want %v", tt.rng, kind, tt.want)
			}
		})
	}
}

// A zeroed table means the section is combat-only and must not consume any
// rng, so the monster rolls keep their position in the rng sequence.
func TestRollEncounterTypeWithoutTableConsumesNoRng(t *testing.T) {
	e := newKindEngine(0)
	stub := &stubRng{values: []int{0}}
	e.rng = stub

	kind := e.rollEncounterType(game.EncounterTypeWeights{})

	if kind != EncounterCombat {
		t.Errorf("zeroed table rolled %v, want combat", kind)
	}
	if stub.calls != 0 {
		t.Errorf("rollEncounterType consumed %d rng values, want 0", stub.calls)
	}
}

// Exploring into a sanctuary heals 35% of the hero's BASE max HP
// (equipment excluded), instantly and without unlocking ascending.
func TestExploreSanctuary(t *testing.T) {
	e := pastFirstExplore(newKindEngine(10)) // Intn(30) = 10 → sanctuary
	// A cheap item to show the % is taken over the base max HP, not MaxHP().
	helm := entities.NewItemFromAsset(game.ItemAsset{
		ID: "helm", Name: "Helm", Slot: "helmet", BonusHP: 50,
	})
	e.Player.EquipItem(&helm)
	e.Player.CurrentHP = 50 // MaxHP 150, BaseMaxHP 100 → heal 35

	events := e.Process(Command{Type: CmdExplore})

	if got := eventTypes(events); !typesEqual(got, EventHeroHealed) {
		t.Fatalf("events = %v, want [hero-healed]", got)
	}
	healed, _ := events[0].Data["healed"].(int)
	if healed != 35 {
		t.Errorf("healed = %d, want 35 (35%% of baseMaxHP 100)", healed)
	}
	// Exact defender state after the heal.
	if hp, _ := events[0].Data["hero_hp_exact"].(int); hp != 85 {
		t.Errorf("hero_hp_exact = %d, want 85", hp)
	}
	if mhp, _ := events[0].Data["hero_max_hp"].(int); mhp != 150 {
		t.Errorf("hero_max_hp = %d, want 150", mhp)
	}
	// Legacy percentage key keeps its meaning: 85/150, floored.
	if pct, _ := events[0].Data["hero_hp"].(int); pct != 56 {
		t.Errorf("hero_hp = %d, want 56 (legacy percentage key untouched)", pct)
	}
	if hp := e.Player.CurrentHP; hp != 85 {
		t.Errorf("hero HP = %d, want 85", hp)
	}
	// Sanctuary is a stopover: back to exploring, nothing unlocked.
	if e.State != StateExploring {
		t.Errorf("state after sanctuary = %v, want StateExploring", e.State)
	}
	if e.CanAscend {
		t.Error("sanctuary must not unlock ascending (only victory does)")
	}
	if e.CurrentEncounter.Type != EncounterCombat {
		t.Errorf("current encounter = %v, want zeroed (cleared after instant resolution)", e.CurrentEncounter.Type)
	}
}

// At full health a sanctuary heals nothing.
func TestExploreSanctuaryAtFullHealth(t *testing.T) {
	e := pastFirstExplore(newKindEngine(10))
	e.Player.CurrentHP = e.Player.MaxHP()

	events := e.Process(Command{Type: CmdExplore})

	if got := eventTypes(events); !typesEqual(got, EventHeroHealed) {
		t.Fatalf("events = %v, want [hero-healed]", got)
	}
	if healed, _ := events[0].Data["healed"].(int); healed != 0 {
		t.Errorf("healed = %d, want 0 (already at full health)", healed)
	}
	// Even a no-op heal reports the defender's exact state.
	if hp, _ := events[0].Data["hero_hp_exact"].(int); hp != e.Player.MaxHP() {
		t.Errorf("hero_hp_exact = %d, want %d (no-op heal)", hp, e.Player.MaxHP())
	}
}

// (Moved to loot_test.go: chests no longer auto-equip — they hold the item
// as PendingLoot and open the StateLoot decision.)

// A registry without any items makes an ITEM chest resolve as empty (a
// gold chest would still pay out — it needs no item registry).
func TestExploreChestWithoutItemRegistryGrantsNothing(t *testing.T) {
	e := pastFirstExplore(newKindEngine(20, 50)) // chest, then rng 50 ≥ 40 → item chest

	events := e.Process(Command{Type: CmdExplore})

	if got := eventTypes(events); !typesEqual(got, EventMessage) {
		t.Errorf("events = %v, want [message (empty chest)]", got)
	}
	if e.State != StateExploring {
		t.Errorf("state after empty chest = %v, want StateExploring", e.State)
	}
}

// A combat encounter in a mixed section still goes through StateEncounter,
// consuming one rng value for the kind and then the monster rolls.
func TestExploreCombatInMixedSection(t *testing.T) {
	// rng 5 → combat; then Intn(105)=104 → orc (5), resta 4 → 0 → goblin.
	e := pastFirstExplore(newKindEngine(5, 104, 0))

	events := e.Process(Command{Type: CmdExplore})

	if got := eventTypes(events); !typesEqual(got, EventEncounterFound) {
		t.Fatalf("events = %v, want [encounter-found]", got)
	}
	if e.State != StateEncounter {
		t.Errorf("state = %v, want StateEncounter", e.State)
	}
	if !idsEqual(monsterIDs(e.CurrentEncounter.Monsters), "orc", "goblin") {
		t.Errorf("encounter = %v, want [orc goblin]", monsterIDs(e.CurrentEncounter.Monsters))
	}
}

// ——— Encounter pacing rules ———

// The FIRST explore of a run is forced combat: the kind roll is skipped
// entirely — zero rng consumed for the kind decision — even where the
// section's table could roll sanctuaries. A sanctuary, chest or merchant
// can never open a run.
func TestFirstExploreIsForcedCombat(t *testing.T) {
	// All-zero stub: every monster pick takes the cheapest goblin; a kind
	// roll would consume a leading Intn(30) and show up in the call count.
	e := newKindEngine()
	stub := &stubRng{values: []int{0}}
	e.rng = stub

	events := e.Process(Command{Type: CmdExplore})

	if got := eventTypes(events); !typesEqual(got, EventEncounterFound) {
		t.Fatalf("events = %v, want [encounter-found] (forced combat)", got)
	}
	if e.State != StateEncounter {
		t.Errorf("state after first explore = %v, want StateEncounter", e.State)
	}
	if !idsEqual(monsterIDs(e.CurrentEncounter.Monsters), "goblin", "goblin", "goblin") {
		t.Errorf("encounter = %v, want [goblin goblin goblin]", monsterIDs(e.CurrentEncounter.Monsters))
	}
	// Exactly the three monster picks: the kind roll never touched the rng
	// (a rolled kind would add an Intn(30) ahead of the picks).
	if stub.calls != 3 {
		t.Errorf("rng calls after first explore = %d, want 3 (monster picks only)", stub.calls)
	}
}

// After the forced first combat, exploring rolls the kind table normally
// again: stub values are consumed and non-combat kinds become possible.
func TestSecondExploreResumesKindRolls(t *testing.T) {
	// Forced combat: 3 picks (0,0,0); sneak away (0); kind Intn(30) = 10
	// → sanctuary.
	e := newKindEngine()
	stub := &stubRng{values: []int{0, 0, 0, 0, 10}}
	e.rng = stub

	e.Process(Command{Type: CmdExplore}) // forced combat
	if e.State != StateEncounter {
		t.Fatalf("state after first explore = %v, want StateEncounter", e.State)
	}
	e.Process(Command{Type: CmdSneak}) // 0 → slips past

	events := e.Process(Command{Type: CmdExplore})

	if got := eventTypes(events); !typesEqual(got, EventHeroHealed) {
		t.Fatalf("events = %v, want [hero-healed] (kind roll resumed)", got)
	}
	if stub.calls != 5 {
		t.Errorf("rng calls = %d, want 5 (3 picks + sneak + kind roll)", stub.calls)
	}
}

// The first-explore flag is per RUN: ascension does not reset it, so the
// first explore after ASCEND rolls the kind table instead of repeating
// the forced combat.
func TestAscendDoesNotResetFirstExploreFlag(t *testing.T) {
	// Forced combat: 3 picks (0,0,0); sneak away (0); ASCEND (empty armor
	// pool → no rng); kind Intn(30) = 10 → sanctuary.
	e := newKindEngine()
	stub := &stubRng{values: []int{0, 0, 0, 0, 10}}
	e.rng = stub
	e.CanAscend = true

	e.Process(Command{Type: CmdExplore}) // forced combat
	e.Process(Command{Type: CmdSneak})   // away
	if got := eventTypes(e.Process(Command{Type: CmdAscend})); !typesEqual(got, EventFloorEntered) {
		t.Fatalf("ascend events = %v, want [floor-entered]", got)
	}

	events := e.Process(Command{Type: CmdExplore})

	if got := eventTypes(events); !typesEqual(got, EventHeroHealed) {
		t.Fatalf("events = %v, want [hero-healed] (flag survived ascension)", got)
	}
	if stub.calls != 5 {
		t.Errorf("rng calls = %d, want 5 (3 picks + sneak + kind roll)", stub.calls)
	}
}

// ——— Pacing rules: non-combat kind streaks ———

// A kind roll landing on the same non-combat kind as the previous two is
// overridden to combat: three identical non-combat kinds in a row never
// happen. The override is not a reroll — the original roll's single Intn
// stays the only rng consumed for that kind decision.
func TestThirdIdenticalSanctuaryBecomesCombat(t *testing.T) {
	// Forced combat: 3 picks (0,0,0); sneak away (0); kind 10 → sanctuary
	// twice; the third 10 is overridden to combat; picks 104 → orc (5),
	// 0 → goblin (3).
	e := newKindEngine(0, 0, 0, 0, 10, 10, 10, 104, 0)
	stub := e.rng.(*stubRng)

	e.Process(Command{Type: CmdExplore}) // forced combat
	e.Process(Command{Type: CmdSneak})   // away
	if got := eventTypes(e.Process(Command{Type: CmdExplore})); !typesEqual(got, EventHeroHealed) {
		t.Fatalf("second explore events = %v, want [hero-healed] (sanctuary #1)", got)
	}
	if got := eventTypes(e.Process(Command{Type: CmdExplore})); !typesEqual(got, EventHeroHealed) {
		t.Fatalf("third explore events = %v, want [hero-healed] (sanctuary #2 — a pair is fine)", got)
	}

	events := e.Process(Command{Type: CmdExplore}) // third sanctuary roll

	if got := eventTypes(events); !typesEqual(got, EventEncounterFound) {
		t.Fatalf("events = %v, want [encounter-found] (overridden to combat)", got)
	}
	if e.State != StateEncounter {
		t.Errorf("state = %v, want StateEncounter", e.State)
	}
	if !idsEqual(monsterIDs(e.CurrentEncounter.Monsters), "orc", "goblin") {
		t.Errorf("encounter = %v, want [orc goblin] (the picks followed the ONE kind Intn)", monsterIDs(e.CurrentEncounter.Monsters))
	}
	// 9 = 3 forced-combat picks + sneak + 3 kind rolls + 2 monster picks;
	// an extra (reroll) or missing kind Intn would shift this count.
	if stub.calls != 9 {
		t.Errorf("rng calls = %d, want 9 (no extra rng for the override)", stub.calls)
	}
}

// The streak rule only blocks the third SAME kind: after two sanctuaries
// a legitimately-rolled chest still happens.
func TestThirdDifferentKindStillHappens(t *testing.T) {
	// Forced combat (0,0,0) + sneak (0); sanctuaries (10, 10); chest kind
	// 20; gold check 0 < 40 → gold chest (no item registry needed).
	e := newKindEngine(0, 0, 0, 0, 10, 10, 20, 0)

	e.Process(Command{Type: CmdExplore}) // forced combat
	e.Process(Command{Type: CmdSneak})   // away
	e.Process(Command{Type: CmdExplore}) // sanctuary #1
	e.Process(Command{Type: CmdExplore}) // sanctuary #2

	events := e.Process(Command{Type: CmdExplore}) // chest roll

	if got := eventTypes(events); !typesEqual(got, EventGoldGained) {
		t.Fatalf("events = %v, want [gold-gained] (the chest happened)", got)
	}
	if e.State != StateExploring {
		t.Errorf("state = %v, want StateExploring (gold chest pays on the spot)", e.State)
	}
}

// The streak rule is kind-specific: two merchants then a MERCHANT roll is
// overridden to combat — and the override skips the merchant entirely (no
// stock is rolled).
func TestMerchantStreakBecomesCombat(t *testing.T) {
	// Merchant kind 30 twice (stock offers 0,1,0; upgrade check 99 → none
	// — LEAVE consumes no rng), then a third merchant kind 30 →
	// overridden; picks 104 → orc (5), 0 → goblin (3).
	e := merchantEngine(30, 0, 1, 0, 99, 30, 0, 1, 0, 99, 30, 104, 0)
	stub := e.rng.(*stubRng)

	for range 2 {
		if got := eventTypes(e.Process(Command{Type: CmdExplore})); !typesEqual(got, EventMerchantFound) {
			t.Fatalf("explore events = %v, want [merchant-found]", got)
		}
		if got := eventTypes(e.Process(Command{Type: CmdLeave})); len(got) != 1 || got[0] != EventMessage {
			t.Fatalf("leave events = %v, want a single message", got)
		}
	}

	events := e.Process(Command{Type: CmdExplore}) // third merchant roll

	if got := eventTypes(events); !typesEqual(got, EventEncounterFound) {
		t.Fatalf("events = %v, want [encounter-found] (overridden to combat)", got)
	}
	if e.State != StateEncounter {
		t.Errorf("state = %v, want StateEncounter", e.State)
	}
	if len(e.MerchantStock) != 0 {
		t.Errorf("stock = %v, want empty (the override skips the merchant)", e.MerchantStock)
	}
	if !idsEqual(monsterIDs(e.CurrentEncounter.Monsters), "orc", "goblin") {
		t.Errorf("encounter = %v, want [orc goblin]", monsterIDs(e.CurrentEncounter.Monsters))
	}
	// 13 = 2 × (kind + 3 offers + upgrade check) + the overridden kind's
	// one Intn + 2 monster picks; a reroll would shift the count.
	if stub.calls != 13 {
		t.Errorf("rng calls = %d, want 13 (no extra rng for the override)", stub.calls)
	}
}

// ...and the mirror of the chest case: after a merchant pair, a
// legitimately-rolled SANCTUARY still happens — only the third SAME kind
// is blocked.
func TestDifferentKindAfterMerchantPairHappens(t *testing.T) {
	// Merchant kind 30 twice, then sanctuary kind 10 — a different
	// non-combat kind, so no override.
	e := merchantEngine(30, 0, 1, 0, 99, 30, 0, 1, 0, 99, 10)

	for range 2 {
		if got := eventTypes(e.Process(Command{Type: CmdExplore})); !typesEqual(got, EventMerchantFound) {
			t.Fatalf("explore events = %v, want [merchant-found]", got)
		}
		e.Process(Command{Type: CmdLeave})
	}

	events := e.Process(Command{Type: CmdExplore}) // sanctuary roll

	if got := eventTypes(events); !typesEqual(got, EventHeroHealed) {
		t.Fatalf("events = %v, want [hero-healed] (a different kind is not blocked)", got)
	}
	if e.State != StateExploring {
		t.Errorf("state = %v, want StateExploring", e.State)
	}
}

// The kind history feeding the streak rule survives ascension (it is per
// run, like the first-explore flag) — while a fresh engine's zero-value
// history [combat, combat] can never satisfy the non-combat streak, so
// nothing overrides before any kind was recorded (TestExploreSanctuary).
func TestKindHistorySurvivesAscension(t *testing.T) {
	// Two sanctuaries (history full), then ASCEND (empty armor pool → no
	// rng), then a third sanctuary roll must still be overridden.
	e := pastFirstExplore(newKindEngine(10, 10, 10, 104, 0))
	e.CanAscend = true

	e.Process(Command{Type: CmdExplore}) // sanctuary #1
	e.Process(Command{Type: CmdExplore}) // sanctuary #2
	if got := eventTypes(e.Process(Command{Type: CmdAscend})); !typesEqual(got, EventFloorEntered) {
		t.Fatalf("ascend events = %v, want [floor-entered]", got)
	}

	events := e.Process(Command{Type: CmdExplore}) // third sanctuary roll

	if got := eventTypes(events); !typesEqual(got, EventEncounterFound) {
		t.Fatalf("events = %v, want [encounter-found] (history survived ascension)", got)
	}
	// Floor 2 difficulty is 12: orc (5), then two goblins (3 + 3).
	if !idsEqual(monsterIDs(e.CurrentEncounter.Monsters), "orc", "goblin", "goblin") {
		t.Errorf("encounter = %v, want [orc goblin goblin]", monsterIDs(e.CurrentEncounter.Monsters))
	}
}
