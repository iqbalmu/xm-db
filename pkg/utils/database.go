package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const (
	ProviderMySQL    = "mysql"
	ProviderPostgres = "postgres"

	DefaultPasswordLength = 16
)

func GetDriverName(provider string) string {
	if provider == ProviderPostgres {
		return "postgres"
	}
	return "mysql"
}

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

func BuildConnectionString(provider, user, password, host string, port int, dbName string) string {
	if provider == ProviderPostgres {
		return fmt.Sprintf("postgres://%s:%s@%s:%d/%s",
			user, password, host, port, dbName)
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
		user, password, host, port, dbName)
}
