package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const (
	// ProviderMySQL represents MySQL database provider
	ProviderMySQL = "mysql"
	// ProviderPostgres represents PostgreSQL database provider
	ProviderPostgres = "postgres"

	// DefaultPasswordLength is the default length for generated passwords
	DefaultPasswordLength = 16
)

// GetDriverName returns the database driver name for sql.Open()
func GetDriverName(provider string) string {
	if provider == ProviderPostgres {
		return "postgres"
	}
	return "mysql"
}

// GeneratePassword generates a cryptographically secure random password
func GeneratePassword(length int) (string, error) {
	if length <= 0 {
		length = DefaultPasswordLength
	}

	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)

	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[n.Int64()]
	}

	return string(b), nil
}

// BuildConnectionString builds a database connection string
func BuildConnectionString(provider, user, password, host string, port int, dbName string) string {
	if provider == ProviderPostgres {
		return fmt.Sprintf("postgres://%s:%s@%s:%d/%s",
			user, password, host, port, dbName)
	}
	return fmt.Sprintf("mysql://%s:%s@%s:%d/%s",
		user, password, host, port, dbName)
}
