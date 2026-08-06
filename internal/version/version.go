// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package version provides functions about the OMS version
package version

// Variables are injected by goreleaser on release
var (
	version = "0.0.0"
	commit  = "none"
	date    = "unknown"
	os      = "unknown"
	arch    = "unknown"
	binName = "oms"
)

//mockery:generate: true

// Version provides access to the build information of the binary.
type Version interface {
	Version() string
	Commit() string
	BuildDate() string
	Os() string
	Arch() string
}

// Build implements Version using the values injected at release time.
type Build struct{}

// Version returns the released version of the binary.
func (b *Build) Version() string {
	return version
}

// Commit returns the git commit the binary was built from.
func (b *Build) Commit() string {
	return commit
}

// BuildDate returns the date the binary was built.
func (b *Build) BuildDate() string {
	return date
}

// Os returns the operating system the binary was built for.
func (b *Build) Os() string {
	return os
}

// Arch returns the architecture the binary was built for.
func (b *Build) Arch() string {
	return arch
}

// BinName returns the name of the binary.
func (b *Build) BinName() string {
	return binName
}
