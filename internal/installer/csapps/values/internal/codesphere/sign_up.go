// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

func signUpValues(signUp *files.SignUpConfig) map[string]any {
	email := map[string]any{}
	if signUp != nil && signUp.Email != nil && signUp.Email.AllowedDomains != nil {
		email["allowedDomains"] = signUp.Email.AllowedDomains
	}

	return map[string]any{"email": email}
}
