package providers

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLProvider struct{}

func NewMySQLProvider() *MySQLProvider {
	return &MySQLProvider{}
}

func (m *MySQLProvider) GetImageName(version string) string {
	if version == "" {
		version = DefaultMySQLVersion
	}
	return fmt.Sprintf("mysql:%s", version)
}

func (m *MySQLProvider) GetDefaultPort() int {
	return DefaultMySQLPort
}

func (m *MySQLProvider) GetContainerPort() string {
	return "3306"
}

func (m *MySQLProvider) GetEnvVars(config *DBConfig) []string {
	return []string{
		fmt.Sprintf("MYSQL_ROOT_PASSWORD=%s", config.RootPassword),
		fmt.Sprintf("MYSQL_DATABASE=%s", config.DatabaseName),
	}
}

func (m *MySQLProvider) GetConnectionString(config *DBConfig) string {
	return fmt.Sprintf("root:%s@tcp(%s:%d)/%s",
		config.RootPassword,
		config.Host,
		config.Port,
		config.DatabaseName,
	)
}

func (m *MySQLProvider) CreateDatabase(ctx context.Context, db *sql.DB, dbName string) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", dbName))
	return err
}

func (m *MySQLProvider) CreateUser(ctx context.Context, db *sql.DB, username, password, dbName string) error {
	// Create user
	_, err := db.ExecContext(ctx, fmt.Sprintf(
		"CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'",
		username, password,
	))
	if err != nil {
		return err
	}

	// Grant privileges
	_, err = db.ExecContext(ctx, fmt.Sprintf(
		"GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%'",
		dbName, username,
	))
	if err != nil {
		return err
	}

	_, err = db.ExecContext(ctx, "FLUSH PRIVILEGES")
	return err
}

func (m *MySQLProvider) ListDatabases(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, "SHOW DATABASES")
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

func (m *MySQLProvider) ListTables(ctx context.Context, db *sql.DB, dbName string) ([]string, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SHOW TABLES FROM `%s`", dbName))
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
