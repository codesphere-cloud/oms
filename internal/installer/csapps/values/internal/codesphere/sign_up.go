// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// SignUp restricts who can sign up.
type SignUp struct {
	Email files.SignUpEmailConfig `yaml:"email"`
}

func signUpValues(signUp *files.SignUpConfig) SignUp {
	if signUp == nil || signUp.Email == nil {
		return SignUp{}
	}

	return SignUp{Email: *signUp.Email}
}
