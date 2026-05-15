// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var _ = Describe("OpenBaoInstaller", func() {
	var (
		helmMock  *installer.MockHelmClient
		clientset *fake.Clientset
		ctx       context.Context
		tmpDir    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		helmMock = installer.NewMockHelmClient(GinkgoT())
		clientset = fake.NewClientset()

		var err error
		tmpDir, err = os.MkdirTemp("", "openbao-test-*")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	Describe("Install — deploy Bank-Vaults Operator", func() {
		It("calls UpgradeChart with InstallIfNotExist for the operator", func() {
			helmMock.EXPECT().UpgradeChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.ReleaseName == "vault-operator" &&
					cfg.ChartName == "oci://ghcr.io/bank-vaults/helm-charts/vault-operator" &&
					cfg.Version == "1.22.5" &&
					cfg.Namespace == "vault" &&
					cfg.CreateNamespace == true
			}), installer.UpgradeChartOptions{InstallIfNotExist: true}).Return(nil)

			inst := &installer.OpenBaoInstaller{
				Helm:      helmMock,
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{},
			}
			inst.SetCtx(ctx)

			err := inst.DeployBankVaultsOperator()
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error when Helm fails", func() {
			helmMock.EXPECT().UpgradeChart(mock.Anything, mock.Anything, mock.Anything).
				Return(fmt.Errorf("chart not found"))

			inst := &installer.OpenBaoInstaller{
				Helm:      helmMock,
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{},
			}
			inst.SetCtx(ctx)

			err := inst.DeployBankVaultsOperator()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("chart not found"))
		})
	})

	Describe("SecurityCleanup", func() {
		It("removes root_token from the unseal secret", func() {
			// Pre-create the secret with root_token and unseal keys
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openbao-unseal-keys",
					Namespace: "vault",
				},
				Data: map[string][]byte{
					"vault-unseal-0": []byte("unseal-key-data"),
					"root_token":     []byte("root-token-value"),
				},
			}
			_, err := clientset.CoreV1().Secrets("vault").Create(ctx, secret, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{},
			}
			inst.SetCtx(ctx)

			err = inst.SecurityCleanup()
			Expect(err).ToNot(HaveOccurred())

			// Verify root_token was removed
			updated, err := clientset.CoreV1().Secrets("vault").Get(ctx, "openbao-unseal-keys", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Data).ToNot(HaveKey("root_token"))
			Expect(updated.Data).To(HaveKey("vault-unseal-0"))
		})

		It("is a no-op when root_token is already absent", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openbao-unseal-keys",
					Namespace: "vault",
				},
				Data: map[string][]byte{
					"vault-unseal-0": []byte("unseal-key-data"),
				},
			}
			_, err := clientset.CoreV1().Secrets("vault").Create(ctx, secret, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{},
			}
			inst.SetCtx(ctx)

			err = inst.SecurityCleanup()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("PreFlightDRCheck", func() {
		Context("when no DR backup file exists", func() {
			It("proceeds without error", func() {
				inst := &installer.OpenBaoInstaller{
					Clientset: clientset,
					Logger:    bootstrap.NewStepLogger(true),
					Config: installer.OpenBaoInstallerConfig{
						DRBackupPath: filepath.Join(tmpDir, "nonexistent.enc.json"),
					},
				}
				inst.SetCtx(ctx)

				err := inst.PreFlightDRCheck()
				Expect(err).ToNot(HaveOccurred())
				Expect(inst.GetDRBackupExists()).To(BeFalse())
			})
		})

		Context("when DR backup path is empty", func() {
			It("returns an error", func() {
				inst := &installer.OpenBaoInstaller{
					Clientset: clientset,
					Logger:    bootstrap.NewStepLogger(true),
					Config: installer.OpenBaoInstallerConfig{
						DRBackupPath: "",
					},
				}
				inst.SetCtx(ctx)

				err := inst.PreFlightDRCheck()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("--dr-backup-path is required"))
			})
		})
	})

	Describe("WaitForInitialization", func() {
		It("returns the secret once it contains unseal keys", func() {
			// Pre-create the namespace
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Pre-create the secret with data
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openbao-unseal-keys",
					Namespace: "vault",
				},
				Data: map[string][]byte{
					"vault-unseal-0": []byte("key-data"),
					"root_token":     []byte("root-token"),
				},
			}
			_, err = clientset.CoreV1().Secrets("vault").Create(ctx, secret, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					Timeout: 5 * time.Second,
				},
			}
			inst.SetCtx(ctx)

			err = inst.WaitForInitialization()
			Expect(err).ToNot(HaveOccurred())
			result := inst.GetUnsealSecret()
			Expect(result.Data).To(HaveKey("vault-unseal-0"))
			Expect(result.Data).To(HaveKey("root_token"))
		})

		It("times out when secret does not appear", func() {
			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					Timeout: 1 * time.Second,
				},
			}
			inst.SetCtx(ctx)

			err := inst.WaitForInitialization()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timed out"))
		})
	})

	Describe("ExtractAndEncrypt", func() {
		It("creates an encrypted DR backup file with password and unseal keys", func() {
			if !sopsAndAgeAvailable() {
				Skip("sops/age not available")
			}

			// Generate a real age key for testing and extract the public key
			// directly from age-keygen output (format: "Public key: age1...").
			// This avoids calling ResolveAgeKey which probes env vars and
			// default config paths, making the test sensitive to the host.
			keyFile := filepath.Join(tmpDir, "age_key.txt")
			out, err := exec.Command("age-keygen", "-o", keyFile).CombinedOutput()
			Expect(err).ToNot(HaveOccurred(), string(out))

			recipient := extractAgeRecipient(string(out))
			Expect(recipient).To(HavePrefix("age1"), "could not extract public key from age-keygen output")

			backupPath := filepath.Join(tmpDir, "backup.enc.json")

			secret := &corev1.Secret{
				Data: map[string][]byte{
					"vault-unseal-0": []byte("test-unseal-key-0"),
					"root_token":     []byte("test-root-token"),
				},
			}

			inst := &installer.OpenBaoInstaller{
				Logger: bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					DRBackupPath: backupPath,
					AgeRecipient: recipient,
					AgeKeyPath:   keyFile,
					Username:     "admin",
				},
			}
			inst.SetUnsealSecret(secret)
			inst.SetPassword("generated-password-123")

			err = inst.ExtractAndEncrypt()
			Expect(err).ToNot(HaveOccurred())

			// Verify the encrypted file exists
			Expect(backupPath).To(BeAnExistingFile())

			// Decrypt it back and verify contents
			cmd := exec.Command("sops", "--decrypt", backupPath)
			cmd.Env = append(os.Environ(), "SOPS_AGE_KEY_FILE="+keyFile)
			decrypted, err := cmd.Output()
			Expect(err).ToNot(HaveOccurred())

			var backup map[string]interface{}
			Expect(json.Unmarshal(decrypted, &backup)).To(Succeed())
			Expect(backup).To(HaveKey("password"))
			Expect(backup["password"]).To(Equal("generated-password-123"))
			Expect(backup).To(HaveKey("username"))
			Expect(backup["username"]).To(Equal("admin"))
			Expect(backup).To(HaveKey("unseal_keys"))
			unsealKeys, ok := backup["unseal_keys"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(unsealKeys).To(HaveKey("vault-unseal-0"))
			Expect(unsealKeys["vault-unseal-0"]).To(Equal("test-unseal-key-0"))
		})
	})

	Describe("GenerateSecurePassword", func() {
		It("generates a non-empty password of expected length", func() {
			password, err := installer.GenerateSecurePassword(32)
			Expect(err).ToNot(HaveOccurred())
			Expect(password).ToNot(BeEmpty())
			// 32 bytes base64url-encoded without padding = 43 chars
			Expect(len(password)).To(Equal(43))
		})

		It("generates unique passwords on each call", func() {
			p1, err := installer.GenerateSecurePassword(32)
			Expect(err).ToNot(HaveOccurred())
			p2, err := installer.GenerateSecurePassword(32)
			Expect(err).ToNot(HaveOccurred())
			Expect(p1).ToNot(Equal(p2))
		})
	})
})

// extractAgeRecipient extracts the public key from age-keygen's output.
// age-keygen -o <file> prints "Public key: age1..." to stderr.
func extractAgeRecipient(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "Public key: ") {
			return strings.TrimPrefix(line, "Public key: ")
		}
	}
	return ""
}
