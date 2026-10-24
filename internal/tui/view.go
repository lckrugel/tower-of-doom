package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

const barWidth = 10

// View assembles the full screen: header, state-dependent body, event log
// and action bar, all inside one rounded window.
func (m Model) View() string {
	parts := []string{
		m.header(),
		m.body(),
		m.logPanel(),
		m.actionBar(),
	}
	return styleWindow.Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
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
	b.WriteString(fmt.Sprintf("  HP %s %d/%d\n", coloredBar(p.CurrentHP, p.MaxHP()), p.CurrentHP, p.MaxHP()))
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
	pct := percent(p.Xp, total)
	fill := pct * barWidth / 100
	bar := strings.Repeat("█", fill) + strings.Repeat("░", barWidth-fill)
	return styleXP.Render(bar)
}

// hpBar renders "███░░░░ 55%" within its fixed width, colored by the
// remaining percentage (green above 50, amber above 20, red below).
func hpBar(cur, max int) string {
	return coloredBar(cur, max) + fmt.Sprintf(" %3d%%", percent(cur, max))
}

// coloredBar is hpBar's fill without the percentage suffix, for rows that
// show the numbers as cur/max instead (the hero panel).
func coloredBar(cur, max int) string {
	pct := percent(cur, max)
	filled := pct * barWidth / 100
	fill := strings.Repeat("█", filled)
	empty := strings.Repeat("░", barWidth-filled)
	switch {
	case pct >= 50:
		return styleBarGood.Render(fill) + styleBarEmpty.Render(empty)
	case pct >= 20:
		return styleBarWarn.Render(fill) + styleBarEmpty.Render(empty)
	default:
		return styleBarBad.Render(fill) + styleBarEmpty.Render(empty)
	}
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
