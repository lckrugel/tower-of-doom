package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/lckrugel/tower-of-doom/internal/game/engine"
)

// Dark-dungeon palette on 256-color ANSI numbers: golds for titles and
// victories, soft-reds for damage, greens for healing, cyan for loot.

var (
	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("228"))

	styleLocation = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	stylePanelTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("228"))

	styleDim = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	// Log tones, resolved per event by toneOf.
	styleDamage  = lipgloss.NewStyle().Foreground(lipgloss.Color("215")) // dealt hits: warm amber
	styleHurt    = lipgloss.NewStyle().Foreground(lipgloss.Color("174")) // damage taken: soft red
	styleKill    = lipgloss.NewStyle().Foreground(lipgloss.Color("213")) // kills: bright violet
	styleHeal    = lipgloss.NewStyle().Foreground(lipgloss.Color("114")) // heals: green
	styleLoot    = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))  // items: cyan
	styleXP      = lipgloss.NewStyle().Foreground(lipgloss.Color("179")) // XP & levels: gold
	styleVictory = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("228"))

	// State badges.
	styleBadgeOk = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("114")) // green: calm exploration
	styleBadgeWarn = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("215")) // amber: something found
	styleBadgeCombat = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("196")) // red: in combat
	styleBadgeOver = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("245")) // gray: it's over
	// Unknown state: loud magenta instead of a blank chip — if the FSM
	// ever reports something unexpected, the header should say so.
	styleBadgeUnknown = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("213"))
	styleDeath = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("196"))

	// Action bar styles.
	styleAction = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
	styleSelected = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("61"))
	styleDisabled = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	// The un-filled part of the HP bar track lives inside the progress
	// component itself; no extra styles needed here.

	styleWindow = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1)

	// styleLogColumn draws the vertical divider between the game area and
	// the log: only the left border of the log column is rendered, using
	// the same rounded piece and dim tone as the window.
	styleLogColumn = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			BorderTop(false).
			BorderRight(false).
			BorderBottom(false).
			BorderLeft(true)
)

// toneOf picks the log color for an event's flavor.
func toneOf(ev engine.Event) lipgloss.Style {
	switch ev.Type {
	case engine.EventCombatDamageDealt:
		return styleDamage
	case engine.EventCombatDamageTaken:
		return styleHurt
	case engine.EventHeroHealed:
		return styleHeal
	case engine.EventEnemyHealed:
		return styleHeal
	case engine.EventEnemyDefeated:
		return styleKill
	case engine.EventLevelUp:
		return styleXP
	case engine.EventItemFound:
		return styleLoot
	case engine.EventHeroDefeated, engine.EventFled:
		return styleDeath
	case engine.EventVictory:
		return styleVictory
	case engine.EventFloorEntered:
		return styleLocation
	case engine.EventQuit:
		return styleDim
	default:
		return styleDim
	}
}
