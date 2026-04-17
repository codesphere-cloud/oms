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
	TestPassword = "Test1234!"
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

// CreateTestUser connects to the Codesphere postgres instance and creates a test user
// with a hashed password and API token via SQL. Returns the plaintext credentials.
func CreateTestUser(opts CreateTestUserOpts) (*TestUserResult, error) {
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

	plaintextToken, err := generateAPIToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API token: %w", err)
	}

	hashedPassword := HashPassword(TestPassword)
	hashedToken := HashAPIToken(plaintextToken)

	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=10",
		opts.Host, opts.Port, opts.User, opts.Password, opts.DBName, opts.SSLMode,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}
	defer func() { _ = db.Close() }()

	db.SetConnMaxLifetime(30 * time.Second)
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to database at %s:%d: %w", opts.Host, opts.Port, err)
	}

	log.Printf("Connected to PostgreSQL at %s:%d", opts.Host, opts.Port)

	result, err := createTestUserInDB(db, hashedPassword, hashedToken)
	if err != nil {
		return nil, err
	}

	result.PlaintextPassword = TestPassword
	result.PlaintextAPIToken = plaintextToken

	return result, nil
}

// createTestUserInDB executes the database inserts inside a transaction.
// Separated from CreateTestUser to enable unit testing with sqlmock.
func createTestUserInDB(db *sql.DB, hashedPassword, hashedToken string) (*TestUserResult, error) {
	// Check if test user already exists
	var exists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM authservice.credentials WHERE email = $1)`, TestEmail).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing test user: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("test user %s already exists", TestEmail)
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Create the user credentials
	var userID int
	err = tx.QueryRow(`
		INSERT INTO authservice.credentials
			(user_id, email, password_hash, authentication_method, signed_up, banned)
		VALUES(nextval('authservice.credentials_user_id_seq'::regclass), $1, $2, 'password'::text, false, false)
		RETURNING user_id`,
		TestEmail, hashedPassword,
	).Scan(&userID)
	if err != nil {
		return nil, fmt.Errorf("failed to insert credentials: %w", err)
	}

	// Create email confirmation (mark as confirmed)
	_, err = tx.Exec(`
		INSERT INTO authservice.email_confirmations
			(id, email, pending, created_at)
		VALUES(uuid_generate_v4(), $1, false, CURRENT_TIMESTAMP)`,
		TestEmail,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert email confirmation: %w", err)
	}

	// Create team
	var teamID int
	err = tx.QueryRow(`
		INSERT INTO "teamService".teams
			(id, "name", description, first_team, default_data_center_id, deleted, deletion_pending, created_at)
		VALUES(nextval('"teamService".teams_id_seq'::regclass), $1, '', true, 1, false, false, CURRENT_TIMESTAMP)
		RETURNING id`,
		TestTeamName,
	).Scan(&teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to insert team: %w", err)
	}

	// Add user to team as owner (role=0)
	_, err = tx.Exec(`
		INSERT INTO "teamService".team_members
			(user_id, team_id, "role", pending, created_at)
		VALUES($1, $2, 0, false, CURRENT_TIMESTAMP)`,
		userID, teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert team member: %w", err)
	}

	// Create API token
	_, err = tx.Exec(`
		INSERT INTO public_api_service.tokens
			(id, "token", user_id, "name", created_at)
		VALUES(nextval('public_api_service.tokens_id_seq'::regclass), $1, $2, 'testkey', now())`,
		hashedToken, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert API token: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	committed = true

	log.Printf("Test user created: email=%s, userID=%d, teamID=%d", TestEmail, userID, teamID)

	return &TestUserResult{
		Email: TestEmail,
	}, nil
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
