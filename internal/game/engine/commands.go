package engine

// `CommandType` are all the possible types of actions that can be taken in the game.
type CommandType int

const (
	// Explores the current floor.
	CmdExplore = iota
	// Ascend to the next floor (if possible).
	CmdAscend
	// Engages in combat in the current encounter.
	//
	// Leads to a standard initiative.
	CmdFight
	// Attempts to sneak past the current encounter.
	//
	// Failling leads to a lost turn in combat to the enemies.
	CmdSneak
	// Attacks the current enemy during combat.
	CmdAttack
	// Attempts to flee during combat.
	//
	// Success exits combat.
	//
	// Failure gives the enemy a free turn.
	CmdFlee
	// Quits the game.
	CmdQuit
	// Wears an item into its equipment slot, in either state that hosts
	// the decision:
	//   - StateLoot: wears the pending loot (ItemIndex ignored); the
	//     displaced item goes to the bag when there is room, else it is
	//     destroyed (EventItemLost). Either way back to StateExploring
	//     with nothing pending.
	//   - StateInventory: wears the bag item at `ItemIndex`; the
	//     displaced item takes the worn item's old bag position (the bag
	//     length is unchanged). The bag stays open.
	CmdWear
	// Stashes the pending chest loot into the hero bag (StateLoot).
	// Rejected with no state change while the bag is full.
	CmdStashLoot
	// Destroys the pending chest loot (StateLoot).
	CmdDiscardLoot
	// Opens the hero bag while exploring (StateInventory).
	CmdInventory
	// Destroys the bag item at `ItemIndex` (StateInventory).
	CmdDrop
	// Closes the bag, returning to exploring (StateInventory).
	CmdClose
	// Buys the merchant offer at `ItemIndex` into the hero bag
	// (StateMerchant). Rejected with no state change when the hero
	// cannot afford it or the bag is full.
	CmdBuy
	// Sells the bag item at `ItemIndex` to the merchant (StateMerchant):
	// the item is destroyed and its SellValue added to the purse.
	CmdSell
	// Leaves the merchant, discarding the remaining stock
	// (StateMerchant → StateExploring).
	CmdLeave
)

// `Command` is an action taken.
type Command struct {
	Type CommandType
	// ItemIndex points into the hero bag for the bag commands (CmdWear
	// and CmdDrop in StateInventory) and for selling (CmdSell in
	// StateMerchant); it points into Engine.MerchantStock for CmdBuy.
	// Every other command ignores it. An out-of-range/negative index is
	// rejected as a generic invalid command (EventMessage) with no state
	// change — never a panic.
	ItemIndex int
}
