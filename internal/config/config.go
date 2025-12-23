package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	ConfigFileName = "config.yaml"
	ConfigDir      = ".dbm"
)

type Config struct {
	DefaultProvider string            `yaml:"default_provider"`
	DefaultVersion  map[string]string `yaml:"default_version"`
	DefaultPort     map[string]int    `yaml:"default_port"`
	VolumeEnabled   bool              `yaml:"volume_enabled"`
	BackupDir       string            `yaml:"backup_dir"`
}

func DefaultConfig() *Config {
	return &Config{
		DefaultProvider: "mysql",
		DefaultVersion: map[string]string{
			"mysql":    "8.0",
			"postgres": "16",
		},
		DefaultPort: map[string]int{
			"mysql":    3306,
			"postgres": 5432,
		},
		VolumeEnabled: true,
		BackupDir:     "./backups",
	}
}

func Load() (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Save() error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func GetConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ConfigDir, ConfigFileName), nil
}

func Exists() bool {
	configPath, err := GetConfigPath()
	if err != nil {
		return false
	}

	_, err = os.Stat(configPath)
	return err == nil
}
