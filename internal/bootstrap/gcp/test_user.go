// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/codesphere-cloud/oms/internal/testuser"
)

// The jumpbox has network access to the primary Kubernetes API and its vault
// contains the kubeconfig written by the installer. No DB credentials or
// generated user credentials are passed in the SSH command.
func (b *GCPBootstrapper) createTestUser() error {
	if err := b.ensureDataCenters(); err != nil {
		return err
	}

	if b.Env.Jumpbox == nil {
		return fmt.Errorf("jumpbox not found in bootstrap environment")
	}

	dc := b.primaryDC()

	const remoteWorkdir = "/root/oms-test-user"

	command := fmt.Sprintf("OMS_WORKDIR=%s oms create test-user --vault %s --vault-priv-key %s",
		shellQuote(remoteWorkdir), shellQuote(dc.RemoteVaultPath()), shellQuote(dc.RemoteAgeKeyPath()))

	if err := b.Env.Jumpbox.RunSSHCommand("root", command); err != nil {
		return fmt.Errorf("creating test user through the internal auth API on the jumpbox: %w", err)
	}

	localPath, err := b.fw.CreateTemp("", "oms-test-user-*.json")
	if err != nil {
		return fmt.Errorf("creating temporary credentials file: %w", err)
	}
	defer func() { _ = b.fw.Remove(localPath) }()

	if err := b.Env.Jumpbox.NodeClient.DownloadFile(b.Env.Jumpbox, remoteWorkdir+"/test-user.json", localPath); err != nil {
		return fmt.Errorf("downloading test-user credentials from jumpbox: %w", err)
	}

	data, err := b.fw.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("reading downloaded test-user credentials: %w", err)
	}

	var result testuser.TestUserResult
	if err := json.Unmarshal(data, &result); err != nil || result.Email == "" || result.PlaintextPassword == "" || result.PlaintextAPIToken == "" {
		return fmt.Errorf("jumpbox returned invalid test-user credentials")
	}

	file, err := testuser.WriteResultToFile(&result, b.Env.OmsWorkdir)
	if err != nil {
		return fmt.Errorf("saving test-user credentials locally: %w", err)
	}

	log.Printf("Test user credentials written to %s", file)

	return nil
}
