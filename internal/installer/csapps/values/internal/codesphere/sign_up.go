// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// SignUp restricts who can sign up.
type SignUp struct {
	Email SignUpEmail `yaml:"email"`
}

// SignUpEmail restricts sign-ups to email domains.
type SignUpEmail struct {
	AllowedDomains []string `yaml:"allowedDomains,omitempty"`
}

func signUpValues(signUp *files.SignUpConfig) SignUp {
	if signUp == nil || signUp.Email == nil {
		return SignUp{}
	}

	return SignUp{Email: SignUpEmail{AllowedDomains: signUp.Email.AllowedDomains}}
}
