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
	// Decides what to do with the pending chest loot (StateLoot):
	// equips it, sending whatever it displaced to the bag (or destroying
	// it there is no room).
	CmdEquipLoot
	// Stashes the pending chest loot into the hero bag (StateLoot).
	// Rejected with no state change while the bag is full.
	CmdStashLoot
	// Destroys the pending chest loot (StateLoot).
	CmdDiscardLoot
	// Opens the hero bag while exploring (StateInventory).
	CmdInventory
	// Equips the bag item at `ItemIndex`, sending the displaced item to
	// the bag item's old position (StateInventory).
	CmdEquip
	// Destroys the bag item at `ItemIndex` (StateInventory).
	CmdDrop
	// Closes the bag, returning to exploring (StateInventory).
	CmdClose
)

// `Command` is an action taken.
type Command struct {
	Type CommandType
	// ItemIndex points into the hero bag for the bag commands (CmdEquip
	// and CmdDrop in StateInventory). Every other command ignores it. An
	// out-of-range/negative index is rejected as a generic invalid
	// command (EventMessage) with no state change — never a panic.
	ItemIndex int
}
