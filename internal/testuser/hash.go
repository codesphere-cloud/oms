// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package testuser

import (
	"crypto/sha256"
	"encoding/hex"
)

// Salts used by Codesphere's auth-service for password hashing.
// Source: https://github.com/codesphere-cloud/codesphere-monorepo/blob/master/packages/auth-service/node/src/ts/config.ts#L14
const (
	salt1 string = "y6E5ZMkjRVtS"
	salt2 string = "EYbWb6pKgWs9T9N8"
)

// HashPassword hashes a password using Codesphere's double-SHA256 scheme with salts.
func HashPassword(password string) string {
	hashed := hashSecret(password, salt1)
	hashed = hashSecret(hashed, salt2)
	return hashed
}

// HashAPIToken hashes an API token using a single SHA256 with no additional salt.
func HashAPIToken(apiToken string) string {
	return hashSecret(apiToken, "")
}

func hashSecret(secret, salt string) string {
	hasher := sha256.New()
	hasher.Write([]byte(secret + salt))
	return hex.EncodeToString(hasher.Sum(nil))
}
