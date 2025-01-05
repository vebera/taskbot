package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type Task struct {
	ID          uuid.UUID      `db:"id"`
	UserID      uuid.UUID      `db:"user_id"`
	Name        string         `db:"name"`
	Description string         `db:"description"`
	Tags        pq.StringArray `db:"tags"`
	Completed   bool           `db:"completed"`
	CreatedAt   time.Time      `db:"created_at"`
}
