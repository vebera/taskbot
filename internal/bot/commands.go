package bot

import (
	"fmt"
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
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "task",
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
					Description: "Time period (today, week, month)",
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
					},
				},
			},
		},
	}
)

func (b *Bot) handleCheckin(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	taskName := options[0].StringValue()
	logCommand(s, i, "checkin", fmt.Sprintf("task: %s", taskName))

	var description string
	if len(options) > 1 {
		description = options[1].StringValue()
	}

	user, err := b.getUserFromInteraction(s, i)
	if err != nil {
		return
	}

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

	// Create or get task
	task := &models.Task{
		ID:          uuid.New(),
		UserID:      user.ID,
		Name:        taskName,
		Description: description,
		CreatedAt:   time.Now(),
	}

	// Save task to database
	if err := b.db.CreateTask(task); err != nil {
		logError(s, i.ChannelID, "CreateTask", err.Error())
		respondWithError(s, i, "Error creating task: "+err.Error())
		return
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

	respondWithSuccess(s, i, fmt.Sprintf("Started working on task: %s", taskName))
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

	activeCheckIns, err := b.db.GetAllActiveCheckIns()
	if err != nil {
		respondWithError(s, i, "Error retrieving active check-ins: "+err.Error())
		return
	}

	if len(activeCheckIns) == 0 {
		respondWithSuccess(s, i, "No active tasks at the moment")
		return
	}

	var response strings.Builder
	response.WriteString("Current active tasks:\n\n")

	for _, ci := range activeCheckIns {
		user, err := s.User(ci.CheckIn.UserID.String())
		if err != nil {
			continue
		}

		duration := time.Since(ci.CheckIn.StartTime)
		response.WriteString(fmt.Sprintf("**%s** is working on: %s\n",
			user.Username,
			ci.Task.Name,
		))
		response.WriteString(fmt.Sprintf("Duration: %s\n\n", formatDuration(duration)))
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: response.String(),
		},
	})
}

func (b *Bot) handleReport(s *discordgo.Session, i *discordgo.InteractionCreate) {
	logCommand(s, i, "report")

	user, err := b.getUserFromInteraction(s, i)
	if err != nil {
		return
	}

	period := i.ApplicationCommandData().Options[0].StringValue()

	now := time.Now()
	var startDate time.Time

	// Use user's timezone for date calculations
	loc, err := time.LoadLocation(user.Timezone)
	if err != nil {
		respondWithError(s, i, "Error loading timezone: "+err.Error())
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
	default:
		respondWithError(s, i, "Invalid time period")
		return
	}

	history, err := b.db.GetTaskHistory(user.ID, startDate, now)
	if err != nil {
		respondWithError(s, i, "Error retrieving task history: "+err.Error())
		return
	}

	if len(history) == 0 {
		respondWithSuccess(s, i, "No completed tasks found for the selected period")
		return
	}

	var response strings.Builder
	response.WriteString(fmt.Sprintf("Task history for %s:\n\n", period))

	var totalDuration time.Duration
	for _, ci := range history {
		duration := ci.CheckIn.EndTime.Sub(ci.CheckIn.StartTime)
		totalDuration += duration

		response.WriteString(fmt.Sprintf("**%s**\n", ci.Task.Name))
		response.WriteString(fmt.Sprintf("Duration: %s\n", formatDuration(duration)))
		response.WriteString(fmt.Sprintf("Started: %s\n\n", ci.CheckIn.StartTime.Format("2006-01-02 15:04:05")))
	}

	response.WriteString(fmt.Sprintf("Total time: %s", formatDuration(totalDuration)))

	respondWithSuccess(s, i, response.String())
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
