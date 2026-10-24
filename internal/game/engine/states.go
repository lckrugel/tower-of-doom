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
	default:
		return "Unknown"
	}
}

// Returns which `CommandType`s are allowed for a given `GameState`.
func AllowedCommands(s GameState) []CommandType {
	switch s {
	case StateExploring:
		return []CommandType{CmdExplore, CmdAscend, CmdInventory, CmdQuit}
	case StateEncounter:
		return []CommandType{CmdFight, CmdSneak, CmdQuit}
	case StateCombat:
		return []CommandType{CmdAttack, CmdFlee, CmdQuit}
	case StateLoot:
		return []CommandType{CmdEquipLoot, CmdStashLoot, CmdDiscardLoot, CmdQuit}
	case StateInventory:
		return []CommandType{CmdEquip, CmdDrop, CmdClose, CmdQuit}
	case StateGameOver:
		return []CommandType{CmdQuit}
	default:
		return []CommandType{}
	}
}
