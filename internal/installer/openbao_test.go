// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
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
		It("performs fresh install when operator does not exist", func() {
			// FindRelease returns nil (no existing release in target namespace)
			helmMock.EXPECT().FindRelease("vault", "vault-operator").Return(nil, nil)

			// No ClusterRole exists (fake clientset has nothing), so InstallChart is called
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.ReleaseName == "vault-operator" &&
					cfg.ChartName == "oci://ghcr.io/bank-vaults/helm-charts/vault-operator" &&
					cfg.Version == "1.22.5" &&
					cfg.Namespace == "vault" &&
					cfg.CreateNamespace == true
			})).Return(nil)

			inst := &installer.OpenBaoInstaller{
				Helm:      helmMock,
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "vault"},
			}
			inst.SetCtx(ctx)

			err := inst.DeployBankVaultsOperator()
			Expect(err).ToNot(HaveOccurred())
		})

		It("upgrades when release already exists in target namespace", func() {
			// FindRelease returns an existing release
			helmMock.EXPECT().FindRelease("vault", "vault-operator").Return(&installer.ReleaseInfo{
				Name:             "vault-operator",
				InstalledVersion: "1.22.0",
			}, nil)

			helmMock.EXPECT().UpgradeChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.ReleaseName == "vault-operator" &&
					cfg.Namespace == "vault"
			}), installer.UpgradeChartOptions{}).Return(nil)

			inst := &installer.OpenBaoInstaller{
				Helm:      helmMock,
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "vault"},
			}
			inst.SetCtx(ctx)

			err := inst.DeployBankVaultsOperator()
			Expect(err).ToNot(HaveOccurred())
		})

		It("skips deployment when operator exists in another namespace", func() {
			// FindRelease returns nil (not in target namespace)
			helmMock.EXPECT().FindRelease("second", "vault-operator").Return(nil, nil)

			// Pre-create the ClusterRole to simulate operator installed elsewhere
			cr := &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{Name: "vault-operator"},
			}
			_, err := clientset.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			inst := &installer.OpenBaoInstaller{
				Helm:      helmMock,
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "second"},
			}
			inst.SetCtx(ctx)

			// Should not call InstallChart or UpgradeChart
			err = inst.DeployBankVaultsOperator()
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error when Helm InstallChart fails", func() {
			helmMock.EXPECT().FindRelease("vault", "vault-operator").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.Anything).
				Return(fmt.Errorf("chart not found"))

			inst := &installer.OpenBaoInstaller{
				Helm:      helmMock,
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "vault"},
			}
			inst.SetCtx(ctx)

			err := inst.DeployBankVaultsOperator()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("chart not found"))
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
				Expect(err.Error()).To(ContainSubstring("DRBackupPath must be set"))
			})
		})
	})

	Describe("WaitForInitialization", func() {
		It("returns the secret once it contains unseal keys", func() {
			// Pre-create the namespace
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Pre-create the secret with data (no root_token — storeRootToken is false)
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openbao-unseal-keys",
					Namespace: "vault",
				},
				Data: map[string][]byte{
					"vault-unseal-0": []byte("key-data"),
				},
			}
			_, err = clientset.CoreV1().Secrets("vault").Create(ctx, secret, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					Namespace: "vault",
					Timeout:   5 * time.Second,
				},
			}
			inst.SetCtx(ctx)

			err = inst.WaitForInitialization()
			Expect(err).ToNot(HaveOccurred())
			result := inst.GetUnsealSecret()
			Expect(result.Data).To(HaveKey("vault-unseal-0"))
		})

		It("times out when secret does not appear", func() {
			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					Namespace: "vault",
					Timeout:   1 * time.Second,
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

	Describe("Vault CR template rendering", func() {
		// templateData mirrors the unexported vaultCRTemplateData struct
		// so the test can render the template independently.
		type templateData struct {
			Namespace         string
			OpenBaoImage      string
			BankVaultsImage   string
			SecretsEngineName string
			BaoUsername       string
			BaoPassword       string
			Replicas          int
			StorageSize       string
			RetryJoinAddrs    []string
		}

		renderTemplate := func(data templateData) []map[string]interface{} {
			raw, err := os.ReadFile("manifests/openbao/vault-cr.yaml")
			Expect(err).ToNot(HaveOccurred())

			tmpl, err := template.New("vault-cr").Parse(string(raw))
			Expect(err).ToNot(HaveOccurred())

			var buf bytes.Buffer
			Expect(tmpl.Execute(&buf, data)).To(Succeed())

			// Decode multi-doc YAML into a slice of generic maps
			decoder := yaml.NewYAMLOrJSONDecoder(&buf, 4096)
			var docs []map[string]interface{}
			for {
				var doc map[string]interface{}
				if err := decoder.Decode(&doc); err != nil {
					break
				}
				if doc != nil {
					docs = append(docs, doc)
				}
			}
			return docs
		}

		findDoc := func(docs []map[string]interface{}, kind string) map[string]interface{} {
			for _, doc := range docs {
				if doc["kind"] == kind {
					return doc
				}
			}
			return nil
		}

		It("renders valid YAML with raft storage and PVC for replicas=1", func() {
			data := templateData{
				Namespace:         "vault",
				OpenBaoImage:      "quay.io/openbao/openbao:2.1.0",
				BankVaultsImage:   "ghcr.io/bank-vaults/bank-vaults:v1.31.3",
				SecretsEngineName: "cs-secrets-engine",
				BaoUsername:       "admin",
				BaoPassword:       "test-password",
				Replicas:          1,
				StorageSize:       "10Gi",
				RetryJoinAddrs:    []string{"http://openbao-0.vault.svc.cluster.local:8200"},
			}

			docs := renderTemplate(data)
			// Expect 4 documents: ServiceAccount, Role, RoleBinding, Vault
			Expect(docs).To(HaveLen(4))

			// Verify Vault CR
			vault := findDoc(docs, "Vault")
			Expect(vault).ToNot(BeNil())

			spec := vault["spec"].(map[string]interface{})
			Expect(spec["size"]).To(BeNumerically("==", 1))
			Expect(spec["image"]).To(Equal("quay.io/openbao/openbao:2.1.0"))

			// Should have volumeClaimTemplates (raft always needs persistent storage)
			Expect(spec).To(HaveKey("volumeClaimTemplates"))
			vcts := spec["volumeClaimTemplates"].([]interface{})
			Expect(vcts).To(HaveLen(1))
			vct := vcts[0].(map[string]interface{})
			vctSpec := vct["spec"].(map[string]interface{})
			resources := vctSpec["resources"].(map[string]interface{})
			requests := resources["requests"].(map[string]interface{})
			Expect(requests["storage"]).To(Equal("10Gi"))

			// Should have volumeMounts
			Expect(spec).To(HaveKey("volumeMounts"))

			// Config should use raft storage (always, even for single node)
			config := spec["config"].(map[string]interface{})
			storage := config["storage"].(map[string]interface{})
			Expect(storage).To(HaveKey("raft"))
			Expect(storage).ToNot(HaveKey("file"))
			raft := storage["raft"].(map[string]interface{})
			retryJoin := raft["retry_join"].([]interface{})
			Expect(retryJoin).To(HaveLen(1))

			// Unseal config should have storeRootToken: false
			unsealConfig := spec["unsealConfig"].(map[string]interface{})
			options := unsealConfig["options"].(map[string]interface{})
			Expect(options["storeRootToken"]).To(BeFalse())
			Expect(options["preFlightChecks"]).To(BeTrue())

			// Verify externalConfig has the secrets engine
			externalConfig := spec["externalConfig"].(map[string]interface{})
			secrets := externalConfig["secrets"].([]interface{})
			Expect(secrets).To(HaveLen(1))
			secretEntry := secrets[0].(map[string]interface{})
			Expect(secretEntry["path"]).To(Equal("cs-secrets-engine"))

			// Verify auth config
			auth := externalConfig["auth"].([]interface{})
			Expect(auth).To(HaveLen(1))
			authEntry := auth[0].(map[string]interface{})
			Expect(authEntry["type"]).To(Equal("userpass"))
			users := authEntry["users"].([]interface{})
			user := users[0].(map[string]interface{})
			Expect(user["username"]).To(Equal("admin"))
			Expect(user["password"]).To(Equal("test-password"))

			// Verify vaultContainerSpec has env vars (always present now)
			containerSpec := spec["vaultContainerSpec"].(map[string]interface{})
			Expect(containerSpec).To(HaveKey("env"))
		})

		It("renders valid YAML with raft storage, PVCs, and retry_join for replicas=3", func() {
			retryJoinAddrs := []string{
				"http://openbao-0.vault.svc.cluster.local:8200",
				"http://openbao-1.vault.svc.cluster.local:8200",
				"http://openbao-2.vault.svc.cluster.local:8200",
			}

			data := templateData{
				Namespace:         "vault",
				OpenBaoImage:      "quay.io/openbao/openbao:2.1.0",
				BankVaultsImage:   "ghcr.io/bank-vaults/bank-vaults:v1.31.3",
				SecretsEngineName: "cs-secrets-engine",
				BaoUsername:       "admin",
				BaoPassword:       "test-password",
				Replicas:          3,
				StorageSize:       "20Gi",
				RetryJoinAddrs:    retryJoinAddrs,
			}

			docs := renderTemplate(data)
			Expect(docs).To(HaveLen(4))

			vault := findDoc(docs, "Vault")
			Expect(vault).ToNot(BeNil())

			spec := vault["spec"].(map[string]interface{})
			Expect(spec["size"]).To(BeNumerically("==", 3))

			// Should have volumeClaimTemplates for HA
			Expect(spec).To(HaveKey("volumeClaimTemplates"))
			vcts := spec["volumeClaimTemplates"].([]interface{})
			Expect(vcts).To(HaveLen(1))
			vct := vcts[0].(map[string]interface{})
			vctSpec := vct["spec"].(map[string]interface{})
			resources := vctSpec["resources"].(map[string]interface{})
			requests := resources["requests"].(map[string]interface{})
			Expect(requests["storage"]).To(Equal("20Gi"))

			// Should have volumeMounts
			Expect(spec).To(HaveKey("volumeMounts"))

			// Config should use raft storage with retry_join
			config := spec["config"].(map[string]interface{})
			storage := config["storage"].(map[string]interface{})
			Expect(storage).To(HaveKey("raft"))
			Expect(storage).ToNot(HaveKey("file"))
			raft := storage["raft"].(map[string]interface{})
			retryJoin := raft["retry_join"].([]interface{})
			Expect(retryJoin).To(HaveLen(3))

			// Verify vaultContainerSpec has HA env vars
			containerSpec := spec["vaultContainerSpec"].(map[string]interface{})
			Expect(containerSpec).To(HaveKey("env"))
			envVars := containerSpec["env"].([]interface{})
			envNames := make([]string, 0, len(envVars))
			for _, e := range envVars {
				envNames = append(envNames, e.(map[string]interface{})["name"].(string))
			}
			Expect(envNames).To(ContainElements("POD_NAME", "BAO_CLUSTER_ADDR", "BAO_API_ADDR"))
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
