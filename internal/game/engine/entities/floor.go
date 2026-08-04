package entities

type Floor struct {
	Number          int             `json:"floorNumber"`
	XpReward        int             `json:"xpReward"`
	GoldReward      float32         `json:"goldReward"`
	Difficulty      int             `json:"difficulty"`
	MonstersAllowed []*FloorMonster `json:"monsters"`
}

type FloorMonster struct {
	MonsterTag  string  `json:"monsterTag"`
	SpawnChance float32 `json:"spawnChance"`
}
