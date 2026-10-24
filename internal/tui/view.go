package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/lipgloss"

	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

const barWidth = 10

// The canvas the UI occupies in every state. Content is fitted to it, so
// the window never changes size as the game progresses.
const contentWidth = 72

// bodySlotHeight reserves room for the worst-case body: the ENEMIES
// title, a full queue, a blank separator and the 5-line hero panel.
// Early floors (resp. most content) are far below it; that slack is
// blank space below the panels, by design.
var bodySlotHeight = engine.MAX_ENCOUNTER_SIZE + 7

var (
	// HP fills the gradient from red (low) to green (high). The component's
	// built-in percentage is off: bars keep their full width and the text
	// around each bar formats its own numbers.
	hpBarModel = progress.New(
		progress.WithWidth(barWidth),
		progress.WithScaledGradient("#EF4444", "#22C55E"),
		progress.WithoutPercentage(),
	)
	// XP is a plain gold bar.
	xpBarModel = progress.New(
		progress.WithWidth(barWidth),
		progress.WithSolidFill("214"),
		progress.WithoutPercentage(),
	)
)

// View assembles the full screen from rigid vertical slots — header,
// body, event log and action bar — each padded to its exact reserved
// height. The frame geometry never reflows, and the action bar stays on
// the bottom row no matter how busy the log or the enemy queue get.
func (m Model) View() string {
	content := lipgloss.JoinVertical(lipgloss.Left,
		slot(m.header(), 1),
		slot(m.body(), bodySlotHeight),
		slot(m.logPanel(), maxLogLines),
		slot(m.actionBar(), 1),
	)
	return styleWindow.Render(content)
}

// slot clamps a section's width and pads it to its exact height. Clamping
// width per section (ANSI-aware) keeps a wordy line in one slot from
// breaking the shared frame; the height padding keeps the section in its
// own reserved rows.
func slot(content string, height int) string {
	fitted := lipgloss.NewStyle().
		MaxWidth(contentWidth).
		MaxHeight(height).
		Render(content)
	return lipgloss.Place(contentWidth, height, lipgloss.Top, lipgloss.Left, fitted)
}

func (m Model) header() string {
	return lipgloss.JoinHorizontal(lipgloss.Center,
		styleTitle.Render("Tower of Doom"),
		styleLocation.Render("   "+m.locationLabel()),
		m.badge(),
	)
}

// locationLabel is "Floor N · Section"; a section covers every floor while
// content is validated, but the label degrades gracefully anyway.
func (m Model) locationLabel() string {
	if name := m.engine.SectionName(m.engine.CurrentFloor); name != "" {
		return fmt.Sprintf("Floor %d · %s", m.engine.CurrentFloor, name)
	}
	return fmt.Sprintf("Floor %d", m.engine.CurrentFloor)
}

// badge marks the current state for a quick state read.
func (m Model) badge() string {
	var style lipgloss.Style
	switch m.engine.State {
	case engine.StateExploring:
		style = styleBadgeOk
	case engine.StateEncounter:
		style = styleBadgeWarn
	case engine.StateCombat:
		style = styleBadgeCombat
	case engine.StateGameOver:
		style = styleBadgeOver
	}
	return style.MarginLeft(2).Render(badgeText(m.engine.State))
}

// badgeText names the state for the header badge, spacing multi-word
// states that the FSM strings squash together (e.g. "gameover").
func badgeText(state engine.GameState) string {
	if state == engine.StateGameOver {
		return "GAME OVER"
	}
	return strings.ToUpper(state.String())
}

// body swaps the main area per state.
func (m Model) body() string {
	switch m.engine.State {
	case engine.StateExploring:
		return styleDim.Render("The tower awaits — EXPLORE to investigate.") + "\n\n" + m.heroPanel()
	case engine.StateEncounter, engine.StateCombat: // both share the queue layout
		return m.enemyPanel() + "\n" + m.heroPanel()
	case engine.StateGameOver:
		return styleDeath.Render("☠ YOU DIED — the tower claims another soul.") + "\n\n" + m.heroPanel()
	default:
		return ""
	}
}

// enemyPanel shows the encounter queue; the head of the queue (marked ▶)
// is the enemy currently engaged.
func (m Model) enemyPanel() string {
	var b strings.Builder
	b.WriteString(stylePanelTitle.Render("⚔ ENEMIES") + "\n")
	monsters := m.engine.CurrentEncounter.Monsters
	if len(monsters) == 0 {
		return b.String() + styleDim.Render("   (no enemies in sight)")
	}
	// Pad names so the HP bars line up across the queue.
	nameWidth := 0
	for i := range monsters {
		if n := len([]rune(monsters[i].Name())); n > nameWidth {
			nameWidth = n
		}
	}
	for i := range monsters {
		mon := monsters[i]
		marker, nameStyle := "  ", styleAction
		if i == 0 {
			marker, nameStyle = "▶ ", stylePanelTitle
		}
		name := padRight(mon.Name(), nameWidth)
		b.WriteString(marker + " " + nameStyle.Render(name) + "  " + hpBar(mon.CurrentHP, mon.MaxHP()))
		b.WriteString("\n")
	}
	return b.String()
}

// padRight pads s with spaces on the right up to width runes.
func padRight(s string, width int) string {
	if pad := width - len([]rune(s)); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

// heroPanel is the player's vital stats and currently equipped gear.
func (m Model) heroPanel() string {
	p := m.engine.Player
	var b strings.Builder
	b.WriteString(stylePanelTitle.Render("HERO") + "\n")
	b.WriteString(fmt.Sprintf("  Lv %d", p.Lvl))
	b.WriteString(styleDim.Render(fmt.Sprintf("   XP %s %d/%d to Lv %d", xpBar(p), p.Xp, p.Xp+p.XpToLevel(), p.Lvl+1)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  HP %s %d/%d\n", hpBarPlain(p.CurrentHP, p.MaxHP()), p.CurrentHP, p.MaxHP()))
	b.WriteString(fmt.Sprintf("  DMG %d · DEF %d · SPD %d\n", p.Damage(), p.Defense(), p.Speed()))
	b.WriteString(styleDim.Render("  Gear: " + gearList(p)))
	return b.String()
}

func (m Model) logPanel() string {
	return strings.Join(m.log, "\n")
}

// actionBar renders the current state's options with the cursor
// highlighted; the controls hint closes the line.
func (m Model) actionBar() string {
	actions := actionsFor(m.engine)
	spots := make([]string, 0, len(actions))
	for i, a := range actions {
		label := fmt.Sprintf("[%s] %s", strings.ToUpper(a.key), a.label)
		if !a.enabled {
			spots = append(spots, styleDisabled.Render(label+" ✗"))
			continue
		}
		if i == m.cursor {
			spots = append(spots, styleSelected.Render(" "+label+" "))
			continue
		}
		spots = append(spots, styleAction.Render(label))
	}
	hint := styleDim.Render("   ← → select · ⏎ confirm")
	return lipgloss.JoinHorizontal(lipgloss.Center, strings.Join(spots, "   "), hint)
}

// gearList names everything currently equipped.
func gearList(p *entities.Hero) string {
	slots := []struct {
		name string
		item *entities.Item
	}{
		{"Head", p.Head}, {"Body", p.Body}, {"Feet", p.Feet},
		{"Wpn", p.Weapon}, {"Ring", p.Ring}, {"Amu", p.Amulet},
	}
	var worn []string
	for _, s := range slots {
		if s.item != nil {
			worn = append(worn, fmt.Sprintf("%s %s%s", s.name, s.item.Name, itemBonusList(s.item)))
		}
	}
	if len(worn) == 0 {
		return "bare hands"
	}
	return strings.Join(worn, " · ")
}

// xpBar renders the progress toward the next level in gold tones.
func xpBar(p *entities.Hero) string {
	total := p.Xp + p.XpToLevel()
	return xpBarModel.ViewAs(float64(percent(p.Xp, total)) / 100)
}

// hpBar renders the gradient HP bar plus its percentage, e.g. "███ 55%".
func hpBar(cur, max int) string {
	return barFill(cur, max) + fmt.Sprintf(" %3d%%", percent(cur, max))
}

// hpBarPlain is the bar without the percentage suffix, for rows that show
// the numbers as cur/max instead (the hero panel).
func hpBarPlain(cur, max int) string {
	return barFill(cur, max)
}

// barFill renders the actual bar via the bubbles progress component.
func barFill(cur, max int) string {
	return hpBarModel.ViewAs(float64(percent(cur, max)) / 100)
}

// percent clamps a cur/max pair into [0, 100].
func percent(cur, max int) int {
	switch {
	case max <= 0 || cur <= 0:
		return 0
	case cur >= max:
		return 100
	default:
		return cur * 100 / max
	}
}
