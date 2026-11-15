package commands

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/iqbalmu/xm-db/internal/container"
	"github.com/iqbalmu/xm-db/internal/providers"
	"github.com/iqbalmu/xm-db/internal/state"
)

const dashboardRefreshInterval = 5 * time.Second

type dbEntry struct {
	db      *state.Database
	running bool
	connStr string
}

// Messages
type msgDatabasesLoaded struct {
	entries   []dbEntry
	dockerErr error
}

type msgDetailLoaded struct {
	name   string
	tab    int
	logs   string
	tables []string
	err    error
}

type msgActionDone struct {
	toast string
	err   error
}

type msgTick time.Time

// tea.ExecCommand wrappers so Bubble Tea suspends alt-screen cleanly for
// interactive sub-flows (create wizard and SQL shell).
type createExecCmd struct{}

func (c createExecCmd) Run() error {
	_, err := runCreate(nil)
	return err
}
func (c createExecCmd) SetStdin(io.Reader)  {}
func (c createExecCmd) SetStdout(io.Writer) {}
func (c createExecCmd) SetStderr(io.Writer) {}

type shellExecCmd struct {
	name string
}

func (s shellExecCmd) Run() error {
	err := runShell(s.name)
	if err != nil {
		fmt.Printf("\nShell error: %v\nPress Enter to return to dashboard...", err)
		var dummy string
		_, _ = fmt.Scanln(&dummy)
	}
	return nil
}
func (s shellExecCmd) SetStdin(io.Reader)  {}
func (s shellExecCmd) SetStdout(io.Writer) {}
func (s shellExecCmd) SetStderr(io.Writer) {}

type dashboardModel struct {
	width  int
	height int

	entries   []dbEntry
	filtered  []dbEntry
	cursor    int
	dockerErr error

	activeTab    int // 0=Info, 1=Logs, 2=Tables
	detailLogs   string
	detailTables []string
	detailErr    error
	detailOffset int
	showPassword bool

	filtering   bool
	filterInput textinput.Model

	confirmMode string // "", "remove", "reset"
	busy        string
	toast       string
	toastIsErr  bool
}

func newDashboardModel() dashboardModel {
	fi := textinput.New()
	fi.Placeholder = "filter by name or group..."
	fi.CharLimit = 64
	fi.Width = 28

	return dashboardModel{
		width:       100,
		height:      30,
		filterInput: fi,
		busy:        "Loading databases...",
	}
}

func (m dashboardModel) Init() tea.Cmd {
	return tea.Batch(
		loadDatabasesCmd(),
		tickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(dashboardRefreshInterval, func(t time.Time) tea.Msg {
		return msgTick(t)
	})
}

func loadDatabasesCmd() tea.Cmd {
	return func() tea.Msg {
		stateMgr, err := state.NewManager()
		if err != nil {
			return msgDatabasesLoaded{dockerErr: err}
		}
		dbs, err := stateMgr.List()
		if err != nil {
			return msgDatabasesLoaded{dockerErr: err}
		}

		dockerMgr, dockerErr := container.NewDockerManager(runtimeMode)
		ctx := context.Background()

		entries := make([]dbEntry, 0, len(dbs))
		for _, db := range dbs {
			running := false
			if dockerErr == nil && db.ContainerID != "" {
				if r, err := dockerMgr.IsContainerRunning(ctx, db.ContainerID, db.ContainerName); err == nil {
					running = r
				}
			}
			connStr := ""
			if prov, err := providers.For(db.Provider); err == nil {
				connStr = prov.GetConnectionString(connectionConfig(db))
			}
			entries = append(entries, dbEntry{
				db:      db,
				running: running,
				connStr: connStr,
			})
		}
		return msgDatabasesLoaded{entries: entries, dockerErr: dockerErr}
	}
}

func loadDetailCmd(entry *dbEntry, tab int) tea.Cmd {
	if entry == nil || tab == 0 {
		return nil
	}
	db := entry.db
	running := entry.running
	return func() tea.Msg {
		if tab == 1 {
			dockerMgr, err := container.NewDockerManager(runtimeMode)
			if err != nil {
				return msgDetailLoaded{name: db.Name, tab: tab, err: err}
			}
			logs, err := dockerMgr.TailLogs(context.Background(), db.ContainerID, db.ContainerName, 80)
			return msgDetailLoaded{name: db.Name, tab: tab, logs: logs, err: err}
		}
		if tab == 2 {
			if !running {
				return msgDetailLoaded{name: db.Name, tab: tab, err: fmt.Errorf("database is stopped")}
			}
			prov, err := providers.For(db.Provider)
			if err != nil {
				return msgDetailLoaded{name: db.Name, tab: tab, err: err}
			}
			conn, err := prov.Connect(&providers.DBConfig{
				Host:         "127.0.0.1",
				Port:         db.Port,
				RootPassword: db.RootPassword,
				DatabaseName: db.DatabaseName,
			})
			if err != nil {
				return msgDetailLoaded{name: db.Name, tab: tab, err: err}
			}
			defer conn.Close()
			tables, err := prov.ListTables(context.Background(), conn, db.DatabaseName)
			return msgDetailLoaded{name: db.Name, tab: tab, tables: tables, err: err}
		}
		return nil
	}
}

func toggleDBCmd(entry dbEntry) tea.Cmd {
	return func() tea.Msg {
		dockerMgr, err := container.NewDockerManager(runtimeMode)
		if err != nil {
			return msgActionDone{err: err}
		}
		ctx := context.Background()
		if entry.running {
			if err := dockerMgr.StopContainer(ctx, entry.db.ContainerID, entry.db.ContainerName); err != nil {
				return msgActionDone{err: fmt.Errorf("stopping %s: %w", entry.db.Name, err)}
			}
			return msgActionDone{toast: fmt.Sprintf("Stopped '%s'", entry.db.Name)}
		}
		if err := dockerMgr.StartContainer(ctx, entry.db.ContainerID, entry.db.ContainerName); err != nil {
			return msgActionDone{err: fmt.Errorf("starting %s: %w", entry.db.Name, err)}
		}
		return msgActionDone{toast: fmt.Sprintf("Started '%s'", entry.db.Name)}
	}
}

func removeDBCmd(entry dbEntry) tea.Cmd {
	return func() tea.Msg {
		stateMgr, err := state.NewManager()
		if err != nil {
			return msgActionDone{err: err}
		}
		dockerMgr, err := container.NewDockerManager(runtimeMode)
		if err != nil {
			return msgActionDone{err: err}
		}
		ctx := context.Background()
		if entry.running {
			_ = dockerMgr.StopContainer(ctx, entry.db.ContainerID, entry.db.ContainerName)
		}
		_ = dockerMgr.RemoveContainer(ctx, entry.db.ContainerID, entry.db.ContainerName)
		if entry.db.VolumeName != "" {
			_ = dockerMgr.RemoveVolume(ctx, entry.db.VolumeName)
		}
		if err := stateMgr.Delete(entry.db.Name); err != nil {
			return msgActionDone{err: err}
		}
		return msgActionDone{toast: fmt.Sprintf("Removed '%s'", entry.db.Name)}
	}
}

func resetDBCmd(entry dbEntry) tea.Cmd {
	return func() tea.Msg {
		resetForce = true
		defer func() { resetForce = false }()
		if err := runReset(entry.db.Name); err != nil {
			return msgActionDone{err: err}
		}
		return msgActionDone{toast: fmt.Sprintf("Reset '%s' complete", entry.db.Name)}
	}
}

func (m *dashboardModel) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(m.filterInput.Value()))
	if q == "" {
		m.filtered = m.entries
	} else {
		var out []dbEntry
		for _, e := range m.entries {
			if strings.Contains(strings.ToLower(e.db.Name), q) ||
				strings.Contains(strings.ToLower(groupLabel(e.db.Group)), q) ||
				strings.Contains(strings.ToLower(e.db.Provider), q) {
				out = append(out, e)
			}
		}
		m.filtered = out
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m dashboardModel) selected() *dbEntry {
	if len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	return &m.filtered[m.cursor]
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case msgTick:
		return m, tea.Batch(loadDatabasesCmd(), tickCmd())

	case msgDatabasesLoaded:
		m.busy = ""
		m.dockerErr = msg.dockerErr
		prevName := ""
		if sel := m.selected(); sel != nil {
			prevName = sel.db.Name
		}
		m.entries = msg.entries
		m.applyFilter()
		if prevName != "" {
			for i, e := range m.filtered {
				if e.db.Name == prevName {
					m.cursor = i
					break
				}
			}
		}
		return m, loadDetailCmd(m.selected(), m.activeTab)

	case msgDetailLoaded:
		if sel := m.selected(); sel != nil && sel.db.Name == msg.name && m.activeTab == msg.tab {
			m.detailLogs = msg.logs
			m.detailTables = msg.tables
			m.detailErr = msg.err
		}
		return m, nil

	case msgActionDone:
		m.busy = ""
		if msg.err != nil {
			m.toast = msg.err.Error()
			m.toastIsErr = true
		} else {
			m.toast = msg.toast
			m.toastIsErr = false
		}
		return m, loadDatabasesCmd()

	case tea.KeyMsg:
		// Handle confirmation prompt first
		if m.confirmMode != "" {
			sel := m.selected()
			mode := m.confirmMode
			m.confirmMode = ""
			if strings.ToLower(msg.String()) == "y" && sel != nil {
				if mode == "remove" {
					m.busy = fmt.Sprintf("Removing '%s'...", sel.db.Name)
					return m, removeDBCmd(*sel)
				}
				if mode == "reset" {
					m.busy = fmt.Sprintf("Resetting '%s'...", sel.db.Name)
					return m, resetDBCmd(*sel)
				}
			}
			m.toast = "Cancelled"
			m.toastIsErr = false
			return m, nil
		}

		// Handle filter input mode
		if m.filtering {
			switch msg.String() {
			case "esc":
				m.filtering = false
				m.filterInput.SetValue("")
				m.filterInput.Blur()
				m.applyFilter()
				return m, loadDetailCmd(m.selected(), m.activeTab)
			case "enter":
				m.filtering = false
				m.filterInput.Blur()
				return m, loadDetailCmd(m.selected(), m.activeTab)
			}
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			m.applyFilter()
			return m, cmd
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "/":
			m.filtering = true
			m.filterInput.Focus()
			return m, textinput.Blink

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.detailOffset = 0
				return m, loadDetailCmd(m.selected(), m.activeTab)
			}

		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m.detailOffset = 0
				return m, loadDetailCmd(m.selected(), m.activeTab)
			}

		case "tab":
			m.activeTab = (m.activeTab + 1) % 3
			m.detailOffset = 0
			return m, loadDetailCmd(m.selected(), m.activeTab)

		case "1":
			m.activeTab = 0
			m.detailOffset = 0
			return m, nil

		case "2":
			m.activeTab = 1
			m.detailOffset = 0
			return m, loadDetailCmd(m.selected(), 1)

		case "3":
			m.activeTab = 2
			m.detailOffset = 0
			return m, loadDetailCmd(m.selected(), 2)

		case "pgup", "ctrl+u":
			if m.detailOffset > 0 {
				m.detailOffset = max(0, m.detailOffset-5)
			}

		case "pgdown", "ctrl+d":
			m.detailOffset += 5

		case "p":
			m.showPassword = !m.showPassword

		case "r":
			m.busy = "Refreshing..."
			return m, loadDatabasesCmd()

		case "s":
			if sel := m.selected(); sel != nil && m.busy == "" {
				if sel.running {
					m.busy = fmt.Sprintf("Stopping '%s'...", sel.db.Name)
				} else {
					m.busy = fmt.Sprintf("Starting '%s'...", sel.db.Name)
				}
				return m, toggleDBCmd(*sel)
			}

		case "c":
			if sel := m.selected(); sel != nil && sel.connStr != "" {
				if err := clipboard.WriteAll(sel.connStr); err != nil {
					m.toast = fmt.Sprintf("Clipboard error: %v", err)
					m.toastIsErr = true
				} else {
					m.toast = fmt.Sprintf("Copied connection string for '%s'", sel.db.Name)
					m.toastIsErr = false
				}
			}

		case "x":
			if sel := m.selected(); sel != nil {
				if !sel.running {
					m.toast = fmt.Sprintf("'%s' is stopped — press 's' to start it first", sel.db.Name)
					m.toastIsErr = true
					return m, nil
				}
				return m, tea.Exec(shellExecCmd{name: sel.db.Name}, func(err error) tea.Msg {
					return msgDatabasesLoaded{}
				})
			}

		case "n":
			return m, tea.Exec(createExecCmd{}, func(err error) tea.Msg {
				if err != nil {
					return msgActionDone{err: err}
				}
				return msgActionDone{toast: "Refreshed database list"}
			})

		case "d":
			if m.selected() != nil && m.busy == "" {
				m.confirmMode = "remove"
			}

		case "R":
			if m.selected() != nil && m.busy == "" {
				m.confirmMode = "reset"
			}
		}
	}

	return m, nil
}

func (m dashboardModel) View() string {
	// Header
	dockerBadge := styleBadgeRun.Render("● runtime ok")
	if m.dockerErr != nil {
		dockerBadge = styleErrorText.Render("✗ runtime unavailable")
	}
	header := fmt.Sprintf("%s   %s   %s",
		styleHeader.Render(" xm-db "),
		dockerBadge,
		styleMutedText.Render(fmt.Sprintf("%d database(s)", len(m.entries))),
	)

	// Layout dimensions
	bodyHeight := max(10, m.height-5)
	leftWidth := max(28, m.width*36/100)
	rightWidth := max(36, m.width-leftWidth-4)

	leftPane := m.renderLeftPane(leftWidth, bodyHeight)
	rightPane := m.renderRightPane(rightWidth, bodyHeight)
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)

	// Status / confirmation / help bar
	footer := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m dashboardModel) renderLeftPane(width, height int) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render("Databases") + "\n")

	if m.filtering {
		b.WriteString(m.filterInput.View() + "\n")
	} else if q := strings.TrimSpace(m.filterInput.Value()); q != "" {
		b.WriteString(styleMutedText.Render("Filter: "+q+" (Esc to clear)") + "\n")
	}
	b.WriteString("\n")

	if len(m.filtered) == 0 {
		if len(m.entries) == 0 {
			b.WriteString(styleMutedText.Render("No databases yet.\nPress 'n' to create one!"))
		} else {
			b.WriteString(styleMutedText.Render("No databases match filter."))
		}
	} else {
		innerW := max(16, width-4)
		for i, e := range m.filtered {
			badge := styleBadgeStop.Render("○")
			if e.running {
				badge = styleBadgeRun.Render("●")
			}
			line := fmt.Sprintf("%s %s (%s:%d) [%s]", badge, e.db.Name, e.db.Provider, e.db.Port, groupLabel(e.db.Group))
			if len(line) > innerW {
				line = line[:innerW-1] + "…"
			}
			if i == m.cursor {
				b.WriteString(styleSelectedRow.Width(innerW).Render(line) + "\n")
			} else {
				b.WriteString(line + "\n")
			}
		}
	}

	return stylePaneFocused.Width(width).Height(height).Render(b.String())
}

func (m dashboardModel) renderRightPane(width, height int) string {
	tabs := []string{"[1] Info", "[2] Logs", "[3] Tables"}
	var tabParts []string
	for i, t := range tabs {
		if i == m.activeTab {
			tabParts = append(tabParts, styleTabActive.Render(t))
		} else {
			tabParts = append(tabParts, styleTabInactive.Render(t))
		}
	}
	tabBar := strings.Join(tabParts, "  ")

	sel := m.selected()
	if sel == nil {
		content := tabBar + "\n\n" + styleMutedText.Render("Select a database from the list.")
		return stylePane.Width(width).Height(height).Render(content)
	}

	var body string
	switch m.activeTab {
	case 0:
		body = m.renderInfoTab(sel)
	case 1:
		body = m.renderLogsTab(height - 5)
	case 2:
		body = m.renderTablesTab()
	}

	content := tabBar + "\n\n" + body
	return stylePane.Width(width).Height(height).Render(content)
}

func (m dashboardModel) renderInfoTab(e *dbEntry) string {
	db := e.db
	statusStr := styleBadgeStop.Render("○ stopped")
	if e.running {
		statusStr = styleBadgeRun.Render("● running")
	}

	pass := "•••••••••••• (press 'p' to reveal)"
	if m.showPassword {
		pass = db.GetPassword()
	}

	rows := [][2]string{
		{"Name", db.Name},
		{"Provider", fmt.Sprintf("%s (%s)", db.Provider, db.Version)},
		{"Group", groupLabel(db.Group)},
		{"Status", statusStr},
		{"Port", fmt.Sprintf("127.0.0.1:%d", db.Port)},
		{"Database", db.DatabaseName},
		{"User", db.GetUsername()},
		{"Password", pass},
		{"Container", db.ContainerName},
		{"Volume", db.VolumeName},
		{"Created", db.CreatedAt.Format("2006-01-02 15:04")},
	}

	var b strings.Builder
	for _, r := range rows {
		b.WriteString(styleLabel.Render(r[0]+":") + " " + r[1] + "\n")
	}
	b.WriteString("\n" + styleLabel.Render("Conn String:") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorSuccess).Render(e.connStr) + "\n")
	return b.String()
}

func (m dashboardModel) renderLogsTab(maxLines int) string {
	if m.detailErr != nil {
		return styleErrorText.Render("Error loading logs: " + m.detailErr.Error())
	}
	if strings.TrimSpace(m.detailLogs) == "" {
		return styleMutedText.Render("(no logs available)")
	}
	lines := strings.Split(strings.TrimRight(m.detailLogs, "\n"), "\n")
	if maxLines < 5 {
		maxLines = 5
	}
	start := max(0, len(lines)-maxLines-m.detailOffset)
	end := min(len(lines), start+maxLines)
	return strings.Join(lines[start:end], "\n")
}

func (m dashboardModel) renderTablesTab() string {
	if m.detailErr != nil {
		return styleMutedText.Render(m.detailErr.Error())
	}
	if len(m.detailTables) == 0 {
		return styleMutedText.Render("(no tables found)")
	}
	var b strings.Builder
	for i, t := range m.detailTables {
		b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, t))
	}
	return b.String()
}

func (m dashboardModel) renderFooter() string {
	if m.confirmMode != "" && m.selected() != nil {
		action := "Remove"
		if m.confirmMode == "reset" {
			action = "Reset (wipe all data)"
		}
		return styleErrorText.Render(fmt.Sprintf("  %s '%s'? Press [y] to confirm, any other key to cancel.", action, m.selected().db.Name))
	}

	var statusLine string
	if m.busy != "" {
		statusLine = lipgloss.NewStyle().Foreground(colorWarning).Render("  ⏳ " + m.busy)
	} else if m.toast != "" {
		if m.toastIsErr {
			statusLine = styleToastErr.Render("  ✗ " + m.toast)
		} else {
			statusLine = styleToastOK.Render("  ✓ " + m.toast)
		}
	}

	help := styleHelpBar.Render("  ↑/↓: select  •  Tab/1-3: tabs  •  s: start/stop  •  x: shell  •  c: copy  •  n: new  •  R: reset  •  d: remove  •  /: filter  •  r: refresh  •  q: quit")
	if statusLine != "" {
		return statusLine + "\n" + help
	}
	return help
}

func runDashboardTUI() {
	p := tea.NewProgram(newDashboardModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running TUI dashboard: %v\n", err)
	}
}
