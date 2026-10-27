// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import "testing"

func TestInstallVersionAtLeast(t *testing.T) {
	t.Run("rejects lower versions", func(t *testing.T) {
		if InstallVersionAtLeast("codesphere-v1.105.0", "v1.106.0") {
			t.Fatal("expected codesphere-v1.105.0 to be below v1.106.0")
		}
	})

	t.Run("accepts exact version matches", func(t *testing.T) {
		if !InstallVersionAtLeast("codesphere-v1.106.0", "v1.106.0") {
			t.Fatal("expected codesphere-v1.106.0 to satisfy v1.106.0")
		}
	})

	t.Run("accepts release branch names", func(t *testing.T) {
		if !InstallVersionAtLeast("codesphere-1-114-0", "v1.106.0") {
			t.Fatal("expected codesphere-1-114-0 to satisfy v1.106.0")
		}
	})

	t.Run("rejects lower release branch names", func(t *testing.T) {
		if InstallVersionAtLeast("codesphere-1-105-2", "v1.106.0") {
			t.Fatal("expected codesphere-1-105-2 to be below v1.106.0")
		}
	})

	t.Run("accepts versions without codesphere prefix", func(t *testing.T) {
		if !InstallVersionAtLeast("1.114.0", "v1.106.0") {
			t.Fatal("expected 1.114.0 to satisfy v1.106.0")
		}
	})

	t.Run("rejects invalid versions", func(t *testing.T) {
		if InstallVersionAtLeast("not-a-version", "v1.106.0") {
			t.Fatal("expected invalid versions to be rejected")
		}
	})
}
