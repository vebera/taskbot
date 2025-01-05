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
	log.Println("Starting TaskBot application...")

	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	database, err := db.New(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Initialize bot
	discordBot, err := bot.New(cfg.Discord, database)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Set up signal handling
	ctx, cancel := context.WithCancel(context.Background())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	go func() {
		s := <-signals
		log.Printf("Received signal: %v", s)
		cancel()
	}()

	// Start the bot
	go func() {
		if err := discordBot.Start(ctx); err != nil {
			log.Printf("Error running bot: %v", err)
			cancel()
		}
	}()

	// Wait for shutdown
	<-ctx.Done()
	log.Println("Shutdown signal received")

	// Perform cleanup
	if err := discordBot.Shutdown(); err != nil {
		log.Printf("Error during shutdown: %v", err)
		os.Exit(1)
	}

	log.Println("Application shutdown complete")
}
