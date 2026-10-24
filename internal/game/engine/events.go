package engine

import "github.com/lckrugel/tower-of-doom/internal/game/engine/entities"

// `EventType` é tudo aquilo que o jogo pode reportar que aconteceu.
type EventType int

const (
	// Hero enters a new floor.
	EventFloorEntered EventType = iota
	// A new encounter was found.
	EventEncounterFound
	// Hero past the encounter successfully.
	EventSneakSuccess
	// Hero failed to sneak past the encounter.
	EventSneakFailed
	// Hero's damage dealt.
	EventCombatDamageDealt
	// Hero's damage taken.
	EventCombatDamageTaken
	EventEnemyHealed
	EventHeroHealed
	EventEnemyDefeated
	// Hero died.
	EventHeroDefeated
	// Hero successfully fled the combat.
	EventFled
	// Defeated all enemies in an encounter.
	EventVictory
	// Hero gained a level (mid-combat level ups included).
	EventLevelUp
	// Quit requested the game.
	EventQuit
	// Generic game message.
	EventMessage
)

// `Event` é o que o jogo envia após processar um `Command`.
type Event struct {
	Type EventType
	Data map[string]any
}

func NewMessage(msg string) Event {
	return Event{
		Type: EventMessage,
		Data: map[string]any{
			"message": msg,
		}}
}

func NewFloorEntered(floor int, sectionName string) Event {
	return Event{
		Type: EventFloorEntered,
		Data: map[string]any{
			"floor":   floor,
			"section": sectionName,
		},
	}
}

func NewEncounterFound(monsters []entities.Monster) Event {
	return Event{
		Type: EventEncounterFound,
		Data: map[string]any{
			"monsters": monsters,
		},
	}
}

func NewDamageDealt(enemy Combatant, dmgDealt int, crit bool) Event {
	return Event{
		Type: EventCombatDamageDealt,
		Data: map[string]any{
			"enemy":        enemy.Name(),
			"enemy_hp":     enemy.CurrentHPPercent(),
			"enemy_alive":  enemy.IsAlive(),
			"damage_dealt": dmgDealt,
			"crit":         crit,
		},
	}
}

func NewDamageTaken(hero Combatant, dmgTaken int, crit bool) Event {
	return Event{
		Type: EventCombatDamageTaken,
		Data: map[string]any{
			"hero":         hero.Name(),
			"hero_hp":      hero.CurrentHPPercent(),
			"hero_alive":   hero.IsAlive(),
			"damage_taken": dmgTaken,
			"crit":         crit,
		},
	}
}

func NewSneakSuccess() Event {
	return Event{Type: EventSneakSuccess}
}

func NewSneakFailed() Event {
	return Event{Type: EventSneakFailed}
}

func NewFled() Event {
	return Event{Type: EventFled}
}

func NewVictory() Event {
	return Event{Type: EventVictory}
}

// NewEnemyDefeated reports a defeated enemy and the instant XP reward
// (mid-combat kills included).
func NewEnemyDefeated(monsterName string, xp int) Event {
	return Event{
		Type: EventEnemyDefeated,
		Data: map[string]any{
			"enemy":     monsterName,
			"xp_gained": xp,
		},
	}
}

func NewHeroDefeated() Event {
	return Event{Type: EventHeroDefeated}
}

// NewLevelUp reports that the Hero gained at least one level.
// `lvl` is the level reached after all pending level ups.
func NewLevelUp(lvl int) Event {
	return Event{
		Type: EventLevelUp,
		Data: map[string]any{
			"level": lvl,
		},
	}
}

// NewQuit tells the caller to stop the game loop and exit.
func NewQuit() Event {
	return Event{Type: EventQuit}
}
