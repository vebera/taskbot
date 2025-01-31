package bot

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"taskbot/internal/config"
	"taskbot/internal/db"

	"github.com/bwmarrin/discordgo"
)

var (
	dmAllowedCommands = map[string]bool{
		"timezone": true, // Allow timezone setting in DMs
		"help":     true, // If you have a help command
		"status":   true, // Allow checking status in DMs
	}
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

	// Update intents to include all necessary ones
	session.Identify.Intents = discordgo.IntentsAllWithoutPrivileged |
		discordgo.IntentsGuildMembers |
		discordgo.IntentsGuildPresences |
		discordgo.IntentsMessageContent |
		discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages

	// Log configuration details
	log.Printf("Bot intents: %d", session.Identify.Intents)
	log.Printf("Bot permissions: %d", config.Discord.Permissions)

	// Generate and log invite URL with proper scopes
	inviteURL := fmt.Sprintf("https://discord.com/api/oauth2/authorize?client_id=%s&permissions=%d&scope=bot%%20applications.commands",
		config.Discord.ClientID,
		config.Discord.Permissions)
	log.Printf("Bot invite URL: %s", inviteURL)

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
	// Get guild info for better logging
	guild, err := b.session.Guild(guildID)
	if err != nil {
		log.Printf(formatLogMessage(guildID, "Error getting guild info: "+err.Error(), "system", "unknown"))
		return err
	}

	log.Printf(formatLogMessage(guildID, "Starting command registration", "system", guild.Name))

	// First, clean up existing commands
	existingCommands, err := b.session.ApplicationCommands(b.config.Discord.ClientID, guildID)
	if err != nil {
		log.Printf(formatLogMessage(guildID, "Error getting existing commands: "+err.Error(), "system", guild.Name))
	} else {
		for _, cmd := range existingCommands {
			err := b.session.ApplicationCommandDelete(b.config.Discord.ClientID, guildID, cmd.ID)
			if err != nil {
				log.Printf(formatLogMessage(guildID, "Error removing command: "+cmd.Name, "system", guild.Name))
			} else {
				log.Printf(formatLogMessage(guildID, "Successfully removed command: "+cmd.Name, "system", guild.Name))
			}
		}
	}

	// Register new commands
	var registeredCommands []*discordgo.ApplicationCommand
	for _, cmd := range commands {
		log.Printf(formatLogMessage(guildID, "Registering command: "+cmd.Name, "system", guild.Name))
		registered, err := b.session.ApplicationCommandCreate(b.config.Discord.ClientID, guildID, cmd)
		if err != nil {
			log.Printf(formatLogMessage(guildID, "Error registering command: "+cmd.Name+", error: "+err.Error(), "system", guild.Name))
			continue
		}
		registeredCommands = append(registeredCommands, registered)
		log.Printf(formatLogMessage(guildID, "Successfully registered command: "+cmd.Name, "system", guild.Name))
	}

	// Update the bot's command list
	b.mu.Lock()
	b.commands = append(b.commands, registeredCommands...)
	b.mu.Unlock()

	return nil
}

func (b *Bot) Start(ctx context.Context) error {
	log.Println("Starting TaskBot...")

	// Print bot invite URL
	inviteURL := fmt.Sprintf("https://discord.com/api/oauth2/authorize?client_id=%s&permissions=%d&scope=bot%%20applications.commands",
		b.config.Discord.ClientID,
		b.config.Discord.Permissions)
	log.Printf("Bot invite URL: %s", inviteURL)

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

	// Force re-register commands for all guilds
	log.Println("Force re-registering commands for all guilds...")
	for _, guild := range b.session.State.Guilds {
		// First, delete all existing commands
		existingCommands, err := b.session.ApplicationCommands(b.config.Discord.ClientID, guild.ID)
		if err != nil {
			log.Printf("Error getting existing commands for guild %s: %v", guild.ID, err)
			continue
		}

		for _, cmd := range existingCommands {
			err := b.session.ApplicationCommandDelete(b.config.Discord.ClientID, guild.ID, cmd.ID)
			if err != nil {
				log.Printf("Error deleting command %s from guild %s: %v", cmd.Name, guild.ID, err)
			}
		}

		// Then register commands again
		if err := b.registerGuildCommands(guild.ID); err != nil {
			log.Printf("Error registering commands for guild %s: %v", guild.ID, err)
		}
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
	for _, guild := range b.session.State.Guilds {
		// Get guild info for better logging
		guildInfo, err := b.session.Guild(guild.ID)
		guildName := "unknown"
		if err == nil {
			guildName = guildInfo.Name
		}

		log.Printf("Removing commands from guild: %s (Name: %s)", guild.ID, guildName)

		registeredCommands, err := b.session.ApplicationCommands(b.config.Discord.ClientID, guild.ID)
		if err != nil {
			log.Printf("Error getting commands for guild %s (%s): %v", guildName, guild.ID, err)
			continue
		}
		for _, cmd := range registeredCommands {
			err := b.session.ApplicationCommandDelete(b.config.Discord.ClientID, guild.ID, cmd.ID)
			if err != nil {
				log.Printf("Error removing command %s from guild %s (%s): %v", cmd.Name, guildName, guild.ID, err)
			} else {
				log.Printf("Successfully removed command %s from guild %s (%s)", cmd.Name, guildName, guild.ID)
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
	log.Printf("Bot joined new guild: %s (Name: %s)", g.ID, g.Name)

	// Initialize settings for new guild
	if _, err := b.db.GetOrCreateServerSettings(g.ID); err != nil {
		log.Printf("Error initializing settings for guild %s (%s): %v", g.Name, g.ID, err)
	} else {
		log.Printf("Successfully initialized settings for guild %s (%s)", g.Name, g.ID)
	}

	// Register commands for the new guild
	if err := b.registerGuildCommands(g.ID); err != nil {
		log.Printf("Error registering commands for guild %s (%s): %v", g.Name, g.ID, err)
	} else {
		log.Printf("Successfully registered all commands for guild %s (%s)", g.Name, g.ID)
	}
}

func (b *Bot) handleCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Add defer to catch panics with stack trace
	defer func() {
		if r := recover(); r != nil {
			// Get username and context for better error tracking
			var username, context string
			if i.Member != nil && i.Member.User != nil {
				username = i.Member.User.Username
				if guild, err := s.Guild(i.GuildID); err == nil {
					context = fmt.Sprintf("guild %s (%s)", guild.Name, i.GuildID)
				} else {
					context = fmt.Sprintf("guild ID %s", i.GuildID)
				}
			} else if i.User != nil {
				username = i.User.Username
				context = "DM"
			} else {
				username = "unknown"
				context = "unknown context"
			}

			// Log the stack trace with context
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			log.Printf("Panic in command handler for user %s in %s:\nError: %v\nStack Trace:\n%s",
				username, context, r, string(buf[:n]))

			respondWithError(s, i, "An internal error occurred")
		}
	}()

	// Check if command is allowed in current context
	commandName := i.ApplicationCommandData().Name
	if i.GuildID == "" && !dmAllowedCommands[commandName] {
		respondWithError(s, i, fmt.Sprintf("The `/%s` command can only be used in a server", commandName))
		return
	}

	// Add initial acknowledgment for long-running commands
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf(formatLogMessage(i.GuildID, "Error acknowledging interaction: "+err.Error(), "", ""))
		return
	}

	// Handle the command
	switch commandName {
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
	default:
		log.Printf(formatLogMessage(i.GuildID, "Unknown command: "+commandName, "", ""))
		respondWithError(s, i, "Unknown command")
	}
}
