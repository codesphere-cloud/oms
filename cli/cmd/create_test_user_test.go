// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"testing"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/spf13/cobra"
)

func TestCreateTestUserFlagsUseKubernetesInsteadOfPostgres(t *testing.T) {
	parent := &cobra.Command{Use: "create"}
	cmd.AddCreateTestUserCmd(parent, &util.GlobalOptions{})

	child := parent.Commands()[0]
	for _, name := range []string{"postgres-host", "postgres-password", "postgres-port", "postgres-user", "postgres-db", "ssl-mode", "seed-authz", "datacenter-id"} {
		if child.Flags().Lookup(name) != nil {
			t.Errorf("obsolete flag %s is still registered", name)
		}
	}

	for _, name := range []string{"namespace", "auth-service", "service-port", "email", "vault", "vault-priv-key", "timeout"} {
		if child.Flags().Lookup(name) == nil {
			t.Errorf("missing flag %s", name)
		}
	}

	child.RunE = func(*cobra.Command, []string) error { return nil }

	parent.SetArgs([]string{"test-user", "--namespace", "test", "--auth-service", "auth-service-dev", "--service-port", "3010", "--email", "test@codesphere.com"})

	if err := parent.Execute(); err != nil {
		t.Fatal(err)
	}
}
