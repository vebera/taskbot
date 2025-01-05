package models

import (
	"time"

	"github.com/google/uuid"
)

type ServerSettings struct {
	ID              uuid.UUID `db:"id"`
	ServerID        string    `db:"server_id"`
	InactivityLimit int       `db:"inactivity_limit"`
	PingTimeout     int       `db:"ping_timeout"`
	CreatedAt       time.Time `db:"created_at"`
}
