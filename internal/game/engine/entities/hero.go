package entities

import (
	"math"
	"slices"
)

// Status base do Herói
const (
	baseHp           = 100
	baseDmg          = 0
	baseSpd          = 10
	baseDef          = 0
	hpBylvl          = 10
	dmgByLvl         = 2
	baseReqXp        = 100
	requiredXpGrowth = 1.5
	// levelUpHealPercent is how much of the BASE max HP (equipment
	// excluded) a level-up heals, on top of the %HP rescale — floored
	// and clamped by Heal (0 at full health).
	levelUpHealPercent = 25
)

// InventoryCapacity is how many items the Hero's bag can hold. The engine
// treats "bag full" as a player-facing condition (never an overflow), so
// this constant lives here, next to the bag itself.
const InventoryCapacity = 10

type Hero struct {
	Lvl int
	Xp  int
	// Status base recomputados por nível
	BaseMaxHP int
	BaseDMG   int
	BaseSPD   int
	BaseDEF   int
	// Vida atual, persistida entre combates
	CurrentHP int
	// Gold is the hero's purse. It starts empty on a new game and only
	// grows through playing (kill/chest rewards, selling to merchants).
	Gold int
	// Itens equipados atualmente
	Head   *Item
	Body   *Item
	Feet   *Item
	Weapon *Item
	Ring   *Item
	Amulet *Item
	// bag holds stashed (unequipped) items in insertion order. Items enter
	// via StashItem and leave via EquipFromBag/DropFromBag; the engine owns
	// every disposition decision (equip, drop, destroy).
	bag []*Item
	// learnedMoves holds the hero's LEARNED moves (shrine learning) in
	// learning order, capped at MaxLearnedMoves. Weapon-granted moves are
	// NOT stored here — they ride the equipped weapon (EffectiveMoves).
	// A fresh hero starts with the Punch pre-learned in slot 1.
	learnedMoves []Move
}

func NewPlayer(starterItems ...*Item) *Hero {
	hero := &Hero{
		Lvl:       1,
		Xp:        0,
		Gold:      0,
		BaseMaxHP: baseHp,
		CurrentHP: baseHp,
		BaseDMG:   baseDmg,
		BaseSPD:   baseSpd,
		BaseDEF:   baseDef,
	}
	// The punch start: every hero is born knowing how to throw a fist.
	// It is PRE-LEARNED into learned slot 1 of MaxLearnedMoves (2) — the
	// opening set is Punch (◆) + the starter weapon's explicit grants,
	// and a late shrine may replace it through the forget flow like any
	// other learned move. No rng, no registry: the move is canonical.
	hero.learnedMoves = append(hero.learnedMoves, Punch())

	for _, i := range starterItems {
		hero.EquipItem(i)
	}

	return hero
}

func (h *Hero) equippedItems() []*Item {
	items := []*Item{h.Head, h.Body, h.Feet, h.Weapon, h.Ring, h.Amulet}
	result := make([]*Item, 0, len(items))
	for _, i := range items {
		if i != nil {
			result = append(result, i)
		}
	}
	return result
}

func (h *Hero) Name() string {
	return "Hero"
}

func (h *Hero) Damage() int {
	total := h.BaseDMG
	for _, i := range h.equippedItems() {
		total += i.BonusDMG
	}
	return total
}

func (h *Hero) Speed() int {
	total := h.BaseSPD
	for _, i := range h.equippedItems() {
		total += i.BonusSPD
	}
	return total
}

func (h *Hero) MaxHP() int {
	total := h.BaseMaxHP
	for _, i := range h.equippedItems() {
		total += i.BonusHP
	}
	return total
}

func (h *Hero) Defense() int {
	total := h.BaseDEF
	for _, i := range h.equippedItems() {
		total += i.BonusDEF
	}
	return total
}

func (h *Hero) CurrentHPPercent() int {
	return int(math.Floor(float64(h.CurrentHP) / float64(h.MaxHP()) * 100))
}

// HP reports the exact current HP value (Combatant interface;
// CurrentHPPercent is its percentage view).
func (h *Hero) HP() int { return h.CurrentHP }

func (h *Hero) CritThreshold() int {
	return 10
}

func (h *Hero) IsAlive() bool {
	return h.CurrentHP > 0
}

// GiveXP grants XP, applying every level up it unlocks. Each level up
// also heals (see levelUp); the total HP actually healed across all
// applied level ups is returned, so callers can report it. A dead hero
// is a no-op returning 0.
func (h *Hero) GiveXP(xp int) int {
	if !h.IsAlive() {
		return 0
	}
	healed := 0
	h.Xp += xp
	for h.Xp >= h.requiredXp() {
		h.Xp -= h.requiredXp()
		healed += h.levelUp()
	}
	return healed
}

// levelUp raises the base stats, rescales the current HP as a fraction of
// the pre-change max (rescaleHP) and then heals levelUpHealPercent of the
// (new) base max HP — equipment excluded, floored, clamped by Heal, so a
// full-health Hero heals nothing and HP never exceeds the max. Returns
// the amount actually healed.
func (h *Hero) levelUp() int {
	if !h.IsAlive() {
		return 0
	}
	// Guarda o HP máximo de antes para reescalar a vida atual proporcionalmente.
	oldMax := h.MaxHP()
	// Raises base stats
	h.BaseMaxHP += hpBylvl
	h.BaseDMG += dmgByLvl
	h.rescaleHP(oldMax, h.MaxHP())
	h.Lvl++
	// The level-up heal: computed from the base max HP AFTER the stat
	// raise (equipment never counts).
	return h.Heal(h.BaseMaxHP * levelUpHealPercent / 100)
}

// XpToLevel reports the XP still needed to reach the next level
// (for UI/progress display).
func (h *Hero) XpToLevel() int {
	return h.requiredXp() - h.Xp
}

func (h *Hero) requiredXp() int {
	return int(baseReqXp * math.Pow(float64(max(h.Lvl-1, 1)), requiredXpGrowth))
}

// Heals the Hero with a set amount.
// Returns the amount that was actually healed.
func (h *Hero) Heal(heal int) int {
	healDone := min(heal, h.MaxHP()-h.CurrentHP)
	h.CurrentHP += healDone
	return healDone
}

// TakeDmg applies damage as a plain clamp: negative damage is ignored
// (never heals), HP floors at 0 and a dead Hero is never resurrected by
// this call. Mitigation (the defense damage curve) is the engine's job —
// it applies effectiveDamage(swing, DEF) BEFORE this clamp (combat.go).
// Returns the damage actually subtracted (clamped to the current HP).
func (h *Hero) TakeDmg(dmg int) int {
	dmgReceived := max(0, min(dmg, h.CurrentHP))
	h.CurrentHP -= dmgReceived
	return dmgReceived
}

// rescaleHP reaplica o % de vida atual após uma mudança no HP máximo.
//
// oldMax deve ser o HP máximo ANTES da mudança; a vida atual é preservada
// como fração do máximo (floor), limitada ao novo máximo.
func (h *Hero) rescaleHP(oldMax, newMax int) {
	if oldMax <= 0 || newMax <= 0 {
		// Sem máximo válido para escalar — mantém CurrentHP como está.
		return
	}
	scaled := h.CurrentHP * newMax / oldMax
	if scaled < 0 {
		scaled = 0
	}
	h.CurrentHP = min(scaled, newMax)
}

func (h *Hero) EquipItem(i *Item) *Item {
	if i == nil {
		return nil
	}
	oldItem := h.UnequipItem(i.Slot)

	// Máximo antes do novo item entrar no slot.
	oldMax := h.MaxHP()

	switch i.Slot {
	case weapon:
		h.Weapon = i
	case helmet:
		h.Head = i
	case body:
		h.Body = i
	case boots:
		h.Feet = i
	case amulet:
		h.Amulet = i
	case ring:
		h.Ring = i
	}

	// Preserva o % de vida quando o novo item muda o HP máximo.
	if i.BonusHP != 0 {
		h.rescaleHP(oldMax, h.MaxHP())
	}
	return oldItem
}

func (h *Hero) UnequipItem(slot ItemSlot) *Item {
	// Máximo antes do item sair do slot.
	oldMax := h.MaxHP()

	var item *Item
	switch slot {
	case weapon:
		item = h.Weapon
		h.Weapon = nil
	case helmet:
		item = h.Head
		h.Head = nil
	case body:
		item = h.Body
		h.Body = nil
	case boots:
		item = h.Feet
		h.Feet = nil
	case amulet:
		item = h.Amulet
		h.Amulet = nil
	case ring:
		item = h.Ring
		h.Ring = nil
	}

	// Preserva o % de vida quando o item que saiu muda o HP máximo.
	if item != nil && item.BonusHP != 0 {
		h.rescaleHP(oldMax, h.MaxHP())
	}
	return item
}

// Bag returns the stashed items in insertion order. The returned slice is a
// copy: mutate the bag only through StashItem/EquipFromBag/DropFromBag.
// (The items themselves are shared pointers.)
func (h *Hero) Bag() []*Item {
	bag := make([]*Item, len(h.bag))
	copy(bag, h.bag)
	return bag
}

// BagRoom reports how many more items the bag can take.
func (h *Hero) BagRoom() int {
	return InventoryCapacity - len(h.bag)
}

// HasBagRoom reports whether the bag can stash another item.
func (h *Hero) HasBagRoom() bool {
	return len(h.bag) < InventoryCapacity
}

// StashItem stores an item at the end of the bag, in insertion order.
// Returns false (storing nothing) when the bag is full or the item is nil.
func (h *Hero) StashItem(i *Item) bool {
	if i == nil || !h.HasBagRoom() {
		return false
	}
	h.bag = append(h.bag, i)
	return true
}

// EquipFromBag equips the bag item at index. When the target slot was
// occupied, the displaced item takes the bag item's old list position, so
// the bag length is unchanged and always still fits; when the slot was
// empty the bag shrinks by one. Returns the displaced item (nil when the
// slot was empty). An out-of-range index is a no-op returning nil — this is
// player-reachable, so it must never panic.
func (h *Hero) EquipFromBag(index int) *Item {
	if index < 0 || index >= len(h.bag) {
		return nil
	}
	item := h.bag[index]
	displaced := h.EquipItem(item)
	if displaced != nil {
		h.bag[index] = displaced
	} else {
		h.bag = slices.Delete(h.bag, index, index+1)
	}
	return displaced
}

// DropFromBag removes and returns the bag item at index; the caller decides
// its fate (the engine destroys it). An out-of-range index is a no-op
// returning nil — player-reachable, must never panic.
func (h *Hero) DropFromBag(index int) *Item {
	if index < 0 || index >= len(h.bag) {
		return nil
	}
	item := h.bag[index]
	h.bag = slices.Delete(h.bag, index, index+1)
	return item
}

// LearnedMoves returns the hero's learned moves in learning order. The
// returned slice is a copy: mutate the set only through
// LearnMove/ForgetMove.
func (h *Hero) LearnedMoves() []Move {
	moves := make([]Move, len(h.learnedMoves))
	copy(moves, h.learnedMoves)
	return moves
}

// LearnMove appends a move to the hero's learned set. Under the cap
// (MaxLearnedMoves) and for a new id it learns and returns (nil, true).
//
// At the cap — or when the move's id is already learned — nothing changes
// and it returns (nil, false): forgetting is the CALLER's decision (query
// LearnedMoves, have the player pick, call ForgetMove, then retry).
// `forgotten` is therefore always nil — LearnMove never displaces a move
// on its own; the two-value shape keeps call sites uniform and leaves
// room for a replace-style variant.
func (h *Hero) LearnMove(m Move) (forgotten *Move, learned bool) {
	if len(h.learnedMoves) >= MaxLearnedMoves || h.knowsMove(m.ID) {
		return nil, false
	}
	h.learnedMoves = append(h.learnedMoves, m)
	return nil, true
}

// ForgetMove removes the learned move with the given id. Returns true
// when a move was removed; an unknown id (or an empty set) is a no-op —
// this is player-reachable, so it must never panic.
func (h *Hero) ForgetMove(moveID string) bool {
	for i, m := range h.learnedMoves {
		if m.ID == moveID {
			h.learnedMoves = slices.Delete(h.learnedMoves, i, i+1)
			return true
		}
	}
	return false
}

func (h *Hero) knowsMove(moveID string) bool {
	for _, m := range h.learnedMoves {
		if m.ID == moveID {
			return true
		}
	}
	return false
}

// EffectiveMoves returns the hero's combat move set: the LEARNED moves
// first (learning order), then the equipped weapon's granted moves —
// deduplicated by id (the learned copy wins) and capped at MaxMoves,
// dropping weapon moves beyond the cap. It can therefore never exceed
// MaxMoves entries.
//
// The weapon is passed in by the CALLER so entities stay engine-agnostic
// (no registry lookups here); nil means no weapon. The canonical
// basic_attack is NOT implicitly added: the set can be empty, and what an
// empty set means in combat is the combat integration's decision. The
// returned slice is a copy.
func (h *Hero) EffectiveMoves(weapon *Item) []Move {
	sourced := h.EffectiveMovesWithSource(weapon)
	moves := make([]Move, len(sourced))
	for i, s := range sourced {
		moves[i] = s.Move
	}
	return moves
}

// EffectiveMovesWithSource is EffectiveMoves with an origin tag per
// move — same order (learned first, weapon after), same dedup (the
// learned copy wins, reported as SourceLearned) and same MaxMoves cap
// (weapon moves beyond it drop). SourceWeapon entries leave the set
// when the weapon is unworn; SourceLearned ones persist until
// forgotten. The returned slice is a copy.
func (h *Hero) EffectiveMovesWithSource(weapon *Item) []SourcedMove {
	sourced := make([]SourcedMove, 0, MaxMoves)
	seen := make(map[string]bool, MaxMoves)
	for _, m := range h.learnedMoves {
		if len(sourced) >= MaxMoves {
			break
		}
		if !seen[m.ID] {
			seen[m.ID] = true
			sourced = append(sourced, SourcedMove{Move: m, Source: SourceLearned})
		}
	}
	if weapon != nil {
		for _, m := range weapon.GrantedMoves() {
			if len(sourced) >= MaxMoves {
				break
			}
			if !seen[m.ID] {
				seen[m.ID] = true
				sourced = append(sourced, SourcedMove{Move: m, Source: SourceWeapon})
			}
		}
	}
	return sourced
}
