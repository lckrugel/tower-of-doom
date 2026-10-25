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

// Exploring into a sanctuary heals 50% of the hero's BASE max HP
// (equipment excluded), instantly and without unlocking ascending.
func TestExploreSanctuary(t *testing.T) {
	e := pastFirstExplore(newKindEngine(10)) // Intn(30) = 10 → sanctuary
	// A cheap item to show the % is taken over the base max HP, not MaxHP().
	helm := entities.NewItemFromAsset(game.ItemAsset{
		ID: "helm", Name: "Helm", Slot: "helmet", BonusHP: 50,
	})
	e.Player.EquipItem(&helm)
	e.Player.CurrentHP = 50 // MaxHP 150, BaseMaxHP 100 → heal 50

	events := e.Process(Command{Type: CmdExplore})

	if got := eventTypes(events); !typesEqual(got, EventHeroHealed) {
		t.Fatalf("events = %v, want [hero-healed]", got)
	}
	healed, _ := events[0].Data["healed"].(int)
	if healed != 50 {
		t.Errorf("healed = %d, want 50 (50%% of baseMaxHP 100)", healed)
	}
	// Exact defender state after the heal.
	if hp, _ := events[0].Data["hero_hp_exact"].(int); hp != 100 {
		t.Errorf("hero_hp_exact = %d, want 100", hp)
	}
	if mhp, _ := events[0].Data["hero_max_hp"].(int); mhp != 150 {
		t.Errorf("hero_max_hp = %d, want 150", mhp)
	}
	// Legacy percentage key keeps its meaning: 100/150, floored.
	if pct, _ := events[0].Data["hero_hp"].(int); pct != 66 {
		t.Errorf("hero_hp = %d, want 66 (legacy percentage key untouched)", pct)
	}
	if hp := e.Player.CurrentHP; hp != 100 {
		t.Errorf("hero HP = %d, want 100", hp)
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
	// The test registry has no armor pieces: the ascend emits the
	// floor event plus the empty-pool message, consuming no rng.
	if got := eventTypes(e.Process(Command{Type: CmdAscend})); !typesEqual(got, EventFloorEntered, EventMessage) {
		t.Fatalf("ascend events = %v, want [floor-entered message]", got)
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
	// Merchant kind 30 twice (the offer picks drain the shrinking pool —
	// 0,1,0,0 are valid for Intn(4/3/2/1); upgrade check 99 → none; LEAVE
	// consumes no rng), then a third merchant kind 30 → overridden; picks
	// 104 → orc (5), 0 → goblin (3).
	e := merchantEngine(30, 0, 1, 0, 0, 99, 30, 0, 1, 0, 0, 99, 30, 104, 0)
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
	// 15 = 2 × (kind + 4 offers + upgrade check) + the overridden kind's
	// one Intn + 2 monster picks; a reroll would shift the count.
	if stub.calls != 15 {
		t.Errorf("rng calls = %d, want 15 (no extra rng for the override)", stub.calls)
	}
}

// ...and the mirror of the chest case: after a merchant pair, a
// legitimately-rolled SANCTUARY still happens — only the third SAME kind
// is blocked.
func TestDifferentKindAfterMerchantPairHappens(t *testing.T) {
	// Merchant kind 30 twice (the offer picks drain the shrinking pool —
	// 0,1,0,0 are valid for Intn(4/3/2/1); upgrade check 99 → none), then
	// sanctuary kind 10 — a different non-combat kind, so no override.
	e := merchantEngine(30, 0, 1, 0, 0, 99, 30, 0, 1, 0, 0, 99, 10)

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
	// The test registry has no armor pieces: the ascend emits the floor
	// event plus the empty-pool message, consuming no rng.
	if got := eventTypes(e.Process(Command{Type: CmdAscend})); !typesEqual(got, EventFloorEntered, EventMessage) {
		t.Fatalf("ascend events = %v, want [floor-entered message]", got)
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

// ——— Shrines: the fifth encounter kind ———

// shrineSection is the mixed table extended with a shrine weight: with the
// extended roll order [combat, sanctuary, chest, merchant, shrine] over a
// total of 40, the shrine bucket is the LAST one (rng 30–39; the zeroed
// merchant weight between chest and shrine is a zero-width bucket).
func shrineSection() game.SectionAsset {
	s := mixedSection()
	s.EncounterTypes.Shrine = 10
	s.MoveTiers = []int{1, 2}
	return s
}

// shrineEngine is an engine over the shrine section with the move fixtures
// (testMoves) in the registry, past the run's forced first combat. Its
// move pool (tier ≤ 2, sorted by id) is [basic_attack, heavy_slash,
// quick_stab]; the canonical basic_attack is never offered, so the
// filtered pool starts as [heavy_slash, quick_stab].
func shrineEngine(rngValues ...int) *Engine {
	e := newTestEngine(rngValues...)
	e.reg.Sections = []game.SectionAsset{shrineSection()}
	e.reg.Moves = testMoves()
	return pastFirstExplore(e)
}

// The shrine bucket sits AFTER the merchant's: with all five weights at
// 10 the buckets roll as [combat 0-9, sanctuary 10-19, chest 20-29,
// merchant 30-39, shrine 40-49].
func TestShrineBucketSitsAfterMerchant(t *testing.T) {
	s := mixedSection()
	s.EncounterTypes.Merchant = 10
	s.EncounterTypes.Shrine = 10

	tests := []struct {
		name string
		rng  int
		want EncounterType
	}{
		{"merchant starts at 30", 30, EncounterMerchant},
		{"merchant still owns 39", 39, EncounterMerchant},
		{"shrine starts at 40", 40, EncounterShrine},
		{"shrine owns the last bucket", 49, EncounterShrine},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(tt.rng)
			e.reg.Sections = []game.SectionAsset{s}

			kind := e.rollEncounterType(s.EncounterTypes)

			if kind != tt.want {
				t.Errorf("rollEncounterType(rng=%d) = %v, want %v", tt.rng, kind, tt.want)
			}
		})
	}
}

// shrineCleared reports the PendingLoot discipline over the shrine's
// pending state: nothing survives a resolution path.
func shrineCleared(e *Engine) bool {
	return e.PendingMoves == nil && e.PendingMove == nil && e.ChosenMove == nil
}

// Exploring into a shrine rolls ONE OR TWO offers from the filtered
// pool — the pool SHRINKING after the first pick, so the offers never
// duplicate — and opens StateLearn in the choosing step: nothing is
// learned until the player picks, ascending untouched.
func TestExploreShrineOffersTwoMoves(t *testing.T) {
	// rng 30 → shrine kind; Intn(2) = 0 → heavy_slash, then Intn(1) = 0
	// → quick_stab (the pool shrank after the first pick).
	e := shrineEngine(30, 0, 0)

	events := e.Process(Command{Type: CmdExplore})

	if got := eventTypes(events); !typesEqual(got, EventShrineOffered) {
		t.Fatalf("events = %v, want [shrine-offered]", got)
	}
	if count, _ := events[0].Data["count"].(int); count != 2 {
		t.Errorf("shrine-offered count = %v, want 2", events[0].Data["count"])
	}
	if len(e.PendingMoves) != 2 || e.PendingMoves[0].ID != "heavy_slash" || e.PendingMoves[1].ID != "quick_stab" {
		t.Errorf("pending moves = %+v, want [heavy_slash quick_stab] (distinct, pool shrinking)", e.PendingMoves)
	}
	if e.PendingMove == nil || e.PendingMove.ID != "heavy_slash" {
		t.Errorf("deprecated mirror = %+v, want the FIRST offer", e.PendingMove)
	}
	if e.State != StateLearn {
		t.Errorf("state = %v, want StateLearn (the choosing step)", e.State)
	}
	if len(e.Player.LearnedMoves()) != 0 {
		t.Error("the shrine learned without the player's choice")
	}
	if e.CanAscend {
		t.Error("shrine must not unlock ascending (only victory does)")
	}
	if e.CurrentEncounter.Type != EncounterCombat {
		t.Errorf("current encounter = %v, want zeroed", e.CurrentEncounter.Type)
	}
}

// The choosing step under the learned cap: CHOOSE picks an offer and it
// is learned on the spot — exploring continues, everything cleared, no
// rng consumed by the decision.
func TestShrineChooseUnderCapLearnsOnTheSpot(t *testing.T) {
	t.Run("the first offer", func(t *testing.T) {
		// rng 30 → shrine kind; offers 0, 0 → [heavy_slash, quick_stab].
		e := shrineEngine(30, 0, 0)
		e.Process(Command{Type: CmdExplore})
		stub := &stubRng{values: []int{0}}
		e.rng = stub

		events := e.Process(Command{Type: CmdChooseOffer, Index: 0})

		if got := eventTypes(events); !typesEqual(got, EventMoveLearned) {
			t.Fatalf("events = %v, want [move-learned]", got)
		}
		if move, _ := events[0].Data["move"].(entities.Move); move.ID != "heavy_slash" {
			t.Errorf("learned move = %+v, want heavy_slash (offer 0)", move)
		}
		if _, hasForgotten := events[0].Data["forgotten"]; hasForgotten {
			t.Error("under-cap learning carries no forgotten key")
		}
		learned := e.Player.LearnedMoves()
		if len(learned) != 1 || learned[0].ID != "heavy_slash" {
			t.Errorf("learned set = %+v, want [heavy_slash]", learned)
		}
		if e.State != StateExploring {
			t.Errorf("state = %v, want StateExploring (learned on the spot)", e.State)
		}
		if !shrineCleared(e) {
			t.Error("the pending offers survived the choose")
		}
		if stub.calls != 0 {
			t.Errorf("choosing consumed %d rng calls, want 0", stub.calls)
		}
	})

	t.Run("the second offer", func(t *testing.T) {
		e := shrineEngine(30, 0, 0)
		e.Process(Command{Type: CmdExplore})

		events := e.Process(Command{Type: CmdChooseOffer, Index: 1})

		if move, _ := events[0].Data["move"].(entities.Move); move.ID != "quick_stab" {
			t.Errorf("learned move = %+v, want quick_stab (offer 1)", move)
		}
	})
}

// The shrine's rng contract: ONE Intn PER offer (1–2 total) at shrine
// resolution, over the shrinking filtered pool — the kind roll plus the
// offer rolls, nothing else. Choosing, forgetting and declining consume
// no rng.
func TestShrineOfferRngContract(t *testing.T) {
	t.Run("two offers = exactly two offer rolls", func(t *testing.T) {
		e := shrineEngine()
		stub := &stubRng{values: []int{30, 0, 0}}
		e.rng = stub

		e.Process(Command{Type: CmdExplore})

		if stub.calls != 3 {
			t.Errorf("rng calls = %d, want 3 (kind roll + the TWO offer rolls)", stub.calls)
		}
	})

	t.Run("a one-move pool = one offer roll", func(t *testing.T) {
		// quick_stab learned: the filtered pool holds heavy_slash only.
		e := shrineEngine(30, 0)
		stub := e.rng.(*stubRng)
		learn(e, "quick_stab")

		e.Process(Command{Type: CmdExplore})

		if stub.calls != 2 {
			t.Errorf("rng calls = %d, want 2 (kind roll + the ONE offer roll)", stub.calls)
		}
	})

	t.Run("the decision consumes no rng", func(t *testing.T) {
		// rng 30 → shrine kind; offers 0, 0 → [heavy_slash, quick_stab].
		t.Run("choosing", func(t *testing.T) {
			e := shrineEngine(30, 0, 0)
			e.Process(Command{Type: CmdExplore})
			stub := &stubRng{values: []int{0}}
			e.rng = stub

			e.Process(Command{Type: CmdChooseOffer, Index: 1})

			if stub.calls != 0 {
				t.Errorf("choosing consumed %d rng calls, want 0", stub.calls)
			}
		})

		t.Run("declining", func(t *testing.T) {
			e := shrineEngine(30, 0, 0)
			e.Process(Command{Type: CmdExplore})
			stub := &stubRng{values: []int{0}}
			e.rng = stub

			e.Process(Command{Type: CmdDecline})

			if stub.calls != 0 {
				t.Errorf("declining consumed %d rng calls, want 0", stub.calls)
			}
		})
	})
}

// The offer pool filters out moves the hero already knows — shrinking
// before the offers roll.
func TestShrineOffersFilterKnownMoves(t *testing.T) {
	// heavy_slash learned: the only unknown pool move left is quick_stab.
	e := shrineEngine(30, 0)
	learn(e, "heavy_slash")

	e.Process(Command{Type: CmdExplore})

	if len(e.PendingMoves) != 1 || e.PendingMoves[0].ID != "quick_stab" {
		t.Errorf("pending moves = %+v, want [quick_stab] (heavy_slash is known)", e.PendingMoves)
	}
}

// Weapon-granted moves count as known too: a hero wielding a blade that
// grants heavy_slash is never offered it.
func TestShrineSkipsWeaponGrantedMoves(t *testing.T) {
	e := shrineEngine(30, 0)
	weapon := e.itemFromAsset(game.ItemAsset{
		ID: "fang_blade", Name: "Fang Blade", Slot: "weapon", Moves: []string{"heavy_slash"},
	})
	e.Player.EquipItem(&weapon)

	e.Process(Command{Type: CmdExplore})

	if len(e.PendingMoves) != 1 || e.PendingMoves[0].ID != "quick_stab" {
		t.Errorf("pending moves = %+v, want [quick_stab] (the granted heavy_slash filtered out)", e.PendingMoves)
	}
}

// The canonical basic_attack is never offered: it is the move-less
// fallback every combatant already fights with — teaching it would waste
// one of the two learned slots.
func TestShrineNeverOffersTheCanonicalBasicAttack(t *testing.T) {
	e := shrineEngine(30)
	e.reg.Moves = map[string]game.MoveAsset{
		"basic_attack": {ID: "basic_attack", Name: "Basic Attack", Tier: 1, DmgModifier: 100, Weight: 100},
	}

	events := e.Process(Command{Type: CmdExplore})

	if got := eventTypes(events); !typesEqual(got, EventMessage) {
		t.Fatalf("events = %v, want [message]", got)
	}
	if len(e.Player.LearnedMoves()) != 0 || !shrineCleared(e) {
		t.Error("the shrine offered the canonical basic attack")
	}
}

// A shrine whose filtered pool is empty (everything it could offer is
// already known, or the registry has no moves at all) resolves as a
// message: exploring continues, no decision opened, NO offer roll.
func TestShrineWithNothingNewToOffer(t *testing.T) {
	t.Run("every pool move is known", func(t *testing.T) {
		e := shrineEngine(30)
		stub := e.rng.(*stubRng)
		learn(e, "quick_stab", "heavy_slash")

		events := e.Process(Command{Type: CmdExplore})

		if got := eventTypes(events); !typesEqual(got, EventMessage) {
			t.Fatalf("events = %v, want [message]", got)
		}
		if msg := events[0].Data["message"]; msg != "The shrine holds nothing new for you." {
			t.Errorf("message = %q, want the nothing-new flavor", msg)
		}
		if e.State != StateExploring || !shrineCleared(e) {
			t.Errorf("state = %v, cleared = %v, want Exploring/true", e.State, shrineCleared(e))
		}
		if stub.calls != 1 {
			t.Errorf("rng calls = %d, want 1 (an empty pool consumes no offer roll)", stub.calls)
		}
	})

	t.Run("a registry without moves", func(t *testing.T) {
		e := shrineEngine(30)
		stub := e.rng.(*stubRng)
		e.reg.Moves = map[string]game.MoveAsset{}

		events := e.Process(Command{Type: CmdExplore})

		if got := eventTypes(events); !typesEqual(got, EventMessage) {
			t.Fatalf("events = %v, want [message]", got)
		}
		if stub.calls != 1 {
			t.Errorf("rng calls = %d, want 1 (an empty pool consumes no offer roll)", stub.calls)
		}
	})
}

// StateLearn ALWAYS begins in the choosing step — at the learned cap
// too: the player picks an offer first; only the choice opens the
// forget prompt.
func TestExploreShrineAtCapOpensTheChooseStep(t *testing.T) {
	e := shrineEngine(30, 0)
	learn(e, "quick_stab", "heavy_slash")
	e.reg.Moves["power_surge"] = game.MoveAsset{
		ID: "power_surge", Name: "Power Surge", Tier: 2, DmgModifier: 150, Weight: 10,
	}

	events := e.Process(Command{Type: CmdExplore})

	if got := eventTypes(events); !typesEqual(got, EventShrineOffered) {
		t.Fatalf("events = %v, want [shrine-offered]", got)
	}
	if count, _ := events[0].Data["count"].(int); count != 1 {
		t.Errorf("count = %v, want 1 (power_surge is the only unknown pool move)", events[0].Data["count"])
	}
	if e.State != StateLearn {
		t.Fatalf("state = %v, want StateLearn (the choosing step)", e.State)
	}
	if len(e.PendingMoves) != 1 || e.PendingMoves[0].ID != "power_surge" {
		t.Errorf("pending moves = %+v, want [power_surge]", e.PendingMoves)
	}
	if e.ChosenMove != nil {
		t.Error("a choice was recorded before the player picked")
	}
	if len(e.Player.LearnedMoves()) != 2 {
		t.Error("the offer was learned without the player's choice")
	}
	if e.CanAscend {
		t.Error("shrine must not unlock ascending")
	}
}

// shrineAtCap is an engine in StateLearn's CHOOSING step: the hero's two
// learned moves are in and the shrine offers ONE unlearned move
// (power_surge — the only unknown left in the tier pool).
func shrineAtCap() *Engine {
	e := shrineEngine(30, 0)
	learn(e, "quick_stab", "heavy_slash")
	e.reg.Moves["power_surge"] = game.MoveAsset{
		ID: "power_surge", Name: "Power Surge", Tier: 2, DmgModifier: 150, Weight: 10,
	}
	if events := e.Process(Command{Type: CmdExplore}); len(events) != 1 || events[0].Type != EventShrineOffered {
		panic("test fixture: expected a shrine offer (StateLearn, choosing step)")
	}
	return e
}

// shrineForgetting is shrineAtCap one command later: the offer was
// CHOSEN at the cap — the engine is in StateLearn's forgetting step,
// the learned set untouched.
func shrineForgetting() *Engine {
	e := shrineAtCap()
	if events := e.Process(Command{Type: CmdChooseOffer, Index: 0}); len(events) != 1 || events[0].Type != EventMessage {
		panic("test fixture: expected the forget prompt (StateLearn, forgetting step)")
	}
	if e.State != StateLearn || e.ChosenMove == nil || e.ChosenMove.ID != "power_surge" {
		panic("test fixture: expected the forgetting step over power_surge")
	}
	return e
}

// shrineTwoOffersAtCap is an engine in StateLearn's choosing step with
// the hero AT the cap and TWO offers pending: the section's move tiers
// widen to 1-3 so the filtered pool (the hero knows quick_stab and
// heavy_slash) holds two unknowns.
func shrineTwoOffersAtCap() *Engine {
	// rng 30 → shrine kind; Intn(2) = 0 → assassins_strike, Intn(1) = 0
	// → power_surge.
	e := shrineEngine(30, 0, 0)
	learn(e, "quick_stab", "heavy_slash")
	e.reg.Moves["power_surge"] = game.MoveAsset{
		ID: "power_surge", Name: "Power Surge", Tier: 2, DmgModifier: 150, Weight: 10,
	}
	e.reg.Sections[0].MoveTiers = []int{1, 2, 3}
	if events := e.Process(Command{Type: CmdExplore}); len(events) != 1 || events[0].Type != EventShrineOffered {
		panic("test fixture: expected a two-offer shrine (StateLearn)")
	}
	if len(e.PendingMoves) != 2 {
		panic("test fixture: want two pending offers")
	}
	return e
}

// Choosing at the cap does NOT learn yet: the choice is recorded and
// StateLearn advances to the forgetting step — the learned set is
// untouched until a move is forgotten (or the offer declined). The
// choice consumes no rng, and the deprecated TUI mirror re-points at
// the chosen offer.
func TestShrineChooseAtCapAdvancesToTheForgettingStep(t *testing.T) {
	e := shrineTwoOffersAtCap()
	stub := &stubRng{values: []int{0}}
	e.rng = stub

	events := e.Process(Command{Type: CmdChooseOffer, Index: 1})

	if got := eventTypes(events); !typesEqual(got, EventMessage) {
		t.Fatalf("events = %v, want [message (the forget prompt)]", got)
	}
	if e.State != StateLearn {
		t.Errorf("state = %v, want StateLearn (the forgetting step)", e.State)
	}
	if e.ChosenMove == nil || e.ChosenMove.ID != "power_surge" {
		t.Errorf("chosen move = %+v, want power_surge (offer 1)", e.ChosenMove)
	}
	if len(e.Player.LearnedMoves()) != 2 {
		t.Error("the choice learned without the forget")
	}
	if len(e.PendingMoves) != 2 {
		t.Errorf("pending moves = %+v, want both offers still held", e.PendingMoves)
	}
	if e.PendingMove == nil || e.PendingMove.ID != "power_surge" {
		t.Errorf("deprecated mirror = %+v, want the chosen offer", e.PendingMove)
	}
	if stub.calls != 0 {
		t.Errorf("choosing consumed %d rng calls, want 0", stub.calls)
	}
}

// Forgetting a learned move makes room for the CHOSEN offer: it is
// learned in the forgotten move's place, reported with the additive
// forgotten key — from either forget index, of either offer.
func TestShrineForgetLearnsTheChosenOffer(t *testing.T) {
	tests := []struct {
		name          string
		choose        int
		forget        int
		wantForgotten string
		wantLearned   []string
	}{
		{"the chosen second offer replaces the first learned", 1, 0, "quick_stab", []string{"heavy_slash", "power_surge"}},
		{"the chosen second offer replaces the second learned", 1, 1, "heavy_slash", []string{"quick_stab", "power_surge"}},
		{"the chosen first offer replaces the second learned", 0, 1, "heavy_slash", []string{"quick_stab", "assassins_strike"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := shrineTwoOffersAtCap()
			e.Process(Command{Type: CmdChooseOffer, Index: tt.choose})

			events := e.Process(Command{Type: CmdForget, Index: tt.forget})

			if got := eventTypes(events); !typesEqual(got, EventMoveLearned) {
				t.Fatalf("events = %v, want [move-learned]", got)
			}
			if move, _ := events[0].Data["move"].(entities.Move); move.ID != tt.wantLearned[1] {
				t.Errorf("learned move = %+v, want %v", move, tt.wantLearned[1])
			}
			if forgotten, _ := events[0].Data["forgotten"].(entities.Move); forgotten.ID != tt.wantForgotten {
				t.Errorf("forgotten = %+v, want %v", forgotten, tt.wantForgotten)
			}
			learned := e.Player.LearnedMoves()
			if len(learned) != 2 || learned[0].ID != tt.wantLearned[0] || learned[1].ID != tt.wantLearned[1] {
				t.Errorf("learned set = %+v, want %v", learned, tt.wantLearned)
			}
			if !shrineCleared(e) {
				t.Error("the pending offers survived the forget")
			}
			if e.State != StateExploring {
				t.Errorf("state = %v, want StateExploring", e.State)
			}
		})
	}
}

// DECLINE works in either step: everything stays as it was and
// exploring continues with nothing pending.
func TestShrineDeclineKeepsBothMoves(t *testing.T) {
	t.Run("in the forgetting step (at the cap)", func(t *testing.T) {
		e := shrineForgetting()

		events := e.Process(Command{Type: CmdDecline})

		if got := eventTypes(events); !typesEqual(got, EventMessage) {
			t.Fatalf("events = %v, want [message]", got)
		}
		if msg := events[0].Data["message"]; msg != "You leave the shrine's teaching behind." {
			t.Errorf("message = %q, want the decline flavor", msg)
		}
		if got := e.Player.LearnedMoves(); len(got) != 2 {
			t.Errorf("learned set = %+v, want both moves kept", got)
		}
		if !shrineCleared(e) {
			t.Error("the pending offers survived the decline")
		}
		if e.State != StateExploring {
			t.Errorf("state = %v, want StateExploring", e.State)
		}
	})

	t.Run("in the choosing step (under the cap)", func(t *testing.T) {
		e := shrineEngine(30, 0, 0)
		e.Process(Command{Type: CmdExplore}) // two offers, choosing step

		e.Process(Command{Type: CmdDecline})

		if len(e.Player.LearnedMoves()) != 0 {
			t.Error("the decline learned an offer")
		}
		if !shrineCleared(e) {
			t.Error("the pending offers survived the decline")
		}
		if e.State != StateExploring {
			t.Errorf("state = %v, want StateExploring", e.State)
		}
	})
}

// An out-of-range forget index is a generic invalid command: no state
// change, the decision stays open, no panic, no rng.
func TestShrineForgetOutOfRangeIndexIsRejected(t *testing.T) {
	e := shrineForgetting()
	stub := &stubRng{values: []int{0}}
	e.rng = stub

	events := e.Process(Command{Type: CmdForget, Index: 2})

	if got := eventTypes(events); !typesEqual(got, EventMessage) {
		t.Fatalf("events = %v, want [message (invalid command)]", got)
	}
	if e.State != StateLearn {
		t.Errorf("state = %v, want StateLearn (rejected, decision intact)", e.State)
	}
	if e.ChosenMove == nil || len(e.PendingMoves) != 1 {
		t.Error("the rejected forget consumed the pending offers")
	}
	if len(e.Player.LearnedMoves()) != 2 {
		t.Error("the rejected forget changed the learned set")
	}
	if stub.calls != 0 {
		t.Errorf("a rejected forget consumed %d rng calls, want 0", stub.calls)
	}
}

// An out-of-range CHOOSE index (negative or past the offers) is a
// generic invalid command too: the choosing step stays open, nothing
// learned, no rng.
func TestShrineChooseOfferOutOfRangeIndexIsRejected(t *testing.T) {
	tests := []struct {
		name  string
		index int
	}{
		{"negative", -1},
		{"past the offers", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := shrineTwoOffersAtCap()
			stub := &stubRng{values: []int{0}}
			e.rng = stub

			events := e.Process(Command{Type: CmdChooseOffer, Index: tt.index})

			if got := eventTypes(events); !typesEqual(got, EventMessage) {
				t.Fatalf("events = %v, want [message (invalid command)]", got)
			}
			if e.State != StateLearn {
				t.Errorf("state = %v, want StateLearn (rejected, decision intact)", e.State)
			}
			if e.ChosenMove != nil || len(e.Player.LearnedMoves()) != 2 {
				t.Error("the rejected choose changed the hero")
			}
			if stub.calls != 0 {
				t.Errorf("a rejected choose consumed %d rng calls, want 0", stub.calls)
			}
		})
	}
}

// CmdForget is a FORGETTING-step command: in the choosing step it is
// out of the step's command matrix — a generic invalid command with no
// state change.
func TestShrineForgetInTheChooseStepIsRejected(t *testing.T) {
	e := shrineTwoOffersAtCap()

	events := e.Process(Command{Type: CmdForget, Index: 0})

	if got := eventTypes(events); !typesEqual(got, EventMessage) {
		t.Fatalf("events = %v, want [message (invalid command)]", got)
	}
	if e.State != StateLearn {
		t.Errorf("state = %v, want StateLearn (rejected, decision intact)", e.State)
	}
	if e.ChosenMove != nil || len(e.Player.LearnedMoves()) != 2 {
		t.Error("the rejected forget changed the hero")
	}
}

// Shrines are non-combat kinds: three in a row are impossible — the
// third roll is overridden to combat (the pacing rule covers every
// non-combat kind, shrines included).
func TestShrineStreakBecomesCombat(t *testing.T) {
	// rng: kind 30 + two offers (0, 0) twice — each shrine DECLINED, so
	// the learned set (and with it the third pool) stays the same — then
	// a third kind 30 → overridden to combat; picks 104 → orc (5), 0 →
	// goblin (3).
	e := shrineEngine(30, 0, 0, 30, 0, 0, 30, 104, 0)
	stub := e.rng.(*stubRng)

	e.Process(Command{Type: CmdExplore}) // shrine #1 → two offers
	e.Process(Command{Type: CmdDecline})
	e.Process(Command{Type: CmdExplore}) // shrine #2 → two offers
	e.Process(Command{Type: CmdDecline})

	events := e.Process(Command{Type: CmdExplore}) // third shrine roll

	if got := eventTypes(events); !typesEqual(got, EventEncounterFound) {
		t.Fatalf("events = %v, want [encounter-found] (overridden to combat)", got)
	}
	if e.State != StateEncounter {
		t.Errorf("state = %v, want StateEncounter", e.State)
	}
	if !idsEqual(monsterIDs(e.CurrentEncounter.Monsters), "orc", "goblin") {
		t.Errorf("encounter = %v, want [orc goblin] (the picks followed the ONE kind Intn)", monsterIDs(e.CurrentEncounter.Monsters))
	}
	// 9 = 2 × (kind + 2 offers) + the overridden kind's one Intn + 2
	// monster picks; a reroll for the override would shift this count.
	if stub.calls != 9 {
		t.Errorf("rng calls = %d, want 9 (no extra rng for the override)", stub.calls)
	}
}
