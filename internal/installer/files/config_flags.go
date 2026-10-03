// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package files

import "slices"

// IsEnabled reports whether a flag is enabled as an internal, preview, or feature flag.
func (c CodesphereConfig) IsEnabled(flag string) bool {
	return slices.Contains(c.Internal, flag) || c.Preview[flag] || c.Features[flag]
}
