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
)

// `Command` is an action taken.
type Command struct {
	Type CommandType
}
