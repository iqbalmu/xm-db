package providers

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

type PostgresProvider struct{}

func NewPostgresProvider() *PostgresProvider {
	return &PostgresProvider{}
}

func (p *PostgresProvider) GetImageName(version string) string {
	if version == "" {
		version = DefaultPostgresVersion
	}
	return fmt.Sprintf("postgres:%s", version)
}

func (p *PostgresProvider) GetDefaultPort() int {
	return DefaultPostgresPort
}

func (p *PostgresProvider) GetContainerPort() string {
	return "5432"
}

func (p *PostgresProvider) GetEnvVars(config *DBConfig) []string {
	return []string{
		fmt.Sprintf("POSTGRES_PASSWORD=%s", config.RootPassword),
		fmt.Sprintf("POSTGRES_DB=%s", config.DatabaseName),
	}
}

func (p *PostgresProvider) GetConnectionString(config *DBConfig) string {
	return fmt.Sprintf("postgres://postgres:%s@%s:%d/%s?sslmode=disable",
		config.RootPassword,
		config.Host,
		config.Port,
		config.DatabaseName,
	)
}

func (p *PostgresProvider) CreateDatabase(ctx context.Context, db *sql.DB, dbName string) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", dbName))
	return err
}

func (p *PostgresProvider) CreateUser(ctx context.Context, db *sql.DB, username, password, dbName string) error {
	// Create user
	_, err := db.ExecContext(ctx, fmt.Sprintf(
		"CREATE USER %s WITH PASSWORD '%s'",
		username, password,
	))
	if err != nil {
		return err
	}

	// Grant privileges
	_, err = db.ExecContext(ctx, fmt.Sprintf(
		"GRANT ALL PRIVILEGES ON DATABASE %s TO %s",
		dbName, username,
	))
	return err
}

func (p *PostgresProvider) ListDatabases(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT datname FROM pg_database WHERE datistemplate = false")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var dbName string
		if err := rows.Scan(&dbName); err != nil {
			continue
		}
		databases = append(databases, dbName)
	}
	return databases, nil
}

func (p *PostgresProvider) ListTables(ctx context.Context, db *sql.DB, dbName string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = 'public'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			continue
		}
		tables = append(tables, tableName)
	}
	return tables, nil
}
