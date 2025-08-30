package game

import (
	"crypto/rand"
	"math/big"
)

func rollDice(n int) int {
	if n <= 0 {
		return 1
	}
	max := big.NewInt(int64(n))
	reader := rand.Reader
	r, _ := rand.Int(reader, max)
	return int(r.Int64())
}

func RollD10() int {
	return rollDice(10)
}

func RollD20() int {
	return rollDice(20)
}

func RollD8() int {
	return rollDice(8)
}

func RollD4() int {
	return rollDice(4)
}

func RollD6() int {
	return rollDice(6)
}

func RollD100() int {
	return rollDice(100)
}
