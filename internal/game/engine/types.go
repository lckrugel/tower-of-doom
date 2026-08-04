package engine

// `GameState define o estado da máquina de estados que decide quais ações são válidas.
type GameState int

const (
	StateMenu GameState = iota
	StateExploring
	StateCombat
	StateGameOver
)

func (s GameState) String() string {
	switch s {
	case StateMenu:
		return "Menu"
	case StateExploring:
		return "Exploring"
	case StateCombat:
		return "Combat"
	case StateGameOver:
		return "GameOver"
	default:
		return "Unknown"
	}
}

// `CommandType` são todas as ações que um jogador pode enviar ao jogo.
type CommandType int

const (
	CmdNewGame CommandType = iota // inicia um novo jogo
	CmdExplore                    // explora o andar atual
	CmdAscend                     // sobe para o próximo andar
	CmdFight                      // inicia combate com os inimigos
	CmdSneak                      // tenta pular o combate
	CmdAttack                     // ataca durante o combate
	CmdFlee                       // tenta fugir durante o combate
	CmdQuit                       // fecha o jogo
)

// `Command` é o que é enviado para jogo como um ação.
type Command struct {
	Type CommandType
}
