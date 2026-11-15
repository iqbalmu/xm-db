package providers

import (
	"context"
	"database/sql"
	"fmt"
)

const (
	DefaultMySQLPort    = 3306
	DefaultPostgresPort = 5432

	DefaultMySQLVersion    = "8.0"
	DefaultPostgresVersion = "16"

	// connectTimeoutSeconds bounds one TCP + handshake attempt, so a hung
	// dial cannot stall past the caller's own deadline.
	connectTimeoutSeconds = 5
)

type Provider interface {
	GetImageName(version string) string
	GetDefaultPort() int
	GetContainerPort() string
	GetEnvVars(config *DBConfig) []string
	GetConnectionString(config *DBConfig) string
	// Connect opens a working sql.DB for the given config. Passwords that are
	// awkward in a connection string (MySQL ':', '/', '@') are safe here.
	Connect(cfg *DBConfig) (*sql.DB, error)
	CreateUser(ctx context.Context, db *sql.DB, username, password, dbName string) error
	ListTables(ctx context.Context, db *sql.DB, dbName string) ([]string, error)
}

// For returns the provider implementation for a provider name.
func For(name string) (Provider, error) {
	switch name {
	case "mysql":
		return NewMySQLProvider(), nil
	case "postgres":
		return NewPostgresProvider(), nil
	default:
		return nil, fmt.Errorf("unsupported provider '%s'", name)
	}
}

type DBConfig struct {
	Host         string
	Port         int
	RootPassword string
	DatabaseName string
	User         string
	UserPassword string
}

// KnownVersions lists Docker tags commonly used for local development,
// ordered with the default first. Users can always enter any other tag
// manually.
func KnownVersions(provider string) []string {
	if provider == "postgres" {
		return []string{"16", "17", "18", "15", "14"}
	}
	return []string{"8.0", "8.4", "9.0"}
}

// Auth returns the username and password to connect with, preferring the
// dedicated user over the superuser account.
func (c *DBConfig) Auth(superuser string) (string, string) {
	if c.User != "" {
		pass := c.UserPassword
		if pass == "" {
			pass = c.RootPassword
		}
		return c.User, pass
	}
	return superuser, c.RootPassword
}
