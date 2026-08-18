// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package sops

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"filippo.io/age"
	"github.com/codesphere-cloud/oms/internal/util"
	sopsage "github.com/getsops/sops/v3/age"
)

var xdgConfigHome = "XDG_CONFIG_HOME"

// ResolveAgeKey resolves an existing age key or generates one in fallbackDir.
func ResolveAgeKey(explicitKeyFile, fallbackDir string) (recipient string, keyPath string, err error) {
	return resolveAgeKey(util.NewFilesystemWriter(), explicitKeyFile, fallbackDir)
}

func resolveAgeKey(fileIO util.FileIO, explicitKeyFile, fallbackDir string) (recipient string, keyPath string, err error) {
	if explicitKeyFile != "" {
		recipient, err = readRecipientFromFile(fileIO, explicitKeyFile)
		if err != nil {
			return "", "", fmt.Errorf("failed to read age key from %s: %w", explicitKeyFile, err)
		}

		return recipient, explicitKeyFile, nil
	}

	if raw := os.Getenv(sopsage.SopsAgeKeyEnv); raw != "" {
		recipient, err = parseAgeRecipient(strings.NewReader(raw))
		if err != nil {
			return "", "", fmt.Errorf("failed to parse age key from SOPS_AGE_KEY environment variable: %w", err)
		}

		return recipient, "", nil
	}

	if keyFile := os.Getenv(sopsage.SopsAgeKeyFileEnv); keyFile != "" {
		recipient, err = readRecipientFromFile(fileIO, keyFile)
		if err != nil {
			return "", "", fmt.Errorf("failed to read age key from %s: %w", keyFile, err)
		}

		return recipient, keyFile, nil
	}

	defaultPath, configErr := getUserConfigDir()
	if configErr == nil {
		defaultPath = filepath.Join(defaultPath, sopsage.SopsAgeKeyUserConfigPath)

		recipient, err = readRecipientFromFile(fileIO, defaultPath)
		if err == nil {
			return recipient, defaultPath, nil
		}

		if !errors.Is(err, fs.ErrNotExist) {
			return "", "", fmt.Errorf("failed to read age key from default location %s: %w", defaultPath, err)
		}
	}

	keyPath = filepath.Join(fallbackDir, "age_key.txt")

	recipient, err = readRecipientFromFile(fileIO, keyPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return "", "", fmt.Errorf("failed to read age key from fallback location %s: %w", keyPath, err)
		}

		recipient, err = generateAgeKey(fileIO, keyPath)
		if err != nil {
			return "", "", fmt.Errorf("failed to generate age key: %w", err)
		}
	}

	return recipient, keyPath, nil
}

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

	switch id := ids[0].(type) {
	case *age.X25519Identity:
		return id.Recipient().String(), nil
	case *age.HybridIdentity:
		return id.Recipient().String(), nil
	default:
		return "", fmt.Errorf("internal error: unexpected identity type: %T", id)
	}
}

func readRecipientFromFile(fileIO util.FileIO, path string) (string, error) {
	data, err := fileIO.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read age key file %s: %w", path, err)
	}

	return parseAgeRecipient(strings.NewReader(string(data)))
}

func getUserConfigDir() (string, error) {
	if runtime.GOOS == "darwin" {
		if userConfigDir, ok := os.LookupEnv(xdgConfigHome); ok && userConfigDir != "" {
			return userConfigDir, nil
		}
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve user config directory: %w", err)
	}

	return configDir, nil
}

func generateAgeKey(fileIO util.FileIO, keyPath string) (string, error) {
	if err := fileIO.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return "", fmt.Errorf("failed to create directory for age key: %w", err)
	}

	cmd := exec.Command("age-keygen", "-o", keyPath)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("age-keygen failed: %w: %s", err, out)
	}

	recipient, err := readRecipientFromFile(fileIO, keyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read generated age key: %w", err)
	}

	return recipient, nil
}

// EncryptFile encrypts src with SOPS and age and writes ciphertext to target.
func EncryptFile(src, target, recipient string) error {
	cmd := exec.Command("sops", "--encrypt", "--input-type", "yaml", "--age", recipient, "--output", target, src)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sops encrypt failed: %w: %s", err, out)
	}

	return nil
}

// DecryptFile decrypts a SOPS-encrypted file and returns its plaintext.
func DecryptFile(src, keyPath string) ([]byte, error) {
	cmd := exec.Command("sops", "--decrypt", "--input-type", "yaml", src)
	if keyPath != "" {
		cmd.Env = append(os.Environ(), "SOPS_AGE_KEY_FILE="+keyPath)
	}

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("sops decrypt failed: %s", string(exitErr.Stderr))
		}

		return nil, fmt.Errorf("sops decrypt failed: %w", err)
	}

	return out, nil
}
