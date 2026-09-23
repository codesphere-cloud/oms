// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package steps holds every step a test playlist can be built from.
//
// Each step lives in its own file in this folder and registers itself from an
// init function, so adding a step to the catalog means dropping a file in
// here — nothing else has to be edited. The playlists that group the steps are
// defined in playlists.go.
package steps

import (
	"context"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/codesphere-cloud/oms/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/codesphere/testplan"
)

// Options configures a test run against a Codesphere installation. Every step
// is handed the same options, and reads the ones it needs.
type Options struct {
	BaseURL     string
	Token       string
	TeamID      string
	PlanID      string
	Profile     string
	Wait        bool
	WaitTimeout time.Duration
	Timeout     time.Duration
	Quiet       bool
	Client      codesphere.Client
}

// Step is one possible entry of a test playlist.
type Step struct {
	// Name is how the step is selected, on the command line and in playlists.
	Name string
	// Description is what the step does, as shown in listings and progress logs.
	Description string
	// Run executes the step. Anything it writes to out ends up in the test log.
	Run func(ctx context.Context, out io.Writer, opts *Options) error
}

// catalog holds the registered steps. It is only written from the init
// functions of this package, which run before any other code can read it.
var catalog []Step

// register adds a step to the catalog. Step files call it from their init
// function; a duplicate name is a programming error and panics at startup.
func register(s Step) {
	if slices.ContainsFunc(catalog, func(c Step) bool { return c.Name == s.Name }) {
		panic("testplan/steps: duplicate step name " + s.Name)
	}

	catalog = append(catalog, s)
}

// Catalog returns every registered step, sorted by name so listings do not
// depend on the order the step files happen to be initialized in.
func Catalog() []Step {
	sorted := slices.Clone(catalog)
	slices.SortFunc(sorted, func(a, b Step) int { return strings.Compare(a.Name, b.Name) })

	return sorted
}

// Names returns the names of every registered step, sorted.
func Names() []string {
	steps := Catalog()

	names := make([]string, 0, len(steps))
	for _, s := range steps {
		names = append(names, s.Name)
	}

	return names
}

// Tests binds the catalog to opts, so the steps can be run by a testplan
// Runner. The steps keep the pointer, so options set after this call — the
// client, for example — are still picked up.
func Tests(opts *Options) []testplan.Test {
	steps := Catalog()

	tests := make([]testplan.Test, 0, len(steps))
	for _, s := range steps {
		tests = append(tests, &testplan.Func{
			TestName: s.Name,
			Desc:     s.Description,
			Fn: func(ctx context.Context, out io.Writer) error {
				return s.Run(ctx, out, opts)
			},
		})
	}

	return tests
}

// Registry returns all steps and playlists, ready to be selected from and run.
func Registry(opts *Options) *testplan.Registry {
	registry := testplan.NewRegistry(Tests(opts)...)
	for _, p := range Playlists() {
		registry.AddPlaylist(p)
	}

	return registry
}
