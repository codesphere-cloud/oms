// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package steps

import (
	"context"
	"fmt"
	"io"

	"github.com/codesphere-cloud/oms/internal/codesphere/status"
)

// StatusName is the name the status step is selected by.
const StatusName = "status"

func init() {
	register(Step{
		Name:        StatusName,
		Description: "Report the state of the installation and verify the API answers",
		Run:         runStatus,
	})
}

// runStatus prints the status report of the installation and fails unless it
// is ready. With Options.Wait set it blocks until the installation comes up,
// so it can be used as the gate in front of the steps that need a live API.
func runStatus(ctx context.Context, out io.Writer, opts *Options) error {
	report := status.Fetch(ctx, &status.Options{
		BaseURL: opts.BaseURL,
		Token:   opts.Token,
		Wait:    opts.Wait,
		Timeout: opts.WaitTimeout,
		Client:  opts.Client,
	})

	status.Print(out, opts.BaseURL, report)

	if !report.Ready {
		if report.Err != nil {
			return fmt.Errorf("codesphere installation is not ready: %w", report.Err)
		}

		return fmt.Errorf("codesphere installation is not ready")
	}

	return nil
}
