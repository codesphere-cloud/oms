// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package testuser

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// HashPassword hashes a password using Codesphere's double-SHA256 scheme with salts.
// The salts are read from environment variables OMS_CS_SALT_1 and OMS_CS_SALT_2.
func HashPassword(password string) (string, error) {
	salt1 := os.Getenv("OMS_CS_SALT_1")
	if salt1 == "" {
		return "", fmt.Errorf("OMS_CS_SALT_1 environment variable is not set")
	}
	salt2 := os.Getenv("OMS_CS_SALT_2")
	if salt2 == "" {
		return "", fmt.Errorf("OMS_CS_SALT_2 environment variable is not set")
	}

	hashed := hashSecret(password, salt1)
	hashed = hashSecret(hashed, salt2)
	return hashed, nil
}

// HashAPIToken hashes an API token using a single SHA256 with no additional salt.
func HashAPIToken(apiToken string) string {
	return hashSecret(apiToken, "")
}

func hashSecret(secret, salt string) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(secret + salt))
	return hex.EncodeToString(hasher.Sum(nil))
}
