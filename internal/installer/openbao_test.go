// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"bytes"
	"context"
	"encoding/base64"
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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
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
			// Pre-create the namespace so FindRelease is reachable.
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// FindRelease returns nil (no existing release in target namespace)
			helmMock.EXPECT().FindRelease("vault", "vault-operator").Return(nil, nil)

			// No operator Deployment exists (fake clientset has nothing), so InstallChart is called
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.ReleaseName == "vault-operator" &&
					cfg.ChartName == installer.DefaultBankVaultsChartRepo+"/vault-operator" &&
					cfg.Version == "1.24.0" &&
					cfg.Namespace == "vault" &&
					cfg.CreateNamespace == false
			}), mock.Anything).Return(nil)

			inst := &installer.OpenBaoInstaller{
				Helm:      helmMock,
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					Namespace:         "vault",
					OperatorChartRepo: installer.DefaultBankVaultsChartRepo,
				},
			}
			inst.SetCtx(ctx)

			err = inst.DeployBankVaultsOperator()
			Expect(err).ToNot(HaveOccurred())
		})

		It("installs with mirror overrides: custom chart repo, operator image, and pull secret", func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			helmMock.EXPECT().FindRelease("vault", "vault-operator").Return(nil, nil)
			// Credentials are set, so the chart registry login must happen first.
			helmMock.EXPECT().LoginRegistry(mock.Anything, "mirror.example.com", "u", "p").Return(nil)

			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				if cfg.ChartName != "oci://mirror.example.com/bank-vaults/helm-charts/vault-operator" {
					return false
				}
				image, ok := cfg.Values["image"].(map[string]interface{})
				if !ok {
					return false
				}
				if image["repository"] != "mirror.example.com/bank-vaults/vault-operator" || image["tag"] != "1.24.0" {
					return false
				}
				// The chart expects a list of secret names, not
				// LocalObjectReferences.
				secrets, ok := image["imagePullSecrets"].([]string)
				return ok && len(secrets) == 1 && secrets[0] == "openbao-registry"
			}), mock.Anything).Return(nil)

			inst := &installer.OpenBaoInstaller{
				Helm:      helmMock,
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					Namespace:         "vault",
					RegistryUser:      "u",
					RegistryPassword:  "p",
					OperatorImage:     "mirror.example.com/bank-vaults/vault-operator:1.24.0",
					OperatorChartRepo: "oci://mirror.example.com/bank-vaults/helm-charts",
				},
			}
			inst.SetCtx(ctx)

			Expect(inst.DeployBankVaultsOperator()).To(Succeed())
		})

		It("rejects an operator image without a tag or with only a digest", func() {
			newInstaller := func(operatorImage string) *installer.OpenBaoInstaller {
				inst := &installer.OpenBaoInstaller{
					Helm:      helmMock,
					Clientset: clientset,
					Logger:    bootstrap.NewStepLogger(true),
					Config: installer.OpenBaoInstallerConfig{
						Namespace:         "vault",
						OperatorImage:     operatorImage,
						OperatorChartRepo: "oci://mirror.example.com/bank-vaults/helm-charts",
					},
				}
				inst.SetCtx(ctx)
				return inst
			}

			// The vault-operator chart renders the image as repository:tag, so
			// untagged and digest-only references cannot be expressed.
			untagged := newInstaller("mirror.example.com/bank-vaults/vault-operator")
			Expect(untagged.DeployBankVaultsOperator()).To(MatchError(ContainSubstring("must include a tag")))

			digestOnly := newInstaller("mirror.example.com/bank-vaults/vault-operator@sha256:" + strings.Repeat("ab", 32))
			Expect(digestOnly.DeployBankVaultsOperator()).To(MatchError(ContainSubstring("must include a tag")))
		})

		It("performs fresh install when target namespace does not exist", func() {
			// Namespace "new-ns" is NOT created — FindRelease must be skipped.
			// No operator Deployment exists, so InstallChart is called directly.
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.ReleaseName == "vault-operator" &&
					cfg.Namespace == "new-ns" &&
					cfg.CreateNamespace == false
			}), mock.Anything).Return(nil)

			inst := &installer.OpenBaoInstaller{
				Helm:      helmMock,
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "new-ns"},
			}
			inst.SetCtx(ctx)

			err := inst.DeployBankVaultsOperator()
			Expect(err).ToNot(HaveOccurred())
		})

		It("upgrades when release already exists in target namespace", func() {
			// Pre-create the namespace so FindRelease is reachable.
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

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

			err = inst.DeployBankVaultsOperator()
			Expect(err).ToNot(HaveOccurred())
		})

		It("skips deployment when operator exists in another namespace", func() {
			// Pre-create the namespace so FindRelease is reachable.
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "second"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// FindRelease returns nil (not in target namespace)
			helmMock.EXPECT().FindRelease("second", "vault-operator").Return(nil, nil)

			// A running operator Deployment in another namespace simulates the
			// operator being installed elsewhere.
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vault-operator",
					Namespace: "other-ns",
					Labels:    map[string]string{"app.kubernetes.io/name": "vault-operator"},
				},
				Status: appsv1.DeploymentStatus{AvailableReplicas: 1},
			}
			_, err = clientset.AppsV1().Deployments("other-ns").Create(ctx, dep, metav1.CreateOptions{})
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

		It("installs and cleans orphaned RBAC when the ClusterRole lingers but no operator runs", func() {
			// Pre-create the namespace so FindRelease is reachable.
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			helmMock.EXPECT().FindRelease("vault", "vault-operator").Return(nil, nil)

			// Orphaned cluster-scoped RBAC from a torn-down install, but NO
			// operator Deployment anywhere — the regression scenario.
			cr := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "vault-operator"}}
			_, err = clientset.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			crb := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "vault-operator"}}
			_, err = clientset.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Must perform a fresh install rather than skip.
			helmMock.EXPECT().InstallChart(mock.Anything, mock.MatchedBy(func(cfg installer.ChartConfig) bool {
				return cfg.ReleaseName == "vault-operator" && cfg.Namespace == "vault"
			}), mock.Anything).Return(nil)

			inst := &installer.OpenBaoInstaller{
				Helm:      helmMock,
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "vault"},
			}
			inst.SetCtx(ctx)

			err = inst.DeployBankVaultsOperator()
			Expect(err).ToNot(HaveOccurred())

			// Orphaned RBAC must have been removed before install.
			_, err = clientset.RbacV1().ClusterRoles().Get(ctx, "vault-operator", metav1.GetOptions{})
			Expect(err).To(HaveOccurred())
			_, err = clientset.RbacV1().ClusterRoleBindings().Get(ctx, "vault-operator", metav1.GetOptions{})
			Expect(err).To(HaveOccurred())
		})

		It("returns an error when Helm InstallChart fails", func() {
			// Pre-create the namespace so FindRelease is reachable.
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			helmMock.EXPECT().FindRelease("vault", "vault-operator").Return(nil, nil)
			helmMock.EXPECT().InstallChart(mock.Anything, mock.Anything, mock.Anything).
				Return(fmt.Errorf("chart not found"))

			inst := &installer.OpenBaoInstaller{
				Helm:      helmMock,
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "vault"},
			}
			inst.SetCtx(ctx)

			err = inst.DeployBankVaultsOperator()
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

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					Namespace: "vault",
					Replicas:  3,
					Timeout:   5 * time.Second,
				},
			}
			inst.SetCtx(ctx)

			err = inst.WaitForPodsReady()
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

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					Namespace: "vault",
					Replicas:  1,
					Timeout:   5 * time.Second,
				},
			}
			inst.SetCtx(ctx)

			err = inst.WaitForPodsReady()
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

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					Namespace: "vault",
					Replicas:  3,
					Timeout:   1 * time.Second,
				},
			}
			inst.SetCtx(ctx)

			err = inst.WaitForPodsReady()
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

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					Namespace: "vault",
					Replicas:  2,
					Timeout:   1 * time.Second,
				},
			}
			inst.SetCtx(ctx)

			// Only 1 active pod (terminating is excluded), but need 2 → times out
			err = inst.WaitForPodsReady()
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

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					Namespace: "vault",
					Replicas:  1,
					Timeout:   1 * time.Second,
				},
			}
			inst.SetCtx(ctx)

			err = inst.WaitForPodsReady()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timed out"))
		})
	})

	Describe("readiness timeout scaling", func() {
		It("adds the per-replica allowance to the base timeout (5m + 3m/replica by default)", func() {
			inst := &installer.OpenBaoInstaller{
				Config: installer.OpenBaoInstallerConfig{Replicas: 3, Timeout: 5 * time.Minute},
			}
			// Defaults (incl. ReadinessTimeoutPerReplica) are applied here.
			Expect(inst.ValidateConfig()).To(Succeed())
			// 5m + 3m*3 = 14m
			Expect(inst.ReadinessTimeout()).To(Equal(14 * time.Minute))
		})

		It("honors an explicit per-replica allowance", func() {
			inst := &installer.OpenBaoInstaller{
				Config: installer.OpenBaoInstallerConfig{
					Replicas:                   5,
					Timeout:                    2 * time.Minute,
					ReadinessTimeoutPerReplica: 1 * time.Minute,
				},
			}
			// 2m + 1m*5 = 7m
			Expect(inst.ReadinessTimeout()).To(Equal(7 * time.Minute))
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

	Describe("EnsureImagePullSecret", func() {
		newInstaller := func(user, password string) *installer.OpenBaoInstaller {
			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					Namespace:        "vault",
					RegistryUser:     user,
					RegistryPassword: password,
					// Both default images live on ghcr.io.
					OpenBaoImage:    installer.DefaultOpenBaoImage,
					BankVaultsImage: installer.DefaultBankVaultsImage,
				},
			}
			inst.SetCtx(ctx)
			return inst
		}

		It("creates a dockerconfigjson secret when both credentials are set", func() {
			inst := newInstaller("gh-user", "gh-token")
			Expect(inst.EnsureImagePullSecret()).To(Succeed())

			secret, err := clientset.CoreV1().Secrets("vault").Get(ctx, "openbao-registry", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Type).To(Equal(corev1.SecretTypeDockerConfigJson))

			var cfg struct {
				Auths map[string]struct {
					Username string `json:"username"`
					Password string `json:"password"`
					Auth     string `json:"auth"`
				} `json:"auths"`
			}
			Expect(json.Unmarshal(secret.Data[corev1.DockerConfigJsonKey], &cfg)).To(Succeed())
			entry, ok := cfg.Auths["ghcr.io"]
			Expect(ok).To(BeTrue())
			Expect(entry.Username).To(Equal("gh-user"))
			Expect(entry.Password).To(Equal("gh-token"))
			Expect(entry.Auth).To(Equal(base64.StdEncoding.EncodeToString([]byte("gh-user:gh-token"))))
		})

		It("is a no-op when no credentials are set", func() {
			inst := newInstaller("", "")
			Expect(inst.EnsureImagePullSecret()).To(Succeed())

			_, err := clientset.CoreV1().Secrets("vault").Get(ctx, "openbao-registry", metav1.GetOptions{})
			Expect(k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("is a no-op when only one credential is set (rejected earlier by validateConfig)", func() {
			Expect(newInstaller("gh-user", "").EnsureImagePullSecret()).To(Succeed())
			Expect(newInstaller("", "gh-token").EnsureImagePullSecret()).To(Succeed())

			_, err := clientset.CoreV1().Secrets("vault").Get(ctx, "openbao-registry", metav1.GetOptions{})
			Expect(k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("is idempotent and refreshes credentials on re-run", func() {
			Expect(newInstaller("gh-user", "old-token").EnsureImagePullSecret()).To(Succeed())
			Expect(newInstaller("gh-user", "new-token").EnsureImagePullSecret()).To(Succeed())

			secret, err := clientset.CoreV1().Secrets("vault").Get(ctx, "openbao-registry", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(string(secret.Data[corev1.DockerConfigJsonKey])).To(ContainSubstring("new-token"))
		})

		It("emits one deduplicated auths entry per distinct registry host", func() {
			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					Namespace:        "vault",
					RegistryUser:     "u",
					RegistryPassword: "p",
					OpenBaoImage:     "registry-a.example.com/openbao/openbao:2.5.4",
					BankVaultsImage:  "registry-b.example.com/bank-vaults/bank-vaults:1.19.0",
					// Same host as OpenBaoImage — must be deduplicated.
					OperatorImage: "registry-a.example.com/bank-vaults/vault-operator:1.24.0",
				},
			}
			inst.SetCtx(ctx)
			Expect(inst.EnsureImagePullSecret()).To(Succeed())

			secret, err := clientset.CoreV1().Secrets("vault").Get(ctx, "openbao-registry", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			var cfg struct {
				Auths map[string]json.RawMessage `json:"auths"`
			}
			Expect(json.Unmarshal(secret.Data[corev1.DockerConfigJsonKey], &cfg)).To(Succeed())
			Expect(cfg.Auths).To(HaveLen(2))
			Expect(cfg.Auths).To(HaveKey("registry-a.example.com"))
			Expect(cfg.Auths).To(HaveKey("registry-b.example.com"))
		})

		It("derives the registry host from digest-only image references", func() {
			digest := "sha256:" + strings.Repeat("ab", 32)
			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					Namespace:        "vault",
					RegistryUser:     "u",
					RegistryPassword: "p",
					OpenBaoImage:     "registry.example.com/openbao/openbao@" + digest,
					BankVaultsImage:  "registry.example.com/bank-vaults/bank-vaults@" + digest,
				},
			}
			inst.SetCtx(ctx)
			Expect(inst.EnsureImagePullSecret()).To(Succeed())

			secret, err := clientset.CoreV1().Secrets("vault").Get(ctx, "openbao-registry", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			var cfg struct {
				Auths map[string]json.RawMessage `json:"auths"`
			}
			Expect(json.Unmarshal(secret.Data[corev1.DockerConfigJsonKey], &cfg)).To(Succeed())
			Expect(cfg.Auths).To(HaveLen(1))
			Expect(cfg.Auths).To(HaveKey("registry.example.com"))
		})
	})

	Describe("validateConfig image/chart defaults", func() {
		It("backfills empty image and chart fields with the Default* values", func() {
			inst := &installer.OpenBaoInstaller{
				Logger: bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{Namespace: "vault", Replicas: 1},
			}
			Expect(inst.ValidateConfig()).To(Succeed())
			Expect(inst.Config.OpenBaoImage).To(Equal(installer.DefaultOpenBaoImage))
			Expect(inst.Config.BankVaultsImage).To(Equal(installer.DefaultBankVaultsImage))
			Expect(inst.Config.OperatorImage).To(Equal(installer.DefaultOperatorImage))
			Expect(inst.Config.OperatorChartRepo).To(Equal(installer.DefaultBankVaultsChartRepo))
		})

		It("leaves explicitly-set overrides untouched", func() {
			inst := &installer.OpenBaoInstaller{
				Logger: bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					Namespace:    "vault",
					Replicas:     1,
					OpenBaoImage: "mirror.example.com/openbao:2.5.4",
				},
			}
			Expect(inst.ValidateConfig()).To(Succeed())
			Expect(inst.Config.OpenBaoImage).To(Equal("mirror.example.com/openbao:2.5.4"))
		})

		It("rejects an operator chart repo without the oci:// scheme", func() {
			inst := &installer.OpenBaoInstaller{
				Logger: bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					Namespace:         "vault",
					Replicas:          1,
					OperatorChartRepo: "https://mirror.example.com/bank-vaults/helm-charts",
				},
			}
			Expect(inst.ValidateConfig()).To(MatchError(ContainSubstring("oci://")))
		})

		It("rejects partial registry credentials", func() {
			newInstaller := func(user, password string) *installer.OpenBaoInstaller {
				return &installer.OpenBaoInstaller{
					Logger: bootstrap.NewStepLogger(true),
					Config: installer.OpenBaoInstallerConfig{
						Namespace:        "vault",
						Replicas:         1,
						RegistryUser:     user,
						RegistryPassword: password,
					},
				}
			}
			Expect(newInstaller("gh-user", "").ValidateConfig()).ToNot(Succeed())
			Expect(newInstaller("", "gh-token").ValidateConfig()).ToNot(Succeed())
			Expect(newInstaller("gh-user", "gh-token").ValidateConfig()).To(Succeed())
			Expect(newInstaller("", "").ValidateConfig()).To(Succeed())
		})
	})

	Describe("Vault CR template rendering", func() {
		// templateData mirrors the unexported vaultCRTemplateData struct
		// so the test can render the template independently.
		type templateData struct {
			Namespace           string
			OpenBaoImage        string
			BankVaultsImage     string
			SecretsEngineName   string
			BaoUsername         string
			BaoPassword         string
			Replicas            int
			StorageSize         string
			RetryJoinAddrs      []string
			ImagePullSecretName string
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

		It("wires imagePullSecrets onto the openbao ServiceAccount when set", func() {
			data := templateData{
				Namespace:           "vault",
				OpenBaoImage:        "ghcr.io/codesphere-cloud/docker/quay.io/openbao/openbao-cs-patched:2.5.4",
				BankVaultsImage:     "ghcr.io/codesphere-cloud/docker/banzaicloud/bank-vaults:1.19.0",
				SecretsEngineName:   "cs-secrets-engine",
				BaoUsername:         "admin",
				BaoPassword:         "test-password",
				Replicas:            1,
				StorageSize:         "10Gi",
				RetryJoinAddrs:      []string{"http://openbao-0.vault.svc.cluster.local:8200"},
				ImagePullSecretName: "openbao-registry",
			}

			docs := renderTemplate(data)
			sa := findDoc(docs, "ServiceAccount")
			Expect(sa).ToNot(BeNil())
			pullSecrets := sa["imagePullSecrets"].([]interface{})
			Expect(pullSecrets).To(HaveLen(1))
			Expect(pullSecrets[0].(map[string]interface{})["name"]).To(Equal("openbao-registry"))
		})

		It("omits imagePullSecrets when no pull secret name is set", func() {
			data := templateData{
				Namespace:         "vault",
				OpenBaoImage:      "ghcr.io/codesphere-cloud/docker/quay.io/openbao/openbao-cs-patched:2.5.4",
				BankVaultsImage:   "ghcr.io/codesphere-cloud/docker/banzaicloud/bank-vaults:1.19.0",
				SecretsEngineName: "cs-secrets-engine",
				BaoUsername:       "admin",
				BaoPassword:       "test-password",
				Replicas:          1,
				StorageSize:       "10Gi",
				RetryJoinAddrs:    []string{"http://openbao-0.vault.svc.cluster.local:8200"},
			}

			docs := renderTemplate(data)
			sa := findDoc(docs, "ServiceAccount")
			Expect(sa).ToNot(BeNil())
			Expect(sa).ToNot(HaveKey("imagePullSecrets"))
		})

		It("renders valid YAML with raft storage and PVC for replicas=1", func() {
			data := templateData{
				Namespace:         "vault",
				OpenBaoImage:      "ghcr.io/codesphere-cloud/docker/quay.io/openbao/openbao-cs-patched:2.5.4",
				BankVaultsImage:   "ghcr.io/codesphere-cloud/docker/banzaicloud/bank-vaults:1.19.0",
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
			Expect(spec["image"]).To(Equal("ghcr.io/codesphere-cloud/docker/quay.io/openbao/openbao-cs-patched:2.5.4"))

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

			// Verify the rw policy grants KV access plus the password-policy
			// management the `generate` secret flow needs.
			policies := externalConfig["policies"].([]interface{})
			Expect(policies).To(HaveLen(1))
			policy := policies[0].(map[string]interface{})
			Expect(policy["name"]).To(Equal("cs-secrets-engine-rw"))
			rules := policy["rules"].(string)
			Expect(rules).To(ContainSubstring(`path "cs-secrets-engine/data/*"`))
			Expect(rules).To(ContainSubstring(`path "cs-secrets-engine/metadata/*"`))
			Expect(rules).To(ContainSubstring(`path "sys/policies/password/*"`))

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
				OpenBaoImage:      "ghcr.io/codesphere-cloud/docker/quay.io/openbao/openbao-cs-patched:2.5.4",
				BankVaultsImage:   "ghcr.io/codesphere-cloud/docker/banzaicloud/bank-vaults:1.19.0",
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
			envValues := make(map[string]string, len(envVars))
			for _, e := range envVars {
				m := e.(map[string]interface{})
				name := m["name"].(string)
				envNames = append(envNames, name)
				if v, ok := m["value"].(string); ok {
					envValues[name] = v
				}
			}
			Expect(envNames).To(ContainElements("POD_NAME", "BAO_CLUSTER_ADDR", "BAO_API_ADDR"))

			Expect(envValues["BAO_CLUSTER_ADDR"]).To(Equal("http://$(POD_NAME).vault.svc.cluster.local:8201"))
			Expect(envValues["BAO_API_ADDR"]).To(Equal("http://$(POD_NAME).vault.svc.cluster.local:8200"))
		})
	})

	Describe("BuildRetryJoinAddrs", func() {
		It("targets the per-pod ClusterIP service, not a headless service", func() {
			addrs := installer.BuildRetryJoinAddrs(3, "second")
			Expect(addrs).To(Equal([]string{
				"http://openbao-0.second.svc.cluster.local:8200",
				"http://openbao-1.second.svc.cluster.local:8200",
				"http://openbao-2.second.svc.cluster.local:8200",
			}))
		})

		It("produces a single self-referencing address for one replica", func() {
			Expect(installer.BuildRetryJoinAddrs(1, "vault")).To(Equal([]string{
				"http://openbao-0.vault.svc.cluster.local:8200",
			}))
		})
	})

	Describe("HasExistingDeployment", func() {
		It("returns false when the target namespace does not exist", func() {
			// Use a fake dynamic client with no objects — namespace "new-ns" does not exist.
			scheme := runtime.NewScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme)

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				DynClient: dynClient,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "non-existent-ns"},
			}
			inst.SetCtx(ctx)

			exists, err := inst.HasExistingDeployment()
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeFalse())
		})

		It("returns true when PVCs with vault_cr=openbao exist in the namespace", func() {
			scheme := runtime.NewScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme)

			// Pre-create the namespace
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Since fake dynamic client returns NotFound for unregistered resources,
			// the hasExistingDeployment method will fall through to PVC check.
			// Create a PVC with the vault_cr=openbao label to verify detection.
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vault-raft-openbao-0",
					Namespace: "vault",
					Labels:    map[string]string{"vault_cr": "openbao"},
				},
			}
			_, err = clientset.CoreV1().PersistentVolumeClaims("vault").Create(ctx, pvc, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				DynClient: dynClient,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "vault"},
			}
			inst.SetCtx(ctx)

			exists, checkErr := inst.HasExistingDeployment()
			Expect(checkErr).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("returns false when namespace exists but has no vault resources", func() {
			scheme := runtime.NewScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme)

			// Pre-create the namespace
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "empty-ns"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				DynClient: dynClient,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "empty-ns"},
			}
			inst.SetCtx(ctx)

			exists, checkErr := inst.HasExistingDeployment()
			Expect(checkErr).ToNot(HaveOccurred())
			Expect(exists).To(BeFalse())
		})
	})

	Describe("releaseExistsInTargetNamespace", func() {
		It("returns false without querying Helm when the namespace does not exist", func() {
			// No namespace created, and no Helm expectations set — FindRelease
			// must not be called.
			inst := &installer.OpenBaoInstaller{
				Helm:      helmMock,
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "absent-ns"},
			}
			inst.SetCtx(ctx)

			exists, err := inst.ReleaseExistsInTargetNamespace("vault-operator")
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeFalse())
		})

		It("returns true when a release exists in the namespace", func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			helmMock.EXPECT().FindRelease("vault", "vault-operator").Return(&installer.ReleaseInfo{Name: "vault-operator"}, nil)

			inst := &installer.OpenBaoInstaller{
				Helm:      helmMock,
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "vault"},
			}
			inst.SetCtx(ctx)

			exists, err := inst.ReleaseExistsInTargetNamespace("vault-operator")
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("wraps the error when FindRelease fails", func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			helmMock.EXPECT().FindRelease("vault", "vault-operator").Return(nil, fmt.Errorf("helm boom"))

			inst := &installer.OpenBaoInstaller{
				Helm:      helmMock,
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "vault"},
			}
			inst.SetCtx(ctx)

			_, err = inst.ReleaseExistsInTargetNamespace("vault-operator")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("finding release vault-operator in namespace vault"))
			Expect(err.Error()).To(ContainSubstring("helm boom"))
		})
	})

	Describe("operatorRunningClusterWide", func() {
		It("returns true when an available operator Deployment runs in any namespace", func() {
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vault-operator",
					Namespace: "other-ns",
					Labels:    map[string]string{"app.kubernetes.io/name": "vault-operator"},
				},
				Status: appsv1.DeploymentStatus{AvailableReplicas: 1},
			}
			_, err := clientset.AppsV1().Deployments("other-ns").Create(ctx, dep, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "vault"},
			}
			inst.SetCtx(ctx)

			running, err := inst.OperatorRunningClusterWide()
			Expect(err).ToNot(HaveOccurred())
			Expect(running).To(BeTrue())
		})

		It("returns false when the operator Deployment has no available replicas", func() {
			// A Deployment that exists but is scaled to zero / has no ready pods
			// cannot reconcile the Vault CR, so it must not suppress a (re)deploy.
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vault-operator",
					Namespace: "other-ns",
					Labels:    map[string]string{"app.kubernetes.io/name": "vault-operator"},
				},
				Status: appsv1.DeploymentStatus{AvailableReplicas: 0},
			}
			_, err := clientset.AppsV1().Deployments("other-ns").Create(ctx, dep, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "vault"},
			}
			inst.SetCtx(ctx)

			running, err := inst.OperatorRunningClusterWide()
			Expect(err).ToNot(HaveOccurred())
			Expect(running).To(BeFalse())
		})

		It("returns false when only an orphaned ClusterRole exists (no Deployment)", func() {
			// A lingering ClusterRole must NOT be mistaken for a running operator.
			cr := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "vault-operator"}}
			_, err := clientset.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "vault"},
			}
			inst.SetCtx(ctx)

			running, err := inst.OperatorRunningClusterWide()
			Expect(err).ToNot(HaveOccurred())
			Expect(running).To(BeFalse())
		})
	})

	Describe("ensureUnsealSecret", func() {
		const ns = "vault"

		newInstaller := func(backup map[string][]byte) *installer.OpenBaoInstaller {
			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: ns},
			}
			inst.SetCtx(ctx)
			inst.SetBackupUnsealKeys(backup)
			return inst
		}

		It("creates the secret from the backup keys when absent", func() {
			inst := newInstaller(map[string][]byte{"vault-unseal-0": []byte("backup-key")})

			Expect(inst.EnsureUnsealSecret()).To(Succeed())

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

			inst := newInstaller(map[string][]byte{"vault-unseal-0": []byte("backup-key")})
			Expect(inst.EnsureUnsealSecret()).To(Succeed())

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

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				Logger:    bootstrap.NewStepLogger(true),
				Config: installer.OpenBaoInstallerConfig{
					Namespace: "vault",
					Timeout:   30 * time.Second,
				},
			}
			inst.SetCtx(ctx)
			inst.SetBackupUnsealKeys(map[string][]byte{"vault-unseal-0": []byte("backup-key")})

			// First poll creates the secret from backup (returns "not done yet");
			// the next poll observes the populated secret and succeeds.
			err = inst.WaitForInitialization()
			Expect(err).ToNot(HaveOccurred())

			secret := inst.GetUnsealSecret()
			Expect(secret.Data).To(HaveKeyWithValue("vault-unseal-0", []byte("backup-key")))
		})
	})

	Describe("CleanStaleInstallState", func() {
		It("succeeds and skips the pod wait when no Vault CR exists, even if labeled pods linger", func() {
			scheme := runtime.NewScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme) // no Vault CR registered → Delete returns NotFound

			nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, nsObj, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// A lingering pod that never terminates — if the code waited for pods
			// without a deleted CR, this would time out.
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openbao-0",
					Namespace: "vault",
					Labels:    map[string]string{"vault_cr": "openbao", "app.kubernetes.io/name": "vault"},
				},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			}
			_, err = clientset.CoreV1().Pods("vault").Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				DynClient: dynClient,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "vault", Timeout: 2 * time.Second},
			}
			inst.SetCtx(ctx)

			err = inst.CleanStaleInstallState()
			Expect(err).ToNot(HaveOccurred())
		})

		It("deletes a stale Vault CR, PVCs, and the unseal secret", func() {
			vaultGVR := schema.GroupVersionResource{Group: "vault.banzaicloud.com", Version: "v1alpha1", Resource: "vaults"}
			scheme := runtime.NewScheme()
			vaultCR := &unstructured.Unstructured{}
			vaultCR.SetGroupVersionKind(schema.GroupVersionKind{Group: "vault.banzaicloud.com", Version: "v1alpha1", Kind: "Vault"})
			vaultCR.SetName("openbao")
			vaultCR.SetNamespace("vault")
			dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
				scheme,
				map[schema.GroupVersionResource]string{vaultGVR: "VaultList"},
				vaultCR,
			)

			nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vault"}}
			_, err := clientset.CoreV1().Namespaces().Create(ctx, nsObj, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Two stale PVCs labeled for the openbao Vault CR.
			for i := 0; i < 2; i++ {
				pvc := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("vault-raft-openbao-%d", i),
						Namespace: "vault",
						Labels:    map[string]string{"vault_cr": "openbao"},
					},
				}
				_, err = clientset.CoreV1().PersistentVolumeClaims("vault").Create(ctx, pvc, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
			}

			// A stale unseal secret.
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "openbao-unseal-keys", Namespace: "vault"},
				Data:       map[string][]byte{"vault-unseal-0": []byte("old")},
			}
			_, err = clientset.CoreV1().Secrets("vault").Create(ctx, secret, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			inst := &installer.OpenBaoInstaller{
				Clientset: clientset,
				DynClient: dynClient,
				Logger:    bootstrap.NewStepLogger(true),
				Config:    installer.OpenBaoInstallerConfig{Namespace: "vault", Timeout: 5 * time.Second},
			}
			inst.SetCtx(ctx)

			err = inst.CleanStaleInstallState()
			Expect(err).ToNot(HaveOccurred())

			// Vault CR removed.
			_, getErr := dynClient.Resource(vaultGVR).Namespace("vault").Get(ctx, "openbao", metav1.GetOptions{})
			Expect(getErr).To(HaveOccurred())

			// PVCs removed.
			pvcs, listErr := clientset.CoreV1().PersistentVolumeClaims("vault").List(ctx, metav1.ListOptions{})
			Expect(listErr).ToNot(HaveOccurred())
			Expect(pvcs.Items).To(BeEmpty())

			// Unseal secret removed.
			_, secretErr := clientset.CoreV1().Secrets("vault").Get(ctx, "openbao-unseal-keys", metav1.GetOptions{})
			Expect(secretErr).To(HaveOccurred())
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
