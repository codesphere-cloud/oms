// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

func smtpValues(email *files.EmailConfig) any {
	if email == nil {
		return nil
	}

	smtp := map[string]any{
		"host":   email.SMTP.Host,
		"port":   email.SMTP.Port,
		"secure": email.SMTP.Secure,
	}
	if email.SMTP.Username != "" {
		smtp["username"] = email.SMTP.Username
	}

	return smtp
}
