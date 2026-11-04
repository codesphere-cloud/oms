// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"context"
	"errors"
	"os"

	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Shared GCP install inputs", func() {
	var (
		b       *gcp.GCPBootstrapper
		manager *installer.InstallConfig
		fw      *util.MockFileIO
		loader  *gcp.MockInstallVaultLoader
	)

	BeforeEach(func() {
		fw = util.NewMockFileIO(GinkgoT())
		loader = gcp.NewMockInstallVaultLoader(GinkgoT())
		configManager, err := installer.NewInstallConfigManager("plain", "")
		Expect(err).NotTo(HaveOccurred())

		manager = configManager.(*installer.InstallConfig)
		manager.SetFileIO(fw)
		b, err = gcp.NewGCPBootstrapper(context.Background(), nil, bootstrap.NewStepLogger(true),
			&gcp.CodesphereEnvironment{InstallConfigPath: "/out/config.yaml", SecretsFilePath: "/out/vault.yaml"},
			manager, nil, fw, nil, nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		b.InstallVaultLoader = loader
	})

	It("merges the template and config overlays into the multi-VM profile in order", func() {
		b.Env.InstallConfigTemplatePath = "/template.yaml"
		b.Env.InstallConfigs = []string{"/first.yaml", "/second.yaml"}

		fw.EXPECT().Exists("/out/config.yaml").Return(false).Once()
		fw.EXPECT().ReadFile("/out/vault.yaml").Return(nil, os.ErrNotExist).Maybe()
		fw.EXPECT().ReadFile("/template.yaml").Return([]byte("dataCenter:\n  name: template\n  city: Berlin\n"), nil).Once()
		fw.EXPECT().ReadFile("/first.yaml").Return([]byte("dataCenter:\n  name: first\npcApps:\n  custom: retained\n"), nil).Once()
		fw.EXPECT().ReadFile("/second.yaml").Return([]byte("dataCenter:\n  name: second\n"), nil).Once()

		Expect(b.EnsureInstallConfig()).To(Succeed())

		config := manager.GetInstallConfig()
		Expect(config.Datacenter.Name).To(Equal("second"))
		Expect(config.Datacenter.City).To(Equal("Berlin"))
		Expect(config.PcApps).To(HaveKeyWithValue("custom", "retained"))
		Expect(b.Env.DataCenters[0].InstallConfig).To(BeIdenticalTo(config))
	})

	It("uses seed secrets when rendering an existing multi-VM config and preserves active secrets", func() {
		b.Env.InstallVaultPath = "/seed.yaml"
		b.Env.InstallVaultPrivKey = "/age-key"
		seed := &files.InstallVault{}
		seed.SetSecret(files.SecretEntry{Name: "city", File: &files.SecretFile{Content: "Berlin"}})
		seed.SetSecret(files.SecretEntry{Name: "password", Fields: &files.SecretFields{Password: "seed"}})
		loader.EXPECT().Load("/seed.yaml", "/age-key").Return(seed, nil).Once()
		fw.EXPECT().Exists("/out/config.yaml").Return(true).Once()

		active := []byte("secrets:\n- name: password\n  fields:\n    password: active\n")
		fw.EXPECT().ReadFile("/out/vault.yaml").Return(active, nil).Twice()
		fw.EXPECT().ReadFile("/out/config.yaml").Return([]byte("dataCenter:\n  city: '{{ secret \"city\" }}'\n"), nil).Once()

		Expect(b.EnsureInstallConfig()).To(Succeed())
		Expect(manager.GetInstallConfig().Datacenter.City).To(Equal("Berlin"))
		Expect(b.EnsureSecrets()).To(Succeed())
		Expect(manager.GetVault().GetSecret("password").Fields.Password).To(Equal("active"))
		Expect(manager.GetVault().GetSecret("city").File.Content).To(Equal("Berlin"))
		Expect(b.Env.Secrets).To(BeIdenticalTo(manager.GetVault()))
	})

	It("renders overlays from the seed vault on a new multi-VM installation", func() {
		b.Env.InstallVaultPath = "/seed.yaml"
		b.Env.InstallConfigs = []string{"/overlay.yaml"}
		seed := &files.InstallVault{}
		seed.SetSecret(files.SecretEntry{Name: "city", File: &files.SecretFile{Content: "Berlin"}})
		loader.EXPECT().Load("/seed.yaml", "").Return(seed, nil).Once()
		fw.EXPECT().Exists("/out/config.yaml").Return(false).Once()
		fw.EXPECT().ReadFile("/out/vault.yaml").Return(nil, os.ErrNotExist).Maybe()
		fw.EXPECT().ReadFile("/overlay.yaml").Return([]byte("dataCenter:\n  city: '{{ secret \"city\" }}'\n"), nil).Once()

		Expect(b.EnsureInstallConfig()).To(Succeed())
		Expect(manager.GetInstallConfig().Datacenter.City).To(Equal("Berlin"))
	})

	It("renders a new config overlay from an existing vault before seed defaults", func() {
		b.Env.InstallVaultPath = "/seed.yaml"
		b.Env.InstallConfigs = []string{"/overlay.yaml"}
		seed := &files.InstallVault{}
		seed.SetSecret(files.SecretEntry{Name: "city", File: &files.SecretFile{Content: "seed-city"}})
		loader.EXPECT().Load("/seed.yaml", "").Return(seed, nil).Once()
		fw.EXPECT().Exists("/out/config.yaml").Return(false).Once()
		fw.EXPECT().ReadFile("/out/vault.yaml").Return([]byte("secrets:\n- name: city\n  file:\n    content: active-city\n"), nil).Once()
		fw.EXPECT().ReadFile("/overlay.yaml").Return([]byte("dataCenter:\n  city: '{{ secret \"city\" }}'\n"), nil).Once()
		Expect(b.EnsureInstallConfig()).To(Succeed())
		Expect(manager.GetInstallConfig().Datacenter.City).To(Equal("active-city"))
	})

	It("reports vault load failures before rendering configuration", func() {
		b.Env.InstallVaultPath = "/seed.yaml"

		loader.EXPECT().Load("/seed.yaml", "").Return(nil, errors.New("decryption failed")).Once()
		fw.EXPECT().Exists("/out/config.yaml").Return(false).Once()
		fw.EXPECT().ReadFile("/out/vault.yaml").Return(nil, os.ErrNotExist).Maybe()
		Expect(b.EnsureInstallConfig()).To(MatchError(ContainSubstring("decryption failed")))
	})

	It("reports unreadable overlays", func() {
		b.Env.InstallConfigs = []string{"/overlay.yaml"}

		fw.EXPECT().Exists("/out/config.yaml").Return(false).Once()
		fw.EXPECT().ReadFile("/out/vault.yaml").Return(nil, os.ErrNotExist).Maybe()
		fw.EXPECT().ReadFile("/overlay.yaml").Return(nil, errors.New("permission denied")).Once()
		Expect(b.EnsureInstallConfig()).To(MatchError(ContainSubstring("failed to read install config /overlay.yaml")))
	})

	It("rejects malformed overlay YAML", func() {
		b.Env.InstallConfigs = []string{"/overlay.yaml"}

		fw.EXPECT().Exists("/out/config.yaml").Return(false).Once()
		fw.EXPECT().ReadFile("/out/vault.yaml").Return(nil, os.ErrNotExist).Maybe()
		fw.EXPECT().ReadFile("/overlay.yaml").Return([]byte("dataCenter: ["), nil).Once()
		Expect(b.EnsureInstallConfig()).To(MatchError(ContainSubstring("failed to parse install config /overlay.yaml")))
	})

	It("rejects unresolved template secrets", func() {
		b.Env.InstallConfigs = []string{"/overlay.yaml"}

		fw.EXPECT().Exists("/out/config.yaml").Return(false).Once()
		fw.EXPECT().ReadFile("/out/vault.yaml").Return(nil, os.ErrNotExist).Maybe()
		fw.EXPECT().ReadFile("/overlay.yaml").Return([]byte("dataCenter:\n  city: '{{ secret \"missing\" }}'\n"), nil).Once()
		Expect(b.EnsureInstallConfig()).To(MatchError(ContainSubstring("failed to render install config /overlay.yaml")))
	})

	It("rejects missing config inputs in the normal bootstrap validation", func() {
		b.Env.InstallConfigs = []string{"/missing.yaml"}

		fw.EXPECT().Exists("/missing.yaml").Return(false).Once()
		Expect(b.ValidateInput()).To(MatchError(ContainSubstring("/missing.yaml")))
	})

	It("rejects missing config inputs in single-VM validation", func() {
		b.Env.InstallLocal = "installer.tar.gz"

		fw.EXPECT().Exists("installer.tar.gz").Return(true).Once()

		b.Env.InstallConfigs = []string{"/missing.yaml"}

		fw.EXPECT().Exists("/missing.yaml").Return(false).Once()
		Expect(b.BootstrapSingleVM("registry", "local", false)).To(MatchError(ContainSubstring("/missing.yaml")))
	})
})

var _ = Describe("GCP machine type overrides", func() {
	It("keeps the role-specific multi-VM defaults when no override is supplied", func() {
		defs := gcp.VMDefsForEnv(&gcp.CodesphereEnvironment{})
		Expect(defs).To(ContainElement(And(HaveField("Name", "jumpbox"), HaveField("MachineType", "e2-medium"))))
		Expect(defs).To(ContainElement(And(HaveField("Name", "postgres"), HaveField("MachineType", "e2-standard-2"))))
		Expect(defs).To(ContainElement(And(HaveField("Name", "k0s-1"), HaveField("MachineType", "e2-standard-8"))))
	})

	It("overrides all multi-VM roles", func() {
		defs := gcp.VMDefsForEnv(&gcp.CodesphereEnvironment{MachineType: "n2-standard-8"})
		Expect(defs).To(HaveLen(8))
		Expect(defs).To(HaveEach(HaveField("MachineType", "n2-standard-8")))
	})

	It("overrides both data centers as well as the shared machines", func() {
		environment := &gcp.CodesphereEnvironment{MultiDC: true, MachineType: "n2-standard-8"}
		environment.DataCenters = gcp.BuildDataCenters(environment)
		defs := gcp.VMDefsForEnv(environment)
		Expect(defs).To(HaveLen(14))
		Expect(defs).To(HaveEach(HaveField("MachineType", "n2-standard-8")))
	})

	It("keeps the single-VM default without an override", func() {
		defs := gcp.VMDefsForEnv(&gcp.CodesphereEnvironment{SingleVM: true})
		Expect(defs).To(ConsistOf(HaveField("MachineType", "e2-standard-8")))
	})
})

var _ = Describe("Shared remote OMS installation", func() {
	var (
		b      *gcp.GCPBootstrapper
		client *node.MockNodeClient
		host   *node.Node
	)

	BeforeEach(func() {
		client = node.NewMockNodeClient(GinkgoT())
		host = &node.Node{NodeClient: client}

		var err error

		b, err = gcp.NewGCPBootstrapper(context.Background(), nil, bootstrap.NewStepLogger(true),
			&gcp.CodesphereEnvironment{SingleVM: true, Jumpbox: host}, nil, nil, nil, client, nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("reuses OMS on the single VM when no binary is supplied", func() {
		client.EXPECT().RunCommand(host, "root", "command -v oms >/dev/null 2>&1").Return(nil).Once()
		Expect(b.EnsureOmsInstalled()).To(Succeed())
	})

	It("downloads OMS on the single VM when the command is missing", func() {
		client.EXPECT().RunCommand(host, "root", "command -v oms >/dev/null 2>&1").Return(errors.New("not installed")).Once()
		client.EXPECT().RunCommand(host, "root", "curl -fsSL 'https://api.github.com/repos/codesphere-cloud/oms/releases/latest' | jq -r '.assets[] | select(.name | match(\"oms.*linux_amd64\")) | .browser_download_url' | xargs curl -fsSL -o oms").Return(nil).Once()
		client.EXPECT().RunCommand(host, "root", "chmod +x oms; sudo mv oms /usr/local/bin/").Return(nil).Once()
		Expect(b.EnsureOmsInstalled()).To(Succeed())
	})
})
