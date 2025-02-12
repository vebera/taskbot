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
		"help": true, // Keep only essential commands in DMs
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

	// Update these intents
	session.Identify.Intents = discordgo.IntentsAllWithoutPrivileged |
		discordgo.IntentsGuildMembers |
		discordgo.IntentsGuildPresences |
		discordgo.IntentsMessageContent |
		discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages

	// Required permissions for visibility
	requiredPermissions := int64(
		discordgo.PermissionViewChannel |
			discordgo.PermissionSendMessages |
			discordgo.PermissionReadMessageHistory |
			discordgo.PermissionUseSlashCommands)

	config.Discord.Permissions = requiredPermissions

	// Log configuration details
	log.Printf("Bot intents: %d", session.Identify.Intents)
	log.Printf("Bot permissions: %d", config.Discord.Permissions)

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
	maxRetries := 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		err := b.registerGuildCommandsOnce(guildID)
		if err == nil {
			return nil
		}
		lastErr = err
		log.Printf("Attempt %d to register commands failed: %v", i+1, err)
		time.Sleep(time.Second * time.Duration(i+1))
	}
	return fmt.Errorf("failed to register commands after %d attempts: %v", maxRetries, lastErr)
}

func (b *Bot) registerGuildCommandsOnce(guildID string) error {
	serverName := getServerName(b.session, guildID)

	log.Printf(formatLogMessage(
		guildID,
		"Registering commands",
		"BOT",
		serverName,
	))

	// Clear existing commands
	existing, err := b.session.ApplicationCommands(b.config.Discord.ClientID, guildID)
	if err != nil {
		return fmt.Errorf("error getting existing commands: %w", err)
	}

	// Delete all existing commands first
	for _, v := range existing {
		err := b.session.ApplicationCommandDelete(b.config.Discord.ClientID, guildID, v.ID)
		if err != nil {
			log.Printf(formatLogMessage(
				guildID,
				fmt.Sprintf("%s: Failed to delete command (%v)", v.Name, err),
				"BOT",
				serverName,
			))
		} else {
			log.Printf(formatLogMessage(
				guildID,
				fmt.Sprintf("%s: Successfully removed command", v.Name),
				"BOT",
				serverName,
			))
		}
	}

	// Wait a moment to ensure all deletions are processed
	time.Sleep(time.Second)

	// Register new commands
	for _, v := range commands {
		_, err := b.session.ApplicationCommandCreate(b.config.Discord.ClientID, guildID, v)
		if err != nil {
			return fmt.Errorf("error creating command %s: %w", v.Name, err)
		}
		log.Printf(formatLogMessage(
			guildID,
			fmt.Sprintf("%s: Registered command", v.Name),
			"BOT",
			serverName,
		))
	}

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
		if err := b.registerGuildCommands(guild.ID); err != nil {
			log.Printf("Error registering commands for guild %s: %v", guild.ID, err)
		}
	}

	// Now add the guild create handler for future guilds
	b.session.AddHandler(b.handleGuildCreate)

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
	log.Printf(formatLogMessage("", "Removing Discord commands", "BOT", ""))

	for _, guild := range b.session.State.Guilds {
		// Get guild info for better logging
		serverName := getServerName(b.session, guild.ID)

		log.Printf(formatLogMessage(guild.ID, "Removing commands", "BOT", serverName))

		registeredCommands, err := b.session.ApplicationCommands(b.config.Discord.ClientID, guild.ID)
		if err != nil {
			log.Printf(formatLogMessage(guild.ID, fmt.Sprintf("Error getting commands: %v", err), "BOT", serverName))
			continue
		}
		for _, cmd := range registeredCommands {
			err := b.session.ApplicationCommandDelete(b.config.Discord.ClientID, guild.ID, cmd.ID)
			if err != nil {
				log.Printf(formatLogMessage(guild.ID, fmt.Sprintf("%s: Failed to remove command (%v)", cmd.Name, err), "BOT", serverName))
			} else {
				log.Printf(formatLogMessage(guild.ID, fmt.Sprintf("%s: Successfully removed command", cmd.Name), "BOT", serverName))
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
	log.Printf(formatLogMessage(g.ID, "Bot joined new guild", "BOT", g.Name))

	// Initialize settings for new guild
	if _, err := b.db.GetOrCreateServerSettings(g.ID); err != nil {
		log.Printf(formatLogMessage(g.ID, fmt.Sprintf("Error initializing settings: %v", err), "BOT", g.Name))
	} else {
		log.Printf(formatLogMessage(g.ID, "Successfully initialized settings", "BOT", g.Name))
	}

	// Register commands for the new guild
	if err := b.registerGuildCommands(g.ID); err != nil {
		log.Printf(formatLogMessage(g.ID, fmt.Sprintf("Error registering commands: %v", err), "BOT", g.Name))
	} else {
		log.Printf(formatLogMessage(g.ID, "Successfully registered all commands", "BOT", g.Name))
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

	// Strict DM check
	if i.GuildID == "" {
		if !dmAllowedCommands[commandName] {
			respondWithError(s, i, fmt.Sprintf("The `/%s` command can only be used in a server", commandName))
			return
		}
	}

	// Add guild-specific permission check
	if i.GuildID != "" {
		if !hasPermission(s, i.GuildID, i.Member.User.ID, discordgo.PermissionViewChannel) {
			respondWithError(s, i, "You don't have permission to use this command here")
			return
		}
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
