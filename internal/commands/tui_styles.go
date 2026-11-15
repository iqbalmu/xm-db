package commands

import "github.com/charmbracelet/lipgloss"

var (
	colorPrimary   = lipgloss.Color("39")  // bright cyan/blue
	colorAccent    = lipgloss.Color("212") // magenta/pink
	colorSuccess   = lipgloss.Color("42")  // green
	colorWarning   = lipgloss.Color("214") // amber
	colorDanger    = lipgloss.Color("196") // red
	colorMuted     = lipgloss.Color("241") // gray
	colorSubtle    = lipgloss.Color("238") // dark border
	colorHighlight = lipgloss.Color("236") // selected row bg

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("24")).
			Padding(0, 1)

	stylePane = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorSubtle).
			Padding(0, 1)

	stylePaneFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorPrimary).
				Padding(0, 1)

	styleSelectedRow = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("15")).
				Background(colorHighlight)

	styleTabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Underline(true).
			Padding(0, 1)

	styleTabInactive = lipgloss.NewStyle().
				Foreground(colorMuted).
				Padding(0, 1)

	styleLabel = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("250")).
			Width(14)

	styleMutedText = lipgloss.NewStyle().Foreground(colorMuted)
	styleErrorText = lipgloss.NewStyle().Bold(true).Foreground(colorDanger)
	styleToastOK   = lipgloss.NewStyle().Bold(true).Foreground(colorSuccess)
	styleToastErr  = lipgloss.NewStyle().Bold(true).Foreground(colorDanger)
	styleBadgeRun  = lipgloss.NewStyle().Bold(true).Foreground(colorSuccess)
	styleBadgeStop = lipgloss.NewStyle().Foreground(colorMuted)

	styleHelpBar = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)
)
