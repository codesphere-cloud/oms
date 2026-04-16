// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/testuser"
	"github.com/codesphere-cloud/oms/internal/util"
)

type CreateTestUserCmd struct {
	cmd  *cobra.Command
	Opts CreateTestUserOpts
	Env  env.Env
}

type CreateTestUserOpts struct {
	*GlobalOptions
	testuser.CreateTestUserOpts
}

func (c *CreateTestUserCmd) RunE(_ *cobra.Command, args []string) error {
	result, err := testuser.CreateTestUser(c.Opts.CreateTestUserOpts)
	if err != nil {
		return fmt.Errorf("failed to create test user: %w", err)
	}

	testuser.LogAndPersistResult(result, c.Env.GetOmsWorkdir())

	return nil
}

func AddCreateTestUserCmd(parent *cobra.Command, opts *GlobalOptions) {
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
		Opts: CreateTestUserOpts{GlobalOptions: opts},
		Env:  env.NewEnv(),
	}
	c.cmd.RunE = c.RunE

	flags := c.cmd.Flags()
	flags.StringVar(&c.Opts.Host, "postgres-host", "", "PostgreSQL host address (required)")
	flags.IntVar(&c.Opts.Port, "postgres-port", testuser.DefaultPort, "PostgreSQL port")
	flags.StringVar(&c.Opts.User, "postgres-user", testuser.DefaultUser, "PostgreSQL username")
	flags.StringVar(&c.Opts.Password, "postgres-password", "", "PostgreSQL password (required)")
	flags.StringVar(&c.Opts.DBName, "postgres-db", testuser.DefaultDBName, "PostgreSQL database name")
	flags.StringVar(&c.Opts.SSLMode, "ssl-mode", testuser.DefaultSSLMode, "PostgreSQL SSL mode")

	util.MarkFlagRequired(c.cmd, "postgres-host")
	util.MarkFlagRequired(c.cmd, "postgres-password")

	AddCmd(parent, c.cmd)
}
