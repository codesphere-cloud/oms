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
	"github.com/codesphere-cloud/oms/internal/installer/vault/sops"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

var _ = Describe("OpenBaoBootstrapper", func() {
	var (
		clientset *fake.Clientset
		ctx       context.Context
		tmpDir    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		clientset = fake.NewClientset()

		var err error
		tmpDir, err = os.MkdirTemp("", "openbao-test-*")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	Describe("Bootstrap (export)", func() {
		It("fails when a DR backup already exists at the path", func() {
			backupPath := filepath.Join(tmpDir, "backup.enc.json")
			Expect(os.WriteFile(backupPath, []byte("{}"), 0o600)).To(Succeed())

			b := &installer.OpenBaoBootstrapper{
				Clientset: clientset,
				DynClient: newVaultDynClient(),
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoBootstrapConfig{DRBackupPath: backupPath},
			}

			err := b.Bootstrap(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already exists"))
		})

		It("fails fast when the Vault CR does not exist", func() {
			b := &installer.OpenBaoBootstrapper{
				Clientset: clientset,
				DynClient: newVaultDynClient(),
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoBootstrapConfig{DRBackupPath: filepath.Join(tmpDir, "backup.enc.json")},
			}

			err := b.Bootstrap(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("sync the OpenBao ArgoCD app first"))
		})
	})

	Describe("Bootstrap (restore)", func() {
		It("fails when the DR backup does not exist", func() {
			b := &installer.OpenBaoBootstrapper{
				Clientset: clientset,
				DynClient: newVaultDynClient(),
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoBootstrapConfig{
					DRBackupPath: filepath.Join(tmpDir, "missing.enc.json"),
					Restore:      true,
				},
			}

			err := b.Bootstrap(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reading DR backup file"))
		})

		It("seeds the unseal secret before the Vault CR exists", func() {
			if !sopsAndAgeAvailable() {
				Skip("sops/age not available")
			}
			backupPath, keyFile := writeEncryptedBackup(tmpDir, `{"unseal_keys":{"vault-unseal-0":"backup-key"}}`)

			b := &installer.OpenBaoBootstrapper{
				Clientset: clientset,
				DynClient: newVaultDynClient(),
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoBootstrapConfig{
					DRBackupPath: backupPath,
					AgeKeyPath:   keyFile,
					Restore:      true,
					Timeout:      1 * time.Second,
				},
			}

			// No Vault CR → the restore times out waiting for it, but only after
			// the namespace and secret were seeded.
			err := b.Bootstrap(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("waiting for Vault CR"))

			_, err = clientset.CoreV1().Namespaces().Get(ctx, "vault", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			secret, err := clientset.CoreV1().Secrets("vault").Get(ctx, "openbao-unseal-keys", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Data).To(HaveKeyWithValue("vault-unseal-0", []byte("backup-key")))
		})
	})

	Describe("WaitForVaultCR", func() {
		newBootstrapper := func(objs ...runtime.Object) *installer.OpenBaoBootstrapper {
			b := &installer.OpenBaoBootstrapper{
				DynClient: newVaultDynClient(objs...),
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoBootstrapConfig{Namespace: "vault", Timeout: 1 * time.Second},
			}
			b.SetCtx(ctx)
			return b
		}

		It("reads the replica count from spec.size", func() {
			b := newBootstrapper(newVaultCR(map[string]interface{}{"size": int64(3)}))
			Expect(b.WaitForVaultCR()).To(Succeed())
			Expect(b.GetReplicas()).To(Equal(3))
		})

		It("defaults to a single replica when spec.size is unset", func() {
			b := newBootstrapper(newVaultCR(map[string]interface{}{}))
			Expect(b.WaitForVaultCR()).To(Succeed())
			Expect(b.GetReplicas()).To(Equal(1))
		})

		It("times out when the Vault CR never appears", func() {
			b := newBootstrapper()
			err := b.WaitForVaultCR()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timed out"))
		})
	})

	Describe("PreFlightDRCheck", func() {
		Context("when no DR backup file exists", func() {
			It("returns an error", func() {
				b := &installer.OpenBaoBootstrapper{
					Clientset: clientset,
					Logger:    bootstrap.NewStepLogger(true),
					Config: installer.OpenBaoBootstrapConfig{
						DRBackupPath: filepath.Join(tmpDir, "nonexistent.enc.json"),
					},
				}
				b.SetCtx(ctx)

				err := b.PreFlightDRCheck()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("reading DR backup file"))
			})
		})

		Context("when DR backup path is empty", func() {
			It("returns an error", func() {
				b := &installer.OpenBaoBootstrapper{
					Clientset: clientset,
					Logger:    bootstrap.NewStepLogger(true),
					Config: installer.OpenBaoBootstrapConfig{
						DRBackupPath: "",
					},
				}
				b.SetCtx(ctx)

				err := b.PreFlightDRCheck()
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

			b := &installer.OpenBaoBootstrapper{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoBootstrapConfig{
					Namespace: "vault",
					Timeout:   5 * time.Second,
				},
			}
			b.SetCtx(ctx)

			err = b.WaitForInitialization()
			Expect(err).ToNot(HaveOccurred())
			result := b.GetUnsealSecret()
			Expect(result.Data).To(HaveKey("vault-unseal-0"))
		})

		It("times out when secret does not appear", func() {
			b := &installer.OpenBaoBootstrapper{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoBootstrapConfig{
					Namespace: "vault",
					Timeout:   1 * time.Second,
				},
			}
			b.SetCtx(ctx)

			err := b.WaitForInitialization()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timed out"))
		})
	})

	Describe("WaitForPodsReady", func() {
		It("succeeds when all expected pods are running and ready", func() {
			// Pre-create the namespace
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Create 3 ready pods
			for i := 0; i < 3; i++ {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("openbao-%d", i),
						Namespace: "vault",
						Labels:    map[string]string{"vault_cr": "openbao", "app.kubernetes.io/name": "vault"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						Conditions: []corev1.PodCondition{
							{Type: corev1.PodReady, Status: corev1.ConditionTrue},
						},
					},
				}
				_, err = clientset.CoreV1().Pods("vault").Create(ctx, pod, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
			}

			b := &installer.OpenBaoBootstrapper{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoBootstrapConfig{
					Namespace: "vault",
					Timeout:   5 * time.Second,
				},
			}
			b.SetCtx(ctx)
			b.SetReplicas(3)

			err = b.WaitForPodsReady()
			Expect(err).ToNot(HaveOccurred())
		})

		It("does not count the configurer pod toward the replica count", func() {
			// Pre-create the namespace
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// One ready server pod (expected: 1).
			serverPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openbao-0",
					Namespace: "vault",
					Labels:    map[string]string{"vault_cr": "openbao", "app.kubernetes.io/name": "vault"},
				},
				Status: corev1.PodStatus{
					Phase:      corev1.PodRunning,
					Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
				},
			}
			// The configurer pod also carries vault_cr=openbao — it must be
			// excluded, otherwise activePods (2) never equals expected (1).
			configurerPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openbao-configurer-abc123",
					Namespace: "vault",
					Labels:    map[string]string{"vault_cr": "openbao", "app.kubernetes.io/name": "vault-configurator"},
				},
				Status: corev1.PodStatus{
					Phase:      corev1.PodRunning,
					Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
				},
			}
			_, err = clientset.CoreV1().Pods("vault").Create(ctx, serverPod, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			_, err = clientset.CoreV1().Pods("vault").Create(ctx, configurerPod, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			b := &installer.OpenBaoBootstrapper{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoBootstrapConfig{
					Namespace: "vault",
					Timeout:   5 * time.Second,
				},
			}
			b.SetCtx(ctx)
			b.SetReplicas(1)

			err = b.WaitForPodsReady()
			Expect(err).ToNot(HaveOccurred())
		})

		It("times out when fewer pods than expected exist", func() {
			// Pre-create the namespace
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Create only 1 ready pod but expect 3
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openbao-0",
					Namespace: "vault",
					Labels:    map[string]string{"vault_cr": "openbao", "app.kubernetes.io/name": "vault"},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{Type: corev1.PodReady, Status: corev1.ConditionTrue},
					},
				},
			}
			_, err = clientset.CoreV1().Pods("vault").Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			b := &installer.OpenBaoBootstrapper{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoBootstrapConfig{
					Namespace: "vault",
					Timeout:   1 * time.Second,
				},
			}
			b.SetCtx(ctx)
			b.SetReplicas(3)

			err = b.WaitForPodsReady()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timed out"))
		})

		It("excludes terminating pods from the count", func() {
			// Pre-create the namespace
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			now := metav1.Now()

			// Create 1 ready pod and 1 terminating pod — expect 2 replicas
			readyPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openbao-0",
					Namespace: "vault",
					Labels:    map[string]string{"vault_cr": "openbao", "app.kubernetes.io/name": "vault"},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{Type: corev1.PodReady, Status: corev1.ConditionTrue},
					},
				},
			}
			terminatingPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "openbao-1",
					Namespace:         "vault",
					Labels:            map[string]string{"vault_cr": "openbao", "app.kubernetes.io/name": "vault"},
					DeletionTimestamp: &now,
					Finalizers:        []string{"test-finalizer"}, // Required for DeletionTimestamp in fake
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{Type: corev1.PodReady, Status: corev1.ConditionTrue},
					},
				},
			}
			_, err = clientset.CoreV1().Pods("vault").Create(ctx, readyPod, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			_, err = clientset.CoreV1().Pods("vault").Create(ctx, terminatingPod, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			b := &installer.OpenBaoBootstrapper{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoBootstrapConfig{
					Namespace: "vault",
					Timeout:   1 * time.Second,
				},
			}
			b.SetCtx(ctx)
			b.SetReplicas(2)

			// Only 1 active pod (terminating is excluded), but need 2 → times out
			err = b.WaitForPodsReady()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timed out"))
		})

		It("times out when pod exists but is not ready", func() {
			// Pre-create the namespace
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Create a pod that is Running but not Ready
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openbao-0",
					Namespace: "vault",
					Labels:    map[string]string{"vault_cr": "openbao", "app.kubernetes.io/name": "vault"},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{Type: corev1.PodReady, Status: corev1.ConditionFalse},
					},
				},
			}
			_, err = clientset.CoreV1().Pods("vault").Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			b := &installer.OpenBaoBootstrapper{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoBootstrapConfig{
					Namespace: "vault",
					Timeout:   1 * time.Second,
				},
			}
			b.SetCtx(ctx)
			b.SetReplicas(1)

			err = b.WaitForPodsReady()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timed out"))
		})
	})

	Describe("readiness timeout scaling", func() {
		It("adds the per-replica allowance to the base timeout (5m + 3m/replica by default)", func() {
			b := &installer.OpenBaoBootstrapper{
				Config: installer.OpenBaoBootstrapConfig{DRBackupPath: "backup.enc.json", Timeout: 5 * time.Minute},
			}
			b.SetReplicas(3)
			// Defaults (incl. ReadinessTimeoutPerReplica) are applied here.
			Expect(b.ValidateConfig()).To(Succeed())
			// 5m + 3m*3 = 14m
			Expect(b.ReadinessTimeout()).To(Equal(14 * time.Minute))
		})

		It("honors an explicit per-replica allowance", func() {
			b := &installer.OpenBaoBootstrapper{
				Config: installer.OpenBaoBootstrapConfig{
					Timeout:                    2 * time.Minute,
					ReadinessTimeoutPerReplica: 1 * time.Minute,
				},
			}
			b.SetReplicas(5)
			// 2m + 1m*5 = 7m
			Expect(b.ReadinessTimeout()).To(Equal(7 * time.Minute))
		})
	})

	Describe("ExtractAndEncrypt", func() {
		It("creates an encrypted DR backup file with the unseal keys", func() {
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

			b := &installer.OpenBaoBootstrapper{
				Logger: bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoBootstrapConfig{
					DRBackupPath: backupPath,
					AgeRecipient: recipient,
					AgeKeyPath:   keyFile,
				},
			}
			b.SetUnsealSecret(secret)

			err = b.ExtractAndEncrypt()
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
			Expect(backup).ToNot(HaveKey("password"))
			Expect(backup).To(HaveKey("unseal_keys"))
			unsealKeys, ok := backup["unseal_keys"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(unsealKeys).To(HaveKey("vault-unseal-0"))
			Expect(unsealKeys["vault-unseal-0"]).To(Equal("test-unseal-key-0"))
		})
	})

	Describe("ensureUnsealSecret", func() {
		const ns = "vault"

		newBootstrapper := func(backup map[string][]byte) *installer.OpenBaoBootstrapper {
			b := &installer.OpenBaoBootstrapper{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoBootstrapConfig{Namespace: ns},
			}
			b.SetCtx(ctx)
			b.SetBackupUnsealKeys(backup)
			return b
		}

		It("creates the secret from the backup keys when absent", func() {
			b := newBootstrapper(map[string][]byte{"vault-unseal-0": []byte("backup-key")})

			Expect(b.EnsureUnsealSecret()).To(Succeed())

			secret, err := clientset.CoreV1().Secrets(ns).Get(ctx, "openbao-unseal-keys", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Data).To(HaveKeyWithValue("vault-unseal-0", []byte("backup-key")))
		})

		It("overwrites an existing secret holding empty/wrong data", func() {
			// Pre-create a secret with empty data to simulate a partially
			// reconciled / wrong secret left by the operator.
			existing := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "openbao-unseal-keys", Namespace: ns},
				Data:       map[string][]byte{},
			}
			_, err := clientset.CoreV1().Secrets(ns).Create(ctx, existing, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			b := newBootstrapper(map[string][]byte{"vault-unseal-0": []byte("backup-key")})
			Expect(b.EnsureUnsealSecret()).To(Succeed())

			secret, err := clientset.CoreV1().Secrets(ns).Get(ctx, "openbao-unseal-keys", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Data).To(HaveKeyWithValue("vault-unseal-0", []byte("backup-key")))
		})
	})

	Describe("WaitForInitialization (DR restore)", func() {
		It("populates the secret from the backup when it is initially absent", func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			b := &installer.OpenBaoBootstrapper{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoBootstrapConfig{
					Namespace: "vault",
					Timeout:   30 * time.Second,
				},
			}
			b.SetCtx(ctx)
			b.SetBackupUnsealKeys(map[string][]byte{"vault-unseal-0": []byte("backup-key")})

			// First poll creates the secret from backup (returns "not done yet");
			// the next poll observes the populated secret and succeeds.
			err = b.WaitForInitialization()
			Expect(err).ToNot(HaveOccurred())

			secret := b.GetUnsealSecret()
			Expect(secret.Data).To(HaveKeyWithValue("vault-unseal-0", []byte("backup-key")))
		})
	})

})

func newVaultCR(spec map[string]interface{}) *unstructured.Unstructured {
	cr := &unstructured.Unstructured{Object: map[string]interface{}{"spec": spec}}
	cr.SetGroupVersionKind(schema.GroupVersionKind{Group: "vault.banzaicloud.com", Version: "v1alpha1", Kind: "Vault"})
	cr.SetName("openbao")
	cr.SetNamespace("vault")
	return cr
}

func newVaultDynClient(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	vaultGVR := schema.GroupVersionResource{Group: "vault.banzaicloud.com", Version: "v1alpha1", Resource: "vaults"}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{vaultGVR: "VaultList"},
		objs...,
	)
}

// writeEncryptedBackup SOPS-encrypts plaintext with a fresh age key and
// returns the backup and key file paths.
func writeEncryptedBackup(dir, plaintext string) (string, string) {
	keyFile := filepath.Join(dir, "age_key.txt")
	out, err := exec.Command("age-keygen", "-o", keyFile).CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), string(out))
	recipient := extractAgeRecipient(string(out))
	Expect(recipient).To(HavePrefix("age1"))

	plainPath := filepath.Join(dir, "backup.json")
	Expect(os.WriteFile(plainPath, []byte(plaintext), 0o600)).To(Succeed())
	backupPath := filepath.Join(dir, "backup.enc.json")
	Expect(sops.EncryptFile(plainPath, backupPath, recipient)).To(Succeed())
	return backupPath, keyFile
}

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
