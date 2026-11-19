package commands

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/iqbalmu/xm-db/internal/container"
	"github.com/iqbalmu/xm-db/internal/providers"
	"github.com/iqbalmu/xm-db/internal/state"
)

const (
	dashboardRefreshInterval = 5 * time.Second
	toastDuration            = 4 * time.Second
	tabCount                 = 4
)

type dbEntry struct {
	db      *state.Database
	running bool
	connStr string
}

type listRow struct {
	isGroup      bool
	group        string
	count        int
	runningCount int
	entry        *dbEntry
}

// Messages
type msgDatabasesLoaded struct {
	entries   []dbEntry
	dockerErr error
}

type msgDetailLoaded struct {
	name        string
	tab         int
	logs        string
	tables      []string
	tableSchema *tableSchemaGraph
	err         error
}

type msgTableDetailLoaded struct {
	dbName string
	detail *tableDetail
	err    error
}

type msgQueryResult struct {
	dbName string
	result *sqlQueryResult
	err    error
}

type msgActionDone struct {
	toast string
	err   error
}

type msgTick time.Time

type msgClearToast struct {
	seq int
}

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

	entries         []dbEntry
	filtered        []dbEntry
	rows            []listRow
	collapsedGroups map[string]bool
	cursor          int
	leftOffset      int
	focusPane       int // 0=Left, 1=Right
	dockerErr       error

	activeTab    int // 0=Info, 1=Logs, 2=Tables, 3=Query
	showPassword bool

	// Logs tab
	detailLogs      string
	detailErr       error
	logsVP          viewport.Model
	logFollow       bool
	logSearching    bool
	logSearchInput  textinput.Model
	logSearchQuery  string
	logMatchLines   []int
	logMatchCurrent int

	// Tables tab
	detailTables     []string
	tableSchema      *tableSchemaGraph
	tableCursor      int
	selectedTable    string
	tableDetail      *tableDetail
	tableOverviewVP  viewport.Model
	tableDetailVP    viewport.Model
	tableViewDiagram bool
	tableShowFields  bool
	erdXOffset       int
	erdYOffset       int
	erdZoom          int // 0=50% (out), 1=100% (normal), 2=150% (in)
	erdLayout        *erdLayoutResult

	// Query tab
	queryInput  textinput.Model
	queryVP     viewport.Model
	queryResult *sqlQueryResult
	queryErr    error

	// Filter
	filtering   bool
	filterInput textinput.Model

	// Modals & confirmations
	confirmMode  string // "", "remove", "reset"
	modal        modalKind
	backupInput  textinput.Model
	restoreState restoreModalState
	createWizard *createModel

	// Status & feedback
	spinner    spinner.Model
	busy       string
	toast      string
	toastIsErr bool
	toastSeq   int
}

func newDashboardModel() dashboardModel {
	fi := textinput.New()
	fi.Placeholder = "filter by name, provider, or group..."
	fi.CharLimit = 64
	fi.Width = 28

	lsi := textinput.New()
	lsi.Placeholder = "search logs..."
	lsi.CharLimit = 64
	lsi.Width = 28

	qi := textinput.New()
	qi.Placeholder = "SELECT * FROM users LIMIT 10;"
	qi.CharLimit = 512
	qi.Width = 48

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorWarning)

	return dashboardModel{
		width:            100,
		height:           30,
		collapsedGroups:  make(map[string]bool),
		filterInput:      fi,
		logSearchInput:   lsi,
		queryInput:       qi,
		logsVP:           viewport.New(50, 18),
		tableOverviewVP:  viewport.New(50, 18),
		tableDetailVP:    viewport.New(50, 18),
		tableViewDiagram: true,
		tableShowFields:  true,
		erdZoom:          1,
		queryVP:          viewport.New(50, 14),
		logFollow:        true,
		spinner:          sp,
		busy:             "Loading databases...",
	}
}

func (m dashboardModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		loadDatabasesCmd(),
		tickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(dashboardRefreshInterval, func(t time.Time) tea.Msg {
		return msgTick(t)
	})
}

func (m *dashboardModel) setToast(msg string, isErr bool) tea.Cmd {
	m.toast = msg
	m.toastIsErr = isErr
	m.toastSeq++
	seq := m.toastSeq
	return tea.Tick(toastDuration, func(time.Time) tea.Msg {
		return msgClearToast{seq: seq}
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
	if entry == nil || (tab != 1 && tab != 2) {
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
			logs, err := dockerMgr.TailLogs(context.Background(), db.ContainerID, db.ContainerName, 200)
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
			if err != nil {
				return msgDetailLoaded{name: db.Name, tab: tab, tables: tables, err: err}
			}
			schemaGraph := fetchTableSchemaGraph(context.Background(), conn, db.Provider, db.DatabaseName, tables)
			return msgDetailLoaded{name: db.Name, tab: tab, tables: tables, tableSchema: schemaGraph, err: nil}
		}
		return nil
	}
}

func loadTableDetailCmd(db *state.Database, tableName string) tea.Cmd {
	return func() tea.Msg {
		detail, err := fetchTableDetail(context.Background(), db, tableName)
		return msgTableDetailLoaded{dbName: db.Name, detail: detail, err: err}
	}
}

func runQuickSQLCmd(db *state.Database, query string) tea.Cmd {
	return func() tea.Msg {
		res, err := executeQuickSQL(context.Background(), db, query)
		return msgQueryResult{dbName: db.Name, result: res, err: err}
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

func toggleGroupCmd(group string, entries []dbEntry) tea.Cmd {
	return func() tea.Msg {
		dockerMgr, err := container.NewDockerManager(runtimeMode)
		if err != nil {
			return msgActionDone{err: err}
		}
		ctx := context.Background()
		anyRunning := false
		for _, e := range entries {
			if e.running {
				anyRunning = true
				break
			}
		}
		changed := 0
		for _, e := range entries {
			if anyRunning && e.running {
				if err := dockerMgr.StopContainer(ctx, e.db.ContainerID, e.db.ContainerName); err != nil {
					return msgActionDone{err: fmt.Errorf("stopping %s: %w", e.db.Name, err)}
				}
				changed++
			} else if !anyRunning && !e.running {
				if err := dockerMgr.StartContainer(ctx, e.db.ContainerID, e.db.ContainerName); err != nil {
					return msgActionDone{err: fmt.Errorf("starting %s: %w", e.db.Name, err)}
				}
				changed++
			}
		}
		if anyRunning {
			return msgActionDone{toast: fmt.Sprintf("Stopped %d database(s) in '%s'", changed, group)}
		}
		return msgActionDone{toast: fmt.Sprintf("Started %d database(s) in '%s'", changed, group)}
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

func backupDBCmd(name, outPath string) tea.Cmd {
	return func() tea.Msg {
		absPath, sizeBytes, _, err := backupDatabase(context.Background(), name, outPath)
		if err != nil {
			return msgActionDone{err: err}
		}
		mb := float64(sizeBytes) / 1024 / 1024
		return msgActionDone{toast: fmt.Sprintf("Backup saved: %s (%.2f MB)", absPath, mb)}
	}
}

func restoreDBCmd(name, filePath string) tea.Cmd {
	return func() tea.Msg {
		absPath, _, _, err := restoreDatabase(context.Background(), name, filePath, io.Discard)
		if err != nil {
			return msgActionDone{err: err}
		}
		return msgActionDone{toast: fmt.Sprintf("Restored '%s' from %s", name, absPath)}
	}
}

func createDBCmd(opts createOptions) tea.Cmd {
	return func() tea.Msg {
		stateMgr, err := state.NewManager()
		if err != nil {
			return msgActionDone{err: err}
		}
		if _, _, err := provisionDatabase(context.Background(), stateMgr, opts, nil); err != nil {
			return msgActionDone{err: err}
		}
		return msgActionDone{toast: fmt.Sprintf("Created database '%s'", opts.Name)}
	}
}

func (m *dashboardModel) applyFilter() {
	// Remember the current selection so the cursor follows the same row
	// across filter changes and periodic reloads.
	prevGroup, prevDB := "", ""
	if row := m.selectedRow(); row != nil {
		if row.isGroup {
			prevGroup = row.group
		} else if row.entry != nil {
			prevDB = row.entry.db.Name
		}
	}

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
	m.rebuildRows()

	if prevDB != "" && m.selectDBByName(prevDB) {
		return
	}
	if prevGroup != "" && m.selectGroupByName(prevGroup) {
		return
	}
	// The selection vanished (filtered out) or nothing was selected yet:
	// land on the first database row.
	for i, r := range m.rows {
		if !r.isGroup && r.entry != nil {
			m.cursor = i
			m.ensureLeftVisible()
			return
		}
	}
}

func (m *dashboardModel) rebuildRows() {
	grouped := make(map[string][]dbEntry)
	for _, e := range m.filtered {
		g := groupLabel(e.db.Group)
		grouped[g] = append(grouped[g], e)
	}

	var groups []string
	hasGlobal := false
	for g := range grouped {
		if g == "global" {
			hasGlobal = true
			continue
		}
		groups = append(groups, g)
	}
	sort.Strings(groups)
	if hasGlobal {
		groups = append(groups, "global")
	}

	m.rows = nil
	for _, g := range groups {
		items := grouped[g]
		sort.Slice(items, func(i, j int) bool {
			return items[i].db.Name < items[j].db.Name
		})
		runCount := 0
		for _, it := range items {
			if it.running {
				runCount++
			}
		}
		m.rows = append(m.rows, listRow{
			isGroup:      true,
			group:        g,
			count:        len(items),
			runningCount: runCount,
		})
		if !m.collapsedGroups[g] {
			for i := range items {
				entryCopy := items[i]
				m.rows = append(m.rows, listRow{
					group: g,
					entry: &entryCopy,
				})
			}
		}
	}

	if len(m.rows) == 0 {
		m.cursor = 0
		m.leftOffset = 0
		return
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	// If cursor is on a group header and there is a db item right below it on initial load,
	// keep it usable; if user navigated onto a group header intentionally, allow it.
	m.ensureLeftVisible()
}

func (m *dashboardModel) ensureLeftVisible() {
	vis := m.leftVisibleLines()
	if vis <= 0 {
		return
	}
	if m.cursor < m.leftOffset {
		m.leftOffset = m.cursor
	} else if m.cursor >= m.leftOffset+vis {
		m.leftOffset = m.cursor - vis + 1
	}
	if m.leftOffset < 0 {
		m.leftOffset = 0
	}
}

func (m dashboardModel) layoutDimensions() (stacked bool, leftW, leftH, rightW, rightH int) {
	bodyH := max(10, m.height-5)
	if m.width < 80 {
		stacked = true
		w := max(30, m.width-2)
		leftH = max(5, bodyH*32/100)
		rightH = max(8, bodyH-leftH-2)
		return true, w, leftH, w, rightH
	}
	leftW = min(38, max(26, m.width*26/100))
	rightW = max(36, m.width-leftW-4)
	return false, leftW, bodyH, rightW, bodyH
}

func (m dashboardModel) leftVisibleLines() int {
	_, _, leftH, _, _ := m.layoutDimensions()
	overhead := 3 // header + blank + border
	if m.filtering || strings.TrimSpace(m.filterInput.Value()) != "" {
		overhead++
	}
	return max(3, leftH-overhead)
}

func (m *dashboardModel) syncViewportSizes() {
	_, _, _, rightW, rightH := m.layoutDimensions()
	innerW := max(20, rightW-4)
	innerH := max(5, rightH-5)

	m.logsVP.Width = innerW
	m.logsVP.Height = max(4, innerH-1)
	m.tableOverviewVP.Width = innerW
	m.tableOverviewVP.Height = max(4, innerH-2)
	m.tableDetailVP.Width = innerW
	m.tableDetailVP.Height = max(4, innerH-2)
	m.queryInput.Width = max(20, innerW-6)
	m.queryVP.Width = innerW
	m.queryVP.Height = max(4, innerH-5)
}

func (m *dashboardModel) refreshLogsViewport() {
	if m.detailErr != nil {
		m.logsVP.SetContent(styleErrorText.Render("Error loading logs: " + m.detailErr.Error()))
		return
	}
	if strings.TrimSpace(m.detailLogs) == "" {
		m.logsVP.SetContent(styleMutedText.Render("(no logs available)"))
		return
	}
	rawLines := strings.Split(strings.TrimRight(m.detailLogs, "\n"), "\n")
	innerW := max(20, m.logsVP.Width)
	q := strings.ToLower(strings.TrimSpace(m.logSearchQuery))
	m.logMatchLines = nil

	var wrapped []string
	for _, line := range rawLines {
		chunks := wrapLine(line, innerW)
		for _, c := range chunks {
			if q != "" && strings.Contains(strings.ToLower(c), q) {
				m.logMatchLines = append(m.logMatchLines, len(wrapped))
				c = highlightSearch(c, q)
			}
			wrapped = append(wrapped, c)
		}
	}
	m.logsVP.SetContent(strings.Join(wrapped, "\n"))
	if m.logFollow && !m.logSearching && q == "" {
		m.logsVP.GotoBottom()
	}
}

func wrapLine(s string, width int) []string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return []string{s}
	}
	var out []string
	rest := s
	for lipgloss.Width(rest) > width {
		part := ansi.Truncate(rest, width, "")
		if len(part) == 0 {
			break
		}
		out = append(out, part)
		rest = rest[len(part):]
	}
	if rest != "" {
		out = append(out, rest)
	}
	return out
}

func highlightSearch(line, q string) string {
	lower := strings.ToLower(line)
	idx := strings.Index(lower, q)
	if idx < 0 {
		return line
	}
	return line[:idx] + styleSearchMatch.Render(line[idx:idx+len(q)]) + line[idx+len(q):]
}

func (m dashboardModel) selected() *dbEntry {
	if len(m.rows) == 0 || m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return m.rows[m.cursor].entry
}

func (m dashboardModel) selectedRow() *listRow {
	if len(m.rows) == 0 || m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return &m.rows[m.cursor]
}

func (m dashboardModel) groupEntries(group string) []dbEntry {
	var out []dbEntry
	for _, e := range m.filtered {
		if groupLabel(e.db.Group) == group {
			out = append(out, e)
		}
	}
	return out
}

func (m *dashboardModel) selectDBByName(name string) bool {
	for i, r := range m.rows {
		if !r.isGroup && r.entry != nil && r.entry.db.Name == name {
			m.cursor = i
			m.ensureLeftVisible()
			return true
		}
	}
	return false
}

func (m *dashboardModel) selectGroupByName(group string) bool {
	for i, r := range m.rows {
		if r.isGroup && r.group == group {
			m.cursor = i
			m.ensureLeftVisible()
			return true
		}
	}
	return false
}

func (m *dashboardModel) resetRightSubstate() {
	m.selectedTable = ""
	m.tableDetail = nil
	m.tableSchema = nil
	m.tableCursor = 0
	m.erdXOffset = 0
	m.erdYOffset = 0
	m.erdLayout = nil
	m.detailErr = nil
	m.queryResult = nil
	m.queryErr = nil
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.syncViewportSizes()
		m.ensureLeftVisible()
		if m.activeTab == 1 {
			m.refreshLogsViewport()
		}
		if m.activeTab == 2 {
			if m.selectedTable != "" {
				m.renderTableDetailToVP()
			} else {
				m.renderTableOverviewToVP()
			}
		}
		return m, nil

	case spinner.TickMsg:
		if m.busy != "" {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case msgClearToast:
		if msg.seq == m.toastSeq {
			m.toast = ""
		}
		return m, nil

	case msgTick:
		return m, tea.Batch(loadDatabasesCmd(), tickCmd())

	case msgDatabasesLoaded:
		m.busy = ""
		m.dockerErr = msg.dockerErr
		m.entries = msg.entries
		m.applyFilter()
		return m, loadDetailCmd(m.selected(), m.activeTab)

	case msgDetailLoaded:
		if sel := m.selected(); sel != nil && sel.db.Name == msg.name && m.activeTab == msg.tab {
			m.detailLogs = msg.logs
			m.detailTables = msg.tables
			m.tableSchema = msg.tableSchema
			m.detailErr = msg.err
			if msg.tab == 1 {
				m.refreshLogsViewport()
			}
			if msg.tab == 2 {
				if m.tableCursor >= len(m.detailTables) {
					m.tableCursor = max(0, len(m.detailTables)-1)
				}
				m.renderTableOverviewToVP()
			}
		}
		return m, nil

	case msgTableDetailLoaded:
		m.busy = ""
		if sel := m.selected(); sel != nil && sel.db.Name == msg.dbName {
			if msg.err != nil {
				return m, m.setToast(msg.err.Error(), true)
			}
			m.tableDetail = msg.detail
			m.renderTableDetailToVP()
		}
		return m, nil

	case msgQueryResult:
		m.busy = ""
		if sel := m.selected(); sel != nil && sel.db.Name == msg.dbName {
			m.queryErr = msg.err
			m.queryResult = msg.result
			m.renderQueryResultToVP()
		}
		return m, nil

	case msgActionDone:
		m.busy = ""
		var toastCmd tea.Cmd
		if msg.err != nil {
			toastCmd = m.setToast(msg.err.Error(), true)
		} else if msg.toast != "" {
			toastCmd = m.setToast(msg.toast, false)
		}
		return m, tea.Batch(toastCmd, loadDatabasesCmd())

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		// Modal takes priority
		if m.modal != modalNone {
			return m.updateModalKey(msg)
		}

		// Confirmation prompt
		if m.confirmMode != "" {
			sel := m.selected()
			mode := m.confirmMode
			m.confirmMode = ""
			if strings.ToLower(msg.String()) == "y" && sel != nil {
				if mode == "remove" {
					m.busy = fmt.Sprintf("Removing '%s'...", sel.db.Name)
					return m, tea.Batch(m.spinner.Tick, removeDBCmd(*sel))
				}
				if mode == "reset" {
					m.busy = fmt.Sprintf("Resetting '%s'...", sel.db.Name)
					return m, tea.Batch(m.spinner.Tick, resetDBCmd(*sel))
				}
			}
			return m, m.setToast("Cancelled", false)
		}

		// Left-pane filter input
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

		// Right-pane log search input
		if m.logSearching {
			switch msg.String() {
			case "esc":
				m.logSearching = false
				m.logSearchInput.SetValue("")
				m.logSearchQuery = ""
				m.logSearchInput.Blur()
				m.refreshLogsViewport()
				return m, nil
			case "enter":
				m.logSearching = false
				m.logSearchInput.Blur()
				m.logSearchQuery = m.logSearchInput.Value()
				m.logMatchCurrent = 0
				m.refreshLogsViewport()
				if len(m.logMatchLines) > 0 {
					m.logsVP.SetYOffset(max(0, m.logMatchLines[0]-2))
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.logSearchInput, cmd = m.logSearchInput.Update(msg)
			m.logSearchQuery = m.logSearchInput.Value()
			m.refreshLogsViewport()
			return m, cmd
		}

		// Right-pane Query input focused
		if m.focusPane == 1 && m.activeTab == 3 && m.queryInput.Focused() {
			switch msg.String() {
			case "esc":
				m.queryInput.Blur()
				return m, nil
			case "enter":
				sel := m.selected()
				q := strings.TrimSpace(m.queryInput.Value())
				if sel == nil || !sel.running {
					return m, m.setToast("Database must be running to execute SQL", true)
				}
				if q == "" {
					return m, nil
				}
				m.busy = "Running query..."
				return m, tea.Batch(m.spinner.Tick, runQuickSQLCmd(sel.db, q))
			}
			var cmd tea.Cmd
			m.queryInput, cmd = m.queryInput.Update(msg)
			return m, cmd
		}

		return m.handleNormalKey(msg)
	}

	return m, nil
}

func (m dashboardModel) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "?":
		m.modal = modalHelp
		return m, nil

	case "left", "h":
		m.focusPane = 0
		if m.activeTab == 2 && m.selectedTable == "" {
			m.renderTableOverviewToVP()
		}
		return m, nil

	case "right", "l":
		m.focusPane = 1
		if m.activeTab == 2 && m.selectedTable == "" {
			m.renderTableOverviewToVP()
		}
		return m, nil

	case "tab", "]":
		if m.focusPane == 1 {
			m.activeTab = (m.activeTab + 1) % tabCount
			return m, loadDetailCmd(m.selected(), m.activeTab)
		}

	case "[":
		if m.focusPane == 1 {
			m.activeTab = (m.activeTab + tabCount - 1) % tabCount
			return m, loadDetailCmd(m.selected(), m.activeTab)
		}

	case "1", "2", "3", "4":
		if m.focusPane == 1 {
			m.activeTab = int(msg.String()[0] - '1')
			return m, loadDetailCmd(m.selected(), m.activeTab)
		}

	case "/":
		if m.focusPane == 1 {
			if m.activeTab == 1 {
				m.logSearching = true
				m.logSearchInput.Focus()
				return m, textinput.Blink
			}
			return m, nil
		}
		m.filtering = true
		m.filterInput.Focus()
		return m, textinput.Blink

	case "esc":
		if m.focusPane == 1 && m.activeTab == 2 && m.selectedTable != "" {
			m.selectedTable = ""
			m.tableDetail = nil
			m.renderTableOverviewToVP()
			return m, nil
		}
		if m.focusPane == 1 && m.activeTab == 1 && m.logSearchQuery != "" {
			m.logSearchQuery = ""
			m.logSearchInput.SetValue("")
			m.refreshLogsViewport()
			return m, nil
		}
		if m.focusPane == 1 {
			m.focusPane = 0
			if m.activeTab == 2 && m.selectedTable == "" {
				m.renderTableOverviewToVP()
			}
			return m, nil
		}
		if strings.TrimSpace(m.filterInput.Value()) != "" {
			m.filterInput.SetValue("")
			m.applyFilter()
			return m, loadDetailCmd(m.selected(), m.activeTab)
		}

	case "up", "k":
		if m.focusPane == 0 {
			if m.cursor > 0 {
				m.cursor--
				m.ensureLeftVisible()
				m.resetRightSubstate()
				return m, loadDetailCmd(m.selected(), m.activeTab)
			}
		} else {
			return m.scrollRight(-1)
		}

	case "down", "j":
		if m.focusPane == 0 {
			if m.cursor < len(m.rows)-1 {
				m.cursor++
				m.ensureLeftVisible()
				m.resetRightSubstate()
				return m, loadDetailCmd(m.selected(), m.activeTab)
			}
		} else {
			return m.scrollRight(1)
		}

	case "pgup", "ctrl+u":
		return m.scrollRight(-8)

	case "pgdown", "ctrl+d":
		return m.scrollRight(8)

	case "shift+left":
		if m.focusPane == 1 && m.activeTab == 2 && m.selectedTable == "" && m.tableViewDiagram {
			m.panERD(-8, 0)
			return m, nil
		}

	case "shift+right":
		if m.focusPane == 1 && m.activeTab == 2 && m.selectedTable == "" && m.tableViewDiagram {
			m.panERD(8, 0)
			return m, nil
		}

	case "shift+up":
		if m.focusPane == 1 && m.activeTab == 2 && m.selectedTable == "" && m.tableViewDiagram {
			m.panERD(0, -3)
			return m, nil
		}

	case "shift+down":
		if m.focusPane == 1 && m.activeTab == 2 && m.selectedTable == "" && m.tableViewDiagram {
			m.panERD(0, 3)
			return m, nil
		}

	case "+", "=":
		if m.focusPane == 1 && m.activeTab == 2 && m.selectedTable == "" && m.tableViewDiagram {
			if m.erdZoom < 2 {
				m.erdZoom++
				m.renderTableOverviewToVP()
			}
			return m, nil
		}

	case "-":
		if m.focusPane == 1 && m.activeTab == 2 && m.selectedTable == "" && m.tableViewDiagram {
			if m.erdZoom > 0 {
				m.erdZoom--
				m.renderTableOverviewToVP()
			}
			return m, nil
		}

	case "0":
		if m.focusPane == 1 && m.activeTab == 2 && m.selectedTable == "" && m.tableViewDiagram {
			m.erdZoom = 1
			m.erdXOffset = 0
			m.erdYOffset = 0
			m.renderTableOverviewToVP()
			return m, nil
		}

	case "enter", " ":
		if m.focusPane == 0 {
			if row := m.selectedRow(); row != nil && row.isGroup {
				m.collapsedGroups[row.group] = !m.collapsedGroups[row.group]
				m.rebuildRows()
				return m, nil
			}
			if m.selected() != nil && msg.String() == "enter" {
				m.focusPane = 1
				if m.activeTab == 2 && m.selectedTable == "" {
					m.renderTableOverviewToVP()
				}
				return m, nil
			}
		} else if m.activeTab == 2 {
			if m.selectedTable == "" && len(m.detailTables) > 0 && m.tableCursor < len(m.detailTables) {
				sel := m.selected()
				if sel != nil && sel.running {
					tName := m.detailTables[m.tableCursor]
					m.selectedTable = tName
					m.busy = fmt.Sprintf("Inspecting '%s'...", tName)
					return m, tea.Batch(m.spinner.Tick, loadTableDetailCmd(sel.db, tName))
				}
			}
		} else if m.activeTab == 3 && msg.String() == "enter" {
			m.queryInput.Focus()
			return m, textinput.Blink
		}

	case "i":
		if m.focusPane == 1 && m.activeTab == 3 {
			m.queryInput.Focus()
			return m, textinput.Blink
		}

	case "v":
		if m.focusPane == 1 && m.activeTab == 2 {
			m.tableViewDiagram = !m.tableViewDiagram
			if m.selectedTable != "" {
				m.renderTableDetailToVP()
			} else {
				m.renderTableOverviewToVP()
			}
			modeLabel := "list/table"
			if m.tableViewDiagram {
				modeLabel = "diagram"
			}
			return m, m.setToast("Tables view: "+modeLabel, false)
		}

	case "f":
		if m.focusPane == 1 && m.activeTab == 1 {
			m.logFollow = !m.logFollow
			if m.logFollow {
				m.logsVP.GotoBottom()
			}
			stateLabel := "off"
			if m.logFollow {
				stateLabel = "on"
			}
			return m, m.setToast("Log auto-follow: "+stateLabel, false)
		}
		if m.focusPane == 1 && m.activeTab == 2 {
			m.tableShowFields = !m.tableShowFields
			if m.selectedTable != "" {
				m.renderTableDetailToVP()
			} else {
				m.renderTableOverviewToVP()
			}
			stateLabel := "hidden"
			if m.tableShowFields {
				stateLabel = "shown"
			}
			return m, m.setToast("Table fields: "+stateLabel, false)
		}

	case "N":
		if m.focusPane == 1 && m.activeTab == 1 && len(m.logMatchLines) > 0 {
			m.logMatchCurrent = (m.logMatchCurrent + len(m.logMatchLines) - 1) % len(m.logMatchLines)
			m.logsVP.SetYOffset(max(0, m.logMatchLines[m.logMatchCurrent]-2))
			return m, nil
		}

	case "n":
		if m.focusPane == 1 {
			if m.activeTab == 1 && m.logSearchQuery != "" && len(m.logMatchLines) > 0 {
				m.logMatchCurrent = (m.logMatchCurrent + 1) % len(m.logMatchLines)
				m.logsVP.SetYOffset(max(0, m.logMatchLines[m.logMatchCurrent]-2))
			}
			return m, nil
		}
		stateMgr, err := state.NewManager()
		if err != nil {
			return m, m.setToast(err.Error(), true)
		}
		cm := newCreateModel(stateMgr)
		m.createWizard = &cm
		m.modal = modalCreate
		return m, textinput.Blink

	case "p":
		if m.focusPane == 1 && m.activeTab == 0 {
			m.showPassword = !m.showPassword
		}

	case "r":
		m.busy = "Refreshing..."
		return m, tea.Batch(m.spinner.Tick, loadDatabasesCmd())

	case "s":
		if m.focusPane != 0 || m.busy != "" {
			return m, nil
		}
		if row := m.selectedRow(); row != nil && row.isGroup {
			entries := m.groupEntries(row.group)
			if len(entries) > 0 {
				m.busy = fmt.Sprintf("Toggling group '%s'...", row.group)
				return m, tea.Batch(m.spinner.Tick, toggleGroupCmd(row.group, entries))
			}
		}
		if sel := m.selected(); sel != nil {
			if sel.running {
				m.busy = fmt.Sprintf("Stopping '%s'...", sel.db.Name)
			} else {
				m.busy = fmt.Sprintf("Starting '%s'...", sel.db.Name)
			}
			return m, tea.Batch(m.spinner.Tick, toggleDBCmd(*sel))
		}

	case "c":
		if m.focusPane != 1 || m.activeTab != 0 {
			return m, nil
		}
		if sel := m.selected(); sel != nil && sel.connStr != "" {
			if err := clipboard.WriteAll(sel.connStr); err != nil {
				return m, m.setToast(fmt.Sprintf("Clipboard error: %v", err), true)
			}
			return m, m.setToast(fmt.Sprintf("Copied connection string for '%s'", sel.db.Name), false)
		}

	case "x":
		if m.focusPane != 0 {
			return m, nil
		}
		if sel := m.selected(); sel != nil {
			if !sel.running {
				return m, m.setToast(fmt.Sprintf("'%s' is stopped — press 's' to start it first", sel.db.Name), true)
			}
			return m, tea.Exec(shellExecCmd{name: sel.db.Name}, func(err error) tea.Msg {
				return loadDatabasesCmd()()
			})
		}

	case "b":
		if m.focusPane != 0 {
			return m, nil
		}
		if sel := m.selected(); sel != nil && m.busy == "" {
			if !sel.running {
				return m, m.setToast(fmt.Sprintf("'%s' is stopped — start it before backing up", sel.db.Name), true)
			}
			m.backupInput = newBackupInput(sel.db.Name)
			m.modal = modalBackup
			return m, textinput.Blink
		}

	case "L":
		if m.focusPane != 0 {
			return m, nil
		}
		if sel := m.selected(); sel != nil && m.busy == "" {
			if !sel.running {
				return m, m.setToast(fmt.Sprintf("'%s' is stopped — start it before restoring", sel.db.Name), true)
			}
			m.restoreState = newRestoreModalState()
			m.modal = modalRestore
			if m.restoreState.customMode {
				m.restoreState.input.Focus()
				return m, textinput.Blink
			}
			return m, nil
		}

	case "d":
		if m.focusPane == 0 && m.selected() != nil && m.busy == "" {
			m.confirmMode = "remove"
		}

	case "R":
		if m.focusPane == 0 && m.selected() != nil && m.busy == "" {
			m.confirmMode = "reset"
		}
	}

	return m, nil
}

func (m dashboardModel) scrollRight(delta int) (tea.Model, tea.Cmd) {
	switch m.activeTab {
	case 1:
		if delta < 0 {
			m.logsVP.LineUp(-delta)
			m.logFollow = false
		} else {
			m.logsVP.LineDown(delta)
		}
	case 2:
		if m.selectedTable != "" {
			if delta < 0 {
				m.tableDetailVP.LineUp(-delta)
			} else {
				m.tableDetailVP.LineDown(delta)
			}
		} else if len(m.detailTables) > 0 {
			m.tableCursor = max(0, min(len(m.detailTables)-1, m.tableCursor+delta))
			m.renderTableOverviewToVP()
		}
	case 3:
		if delta < 0 {
			m.queryVP.LineUp(-delta)
		} else {
			m.queryVP.LineDown(delta)
		}
	}
	return m, nil
}

func (m dashboardModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.modal != modalNone {
		return m, nil
	}
	stacked, leftW, leftH, _, _ := m.layoutDimensions()

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if (!stacked && msg.X < leftW+2) || (stacked && msg.Y < leftH+2) {
			if m.cursor > 0 {
				m.cursor--
				m.ensureLeftVisible()
				m.resetRightSubstate()
				return m, loadDetailCmd(m.selected(), m.activeTab)
			}
			return m, nil
		}
		return m.scrollRight(-3)

	case tea.MouseButtonWheelDown:
		if (!stacked && msg.X < leftW+2) || (stacked && msg.Y < leftH+2) {
			if m.cursor < len(m.rows)-1 {
				m.cursor++
				m.ensureLeftVisible()
				m.resetRightSubstate()
				return m, loadDetailCmd(m.selected(), m.activeTab)
			}
			return m, nil
		}
		return m.scrollRight(3)

	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		// Click in left pane
		inLeft := (!stacked && msg.X < leftW+2) || (stacked && msg.Y >= 1 && msg.Y < leftH+2)
		if inLeft {
			m.focusPane = 0
			headerLines := 3 // top header + "Databases" + blank
			if m.filtering || strings.TrimSpace(m.filterInput.Value()) != "" {
				headerLines++
			}
			idx := m.leftOffset + (msg.Y - headerLines)
			if idx >= 0 && idx < len(m.rows) {
				if idx == m.cursor && m.rows[idx].isGroup {
					m.collapsedGroups[m.rows[idx].group] = !m.collapsedGroups[m.rows[idx].group]
					m.rebuildRows()
					return m, nil
				}
				m.cursor = idx
				m.ensureLeftVisible()
				m.resetRightSubstate()
				return m, loadDetailCmd(m.selected(), m.activeTab)
			}
			return m, nil
		}
		// Click in right pane
		m.focusPane = 1
		tabY := 2
		if stacked {
			tabY = leftH + 3
		}
		if msg.Y == tabY {
			relX := msg.X
			if !stacked {
				relX -= leftW + 4
			}
			if relX >= 0 {
				tabIdx := relX / 13
				if tabIdx >= 0 && tabIdx < tabCount {
					m.activeTab = tabIdx
					return m, loadDetailCmd(m.selected(), m.activeTab)
				}
			}
		}
	}
	return m, nil
}

func (m *dashboardModel) panERD(dx, dy int) {
	if m.erdLayout == nil || m.erdLayout.canvas == nil {
		return
	}
	viewW := max(24, m.tableOverviewVP.Width)
	viewH := max(4, m.tableOverviewVP.Height)
	maxX := max(0, m.erdLayout.canvas.w-viewW)
	maxY := max(0, m.erdLayout.canvas.h-viewH)
	m.erdXOffset = max(0, min(maxX, m.erdXOffset+dx))
	m.erdYOffset = max(0, min(maxY, m.erdYOffset+dy))
}

func (m *dashboardModel) renderTableOverviewToVP() {
	if len(m.detailTables) == 0 {
		m.erdLayout = nil
		m.tableOverviewVP.SetContent(styleMutedText.Render("(no tables found)"))
		return
	}
	w := max(24, m.tableOverviewVP.Width)
	h := max(4, m.tableOverviewVP.Height)
	focused := m.focusPane == 1
	if m.tableViewDiagram {
		m.erdLayout = buildTableOverviewERDWithZoom(m.detailTables, m.tableSchema, m.tableCursor, focused, m.tableShowFields, m.erdZoom)
		if m.erdLayout != nil && m.erdLayout.canvas != nil {
			cr := m.erdLayout.cursorRect
			if cr.w > 0 && cr.h > 0 {
				if cr.x < m.erdXOffset {
					m.erdXOffset = max(0, cr.x-2)
				} else if cr.x+cr.w > m.erdXOffset+w {
					m.erdXOffset = max(0, cr.x+cr.w-w+2)
				}
				if cr.y < m.erdYOffset {
					m.erdYOffset = max(0, cr.y-1)
				} else if cr.y+cr.h > m.erdYOffset+h {
					m.erdYOffset = max(0, cr.y+cr.h-h+1)
				}
			}
			m.panERD(0, 0)
		}
		return
	}

	m.erdLayout = nil
	content, cursorLine := formatTableOverviewList(m.detailTables, m.tableSchema, m.tableCursor, focused, m.tableShowFields, w)
	m.tableOverviewVP.SetContent(content)
	// Keep selected table/card visible within viewport.
	if m.tableOverviewVP.Height > 0 {
		if cursorLine < m.tableOverviewVP.YOffset {
			m.tableOverviewVP.SetYOffset(cursorLine)
		} else if cursorLine >= m.tableOverviewVP.YOffset+m.tableOverviewVP.Height {
			m.tableOverviewVP.SetYOffset(max(0, cursorLine-m.tableOverviewVP.Height+3))
		}
	}
}

func (m *dashboardModel) renderTableDetailToVP() {
	if m.tableDetail == nil {
		return
	}
	w := max(24, m.tableDetailVP.Width)
	var b strings.Builder

	if m.tableViewDiagram {
		b.WriteString(formatTableDetailDiagram(m.tableDetail, m.tableSchema, m.tableShowFields, w))
	} else {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render("Schema: "+m.tableDetail.tableName) + "\n")
		if m.tableShowFields {
			b.WriteString(formatCompactTable(m.tableDetail.schemaCols, m.tableDetail.schemaRows, w))
		} else {
			b.WriteString(styleMutedText.Render(fmt.Sprintf("(%d fields hidden — press 'f' to show)", len(m.tableDetail.schemaRows))) + "\n")
		}
	}

	b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render("Preview (up to 25 rows)") + "\n")
	if len(m.tableDetail.previewCols) == 0 {
		b.WriteString(styleMutedText.Render("(unable to preview rows or empty)") + "\n")
	} else {
		b.WriteString(formatCompactTable(m.tableDetail.previewCols, m.tableDetail.previewRows, w))
	}
	m.tableDetailVP.SetContent(b.String())
	m.tableDetailVP.GotoTop()
}

func (m *dashboardModel) renderQueryResultToVP() {
	if m.queryErr != nil {
		m.queryVP.SetContent(styleErrorText.Render("SQL Error:\n" + m.queryErr.Error()))
		m.queryVP.GotoTop()
		return
	}
	if m.queryResult == nil {
		m.queryVP.SetContent(styleMutedText.Render("Press 'i' or Enter to type a SQL query."))
		return
	}
	if !m.queryResult.isSelect {
		m.queryVP.SetContent(styleToastOK.Render(fmt.Sprintf("✓ Query executed (%d row(s) affected)", m.queryResult.affected)))
		m.queryVP.GotoTop()
		return
	}
	w := max(24, m.queryVP.Width)
	header := styleMutedText.Render(fmt.Sprintf("%d row(s) returned (max 200 shown)", len(m.queryResult.rows)))
	m.queryVP.SetContent(header + "\n\n" + formatCompactTable(m.queryResult.cols, m.queryResult.rows, w))
	m.queryVP.GotoTop()
}

func (m dashboardModel) View() string {
	dockerBadge := styleBadgeRun.Render("● runtime ok")
	if m.dockerErr != nil {
		dockerBadge = styleErrorText.Render("✗ runtime unavailable")
	}
	header := fmt.Sprintf("%s   %s   %s",
		styleHeader.Render(" xm-db "),
		dockerBadge,
		styleMutedText.Render(fmt.Sprintf("%d database(s)", len(m.entries))),
	)

	stacked, leftW, leftH, rightW, rightH := m.layoutDimensions()
	leftPane := m.renderLeftPane(leftW, leftH)
	rightPane := m.renderRightPane(rightW, rightH)

	var body string
	if stacked {
		body = lipgloss.JoinVertical(lipgloss.Left, leftPane, rightPane)
	} else {
		body = lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
	}

	footer := m.renderFooter()
	base := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)

	switch m.modal {
	case modalHelp:
		return renderOverlay(base, renderHelpModal(m.width), m.width, m.height)
	case modalBackup:
		name := ""
		if sel := m.selected(); sel != nil {
			name = sel.db.Name
		}
		return renderOverlay(base, renderBackupModal(name, m.backupInput, m.width), m.width, m.height)
	case modalRestore:
		name := ""
		if sel := m.selected(); sel != nil {
			name = sel.db.Name
		}
		return renderOverlay(base, renderRestoreModal(name, m.restoreState, m.width), m.width, m.height)
	case modalCreate:
		if m.createWizard != nil {
			w := min(74, max(48, m.width-6))
			box := styleModal.Width(w).Render(m.createWizard.ViewContent(w - 6))
			return renderOverlay(base, box, m.width, m.height)
		}
	}
	return base
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

	if len(m.rows) == 0 {
		if len(m.entries) == 0 {
			b.WriteString(styleMutedText.Render("No databases yet.\nPress 'n' to create one!"))
		} else {
			b.WriteString(styleMutedText.Render("No databases match filter."))
		}
	} else {
		innerW := max(16, width-4)
		vis := m.leftVisibleLines()
		end := min(len(m.rows), m.leftOffset+vis)
		for i := m.leftOffset; i < end; i++ {
			r := m.rows[i]
			var line string
			if r.isGroup {
				arrow := "▼"
				if m.collapsedGroups[r.group] {
					arrow = "▶"
				}
				label := fmt.Sprintf("%s %s (%d/%d running)", arrow, r.group, r.runningCount, r.count)
				if lipgloss.Width(label) > innerW {
					label = ansi.Truncate(label, innerW-1, "…")
				}
				if i == m.cursor {
					line = styleSelectedRow.Width(innerW).Render(label)
				} else {
					line = styleGroupHeader.Render(label)
				}
			} else if r.entry != nil {
				e := r.entry
				badge := styleBadgeStop.Render("○")
				if e.running {
					badge = styleBadgeRun.Render("●")
				}
				txt := fmt.Sprintf("  %s %s (%s:%d)", badge, e.db.Name, e.db.Provider, e.db.Port)
				if lipgloss.Width(txt) > innerW {
					txt = ansi.Truncate(txt, innerW-1, "…")
				}
				if i == m.cursor {
					line = styleSelectedRow.Width(innerW).Render(txt)
				} else {
					line = txt
				}
			}
			b.WriteString(line + "\n")
		}
	}

	paneStyle := stylePane
	if m.focusPane == 0 {
		paneStyle = stylePaneFocused
	}
	return paneStyle.Width(width).Height(height).Render(b.String())
}

func (m dashboardModel) renderRightPane(width, height int) string {
	tabs := []string{"[1] Info", "[2] Logs", "[3] Tables", "[4] Query"}
	var tabParts []string
	for i, t := range tabs {
		if i == m.activeTab {
			tabParts = append(tabParts, styleTabActive.Render(t))
		} else {
			tabParts = append(tabParts, styleTabInactive.Render(t))
		}
	}
	tabBar := strings.Join(tabParts, " ")

	paneStyle := stylePane
	if m.focusPane == 1 {
		paneStyle = stylePaneFocused
	}

	if row := m.selectedRow(); row != nil && row.isGroup {
		content := tabBar + "\n\n" +
			styleGroupHeader.Render("Group: "+row.group) + "\n\n" +
			fmt.Sprintf("Databases: %d (%d running)\n\n", row.count, row.runningCount) +
			styleMutedText.Render("Enter/Space: expand/collapse  •  s: start/stop all in group")
		return paneStyle.Width(width).Height(height).Render(content)
	}

	sel := m.selected()
	if sel == nil {
		content := tabBar + "\n\n" + styleMutedText.Render("Select a database from the list.")
		return paneStyle.Width(width).Height(height).Render(content)
	}

	var body string
	switch m.activeTab {
	case 0:
		body = m.renderInfoTab(sel)
	case 1:
		body = m.renderLogsTab()
	case 2:
		body = m.renderTablesTab()
	case 3:
		body = m.renderQueryTab()
	}

	content := tabBar + "\n\n" + body
	return paneStyle.Width(width).Height(height).Render(content)
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

func (m dashboardModel) renderLogsTab() string {
	var b strings.Builder
	followBadge := styleMutedText.Render("follow:off")
	if m.logFollow {
		followBadge = styleBadgeRun.Render("follow:on")
	}
	status := fmt.Sprintf("%s  •  f: follow  •  /: search", followBadge)
	if m.logSearching {
		status = m.logSearchInput.View()
	} else if m.logSearchQuery != "" {
		status = fmt.Sprintf("search: %q (%d matches, n/N: jump, Esc: clear)", m.logSearchQuery, len(m.logMatchLines))
	}
	b.WriteString(styleMutedText.Render(status) + "\n")
	b.WriteString(m.logsVP.View())
	return b.String()
}

func (m dashboardModel) renderTablesTab() string {
	if m.detailErr != nil {
		return styleMutedText.Render(m.detailErr.Error())
	}

	viewBadge := styleMutedText.Render("view:list")
	if m.tableViewDiagram {
		viewBadge = styleBadgeRun.Render("view:diagram")
	}
	fieldsBadge := styleMutedText.Render("fields:off")
	if m.tableShowFields {
		fieldsBadge = styleBadgeRun.Render("fields:on")
	}

	if m.selectedTable != "" {
		var b strings.Builder
		status := fmt.Sprintf("%s  %s  •  Esc: back  •  v: view  •  f: fields  •  ↑/↓: scroll", viewBadge, fieldsBadge)
		b.WriteString(styleMutedText.Render(status) + "\n")
		if m.tableDetail == nil {
			b.WriteString(styleMutedText.Render("Loading table details..."))
		} else {
			b.WriteString(m.tableDetailVP.View())
		}
		return b.String()
	}
	if len(m.detailTables) == 0 {
		return styleMutedText.Render("(no tables found)")
	}
	var b strings.Builder
	if m.tableViewDiagram {
		zoomLabel := "100%"
		switch m.erdZoom {
		case 0:
			zoomLabel = "50%"
		case 2:
			zoomLabel = "150%"
		}
		zoomBadge := styleBadgeRun.Render("zoom:" + zoomLabel)
		status := fmt.Sprintf("%s  %s  %s  •  +/-: zoom  •  Shift+Arrows: pan  •  v: view  •  f: fields", viewBadge, fieldsBadge, zoomBadge)
		b.WriteString(styleMutedText.Render(status) + "\n")
		if m.erdLayout != nil && m.erdLayout.canvas != nil {
			w := max(24, m.tableOverviewVP.Width)
			h := max(4, m.tableOverviewVP.Height)
			b.WriteString(m.erdLayout.canvas.renderWindow(m.erdXOffset, m.erdYOffset, w, h))
		}
	} else {
		status := fmt.Sprintf("%s  %s  •  Enter: inspect  •  v: view  •  f: fields", viewBadge, fieldsBadge)
		b.WriteString(styleMutedText.Render(status) + "\n")
		b.WriteString(m.tableOverviewVP.View())
	}
	return b.String()
}

func (m dashboardModel) renderQueryTab() string {
	var b strings.Builder
	hint := "Press 'i' or Enter to type SQL, Esc to blur"
	if m.queryInput.Focused() {
		hint = "Enter: execute SQL  •  Esc: blur"
	}
	b.WriteString("SQL> " + m.queryInput.View() + "\n")
	b.WriteString(styleMutedText.Render(hint) + "\n\n")
	if m.queryResult == nil && m.queryErr == nil {
		b.WriteString(styleMutedText.Render("Results will appear here."))
	} else {
		b.WriteString(m.queryVP.View())
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
		statusLine = lipgloss.NewStyle().Foreground(colorWarning).Render("  " + m.spinner.View() + " " + m.busy)
	} else if m.toast != "" {
		if m.toastIsErr {
			statusLine = styleToastErr.Render("  ✗ " + m.toast)
		} else {
			statusLine = styleToastOK.Render("  ✓ " + m.toast)
		}
	}

	helpText := m.footerHelpText()
	if m.width > 0 && lipgloss.Width(helpText) > m.width {
		helpText = ansi.Truncate(helpText, max(20, m.width-1), "…")
	}
	help := styleHelpBar.Render(helpText)
	if statusLine != "" {
		return statusLine + "\n" + help
	}
	return help
}

func (m dashboardModel) footerHelpText() string {
	if m.focusPane == 0 {
		return "  [Sidebar] ↑/↓: nav  •  →/Enter: pane  •  /: filter  •  s: start/stop  •  n: new  •  x: shell  •  b/L: backup/restore  •  R/d: reset/del  •  ?: help  •  q: quit"
	}

	switch m.activeTab {
	case 0:
		return "  [Info] c: copy conn  •  p: password  •  ←/Esc: sidebar  •  1-4/Tab: tab  •  r: refresh  •  ?: help  •  q: quit"
	case 1:
		return "  [Logs] ↑/↓: scroll  •  f: follow  •  /: search  •  n/N: match  •  ←/Esc: sidebar  •  1-4/Tab: tab  •  ?: help  •  q: quit"
	case 2:
		if m.selectedTable != "" {
			return "  [Table Detail] ↑/↓: scroll  •  v: view  •  f: fields  •  Esc: back  •  1-4/Tab: tab  •  ?: help  •  q: quit"
		}
		if m.tableViewDiagram {
			return "  [Tables ERD] ↑/↓: select  •  +/-: zoom  •  Shift+Arrows: pan  •  Enter: inspect  •  v: view  •  f: fields  •  ←/Esc: sidebar  •  ?: help  •  q: quit"
		}
		return "  [Tables] ↑/↓: select  •  Enter: inspect  •  v: view  •  f: fields  •  ←/Esc: sidebar  •  1-4/Tab: tab  •  ?: help  •  q: quit"
	case 3:
		return "  [Query] i/Enter: write SQL  •  ↑/↓: scroll  •  ←/Esc: sidebar  •  1-4/Tab: tab  •  ?: help  •  q: quit"
	default:
		return "  ←/Esc: sidebar  •  1-4/Tab: tab  •  ?: help  •  q: quit"
	}
}

func runDashboardTUI() {
	p := tea.NewProgram(
		newDashboardModel(),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running TUI dashboard: %v\n", err)
	}
}
