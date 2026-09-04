package util

import "testing"

func TestInstallVersionAtLeast(t *testing.T) {
	t.Run("rejects lower versions", func(t *testing.T) {
		if InstallVersionAtLeast("v1.105.0", "v1.106.0") {
			t.Fatal("expected v1.105.0 to be below v1.106.0")
		}
	})

	t.Run("accepts exact version matches", func(t *testing.T) {
		if !InstallVersionAtLeast("v1.106.0", "v1.106.0") {
			t.Fatal("expected v1.106.0 to satisfy v1.106.0")
		}
	})

	t.Run("supports codesphere-prefixed versions", func(t *testing.T) {
		if !InstallVersionAtLeast("codesphere-v1.106.0", "v1.106.0") {
			t.Fatal("expected codesphere-v1.106.0 to satisfy v1.106.0")
		}
	})

	t.Run("rejects invalid versions", func(t *testing.T) {
		if InstallVersionAtLeast("not-a-version", "v1.106.0") {
			t.Fatal("expected invalid versions to be rejected")
		}
	})
}
