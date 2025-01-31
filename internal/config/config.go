package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Discord struct {
		Token       string `yaml:"token" env:"DISCORD_TOKEN,required"`
		ClientID    string `yaml:"client_id" env:"DISCORD_CLIENT_ID,required"`
		Permissions int64  `yaml:"permissions" env:"DISCORD_PERMISSIONS"`
	} `yaml:"discord"`

	Database struct {
		Host     string `yaml:"host" env:"DB_HOST,required"`
		Port     int    `yaml:"port" env:"DB_PORT,required"`
		User     string `yaml:"user" env:"DB_USER,required"`
		Password string `yaml:"password" env:"DB_PASSWORD,required"`
		DBName   string `yaml:"dbname" env:"DB_NAME,required"`
		SSLMode  string `yaml:"sslmode" env:"DB_SSLMODE,required"`
	} `yaml:"database"`
}

var configPaths = []string{
	"config.yaml",              // Current directory
	"/etc/taskbot/config.yaml", // Docker mount path
	"../config.yaml",           // Parent directory
	"../../config.yaml",        // Two levels up
}

func Load() (*Config, error) {
	var data []byte
	var err error
	var loadedPath string

	// Try each config path
	for _, path := range configPaths {
		data, err = os.ReadFile(path)
		if err == nil {
			loadedPath = path
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("error reading config file from paths %v: %w", configPaths, err)
	}

	// Replace environment variables in the YAML content
	content := string(data)
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 {
			continue
		}
		placeholder := "${" + pair[0] + "}"
		// For DISCORD_PERMISSIONS, convert to int64 if needed
		if pair[0] == "DISCORD_PERMISSIONS" {
			if perm, err := strconv.ParseInt(pair[1], 10, 64); err == nil {
				content = strings.ReplaceAll(content, placeholder, fmt.Sprintf("%d", perm))
			}
		} else {
			content = strings.ReplaceAll(content, placeholder, pair[1])
		}
	}

	var cfg Config
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return nil, fmt.Errorf("error parsing config from %s: %w", loadedPath, err)
	}

	// Load permissions from environment variable if present
	if permStr := os.Getenv("DISCORD_PERMISSIONS"); permStr != "" {
		perm, err := strconv.ParseInt(permStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid DISCORD_PERMISSIONS value: %w", err)
		}
		cfg.Discord.Permissions = perm
	}

	// Convert DB_PORT from string to int if it's an environment variable
	if portStr := os.Getenv("DB_PORT"); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid DB_PORT value: %w", err)
		}
		cfg.Database.Port = port
	}

	return &cfg, nil
}
