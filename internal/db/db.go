package db

import (
	"context"
	"database/sql"
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
	Host     string `yaml:"host" env:"DB_HOST,required"`
	Port     int    `yaml:"port" env:"DB_PORT,required"`
	User     string `yaml:"user" env:"DB_USER,required"`
	Password string `yaml:"password" env:"DB_PASSWORD,required"`
	DBName   string `yaml:"dbname" env:"DB_NAME,required"`
	SSLMode  string `yaml:"sslmode" env:"DB_SSLMODE,required"`
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
		INSERT INTO tasks (id, user_id, server_id, name, description, tags, completed, global, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := db.Exec(context.Background(), query,
		task.ID.String(),
		task.UserID.String(),
		task.ServerID,
		task.Name,
		task.Description,
		task.Tags,
		task.Completed,
		task.Global,
		task.CreatedAt,
	)
	return err
}

// CreateCheckIn creates a new check-in record
func (db *DB) CreateCheckIn(checkIn *models.CheckIn) error {
	query := `
		INSERT INTO check_ins (id, user_id, server_id, task_id, start_time, active)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := db.Exec(context.Background(), query,
		checkIn.ID.String(),
		checkIn.UserID.String(),
		checkIn.ServerID,
		checkIn.TaskID.String(),
		checkIn.StartTime,
		true,
	)
	return err
}

// GetActiveCheckIn gets the active check-in for a user if one exists
func (db *DB) GetActiveCheckIn(userID uuid.UUID, serverID string) (*models.CheckIn, error) {
	query := `
		SELECT id, user_id, server_id, task_id, start_time, end_time, active
		FROM check_ins
		WHERE user_id = $1 AND server_id = $2 AND active = true
		LIMIT 1`

	var checkIn models.CheckIn
	var endTime sql.NullTime
	err := db.QueryRow(context.Background(), query, userID.String(), serverID).Scan(
		&checkIn.ID,
		&checkIn.UserID,
		&checkIn.ServerID,
		&checkIn.TaskID,
		&checkIn.StartTime,
		&endTime,
		&checkIn.Active,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if endTime.Valid {
		checkIn.EndTime = &endTime.Time
	}
	return &checkIn, nil
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
		SET end_time = $1, active = false
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

// GetAllActiveCheckIns retrieves all active check-ins with associated tasks for a server
func (db *DB) GetAllActiveCheckIns(serverID string) ([]*models.CheckInWithTask, error) {
	query := `
		SELECT 
			COALESCE(c.id, ''),
			u.id,
			COALESCE(c.server_id, ''),
			c.task_id,
			COALESCE(c.start_time, NOW()),
			c.end_time,
			t.name,
			t.description,
			u.username
		FROM users u
		LEFT JOIN check_ins c ON u.id = c.user_id AND c.active = true AND c.server_id = $1
		LEFT JOIN tasks t ON c.task_id = t.id
		WHERE EXISTS (
			SELECT 1 
			FROM check_ins ci 
			WHERE ci.user_id = u.id 
			AND ci.server_id = $1
		)
		ORDER BY u.username ASC`

	rows, err := db.Query(context.Background(), query, serverID)
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
		var (
			checkInID, userID, serverIDScan sql.NullString
			taskID                          sql.NullString
			startTime                       sql.NullTime
			endTime                         sql.NullTime
			taskName, taskDesc              sql.NullString
			username                        string
		)

		err := rows.Scan(
			&checkInID,
			&userID,
			&serverIDScan,
			&taskID,
			&startTime,
			&endTime,
			&taskName,
			&taskDesc,
			&username,
		)
		if err != nil {
			return nil, err
		}

		// Set check-in details
		if checkInID.Valid {
			ci.CheckIn.ID = uuid.MustParse(checkInID.String)
		}
		if userID.Valid {
			ci.CheckIn.UserID = uuid.MustParse(userID.String)
		}
		if serverIDScan.Valid {
			ci.CheckIn.ServerID = serverIDScan.String
		}
		if taskID.Valid {
			ci.CheckIn.TaskID = uuid.MustParse(taskID.String)
		}
		if startTime.Valid {
			ci.CheckIn.StartTime = startTime.Time
		}
		if endTime.Valid {
			ci.CheckIn.EndTime = &endTime.Time
		}

		// Set task details
		if taskName.Valid {
			ci.Task.Name = taskName.String
		} else {
			ci.Task.Name = "Not checked in"
		}
		if taskDesc.Valid {
			ci.Task.Description = taskDesc.String
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

// GetAllTaskHistory retrieves completed check-ins for all users within a date range for a server
func (db *DB) GetAllTaskHistory(serverID string, startDate, endDate time.Time) ([]*models.CheckInWithTask, error) {
	query := `
		SELECT 
			c.id, c.user_id, c.task_id, c.start_time, c.end_time,
			t.name, t.description
		FROM check_ins c
		JOIN tasks t ON c.task_id = t.id
		WHERE c.server_id = $1 
		AND c.start_time >= $2 
		AND c.start_time < $3
		AND c.end_time IS NOT NULL
		ORDER BY c.start_time DESC`

	rows, err := db.Query(context.Background(), query, serverID, startDate, endDate)
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
		var endTime time.Time
		err := rows.Scan(
			&ci.CheckIn.ID,
			&ci.CheckIn.UserID,
			&ci.CheckIn.TaskID,
			&ci.CheckIn.StartTime,
			&endTime,
			&ci.Task.Name,
			&ci.Task.Description,
		)
		if err != nil {
			return nil, err
		}
		ci.CheckIn.EndTime = &endTime
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
		SELECT id, user_id, task_id, start_time, end_time, active
		FROM check_ins
		WHERE id = $1`

	var checkIn models.CheckIn
	var endTime sql.NullTime
	err := db.QueryRow(context.Background(), query, checkInID.String()).Scan(
		&checkIn.ID,
		&checkIn.UserID,
		&checkIn.TaskID,
		&checkIn.StartTime,
		&endTime,
		&checkIn.Active,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if endTime.Valid {
		checkIn.EndTime = &endTime.Time
	}
	return &checkIn, nil
}

// GetUserTasks retrieves all tasks for a user in a specific server
func (db *DB) GetUserTasks(userID uuid.UUID, serverID string) ([]*models.Task, error) {
	query := `
		SELECT id, user_id, server_id, name, description, tags, completed, global, created_at
		FROM tasks
		WHERE (user_id = $1 OR global = true) AND server_id = $2
		ORDER BY created_at DESC`

	rows, err := db.Query(context.Background(), query, userID.String(), serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*models.Task
	for rows.Next() {
		task := &models.Task{}
		err := rows.Scan(
			&task.ID,
			&task.UserID,
			&task.ServerID,
			&task.Name,
			&task.Description,
			&task.Tags,
			&task.Completed,
			&task.Global,
			&task.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// GetAllUsers retrieves all users from the database
func (db *DB) GetAllUsers() ([]*models.User, error) {
	query := `
		SELECT DISTINCT 
			u.id, 
			u.discord_id, 
			u.username, 
			u.timezone, 
			u.created_at,
			CASE WHEN c.id IS NOT NULL THEN 0 ELSE 1 END AS has_activity
		FROM users u
		LEFT JOIN check_ins c ON u.id = c.user_id
		ORDER BY 
			has_activity,
			u.username ASC`

	rows, err := db.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		user := &models.User{}
		var hasActivity int
		err := rows.Scan(
			&user.ID,
			&user.DiscordID,
			&user.Username,
			&user.Timezone,
			&user.CreatedAt,
			&hasActivity,
		)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// GetServerSettings retrieves settings for a specific server
func (db *DB) GetServerSettings(serverID string) (*models.ServerSettings, error) {
	query := `
		SELECT id, server_id, inactivity_limit, ping_timeout, created_at
		FROM server_settings
		WHERE server_id = $1`

	settings := &models.ServerSettings{}
	err := db.QueryRow(context.Background(), query, serverID).Scan(
		&settings.ID,
		&settings.ServerID,
		&settings.InactivityLimit,
		&settings.PingTimeout,
		&settings.CreatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error getting server settings: %w", err)
	}

	return settings, nil
}

// CreateServerSettings creates new settings for a server with default values
func (db *DB) CreateServerSettings(serverID string) (*models.ServerSettings, error) {
	settings := &models.ServerSettings{
		ID:              uuid.New(),
		ServerID:        serverID,
		InactivityLimit: 30, // Default 30 minutes
		PingTimeout:     5,  // Default 5 minutes
		CreatedAt:       time.Now(),
	}

	query := `
		INSERT INTO server_settings (id, server_id, inactivity_limit, ping_timeout, created_at)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := db.Exec(context.Background(), query,
		settings.ID.String(),
		settings.ServerID,
		settings.InactivityLimit,
		settings.PingTimeout,
		settings.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("error creating server settings: %w", err)
	}

	return settings, nil
}

// GetOrCreateServerSettings retrieves server settings or creates them with defaults
func (db *DB) GetOrCreateServerSettings(serverID string) (*models.ServerSettings, error) {
	settings, err := db.GetServerSettings(serverID)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		return db.CreateServerSettings(serverID)
	}
	return settings, nil
}

// GetUserByID retrieves a user by their ID
func (db *DB) GetUserByID(userID uuid.UUID) (*models.User, error) {
	query := `
		SELECT id, discord_id, username, timezone, created_at
		FROM users
		WHERE id = $1`

	user := &models.User{}
	err := db.QueryRow(context.Background(), query, userID.String()).Scan(
		&user.ID,
		&user.DiscordID,
		&user.Username,
		&user.Timezone,
		&user.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("error getting user: %w", err)
	}
	return user, nil
}

// UpdateTaskStatus updates a task's completed status
func (db *DB) UpdateTaskStatus(taskID uuid.UUID, completed bool) error {
	query := `
		UPDATE tasks
		SET completed = $1
		WHERE id = $2
	`
	ctx := context.Background()
	result, err := db.Exec(ctx, query, completed, taskID)
	if err != nil {
		return fmt.Errorf("error updating task status: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("task not found")
	}

	return nil
}

// GetGuildUsers returns all users who have activity in the specified guild
func (db *DB) GetGuildUsers(guildID string) ([]*models.User, error) {
	query := `
		SELECT DISTINCT u.id, u.discord_id, u.username, u.timezone, u.created_at
		FROM users u
		JOIN check_ins ci ON u.id = ci.user_id
		WHERE ci.server_id = $1
		ORDER BY u.username ASC`

	rows, err := db.Query(context.Background(), query, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		user := &models.User{}
		err := rows.Scan(
			&user.ID,
			&user.DiscordID,
			&user.Username,
			&user.Timezone,
			&user.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}
