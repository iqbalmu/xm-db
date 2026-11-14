package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	return &Manager{stateDir: t.TempDir()}
}

func sampleDB(name string) *Database {
	return &Database{
		Name:          name,
		Provider:      "postgres",
		Version:       "16",
		ContainerName: "xmdb_" + name,
		ContainerID:   "abc123",
		Port:          5432,
		RootPassword:  "rootpw",
		DatabaseName:  name,
		User:          "dev_user",
		UserPassword:  "devpw",
		VolumeName:    "xmdb_" + name + "_data",
		CreatedAt:     time.Now(),
		Status:        "running",
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	m := newTestManager(t)
	want := sampleDB("app")

	if err := m.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := m.Load("app")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Name != want.Name || got.Provider != want.Provider ||
		got.Port != want.Port || got.UserPassword != want.UserPassword {
		t.Errorf("roundtrip mismatch: got %+v", got)
	}
}

func TestStateFileIsOwnerOnly(t *testing.T) {
	m := newTestManager(t)
	if err := m.Save(sampleDB("app")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(filepath.Join(m.stateDir, "app.json"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("state file mode = %o, want 600", perm)
	}
}

func TestGroupRoundtrip(t *testing.T) {
	m := newTestManager(t)
	want := sampleDB("app")
	want.Group = "work"

	if err := m.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := m.Load("app")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Group != "work" {
		t.Errorf("Group = %q, want work", got.Group)
	}

	// Legacy state files without the group field load as global.
	plain := sampleDB("plain")
	plain.Group = ""
	if err := m.Save(plain); err != nil {
		t.Fatalf("Save plain: %v", err)
	}
	got, err = m.Load("plain")
	if err != nil {
		t.Fatalf("Load plain: %v", err)
	}
	if got.Group != "" {
		t.Errorf("ungrouped Group = %q, want empty", got.Group)
	}
}

func TestExistsAndDelete(t *testing.T) {
	m := newTestManager(t)
	if m.Exists("app") {
		t.Fatal("Exists before save should be false")
	}

	if err := m.Save(sampleDB("app")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !m.Exists("app") {
		t.Fatal("Exists after save should be true")
	}

	if err := m.Delete("app"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if m.Exists("app") {
		t.Fatal("Exists after delete should be false")
	}
	if err := m.Delete("app"); err == nil {
		t.Error("Delete of missing database should error")
	}
}

func TestLoadMissing(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Load("nope"); err == nil {
		t.Error("Load of missing database should error")
	}
}

func TestList(t *testing.T) {
	m := newTestManager(t)
	for _, name := range []string{"a", "b"} {
		if err := m.Save(sampleDB(name)); err != nil {
			t.Fatalf("Save %s: %v", name, err)
		}
	}

	dbs, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(dbs) != 2 {
		t.Fatalf("List returned %d databases, want 2", len(dbs))
	}

	names := map[string]bool{}
	for _, d := range dbs {
		names[d.Name] = true
	}
	if !names["a"] || !names["b"] {
		t.Errorf("List missing expected names: %v", names)
	}
}
