package state

import (
	"time"
)

// Database represents a managed database instance
type Database struct {
	Name          string    `json:"name"`
	Provider      string    `json:"provider"`
	Version       string    `json:"version"`
	ContainerName string    `json:"container_name"`
	ContainerID   string    `json:"container_id"`
	Port          int       `json:"port"`
	RootPassword  string    `json:"root_password"`
	DatabaseName  string    `json:"database_name"`
	User          string    `json:"user,omitempty"`
	UserPassword  string    `json:"user_password,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	Status        string    `json:"status"`
}

// GetUsername returns the username to use for connections
func (d *Database) GetUsername() string {
	if d.User != "" {
		return d.User
	}
	if d.Provider == "postgres" {
		return "postgres"
	}
	return "root"
}

// GetPassword returns the password to use for connections
func (d *Database) GetPassword() string {
	if d.UserPassword != "" {
		return d.UserPassword
	}
	return d.RootPassword
}
