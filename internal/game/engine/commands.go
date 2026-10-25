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
	// Uses the hero move at `Index` during combat (StateCombat): the
	// index points into the hero's combat move set — the same set the UI
	// offers as the combat options. An out-of-range index is rejected as
	// a generic invalid command (EventMessage) with no state change; a
	// move on cooldown is answered with a blocking message (and, when
	// every move is locked, the turn resolves as a RECOVER round).
	CmdMove
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
	//   - StateLoot: wears the pending loot (Index ignored); the
	//     displaced item goes to the bag when there is room, else it is
	//     destroyed (EventItemLost). Either way back to StateExploring
	//     with nothing pending.
	//   - StateInventory: wears the bag item at `Index`; the
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
	// Destroys the bag item at `Index` (StateInventory).
	CmdDrop
	// Closes the bag, returning to exploring (StateInventory).
	CmdClose
	// Buys the merchant offer at `Index` into the hero bag
	// (StateMerchant). Rejected with no state change when the hero
	// cannot afford it or the bag is full.
	CmdBuy
	// Sells the bag item at `Index` to the merchant (StateMerchant):
	// the item is destroyed and its SellValue added to the purse.
	CmdSell
	// Leaves the merchant, discarding the remaining stock
	// (StateMerchant → StateExploring).
	CmdLeave
	// Challenges the section's boss (StateExploring, only while the
	// challenge is unlocked — Engine.BossUnlocked): the PREDEFINED boss
	// group is engaged straight into StateCombat — no StateEncounter, no
	// sneak step, no encounter-kind roll. While locked it is a blocked
	// action: a message, state intact (like a locked ASCEND).
	CmdChallengeBoss
	// Chooses one of the shrine's pending offers (StateLearn's choosing
	// step): `Index` points into Engine.PendingMoves (1–2 offers). Under
	// the 2-learned cap the chosen move is learned on the spot
	// (EventMoveLearned) and exploring continues; at the cap the choice
	// is recorded (Engine.ChosenMove) and StateLearn advances to the
	// forgetting step. An out-of-range index is a generic invalid
	// command with no state change — never a panic.
	CmdChooseOffer
	// Forgets the learned move at `Index` to make room for the CHOSEN
	// shrine offer (StateLearn's forgetting step — the choice was
	// recorded in Engine.ChosenMove by CmdChooseOffer at the learned
	// cap): the chosen offer is learned in the forgotten move's place
	// (EventMoveLearned with the additive `forgotten` key) and exploring
	// continues. The index points into `LearnedMoves()`; an
	// out-of-range index is a generic invalid command with no state
	// change — never a panic.
	CmdForget
	// Declines the shrine's teaching (StateLearn, EITHER step): keep
	// everything as it was — both learned moves and no choice — and
	// exploring continues with nothing pending.
	CmdDecline
)

// `Command` is an action taken.
type Command struct {
	Type CommandType
	// Index is the subject of the index-carrying commands: the move
	// index into the hero's combat move set for CmdMove (StateCombat);
	// the bag index for the bag commands (CmdWear and CmdDrop in
	// StateInventory) and for selling (CmdSell in StateMerchant takes a
	// bag index); the stock index into Engine.MerchantStock for CmdBuy;
	// the pending-offer index into Engine.PendingMoves for the shrine's
	// CmdChooseOffer (StateLearn's choosing step); the learned-move
	// index for the shrine's CmdForget (StateLearn's forgetting step).
	// Every other command ignores it. An out-of-range/negative index is
	// rejected as a generic invalid command (EventMessage) with no state
	// change — never a panic.
	Index int
}
