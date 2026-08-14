// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiCyan  = "\x1b[36m"
	ansiGreen = "\x1b[32m"
	ansiRed   = "\x1b[31m"
)

// logo is a small ASCII mark printed next to the status report, neofetch-style.
var logo = []string{
	"      ▄▄▄▄▄▄▄▄▄▄▄▄      ",
	"   ▄█████████████████▄   ",
	" ▄███▀▀▀        ▀▀▀███▄ ",
	"███                ████",
	"██    ▄▄▄▄▄▄▄▄▄     ███",
	"██   ███████████    ███",
	"██   ███████████    ███",
	"██    ▀▀▀▀▀▀▀▀▀     ███",
	"████                ████",
	" ▀███▄▄▄        ▄▄▄███▀ ",
	"   ▀██████████████████▀   ",
	"      ▀▀▀▀▀▀▀▀▀▀▀▀      ",
}

type statusReport struct {
	Ready    bool
	Latency  time.Duration
	Teams    int
	Plans    int
	Attempts int
	Err      error
}

// fetchStatus pings the Codesphere API with a cheap, side-effect-free call
// (ListWorkspacePlans) to determine readiness. With Wait set, it retries on
// failure until the installation becomes ready or opts.Timeout elapses.
func fetchStatus(ctx context.Context, opts *StatusCodesphereOpts) *statusReport {
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	report := &statusReport{}
	for {
		report.Attempts++

		start := time.Now()
		plans, err := opts.Client.ListWorkspacePlans()
		report.Latency = time.Since(start)

		if err == nil {
			report.Ready = true

			report.Plans = len(plans)
			if teams, terr := opts.Client.ListTeams(""); terr == nil {
				report.Teams = len(teams)
			}

			return report
		}

		report.Err = err

		if !opts.Wait {
			return report
		}

		select {
		case <-ctx.Done():
			return report
		case <-time.After(statusPollInterval):
		}
	}
}

// printStatus renders a neofetch-style report: a small ASCII logo alongside
// key/value status lines.
func printStatus(w io.Writer, baseURL string, r *statusReport) {
	host := baseURL
	if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
		host = u.Host
	}

	statusColor, statusText := ansiGreen, "Ready"
	if !r.Ready {
		statusColor, statusText = ansiRed, "Not Ready"
	}

	header := fmt.Sprintf("%s%scodesphere%s@%s", ansiBold, ansiCyan, ansiReset, host)
	rule := strings.Repeat("-", len("codesphere@")+len(host))

	lines := []string{
		header,
		rule,
		fmt.Sprintf("%sStatus%s: %s%s%s", ansiBold, ansiReset, statusColor, statusText, ansiReset),
		fmt.Sprintf("%sLatency%s: %s", ansiBold, ansiReset, r.Latency.Round(time.Millisecond)),
	}
	if r.Ready {
		lines = append(lines,
			fmt.Sprintf("%sTeams%s: %d", ansiBold, ansiReset, r.Teams),
			fmt.Sprintf("%sPlans%s: %d", ansiBold, ansiReset, r.Plans),
		)
	} else {
		lines = append(lines, fmt.Sprintf("%sError%s: %s", ansiBold, ansiReset, r.Err))
	}

	if r.Attempts > 1 {
		lines = append(lines, fmt.Sprintf("%sAttempts%s: %d", ansiBold, ansiReset, r.Attempts))
	}

	rows := len(logo)
	if len(lines) > rows {
		rows = len(lines)
	}

	// Pad the logo to a fixed width so the status lines form a straight column.
	logoWidth := 0
	for _, l := range logo {
		if n := len([]rune(l)); n > logoWidth {
			logoWidth = n
		}
	}

	_, _ = fmt.Fprintln(w)

	for i := 0; i < rows; i++ {
		logoLine := ""
		if i < len(logo) {
			logoLine = logo[i]
		}

		logoLine += strings.Repeat(" ", logoWidth-len([]rune(logoLine)))

		statLine := ""
		if i < len(lines) {
			statLine = lines[i]
		}

		_, _ = fmt.Fprintf(w, "  %s%s%s  %s\n", ansiCyan, logoLine, ansiReset, statLine)
	}

	_, _ = fmt.Fprintln(w)
}
