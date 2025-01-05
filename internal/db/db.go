package db

import (
	"context"
	"fmt"
	"time"

	"taskbot/internal/db/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	*pgxpool.Pool
}

func New(config struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"`
}) (*DB, error) {
	// Create a configuration object
	cfg, err := pgxpool.ParseConfig(fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		config.User, config.Password, config.Host, config.Port, config.DBName, config.SSLMode,
	))
	if err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
	}

	// Configure connection pool and statement cache
	cfg.MaxConns = 10
	cfg.MinConns = 2
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("error creating connection pool: %w", err)
	}

	return &DB{pool}, nil
}

// CreateTask creates a new task in the database
func (db *DB) CreateTask(task *models.Task) error {
	query := `
		INSERT INTO tasks (id, user_id, name, description, tags, completed, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := db.Exec(context.Background(), query,
		task.ID.String(),
		task.UserID.String(),
		task.Name,
		task.Description,
		task.Tags,
		task.Completed,
		task.CreatedAt,
	)
	return err
}

// CreateCheckIn creates a new check-in record
func (db *DB) CreateCheckIn(checkIn *models.CheckIn) error {
	query := `
		INSERT INTO check_ins (id, user_id, task_id, start_time)
		VALUES ($1, $2, $3, $4)`

	_, err := db.Exec(context.Background(), query,
		checkIn.ID.String(),
		checkIn.UserID.String(),
		checkIn.TaskID.String(),
		checkIn.StartTime,
	)
	return err
}

// GetActiveCheckIn gets the active check-in for a user if one exists
func (db *DB) GetActiveCheckIn(userID uuid.UUID) (*models.CheckIn, error) {
	query := `
		SELECT id, user_id, task_id, start_time, end_time
		FROM check_ins
		WHERE user_id = $1 AND end_time IS NULL
		LIMIT 1`

	checkIn := &models.CheckIn{}
	err := db.QueryRow(context.Background(), query, userID.String()).Scan(
		&checkIn.ID,
		&checkIn.UserID,
		&checkIn.TaskID,
		&checkIn.StartTime,
		&checkIn.EndTime,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return checkIn, nil
}

// CheckOut updates the end_time of a check-in
func (db *DB) CheckOut(checkInID uuid.UUID) error {
	// First get the check-in to validate it exists and isn't already checked out
	query := `
		SELECT start_time
		FROM check_ins
		WHERE id = $1 AND end_time IS NULL`

	var startTime time.Time
	err := db.QueryRow(context.Background(), query, checkInID.String()).Scan(&startTime)
	if err != nil {
		return fmt.Errorf("error getting check-in: %w", err)
	}

	// Calculate end time
	endTime := time.Now()
	if endTime.Before(startTime) {
		endTime = startTime.Add(time.Second)
	}

	query = `
		UPDATE check_ins
		SET end_time = $1
		WHERE id = $2 AND end_time IS NULL`

	_, err = db.Exec(context.Background(), query, endTime, checkInID.String())
	return err
}

// GetTaskByID retrieves a task by its ID
func (db *DB) GetTaskByID(taskID uuid.UUID) (*models.Task, error) {
	query := `
		SELECT id, user_id, name, description, tags, completed, created_at
		FROM tasks
		WHERE id = $1`

	task := &models.Task{}
	err := db.QueryRow(context.Background(), query, taskID.String()).Scan(
		&task.ID,
		&task.UserID,
		&task.Name,
		&task.Description,
		&task.Tags,
		&task.Completed,
		&task.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return task, err
}

// GetAllActiveCheckIns retrieves all active check-ins with associated tasks
func (db *DB) GetAllActiveCheckIns() ([]*models.CheckInWithTask, error) {
	query := `
		SELECT 
			c.id, c.user_id, c.task_id, c.start_time, c.end_time,
			t.name, t.description
		FROM check_ins c
		JOIN tasks t ON c.task_id = t.id
		WHERE c.end_time IS NULL`

	rows, err := db.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checkIns []*models.CheckInWithTask
	for rows.Next() {
		ci := &models.CheckInWithTask{
			CheckIn: &models.CheckIn{},
			Task:    &models.Task{},
		}
		err := rows.Scan(
			&ci.CheckIn.ID,
			&ci.CheckIn.UserID,
			&ci.CheckIn.TaskID,
			&ci.CheckIn.StartTime,
			&ci.CheckIn.EndTime,
			&ci.Task.Name,
			&ci.Task.Description,
		)
		if err != nil {
			return nil, err
		}
		checkIns = append(checkIns, ci)
	}
	return checkIns, rows.Err()
}

// GetTaskHistory retrieves completed check-ins for a user within a date range
func (db *DB) GetTaskHistory(userID uuid.UUID, startDate, endDate time.Time) ([]*models.CheckInWithTask, error) {
	query := `
		SELECT 
			c.id, c.user_id, c.task_id, c.start_time, c.end_time,
			t.name, t.description
		FROM check_ins c
		JOIN tasks t ON c.task_id = t.id
		WHERE c.user_id = $1 
		AND c.start_time >= $2 
		AND c.start_time < $3
		AND c.end_time IS NOT NULL
		ORDER BY c.start_time DESC`

	rows, err := db.Query(context.Background(), query, userID.String(), startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checkIns []*models.CheckInWithTask
	for rows.Next() {
		ci := &models.CheckInWithTask{
			CheckIn: &models.CheckIn{},
			Task:    &models.Task{},
		}
		err := rows.Scan(
			&ci.CheckIn.ID,
			&ci.CheckIn.UserID,
			&ci.CheckIn.TaskID,
			&ci.CheckIn.StartTime,
			&ci.CheckIn.EndTime,
			&ci.Task.Name,
			&ci.Task.Description,
		)
		if err != nil {
			return nil, err
		}
		checkIns = append(checkIns, ci)
	}
	return checkIns, rows.Err()
}

// GetOrCreateUser retrieves a user by Discord ID or creates a new one
func (db *DB) GetOrCreateUser(discordID string, username string) (*models.User, error) {
	// Try to get existing user
	query := `
		SELECT id, discord_id, username, timezone, created_at
		FROM users
		WHERE discord_id = $1`

	user := &models.User{}
	err := db.QueryRow(context.Background(), query, discordID).Scan(
		&user.ID,
		&user.DiscordID,
		&user.Username,
		&user.Timezone,
		&user.CreatedAt,
	)

	if err == pgx.ErrNoRows {
		// Create new user with UTC timezone by default
		user = &models.User{
			ID:        uuid.New(),
			DiscordID: discordID,
			Username:  username,
			Timezone:  "UTC",
			CreatedAt: time.Now(),
		}

		insertQuery := `
			INSERT INTO users (id, discord_id, username, timezone, created_at)
			VALUES ($1, $2, $3, $4, $5)`

		_, err = db.Exec(context.Background(), insertQuery,
			user.ID.String(),
			user.DiscordID,
			user.Username,
			user.Timezone,
			user.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("error creating user: %w", err)
		}
		return user, nil
	}

	if err != nil {
		return nil, fmt.Errorf("error getting user: %w", err)
	}

	return user, nil
}

// UpdateUserTimezone updates a user's timezone
func (db *DB) UpdateUserTimezone(userID uuid.UUID, timezone string) error {
	query := `
		UPDATE users
		SET timezone = $1
		WHERE id = $2`

	_, err := db.Exec(context.Background(), query, timezone, userID.String())
	return err
}

// GetCheckInByID retrieves a check-in by its ID
func (db *DB) GetCheckInByID(checkInID uuid.UUID) (*models.CheckIn, error) {
	query := `
		SELECT id, user_id, task_id, start_time, end_time
		FROM check_ins
		WHERE id = $1`

	checkIn := &models.CheckIn{}
	err := db.QueryRow(context.Background(), query, checkInID.String()).Scan(
		&checkIn.ID,
		&checkIn.UserID,
		&checkIn.TaskID,
		&checkIn.StartTime,
		&checkIn.EndTime,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return checkIn, nil
}
