package bot

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"taskbot/internal/db/models"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"
)

var (
	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "timezone",
			Description: "Set your timezone",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "zone",
					Description: "Timezone (e.g., America/New_York, Europe/London)",
					Required:    true,
				},
			},
		},
		{
			Name:        "declare",
			Description: "Declare time spent on a task",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:         discordgo.ApplicationCommandOptionString,
					Name:         "task",
					Description:  "Select a task",
					Required:     true,
					Autocomplete: true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "time",
					Description: "Time spent (format: hh:mm)",
					Required:    true,
				},
			},
		},
		{
			Name:        "checkin",
			Description: "Start working on a task",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "existing",
					Description: "Check in to an existing task",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:         discordgo.ApplicationCommandOptionString,
							Name:         "task",
							Description:  "Select a task",
							Required:     true,
							Autocomplete: true,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "new",
					Description: "Create and check in to a new task",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "name",
							Description: "Task name",
							Required:    true,
						},
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "description",
							Description: "Task description",
							Required:    false,
						},
					},
				},
			},
		},
		{
			Name:        "checkout",
			Description: "Stop working on the current task",
		},
		{
			Name:        "status",
			Description: "Show current task status for all users",
		},
		{
			Name:        "report",
			Description: "Show task history",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "period",
					Description: "Time period",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{
							Name:  "Today",
							Value: "today",
						},
						{
							Name:  "This Week",
							Value: "week",
						},
						{
							Name:  "This Month",
							Value: "month",
						},
						{
							Name:  "Last Month",
							Value: "last_month",
						},
						{
							Name:  "2 Months Ago",
							Value: "month_2",
						},
						{
							Name:  "3 Months Ago",
							Value: "month_3",
						},
						{
							Name:  "4 Months Ago",
							Value: "month_4",
						},
						{
							Name:  "5 Months Ago",
							Value: "month_5",
						},
						{
							Name:  "6 Months Ago",
							Value: "month_6",
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "format",
					Description: "Output format (CSV available for admins only)",
					Required:    false,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{
							Name:  "Text",
							Value: "text",
						},
						{
							Name:  "CSV",
							Value: "csv",
						},
					},
				},
				{
					Type:         discordgo.ApplicationCommandOptionString,
					Name:         "username",
					Description:  "Filter by username",
					Required:     false,
					Autocomplete: true,
				},
			},
		},
		{
			Name:        "task",
			Description: "Update task status",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:         discordgo.ApplicationCommandOptionString,
					Name:         "task",
					Description:  "Select a task",
					Required:     true,
					Autocomplete: true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "status",
					Description: "New task status",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{
							Name:  "Open",
							Value: "open",
						},
						{
							Name:  "Completed",
							Value: "completed",
						},
					},
				},
			},
		},
		{
			Name:                     "globaltask",
			Description:              "Create a global task visible to everyone (admin only)",
			DefaultMemberPermissions: &adminPermission,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "name",
					Description: "Task name",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "description",
					Description: "Task description",
					Required:    false,
				},
			},
		},
	}

	// Permission for admin commands (Manage Server permission)
	adminPermission = int64(discordgo.PermissionManageServer)
)

func (b *Bot) handleAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.ApplicationCommandData().Name {
	case "checkin", "task", "declare":
		b.handleTaskAutocomplete(s, i)
	case "report":
		b.handleUsernameAutocomplete(s, i)
	}
}

func (b *Bot) handleTaskAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Get the user's tasks for autocomplete
	user, err := b.getUserFromInteraction(s, i)
	if err != nil || user == nil {
		log.Printf("Error getting user from interaction: %v", err)
		return
	}

	// Check if user is admin
	if i == nil || i.Member == nil || i.Member.User == nil {
		log.Printf("Interaction or member is nil")
		return
	}
	isUserAdmin := isAdmin(s, i.GuildID, i.Member.User.ID)

	// Get active check-in to filter out active task
	var activeTaskID *uuid.UUID
	if i.ApplicationCommandData().Name == "checkin" {
		activeCheckIn, err := b.db.GetActiveCheckIn(user.ID, i.GuildID)
		if err != nil {
			log.Printf("Error getting active check-in: %v", err)
			return
		}
		if activeCheckIn != nil {
			activeTaskID = &activeCheckIn.TaskID
		}
	}

	tasks, err := b.db.GetUserTasks(user.ID, i.GuildID)
	if err != nil {
		log.Printf("Error getting tasks for autocomplete: %v", err)
		return
	}

	// Get the current input value
	var focusedOption *discordgo.ApplicationCommandInteractionDataOption
	if i.ApplicationCommandData().Name == "checkin" {
		// For checkin command, the task option is nested under the "existing" subcommand
		if len(i.ApplicationCommandData().Options) > 0 && len(i.ApplicationCommandData().Options[0].Options) > 0 {
			focusedOption = i.ApplicationCommandData().Options[0].Options[0]
		}
	} else {
		// For task and declare commands, the task option is directly in the options
		if len(i.ApplicationCommandData().Options) > 0 {
			focusedOption = i.ApplicationCommandData().Options[0]
		}
	}

	if focusedOption == nil {
		return
	}

	input := strings.ToLower(focusedOption.StringValue())

	// Filter and create choices
	var choices []*discordgo.ApplicationCommandOptionChoice
	for _, task := range tasks {
		// For checkin command:
		// - Skip completed tasks
		// - Skip currently active task
		if i.ApplicationCommandData().Name == "checkin" {
			if task.Completed {
				continue
			}
			if activeTaskID != nil && task.ID == *activeTaskID {
				continue
			}
		}

		// For task command, show all tasks that user owns or global tasks if admin
		if i.ApplicationCommandData().Name == "task" {
			if !isUserAdmin && task.Global && task.UserID != user.ID {
				continue // Skip global tasks for non-admins unless they created them
			}
		}

		if strings.Contains(strings.ToLower(task.Name), input) {
			// Add task status to the name for /task command
			displayName := task.Name
			if i.ApplicationCommandData().Name == "task" {
				if task.Global {
					displayName = fmt.Sprintf("%s [Global]", task.Name)
				}
				if task.Completed {
					displayName = fmt.Sprintf("%s (Completed)", displayName)
				}
			}

			choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
				Name:  displayName,
				Value: task.ID.String(),
			})
		}
		if len(choices) >= 25 { // Discord limit
			break
		}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{
			Choices: choices,
		},
	})
}

func (b *Bot) handleUsernameAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Get all users who have any activity
	users, err := b.db.GetAllUsers()
	if err != nil {
		logError(s, i.ChannelID, "GetAllUsers", err.Error())
		return
	}

	// Get the current input value
	var focusedOption *discordgo.ApplicationCommandInteractionDataOption
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "username" && opt.Focused {
			focusedOption = opt
			break
		}
	}

	if focusedOption == nil {
		return
	}

	input := strings.ToLower(focusedOption.StringValue())

	// Filter and create choices
	var choices []*discordgo.ApplicationCommandOptionChoice
	for _, user := range users {
		if strings.Contains(strings.ToLower(user.Username), strings.ToLower(input)) {
			choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
				Name:  user.Username,
				Value: user.DiscordID,
			})
		}
		if len(choices) >= 25 {
			break
		}
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{
			Choices: choices,
		},
	})
	if err != nil {
		log.Printf("Error responding to autocomplete: %v", err)
	}
}

func (b *Bot) handleCheckin(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Validate interaction data
	if i.ApplicationCommandData().Options == nil || len(i.ApplicationCommandData().Options) == 0 {
		respondWithError(s, i, "Invalid command options")
		return
	}

	// Get user information, handling both DM and guild contexts
	var userID, username string
	if i.Member != nil && i.Member.User != nil {
		userID = i.Member.User.ID
		username = i.Member.User.Username
	} else if i.User != nil {
		userID = i.User.ID
		username = i.User.Username
	} else {
		respondWithError(s, i, "Could not determine user information")
		return
	}

	// Log the interaction context
	log.Printf("Processing command for user %s (ID: %s) in context: %s",
		username,
		userID,
		map[bool]string{true: "DM", false: "Guild"}[i.Member == nil])

	subcommand := i.ApplicationCommandData().Options[0]
	if subcommand == nil {
		respondWithError(s, i, "Invalid subcommand")
		return
	}

	// Validate member data
	if i.Member == nil || i.Member.User == nil {
		respondWithError(s, i, "Could not determine user information")
		return
	}

	options := subcommand.Options
	if options == nil {
		respondWithError(s, i, "Missing required options")
		return
	}

	var task *models.Task
	var err error

	user, err := b.getUserFromInteraction(s, i)
	if err != nil || user == nil {
		log.Printf("Error getting user from interaction: %v", err)
		respondWithError(s, i, "Could not get user information")
		return
	}

	switch subcommand.Name {
	case "existing":
		if len(options) == 0 {
			respondWithError(s, i, "Missing task ID")
			return
		}

		taskID, err := uuid.Parse(options[0].StringValue())
		if err != nil {
			respondWithError(s, i, "Invalid task ID")
			return
		}

		task, err = b.db.GetTaskByID(taskID)
		if err != nil {
			respondWithError(s, i, "Error getting task: "+err.Error())
			return
		}
		if task == nil {
			respondWithError(s, i, "Task not found")
			return
		}

	case "new":
		if len(options) == 0 {
			respondWithError(s, i, "Missing task name")
			return
		}

		taskName := options[0].StringValue()
		var description string
		if len(options) > 1 {
			description = options[1].StringValue()
		}

		task = &models.Task{
			ID:          uuid.New(),
			UserID:      user.ID,
			ServerID:    i.GuildID,
			Name:        taskName,
			Description: description,
			CreatedAt:   time.Now(),
		}

		if err := b.db.CreateTask(task); err != nil {
			logError(s, i.ChannelID, "CreateTask", err.Error())
			respondWithError(s, i, "Error creating task: "+err.Error())
			return
		}
	default:
		respondWithError(s, i, "Invalid subcommand")
		return
	}

	logCommand(s, i, "checkin", task.Name)

	// Check for active check-in
	activeCheckIn, err := b.db.GetActiveCheckIn(user.ID, i.GuildID)
	if err != nil {
		logError(s, i.ChannelID, "GetActiveCheckIn", err.Error())
		respondWithError(s, i, "Error checking active tasks: "+err.Error())
		return
	}

	// If there's an active check-in, check out first
	if activeCheckIn != nil {
		if err := b.db.CheckOut(activeCheckIn.ID); err != nil {
			logError(s, i.ChannelID, "CheckOut", err.Error())
			respondWithError(s, i, "Error checking out from previous task: "+err.Error())
			return
		}
	}

	// Create check-in record
	checkIn := &models.CheckIn{
		ID:        uuid.New(),
		UserID:    user.ID,
		ServerID:  i.GuildID,
		TaskID:    task.ID,
		StartTime: time.Now(),
	}

	if err := b.db.CreateCheckIn(checkIn); err != nil {
		logError(s, i.ChannelID, "CreateCheckIn", err.Error())
		respondWithError(s, i, "Error creating check-in: "+err.Error())
		return
	}

	respondWithSuccess(s, i, fmt.Sprintf("Started working on task: %s", task.Name))
}

func (b *Bot) handleCheckout(s *discordgo.Session, i *discordgo.InteractionCreate) {
	logCommand(s, i, "checkout")

	user, err := b.getUserFromInteraction(s, i)
	if err != nil || user == nil {
		log.Printf("Error getting user from interaction: %v", err)
		return
	}

	// Get active check-in
	activeCheckIn, err := b.db.GetActiveCheckIn(user.ID, i.GuildID)
	if err != nil {
		logError(s, i.ChannelID, "GetActiveCheckIn", err.Error())
		respondWithError(s, i, "Error checking active tasks: "+err.Error())
		return
	}

	if activeCheckIn == nil {
		respondWithError(s, i, "No active task to check out from")
		return
	}

	// Get task details
	task, err := b.db.GetTaskByID(activeCheckIn.TaskID)
	if err != nil {
		logError(s, i.ChannelID, "GetTaskByID", err.Error())
		respondWithError(s, i, "Error retrieving task details: "+err.Error())
		return
	}

	// Check out
	if err := b.db.CheckOut(activeCheckIn.ID); err != nil {
		respondWithError(s, i, "Error checking out: "+err.Error())
		return
	}

	// Get the updated check-in to get the actual end time
	updatedCheckIn, err := b.db.GetCheckInByID(activeCheckIn.ID)
	if err != nil {
		respondWithError(s, i, "Error retrieving checkout details: "+err.Error())
		return
	}

	duration := updatedCheckIn.EndTime.Sub(updatedCheckIn.StartTime)
	respondWithSuccess(s, i, fmt.Sprintf("Checked out from task: %s\nTime spent: %s", task.Name, formatDuration(duration)))
}

func (b *Bot) handleStatus(s *discordgo.Session, i *discordgo.InteractionCreate) {
	logCommand(s, i, "status")

	// Get all active check-ins for THIS guild only
	activeCheckIns, err := b.db.GetAllActiveCheckIns(i.GuildID)
	if err != nil {
		respondWithError(s, i, "Error retrieving active check-ins: "+err.Error())
		return
	}

	var response strings.Builder
	response.WriteString("# Current Status\n\n")
	response.WriteString("```\n")
	response.WriteString(fmt.Sprintf("%-2s %-20s %-30s %-15s\n", "", "USER", "TASK", "TIME"))
	response.WriteString(strings.Repeat("-", 70) + "\n")

	for _, ci := range activeCheckIns {
		user, err := b.db.GetUserByID(ci.CheckIn.UserID)
		if err != nil {
			continue
		}

		var (
			statusIcon string
			duration   string
		)

		if ci.Task.Name == "Not checked in" {
			statusIcon = "⚫" // Grey circle for not checked in
			duration = "-"
		} else {
			statusIcon = "🟢" // Green circle for active
			duration = formatDuration(time.Since(ci.CheckIn.StartTime))
		}

		response.WriteString(fmt.Sprintf("%-2s %-20s %-30s %-15s\n",
			statusIcon,
			truncateString(user.Username, 20),
			truncateString(ci.Task.Name, 30),
			duration,
		))
	}

	response.WriteString("```\n")
	response.WriteString("\n🟢 Active  ⚫ Not checked in")

	respondWithSuccess(s, i, response.String())
}

func (b *Bot) handleReport(s *discordgo.Session, i *discordgo.InteractionCreate) {
	logCommand(s, i, "report")

	// Ensure we're in a guild
	if i.GuildID == "" {
		respondWithError(s, i, "This command must be used in a server")
		return
	}

	// Get users for THIS guild only
	allUsers, err := b.db.GetGuildUsers(i.GuildID) // You'll need to create this function
	if err != nil {
		respondWithError(s, i, "Error retrieving users: "+err.Error())
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
	for _, ci := range history {
		if ci.CheckIn.EndTime != nil {
			duration := ci.CheckIn.EndTime.Sub(ci.CheckIn.StartTime)
			userHours[ci.CheckIn.UserID.String()] += duration
		}
	}

	// Create a map for quick lookup
	userMap := make(map[uuid.UUID]*models.User)
	for _, user := range allUsers {
		userMap[user.ID] = user
	}

	// Build report including all users
	var reportRows [][]string
	for userID, duration := range userHours {
		uid, _ := uuid.Parse(userID)
		if user, exists := userMap[uid]; exists {
			reportRows = append(reportRows, []string{
				user.Username,
				formatDuration(duration),
				strconv.Itoa(len(history)),
			})
			delete(userMap, uid) // Remove tracked users
		}
	}

	// Add users with 0 hours
	for _, user := range userMap {
		reportRows = append(reportRows, []string{
			user.Username,
			"0h 0m",
			"0",
		})
	}

	if format == "csv" {
		// Create CSV content
		var csvContent strings.Builder
		csvContent.WriteString("User,Duration,Task Count\n")

		for _, row := range reportRows {
			csvContent.WriteString(fmt.Sprintf("%s,%s,%s\n", row[0], row[1], row[2]))
		}

		// Create a temporary file for the CSV
		tmpFile, err := os.CreateTemp("", "task_report_*.csv")
		if err != nil {
			respondWithError(s, i, "Error creating CSV file: "+err.Error())
			return
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(csvContent.String()); err != nil {
			respondWithError(s, i, "Error writing CSV file: "+err.Error())
			return
		}
		tmpFile.Close()

		// Send the file
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
	response.WriteString(fmt.Sprintf("# Task history for %s\n\n", period))
	response.WriteString("```\n")

	// Write header
	response.WriteString(fmt.Sprintf("%-20s %-30s %-15s %s\n", "USER", "TASK", "TIME", "STATUS"))
	response.WriteString(strings.Repeat("-", 79) + "\n")

	// Filter usernames if a specific username was requested
	if filterUsername != "" {
		filteredRows := make([][]string, 0)
		for _, row := range reportRows {
			if row[0] == filterUsername {
				filteredRows = append(filteredRows, row)
				break
			}
		}
		reportRows = filteredRows
	}

	// Format each user's tasks
	for _, row := range reportRows {
		response.WriteString(fmt.Sprintf("%-20s %-30s %-15s %s\n",
			truncateString(row[0], 20),
			truncateString(row[1], 30),
			row[2],
			row[1],
		))
	}

	response.WriteString("```")
	respondWithSuccess(s, i, response.String())
}

// Helper function to check if a user is an admin
func isAdmin(s *discordgo.Session, guildID string, userID string) bool {
	// If this is a DM channel (no guild), check if the user is a bot owner
	if guildID == "" {
		// In DMs, we consider the user an admin if they have admin permissions in any mutual guild
		guilds, err := s.UserGuilds(100, "", "")
		if err != nil {
			log.Printf("Error getting user guilds: %v", err)
			return false
		}

		for _, guild := range guilds {
			member, err := s.GuildMember(guild.ID, userID)
			if err != nil {
				continue
			}

			// Get guild to check roles
			g, err := s.Guild(guild.ID)
			if err != nil {
				continue
			}

			// Check if user is the guild owner
			if g.OwnerID == userID {
				log.Printf("User %s is the owner of guild %s", userID, guild.ID)
				return true
			}

			// Check roles for admin permissions
			for _, roleID := range member.Roles {
				for _, role := range g.Roles {
					if role.ID == roleID {
						if role.Permissions&discordgo.PermissionAdministrator != 0 || role.Permissions&discordgo.PermissionManageServer != 0 {
							return true
						}
						break
					}
				}
			}
		}
		return false
	}

	// For guild channels, check the guild roles
	member, err := s.GuildMember(guildID, userID)
	if err != nil {
		log.Printf("Error getting guild member: %v", err)
		return false
	}

	// Get guild to check roles
	guild, err := s.Guild(guildID)
	if err != nil {
		log.Printf("Error getting guild: %v", err)
		return false
	}

	// First check if user is the guild owner
	if guild.OwnerID == userID {
		log.Printf("User %s is the owner of guild %s", userID, guildID)
		return true
	}

	// Check each role the user has
	for _, roleID := range member.Roles {
		for _, role := range guild.Roles {
			if role.ID == roleID {
				// Log role details for debugging
				log.Printf("Checking role %s (ID: %s) with permissions: %d", role.Name, role.ID, role.Permissions)

				if role.Permissions&discordgo.PermissionAdministrator != 0 || role.Permissions&discordgo.PermissionManageServer != 0 {
					log.Printf("User %s is admin via role %s", userID, role.Name)
					return true
				}
				break
			}
		}
	}

	log.Printf("User %s is not an admin in guild %s", userID, guildID)
	return false
}

func (b *Bot) handleTask(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	taskID, err := uuid.Parse(options[0].StringValue())
	if err != nil {
		respondWithError(s, i, "Invalid task ID")
		return
	}

	newStatus := options[1].StringValue()
	completed := newStatus == "completed"

	// Get the user to verify ownership
	user, err := b.getUserFromInteraction(s, i)
	if err != nil || user == nil {
		log.Printf("Error getting user from interaction: %v", err)
		return
	}

	// Get the task
	task, err := b.db.GetTaskByID(taskID)
	if err != nil {
		respondWithError(s, i, "Error getting task: "+err.Error())
		return
	}
	if task == nil {
		respondWithError(s, i, "Task not found")
		return
	}

	logCommand(s, i, "task", fmt.Sprintf("task: %s, status: %s"))

	// Check if user is admin or task owner
	isUserAdmin := isAdmin(s, i.GuildID, i.Member.User.ID)
	if !isUserAdmin && task.UserID != user.ID {
		respondWithError(s, i, "You can only update your own tasks")
		return
	}

	// Check if task is currently active
	activeCheckIn, err := b.db.GetActiveCheckIn(user.ID, i.GuildID)
	if err != nil {
		respondWithError(s, i, "Error checking active tasks: "+err.Error())
		return
	}

	if activeCheckIn != nil && activeCheckIn.TaskID == taskID {
		respondWithError(s, i, "Cannot update status of an active task. Please checkout first.")
		return
	}

	// Update task status
	if err := b.db.UpdateTaskStatus(taskID, completed); err != nil {
		respondWithError(s, i, "Error updating task status: "+err.Error())
		return
	}

	statusText := "open"
	if completed {
		statusText = "completed"
	}

	// Add admin action note to the message if applicable
	message := fmt.Sprintf("Task '%s' marked as %s", task.Name, statusText)
	if isUserAdmin && task.UserID != user.ID {
		message += " (admin action)"
	}
	respondWithSuccess(s, i, message)
}

// Helper function to truncate strings that are too long
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s + strings.Repeat(" ", maxLen-len(s))
	}
	return s[:maxLen-3] + "..."
}

func (b *Bot) handleTimezone(s *discordgo.Session, i *discordgo.InteractionCreate) {
	logCommand(s, i, "timezone")

	timezone := i.ApplicationCommandData().Options[0].StringValue()

	// Validate timezone
	_, err := time.LoadLocation(timezone)
	if err != nil {
		respondWithError(s, i, "Invalid timezone. Please use IANA timezone names (e.g., America/New_York, Europe/London)")
		return
	}

	user, err := b.getUserFromInteraction(s, i)
	if err != nil || user == nil {
		log.Printf("Error getting user from interaction: %v", err)
		return
	}

	// Update timezone
	if err := b.db.UpdateUserTimezone(user.ID, timezone); err != nil {
		respondWithError(s, i, "Error updating timezone: "+err.Error())
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Timezone updated to %s", timezone),
		},
	})
}

func (b *Bot) handleGlobalTask(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	taskName := options[0].StringValue()
	var description string
	if len(options) > 1 {
		description = options[1].StringValue()
	}

	// Get the admin user
	user, err := b.getUserFromInteraction(s, i)
	if err != nil || user == nil {
		log.Printf("Error getting user from interaction: %v", err)
		return
	}

	// Create the global task
	task := &models.Task{
		ID:          uuid.New(),
		UserID:      user.ID,
		ServerID:    i.GuildID,
		Name:        taskName,
		Description: description,
		Global:      true,
		CreatedAt:   time.Now(),
	}

	logCommand(s, i, "globaltask", fmt.Sprintf("name: %s", taskName))

	if err := b.db.CreateTask(task); err != nil {
		logError(s, i.ChannelID, "CreateTask", err.Error())
		respondWithError(s, i, "Error creating global task: "+err.Error())
		return
	}

	respondWithSuccess(s, i, fmt.Sprintf("Created global task: %s", task.Name))
}

func (b *Bot) handleDeclare(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	taskID, err := uuid.Parse(options[0].StringValue())
	if err != nil {
		respondWithError(s, i, "Invalid task ID")
		return
	}

	timeStr := options[1].StringValue()
	// Parse time in format "hh:mm"
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		respondWithError(s, i, "Invalid time format. Please use hh:mm")
		return
	}

	hours, err := strconv.Atoi(parts[0])
	if err != nil || hours < 0 {
		respondWithError(s, i, "Invalid hours value")
		return
	}

	minutes, err := strconv.Atoi(parts[1])
	if err != nil || minutes < 0 || minutes >= 60 {
		respondWithError(s, i, "Invalid minutes value")
		return
	}

	duration := time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute

	// Get the user
	user, err := b.getUserFromInteraction(s, i)
	if err != nil || user == nil {
		log.Printf("Error getting user from interaction: %v", err)
		return
	}

	// Get the task
	task, err := b.db.GetTaskByID(taskID)
	if err != nil {
		respondWithError(s, i, "Error getting task: "+err.Error())
		return
	}
	if task == nil {
		respondWithError(s, i, "Task not found")
		return
	}

	// Log command with warning if over 8 hours
	if duration > 8*time.Hour {
		logCommand(s, i, "declare", fmt.Sprintf("⚠️ OVER 8 HOURS: %s on task: %s", formatDuration(duration), task.Name))
	} else {
		logCommand(s, i, "declare", fmt.Sprintf("%s on task: %s", formatDuration(duration), task.Name))
	}

	// Create check-in record with end time
	now := time.Now()
	startTime := now.Add(-duration)
	checkIn := &models.CheckIn{
		ID:        uuid.New(),
		UserID:    user.ID,
		ServerID:  i.GuildID,
		TaskID:    task.ID,
		StartTime: startTime,
		EndTime:   &now,
	}

	if err := b.db.CreateCheckIn(checkIn); err != nil {
		logError(s, i.ChannelID, "CreateCheckIn", err.Error())
		respondWithError(s, i, "Error creating check-in: "+err.Error())
		return
	}

	// Check for and handle any active check-in
	activeCheckIn, err := b.db.GetActiveCheckIn(user.ID, i.GuildID)
	if err != nil {
		logError(s, i.ChannelID, "GetActiveCheckIn", err.Error())
		respondWithError(s, i, "Error checking active tasks: "+err.Error())
		return
	}

	var checkoutMsg string
	if activeCheckIn != nil {
		// Get active task details
		activeTask, err := b.db.GetTaskByID(activeCheckIn.TaskID)
		if err != nil {
			logError(s, i.ChannelID, "GetTaskByID", err.Error())
			respondWithError(s, i, "Error retrieving active task details: "+err.Error())
			return
		}

		// Check out from active task
		if err := b.db.CheckOut(activeCheckIn.ID); err != nil {
			respondWithError(s, i, "Error checking out: "+err.Error())
			return
		}

		// Get the updated check-in to get the actual end time
		updatedCheckIn, err := b.db.GetCheckInByID(activeCheckIn.ID)
		if err != nil {
			respondWithError(s, i, "Error retrieving checkout details: "+err.Error())
			return
		}

		activeDuration := updatedCheckIn.EndTime.Sub(updatedCheckIn.StartTime)
		checkoutMsg = fmt.Sprintf("\nChecked out from active task: %s (Time spent: %s)",
			activeTask.Name, formatDuration(activeDuration))
	}

	respondWithSuccess(s, i, fmt.Sprintf("Declared %s spent on task: %s%s",
		formatDuration(duration), task.Name, checkoutMsg))
}
