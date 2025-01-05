package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"taskbot/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to database
	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.DBName,
		cfg.Database.SSLMode,
	)

	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer pool.Close()

	// Read and execute migration file
	migrations := []string{
		"migrations/001_initial_schema.sql",
		"migrations/002_add_active_status.sql",
	}

	for _, migrationFile := range migrations {
		migration, err := os.ReadFile(migrationFile)
		if err != nil {
			log.Fatalf("Error reading migration file %s: %v", migrationFile, err)
		}

		_, err = pool.Exec(context.Background(), string(migration))
		if err != nil {
			log.Fatalf("Error executing migration %s: %v", migrationFile, err)
		}
		log.Printf("Successfully applied migration: %s", migrationFile)
	}

	log.Println("Migration completed successfully")
}
