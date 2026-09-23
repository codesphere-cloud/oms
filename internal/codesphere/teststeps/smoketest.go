// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package teststeps

import (
	"context"
	"log"
	"slices"
	"time"
)

const (
	// DefaultTimeout bounds a whole smoke test run.
	DefaultTimeout = 10 * time.Minute
	// DefaultProfile is the CI profile used for landscape and pipeline.
	DefaultProfile = "ci.yml"
)

// AvailableSteps are the smoke test steps, in the order they are run.
var AvailableSteps = []SmokeTestStep{
	&CreateWorkspaceStep{},
	&SetEnvVarStep{},
	&CreateFilesStep{},
	&SyncLandscapeStep{},
	&ExecuteRunStageStep{},
	&DeleteWorkspaceStep{},
}

// StepNames returns the names of all available smoke test steps, in run order.
func StepNames() []string {
	names := make([]string, 0, len(AvailableSteps))
	for _, s := range AvailableSteps {
		names = append(names, s.Name())
	}

	return names
}

// RunSmoketest runs the selected smoke test steps. The passed context bounds
// the run in addition to the configured timeout, so callers that orchestrate
// several tests (see the test command) can cancel it.
func RunSmoketest(ctx context.Context, opts *SmoketestCodesphereOpts) (err error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	stepsToRun := slices.Clone(AvailableSteps)

	if len(opts.Steps) > 0 {
		stepsToRun = slices.DeleteFunc(stepsToRun, func(s SmokeTestStep) bool {
			return !slices.Contains(opts.Steps, s.Name())
		})
	}

	var workspaceID int
	deleteStep := &DeleteWorkspaceStep{}
	defer func() {
		if err != nil {
			log.Printf("Smoketest failed: %s", err.Error())
		}

		shouldDelete := slices.ContainsFunc(stepsToRun, func(s SmokeTestStep) bool {
			return s.Name() == deleteStep.Name()
		})

		if workspaceID != 0 && shouldDelete {
			deleteErr := deleteStep.Run(context.Background(), opts, &workspaceID)
			if deleteErr != nil {
				if err == nil {
					err = deleteErr
				}
			}
		}

		if err == nil {
			log.Println("Smoketest completed successfully!")
		}
	}()

	// Execute steps
	for _, step := range stepsToRun {
		// Skip deleteWorkspace in the main loop as it's handled in defer
		if step.Name() == deleteStep.Name() {
			continue
		}
		if err = step.Run(ctx, opts, &workspaceID); err != nil {
			return err
		}
	}

	return nil
}
