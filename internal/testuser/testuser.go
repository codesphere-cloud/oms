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
	testEmail    = "test@codesphere.com"
	testPassword = "Test1234!"
	testTeamName = "Tests"
	tokenPrefix  = "CS_"
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
		opts.Port = 5432
	}
	if opts.User == "" {
		opts.User = "postgres"
	}
	if opts.DBName == "" {
		opts.DBName = "codesphere"
	}
	if opts.SSLMode == "" {
		opts.SSLMode = "disable"
	}

	plaintextToken, err := generateAPIToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API token: %w", err)
	}

	hashedPassword := HashPassword(testPassword)
	hashedToken := HashAPIToken(plaintextToken)

	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=10",
		opts.Host, opts.Port, opts.User, opts.Password, opts.DBName, opts.SSLMode,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}
	defer db.Close()

	db.SetConnMaxLifetime(30 * time.Second)
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to database at %s:%d: %w", opts.Host, opts.Port, err)
	}

	log.Printf("Connected to PostgreSQL at %s:%d", opts.Host, opts.Port)

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
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
		testEmail, hashedPassword,
	).Scan(&userID)
	if err != nil {
		return nil, fmt.Errorf("failed to insert credentials: %w", err)
	}

	// Create email confirmation (mark as confirmed)
	_, err = tx.Exec(`
		INSERT INTO authservice.email_confirmations
			(id, email, pending, created_at)
		VALUES(uuid_generate_v4(), $1, false, CURRENT_TIMESTAMP)`,
		testEmail,
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
		testTeamName,
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

	log.Printf("Test user created: email=%s, userID=%d, teamID=%d", testEmail, userID, teamID)

	return &TestUserResult{
		Email:             testEmail,
		PlaintextPassword: testPassword,
		PlaintextAPIToken: plaintextToken,
	}, nil
}

// WriteResultToFile writes the test user result to a JSON file in the given directory.
func WriteResultToFile(result *TestUserResult, dir string) (string, error) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal test user result: %w", err)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
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
