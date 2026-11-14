package providers

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strconv"

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
	user, password := config.Auth("postgres")

	u := url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(config.Host, strconv.Itoa(config.Port)),
		Path:   "/" + config.DatabaseName,
	}
	u.User = url.UserPassword(user, password)
	q := u.Query()
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()
	return u.String()
}

func (p *PostgresProvider) CreateDatabase(ctx context.Context, db *sql.DB, dbName string) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", QuoteIdentPG(dbName)))
	return err
}

func (p *PostgresProvider) CreateUser(ctx context.Context, db *sql.DB, username, password, dbName string) error {
	user := QuoteIdentPG(username)

	_, err := db.ExecContext(ctx, fmt.Sprintf(
		"CREATE USER %s WITH PASSWORD %s",
		user, QuoteLiteralPG(password),
	))
	if err != nil {
		return err
	}

	// PostgreSQL 15+ removed CREATE on schema public for non-owners. Making the
	// user the database owner restores it via the pg_database_owner pseudo-role.
	if _, err := db.ExecContext(ctx, fmt.Sprintf(
		"ALTER DATABASE %s OWNER TO %s",
		QuoteIdentPG(dbName), user,
	)); err != nil {
		return err
	}

	_, err = db.ExecContext(ctx, fmt.Sprintf(
		"GRANT ALL PRIVILEGES ON DATABASE %s TO %s",
		QuoteIdentPG(dbName), user,
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
