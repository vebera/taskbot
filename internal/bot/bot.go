package bot

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

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

func New(config *config.Config, database *db.DB) (*Bot, error) {
	session, err := discordgo.New("Bot " + config.Discord.Token)
	if err != nil {
		return nil, fmt.Errorf("error creating Discord session: %w", err)
	}

	return &Bot{
		db:         database,
		session:    session,
		config:     config,
		shutdownCh: make(chan struct{}),
		isShutdown: false,
	}, nil
}

// Helper function to register commands for a guild
func (b *Bot) registerGuildCommands(guildID string) error {
	log.Printf("Registering commands for guild: %s", guildID)

	// First, clean up any existing commands
	existingCommands, err := b.session.ApplicationCommands(b.config.Discord.ClientID, guildID)
	if err != nil {
		log.Printf("Error getting existing commands for guild %s: %v", guildID, err)
	} else {
		for _, cmd := range existingCommands {
			err := b.session.ApplicationCommandDelete(b.config.Discord.ClientID, guildID, cmd.ID)
			if err != nil {
				log.Printf("Error removing old command %s from guild %s: %v", cmd.Name, guildID, err)
			}
		}
	}

	// Register new commands
	var registeredCommands []*discordgo.ApplicationCommand
	for _, cmd := range commands {
		log.Printf("Registering command: %s", cmd.Name)
		registered, err := b.session.ApplicationCommandCreate(b.config.Discord.ClientID, guildID, cmd)
		if err != nil {
			log.Printf("Error registering command %s for guild %s: %v", cmd.Name, guildID, err)
			continue
		}
		registeredCommands = append(registeredCommands, registered)
		log.Printf("Successfully registered command: %s", cmd.Name)
	}

	// Update the bot's command list
	b.mu.Lock()
	b.commands = append(b.commands, registeredCommands...)
	b.mu.Unlock()

	return nil
}

func (b *Bot) Start(ctx context.Context) error {
	log.Println("Starting TaskBot...")

	// Keep trying to connect until successful
	for {
		// Test Discord API connection
		log.Println("Testing Discord API connection...")
		if _, err := b.session.User("@me"); err != nil {
			log.Printf("Failed to connect to Discord API: %v. Retrying in 5 seconds...", err)
			time.Sleep(5 * time.Second)
			continue
		}
		log.Println("Successfully connected to Discord API")
		break
	}

	// Keep trying to open session until successful
	for {
		if err := b.session.Open(); err != nil {
			log.Printf("Error opening Discord session: %v. Retrying in 5 seconds...", err)
			time.Sleep(5 * time.Second)
			continue
		}
		log.Printf("Session opened successfully (Session ID: %s)", b.session.State.SessionID)
		break
	}

	// Register handlers
	b.session.AddHandler(b.handleReady)
	b.session.AddHandler(b.handleGuildCreate)
	b.session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			b.handleCommand(s, i)
		case discordgo.InteractionApplicationCommandAutocomplete:
			b.handleAutocomplete(s, i)
		}
	})

	// Register commands for each guild the bot is in
	log.Println("Registering commands for all guilds...")
	for _, guild := range b.session.State.Guilds {
		if err := b.registerGuildCommands(guild.ID); err != nil {
			log.Printf("Error registering commands for guild %s: %v", guild.ID, err)
		}
	}
	log.Printf("Successfully registered commands for %d guilds", len(b.session.State.Guilds))

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
	for _, guild := range b.session.State.Guilds {
		log.Printf("Removing commands from guild: %s", guild.ID)
		registeredCommands, err := b.session.ApplicationCommands(b.config.Discord.ClientID, guild.ID)
		if err != nil {
			log.Printf("Error getting commands for guild %s: %v", guild.ID, err)
			continue
		}
		for _, cmd := range registeredCommands {
			err := b.session.ApplicationCommandDelete(b.config.Discord.ClientID, guild.ID, cmd.ID)
			if err != nil {
				log.Printf("Error removing command %s from guild %s: %v", cmd.Name, guild.ID, err)
			}
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

func (b *Bot) handleReady(s *discordgo.Session, r *discordgo.Ready) {
	log.Printf("Bot is ready! Connected to %d guilds", len(r.Guilds))

	// Initialize settings for all current guilds
	for _, guild := range r.Guilds {
		log.Printf("Initializing settings for guild: %s", guild.ID)
		if _, err := b.db.GetOrCreateServerSettings(guild.ID); err != nil {
			log.Printf("Error initializing settings for guild %s: %v", guild.ID, err)
		}
	}
}

func (b *Bot) handleGuildCreate(s *discordgo.Session, g *discordgo.GuildCreate) {
	log.Printf("Bot joined new guild: %s", g.ID)

	// Initialize settings for new guild
	if _, err := b.db.GetOrCreateServerSettings(g.ID); err != nil {
		log.Printf("Error initializing settings for guild %s: %v", g.ID, err)
	}

	// Register commands for the new guild
	if err := b.registerGuildCommands(g.ID); err != nil {
		log.Printf("Error registering commands for new guild %s: %v", g.ID, err)
	}
}

func (b *Bot) handleCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.ApplicationCommandData().Name {
	case "timezone":
		b.handleTimezone(s, i)
	case "declare":
		b.handleDeclare(s, i)
	case "checkin":
		b.handleCheckin(s, i)
	case "checkout":
		b.handleCheckout(s, i)
	case "status":
		b.handleStatus(s, i)
	case "report":
		b.handleReport(s, i)
	case "task":
		b.handleTask(s, i)
	case "globaltask":
		b.handleGlobalTask(s, i)
	}
}
