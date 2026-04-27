// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package testuser

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	// PostgreSQL driver
	_ "github.com/lib/pq"
)

const (
	TestEmail    = "test@codesphere.com"
	TestTeamName = "Tests"
	tokenPrefix  = "CS_"

	// Default connection parameters.
	DefaultPort    = 5432
	DefaultUser    = "postgres"
	DefaultDBName  = "codesphere"
	DefaultSSLMode = "disable"
)

// CreateTestUserOpts contains the options for creating a test user.
type CreateTestUserOpts struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// TestUserResult contains the result of creating a test user.
type TestUserResult struct {
	Email             string `json:"email"`
	PlaintextPassword string `json:"password"`
	PlaintextAPIToken string `json:"api_token"`
}

// TestUserCreator manages the lifecycle of test user creation.
type TestUserCreator struct {
	opts CreateTestUserOpts
	db   *sql.DB
}

// New creates a new TestUserCreator with the given options.
func New(opts CreateTestUserOpts) (*TestUserCreator, error) {
	if opts.Port == 0 {
		opts.Port = DefaultPort
	}
	if opts.User == "" {
		opts.User = DefaultUser
	}
	if opts.DBName == "" {
		opts.DBName = DefaultDBName
	}
	if opts.SSLMode == "" {
		opts.SSLMode = DefaultSSLMode
	}
	if opts.Host == "" {
		return nil, fmt.Errorf("host is required")
	}
	if opts.Password == "" {
		return nil, fmt.Errorf("password is required")
	}

	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=10",
		opts.Host, opts.Port, opts.User, opts.Password, opts.DBName, opts.SSLMode,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	db.SetConnMaxLifetime(30 * time.Second)
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to connect to database at %s:%d: %w", opts.Host, opts.Port, err)
	}

	log.Printf("Connected to PostgreSQL at %s:%d", opts.Host, opts.Port)
	return &TestUserCreator{opts: opts, db: db}, nil
}

// close closes the underlying database connection.
func (c *TestUserCreator) close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// Create generates credentials and inserts the test user into the database.
func (c *TestUserCreator) Create() (*TestUserResult, error) {
	plaintextPassword := os.Getenv("OMS_CS_TEST_USER_PASSWORD")
	if plaintextPassword == "" {
		return nil, fmt.Errorf("OMS_CS_TEST_USER_PASSWORD environment variable is not set")
	}
	hashedPassword := os.Getenv("OMS_CS_TEST_USER_PASSWORD_HASHED")
	if hashedPassword == "" {
		return nil, fmt.Errorf("OMS_CS_TEST_USER_PASSWORD_HASHED environment variable is not set")
	}

	plaintextToken, err := generateAPIToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API token: %w", err)
	}

	hashedToken := HashAPIToken(plaintextToken)

	result, err := c.createInDB(hashedPassword, hashedToken)
	if err != nil {
		return nil, err
	}

	result.PlaintextPassword = plaintextPassword
	result.PlaintextAPIToken = plaintextToken

	return result, nil
}

// CreateTestUser is a convenience facade: New -> Create -> close.
func CreateTestUser(opts CreateTestUserOpts) (*TestUserResult, error) {
	creator, err := New(opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = creator.close() }()
	return creator.Create()
}

// createInDB executes the database inserts inside a transaction.
func (c *TestUserCreator) createInDB(hashedPassword, hashedToken string) (*TestUserResult, error) {
	exists, err := c.userExists()
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("test user %s already exists", TestEmail)
	}

	tx, err := c.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	userID, err := c.insertCredentials(tx, hashedPassword)
	if err != nil {
		return nil, err
	}

	if err := c.insertEmailConfirmation(tx); err != nil {
		return nil, err
	}

	teamID, err := c.insertTeam(tx)
	if err != nil {
		return nil, err
	}

	if err := c.insertTeamMember(tx, userID, teamID); err != nil {
		return nil, err
	}

	if err := c.insertAPIToken(tx, hashedToken, userID); err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Test user created: email=%s, userID=%d, teamID=%d", TestEmail, userID, teamID)

	return &TestUserResult{Email: TestEmail}, nil
}

func (c *TestUserCreator) userExists() (bool, error) {
	var exists bool
	err := c.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM authservice.credentials WHERE email = $1)`, TestEmail).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check for existing test user: %w", err)
	}
	return exists, nil
}

func (c *TestUserCreator) insertCredentials(tx *sql.Tx, hashedPassword string) (int, error) {
	var userID int
	err := tx.QueryRow(`
		INSERT INTO authservice.credentials
			(user_id, email, password_hash, authentication_method, signed_up, banned)
		VALUES(nextval('authservice.credentials_user_id_seq'::regclass), $1, $2, 'password'::text, false, false)
		RETURNING user_id`,
		TestEmail, hashedPassword,
	).Scan(&userID)
	if err != nil {
		return 0, fmt.Errorf("failed to insert credentials: %w", err)
	}
	return userID, nil
}

func (c *TestUserCreator) insertEmailConfirmation(tx *sql.Tx) error {
	_, err := tx.Exec(`
		INSERT INTO authservice.email_confirmations
			(id, email, pending, created_at)
		VALUES(uuid_generate_v4(), $1, false, CURRENT_TIMESTAMP)`,
		TestEmail,
	)
	if err != nil {
		return fmt.Errorf("failed to insert email confirmation: %w", err)
	}
	return nil
}

func (c *TestUserCreator) insertTeam(tx *sql.Tx) (int, error) {
	var teamID int
	err := tx.QueryRow(`
		INSERT INTO "teamService".teams
			(id, "name", description, first_team, default_data_center_id, deleted, deletion_pending, created_at)
		VALUES(nextval('"teamService".teams_id_seq'::regclass), $1, '', true, 1, false, false, CURRENT_TIMESTAMP)
		RETURNING id`,
		TestTeamName,
	).Scan(&teamID)
	if err != nil {
		return 0, fmt.Errorf("failed to insert team: %w", err)
	}
	return teamID, nil
}

func (c *TestUserCreator) insertTeamMember(tx *sql.Tx, userID, teamID int) error {
	_, err := tx.Exec(`
		INSERT INTO "teamService".team_members
			(user_id, team_id, "role", pending, created_at)
		VALUES($1, $2, 0, false, CURRENT_TIMESTAMP)`,
		userID, teamID,
	)
	if err != nil {
		return fmt.Errorf("failed to insert team member: %w", err)
	}
	return nil
}

func (c *TestUserCreator) insertAPIToken(tx *sql.Tx, hashedToken string, userID int) error {
	_, err := tx.Exec(`
		INSERT INTO public_api_service.tokens
			(id, "token", user_id, "name", created_at)
		VALUES(nextval('public_api_service.tokens_id_seq'::regclass), $1, $2, 'testkey', now())`,
		hashedToken, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to insert API token: %w", err)
	}
	return nil
}

// LogAndPersistResult writes the test user result to a JSON file and logs the credentials.
func LogAndPersistResult(result *TestUserResult, workdir string) {
	filePath, err := WriteResultToFile(result, workdir)
	if err != nil {
		log.Printf("warning: failed to write test user result to file: %v", err)
	} else {
		log.Printf("Test user credentials written to %s", filePath)
	}

	log.Printf("Email:     %s", result.Email)
	log.Printf("Password:  %s", result.PlaintextPassword)
	log.Printf("API Token: %s", result.PlaintextAPIToken)
}

// WriteResultToFile writes the test user result to a JSON file in the given directory.
func WriteResultToFile(result *TestUserResult, dir string) (string, error) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal test user result: %w", err)
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	filePath := filepath.Join(dir, "test-user.json")
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return "", fmt.Errorf("failed to write test user file: %w", err)
	}

	return filePath, nil
}

func generateAPIToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return tokenPrefix + hex.EncodeToString(b), nil
}
