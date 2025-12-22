package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Manager struct {
	stateDir string
}

func NewManager() (*Manager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	stateDir := filepath.Join(homeDir, ".dbm", "databases")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, err
	}

	return &Manager{stateDir: stateDir}, nil
}

func (m *Manager) Save(db *Database) error {
	filePath := filepath.Join(m.stateDir, db.Name+".json")
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

func (m *Manager) Load(name string) (*Database, error) {
	filePath := filepath.Join(m.stateDir, name+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var db Database
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, err
	}
	return &db, nil
}

func (m *Manager) Delete(name string) error {
	filePath := filepath.Join(m.stateDir, name+".json")
	return os.Remove(filePath)
}

func (m *Manager) List() ([]*Database, error) {
	entries, err := os.ReadDir(m.stateDir)
	if err != nil {
		return nil, err
	}

	var databases []*Database
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		name := entry.Name()[:len(entry.Name())-5] // remove .json
		db, err := m.Load(name)
		if err != nil {
			continue
		}
		databases = append(databases, db)
	}

	return databases, nil
}

func (m *Manager) Exists(name string) bool {
	filePath := filepath.Join(m.stateDir, name+".json")
	_, err := os.Stat(filePath)
	return err == nil
}
