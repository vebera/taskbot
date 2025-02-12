-- Create guild_users table to track guild membership
CREATE TABLE IF NOT EXISTS guild_users (
    user_id UUID NOT NULL REFERENCES users(id),
    guild_id VARCHAR(64) NOT NULL,
    joined_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, guild_id)
);

-- Create index for faster lookups
CREATE INDEX IF NOT EXISTS idx_guild_users_guild_id ON guild_users(guild_id); 