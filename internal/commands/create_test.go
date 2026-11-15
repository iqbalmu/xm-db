package commands

import (
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/iqbalmu/xm-db/internal/state"
)

// newTestStateManager returns a manager rooted at a temp HOME so tests never
// touch the real ~/.xmdb.
func newTestStateManager(t *testing.T) *state.Manager {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	m, err := state.NewManager()
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

func TestDbNameValidator(t *testing.T) {
	v := dbNameValidator(newTestStateManager(t))

	for _, name := range []string{"app", "app-db", "app_db", "app.db", "a", "App1"} {
		if err := v(name); err != nil {
			t.Errorf("dbNameValidator(%q) = %v, want nil", name, err)
		}
	}

	for _, name := range []string{
		"",                      // empty
		"-start",                // must start alphanumeric
		".start",                // must start alphanumeric
		"has space",             // whitespace
		"sla/sh",                // path separator
		strings.Repeat("a", 51), // over the 50-char cap
	} {
		if err := v(name); err == nil {
			t.Errorf("dbNameValidator(%q) = nil, want an error", name)
		}
	}
}

func TestDbNameValidatorRejectsExisting(t *testing.T) {
	m := newTestStateManager(t)
	v := dbNameValidator(m)

	if err := m.Save(&state.Database{Name: "taken"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := v("taken"); err == nil {
		t.Error("existing name should be rejected")
	}
	if err := v("other"); err != nil {
		t.Errorf("fresh name rejected: %v", err)
	}
}

func TestPortValidator(t *testing.T) {
	m := newTestStateManager(t)
	if err := m.Save(&state.Database{Name: "app", Port: 3306}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	v := portValidator(m)

	for _, port := range []string{"1", "65535", "35432"} {
		if err := v(port); err != nil {
			t.Errorf("portValidator(%q) = %v, want nil", port, err)
		}
	}

	for _, port := range []string{"0", "-1", "65536", "not-a-number", "3306"} {
		if err := v(port); err == nil {
			t.Errorf("portValidator(%q) = nil, want an error", port)
		}
	}
}

func TestVersionAndIdentifierValidators(t *testing.T) {
	for _, v := range []string{"8.0", "17", "9.0-alpine", "18-beta1"} {
		if err := versionValidator(v); err != nil {
			t.Errorf("versionValidator(%q) = %v, want nil", v, err)
		}
	}
	for _, v := range []string{"", "-start", "has space"} {
		if err := versionValidator(v); err == nil {
			t.Errorf("versionValidator(%q) = nil, want an error", v)
		}
	}

	for _, u := range []string{"dev_user", "user1"} {
		if err := identifierValidator(u); err != nil {
			t.Errorf("identifierValidator(%q) = %v, want nil", u, err)
		}
	}
	if err := identifierValidator("bad-name"); err == nil {
		t.Error("identifierValidator should reject dashes")
	}

	if err := requiredText("x"); err != nil {
		t.Errorf("requiredText(x) = %v, want nil", err)
	}
	if err := requiredText("   "); err == nil {
		t.Error("requiredText(whitespace) should error")
	}
}

func TestNextFreePortSkipsManaged(t *testing.T) {
	m := newTestStateManager(t)
	if err := m.Save(&state.Database{Name: "app", Port: 35444}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got := nextFreePort(m, 35444); got != 35445 {
		t.Errorf("nextFreePort from a managed port = %d, want 35445", got)
	}
	if got := nextFreePort(m, 35445); got != 35445 {
		t.Errorf("nextFreePort from a free port = %d, want 35445", got)
	}
}

func TestPortAvailabilityRows(t *testing.T) {
	m := newTestStateManager(t)
	for _, db := range []*state.Database{
		{Name: "analytics", Port: 5432},
		{Name: "app", Port: 3306},
	} {
		if err := m.Save(db); err != nil {
			t.Fatalf("Save %s: %v", db.Name, err)
		}
	}

	// Suggested port is free and equals the default: no extra row.
	rows := portAvailabilityRows(m, 3307, 3307)
	want := []string{
		"  3306  in use  managed database 'app'",
		"  5432  in use  managed database 'analytics'",
		"  3307  free    suggested",
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %v, want %v", rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, rows[i], want[i])
		}
	}

	// A default port held by a non-managed listener gets its own row.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer listener.Close()
	busy := listener.Addr().(*net.TCPAddr).Port

	rows = portAvailabilityRows(m, busy, busy+1)
	found := false
	for _, r := range rows {
		if strings.Contains(r, "another process") && strings.Contains(r, strconv.Itoa(busy)) {
			found = true
		}
	}
	if !found {
		t.Errorf("rows missing 'another process' row for busy port %d: %v", busy, rows)
	}
}

func TestKnownGroups(t *testing.T) {
	m := newTestStateManager(t)
	for _, db := range []*state.Database{
		{Name: "a", Group: "work"},
		{Name: "b", Group: "personal"},
		{Name: "c", Group: "work"}, // duplicate
		{Name: "d"},                // global: not listed
	} {
		if err := m.Save(db); err != nil {
			t.Fatalf("Save %s: %v", db.Name, err)
		}
	}

	got := knownGroups(m)
	want := []string{"personal", "work"}
	if len(got) != len(want) {
		t.Fatalf("knownGroups = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("knownGroups[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestGroupValidator(t *testing.T) {
	for _, g := range []string{"work", "my projects", "dev-team.v2"} {
		if err := groupValidator(g); err != nil {
			t.Errorf("groupValidator(%q) = %v, want nil", g, err)
		}
	}
	for _, g := range []string{"", " leadingspace", strings.Repeat("a", 31)} {
		if err := groupValidator(g); err == nil {
			t.Errorf("groupValidator(%q) = nil, want an error", g)
		}
	}
}
