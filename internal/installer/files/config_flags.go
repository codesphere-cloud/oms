// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package files defines installer configuration and its file formats.
package files

import "slices"

// IsEnabled reports whether a flag is enabled as an internal, deprecated experiment, preview, or
// feature flag.
func (c CodesphereConfig) IsEnabled(flag string) bool {
	return slices.Contains(c.Internal, flag) || slices.Contains(c.Experiments, flag) || c.Preview[flag] || c.Features[flag] //nolint:staticcheck
}
