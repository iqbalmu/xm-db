package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	StateDir     = ".dbm"
	DatabasesDir = "databases"
	StateFileExt = ".json"
)

type Manager struct {
	stateDir string
}

func NewManager() (*Manager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	stateDir := filepath.Join(homeDir, StateDir, DatabasesDir)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	return &Manager{stateDir: stateDir}, nil
}

func (m *Manager) Save(db *Database) error {
	filePath := filepath.Join(m.stateDir, db.Name+StateFileExt)
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal database state: %w", err)
	}
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}
	return nil
}

func (m *Manager) Load(name string) (*Database, error) {
	filePath := filepath.Join(m.stateDir, name+StateFileExt)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("database '%s' not found", name)
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var db Database
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, fmt.Errorf("failed to unmarshal database state: %w", err)
	}
	return &db, nil
}

func (m *Manager) Delete(name string) error {
	filePath := filepath.Join(m.stateDir, name+StateFileExt)
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("database '%s' not found", name)
		}
		return fmt.Errorf("failed to delete state file: %w", err)
	}
	return nil
}

func (m *Manager) List() ([]*Database, error) {
	entries, err := os.ReadDir(m.stateDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read state directory: %w", err)
	}

	var databases []*Database
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != StateFileExt {
			continue
		}

		name := entry.Name()[:len(entry.Name())-len(StateFileExt)]
		db, err := m.Load(name)
		if err != nil {
			continue
		}
		databases = append(databases, db)
	}

	return databases, nil
}

func (m *Manager) Exists(name string) bool {
	filePath := filepath.Join(m.stateDir, name+StateFileExt)
	_, err := os.Stat(filePath)
	return err == nil
}
