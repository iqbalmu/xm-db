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
)

type Provider interface {
	GetImageName(version string) string
	GetDefaultPort() int
	GetContainerPort() string
	GetEnvVars(config *DBConfig) []string
	GetConnectionString(config *DBConfig) string
	CreateDatabase(ctx context.Context, db *sql.DB, dbName string) error
	CreateUser(ctx context.Context, db *sql.DB, username, password, dbName string) error
	ListDatabases(ctx context.Context, db *sql.DB) ([]string, error)
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
