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

const defaultPostgresMajor = 16 // matches DefaultPostgresVersion

// GetVolumePath returns where the named volume mounts. PostgreSQL 18+ images
// expect the mount at /var/lib/postgresql (data lands in a major-version
// subdirectory); older images use /var/lib/postgresql/data.
func (p *PostgresProvider) GetVolumePath(version string) string {
	if postgresMajor(version) >= 18 {
		return "/var/lib/postgresql"
	}
	return "/var/lib/postgresql/data"
}

// postgresMajor extracts the leading major version number; unrecognised
// forms fall back to the configured default.
func postgresMajor(version string) int {
	if version == "" {
		version = DefaultPostgresVersion
	}
	i := 0
	for i < len(version) && version[i] >= '0' && version[i] <= '9' {
		i++
	}
	if i == 0 {
		return defaultPostgresMajor
	}
	n, err := strconv.Atoi(version[:i])
	if err != nil || n <= 0 {
		return defaultPostgresMajor
	}
	return n
}

func (p *PostgresProvider) GetEnvVars(config *DBConfig) []string {
	return []string{
		fmt.Sprintf("POSTGRES_PASSWORD=%s", config.RootPassword),
		fmt.Sprintf("POSTGRES_DB=%s", config.DatabaseName),
	}
}

// GetConnectionString returns a display form of the connection details.
func (p *PostgresProvider) GetConnectionString(config *DBConfig) string {
	user, password := config.Auth("postgres")
	return postgresURL(config, user, password, 0)
}

// Connect opens a sql.DB over a URL DSN; unlike MySQL's format this one
// percent-escapes the credentials, so special characters are safe.
func (p *PostgresProvider) Connect(config *DBConfig) (*sql.DB, error) {
	user, password := config.Auth("postgres")
	return sql.Open("postgres", postgresURL(config, user, password, connectTimeoutSeconds))
}

// postgresURL builds the lib/pq DSN; connectTimeout is included only when
// positive so the display form stays clean.
func postgresURL(config *DBConfig, user, password string, connectTimeout int) string {
	u := url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(config.Host, strconv.Itoa(config.Port)),
		Path:   "/" + config.DatabaseName,
	}
	u.User = url.UserPassword(user, password)
	q := u.Query()
	q.Set("sslmode", "disable")
	if connectTimeout > 0 {
		q.Set("connect_timeout", strconv.Itoa(connectTimeout))
	}
	u.RawQuery = q.Encode()
	return u.String()
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
