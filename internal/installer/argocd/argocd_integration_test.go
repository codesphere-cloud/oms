// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// -- stub implementations of the injectable interfaces --

type stubArgoCDInstaller struct{ err error }

func (s *stubArgoCDInstaller) Install() error { return s.err }

type stubArgoCDResources struct{ err error }

func (s *stubArgoCDResources) ApplyAll(_ context.Context) error { return s.err }

type stubVaultSecretsDeployer struct {
	err            error
	calledVaultFile string
	calledNamespace string
	calledSecretName string
}

func (s *stubVaultSecretsDeployer) CreateSecretFromVault(_ context.Context, vaultFile, _, namespace, secretName string) error {
	s.calledVaultFile = vaultFile
	s.calledNamespace = namespace
	s.calledSecretName = secretName
	return s.err
}

type stubPCAppsRunner struct {
	err             error
	calledVersion   string
	calledNamespace string
}

func (s *stubPCAppsRunner) Install(_ context.Context) error { return s.err }

// -- helpers --

// writeBom writes a minimal bom.json with an optional pc-applications entry.
func writeBom(dir string, pcAppsOCIRef string) string {
	components := map[string]interface{}{}
	if pcAppsOCIRef != "" {
		components["pc-applications"] = map[string]interface{}{
			"files": map[string]interface{}{
				"chart": map[string]interface{}{
					"ociRef": pcAppsOCIRef,
				},
			},
		}
	}
	data, _ := json.Marshal(map[string]interface{}{"components": components})
	path := filepath.Join(dir, "bom.json")
	Expect(os.WriteFile(path, data, 0644)).To(Succeed())
	return path
}

// -- tests --

var _ = Describe("RunWithDeps", func() {
	var (
		ctx        context.Context
		kubeClient ctrlclient.Client
		tmpDir     string

		argoCDInstaller  *stubArgoCDInstaller
		argoCDResources  *stubArgoCDResources
		vaultDeployer    *stubVaultSecretsDeployer
		pcAppsRunner     *stubPCAppsRunner

		deps argocd.Deps
		opts argocd.Opts
	)

	BeforeEach(func() {
		ctx = context.Background()
		kubeClient = fake.NewClientBuilder().Build()

		var err error
		tmpDir, err = os.MkdirTemp("", "argocd-integration-test-*")
		Expect(err).NotTo(HaveOccurred())

		argoCDInstaller = &stubArgoCDInstaller{}
		argoCDResources = &stubArgoCDResources{}
		vaultDeployer = &stubVaultSecretsDeployer{}
		pcAppsRunner = &stubPCAppsRunner{}

		deps = argocd.Deps{
			NewArgoCDInstaller: func(_, _, _ string) (argocd.ArgoCDInstaller, error) {
				return argoCDInstaller, nil
			},
			NewArgoCDResources: func(_, _, _ string) (argocd.ArgoCDResourcesApplier, error) {
				return argoCDResources, nil
			},
			NewVaultSecretsDeployer: func(_ ctrlclient.Client) argocd.VaultSecretsDeployer {
				return vaultDeployer
			},
			NewPCAppsRunner: func(_ ctrlclient.Client, version, namespace string) (argocd.PCAppsRunner, error) {
				pcAppsRunner.calledVersion = version
				pcAppsRunner.calledNamespace = namespace
				return pcAppsRunner, nil
			},
		}

		opts = argocd.Opts{
			DatacenterID: "42",
			OCIPassword:  "token",
			RegistryURL:  "ghcr.io/codesphere-cloud/charts",
		}
	})

	AfterEach(func() {
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	Context("BOM parsing", func() {
		It("returns an error when bom.json does not exist", func() {
			opts.BomPath = filepath.Join(tmpDir, "missing.json")
			err := argocd.RunWithDeps(ctx, kubeClient, opts, deps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse bom.json"))
		})

		It("returns an error when bom.json contains invalid JSON", func() {
			path := filepath.Join(tmpDir, "bom.json")
			Expect(os.WriteFile(path, []byte("{invalid"), 0644)).To(Succeed())
			opts.BomPath = path
			err := argocd.RunWithDeps(ctx, kubeClient, opts, deps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse bom.json"))
		})
	})

	Context("with a valid BOM containing pc-applications", func() {
		BeforeEach(func() {
			opts.BomPath = writeBom(tmpDir, "ghcr.io/codesphere-cloud/charts/pc-applications:2.3.4")
		})

		It("succeeds and installs pc-apps at the version from the BOM", func() {
			err := argocd.RunWithDeps(ctx, kubeClient, opts, deps)
			Expect(err).NotTo(HaveOccurred())
			Expect(pcAppsRunner.calledVersion).To(Equal("2.3.4"))
			Expect(pcAppsRunner.calledNamespace).To(Equal("argocd"))
		})

		It("does not call the ArgoCD installer when InstallArgoCD is false", func() {
			opts.InstallArgoCD = false
			argoCDInstaller.err = errors.New("should not be called")
			Expect(argocd.RunWithDeps(ctx, kubeClient, opts, deps)).To(Succeed())
		})

		It("calls the ArgoCD installer when InstallArgoCD is true", func() {
			opts.InstallArgoCD = true
			Expect(argocd.RunWithDeps(ctx, kubeClient, opts, deps)).To(Succeed())
		})

		It("propagates ArgoCD installer failure", func() {
			opts.InstallArgoCD = true
			argoCDInstaller.err = errors.New("helm timeout")
			err := argocd.RunWithDeps(ctx, kubeClient, opts, deps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install ArgoCD"))
			Expect(err.Error()).To(ContainSubstring("helm timeout"))
		})

		It("does not call the vault deployer when VaultFile is empty", func() {
			opts.VaultFile = ""
			vaultDeployer.err = errors.New("should not be called")
			Expect(argocd.RunWithDeps(ctx, kubeClient, opts, deps)).To(Succeed())
		})

		It("calls the vault deployer with the correct arguments when VaultFile is set", func() {
			opts.VaultFile = "/path/to/vault.yaml"
			opts.VaultNamespace = "my-ns"
			opts.VaultSecretName = "my-secret"
			Expect(argocd.RunWithDeps(ctx, kubeClient, opts, deps)).To(Succeed())
			Expect(vaultDeployer.calledVaultFile).To(Equal("/path/to/vault.yaml"))
			Expect(vaultDeployer.calledNamespace).To(Equal("my-ns"))
			Expect(vaultDeployer.calledSecretName).To(Equal("my-secret"))
		})

		It("propagates vault deployer failure", func() {
			opts.VaultFile = "/path/to/vault.yaml"
			vaultDeployer.err = errors.New("decrypt failed")
			err := argocd.RunWithDeps(ctx, kubeClient, opts, deps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to deploy vault secrets"))
			Expect(err.Error()).To(ContainSubstring("decrypt failed"))
		})

		It("propagates ArgoCDResources.ApplyAll failure", func() {
			argoCDResources.err = errors.New("cluster unreachable")
			err := argocd.RunWithDeps(ctx, kubeClient, opts, deps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to apply ArgoCD resources"))
			Expect(err.Error()).To(ContainSubstring("cluster unreachable"))
		})

		It("propagates pc-apps installation failure", func() {
			pcAppsRunner.err = errors.New("chart not found")
			err := argocd.RunWithDeps(ctx, kubeClient, opts, deps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install pc-apps"))
			Expect(err.Error()).To(ContainSubstring("chart not found"))
		})
	})

	Context("with a BOM that has no pc-applications component", func() {
		BeforeEach(func() {
			opts.BomPath = writeBom(tmpDir, "")
		})

		It("succeeds without attempting pc-apps installation", func() {
			pcAppsRunner.err = errors.New("should not be called")
			Expect(argocd.RunWithDeps(ctx, kubeClient, opts, deps)).To(Succeed())
		})

		It("still applies ArgoCD resources", func() {
			argoCDResources.err = errors.New("apply failed")
			err := argocd.RunWithDeps(ctx, kubeClient, opts, deps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("apply failed"))
		})
	})

	Context("default vault namespace and secret name", func() {
		BeforeEach(func() {
			opts.BomPath = writeBom(tmpDir, "")
			opts.VaultFile = "/some/vault.yaml"
			opts.VaultNamespace = ""
			opts.VaultSecretName = ""
		})

		It("applies DefaultVaultNamespace when VaultNamespace is empty", func() {
			Expect(argocd.RunWithDeps(ctx, kubeClient, opts, deps)).To(Succeed())
			Expect(vaultDeployer.calledNamespace).To(Equal(argocd.DefaultVaultNamespace))
		})

		It("applies DefaultVaultSecretName when VaultSecretName is empty", func() {
			Expect(argocd.RunWithDeps(ctx, kubeClient, opts, deps)).To(Succeed())
			Expect(vaultDeployer.calledSecretName).To(Equal(argocd.DefaultVaultSecretName))
		})
	})
})
