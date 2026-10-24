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
	default:
		return "Unknown"
	}
}

// Returns which `CommandType`s are allowed for a given `GameState`.
func AllowedCommands(s GameState) []CommandType {
	switch s {
	case StateExploring:
		return []CommandType{CmdExplore, CmdAscend}
	case StateEncounter:
		return []CommandType{CmdFight, CmdSneak}
	case StateCombat:
		return []CommandType{CmdAttack, CmdFlee}
	default:
		return []CommandType{}
	}
}
