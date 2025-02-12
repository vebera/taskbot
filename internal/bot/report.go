package bot

import (
	"bytes"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"taskbot/internal/db/models"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"
)

func (b *Bot) handleReport(s *discordgo.Session, i *discordgo.InteractionCreate) {
	logCommand(s, i, "report")

	// Ensure we're in a guild
	if i.GuildID == "" {
		respondWithError(s, i, "This command must be used in a server")
		return
	}

	// Add permission check
	if !hasPermission(s, i.GuildID, i.Member.User.ID, discordgo.PermissionViewChannel) {
		respondWithError(s, i, "You don't have permission to use this command here")
		return
	}

	period := i.ApplicationCommandData().Options[0].StringValue()
	format := "text"     // default format
	filterUsername := "" // default to no filter

	// Get format and username filter if provided
	for _, opt := range i.ApplicationCommandData().Options {
		switch opt.Name {
		case "format":
			format = opt.StringValue()
		case "username":
			filterUsername = opt.StringValue()
		}
	}

	// Get user ID safely
	var userID string
	if i.Member != nil && i.Member.User != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	} else {
		respondWithError(s, i, "Could not determine user information")
		return
	}

	// Check if user is admin when requesting CSV
	isUserAdmin := isAdmin(s, i.GuildID, userID)
	if format == "csv" && !isUserAdmin {
		log.Printf("CSV access denied for user %s in guild %s", userID, i.GuildID)
		respondWithError(s, i, "CSV format is only available for administrators")
		return
	}

	now := time.Now()
	var startDate time.Time

	// Use a default timezone or retrieve from interaction
	loc, err := time.LoadLocation("UTC") // Default to UTC
	if err != nil {
		respondWithError(s, i, "Error loading default timezone: "+err.Error())
		return
	}

	now = now.In(loc)
	switch period {
	case "today":
		startDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	case "week":
		startDate = now.AddDate(0, 0, -7)
	case "month":
		startDate = now.AddDate(0, -1, 0)
	case "last_month":
		startDate = time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, loc)
		now = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).Add(-time.Second)
	case "month_2":
		startDate = time.Date(now.Year(), now.Month()-2, 1, 0, 0, 0, 0, loc)
		now = time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, loc).Add(-time.Second)
	case "month_3":
		startDate = time.Date(now.Year(), now.Month()-3, 1, 0, 0, 0, 0, loc)
		now = time.Date(now.Year(), now.Month()-2, 1, 0, 0, 0, 0, loc).Add(-time.Second)
	case "month_4":
		startDate = time.Date(now.Year(), now.Month()-4, 1, 0, 0, 0, 0, loc)
		now = time.Date(now.Year(), now.Month()-3, 1, 0, 0, 0, 0, loc).Add(-time.Second)
	case "month_5":
		startDate = time.Date(now.Year(), now.Month()-5, 1, 0, 0, 0, 0, loc)
		now = time.Date(now.Year(), now.Month()-4, 1, 0, 0, 0, 0, loc).Add(-time.Second)
	case "month_6":
		startDate = time.Date(now.Year(), now.Month()-6, 1, 0, 0, 0, 0, loc)
		now = time.Date(now.Year(), now.Month()-5, 1, 0, 0, 0, 0, loc).Add(-time.Second)
	default:
		respondWithError(s, i, "Invalid time period")
		return
	}

	// Get all task history for this server
	history, err := b.db.GetAllTaskHistory(i.GuildID, startDate, now)
	if err != nil {
		respondWithError(s, i, "Error retrieving task history: "+err.Error())
		return
	}

	// Then add user aggregation:
	userHours := make(map[string]time.Duration)
	userTasks := make(map[string]map[uuid.UUID]time.Duration) // Track time per task for each user
	taskNames := make(map[uuid.UUID]string)                   // Map to store task names
	userIDs := make(map[string]uuid.UUID)                     // Map Discord IDs to UUIDs

	for _, ci := range history {
		if ci.CheckIn.EndTime != nil {
			duration := ci.CheckIn.EndTime.Sub(ci.CheckIn.StartTime)
			userHours[ci.CheckIn.UserID.String()] += duration

			// Track individual task times
			if userTasks[ci.CheckIn.UserID.String()] == nil {
				userTasks[ci.CheckIn.UserID.String()] = make(map[uuid.UUID]time.Duration)
			}
			userTasks[ci.CheckIn.UserID.String()][ci.CheckIn.TaskID] += duration

			// Store task name
			taskNames[ci.CheckIn.TaskID] = ci.Task.Name
		}
	}

	// Get users for THIS guild only
	allUsers, err := b.db.GetGuildUsers(i.GuildID)
	if err != nil {
		respondWithError(s, i, "Error retrieving users: "+err.Error())
		return
	}

	// Create a map for quick lookup
	userMap := make(map[uuid.UUID]*models.User)
	for _, user := range allUsers {
		userMap[user.ID] = user
		userIDs[user.DiscordID] = user.ID
	}

	// Build report including all users
	var reportRows [][]string
	if filterUsername != "" {
		// Single user report - show task breakdown
		for userID, taskDurations := range userTasks {
			uid, _ := uuid.Parse(userID)
			user, exists := userMap[uid]
			if !exists || user.DiscordID != filterUsername {
				continue
			}

			// Add a row for each task
			for taskID, duration := range taskDurations {
				taskName := taskNames[taskID]
				reportRows = append(reportRows, []string{
					user.Username,
					taskName,
					formatDuration(duration),
				})
			}
		}
	} else {
		// All users report - show task breakdown for everyone
		for userID, taskDurations := range userTasks {
			uid, _ := uuid.Parse(userID)
			if user, exists := userMap[uid]; exists {
				// Add a row for each task
				for taskID, duration := range taskDurations {
					taskName := taskNames[taskID]
					reportRows = append(reportRows, []string{
						user.Username,
						taskName,
						formatDuration(duration),
					})
				}
				delete(userMap, uid) // Remove tracked users
			}
		}

		// Add users with 0 hours
		for _, user := range userMap {
			reportRows = append(reportRows, []string{
				user.Username,
				"No tasks",
				"0h 0m",
			})
		}
	}

	// Sort rows
	sort.Slice(reportRows, func(i, j int) bool {
		if reportRows[i][0] != reportRows[j][0] {
			return reportRows[i][0] < reportRows[j][0]
		}
		return reportRows[i][1] < reportRows[j][1]
	})

	// Prepare the report title based on whether it's filtered
	reportTitle := fmt.Sprintf("Task history for %s", period)
	if filterUsername != "" {
		if user, exists := userMap[userIDs[filterUsername]]; exists {
			reportTitle = fmt.Sprintf("Task history for %s - %s", user.Username, period)
		}
	}

	if format == "csv" {
		// Create CSV content
		var csvContent strings.Builder
		if filterUsername != "" {
			csvContent.WriteString("User,Task,Duration\n")
		} else {
			csvContent.WriteString("User,Total Duration,Task Count\n")
		}

		for _, row := range reportRows {
			csvContent.WriteString(fmt.Sprintf("%s,%s,%s\n", row[0], row[1], row[2]))
		}

		// Create and send file
		file := &discordgo.File{
			Name:        fmt.Sprintf("task_report_%s.csv", period),
			ContentType: "text/csv",
			Reader:      bytes.NewReader([]byte(csvContent.String())),
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Files: []*discordgo.File{file},
				Flags: discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Original text format response
	var response strings.Builder
	response.WriteString(fmt.Sprintf("# %s\n\n", reportTitle))
	response.WriteString("```\n")

	// Write header
	if filterUsername != "" {
		response.WriteString(fmt.Sprintf("%-20s %-30s %-15s\n", "USER", "TASK", "DURATION"))
	} else {
		response.WriteString(fmt.Sprintf("%-20s %-15s %-10s\n", "USER", "TOTAL TIME", "TASKS"))
	}
	response.WriteString(strings.Repeat("-", 79) + "\n")

	// Format each user's tasks
	for _, row := range reportRows {
		if filterUsername != "" {
			response.WriteString(fmt.Sprintf("%-20s %-30s %-15s\n",
				truncateString(row[0], 20),
				truncateString(row[1], 30),
				row[2],
			))
		} else {
			response.WriteString(fmt.Sprintf("%-20s %-15s %-10s\n",
				truncateString(row[0], 20),
				row[1],
				row[2],
			))
		}
	}

	response.WriteString("```")
	respondWithSuccess(s, i, response.String())
}
