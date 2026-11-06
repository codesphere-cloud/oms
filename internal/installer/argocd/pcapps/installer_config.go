// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"slices"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// installerConfig is the merged config.yaml the PC installer reads, decoded generically. It asks
// the installer's questions of the installer's input, which files.RootConfig cannot answer: its
// `Enabled bool` cannot tell an unset load balancer (installed) from a disabled one, and it has no
// `cluster.rgw`.
type installerConfig map[string]any

// at returns the value at path when it is a T. A missing key, a null, a value of another type and
// a scalar on the way all mean "not set". Decoded YAML holds maps, lists, scalars and null, so the
// walk stays untyped and T is what the caller wants out.
func at[T any](c installerConfig, path ...string) (T, bool) {
	var node any = map[string]any(c)

	for _, key := range path {
		mapping, ok := node.(map[string]any)
		if !ok {
			var unset T

			return unset, false
		}

		node = mapping[key]
	}

	value, ok := node.(T)

	return value, ok
}

// has reports whether the config sets a mapping at path.
func (c installerConfig) has(path ...string) bool {
	_, ok := at[map[string]any](c, path...)

	return ok
}

// componentEnabled mirrors the installer's isComponentEnabled: an unset component is enabled.
func (c installerConfig) componentEnabled(path ...string) bool {
	component, ok := at[map[string]any](c, path...)

	return !ok || component["enabled"] != false
}

// internalFlagEnabled mirrors the installer's isEnabled for an internal flag: initInstallerFlags
// puts codesphere.internal and codesphere.experiments into one bucket. It also reads the preview
// and features buckets, which this does not.
func (c installerConfig) internalFlagEnabled(flag string) bool {
	return c.listContains(flag, "codesphere", "internal") || c.listContains(flag, "codesphere", "experiments")
}

func (c installerConfig) listContains(item string, path ...string) bool {
	list, _ := at[[]any](c, path...)

	return slices.Contains(list, any(item))
}

// override returns the `override` values the installer merges over the values it writes.
func (c installerConfig) override(path ...string) files.ChartValues {
	component, _ := at[map[string]any](c, path...)
	override, _ := component["override"].(map[string]any)

	return override
}
