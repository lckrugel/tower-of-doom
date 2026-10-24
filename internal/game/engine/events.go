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

func NewDamageDealt(enemy Combatant, dmgDealt int) Event {
	return Event{
		Type: EventCombatDamageDealt,
		Data: map[string]any{
			"enemy":        enemy.Name(),
			"enemy_hp":     enemy.CurrentHPPercent(),
			"enemy_alive":  enemy.IsAlive(),
			"damage_dealt": dmgDealt,
		},
	}
}
