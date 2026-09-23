// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package steps

import (
	"context"
	"io"

	"github.com/codesphere-cloud/oms/internal/codesphere/teststeps"
)

// SmoketestName is the name the smoketest step is selected by.
const SmoketestName = "smoketest"

func init() {
	register(Step{
		Name:        SmoketestName,
		Description: "Create a workspace, deploy a sample app in it and clean up afterwards",
		Run:         runSmoketest,
	})
}

// runSmoketest runs the full smoke test, which creates a workspace, deploys a
// sample app in it and deletes the workspace again. It logs to stdout rather
// than to out, because the smoke test steps report their own progress.
func runSmoketest(ctx context.Context, _ io.Writer, opts *Options) error {
	return teststeps.RunSmoketest(ctx, &teststeps.SmoketestCodesphereOpts{
		BaseURL: opts.BaseURL,
		Token:   opts.Token,
		TeamID:  opts.TeamID,
		PlanID:  opts.PlanID,
		Profile: opts.Profile,
		Quiet:   opts.Quiet,
		Timeout: opts.Timeout,
		Client:  opts.Client,
	})
}
