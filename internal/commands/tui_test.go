package commands

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iqbalmu/xm-db/internal/state"
)

func TestCreateModelStepNavigation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	mgr, err := state.NewManager()
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	m := newCreateModel(mgr)
	if m.step != stepName {
		t.Fatalf("initial step = %d, want %d", m.step, stepName)
	}

	// Empty name should fail validation and stay on stepName
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(createModel)
	if m.validationErr == "" {
		t.Fatalf("expected validation error for empty db name")
	}
	if m.step != stepName {
		t.Fatalf("expected to stay on stepName, got %d", m.step)
	}

	// Valid name advances to stepProvider
	m.input.SetValue("testdb")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(createModel)
	if m.step != stepProvider || m.dbName != "testdb" {
		t.Fatalf("got step=%d dbName=%q, want stepProvider and testdb", m.step, m.dbName)
	}

	// Esc goes back to stepName
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(createModel)
	if m.step != stepName {
		t.Fatalf("expected Esc to return to stepName, got %d", m.step)
	}

	// Esc on stepName sets quit=true
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(createModel)
	if !m.quit {
		t.Fatalf("expected Esc on first step to quit")
	}
}

func TestDashboardModelFilterAndTabs(t *testing.T) {
	m := newDashboardModel()
	m.entries = []dbEntry{
		{db: &state.Database{Name: "app_pg", Provider: "postgres", Port: 5432, Group: "work"}},
		{db: &state.Database{Name: "blog_mysql", Provider: "mysql", Port: 3306, Group: ""}},
	}
	m.applyFilter()
	if len(m.filtered) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m.filtered))
	}

	// Filter by group "work"
	m.filterInput.SetValue("work")
	m.applyFilter()
	if len(m.filtered) != 1 || m.filtered[0].db.Name != "app_pg" {
		t.Fatalf("expected only app_pg, got %+v", m.filtered)
	}

	// Tab switching
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = next.(dashboardModel)
	if m.activeTab != 1 {
		t.Fatalf("expected activeTab=1, got %d", m.activeTab)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(dashboardModel)
	if m.activeTab != 2 {
		t.Fatalf("expected activeTab=2, got %d", m.activeTab)
	}

	// Confirm mode for remove & cancel
	m.busy = ""
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(dashboardModel)
	if m.confirmMode != "remove" {
		t.Fatalf("expected confirmMode=remove, got %q", m.confirmMode)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(dashboardModel)
	if m.confirmMode != "" {
		t.Fatalf("expected confirmMode cleared on cancel, got %q", m.confirmMode)
	}
}

func TestDashboardGroupCollapseAndPaneFocus(t *testing.T) {
	m := newDashboardModel()
	m.busy = ""
	m.entries = []dbEntry{
		{db: &state.Database{Name: "app_pg", Provider: "postgres", Port: 5432, Group: "work"}, running: true},
		{db: &state.Database{Name: "api_mysql", Provider: "mysql", Port: 3306, Group: "work"}, running: false},
		{db: &state.Database{Name: "local_pg", Provider: "postgres", Port: 5433, Group: ""}},
	}
	m.applyFilter()

	// Rows: [group:work, api_mysql, app_pg, group:global, local_pg] -> 5 rows
	if len(m.rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(m.rows))
	}
	// Cursor should land on first DB item (index 1)
	if m.selected() == nil || m.selected().db.Name != "api_mysql" {
		t.Fatalf("expected api_mysql selected, got %+v", m.selected())
	}

	// Move up to group header and collapse it
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(dashboardModel)
	if !m.rows[m.cursor].isGroup || m.rows[m.cursor].group != "work" {
		t.Fatalf("expected cursor on 'work' group header")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(dashboardModel)
	if !m.collapsedGroups["work"] {
		t.Fatalf("expected 'work' group to be collapsed")
	}
	if len(m.rows) != 3 {
		t.Fatalf("expected 3 rows after collapse, got %d", len(m.rows))
	}

	// Switch pane focus right/left
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = next.(dashboardModel)
	if m.focusPane != 1 {
		t.Fatalf("expected focusPane=1, got %d", m.focusPane)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = next.(dashboardModel)
	if m.focusPane != 0 {
		t.Fatalf("expected focusPane=0, got %d", m.focusPane)
	}
}

func TestDashboardSelectionSurvivesReload(t *testing.T) {
	m := newDashboardModel()
	m.busy = ""
	m.entries = []dbEntry{
		{db: &state.Database{Name: "app_pg", Provider: "postgres", Port: 5432, Group: "work"}},
		{db: &state.Database{Name: "api_mysql", Provider: "mysql", Port: 3306, Group: "work"}},
		{db: &state.Database{Name: "local_pg", Provider: "postgres", Port: 5433, Group: ""}},
	}
	m.applyFilter()

	// Rest on the "work" group header (as after collapsing it with enter).
	m.cursor = 0
	m.applyFilter()
	if !m.rows[m.cursor].isGroup || m.rows[m.cursor].group != "work" {
		t.Fatalf("cursor on group header jumped to row %d (%s)", m.cursor, m.rows[m.cursor].group)
	}

	// Simulate a periodic reload: selection must follow the same db by name.
	for i, r := range m.rows {
		if !r.isGroup && r.entry.db.Name == "app_pg" {
			m.cursor = i
			break
		}
	}
	m.applyFilter()
	if sel := m.selected(); sel == nil || sel.db.Name != "app_pg" {
		if sel == nil {
			t.Fatalf("expected selection preserved, got group header row %d", m.cursor)
		} else {
			t.Fatalf("expected app_pg selected, got %s", sel.db.Name)
		}
	}

	// Second reload with identical entries: cursor must not drift.
	m.applyFilter()
	if sel := m.selected(); sel == nil || sel.db.Name != "app_pg" {
		t.Fatalf("cursor drifted after reload: row %d", m.cursor)
	}

	// Header selection also survives a reload while its group stays collapsed.
	m.collapsedGroups["work"] = true
	m.applyFilter()
	m.cursor = 0
	m.applyFilter()
	if !m.rows[m.cursor].isGroup || m.rows[m.cursor].group != "work" {
		t.Fatalf("cursor on collapsed group header jumped to row %d", m.cursor)
	}
}

// The 5s tick reloads via msgDatabasesLoaded -> applyFilter; a cursor resting
// on a group header must not be hijacked to the first db row.
func TestDashboardTickReloadKeepsHeaderSelection(t *testing.T) {
	m := newDashboardModel()
	m.busy = ""
	m.entries = []dbEntry{
		{db: &state.Database{Name: "app_pg", Provider: "postgres", Port: 5432, Group: "work"}},
		{db: &state.Database{Name: "blog_mysql", Provider: "mysql", Port: 3306, Group: ""}},
	}
	m.applyFilter()

	// Move cursor up onto the "work" group header.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(dashboardModel)
	if !m.rows[m.cursor].isGroup {
		t.Fatalf("expected cursor on group header")
	}

	// Deliver what the tick produces: a fresh load with identical entries.
	next, _ = m.Update(msgDatabasesLoaded{
		entries: []dbEntry{
			{db: &state.Database{Name: "app_pg", Provider: "postgres", Port: 5432, Group: "work"}},
			{db: &state.Database{Name: "blog_mysql", Provider: "mysql", Port: 3306, Group: ""}},
		},
	})
	m = next.(dashboardModel)
	if !m.rows[m.cursor].isGroup || m.rows[m.cursor].group != "work" {
		t.Fatalf("tick reload moved cursor off group header to row %d", m.cursor)
	}
}

func TestDashboardSelectionFilteredOut(t *testing.T) {
	m := newDashboardModel()
	m.busy = ""
	m.entries = []dbEntry{
		{db: &state.Database{Name: "app_pg", Provider: "postgres", Port: 5432, Group: "work"}},
		{db: &state.Database{Name: "blog_mysql", Provider: "mysql", Port: 3306, Group: ""}},
	}
	m.applyFilter()

	// Select app_pg, then filter it out: cursor must land on a db row, not a header.
	m.cursor = 1
	m.filterInput.SetValue("blog")
	m.applyFilter()
	if sel := m.selected(); sel == nil || sel.db.Name != "blog_mysql" {
		t.Fatalf("expected fallback to blog_mysql, got cursor row %d", m.cursor)
	}
}

func TestDashboardModalsAndResponsiveLayout(t *testing.T) {
	m := newDashboardModel()
	m.busy = ""
	m.entries = []dbEntry{
		{db: &state.Database{Name: "app_pg", Provider: "postgres", Port: 5432}, running: true},
	}
	m.applyFilter()

	// Help modal
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(dashboardModel)
	if m.modal != modalHelp {
		t.Fatalf("expected modalHelp, got %v", m.modal)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(dashboardModel)
	if m.modal != modalNone {
		t.Fatalf("expected modalNone after Esc, got %v", m.modal)
	}

	// Backup modal on running DB
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = next.(dashboardModel)
	if m.modal != modalBackup {
		t.Fatalf("expected modalBackup, got %v", m.modal)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(dashboardModel)
	if m.modal != modalNone {
		t.Fatalf("expected modalNone after Esc, got %v", m.modal)
	}

	// Responsive stacked layout when width < 80
	next, _ = m.Update(tea.WindowSizeMsg{Width: 70, Height: 28})
	m = next.(dashboardModel)
	stacked, _, _, _, _ := m.layoutDimensions()
	if !stacked {
		t.Fatalf("expected stacked=true for width=70")
	}
	if v := m.View(); v == "" {
		t.Fatalf("expected non-empty View()")
	}
}
