// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package teststeps

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/codesphere-cloud/oms/internal/codesphere"
)

const (
	// ANSI color codes
	colorGreen = "\033[32m"
	colorRed   = "\033[31m"
	colorReset = "\033[0m"
)

type SmokeTestStep interface {
	Run(ctx context.Context, c *SmoketestCodesphereOpts, workspaceID *int) error
	Name() string
}

type SmoketestCodesphereOpts struct {
	Client codesphere.Client
	// Configuration options
	BaseURL string
	Token   string
	TeamID  string
	PlanID  string
	Quiet   bool
	Timeout time.Duration
	Profile string
	Steps   []string
}

// Logging helpers

func (c *SmoketestCodesphereOpts) logf(format string, args ...interface{}) {
	if !c.Quiet {
		log.Printf(format, args...)
	}
}

func (c *SmoketestCodesphereOpts) logStep(message string) {
	if !c.Quiet {
		fmt.Printf("%s...", message)
	}
}

func (c *SmoketestCodesphereOpts) logSuccess() {
	if !c.Quiet {
		fmt.Printf(" %ssucceeded%s\n", colorGreen, colorReset)
	}
}

func (c *SmoketestCodesphereOpts) logFailure() {
	if !c.Quiet {
		fmt.Printf(" %sfailed%s\n", colorRed, colorReset)
	}
}
