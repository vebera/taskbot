package bot

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"sort"
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
	case "checkin", "task":
		b.handleTaskAutocomplete(s, i)
	case "report":
		b.handleUsernameAutocomplete(s, i)
	}
}

func (b *Bot) handleTaskAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Get the user's tasks for autocomplete
	user, err := b.getUserFromInteraction(s, i)
	if err != nil {
		return
	}

	// Check if user is admin
	isUserAdmin := isAdmin(s, i.GuildID, i.Member.User.ID)

	// Get active check-in to filter out active task
	var activeTaskID *uuid.UUID
	if i.ApplicationCommandData().Name == "checkin" {
		activeCheckIn, err := b.db.GetActiveCheckIn(user.ID)
		if err != nil {
			log.Printf("Error getting active check-in: %v", err)
			return
		}
		if activeCheckIn != nil {
			activeTaskID = &activeCheckIn.TaskID
		}
	}

	tasks, err := b.db.GetUserTasks(user.ID)
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
		// For task command, the task option is directly in the options
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
		log.Printf("Error getting users for autocomplete: %v", err)
		return
	}

	log.Printf("Total users found: %d", len(users))

	// Get the current input value
	var focusedOption *discordgo.ApplicationCommandInteractionDataOption
	for _, opt := range i.ApplicationCommandData().Options {
		log.Printf("Checking option: %s, focused: %v", opt.Name, opt.Focused)
		if opt.Focused && opt.Name == "username" {
			focusedOption = opt
			break
		}
	}

	if focusedOption == nil {
		log.Printf("No focused username option found in command. Options: %+v", i.ApplicationCommandData().Options)
		return
	}

	input := strings.ToLower(focusedOption.StringValue())
	log.Printf("Autocomplete input: %s", input)

	// Filter and create choices
	var choices []*discordgo.ApplicationCommandOptionChoice
	for _, user := range users {
		if strings.Contains(strings.ToLower(user.Username), input) {
			choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
				Name:  user.Username,
				Value: user.Username,
			})
			log.Printf("Added choice: %s", user.Username)
		}
		if len(choices) >= 25 { // Discord limit
			break
		}
	}

	log.Printf("Found %d matching users", len(choices))

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
	subcommand := i.ApplicationCommandData().Options[0]
	options := subcommand.Options

	var task *models.Task
	var err error

	user, err := b.getUserFromInteraction(s, i)
	if err != nil {
		return
	}

	switch subcommand.Name {
	case "existing":
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
		taskName := options[0].StringValue()
		var description string
		if len(options) > 1 {
			description = options[1].StringValue()
		}

		task = &models.Task{
			ID:          uuid.New(),
			UserID:      user.ID,
			Name:        taskName,
			Description: description,
			CreatedAt:   time.Now(),
		}

		if err := b.db.CreateTask(task); err != nil {
			logError(s, i.ChannelID, "CreateTask", err.Error())
			respondWithError(s, i, "Error creating task: "+err.Error())
			return
		}
	}

	logCommand(s, i, "checkin", fmt.Sprintf("task: %s", task.Name))

	// Check for active check-in
	activeCheckIn, err := b.db.GetActiveCheckIn(user.ID)
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
	if err != nil {
		return
	}

	// Get active check-in
	activeCheckIn, err := b.db.GetActiveCheckIn(user.ID)
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

	// Get all active check-ins
	activeCheckIns, err := b.db.GetAllActiveCheckIns()
	if err != nil {
		respondWithError(s, i, "Error retrieving active check-ins: "+err.Error())
		return
	}

	// Get today's start time in UTC
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// Create a map to store user status information
	type UserStatus struct {
		username     string
		isActive     bool
		currentTask  string
		taskDuration time.Duration
		totalToday   time.Duration
	}
	userStatuses := make(map[string]*UserStatus)

	// Get all users who have any activity today
	todayHistory, err := b.db.GetAllTaskHistory(todayStart, now)
	if err != nil {
		respondWithError(s, i, "Error retrieving today's history: "+err.Error())
		return
	}

	// Calculate total time for today for each user
	for _, ci := range todayHistory {
		user, err := b.db.GetUserByID(ci.CheckIn.UserID)
		if err != nil {
			continue
		}

		if _, exists := userStatuses[user.ID.String()]; !exists {
			userStatuses[user.ID.String()] = &UserStatus{
				username: user.Username,
			}
		}

		duration := ci.CheckIn.EndTime.Sub(ci.CheckIn.StartTime)
		userStatuses[user.ID.String()].totalToday += duration
	}

	// Update active users' current tasks and status
	for _, ci := range activeCheckIns {
		user, err := b.db.GetUserByID(ci.CheckIn.UserID)
		if err != nil {
			continue
		}

		if _, exists := userStatuses[user.ID.String()]; !exists {
			userStatuses[user.ID.String()] = &UserStatus{
				username: user.Username,
			}
		}

		status := userStatuses[user.ID.String()]
		status.isActive = true
		status.currentTask = ci.Task.Name
		status.taskDuration = time.Since(ci.CheckIn.StartTime)
	}

	// Convert to slice for sorting
	var statusList []*UserStatus
	for _, status := range userStatuses {
		statusList = append(statusList, status)
	}

	// Sort by active status (active first) and then by username
	sort.Slice(statusList, func(i, j int) bool {
		if statusList[i].isActive != statusList[j].isActive {
			return statusList[i].isActive
		}
		return statusList[i].username < statusList[j].username
	})

	// Format the response
	var response strings.Builder
	response.WriteString("```\n")

	// Write header
	response.WriteString(fmt.Sprintf("%-20s %-10s %-15s %-30s %-15s\n",
		"USER", "STATUS", "TODAY TOTAL", "CURRENT TASK", "TIME ELAPSED"))
	response.WriteString(strings.Repeat("-", 95) + "\n")

	// Write user statuses
	for _, status := range statusList {
		currentTask := "N/A"
		taskDuration := ""
		userStatus := "Offline"

		if status.isActive {
			currentTask = status.currentTask
			taskDuration = formatDuration(status.taskDuration)
			userStatus = "🟢 Online"
		}

		response.WriteString(fmt.Sprintf("%-20s %-10s %-15s %-30s %-15s\n",
			truncateString(status.username, 20),
			userStatus,
			formatDuration(status.totalToday),
			truncateString(currentTask, 30),
			taskDuration,
		))
	}

	response.WriteString("```")

	if len(statusList) == 0 {
		respondWithSuccess(s, i, "No activity recorded today")
		return
	}

	respondWithSuccess(s, i, response.String())
}

func (b *Bot) handleReport(s *discordgo.Session, i *discordgo.InteractionCreate) {
	logCommand(s, i, "report")

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

	// Check if user is admin when requesting CSV
	isUserAdmin := isAdmin(s, i.GuildID, i.Member.User.ID)
	if format == "csv" && !isUserAdmin {
		log.Printf("CSV access denied for user %s in guild %s", i.Member.User.ID, i.GuildID)
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

	// Get all users' history
	history, err := b.db.GetAllTaskHistory(startDate, now)
	if err != nil {
		respondWithError(s, i, "Error retrieving task history: "+err.Error())
		return
	}

	if len(history) == 0 {
		respondWithSuccess(s, i, "No completed tasks found for the selected period")
		return
	}

	// Create a map to store user-task combinations and their durations
	type userTaskKey struct {
		username string
		taskName string
	}
	type taskInfo struct {
		duration  time.Duration
		ongoing   bool
		completed bool
		lastSeen  time.Time // Track when we last saw this task
	}
	taskTimes := make(map[userTaskKey]*taskInfo)

	// First, get all active check-ins to mark ongoing tasks
	activeCheckIns, err := b.db.GetAllActiveCheckIns()
	if err != nil {
		logError(s, i.ChannelID, "GetAllActiveCheckIns", err.Error())
		return
	}

	// Create a map of active tasks
	activeTasksByUser := make(map[userTaskKey]bool)
	for _, ci := range activeCheckIns {
		user, err := b.db.GetUserByID(ci.CheckIn.UserID)
		if err != nil {
			continue
		}
		activeTasksByUser[userTaskKey{
			username: user.Username,
			taskName: ci.Task.Name,
		}] = true
	}

	for _, ci := range history {
		user, err := b.db.GetUserByID(ci.CheckIn.UserID)
		if err != nil {
			logError(s, i.ChannelID, "GetUserByID", err.Error())
			continue
		}

		duration := ci.CheckIn.EndTime.Sub(ci.CheckIn.StartTime)
		key := userTaskKey{
			username: user.Username,
			taskName: ci.Task.Name,
		}
		endTime := *ci.CheckIn.EndTime
		if info, exists := taskTimes[key]; exists {
			info.duration += duration
			// Update completion status only if this check-in is more recent
			if endTime.After(info.lastSeen) {
				info.completed = ci.Task.Completed
				info.lastSeen = endTime
			}
		} else {
			taskTimes[key] = &taskInfo{
				duration:  duration,
				ongoing:   activeTasksByUser[key],
				completed: ci.Task.Completed,
				lastSeen:  endTime,
			}
		}
	}

	// Group tasks by user
	userGroups := make(map[string][]struct {
		taskName  string
		duration  time.Duration
		ongoing   bool
		completed bool
	})

	for key, info := range taskTimes {
		userGroups[key.username] = append(userGroups[key.username], struct {
			taskName  string
			duration  time.Duration
			ongoing   bool
			completed bool
		}{
			taskName:  key.taskName,
			duration:  info.duration,
			ongoing:   info.ongoing,
			completed: info.completed,
		})
	}

	// Get usernames and sort them
	var usernames []string
	for username := range userGroups {
		usernames = append(usernames, username)
	}
	sort.Strings(usernames)

	if format == "csv" {
		// Create CSV content
		var csvContent strings.Builder
		csvContent.WriteString("User,Task,Duration,Duration_Dec,Status\n")

		for _, username := range usernames {
			tasks := userGroups[username]
			sort.Slice(tasks, func(i, j int) bool {
				return tasks[i].taskName < tasks[j].taskName
			})

			for _, task := range tasks {
				status := "Checked out"
				if task.ongoing {
					status = "Active"
				} else if task.completed {
					status = "Completed"
				}

				// Calculate decimal duration in hours
				durationDec := float64(task.duration) / float64(time.Hour)

				// Escape fields that might contain commas
				escapedTask := strings.ReplaceAll(task.taskName, "\"", "\"\"")
				if strings.Contains(escapedTask, ",") {
					escapedTask = "\"" + escapedTask + "\""
				}

				csvContent.WriteString(fmt.Sprintf("%s,%s,%s,%.2f,%s\n",
					username,
					escapedTask,
					formatDuration(task.duration),
					durationDec,
					status))
			}
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
		filteredUsernames := []string{}
		for _, username := range usernames {
			if username == filterUsername {
				filteredUsernames = append(filteredUsernames, username)
				break
			}
		}
		usernames = filteredUsernames
	}

	// Format each user's tasks
	for _, username := range usernames {
		tasks := userGroups[username]

		// Sort tasks by name
		sort.Slice(tasks, func(i, j int) bool {
			return tasks[i].taskName < tasks[j].taskName
		})

		for _, task := range tasks {
			status := "Checked out"
			if task.ongoing {
				status = "🟢 Active"
			} else if task.completed {
				status = "Completed"
			}
			response.WriteString(fmt.Sprintf("%-20s %-30s %-15s %s\n",
				truncateString(username, 20),
				truncateString(task.taskName, 30),
				formatDuration(task.duration),
				status,
			))
		}
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
	if err != nil {
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

	logCommand(s, i, "task", fmt.Sprintf("task: %s, status: %s", task.Name, newStatus))

	// Check if user is admin or task owner
	isUserAdmin := isAdmin(s, i.GuildID, i.Member.User.ID)
	if !isUserAdmin && task.UserID != user.ID {
		respondWithError(s, i, "You can only update your own tasks")
		return
	}

	// Check if task is currently active
	activeCheckIn, err := b.db.GetActiveCheckIn(user.ID)
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
	if err != nil {
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
	if err != nil {
		return
	}

	// Create the global task
	task := &models.Task{
		ID:          uuid.New(),
		UserID:      user.ID, // Store admin as creator
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
