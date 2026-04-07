// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// GetDurationFromString parses a string in the format "<days>d" and returns the corresponding time.Duration.
// It is intentionally built to be easily extensible in the future if we want to support more duration formats (e.g., "1h", "3m", etc.).
func GetDurationFromString(validFor string) (time.Duration, error) {
	if !strings.HasSuffix(validFor, "d") {
		return 0, fmt.Errorf("failed to parse valid-for duration: expected format '<days>d', got %q", validFor)
	}

	days, parseErr := strconv.Atoi(strings.TrimSuffix(validFor, "d"))
	if parseErr != nil || days <= 0 {
		return 0, fmt.Errorf("failed to parse valid-for duration: expected format '<days>d' with days > 0, got %q", validFor)
	}

	return time.Duration(days) * 24 * time.Hour, nil
}
