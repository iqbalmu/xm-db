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
