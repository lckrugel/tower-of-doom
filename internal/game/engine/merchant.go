package engine

import (
	"slices"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// merchantStockSize is how many offers a merchant rolls.
const merchantStockSize = 4

// merchantUpgradeChance is the chance (percentage) that one offer is
// replaced by a tier+1 item.
const merchantUpgradeChance = 35

// rollMerchantStock rolls the merchant's offers from the section's
// allowed item tiers (empty = unrestricted). The rng sequence is a
// determinism contract, in this exact order:
//
//  1. Offer rolls: one Intn per offer over the (sorted) allowed-tier
//     pool, REMOVING the picked id before the next roll — the pool
//     shrinks, so the stock never duplicates. Fewer offers only when the
//     pool runs dry (a dry pool consumes no rng for the missing offers).
//  2. One Intn(100): an upgrade fires when the roll is < 35.
//  3. On an upgrade, one Intn over the stock length picks which offer is
//     replaced.
//  4. On an upgrade, one Intn over the replacement pool: every registry
//     item one tier above the replaced offer, capped at the registry's
//     max tier, EXCLUDING the ids already in stock. An empty filtered
//     pool keeps the original offer and consumes no rng for it.
func (e *Engine) rollMerchantStock(section game.SectionAsset) []*entities.Item {
	pool := e.reg.ItemIDsForTiers(section.ItemTiers)
	if len(pool) == 0 {
		// Validation keeps merchant sections stocked; this guard only
		// exists for registries that skipped validation. Consumes no rng.
		return nil
	}
	available := slices.Clone(pool)
	stock := make([]*entities.Item, 0, merchantStockSize)
	for range merchantStockSize {
		if len(available) == 0 {
			break // the pool ran dry: fewer offers, no extra rng
		}
		pick := e.rng.Intn(len(available))
		stock = append(stock, e.itemFromID(available[pick]))
		available = slices.Delete(available, pick, pick+1)
	}
	if e.rng.Intn(100) < merchantUpgradeChance {
		offer := e.rng.Intn(len(stock))
		if upgraded := e.upgradedItem(stock[offer], stockIDs(stock)); upgraded != nil {
			stock[offer] = upgraded
		}
	}
	return stock
}

// stockIDs collects the ids present in a stock list (a replacement roll
// must never duplicate one of them).
func stockIDs(stock []*entities.Item) map[string]bool {
	ids := make(map[string]bool, len(stock))
	for _, offer := range stock {
		ids[offer.ID] = true
	}
	return ids
}

// itemFromID builds a fresh item from a registry id (no rng).
func (e *Engine) itemFromID(id string) *entities.Item {
	item := entities.NewItemFromAsset(e.reg.Items[id])
	return &item
}

// upgradedItem rolls a registry item one tier above base, capped at the
// registry's max tier, EXCLUDING every id in `inStock` (the replacement
// must not duplicate an offer already on display). Returns nil (keeping
// the caller's offer) when no registry item fills the filtered pool.
func (e *Engine) upgradedItem(base *entities.Item, inStock map[string]bool) *entities.Item {
	target := min(base.Tier+1, e.reg.MaxItemTier())
	pool := e.reg.ItemIDsForTiers([]int{target})
	filtered := make([]string, 0, len(pool))
	for _, id := range pool {
		if !inStock[id] {
			filtered = append(filtered, id)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return e.itemFromID(filtered[e.rng.Intn(len(filtered))])
}

// handleMerchant runs the shopping loop (StateMerchant). Buying moves
// gold to the price and the item to the hero bag, removing the offer
// from the stock; selling pays the SellValue into the purse and destroys
// the bag item; leaving discards the stock and returns to exploring.
// Nothing here unlocks ASCEND — only combat victories do.
func (e *Engine) handleMerchant(cmd Command) []Event {
	switch cmd.Type {
	case CmdBuy:
		item := e.MerchantStock[cmd.ItemIndex]
		price := item.BuyPrice()
		// Gold is reported first, then a full bag: both rejections leave
		// gold, bag and stock untouched (the player can pick another
		// option — the shop stays open).
		if e.Player.Gold < price {
			return []Event{NewMessage("Not enough gold.")}
		}
		if !e.Player.HasBagRoom() {
			return []Event{NewBagFull()}
		}
		e.Player.Gold -= price
		e.Player.StashItem(item)
		e.MerchantStock = slices.Delete(e.MerchantStock, cmd.ItemIndex, cmd.ItemIndex+1)
		return []Event{NewItemBought(item, price)}
	case CmdSell:
		// Destroyed on the spot via the hero's existing drop: the
		// merchant takes the item for its SellValue.
		item := e.Player.DropFromBag(cmd.ItemIndex)
		price := item.SellValue()
		e.Player.Gold += price
		return []Event{NewItemSold(item, price)}
	case CmdLeave:
		e.MerchantStock = nil
		e.State = StateExploring
		return []Event{NewMessage("You leave the merchant to their wares.")}
	default:
		return []Event{NewMessage("Unhandled command")}
	}
}
