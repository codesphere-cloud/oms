// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/codesphere-cloud/oms/internal/bootstrap/datacenter"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/util"
)

type testUserNodeClient struct {
	command     string
	downloadErr error
	runErr      error
	downloads   int
}

func (c *testUserNodeClient) RunCommand(_ *node.Node, user, command string) error {
	if user != "root" {
		return fmt.Errorf("unexpected SSH user")
	}

	c.command = command

	return c.runErr
}
func (c *testUserNodeClient) CopyFile(*node.Node, string, string) error {
	return fmt.Errorf("unexpected upload")
}
func (c *testUserNodeClient) DownloadFile(_ *node.Node, src, dst string) error {
	c.downloads++

	if src != "/root/oms-test-user/test-user.json" {
		return fmt.Errorf("unexpected credentials path %s", src)
	}

	if c.downloadErr != nil {
		return c.downloadErr
	}

	if err := os.WriteFile(dst, []byte(`{"email":"test+new@codesphere.com","password":"secret","api_token":"CS_test"}`), 0600); err != nil {
		return fmt.Errorf("writing fake downloaded credentials: %w", err)
	}

	return nil
}
func (c *testUserNodeClient) WaitReady(*node.Node, time.Duration) error { return nil }
func (c *testUserNodeClient) HasFile(*node.Node, string) bool           { return false }

func TestBootstrapCreatesUserOnJumpboxUsingPrimaryClusterVault(t *testing.T) {
	client := &testUserNodeClient{}
	workdir := t.TempDir()

	b := &GCPBootstrapper{
		fw: util.NewFilesystemWriter(),
		Env: &CodesphereEnvironment{
			Jumpbox:    &node.Node{NodeClient: client},
			OmsWorkdir: workdir,
			DataCenters: []*datacenter.DataCenter{
				{ID: 1, SecretsDir: "/primary secrets", ConfigManager: &installer.MockInstallConfigManager{}},
				{ID: 2, Suffix: "-dc2", SecretsDir: "/secondary", ConfigManager: &installer.MockInstallConfigManager{}},
			},
		},
	}
	if err := b.createTestUser(); err != nil {
		t.Fatal(err)
	}

	want := "OMS_WORKDIR='/root/oms-test-user' oms create test-user --vault '/primary secrets/prod.vault.yaml' --vault-priv-key '/primary secrets/age_key.txt'"
	if client.command != want {
		t.Errorf("command = %q", client.command)
	}

	data, err := os.ReadFile(filepath.Join(workdir, "test-user.json"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), "CS_test") {
		t.Error("credentials were not copied locally")
	}

	info, err := os.Stat(filepath.Join(workdir, "test-user.json"))
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode().Perm() != 0600 {
		t.Error("credentials are not private")
	}

	client.runErr = fmt.Errorf("API signup failed")

	if err := b.createTestUser(); err == nil {
		t.Error("remote signup failure ignored")
	}

	if client.downloads != 1 {
		t.Error("downloaded stale credentials after failed remote signup")
	}

	client.runErr = nil
	client.downloadErr = fmt.Errorf("download failed")

	if err := b.createTestUser(); err == nil {
		t.Error("download failure ignored")
	}
}
