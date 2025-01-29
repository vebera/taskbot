package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"taskbot/internal/bot"
	"taskbot/internal/config"
	"taskbot/internal/db"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to database
	database, err := db.New(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Create bot instance
	bot, err := bot.New(cfg, database)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		log.Println("Received shutdown signal")
		cancel()
	}()

	// Start bot
	if err := bot.Start(ctx); err != nil {
		log.Fatalf("Bot error: %v", err)
	}
}
