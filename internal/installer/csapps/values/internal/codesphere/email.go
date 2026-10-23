// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// SMTP is the server Codesphere sends emails through. The password comes from the vault.
type SMTP struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Secure   bool   `yaml:"secure"`
	Username string `yaml:"username,omitempty"`
}

func smtpValues(email *files.EmailConfig) *SMTP {
	if email == nil {
		return nil
	}

	return &SMTP{
		Host:     email.SMTP.Host,
		Port:     email.SMTP.Port,
		Secure:   email.SMTP.Secure,
		Username: email.SMTP.Username,
	}
}
