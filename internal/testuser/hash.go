// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package testuser

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashAPIToken hashes an API token using a single SHA256 with no additional salt.
func HashAPIToken(apiToken string) string {
	return hashSecret(apiToken)
}

func hashSecret(secret string) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(secret))
	return hex.EncodeToString(hasher.Sum(nil))
}
