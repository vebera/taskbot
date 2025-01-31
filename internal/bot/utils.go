package bot

import (
	"fmt"
	"log"
	"strings"
	"time"

	"taskbot/internal/db/models"

	"github.com/bwmarrin/discordgo"
)

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// respondWithError sends an error response to the user
func respondWithError(s *discordgo.Session, i *discordgo.InteractionCreate, errMsg string) {
	var username string
	if i.Member != nil && i.Member.User != nil {
		username = i.Member.User.Username
	} else if i.User != nil {
		username = i.User.Username
	}

	serverName := "DM"
	if i.GuildID != "" {
		if guild, err := s.Guild(i.GuildID); err == nil {
			serverName = guild.Name
		}
	}

	log.Printf(formatLogMessage(i.GuildID, "Error: "+errMsg, username, serverName))

	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: "Error: " + errMsg,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
	if err != nil {
		log.Printf(formatLogMessage(i.GuildID, "Error sending error response: "+err.Error(), username, serverName))
	}
}

// getUserFromInteraction gets or creates a user from the interaction
func (b *Bot) getUserFromInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) (*models.User, error) {
	var userID, username string
	if i.Member != nil && i.Member.User != nil {
		// Server interaction
		userID = i.Member.User.ID
		username = i.Member.User.Username
	} else if i.User != nil {
		// DM interaction
		userID = i.User.ID
		username = i.User.Username
	} else {
		err := fmt.Errorf("could not get user information from interaction")
		respondWithError(s, i, err.Error())
		return nil, err
	}

	user, err := b.db.GetOrCreateUser(userID, username)
	if err != nil {
		respondWithError(s, i, "Error getting user: "+err.Error())
		return nil, err
	}
	return user, nil
}

// formatTime formats a time using the user's timezone
func formatTime(t time.Time, timezone string) string {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return t.Format("2006-01-02 15:04:05")
	}
	return t.In(loc).Format("2006-01-02 15:04:05")
}

// respondWithSuccess sends a success response to the user
func respondWithSuccess(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: msg,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
	if err != nil {
		log.Printf("Error sending success response: %v", err)
	}
}

// Add this helper function for consistent log formatting
func formatLogMessage(guildID, message, username, serverName string) string {
	if serverName == "" {
		serverName = "unknown"
	}
	if username == "" {
		username = "system"
	}
	return fmt.Sprintf("%s: %s (username: %s, server: %s)",
		guildID,
		message,
		username,
		serverName,
	)
}

// Update logCommand to use the new format
func logCommand(s *discordgo.Session, i *discordgo.InteractionCreate, commandName string, details ...string) {
	var username string
	if i.Member != nil && i.Member.User != nil {
		username = i.Member.User.Username
	} else if i.User != nil {
		username = i.User.Username
	}

	// Get server name
	serverName := "DM"
	if i.GuildID != "" {
		if guild, err := s.Guild(i.GuildID); err == nil {
			serverName = guild.Name
		}
	}

	// Build command parameters string
	var params []string
	if options := i.ApplicationCommandData().Options; len(options) > 0 {
		for _, opt := range options {
			switch opt.Type {
			case discordgo.ApplicationCommandOptionSubCommand:
				params = append(params, fmt.Sprintf("%s", opt.Name))
				for _, subOpt := range opt.Options {
					params = append(params, fmt.Sprintf("%s:%s", subOpt.Name, subOpt.StringValue()))
				}
			case discordgo.ApplicationCommandOptionString:
				params = append(params, fmt.Sprintf("%s:%s", opt.Name, opt.StringValue()))
			}
		}
	}

	message := fmt.Sprintf("executed /%s", commandName)
	if len(params) > 0 {
		message += fmt.Sprintf(" [%s]", strings.Join(params, ", "))
	}
	if len(details) > 0 {
		message += fmt.Sprintf(" (%s)", strings.Join(details, " "))
	}

	log.Printf(formatLogMessage(i.GuildID, message, username, serverName))
}

// Update logError to use the new format
func logError(s *discordgo.Session, channelID string, errContext, errMsg string) {
	guildID := "unknown"
	serverName := "unknown"

	// Try to get channel info to get guild ID
	if channel, err := s.Channel(channelID); err == nil {
		guildID = channel.GuildID
		if guild, err := s.Guild(channel.GuildID); err == nil {
			serverName = guild.Name
		}
	}

	message := fmt.Sprintf("ERROR - %s: %s", errContext, errMsg)
	log.Printf(formatLogMessage(guildID, message, "", serverName))
}

// sendServerLog sends a log message to the Discord server
func sendServerLog(s *discordgo.Session, channelID string, message string) {
	_, err := s.ChannelMessageSend(channelID, fmt.Sprintf("`%s`", message))
	if err != nil {
		log.Printf("Error sending log to Discord: %v", err)
	}
}

// formatTable creates a Discord-friendly table with fixed-width columns
func formatTable(headers []string, rows [][]string) string {
	// Find the maximum width for each column
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}

	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	var result strings.Builder

	// Write headers
	result.WriteString("```\n")
	for i, header := range headers {
		result.WriteString(fmt.Sprintf("%-*s", widths[i]+2, header))
	}
	result.WriteString("\n")

	// Write separator
	for _, width := range widths {
		result.WriteString(strings.Repeat("-", width+2))
	}
	result.WriteString("\n")

	// Write rows
	for _, row := range rows {
		for i, cell := range row {
			result.WriteString(fmt.Sprintf("%-*s", widths[i]+2, cell))
		}
		result.WriteString("\n")
	}
	result.WriteString("```")

	return result.String()
}

// Add this new function to handle permission checks
func hasPermission(s *discordgo.Session, guildID string, userID string, requiredPermission int64) bool {
	// If this is a DM channel (no guild), allow the action
	if guildID == "" {
		return true
	}

	// Just check if the user is a member of the guild
	_, err := s.GuildMember(guildID, userID)
	if err != nil {
		log.Printf("Error checking guild membership for user %s in guild %s: %v", userID, guildID, err)
		return false
	}

	// If we can get the member, they have permission to use the bot
	return true
}
