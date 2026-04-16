// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/testuser"
	"github.com/codesphere-cloud/oms/internal/util"
)

type CreateTestUserCmd struct {
	cmd  *cobra.Command
	Opts *CreateTestUserOpts
}

type CreateTestUserOpts struct {
	PostgresHost     string
	PostgresPort     int
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	SSLMode          string
}

func (c *CreateTestUserCmd) RunE(_ *cobra.Command, args []string) error {
	result, err := testuser.CreateTestUser(testuser.CreateTestUserOpts{
		Host:     c.Opts.PostgresHost,
		Port:     c.Opts.PostgresPort,
		User:     c.Opts.PostgresUser,
		Password: c.Opts.PostgresPassword,
		DBName:   c.Opts.PostgresDB,
		SSLMode:  c.Opts.SSLMode,
	})
	if err != nil {
		return fmt.Errorf("failed to create test user: %w", err)
	}

	workdir := env.NewEnv().GetOmsWorkdir()
	filePath, err := testuser.WriteResultToFile(result, workdir)
	if err != nil {
		log.Printf("warning: failed to write test user result to file: %v", err)
	} else {
		log.Printf("Test user credentials written to %s", filePath)
	}

	log.Printf("Email:     %s", result.Email)
	log.Printf("Password:  %s", result.PlaintextPassword)
	log.Printf("API Token: %s", result.PlaintextAPIToken)

	return nil
}

func AddCreateTestUserCmd(parent *cobra.Command) {
	c := CreateTestUserCmd{
		cmd: &cobra.Command{
			Use:   "test-user",
			Short: "Create a test user on a Codesphere database",
			Long: io.Long(`Creates a test user with a hashed password and API token directly in a Codesphere
				PostgreSQL database. The user can be used for automated smoke tests.

				The command connects to the specified PostgreSQL instance and creates the necessary
				database records (credentials, email confirmation, team, team membership, API token).

				Credentials are printed to stdout and saved to the OMS workdir as test-user.json.`),
		},
		Opts: &CreateTestUserOpts{},
	}
	c.cmd.RunE = c.RunE

	flags := c.cmd.Flags()
	flags.StringVar(&c.Opts.PostgresHost, "postgres-host", "", "PostgreSQL host address (required)")
	flags.IntVar(&c.Opts.PostgresPort, "postgres-port", 5432, "PostgreSQL port (default: 5432)")
	flags.StringVar(&c.Opts.PostgresUser, "postgres-user", "postgres", "PostgreSQL username (default: postgres)")
	flags.StringVar(&c.Opts.PostgresPassword, "postgres-password", "", "PostgreSQL password (required)")
	flags.StringVar(&c.Opts.PostgresDB, "postgres-db", "codesphere", "PostgreSQL database name (default: codesphere)")
	flags.StringVar(&c.Opts.SSLMode, "ssl-mode", "disable", "PostgreSQL SSL mode (default: disable)")

	util.MarkFlagRequired(c.cmd, "postgres-host")
	util.MarkFlagRequired(c.cmd, "postgres-password")

	AddCmd(parent, c.cmd)
}
