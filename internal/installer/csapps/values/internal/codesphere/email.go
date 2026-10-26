// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

func smtpValues(email *files.EmailConfig) *files.SMTPConfig {
	if email == nil {
		return nil
	}

	return &email.SMTP
}
