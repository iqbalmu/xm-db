package providers

import (
	"context"
	"database/sql"
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

type DBConfig struct {
	Host         string
	Port         int
	RootPassword string
	DatabaseName string
	User         string
	UserPassword string
}
