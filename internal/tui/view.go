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

// Two-column canvas: game area on the left, event log on the right.
// Content is fitted to the slots, so the window never changes size as
// the game progresses. Everything on the bottom row must be anchored —
// the plan is: hero stats sit directly above the shortcut bar.
const (
	// Total inner canvas: the terminal frame is 2 wider/taller (border
	// and padding).
	contentWidth  = 96
	contentHeight = 22

	// Right column; the divider border it draws adds 1 column.
	logColWidth  = 50
	mainColWidth = contentWidth - logColWidth

	// Equipped panel geometry: the slot-label column and where an
	// occupied slot's second line (the bonus list) starts — gutter +
	// label column + gap.
	equippedLabelWidth = 4 // the longest of Head/Body/Feet/Wpn/Ring/Amu
	equippedIndent     = 2 + equippedLabelWidth + 2

	// Middle bands: header (1) and action bar (1) frame the workspace.
	middleHeight = contentHeight - 2

	// Hero panel: HERO + Lv + HP + stats lines, anchored to the bottom
	// of the left column. Gear is NOT listed here — the equipped panel
	// (right column in the trade states) is the only gear view.
	heroHeight = 4
	bodyHeight = middleHeight - heroHeight
)

// Minimum usable terminal geometry: the canvas (contentWidth x
// contentHeight) needs room for the window border and its horizontal
// padding around it. Anything smaller is unusable — nothing would fit
// without clipping or wrapping.
const (
	minWidth  = contentWidth + 4  // border (2) + horizontal padding (2)
	minHeight = contentHeight + 2 // border (2)
)

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

// View assembles the full screen: one shared header, a two-column
// workspace (game area left, event log right), and the shortcut bar.
// Every region sits in a rigid slot, so the geometry never reflows: the
// hero vitals stay directly above the shortcuts, the log readout is
// bottom-anchored like a console, and nothing shifts with content.
// Terminals that can't hold the fixed canvas get the bare fallback
// screen instead of a clipped frame.
func (m Model) View() string {
	if m.tooSmall() {
		return tooSmallView(m.width, m.height)
	}

	left := lipgloss.JoinVertical(lipgloss.Left,
		slot(m.body(), mainColWidth, bodyHeight, lipgloss.Top),
		slot(m.heroPanel(), mainColWidth, heroHeight, lipgloss.Top),
	)
	right := styleLogColumn.Render(
		slot(m.rightColumn(), logColWidth-1, middleHeight, m.rightAnchor()),
	)

	content := lipgloss.JoinVertical(lipgloss.Left,
		slot(m.header(), contentWidth, 1, lipgloss.Top),
		lipgloss.JoinHorizontal(lipgloss.Top, left, right),
		slot(m.actionBar(), contentWidth, 1, lipgloss.Top),
	)
	return styleWindow.Render(content)
}

// rightColumn is the right band's content: the event log console
// everywhere, except the three trade states (loot, bag, merchant)
// where the equipped panel takes over — gear decisions are what those
// screens are for, and the log is least useful exactly then.
func (m Model) rightColumn() string {
	if m.showsEquipment() {
		return m.equippedPanel()
	}
	return m.logPanel()
}

// rightAnchor picks the right band's vertical anchor: the log grows
// from the bottom like a console; the equipped panel is a titled table
// and reads from the top.
func (m Model) rightAnchor() lipgloss.Position {
	if m.showsEquipment() {
		return lipgloss.Top
	}
	return lipgloss.Bottom
}

// showsEquipment reports whether the current state swaps the log for
// the equipped panel: the trade states where the hero faces gear
// choices (chest decision, bag, shop).
func (m Model) showsEquipment() bool {
	switch m.engine.State {
	case engine.StateLoot, engine.StateInventory, engine.StateMerchant:
		return true
	default:
		return false
	}
}

// tooSmall reports whether a WindowSizeMsg has arrived and the terminal
// can't hold the fixed canvas. No size reported yet (tests, first frame)
// counts as fine: the model renders its canvas and learns the truth with
// the first resize event.
func (m Model) tooSmall() bool {
	if m.width == 0 {
		return false
	}
	return m.width < minWidth || m.height < minHeight
}

// tooSmallView is the fallback for terminals smaller than the canvas:
// a bare, centered notice — no game frame, no action bar, nothing that
// could overflow or wrap. Details are added only while they fit: the
// notice must stay inside a terminal that may be arbitrarily narrow.
func tooSmallView(w, h int) string {
	msg := fmt.Sprintf("Terminal too small — needs %dx%d", minWidth, minHeight)
	if got := fmt.Sprintf(", got %dx%d", w, h); w >= len([]rune(msg))+len([]rune(got)) {
		msg += got
	}
	if w < len([]rune(msg)) {
		msg = string([]rune(msg)[:max(w, 1)])
	}
	return lipgloss.Place(max(w, 1), max(h, 1), lipgloss.Center, lipgloss.Center, styleDim.Render(msg))
}

// slot clamps a section's width and pads it to its exact height. Clamping
// width per section (ANSI-aware) keeps a wordy line in one slot from
// breaking the shared frame; the height padding keeps the section in its
// own reserved rows.
func slot(content string, width, height int, vAlign lipgloss.Position) string {
	fitted := lipgloss.NewStyle().
		MaxWidth(width).
		MaxHeight(height).
		Render(content)
	return lipgloss.Place(width, height, lipgloss.Top, vAlign, fitted)
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

// badge marks the current state for a quick state read. Unknown states
// fall onto their own style so a broken FSM never renders a blank chip.
func (m Model) badge() string {
	var style lipgloss.Style
	switch m.engine.State {
	case engine.StateExploring:
		style = styleBadgeOk
	case engine.StateEncounter:
		style = styleBadgeWarn
	case engine.StateCombat:
		style = styleBadgeCombat
	case engine.StateLoot:
		style = styleBadgeWarn
	case engine.StateMerchant:
		style = styleBadgeWarn
	case engine.StateInventory:
		style = styleBadgeOk
	case engine.StateGameOver:
		style = styleBadgeOver
	default:
		style = styleBadgeUnknown
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

// body swaps the game area (top-left) per state. The hero vitals are not
// part of it: they live in their own slot anchored above the shortcut
// bar, so they never move between states.
func (m Model) body() string {
	switch m.engine.State {
	case engine.StateExploring:
		return styleDim.Render("The tower awaits — EXPLORE to investigate.")
	case engine.StateLoot: // the pending item, ahead of the three-way choice
		return m.lootPanel()
	case engine.StateInventory: // the whole bag, selection included
		return m.inventoryPanel()
	case engine.StateMerchant: // the shop: offers and bag in one list
		return m.merchantPanel()
	case engine.StateEncounter: // the whole queue helps weigh FIGHT vs SNEAK
		return m.encounterPanel()
	case engine.StateCombat: // only the front enemy matters mid-fight
		return m.combatPanel()
	case engine.StateGameOver:
		return m.gameOverPanel()
	default:
		return ""
	}
}

// lootPanel is the chest-decision screen: the pending item and the
// three-way choice. The decision is mandatory — the panel deliberately
// offers no way out but the three actions.
func (m Model) lootPanel() string {
	var b strings.Builder
	b.WriteString(stylePanelTitle.Render("◈ CHEST LOOT") + "\n")
	if item := m.engine.PendingLoot; item != nil {
		b.WriteString("  " + stylePanelTitle.Render(item.Name) + itemBonusList(item) + "\n")
	} else {
		b.WriteString(styleDim.Render("   (the chest is empty)") + "\n")
	}
	b.WriteString(styleDim.Render("  decide — wear it, bag it, or let it go."))
	return b.String()
}

// inventoryPanel is the bag screen: one row per stashed item, the
// selected row highlighted with ▶ (the row WEAR/DROP act on). The
// selection is clamped to the bag: a shrunk bag must never render a
// ghost row.
func (m Model) inventoryPanel() string {
	var b strings.Builder
	bag := m.engine.Player.Bag()
	b.WriteString(stylePanelTitle.Render(
		fmt.Sprintf("◈ BAG (%d/%d)", len(bag), entities.InventoryCapacity),
	) + "\n")
	if len(bag) == 0 {
		return b.String() + styleDim.Render("   (the bag is empty)")
	}
	selected := min(max(m.bagCursor, 0), len(bag)-1)
	for i, item := range bag {
		name := item.Name + itemBonusList(item)
		if i == selected {
			b.WriteString(styleSelected.Render("▶ "+name) + "\n")
		} else {
			b.WriteString("  " + styleAction.Render(name) + "\n")
		}
	}
	return b.String()
}

// merchantPanel is the shop screen: the merchant's offers on top, the
// hero's bag below — one combined list the row cursor walks (stock rows
// first, so the cursor doubles as CmdBuy's stock index). Stock rows show
// the buy price, bag rows what the merchant pays — the difference is
// what makes BUY vs SELL enabled. The body slot holds 16 rows and the
// worst case is title (1) + full stock (4, the engine's
// merchantStockSize) + bag label (1) + a full bag (10) = 16: the panel
// always fits, never leaning on slot() to clip.
func (m Model) merchantPanel() string {
	stock := m.engine.MerchantStock
	bag := m.engine.Player.Bag()
	rows := make([]string, 0, 1+len(stock)+1+max(1, len(bag)))

	rows = append(rows, stylePanelTitle.Render("◈ MERCHANT"))
	if len(stock) == 0 {
		// The offer list empties as the hero buys; the shop stays open
		// for selling (and leaving).
		rows = append(rows, styleDim.Render("   (nothing left to buy)"))
	}
	for i, item := range stock {
		rows = append(rows, m.shopRow(i, item, fmt.Sprintf(" — %d gold", item.BuyPrice())))
	}
	rows = append(rows, styleDim.Render(fmt.Sprintf("  YOUR BAG (%d/%d)", len(bag), entities.InventoryCapacity)))
	if len(bag) == 0 {
		rows = append(rows, styleDim.Render("   (nothing to sell)"))
	}
	for i, item := range bag {
		rows = append(rows, m.shopRow(len(stock)+i, item, fmt.Sprintf(" — sells for %d gold", item.SellValue())))
	}
	return strings.Join(rows, "\n")
}

// shopRow renders one selectable merchant row: the selection marker, the
// item's label and the dim price suffix. The label yields to the price
// when the row would overflow the left column — nothing here may lean
// on slot() to be clipped.
func (m Model) shopRow(index int, item *entities.Item, price string) string {
	label := item.Name + itemBonusList(item)
	if over := len([]rune(label)) + len([]rune(price)) + 2 - mainColWidth; over > 0 {
		label = truncateRunes(label, len([]rune(label))-over)
	}
	if index == m.merchantCursor() {
		return styleSelected.Render("▶ "+label) + styleDim.Render(price)
	}
	return "  " + styleAction.Render(label) + styleDim.Render(price)
}

// merchantCursor is the row cursor clamped to the combined stock+bag
// list: a shrunk list (after a purchase) must never render a ghost row.
func (m Model) merchantCursor() int {
	n := len(m.engine.MerchantStock) + len(m.engine.Player.Bag())
	return min(max(m.bagCursor, 0), max(n-1, 0))
}

// gameOverPanel is the body after death: the banner, then the run's
// recap — where the climb ended (floor + section), which hero fell and
// how many enemies it took down. Floor/level/kills come from the
// defeat event as captured by the model (with the engine's final state
// as fallback); XP is only available on the engine, not on the event.
func (m Model) gameOverPanel() string {
	p := m.engine.Player
	level, kills, hasKills := p.Lvl, 0, false
	if m.recap != nil {
		level = m.recap.level
		kills, hasKills = m.recap.kills, m.recap.hasKills
	}

	var b strings.Builder
	b.WriteString(styleDeath.Render("☠ YOU DIED — the tower claims another soul.") + "\n\n")
	b.WriteString("  Reached " + styleLocation.Render(m.locationLabel()) + "\n")

	summary := fmt.Sprintf("Level %d · %d XP gathered", level, p.Xp)
	if hasKills {
		// Kills stay on the line only when the event carried a count:
		// a legacy event doesn't turn into a suspicious "0 kills".
		summary = fmt.Sprintf("Level %d · %d kills · %d XP gathered", level, kills, p.Xp)
	}
	b.WriteString(styleDim.Render("  " + summary))
	return b.String()
}

// encounterPanel lists every enemy in the queue. Nothing is engaged yet,
// so there is no ▶ marker — this is the planning screen for FIGHT/SNEAK.
// Each enemy spends two lines: name + HP, then its combat profile — the
// queue holds at most MAX_ENCOUNTER_SIZE enemies, so both stay inside
// the body slot's height.
func (m Model) encounterPanel() string {
	var b strings.Builder
	b.WriteString(stylePanelTitle.Render("⚔ ENEMIES") + "\n")
	monsters := m.engine.CurrentEncounter.Monsters
	if len(monsters) == 0 {
		return b.String() + styleDim.Render("   (no enemies in sight)")
	}
	// Pad names so the HP bars line up across the queue.
	nameWidth := maxNameWidth(monsters)
	for i := range monsters {
		mon := monsters[i]
		b.WriteString("  " + styleAction.Render(padRight(mon.Name(), nameWidth)) +
			"  " + hpBar(mon.CurrentHP, mon.MaxHP()) + "\n")
		b.WriteString("    " + statLine(mon) + "\n")
	}
	return b.String()
}

// combatPanel is the combat screen: the enemy currently in front of the
// hero — the rest of the queue appears only as a counter — with its stat
// line and status chips, and the 2×2 move grid below it. Just like the
// encounter panel it shows the engaged enemy's stat line.
func (m Model) combatPanel() string {
	var b strings.Builder
	b.WriteString(stylePanelTitle.Render("⚔ ENEMIES") + "\n")
	monsters := m.engine.CurrentEncounter.Monsters
	if len(monsters) == 0 {
		return b.String() + styleDim.Render("   (no enemies in sight)")
	}
	front := monsters[0]
	b.WriteString("▶ " + stylePanelTitle.Render(front.Name()) +
		"  " + hpBar(front.CurrentHP, front.MaxHP()) + "\n")
	b.WriteString("  " + statLine(front) + "\n")
	if rows := m.enemyStatusRows(); rows != "" {
		b.WriteString(rows + "\n")
	}
	if waiting := len(monsters) - 1; waiting > 0 {
		b.WriteString(styleDim.Render(fmt.Sprintf("  · %d more waiting", waiting)) + "\n")
	}
	b.WriteString(m.moveGrid())
	return b.String()
}

// The move grid: a 2×2 block of cells, two per row, each cell TWO lines
// tall (the move name over its tags) so the widest shipped move
// ("Assassin's Strike", 17 runes) and its tags always fit the
// half-column. gridCellWidth is exactly half the game column; the rows
// join two padded cells and never exceed mainColWidth.
const (
	gridCols      = 2
	gridCellWidth = mainColWidth / gridCols // 23
)

// moveGrid renders the combat move grid: four fixed cells in the hero's
// effective move order — the exact order CmdMove's index points into.
// Each cell shows the move name over a dim tag line (see gridTags); a
// cooling move's cell grays out and its tags yield to the turns-left
// marker; cells beyond the move set render a dim — placeholder. The
// cell cursor (gridCursor) only ever rests on real moves, highlighted
// like every other selection.
func (m Model) moveGrid() string {
	moves := m.engine.CombatMoves()
	rows := []string{stylePanelTitle.Render("◈ MOVES")}
	for row := range gridCols {
		names := make([]string, 0, gridCols)
		tags := make([]string, 0, gridCols)
		for col := range gridCols {
			name, tag := m.gridCell(moves, row*gridCols+col)
			names = append(names, name)
			tags = append(tags, tag)
		}
		rows = append(rows,
			lipgloss.JoinHorizontal(lipgloss.Top, names...),
			lipgloss.JoinHorizontal(lipgloss.Top, tags...),
		)
	}
	return strings.Join(rows, "\n")
}

// gridCell renders one cell's two lines (name, tags), padded to the
// cell width so the two cells of a row align. The cursor's cell gets
// the shared selection style — full-width background highlight; a
// cooling cell grays out (unless the cursor rests on it: visibility of
// where you ARE wins over the gray — dispatching still works and the
// engine explains the block).
func (m Model) gridCell(moves []entities.Move, i int) (name, tags string) {
	if i >= len(moves) {
		return styleDim.Render(padRight("—", gridCellWidth)),
			strings.Repeat(" ", gridCellWidth)
	}
	move := moves[i]
	cooling := m.mirror.cooldownLeft(move.ID) > 0
	style := styleAction
	name = "  " + move.Name
	if cooling {
		style = styleDisabled
	}
	if i == min(max(m.gridCursor, 0), len(moves)-1) {
		name = "▶ " + move.Name
		style = styleSelected
	}
	return style.Render(padRight(name, gridCellWidth)),
		style.Render(padRight(truncateRunes(m.gridTags(move, cooling), gridCellWidth), gridCellWidth))
}

// gridTags is a cell's tag line: the move's damage modifier, its status
// tag ("☠poison 60%" — the status ID; the move payload carries ids, not
// names) and its crit bonus ("✦+20"). Shipped content never carries all
// three on one move, so the line stays ≤ 2 short tags; a modded asset
// that packs more truncates at the cell width (the guard, never slot()).
// A cooling move shows ONLY the turns-left marker: the tags don't matter
// while the move is unusable.
func (m Model) gridTags(move entities.Move, cooling bool) string {
	if cooling {
		return fmt.Sprintf("   (%d turns)", m.mirror.cooldownLeft(move.ID))
	}
	tags := []string{fmt.Sprintf("%d%%", move.DmgModifier)}
	if move.HasStatus() {
		tags = append(tags, fmt.Sprintf("☠%s %d%%", move.Status.ID, move.Status.Chance))
	}
	if move.CritBonus != 0 {
		tags = append(tags, fmt.Sprintf("✦%+d", move.CritBonus))
	}
	return "   " + strings.Join(tags, " ")
}

// maxStatusRows is the fixed budget for the enemy block's status chips:
// they wrap into at most two rows, then the display drops what doesn't
// fit — the frame's height never moves for statuses (the log carries
// every application/expiry faithfully).
const maxStatusRows = 2

// statusChip renders one active status as "Poison(2)" — the name the
// event payload carried plus the rounds still left — toned by kind:
// debuffs soft-red, buffs green (the same tones the log uses).
func statusChip(s combatStatus) string {
	label := fmt.Sprintf("%s(%d)", s.name, s.turns)
	if s.kind == string(entities.StatusBuff) {
		return styleHeal.Render(label)
	}
	return styleHurt.Render(label)
}

// statusChips renders one chip per active status, in application order.
func statusChips(statuses []combatStatus) []string {
	chips := make([]string, 0, len(statuses))
	for _, s := range statuses {
		chips = append(chips, statusChip(s))
	}
	return chips
}

// wrapChips lays chips into rows of at most maxWidth (lipgloss-aware
// widths), at most maxRows rows; chips that no longer fit — including a
// single chip wider than a whole row — are dropped. The cap is the
// documented display budget, never slot() clipping.
func wrapChips(chips []string, maxWidth, maxRows int) []string {
	rows := make([]string, 0, maxRows)
	var row []string
	width := 0
	for _, chip := range chips {
		w := lipgloss.Width(chip) + 1 // the separator space
		if len(row) == 0 {
			w-- // no separator on the first chip of a row
		}
		if width+w > maxWidth && len(row) > 0 {
			rows = append(rows, strings.Join(row, " "))
			row, width = nil, 0
			w = lipgloss.Width(chip)
			if len(rows) == maxRows {
				return rows // the cap: the rest is dropped
			}
		}
		if w > maxWidth {
			continue // a lone chip wider than the budget: dropped
		}
		row = append(row, chip)
		width += w
	}
	if len(row) > 0 && len(rows) < maxRows {
		rows = append(rows, strings.Join(row, " "))
	}
	return rows
}

// enemyStatusRows is the enemy block's status rows: the engaged enemy's
// active chips, wrapped to at most maxStatusRows rows inside the column
// (the two-space indent is part of the budget).
func (m Model) enemyStatusRows() string {
	rows := wrapChips(statusChips(m.mirror.enemyStatuses), mainColWidth-2, maxStatusRows)
	for i := range rows {
		rows[i] = "  " + rows[i]
	}
	return strings.Join(rows, "\n")
}

// statLine is a monster's combat profile on one compact, dim line:
// damage, defense, speed, crit chance and difficulty — the same
// DMG→DEF→SPD order the hero panel uses, prefixed with the ⚔ combat
// icon of the panel titles. Compact by design: it must never overflow
// the left column's width (mainColWidth), which slot() would silently
// clip.
func statLine(mon entities.Monster) string {
	return styleDim.Render(fmt.Sprintf("⚔ %d · ⛨ %d · » %d · ✦ %d%% · ⬆ %d",
		mon.Damage(), mon.Defense(), mon.Speed(), mon.CritThreshold(), mon.Difficulty))
}

// maxNameWidth is the longest (rune-wise) monster name in the queue.
func maxNameWidth(monsters []entities.Monster) int {
	width := 0
	for i := range monsters {
		if n := len([]rune(monsters[i].Name())); n > width {
			width = n
		}
	}
	return width
}

// padRight pads s with spaces on the right up to width runes.
func padRight(s string, width int) string {
	if pad := width - len([]rune(s)); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

// truncateRunes cuts s to at most width runes. A plain cut, no ellipsis:
// keeping the row inside the column is what matters, and a filler glyph
// risks the frame's byte-exactness.
func truncateRunes(s string, width int) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:max(width, 0)])
}

// heroPanel is the player's vital stats: level, XP, HP and the derived
// stats with the purse. Gear is deliberately not listed — the equipped
// panel owns that view in the trade states. While in combat the HP row
// carries the hero's active status chips (heroStatusChips) — the panel's
// four-line shape never changes.
func (m Model) heroPanel() string {
	p := m.engine.Player
	var b strings.Builder
	b.WriteString(stylePanelTitle.Render("HERO") + "\n")
	b.WriteString(fmt.Sprintf("  Lv %d", p.Lvl))
	b.WriteString(styleDim.Render(fmt.Sprintf("   XP %s %d/%d to Lv %d", xpBar(p), p.Xp, p.Xp+p.XpToLevel(), p.Lvl+1)))
	b.WriteString("\n")
	b.WriteString(m.heroHPRow() + "\n")
	b.WriteString(fmt.Sprintf("  DMG %d · DEF %d · SPD %d · ", p.Damage(), p.Defense(), p.Speed()))
	// The purse shares the stats line, dim like the panel's XP segment:
	// the panel labels stay quiet, the log carries the gold-colored news.
	b.WriteString(styleDim.Render(fmt.Sprintf("GOLD %d", p.Gold)))
	return b.String()
}

// heroHPRow is the HP line of the hero panel — bar and readout, then as
// many status chips as fit within the column: the first chips that fit
// render, the rest is dropped (the documented display cap; the log
// carries every status faithfully).
func (m Model) heroHPRow() string {
	p := m.engine.Player
	row := fmt.Sprintf("  HP %s %d/%d", hpBarPlain(p.CurrentHP, p.MaxHP()), p.CurrentHP, p.MaxHP())
	for _, chip := range statusChips(m.mirror.heroStatuses) {
		if lipgloss.Width(row)+1+lipgloss.Width(chip) > mainColWidth {
			break
		}
		row += " " + chip
	}
	return row
}

func (m Model) logPanel() string {
	return strings.Join(m.log, "\n")
}

// equippedPanel is the right column during the trade states: every
// equipment slot in fixed order. Occupied slots spend TWO lines — the
// item name, then its full bonus list dim beneath it — because the
// worst shipped asset ("Armadura de Escamas de Dragão
// (+30 HP, +20 DEF, -10 SPD)", 56 runes) cannot fit one 49-char column
// line; empty slots spend one dim row. Worst case: title + 6×2 = 13
// lines, well inside the middle band's 20, and both lines of an
// occupied slot are width-guarded — nothing leans on slot() to clip.
func (m Model) equippedPanel() string {
	rows := []string{stylePanelTitle.Render("◈ EQUIPPED")}
	for _, s := range equippedSlots(m.engine.Player) {
		label := padRight(s.label, equippedLabelWidth)
		if s.item == nil {
			rows = append(rows, styleDim.Render(fmt.Sprintf("  %s  — (empty)", label)))
			continue
		}
		// The width guard only ever bites on a modded asset longer than
		// any shipped name; the bonus list cannot overflow the rest.
		rows = append(rows, fmt.Sprintf("  %s  %s",
			label, truncateRunes(s.item.Name, logColWidth-1-equippedIndent)))
		if bonus := itemBonusList(s.item); bonus != "" {
			rows = append(rows, styleDim.Render(
				strings.Repeat(" ", equippedIndent)+truncateRunes(bonus, logColWidth-1-equippedIndent)))
		}
	}
	return strings.Join(rows, "\n")
}

// actionBar renders the current state's options with the cursor
// highlighted; the controls hint closes the line. While the quit
// confirmation is armed the whole bar is replaced by the Really-quit
// prompt — no options, no hint: the only way out of the prompt is
// choosing it (y) or cancelling (any other key).
func (m Model) actionBar() string {
	if m.confirmQuit {
		return styleSelected.Render("Really quit? [Y] Yes / [N] No")
	}
	actions := m.barActions()
	// The combat bar is a key legend, not a selectable list: the arrows
	// and Enter belong to the move grid there, so its two spots render
	// plainly (single-space separators — compact on purpose).
	hint := "   ← → select · ⏎ confirm"
	sep := "   "
	inCombat := m.engine.State == engine.StateCombat
	if inCombat {
		hint, sep = "", " "
	}
	spots := make([]string, 0, len(actions))
	for i, a := range actions {
		label := fmt.Sprintf("[%s] %s", strings.ToUpper(a.key), a.label)
		if !a.enabled {
			spots = append(spots, styleDisabled.Render(label+" ✗"))
			continue
		}
		if i == m.cursor && !inCombat {
			spots = append(spots, styleSelected.Render(" "+label+" "))
			continue
		}
		spots = append(spots, styleAction.Render(label))
	}
	// The hint names what the arrows do here: pick a bar option elsewhere,
	// walk the bag rows inside the bag, the shop's combined list while
	// shopping — while the chest screen has nothing to select, only the
	// decision to make.
	switch m.engine.State {
	case engine.StateInventory:
		hint = "   ← → item · ⏎ confirm"
	case engine.StateMerchant:
		hint = "   ← → row · ⏎ act"
	case engine.StateLoot:
		hint = "   decide the loot's fate"
	}
	if hint == "" {
		return lipgloss.JoinHorizontal(lipgloss.Center, strings.Join(spots, sep))
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, strings.Join(spots, sep), styleDim.Render(hint))
}

// gearSlot pairs an equipment slot's short panel label with the item
// worn there (nil = empty).
type gearSlot struct {
	label string
	item  *entities.Item
}

// equippedSlots is the hero's equipment in the fixed order every gear
// rendering shares: Head, Body, Feet, Wpn, Ring, Amu.
func equippedSlots(p *entities.Hero) []gearSlot {
	return []gearSlot{
		{"Head", p.Head}, {"Body", p.Body}, {"Feet", p.Feet},
		{"Wpn", p.Weapon}, {"Ring", p.Ring}, {"Amu", p.Amulet},
	}
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
