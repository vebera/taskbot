package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID        uuid.UUID `db:"id"`
	DiscordID string    `db:"discord_id"`
	Username  string    `db:"username"`
	Timezone  string    `db:"timezone"`
	CreatedAt time.Time `db:"created_at"`
}
