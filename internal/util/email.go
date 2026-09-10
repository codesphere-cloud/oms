// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"
	"net/mail"
)

// ValidateEmail checks that the given string is a syntactically valid email address.
func ValidateEmail(email string) error {
	if _, err := mail.ParseAddress(email); err != nil {
		return fmt.Errorf("expected a valid email address, got %q", email)
	}

	return nil
}
