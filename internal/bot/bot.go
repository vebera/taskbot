package bot

import (
	"context"
	"fmt"
	"log"
	"sync"

	"taskbot/internal/config"
	"taskbot/internal/db"

	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	config     *config.Config
	db         *db.DB
	session    *discordgo.Session
	commands   []*discordgo.ApplicationCommand
	shutdownCh chan struct{}
	isShutdown bool
	mu         sync.Mutex
	wg         sync.WaitGroup
}

func New(discordConfig struct {
	Token    string `yaml:"token"`
	ClientID string `yaml:"client_id"`
	GuildID  string `yaml:"guild_id"`
}, database *db.DB) (*Bot, error) {
	session, err := discordgo.New("Bot " + discordConfig.Token)
	if err != nil {
		return nil, fmt.Errorf("error creating Discord session: %w", err)
	}

	return &Bot{
		db:         database,
		session:    session,
		config:     &config.Config{Discord: discordConfig},
		shutdownCh: make(chan struct{}),
		isShutdown: false,
	}, nil
}

func (b *Bot) Start(ctx context.Context) error {
	log.Println("Starting TaskBot...")

	// Register commands
	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, cmd := range commands {
		registered, err := b.session.ApplicationCommandCreate(b.config.Discord.ClientID, b.config.Discord.GuildID, cmd)
		if err != nil {
			return fmt.Errorf("error creating command %s: %w", cmd.Name, err)
		}
		registeredCommands[i] = registered
	}
	b.commands = registeredCommands
	log.Println("Commands registered successfully")

	// Register command handlers
	commandHandlers := map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"timezone":   b.handleTimezone,
		"checkin":    b.handleCheckin,
		"checkout":   b.handleCheckout,
		"status":     b.handleStatus,
		"report":     b.handleReport,
		"task":       b.handleTask,
		"globaltask": b.handleGlobalTask,
	}

	b.session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		b.wg.Add(1)
		defer b.wg.Done()

		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
				h(s, i)
			}
		case discordgo.InteractionApplicationCommandAutocomplete:
			if i.ApplicationCommandData().Name == "checkin" || i.ApplicationCommandData().Name == "task" {
				b.handleAutocomplete(s, i)
			}
		}
	})

	if err := b.session.Open(); err != nil {
		return fmt.Errorf("error opening connection: %w", err)
	}

	log.Println("Bot is now running. Press CTRL-C to exit.")

	// Wait for shutdown signal
	<-ctx.Done()
	return b.Shutdown()
}

// Shutdown performs a graceful shutdown of the bot
func (b *Bot) Shutdown() error {
	log.Println("Initiating graceful shutdown...")

	// Ensure we only close the channel once
	b.mu.Lock()
	if b.isShutdown {
		b.mu.Unlock()
		return nil
	}
	b.isShutdown = true
	close(b.shutdownCh)
	b.mu.Unlock()

	// Wait for all handlers to complete
	log.Println("Waiting for active handlers to complete...")
	b.wg.Wait()

	// Remove commands
	log.Println("Removing Discord commands...")
	for _, cmd := range b.commands {
		err := b.session.ApplicationCommandDelete(b.config.Discord.ClientID, b.config.Discord.GuildID, cmd.ID)
		if err != nil {
			log.Printf("Error removing command %s: %v", cmd.Name, err)
		}
	}

	// Close Discord session
	log.Println("Closing Discord session...")
	if err := b.session.Close(); err != nil {
		return fmt.Errorf("error closing Discord session: %w", err)
	}

	// Close database connection
	log.Println("Closing database connection...")
	b.db.Close()

	log.Println("Shutdown completed successfully")
	return nil
}
