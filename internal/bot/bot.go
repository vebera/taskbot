package bot

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
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

	// Register commands
	log.Println("Starting command registration...")
	log.Printf("Registering commands for Guild ID: %s", b.config.Discord.GuildID)
	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))

	// Sort commands by complexity (simpler commands first)
	sortedCommands := make([]*discordgo.ApplicationCommand, len(commands))
	copy(sortedCommands, commands)
	sort.Slice(sortedCommands, func(i, j int) bool {
		simpleCommands := map[string]bool{
			"timezone": true,
			"checkin":  true,
			"checkout": true,
		}
		return simpleCommands[sortedCommands[i].Name] && !simpleCommands[sortedCommands[j].Name]
	})

	// Log command registration order
	log.Println("Command registration order:")
	for i, cmd := range sortedCommands {
		log.Printf("%d. %s", i+1, cmd.Name)
	}

	for i, cmd := range sortedCommands {
		cmdCopy := cmd // Create a copy for the goroutine

		// Keep trying until the command is registered successfully
		for {
			log.Printf("Attempting to register command: %s", cmdCopy.Name)

			// Ensure session is connected
			if b.session.State.SessionID == "" {
				log.Printf("Session disconnected, reconnecting...")
				if err := b.session.Open(); err != nil {
					log.Printf("Failed to reconnect: %v. Retrying in 5 seconds...", err)
					time.Sleep(5 * time.Second)
					continue
				}
			}

			// Create context with timeout
			timeout := 30 * time.Second
			if cmdCopy.Name == "status" || cmdCopy.Name == "report" {
				timeout = 60 * time.Second
			}
			cmdCtx, cancel := context.WithTimeout(ctx, timeout)

			// Create channel for result
			resultCh := make(chan struct {
				cmd *discordgo.ApplicationCommand
				err error
			})

			// Launch command registration in goroutine
			go func() {
				start := time.Now()
				reg, regErr := b.session.ApplicationCommandCreate(b.config.Discord.ClientID, b.config.Discord.GuildID, cmdCopy)
				elapsed := time.Since(start)
				log.Printf("Command registration attempt took %v", elapsed)

				if regErr != nil {
					log.Printf("API Error details for %s: %v", cmdCopy.Name, regErr)
					if restErr, ok := regErr.(*discordgo.RESTError); ok {
						log.Printf("REST Error details: Code: %d, Message: %s", restErr.Response.StatusCode, restErr.Message)
						log.Printf("Response headers: %+v", restErr.Response.Header)
					}
				}
				select {
				case resultCh <- struct {
					cmd *discordgo.ApplicationCommand
					err error
				}{reg, regErr}:
				case <-cmdCtx.Done():
					log.Printf("Command registration goroutine cancelled after %v", elapsed)
				}
			}()

			// Wait for result or timeout
			select {
			case result := <-resultCh:
				cancel() // Cancel the context
				if result.err == nil {
					registered := result.cmd
					log.Printf("Successfully registered command: %s (ID: %s)", cmdCopy.Name, registered.ID)
					registeredCommands[i] = registered
					goto nextCommand
				}

				// Handle rate limits
				if restErr, ok := result.err.(*discordgo.RESTError); ok && restErr.Response.StatusCode == 429 {
					waitTime := 10 * time.Second
					if retryAfter, ok := restErr.Response.Header["Retry-After"]; ok && len(retryAfter) > 0 {
						if retrySeconds, parseErr := strconv.Atoi(retryAfter[0]); parseErr == nil {
							waitTime = time.Duration(retrySeconds) * time.Second
						}
					}
					log.Printf("Rate limited. Waiting %s before retry...", waitTime)
					time.Sleep(waitTime)
					continue
				}

				// For other errors, wait a bit and retry
				log.Printf("Command registration failed: %v. Retrying in 5 seconds...", result.err)
				time.Sleep(5 * time.Second)

			case <-cmdCtx.Done():
				cancel() // Cancel the context
				log.Printf("Command registration timed out after %d seconds. Retrying...", timeout/time.Second)
				time.Sleep(5 * time.Second)
			}
		}

	nextCommand:
		// Add delay before next command
		if i < len(sortedCommands)-1 {
			isNextCommandComplex := sortedCommands[i+1].Name == "status" || sortedCommands[i+1].Name == "report"
			delay := 5 * time.Second
			if isNextCommandComplex {
				delay = 10 * time.Second
			}
			log.Printf("Waiting %s before next command...", delay)
			time.Sleep(delay)
		}
	}

	b.commands = registeredCommands
	log.Printf("All %d commands registered successfully", len(commands))

	// Register command handlers
	log.Println("Registering command handlers...")
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
				log.Printf("Handling command: %s", i.ApplicationCommandData().Name)
				h(s, i)
			}
		case discordgo.InteractionApplicationCommandAutocomplete:
			if i.ApplicationCommandData().Name == "checkin" || i.ApplicationCommandData().Name == "task" {
				log.Printf("Handling autocomplete for command: %s", i.ApplicationCommandData().Name)
				b.handleAutocomplete(s, i)
			}
		}
	})
	log.Println("Command handlers registered successfully")

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
