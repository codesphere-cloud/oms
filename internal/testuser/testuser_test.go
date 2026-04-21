// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package testuser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("generateAPIToken", func() {
	It("returns no error", func() {
		_, err := generateAPIToken()
		Expect(err).NotTo(HaveOccurred())
	})

	It("starts with the CS_ prefix", func() {
		token, err := generateAPIToken()
		Expect(err).NotTo(HaveOccurred())
		Expect(token).To(HavePrefix("CS_"))
	})

	It("has the correct length (3 prefix + 32 hex chars)", func() {
		token, err := generateAPIToken()
		Expect(err).NotTo(HaveOccurred())
		Expect(token).To(HaveLen(35))
	})

	It("produces unique tokens on successive calls", func() {
		token1, err := generateAPIToken()
		Expect(err).NotTo(HaveOccurred())
		token2, err := generateAPIToken()
		Expect(err).NotTo(HaveOccurred())
		Expect(token1).NotTo(Equal(token2))
	})

	It("contains only hex characters after the prefix", func() {
		token, err := generateAPIToken()
		Expect(err).NotTo(HaveOccurred())
		hexPart := token[len(tokenPrefix):]
		Expect(hexPart).To(MatchRegexp("^[0-9a-f]{32}$"))
	})
})

var _ = Describe("WriteResultToFile", func() {
	It("writes a valid JSON file to the given directory", func() {
		dir := GinkgoT().TempDir()
		result := &TestUserResult{
			Email:             "test@example.com",
			PlaintextPassword: "secret123",
			PlaintextAPIToken: "CS_abc123",
		}

		filePath, err := WriteResultToFile(result, dir)
		Expect(err).NotTo(HaveOccurred())
		Expect(filePath).To(Equal(filepath.Join(dir, "test-user.json")))

		data, err := os.ReadFile(filePath)
		Expect(err).NotTo(HaveOccurred())

		var loaded TestUserResult
		err = json.Unmarshal(data, &loaded)
		Expect(err).NotTo(HaveOccurred())
		Expect(loaded.Email).To(Equal("test@example.com"))
		Expect(loaded.PlaintextPassword).To(Equal("secret123"))
		Expect(loaded.PlaintextAPIToken).To(Equal("CS_abc123"))
	})

	It("creates the directory if it does not exist", func() {
		dir := filepath.Join(GinkgoT().TempDir(), "nested", "subdir")
		result := &TestUserResult{
			Email:             "user@example.com",
			PlaintextPassword: "pass",
			PlaintextAPIToken: "CS_token",
		}

		filePath, err := WriteResultToFile(result, dir)
		Expect(err).NotTo(HaveOccurred())
		Expect(filePath).To(BeARegularFile())
	})

	It("sets restrictive file permissions (0600)", func() {
		dir := GinkgoT().TempDir()
		result := &TestUserResult{
			Email:             "user@example.com",
			PlaintextPassword: "pass",
			PlaintextAPIToken: "CS_token",
		}

		filePath, err := WriteResultToFile(result, dir)
		Expect(err).NotTo(HaveOccurred())

		info, err := os.Stat(filePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
	})
})

var _ = Describe("generatePassword", func() {
	It("returns no error", func() {
		_, err := generatePassword()
		Expect(err).NotTo(HaveOccurred())
	})

	It("has the correct length (32 hex chars from 16 bytes)", func() {
		password, err := generatePassword()
		Expect(err).NotTo(HaveOccurred())
		Expect(password).To(HaveLen(32))
	})

	It("produces unique passwords on successive calls", func() {
		pw1, err := generatePassword()
		Expect(err).NotTo(HaveOccurred())
		pw2, err := generatePassword()
		Expect(err).NotTo(HaveOccurred())
		Expect(pw1).NotTo(Equal(pw2))
	})

	It("contains only hex characters", func() {
		password, err := generatePassword()
		Expect(err).NotTo(HaveOccurred())
		Expect(password).To(MatchRegexp("^[0-9a-f]{32}$"))
	})
})

var _ = Describe("createInDB", func() {
	const (
		hashedPassword = "fakehashedpassword"
		hashedToken    = "fakehashedtoken"
	)

	It("creates a test user successfully", func() {
		sqlDB, m, err := sqlmock.New()
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = sqlDB.Close() }()

		// Expect: check if user exists
		m.ExpectQuery(`SELECT EXISTS`).
			WithArgs(TestEmail).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		// Expect: begin transaction
		m.ExpectBegin()

		// Expect: insert credentials
		m.ExpectQuery(`INSERT INTO authservice.credentials`).
			WithArgs(TestEmail, hashedPassword).
			WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(42))

		// Expect: insert email confirmation
		m.ExpectExec(`INSERT INTO authservice.email_confirmations`).
			WithArgs(sqlmock.AnyArg(), TestEmail).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// Expect: insert team
		m.ExpectQuery(`INSERT INTO "teamService".teams`).
			WithArgs(TestTeamName).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))

		// Expect: insert team member
		m.ExpectExec(`INSERT INTO "teamService".team_members`).
			WithArgs(42, 7).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// Expect: insert API token
		m.ExpectExec(`INSERT INTO public_api_service.tokens`).
			WithArgs(hashedToken, 42).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// Expect: commit
		m.ExpectCommit()

		result, err := newWithDB(CreateTestUserOpts{Host: "test", Password: "test"}, sqlDB).createInDB(hashedPassword, hashedToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Email).To(Equal(TestEmail))
		Expect(m.ExpectationsWereMet()).NotTo(HaveOccurred())
	})

	It("returns an error when the test user already exists", func() {
		sqlDB, m, err := sqlmock.New()
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = sqlDB.Close() }()

		m.ExpectQuery(`SELECT EXISTS`).
			WithArgs(TestEmail).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		_, err = newWithDB(CreateTestUserOpts{Host: "test", Password: "test"}, sqlDB).createInDB(hashedPassword, hashedToken)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("already exists"))
		Expect(m.ExpectationsWereMet()).NotTo(HaveOccurred())
	})

	It("rolls back the transaction on credential insert failure", func() {
		sqlDB, m, err := sqlmock.New()
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = sqlDB.Close() }()

		m.ExpectQuery(`SELECT EXISTS`).
			WithArgs(TestEmail).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		m.ExpectBegin()
		m.ExpectQuery(`INSERT INTO authservice.credentials`).
			WithArgs(TestEmail, hashedPassword).
			WillReturnError(fmt.Errorf("unique_violation"))
		m.ExpectRollback()

		_, err = newWithDB(CreateTestUserOpts{Host: "test", Password: "test"}, sqlDB).createInDB(hashedPassword, hashedToken)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to insert credentials"))
		Expect(m.ExpectationsWereMet()).NotTo(HaveOccurred())
	})

	It("rolls back the transaction on team insert failure", func() {
		sqlDB, m, err := sqlmock.New()
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = sqlDB.Close() }()

		m.ExpectQuery(`SELECT EXISTS`).
			WithArgs(TestEmail).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		m.ExpectBegin()
		m.ExpectQuery(`INSERT INTO authservice.credentials`).
			WithArgs(TestEmail, hashedPassword).
			WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(42))
		m.ExpectExec(`INSERT INTO authservice.email_confirmations`).
			WithArgs(sqlmock.AnyArg(), TestEmail).
			WillReturnResult(sqlmock.NewResult(1, 1))
		m.ExpectQuery(`INSERT INTO "teamService".teams`).
			WithArgs(TestTeamName).
			WillReturnError(fmt.Errorf("db error"))
		m.ExpectRollback()

		_, err = newWithDB(CreateTestUserOpts{Host: "test", Password: "test"}, sqlDB).createInDB(hashedPassword, hashedToken)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to insert team"))
		Expect(m.ExpectationsWereMet()).NotTo(HaveOccurred())
	})
})
