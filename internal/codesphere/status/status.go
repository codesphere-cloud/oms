// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package status reports whether a Codesphere installation is reachable and
// ready. It backs both the status command and the status test playlist step.
package status

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/codesphere-cloud/oms/internal/codesphere"
)

const (
	// DefaultTimeout bounds how long Fetch waits for the installation to
	// become ready when Options.Wait is set.
	DefaultTimeout = 5 * time.Minute
	// pollInterval is how long Fetch waits between two readiness checks.
	pollInterval = 5 * time.Second

	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiCyan  = "\x1b[36m"
	ansiGreen = "\x1b[32m"
	ansiRed   = "\x1b[31m"
)

// logoArt is the Codesphere mark printed next to the status report,
// neofetch-style. It lives in its own file so it can be edited as artwork:
// pasting block characters straight into a Go string literal breaks the build,
// and gofmt has opinions about what it finds in one.
//
// The file carries its own SGR escapes, so the mark can be recolored without
// touching this package. Every colored run is closed with a reset, which is
// why Print measures the lines with visibleWidth rather than len.
//
// The brand palette is truecolor rather than a palette index, so the ring
// keeps its hue regardless of how a terminal theme maps the 16 base colors:
// the outer ring is #7a4eeb (purple) and the inner ring #2ed9d0 (turquoise).
//
//go:embed logo.txt
var logoArt string

// ansiPattern matches the SGR escape sequences the artwork colors itself with.
var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

// logo is the mark split into the lines Print renders it by.
var logo = logoLines(logoArt)

// logoLines splits the embedded artwork, tolerating CRLF line endings and the
// trailing newline every sane editor leaves behind.
func logoLines(art string) []string {
	lines := strings.Split(strings.Trim(art, "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, "\r")
	}

	return lines
}

// visibleWidth is the number of cells a line occupies on screen, ignoring the
// SGR escapes that take up bytes but no space.
func visibleWidth(s string) int {
	return len([]rune(ansiPattern.ReplaceAllString(s, "")))
}

// Options configures the status report of a Codesphere installation.
type Options struct {
	BaseURL string
	Token   string
	Wait    bool
	Timeout time.Duration
	Client  codesphere.Client
}

// Report is the outcome of a readiness check.
type Report struct {
	Ready    bool
	Latency  time.Duration
	Teams    int
	Plans    int
	Attempts int
	Err      error
}

// Fetch pings the Codesphere API with a cheap, side-effect-free call
// (ListWorkspacePlans) to determine readiness. With Wait set, it retries on
// failure until the installation becomes ready or opts.Timeout elapses.
func Fetch(ctx context.Context, opts *Options) *Report {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	report := &Report{}
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
		case <-time.After(pollInterval):
		}
	}
}

// Print renders a neofetch-style report: a small ASCII logo alongside
// key/value status lines.
func Print(w io.Writer, baseURL string, r *Report) {
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
		if n := visibleWidth(l); n > logoWidth {
			logoWidth = n
		}
	}

	_, _ = fmt.Fprintln(w)

	for i := 0; i < rows; i++ {
		logoLine := ""
		if i < len(logo) {
			logoLine = logo[i]
		}

		logoLine += strings.Repeat(" ", logoWidth-visibleWidth(logoLine))

		statLine := ""
		if i < len(lines) {
			statLine = lines[i]
		}

		_, _ = fmt.Fprintf(w, "  %s  %s\n", logoLine, statLine)
	}

	_, _ = fmt.Fprintln(w)
}
