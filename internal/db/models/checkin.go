package models

import (
	"time"

	"github.com/google/uuid"
)

type CheckIn struct {
	ID        uuid.UUID      `db:"id"`
	UserID    uuid.UUID      `db:"user_id"`
	TaskID    uuid.UUID      `db:"task_id"`
	StartTime time.Time      `db:"start_time"`
	EndTime   *time.Time     `db:"end_time"`
	Duration  *time.Duration `db:"duration"`
}
