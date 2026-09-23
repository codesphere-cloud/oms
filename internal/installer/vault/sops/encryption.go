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

// defaultAgeKeyFileName is the key file looked up next to the vault when no key is configured.
const defaultAgeKeyFileName = "age_key.txt"

// envAgeKeyFilePattern names the file MaterializeEnvAgeKey creates. The random suffix keeps it
// distinct from defaultAgeKeyFileName, so the fallback lookup never picks it up.
const envAgeKeyFilePattern = "oms-env-age-key-*"

// ResolveAgeKey resolves an existing age key or generates one in fallbackDir.
func ResolveAgeKey(explicitKeyFile, fallbackDir string) (recipient string, keyPath string, err error) {
	return resolveAgeKey(util.NewFilesystemWriter(), explicitKeyFile, fallbackDir, true)
}

// ResolveExistingAgeKey resolves an existing age key without generating one. It returns the
// path of the key file, or an empty path when the key comes from SOPS_AGE_KEY. Callers use
// it to decrypt a vault they did not create, where generating a fresh key would silently
// produce a key that cannot read the vault.
func ResolveExistingAgeKey(explicitKeyFile, fallbackDir string) (keyPath string, err error) {
	_, keyPath, err = resolveAgeKey(util.NewFilesystemWriter(), explicitKeyFile, fallbackDir, false)

	return keyPath, err
}

// MaterializeEnvAgeKey writes the identity supplied through SOPS_AGE_KEY to a newly created,
// owner-only file in dir and returns its path. Callers that must hand a key file to a subprocess
// use it, because an identity that comes from the environment has no file of its own. A fresh
// file is created instead of reusing a well-known key path, so an identity that already sits
// next to the vault is never overwritten or left with wider permissions.
func MaterializeEnvAgeKey(fileIO util.FileIO, dir string) (keyPath string, err error) {
	raw := strings.TrimSpace(os.Getenv(sopsage.SopsAgeKeyEnv))
	if raw == "" {
		return "", fmt.Errorf("SOPS_AGE_KEY is not set")
	}

	if _, err := parseEnvAgeKey(raw); err != nil {
		return "", err
	}

	if err := fileIO.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create directory for age key: %w", err)
	}

	keyPath, err = fileIO.CreateTemp(dir, envAgeKeyFilePattern)
	if err != nil {
		return "", fmt.Errorf("failed to create age key file: %w", err)
	}

	// A trailing newline matches the file age-keygen writes.
	if err := fileIO.WriteFile(keyPath, []byte(raw+"\n"), 0600); err != nil {
		writeErr := fmt.Errorf("failed to write age key file %s: %w", keyPath, err)

		return "", errors.Join(writeErr, fileIO.Remove(keyPath))
	}

	return keyPath, nil
}

func resolveAgeKey(fileIO util.FileIO, explicitKeyFile, fallbackDir string, generateIfMissing bool) (recipient string, keyPath string, err error) {
	if explicitKeyFile != "" {
		recipient, err = readRecipientFromFile(fileIO, explicitKeyFile)
		if err != nil {
			return "", "", fmt.Errorf("failed to read age key from %s: %w", explicitKeyFile, err)
		}

		return recipient, explicitKeyFile, nil
	}

	if raw := os.Getenv(sopsage.SopsAgeKeyEnv); raw != "" {
		recipient, err = parseEnvAgeKey(raw)
		if err != nil {
			return "", "", err
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

	keyPath = filepath.Join(fallbackDir, defaultAgeKeyFileName)

	recipient, err = readRecipientFromFile(fileIO, keyPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return "", "", fmt.Errorf("failed to read age key from fallback location %s: %w", keyPath, err)
		}

		if !generateIfMissing {
			return "", "", fmt.Errorf("no existing age key found for the SOPS vault; set an age key argument or provide a key at %s", keyPath)
		}

		recipient, err = generateAgeKey(fileIO, keyPath)
		if err != nil {
			return "", "", fmt.Errorf("failed to generate age key: %w", err)
		}
	}

	return recipient, keyPath, nil
}

// parseEnvAgeKey validates the identity passed through SOPS_AGE_KEY and returns its recipient.
func parseEnvAgeKey(raw string) (string, error) {
	recipient, err := parseAgeRecipient(strings.NewReader(strings.TrimSpace(raw)))
	if err != nil {
		return "", fmt.Errorf("failed to parse age key from SOPS_AGE_KEY environment variable: %w", err)
	}

	return recipient, nil
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
