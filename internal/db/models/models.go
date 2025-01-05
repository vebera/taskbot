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
	TaskID    uuid.UUID
	StartTime time.Time
	EndTime   *time.Time
	Active    bool
}

type CheckInWithTask struct {
	CheckIn *CheckIn
	Task    *Task
}

type ServerSettings struct {
	ID              uuid.UUID `db:"id"`
	ServerID        string    `db:"server_id"`
	InactivityLimit int       `db:"inactivity_limit"`
	PingTimeout     int       `db:"ping_timeout"`
	CreatedAt       time.Time `db:"created_at"`
}

// Add other models here if needed
