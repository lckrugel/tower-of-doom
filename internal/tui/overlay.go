package tui

import (
	"fmt"
	"strings"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// maxOverlayStatuses caps the '?' overlay's status list: shipped content
// defines four; a modded registry with more still renders inside the
// fixed canvas (the documented display cap — the list says what it
// dropped).
const maxOverlayStatuses = 10

// overlayPanel is the '?' lore screen: every registered status with its
// effect and duration (kind-colored), the move-tag legend — every glyph
// the grid and the dossiers use — and the live state's key map. A
// TUI-only pure view: the engine is never asked for commands while it
// is open (any key closes it first). Fits the fixed canvas: 2 + 10
// status rows + 2 legend + 2 keys (combat's grid legend makes 3) + the
// close hint = 18 of the 22 canvas rows, worst case.
func (m Model) overlayPanel() string {
	rows := make([]string, 0, 18)
	rows = append(rows, stylePanelTitle.Render("? TOWER LORE"))

	// The registry's statuses — name, effect, duration — toned by kind
	// like the chips and the log lines.
	rows = append(rows, styleDim.Render("STATUS EFFECTS"))
	lore := m.engine.StatusLore()
	shown := min(len(lore), maxOverlayStatuses)
	if shown == 0 {
		rows = append(rows, styleDim.Render("   (none registered)"))
	}
	for _, a := range lore[:shown] {
		style := styleHurt
		if a.Kind == string(entities.StatusBuff) {
			style = styleHeal
		}
		rows = append(rows, "  "+style.Render(overlayStatusLine(a)))
	}
	if shown < len(lore) {
		rows = append(rows, styleDim.Render(fmt.Sprintf("   … %d more (read the assets)", len(lore)-shown)))
	}

	// The move-tag legend.
	rows = append(rows, styleDim.Render("MOVE TAGS"))
	rows = append(rows, styleDim.Render("  N% damage · +N HP / +N% HP healing · ✦+N crit · ☠<status> N% chance"))
	rows = append(rows, styleDim.Render("  (N turns) move locked · ◆ learned · ⚔ weapon"))

	// The live state's key map — the same chips the action bar shows.
	rows = append(rows, styleDim.Render("KEYS — "+badgeText(m.engine.State)))
	rows = append(rows, "  "+truncateRunes(m.overlayKeys(), m.layout().ContentWidth-4))
	if m.bagVisible() && m.consumablesTab {
		rows = append(rows, styleDim.Render("  ←/→ select stack · Enter use · C/Esc Back (free)"))
	} else if m.engine.State == engine.StateCombat {
		// The moves live in the grid there: the legend says so.
		rows = append(rows, styleDim.Render("  1-4 or arrows pick a move · ⏎ use it"))
	} else if m.engine.State == engine.StateChest {
		rows = append(rows, styleDim.Render("  ←/→ select · Enter confirm · Esc does not dismiss the chest"))
	}

	rows = append(rows, styleDim.Render("? — any key closes the overlay"))
	return strings.Join(rows, "\n")
}

// overlayKeys is the live bar flattened to one line: "[E] Explore · …".
func (m Model) overlayKeys() string {
	spots := make([]string, 0, 6)
	for _, a := range m.barActions() {
		label := fmt.Sprintf("[%s] %s", strings.ToUpper(a.key), a.label)
		if !a.enabled {
			label += " ✗"
		}
		spots = append(spots, label)
	}
	return strings.Join(spots, " · ")
}

// overlayStatusLine explains authored duration and when its clock starts.
func overlayStatusLine(a game.StatusAsset) string {
	return fmt.Sprintf("%s — %s — %d %s", a.Name, statusEffectText(a), a.Duration, statusDurationText(a))
}

func statusDurationText(a game.StatusAsset) string {
	if a.Effect.Type == "dot" {
		return "ticks (first at application turn end)"
	}
	return "future full turns (active immediately)"
}

// statusEffectText renders a status asset's effect: "3 dmg/turn" for a
// DoT, "+5 DEF" for a stat modifier. An unknown effect type (a modded
// gap) says so rather than printing a bare number.
func statusEffectText(a game.StatusAsset) string {
	switch a.Effect.Type {
	case "dot":
		parts := make([]string, 0, 2)
		if a.Effect.Amount != 0 {
			parts = append(parts, fmt.Sprintf("%d", a.Effect.Amount))
		}
		if a.Effect.Percent != 0 {
			p := fmt.Sprintf("%d%% max HP", a.Effect.Percent)
			parts = append(parts, p)
		}
		return strings.Join(parts, " + ") + " dmg/turn"
	case "stat":
		parts := make([]string, 0, 2)
		if a.Effect.Amount != 0 {
			parts = append(parts, fmt.Sprintf("%+d", a.Effect.Amount))
		}
		if a.Effect.Percent != 0 {
			parts = append(parts, fmt.Sprintf("%+d%%", a.Effect.Percent))
		}
		return strings.Join(parts, " and ") + " " + strings.ToUpper(a.Effect.Stat)
	default:
		return "unknown effect"
	}
}

// statusLoreByID resolves a registry status asset by id (move payloads
// carry ids).
func (m Model) statusLoreByID(id string) (game.StatusAsset, bool) {
	for _, a := range m.engine.StatusLore() {
		if a.ID == id {
			return a, true
		}
	}
	return game.StatusAsset{}, false
}

// statusExplanation joins statuses into ONE dim tooltip line: "Poison —
// 3 dmg/turn · 2 ticks left; Weaken — -3 DMG · 3 turns left" — the live
// turns from the engine combat view, the effect from the registry's lore. A status
// the registry doesn't know degrades to its live count. An empty list
// explains nothing.
func (m Model) statusExplanation(statuses []combatStatus) string {
	if len(statuses) == 0 {
		return ""
	}
	parts := make([]string, 0, len(statuses))
	for _, s := range statuses {
		if lore, ok := m.statusLoreByID(s.id); ok {
			parts = append(parts, fmt.Sprintf("%s — %s · %d %s left", s.name, statusEffectText(lore), s.turns, s.lifetimeUnit()))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s · %d %s left", s.name, s.turns, s.lifetimeUnit()))
	}
	return strings.Join(parts, "; ")
}

// moveStatusExplanation is a dossier's tooltip for a status effect.
func (m Model) moveStatusExplanation(s entities.MoveStatus, target entities.MoveTarget) string {
	if s.ID == "" {
		return ""
	}
	tag := fmt.Sprintf("☠%s %d%%", s.ID, s.Chance)
	lore, ok := m.statusLoreByID(s.ID)
	if !ok {
		return tag
	}
	return fmt.Sprintf("%s → %s — %s · %d %s", tag, target, statusEffectText(lore), lore.Duration, statusDurationText(lore))
}

func (s combatStatus) lifetimeUnit() string {
	if s.unit == "ticks" {
		return "ticks"
	}
	return "turns"
}

// moveEffectsExplanation summarizes all effects in their authored order.
func (m Model) moveEffectsExplanation(move entities.Move) string {
	parts := make([]string, 0, len(move.Effects))
	for _, effect := range move.Effects {
		target := string(effect.Target)
		switch effect.Type {
		case "damage":
			parts = append(parts, fmt.Sprintf("damage %d%% → %s", effect.DmgModifier, target))
		case "heal":
			var amounts []string
			if effect.Flat != 0 {
				amounts = append(amounts, fmt.Sprintf("%d HP", effect.Flat))
			}
			if effect.Percent != 0 {
				amounts = append(amounts, fmt.Sprintf("%d%% HP", effect.Percent))
			}
			parts = append(parts, fmt.Sprintf("heal %s → %s", strings.Join(amounts, " + "), target))
		case "status":
			s := effect.Status
			if s.ID == "" {
				continue
			}
			parts = append(parts, m.moveStatusExplanation(s, effect.Target))
		}
	}
	return strings.Join(parts, "; ")
}
