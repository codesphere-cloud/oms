// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"filippo.io/age"
	sopsage "github.com/getsops/sops/v3/age"
)

var (
	xdgConfigHome = "XDG_CONFIG_HOME"
)

// ResolveAgeKey resolves an existing age key or generates a new one.
// It checks (in order):
//  1. SOPS_AGE_KEY environment variable (raw key content)
//  2. SOPS_AGE_KEY_FILE environment variable (path to key file)
//  3. Default location: ~/.config/sops/age/keys.txt
//  4. Generate a new key and write it to <fallbackDir>/age_key.txt
//
// Returns the age public key (recipient) and the path to the key file (empty when
// the key was supplied via SOPS_AGE_KEY).
func ResolveAgeKey(fallbackDir string) (recipient string, keyPath string, err error) {
	// 1. SOPS_AGE_KEY env var – contains raw key content.
	if raw := os.Getenv(sopsage.SopsAgeKeyEnv); raw != "" {
		recipient, err = parseAgeRecipient(strings.NewReader(raw))
		if err != nil {
			return "", "", fmt.Errorf("failed to parse age key from SOPS_AGE_KEY environment variable: %w", err)
		}
		return recipient, "", nil
	}

	// 2. SOPS_AGE_KEY_FILE env var.
	if keyFile := os.Getenv(sopsage.SopsAgeKeyFileEnv); keyFile != "" {
		recipient, err = readRecipientFromFile(keyFile)
		if err != nil {
			return "", "", fmt.Errorf("failed to read age key from %s: %w", keyFile, err)
		}
		return recipient, keyFile, nil
	}

	// 3. Default location: ~/.config/sops/age/keys.txt.
	defaultPath, configErr := getUserConfigDir()
	if configErr == nil {
		defaultPath = filepath.Join(defaultPath, sopsage.SopsAgeKeyUserConfigPath)
		recipient, err = readRecipientFromFile(defaultPath)
		if err == nil {
			return recipient, defaultPath, nil
		}
		if !os.IsNotExist(err) {
			return "", "", fmt.Errorf("failed to read age key from default location %s: %w", defaultPath, err)
		}
	}

	// 4. Generate a new key.
	keyPath = filepath.Join(fallbackDir, "age_key.txt")
	recipient, err = readRecipientFromFile(keyPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", "", fmt.Errorf("failed to read age key from fallback location %s: %w", keyPath, err)
		}
		// File does not exist, will generate a new key.
		recipient, err = generateAgeKey(keyPath)
		if err != nil {
			return "", "", fmt.Errorf("failed to generate age key: %w", err)
		}
		return recipient, keyPath, nil
	}
	return recipient, keyPath, nil
}

// parseAgeRecipient extracts the public key from age key given by reader.
func parseAgeRecipient(reader io.Reader) (string, error) {
	ids, err := age.ParseIdentities(reader)
	if err != nil {
		return "", fmt.Errorf("failed to parse age identities from file: %w", err)
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("no age identities found in file")
	}
	if len(ids) > 1 {
		return "", fmt.Errorf("multiple age identities found in file, expected only one")
	}
	id := ids[0]
	switch id := id.(type) {
	case *age.X25519Identity:
		return id.Recipient().String(), nil
	case *age.HybridIdentity:
		return id.Recipient().String(), nil
	default:
		return "", fmt.Errorf("internal error: unexpected identity type: %T", id)
	}
}

// readRecipientFromFile reads an age key file and extracts the public key.
func readRecipientFromFile(path string) (recipient string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		err = file.Close()
	}()
	return parseAgeRecipient(file)
}

func getUserConfigDir() (string, error) {
	if runtime.GOOS == "darwin" {
		if userConfigDir, ok := os.LookupEnv(xdgConfigHome); ok && userConfigDir != "" {
			return userConfigDir, nil
		}
	}
	return os.UserConfigDir()
}

// generateAgeKey generates a new age keypair and writes it to the given path.
// Returns the public key (recipient).
func generateAgeKey(keyPath string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return "", fmt.Errorf("failed to create directory for age key: %w", err)
	}

	cmd := exec.Command("age-keygen", "-o", keyPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("age-keygen failed: %w: %s", err, out)
	}

	// Read back the generated file to extract the public key.
	recipient, err := readRecipientFromFile(keyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read generated age key: %w", err)
	}
	return recipient, nil
}

// EncryptFileWithSOPS encrypts a file in-place using SOPS with the given age recipient.
func EncryptFileWithSOPS(src, target, recipient string) error {
	cmd := exec.Command("sops", "--encrypt", "--age", recipient, "--output", target, src)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sops encrypt failed: %w: %s", err, out)
	}
	return nil
}
