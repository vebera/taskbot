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
		Token    string `yaml:"token" env:"DISCORD_TOKEN,required"`
		ClientID string `yaml:"client_id" env:"DISCORD_CLIENT_ID,required"`
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
