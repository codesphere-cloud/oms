// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// smtpEmailProvider is the only email provider the chart supports.
const smtpEmailProvider = "smtp"

func smtpValues(email *files.EmailConfig) (*files.SMTPConfig, error) {
	if email == nil {
		return nil, nil
	}

	if email.Provider != smtpEmailProvider {
		return nil, fmt.Errorf("email.provider must be %q, got %q", smtpEmailProvider, email.Provider)
	}

	return &email.SMTP, nil
}
