package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID        uuid.UUID
	DiscordID string
	Username  string
	Timezone  string
	CreatedAt time.Time
}

type Task struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	ServerID    string
	Name        string
	Description string
	Tags        []string
	Completed   bool
	Global      bool
	CreatedAt   time.Time
}

// CheckIn represents a task check-in record
type CheckIn struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ServerID  string
	TaskID    uuid.UUID
	StartTime time.Time
	EndTime   *time.Time
	Active    bool
}

type CheckInWithTask struct {
	CheckIn *CheckIn
	Task    *Task
	User    *User
}

type ServerSettings struct {
	ID              uuid.UUID
	ServerID        string
	InactivityLimit int
	PingTimeout     int
	CreatedAt       time.Time
}

// Add other models here if needed
