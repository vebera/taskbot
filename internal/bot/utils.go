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
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Error: " + errMsg,
		},
	})
}

// getUserFromInteraction gets or creates a user from the interaction
func (b *Bot) getUserFromInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) (*models.User, error) {
	user, err := b.db.GetOrCreateUser(i.Member.User.ID, i.Member.User.Username)
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
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
		},
	})
}

// logCommand logs command execution to console and sends to the server
func logCommand(s *discordgo.Session, i *discordgo.InteractionCreate, commandName string, details ...string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	username := i.Member.User.Username
	logMessage := fmt.Sprintf("[%s] %s executed /%s", timestamp, username, commandName)
	if len(details) > 0 {
		logMessage += fmt.Sprintf(" (%s)", strings.Join(details, " "))
	}

	// Log to console
	log.Println(logMessage)

	// Send log to Discord server
	sendServerLog(s, i.ChannelID, logMessage)
}

// logError logs errors to both console and Discord server
func logError(s *discordgo.Session, channelID string, errContext, errMsg string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logMessage := fmt.Sprintf("[%s] ERROR - %s: %s", timestamp, errContext, errMsg)

	// Log to console
	log.Println(logMessage)

	// Send log to Discord server
	sendServerLog(s, channelID, logMessage)
}

// sendServerLog sends a log message to the Discord server
func sendServerLog(s *discordgo.Session, channelID string, message string) {
	_, err := s.ChannelMessageSend(channelID, fmt.Sprintf("`%s`", message))
	if err != nil {
		log.Printf("Error sending log to Discord: %v", err)
	}
}
