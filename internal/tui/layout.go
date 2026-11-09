package tui

// Layout describes the responsive content geometry for a terminal size.
// LogWidth includes the divider between the main and log columns.
type Layout struct {
	TerminalWidth   int
	TerminalHeight  int
	ContentWidth    int
	ContentHeight   int
	MainWidth       int
	LogWidth        int
	WorkspaceHeight int
	BodyHeight      int
	HeroHeight      int
	HeaderHeight    int
	ActionBarHeight int
	TooSmall        bool
}

// CalculateLayout maps terminal dimensions to the game canvas geometry.
// Dimensions below the minimum are preserved (rather than clamped), while
// TooSmall allows callers to decide how to present that state.
func CalculateLayout(width, height int) Layout {
	canvasWidth, contentHeight := width-4, height-2
	extraWidth := canvasWidth - contentWidth
	mainWidth := mainColWidth + extraWidth/2
	logWidth := logColWidth + (extraWidth+1)/2
	workspaceHeight := contentHeight - 2
	return Layout{
		TerminalWidth: width, TerminalHeight: height,
		ContentWidth: canvasWidth, ContentHeight: contentHeight,
		MainWidth: mainWidth, LogWidth: logWidth,
		WorkspaceHeight: workspaceHeight, BodyHeight: workspaceHeight - heroHeight,
		HeroHeight: heroHeight, HeaderHeight: 1, ActionBarHeight: 1,
		TooSmall: width < minWidth || height < minHeight,
	}
}
