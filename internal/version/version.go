// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package version

// Variables are injected by goreleaser on release
var (
	version string = "0.0.0"
	commit  string = "none"
	date    string = "unknown"
	os      string = "unknown"
	arch    string = "unknown"
	binName string = "oms-cli"
)

type Version interface {
	Version() string
	Commit() string
	BuildDate() string
	Os() string
	Arch() string
}

type Build struct{}

func (b *Build) Version() string {
	return version
}

func (b *Build) Commit() string {
	return commit
}

func (b *Build) BuildDate() string {
	return date
}

func (b *Build) Os() string {
	return os
}

func (b *Build) Arch() string {
	return arch
}
