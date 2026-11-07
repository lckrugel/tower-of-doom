package engine

// `GameState defines the possible states for the FSM that manages the gameloop.
type GameState int

const (
	// Exploring the current floor. Default state for a new game.
	StateExploring = iota
	// A new encounter was generated, player has to choose how to approach it.
	StateEncounter
	// Mid combat state.
	StateCombat
	// Hero died.
	StateGameOver
	// A chest was opened and the loot is held in PendingLoot: the player
	// must decide equip it, stash it or discard it.
	StateLoot
	// The hero bag is open (only reachable from StateExploring).
	StateInventory
	// A merchant is shopped at: Engine.MerchantStock holds the offers;
	// the player buys, sells or leaves.
	StateMerchant
	// A shrine presented its offers (ONE or TWO moves, held in
	// Engine.PendingMoves): the player first CHOOSES one (the choosing
	// step, CmdChooseOffer); at the 2-learned cap the flow then advances
	// to the forgetting step (CmdForget) — Engine.ChosenMove marks which
	// step is live. DECLINE (in either step) keeps everything as it was.
	StateLearn
)

func (s GameState) String() string {
	switch s {
	case StateExploring:
		return "Exploring"
	case StateEncounter:
		return "Encounter"
	case StateCombat:
		return "Combat"
	case StateGameOver:
		return "GameOver"
	case StateLoot:
		return "Loot"
	case StateInventory:
		return "Inventory"
	case StateMerchant:
		return "Merchant"
	case StateLearn:
		return "Learn"
	default:
		return "Unknown"
	}
}

// Returns which `CommandType`s are allowed for a given `GameState`.
//
// StateLearn is STEP-dependent — this state-only query returns the UNION
// of both steps. Use Engine.AvailableCommands for the active instance's
// precise command set.
func AllowedCommands(s GameState) []CommandType {
	switch s {
	case StateExploring:
		return []CommandType{CmdExplore, CmdAscend, CmdChallengeBoss, CmdInventory, CmdQuit}
	case StateEncounter:
		return []CommandType{CmdFight, CmdSneak, CmdQuit}
	case StateCombat:
		return []CommandType{CmdMove, CmdFlee, CmdQuit}
	case StateLoot:
		return []CommandType{CmdWear, CmdStashLoot, CmdDiscardLoot, CmdQuit}
	case StateInventory:
		return []CommandType{CmdWear, CmdDrop, CmdClose, CmdQuit}
	case StateMerchant:
		return []CommandType{CmdBuy, CmdSell, CmdLeave, CmdQuit}
	case StateLearn:
		return []CommandType{CmdChooseOffer, CmdForget, CmdDecline, CmdQuit}
	case StateGameOver:
		return []CommandType{CmdQuit}
	default:
		return []CommandType{}
	}
}

// AvailableCommands reports the commands accepted in the engine's current
// state, including instance-specific workflow steps such as shrine choosing
// versus forgetting. The returned slice is independent of engine state.
func (e *Engine) AvailableCommands() []CommandType {
	return e.allowedCommands(e.State)
}

// allowedCommands is the ENGINE's view of AllowedCommands, resolving the
// StateLearn step matrix (see AllowedCommands — the package-level
// function documents the split):
//   - the CHOOSING step (no choice recorded yet): pick one of
//     Engine.PendingMoves or decline;
//   - the FORGETTING step (Engine.ChosenMove set): forget a learned move
//     or decline.
//
// Every state keeps CmdQuit.
func (e *Engine) allowedCommands(s GameState) []CommandType {
	if s != StateLearn {
		return AllowedCommands(s)
	}
	if e.inForgetStep() {
		return []CommandType{CmdForget, CmdDecline, CmdQuit}
	}
	return []CommandType{CmdChooseOffer, CmdDecline, CmdQuit}
}
