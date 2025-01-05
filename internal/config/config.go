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
		Token    string `yaml:"token"`
		ClientID string `yaml:"client_id"`
		GuildID  string `yaml:"guild_id"`
	} `yaml:"discord"`

	Database struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		DBName   string `yaml:"dbname"`
		SSLMode  string `yaml:"sslmode"`
	} `yaml:"database"`
}

func Load() (*Config, error) {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// Replace environment variables in the YAML content
	content := string(data)
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 {
			continue
		}
		placeholder := "${" + pair[0] + "}"
		content = strings.ReplaceAll(content, placeholder, pair[1])
	}

	var cfg Config
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
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
