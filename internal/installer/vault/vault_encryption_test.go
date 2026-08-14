// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package vault_test

import (
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/installer/vault/sops"
)

func sopsAndAgeAvailable() bool {
	if _, err := exec.LookPath("sops"); err != nil {
		return false
	}
	if _, err := exec.LookPath("age-keygen"); err != nil {
		return false
	}
	return true
}

var _ = Describe("VaultEncryption", func() {
	Describe("ResolveAgeKey", func() {
		var (
			tmpDir         string
			origAgeKey     string
			origAgeKeyFile string
			hasOrigAgeKey  bool
			hasOrigKeyFile bool
		)

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "age-test-*")
			Expect(err).ToNot(HaveOccurred())

			// Save and clear env vars to isolate tests.
			origAgeKey, hasOrigAgeKey = os.LookupEnv("SOPS_AGE_KEY")
			origAgeKeyFile, hasOrigKeyFile = os.LookupEnv("SOPS_AGE_KEY_FILE")
			Expect(os.Unsetenv("SOPS_AGE_KEY")).To(Succeed())
			Expect(os.Unsetenv("SOPS_AGE_KEY_FILE")).To(Succeed())
		})

		AfterEach(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
			// Restore env vars.
			if hasOrigAgeKey {
				Expect(os.Setenv("SOPS_AGE_KEY", origAgeKey)).To(Succeed())
			} else {
				Expect(os.Unsetenv("SOPS_AGE_KEY")).To(Succeed())
			}
			if hasOrigKeyFile {
				Expect(os.Setenv("SOPS_AGE_KEY_FILE", origAgeKeyFile)).To(Succeed())
			} else {
				Expect(os.Unsetenv("SOPS_AGE_KEY_FILE")).To(Succeed())
			}
		})

		Context("with an explicit key file argument", func() {
			It("reads the recipient from the explicit file and returns it as the key path", func() {
				if !sopsAndAgeAvailable() {
					Skip("age-keygen not available")
				}
				keyFile := filepath.Join(tmpDir, "explicit.txt")
				out, err := exec.Command("age-keygen", "-o", keyFile).CombinedOutput()
				Expect(err).ToNot(HaveOccurred(), string(out))

				// Set conflicting env vars to prove the explicit file takes priority.
				Expect(os.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(tmpDir, "ignored.txt"))).To(Succeed())

				recipient, keyPath, err := sops.ResolveAgeKey(keyFile, tmpDir)
				Expect(err).ToNot(HaveOccurred())
				Expect(recipient).To(HavePrefix("age1"))
				Expect(keyPath).To(Equal(keyFile))
			})

			It("returns an error if the explicit file does not exist", func() {
				_, _, err := sops.ResolveAgeKey(filepath.Join(tmpDir, "missing.txt"), tmpDir)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to read age key"))
			})
		})

		Context("with SOPS_AGE_KEY env var containing only a private key (no comment)", func() {
			It("should fall back to age-keygen -y to derive the recipient", func() {
				if !sopsAndAgeAvailable() {
					Skip("age-keygen not available")
				}
				// Generate a real key to get valid content.
				keyFile := filepath.Join(tmpDir, "real_key.txt")
				out, err := exec.Command("age-keygen", "-o", keyFile).CombinedOutput()
				Expect(err).ToNot(HaveOccurred(), string(out))

				data, err := os.ReadFile(keyFile)
				Expect(err).ToNot(HaveOccurred())

				// Extract just the private key line (no comments).
				var privKeyLine string
				for _, line := range splitLines(string(data)) {
					if len(line) > 0 && line[0] != '#' {
						privKeyLine = line
						break
					}
				}
				Expect(privKeyLine).ToNot(BeEmpty())

				Expect(os.Setenv("SOPS_AGE_KEY", privKeyLine)).To(Succeed())

				recipient, keyPath, err := sops.ResolveAgeKey("", tmpDir)
				Expect(err).ToNot(HaveOccurred())
				Expect(recipient).To(HavePrefix("age1"))
				Expect(keyPath).To(BeEmpty())
			})
		})

		Context("with SOPS_AGE_KEY_FILE env var pointing to a key file", func() {
			It("should read the recipient from the referenced file", func() {
				if !sopsAndAgeAvailable() {
					Skip("age-keygen not available")
				}
				keyFile := filepath.Join(tmpDir, "keys.txt")
				out, err := exec.Command("age-keygen", "-o", keyFile).CombinedOutput()
				Expect(err).ToNot(HaveOccurred(), string(out))

				Expect(os.Setenv("SOPS_AGE_KEY_FILE", keyFile)).To(Succeed())

				recipient, keyPath, err := sops.ResolveAgeKey("", tmpDir)
				Expect(err).ToNot(HaveOccurred())
				Expect(recipient).To(HavePrefix("age1"))
				Expect(keyPath).To(Equal(keyFile))
			})

			It("should return error if the file does not exist", func() {
				Expect(os.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(tmpDir, "nonexistent.txt"))).To(Succeed())

				_, _, err := sops.ResolveAgeKey("", tmpDir)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to read age key"))
			})
		})

		Context("with no env vars set", func() {
			It("should generate a new key when no default location exists", func() {
				if !sopsAndAgeAvailable() {
					Skip("age-keygen not available")
				}

				recipient, keyPath, err := sops.ResolveAgeKey("", tmpDir)
				Expect(err).ToNot(HaveOccurred())
				Expect(recipient).To(HavePrefix("age1"))
				Expect(keyPath).To(Equal(filepath.Join(tmpDir, "age_key.txt")))

				// Verify the key file was created.
				Expect(keyPath).To(BeAnExistingFile())
			})
		})
	})

	Describe("file-backed vault loading", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "load-vault-test-*")
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		It("parses a plain vault file without data: wrapper", func() {
			vaultPath := filepath.Join(tmpDir, "plain.vault.yaml")
			plainYAML := "secrets:\n    - name: test-secret\n      fields:\n        password: hunter2\n"
			Expect(os.WriteFile(vaultPath, []byte(plainYAML), 0644)).To(Succeed())

			backend, err := vault.New(vault.TypePlain, vault.Options{Path: vaultPath})
			Expect(err).ToNot(HaveOccurred())
			loaded, err := backend.Load()
			Expect(err).ToNot(HaveOccurred())
			Expect(loaded.Secrets).To(HaveLen(1))
			Expect(loaded.Secrets[0].Name).To(Equal("test-secret"))
			Expect(loaded.Secrets[0].Fields.Password).To(Equal("hunter2"))
		})

		It("unwraps a plain file with data: | wrapper (SOPS whole-file format edge case)", func() {
			vaultPath := filepath.Join(tmpDir, "wrapped.vault.yaml")
			wrappedYAML := "data: |\n    secrets:\n        - name: test-secret\n          fields:\n            password: hunter2\n"
			Expect(os.WriteFile(vaultPath, []byte(wrappedYAML), 0644)).To(Succeed())

			backend, err := vault.New(vault.TypePlain, vault.Options{Path: vaultPath})
			Expect(err).ToNot(HaveOccurred())
			loaded, err := backend.Load()
			Expect(err).ToNot(HaveOccurred())
			Expect(loaded.Secrets).To(HaveLen(1))
			Expect(loaded.Secrets[0].Name).To(Equal("test-secret"))
			Expect(loaded.Secrets[0].Fields.Password).To(Equal("hunter2"))
		})

		It("loads and decrypts a SOPS-encrypted vault end-to-end", func() {
			if !sopsAndAgeAvailable() {
				Skip("sops and age-keygen not available")
			}

			// Generate an age keypair.
			ageKeyPath := filepath.Join(tmpDir, "age_key.txt")
			out, err := exec.Command("age-keygen", "-o", ageKeyPath).CombinedOutput()
			Expect(err).ToNot(HaveOccurred(), string(out))

			// Extract the public key (recipient).
			recipient, _, err := sops.ResolveAgeKey(ageKeyPath, tmpDir)
			Expect(err).ToNot(HaveOccurred())

			// Write a plain vault file.
			plainPath := filepath.Join(tmpDir, "plain.vault.yaml")
			plainYAML := "secrets:\n    - name: sops-secret\n      fields:\n        password: s3cr3t\n"
			Expect(os.WriteFile(plainPath, []byte(plainYAML), 0644)).To(Succeed())

			// Encrypt with SOPS using --input-type yaml (whole-file mode,
			// which wraps content under data: |).
			vaultPath := filepath.Join(tmpDir, "encrypted.vault.yaml")
			encryptCmd := exec.Command("sops", "--encrypt", "--input-type", "yaml", "--age", recipient, "--output", vaultPath, plainPath)
			encOut, err := encryptCmd.CombinedOutput()
			Expect(err).ToNot(HaveOccurred(), string(encOut))

			backend, err := vault.New(vault.TypeSOPS, vault.Options{Path: vaultPath, AgeKey: ageKeyPath})
			Expect(err).ToNot(HaveOccurred())
			loaded, err := backend.Load()
			Expect(err).ToNot(HaveOccurred())
			Expect(loaded.Secrets).To(HaveLen(1))
			Expect(loaded.Secrets[0].Name).To(Equal("sops-secret"))
			Expect(loaded.Secrets[0].Fields.Password).To(Equal("s3cr3t"))
		})

		It("returns an error for a non-existent file", func() {
			backend, err := vault.New(vault.TypePlain, vault.Options{Path: filepath.Join(tmpDir, "missing.yaml")})
			Expect(err).ToNot(HaveOccurred())
			_, err = backend.Load()
			Expect(err).To(HaveOccurred())
		})
	})
})

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
