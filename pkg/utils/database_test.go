package utils

import (
	"strings"
	"testing"
)

func TestGeneratePasswordLengthAndCharset(t *testing.T) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	pw, err := GeneratePassword(16)
	if err != nil {
		t.Fatalf("GeneratePassword: %v", err)
	}
	if len(pw) != 16 {
		t.Fatalf("length = %d, want 16", len(pw))
	}
	for _, c := range pw {
		if !strings.ContainsRune(charset, c) {
			t.Errorf("password contains unexpected character %q", c)
		}
	}

	// A 16-char alphanumeric draw repeating the single-char password is
	// practically impossible; guards against a constant generator.
	pw2, _ := GeneratePassword(16)
	if pw == pw2 {
		t.Error("two generated passwords should differ")
	}
}

func TestGeneratePasswordNonPositive(t *testing.T) {
	pw, err := GeneratePassword(0)
	if err != nil {
		t.Fatalf("GeneratePassword(0): %v", err)
	}
	if len(pw) != DefaultPasswordLength {
		t.Errorf("length = %d, want default %d", len(pw), DefaultPasswordLength)
	}
}

func TestGetDriverName(t *testing.T) {
	if GetDriverName(ProviderMySQL) != "mysql" {
		t.Error("mysql driver name wrong")
	}
	if GetDriverName(ProviderPostgres) != "postgres" {
		t.Error("postgres driver name wrong")
	}
	if GetDriverName("unknown") != "mysql" {
		t.Error("unknown provider should fall back to mysql")
	}
}
