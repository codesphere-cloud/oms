// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"errors"
	"os"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"go.yaml.in/yaml/v3"
)

var _ = Describe("Single VM configuration", func() {
	var (
		b              *GCPBootstrapper
		manager        installer.InstallConfigManager
		client         *MockGCPClientManager
		nodeClient     *node.MockNodeClient
		fw             *util.MockFileIO
		n              *node.Node
		uploadedConfig files.RootConfig
		uploadedVault  files.InstallVault
	)

	BeforeEach(func() {
		var err error

		manager, err = installer.NewInstallConfigManager("plain", "")
		Expect(err).NotTo(HaveOccurred())

		client = NewMockGCPClientManager(GinkgoT())
		nodeClient = node.NewMockNodeClient(GinkgoT())
		fw = util.NewMockFileIO(GinkgoT())
		n = &node.Node{NodeClient: nodeClient, ExternalIP: "192.0.2.13"}
		b = &GCPBootstrapper{
			Env:       &CodesphereEnvironment{BaseDomain: "example.com", ProjectID: "project", DNSProjectID: "dns-project", OmsWorkdir: "/work", Jumpbox: n, GoogleACMEIssuer: true},
			GCPClient: client, icg: manager, fw: fw,
		}
		uploadedConfig = files.NewRootConfig()
		uploadedVault = files.InstallVault{}
	})

	expectConfigUpload := func() {
		GinkgoHelper()
		fw.EXPECT().MkdirAll("/work", os.FileMode(0755)).Return(nil).Once()
		client.EXPECT().CreatePublicCAExternalAccountKey("project").Return("default-id", "default-mac", nil).Once()
		client.EXPECT().CreatePublicCAExternalAccountKey("project").Return("custom-id", "custom-mac", nil).Once()
		fw.EXPECT().WriteFile("/work/single-vm-config.yaml", mock.Anything, os.FileMode(0600)).Run(func(_ string, data []byte, _ os.FileMode) {
			Expect(uploadedConfig.Unmarshal(data)).To(Succeed())
		}).Return(nil).Once()
		fw.EXPECT().WriteFile("/work/single-vm-vault.yaml", mock.Anything, os.FileMode(0600)).Run(func(_ string, data []byte, _ os.FileMode) {
			Expect(yaml.Unmarshal(data, &uploadedVault)).To(Succeed())
		}).Return(nil).Once()
		nodeClient.EXPECT().CopyFile(n, "/work/single-vm-config.yaml", singleVMConfigPath).Return(nil).Once()
		nodeClient.EXPECT().CopyFile(n, "/work/single-vm-vault.yaml", singleVMVaultPath).Return(nil).Once()
	}

	expectVaultRecovery := func(data []byte) {
		GinkgoHelper()
		nodeClient.EXPECT().RunCommand(n, "root", "test ! -e '/root/codesphere/prod.vault.yaml'").Return(errors.New("file exists")).Once()
		fw.EXPECT().CreateTemp("/work", ".single-vm-vault-*").Return("/work/recovered-vault", nil).Once()
		nodeClient.EXPECT().DownloadFile(n, singleVMVaultPath, "/work/recovered-vault").Return(nil).Once()
		fw.EXPECT().ReadFile("/work/recovered-vault").Return(data, nil).Once()
		fw.EXPECT().Remove("/work/recovered-vault").Return(nil).Once()
	}

	It("provides Google issuer credentials and DNS settings that survive the local profile", func() {
		nodeClient.EXPECT().RunCommand(n, "root", "test ! -e '/root/codesphere/prod.vault.yaml'").Return(nil).Once()
		expectConfigUpload()
		Expect(b.provideSingleVMConfig()).To(Succeed())
		manager.SetInstallConfig(&uploadedConfig)
		Expect(manager.ApplyProfile(installer.PROFILE_DEV)).To(Succeed())

		issuer := uploadedConfig.Codesphere.CertIssuer
		Expect(issuer).NotTo(BeNil())
		Expect(issuer.Type).To(Equal(files.CertIssuerTypeACME))
		Expect(issuer.Acme.Server).To(Equal("https://dv.acme-v02.api.pki.goog/directory"))
		Expect(issuer.Acme.EABKeyID).To(Equal("default-id"))
		Expect(issuer.Acme.CustomDomainsEABKeyID).To(Equal("custom-id"))

		issuers := uploadedConfig.Cluster.Certificates.Override["issuers"].(map[string]any)
		Expect(issuers["letsEncryptHttp"]).To(HaveKeyWithValue("enabled", false))
		solver := issuers["acme"].(map[string]any)["dnsSolver"].(map[string]any)["config"].(map[string]any)
		Expect(solver["cloudDNS"]).To(HaveKeyWithValue("project", "dns-project"))
		Expect(uploadedVault.GetSecret(files.SecretAcmeEabMacKey).Fields.Password).To(Equal("default-mac"))
		Expect(uploadedVault.GetSecret(files.SecretAcmeCustomDomainsEabMacKey).Fields.Password).To(Equal("custom-mac"))
	})

	It("preserves generated encryption and signing keys when rerun against an existing VM", func() {
		existing := &files.InstallVault{}
		existing.SetSecret(files.SecretEntry{Name: files.SecretMongoDbPasswordEncryptionKey, Fields: &files.SecretFields{Password: "existing-encryption-key"}})
		existing.SetSecret(files.SecretEntry{Name: files.SecretTokenPrivateKey, File: &files.SecretFile{Content: "existing-signing-key"}})
		data, err := existing.Marshal()
		Expect(err).NotTo(HaveOccurred())
		expectVaultRecovery(data)
		expectConfigUpload()
		Expect(b.provideSingleVMConfig()).To(Succeed())
		Expect(uploadedVault.GetSecret(files.SecretMongoDbPasswordEncryptionKey).Fields.Password).To(Equal("existing-encryption-key"))
		Expect(uploadedVault.GetSecret(files.SecretTokenPrivateKey).File.Content).To(Equal("existing-signing-key"))
		Expect(uploadedVault.GetSecret(files.SecretAcmeEabMacKey).Fields.Password).To(Equal("default-mac"))
	})

	It("keeps active secrets over seed values while retaining new seed entries", func() {
		manager.GetVault().SetSecret(files.SecretEntry{Name: "existing", Fields: &files.SecretFields{Password: "seed"}})
		manager.GetVault().SetSecret(files.SecretEntry{Name: "new", Fields: &files.SecretFields{Password: "new-seed"}})
		expectVaultRecovery([]byte("secrets:\n- name: existing\n  fields:\n    password: active\n"))
		Expect(b.loadSingleVMVault()).To(Succeed())
		Expect(manager.GetVault().GetSecret("existing").Fields.Password).To(Equal("active"))
		Expect(manager.GetVault().GetSecret("new").Fields.Password).To(Equal("new-seed"))
	})

	It("loads the supplied seed vault through the shared loader before recovering active VM secrets", func() {
		b.Env.InstallVaultPath = "/seed.yaml"
		b.Env.InstallVaultPrivKey = "/age-key"
		loader := NewMockInstallVaultLoader(GinkgoT())
		b.InstallVaultLoader = loader
		seed := &files.InstallVault{}
		seed.SetSecret(files.SecretEntry{Name: "existing", Fields: &files.SecretFields{Password: "seed"}})
		seed.SetSecret(files.SecretEntry{Name: "new", Fields: &files.SecretFields{Password: "new-seed"}})
		loader.EXPECT().Load("/seed.yaml", "/age-key").Return(seed, nil).Once()
		expectVaultRecovery([]byte("secrets:\n- name: existing\n  fields:\n    password: active\n"))
		Expect(b.loadSingleVMVault()).To(Succeed())
		Expect(manager.GetVault().GetSecret("existing").Fields.Password).To(Equal("active"))
		Expect(manager.GetVault().GetSecret("new").Fields.Password).To(Equal("new-seed"))
	})

	It("stops before uploading a vault when recovery fails", func() {
		fw.EXPECT().MkdirAll("/work", os.FileMode(0755)).Return(nil).Once()
		nodeClient.EXPECT().RunCommand(n, "root", "test ! -e '/root/codesphere/prod.vault.yaml'").Return(errors.New("SSH disconnected")).Once()
		fw.EXPECT().CreateTemp("/work", ".single-vm-vault-*").Return("/work/recovered-vault", nil).Once()
		nodeClient.EXPECT().DownloadFile(n, singleVMVaultPath, "/work/recovered-vault").Return(errors.New("SSH disconnected")).Once()
		fw.EXPECT().Remove("/work/recovered-vault").Return(nil).Once()
		Expect(b.provideSingleVMConfig()).To(MatchError(ContainSubstring("failed to recover single VM vault")))
	})

	It("rejects a corrupt recovered vault instead of replacing it", func() {
		expectVaultRecovery([]byte("secrets: ["))
		Expect(b.loadSingleVMVault()).To(MatchError(ContainSubstring("failed to parse recovered single VM vault")))
	})

	It("uses an uploaded template as the base for VM settings", func() {
		templatePath := "/config/base.yaml"
		b.Env.InstallConfigTemplatePath = templatePath
		fw.EXPECT().ReadFile(templatePath).Return([]byte("dataCenter:\n  name: custom\npcApps:\n  applications:\n    ssh-workspace-proxy:\n      valuesObject:\n        service:\n          port: 2222\n"), nil).Once()
		nodeClient.EXPECT().CopyFile(n, templatePath, singleVMConfigTemplatePath).Return(nil).Once()
		nodeClient.EXPECT().RunCommand(n, "root", "test ! -e '/root/codesphere/prod.vault.yaml'").Return(nil).Once()
		expectConfigUpload()
		Expect(b.provideSingleVMConfig()).To(Succeed())
		Expect(uploadedConfig.Datacenter.Name).To(Equal("custom"))
		Expect(uploadedConfig.Codesphere.Domain).To(Equal("cs.example.com"))
		Expect(uploadedConfig.Codesphere.PublicIP).To(Equal("192.0.2.13"))
		service := uploadedConfig.PcApps["applications"].(map[string]any)["ssh-workspace-proxy"].(map[string]any)["valuesObject"].(map[string]any)["service"]
		Expect(service).To(HaveKeyWithValue("port", 2222))
		Expect(service).To(HaveKeyWithValue("type", "ClusterIP"))
	})

	It("merges repeated config files in order and renders their secrets", func() {
		b.Env.InstallConfigs = []string{"/config/first.yaml", "/config/second.yaml"}

		manager.GetVault().SetSecret(files.SecretEntry{Name: "dcCity", File: &files.SecretFile{Content: "Berlin"}})
		fw.EXPECT().ReadFile("/config/first.yaml").Return([]byte("dataCenter:\n  name: first\n  city: '{{ secret \"dcCity\" }}'\n"), nil).Once()
		fw.EXPECT().ReadFile("/config/second.yaml").Return([]byte("dataCenter:\n  name: second\n"), nil).Once()
		nodeClient.EXPECT().CopyFile(n, "/config/first.yaml", "/root/codesphere/config-input-0.yaml").Return(nil).Once()
		nodeClient.EXPECT().CopyFile(n, "/config/second.yaml", "/root/codesphere/config-input-1.yaml").Return(nil).Once()

		config, err := b.loadSingleVMBaseConfig()
		Expect(err).NotTo(HaveOccurred())
		Expect(config.Datacenter.Name).To(Equal("second"))
		Expect(config.Datacenter.City).To(Equal("Berlin"))
	})

	It("preserves single VM addresses and internal services through the local profile", func() {
		config := b.singleVMConfig(files.NewRootConfig())
		data, err := config.Marshal()
		Expect(err).NotTo(HaveOccurred())

		config = files.NewRootConfig()
		Expect(config.Unmarshal(data)).To(Succeed())
		manager.SetInstallConfig(&config)
		Expect(manager.ApplyProfile(installer.PROFILE_DEV)).To(Succeed())
		Expect(config.Codesphere.Domain).To(Equal("cs.example.com"))
		Expect(config.Codesphere.WorkspaceHostingBaseDomain).To(Equal("ws.example.com"))
		Expect(config.Codesphere.PublicIP).To(Equal("192.0.2.13"))
		Expect(config.Codesphere.CustomDomains.CNameBaseDomain).To(Equal("ws.example.com"))
		Expect(config.Cluster.Gateway.ServiceType).To(Equal("ClusterIP"))
		Expect(config.Cluster.PublicGateway.ServiceType).To(Equal("ClusterIP"))
		service := config.PcApps["applications"].(map[string]any)["ssh-workspace-proxy"].(map[string]any)["valuesObject"].(map[string]any)["service"]
		Expect(service).To(HaveKeyWithValue("type", "ClusterIP"))
	})

	It("uses the VM address for every public DNS target", func() {
		Expect(b.prepareSingleVMDataCenter()).To(Succeed())
		Expect(b.primaryDC().GatewayIP).To(Equal("192.0.2.13"))
		Expect(b.primaryDC().PublicGatewayIP).To(Equal("192.0.2.13"))
		Expect(b.primaryDC().SSHProxyIP).To(Equal("192.0.2.13"))
	})

	It("requires an installer source", func() {
		Expect(b.validateSingleVMPackageSource()).To(HaveOccurred())
	})

	It("accepts a local installer archive", func() {
		b.Env.InstallLocal = "installer-lite.tar.gz"
		Expect(b.validateSingleVMPackageSource()).To(Succeed())
	})

	It("accepts an installer version", func() {
		b.Env.InstallVersion = "v1.2.3"
		Expect(b.validateSingleVMPackageSource()).To(Succeed())
	})

	It("rejects unsupported installer archives", func() {
		b.Env.InstallLocal = "installer.zip"
		Expect(b.validateSingleVMPackageSource()).To(HaveOccurred())
	})

	It("downloads a versioned installer and removes stale extracted files", func() {
		b.Env.InstallVersion = "v1.2.3"
		b.Env.InstallHash = "abc123"

		nodeClient.EXPECT().RunCommand(n, "root", "cd /root && oms download package -f installer-lite.tar.gz -H 'abc123' 'v1.2.3' && mv -- 'v1.2.3-abc123-installer-lite.tar.gz' /root/installer-lite.tar.gz").Return(nil).Once()
		nodeClient.EXPECT().RunCommand(n, "root", "rm -rf -- /root/installer-lite").Return(nil).Once()
		Expect(b.ensureSingleVMPackage()).To(Succeed())
	})

	It("copies a local installer and removes stale extracted files", func() {
		b.Env.InstallLocal = "installer-lite.tar.gz"

		nodeClient.EXPECT().CopyFile(n, "installer-lite.tar.gz", singleVMPackagePath).Return(nil).Once()
		nodeClient.EXPECT().RunCommand(n, "root", "rm -rf -- /root/installer-lite").Return(nil).Once()
		Expect(b.ensureSingleVMPackage()).To(Succeed())
	})

	It("installs single-node k0s", func() {
		nodeClient.EXPECT().RunCommand(n, "root", "oms install k0s --single").Return(nil).Once()
		Expect(b.installSingleVMK0s()).To(Succeed())
	})

	It("forwards the selected storage engine", func() {
		nodeClient.EXPECT().RunCommand(n, "root", "test -n \"$OMS_REGISTRY_PASSWORD\"").Return(nil).Once()
		nodeClient.EXPECT().RunCommand(n, "root", mock.MatchedBy(func(command string) bool {
			return strings.Contains(command, "--storage-engine 'local'") && strings.Contains(command, "--ceph-device-filter '^sdb$'")
		})).Return(nil).Once()
		Expect(b.installSingleVMCodesphere("user", "local", false)).To(Succeed())
	})

	It("configures unprivileged gateway ports before installation", func() {
		nodeClient.EXPECT().RunCommand(n, "root", "sudo grep -E '^fs.inotify.max_user_watches=1048576' /etc/sysctl.conf >/dev/null 2>&1").Return(nil).Once()
		nodeClient.EXPECT().RunCommand(n, "root", "sudo sysctl -n fs.inotify.max_user_watches | grep -q '^1048576$'").Return(nil).Once()
		nodeClient.EXPECT().RunCommand(n, "root", "sudo grep -E '^fs.inotify.max_user_instances=8192' /etc/sysctl.conf >/dev/null 2>&1").Return(nil).Once()
		nodeClient.EXPECT().RunCommand(n, "root", "sudo sysctl -n fs.inotify.max_user_instances | grep -q '^8192$'").Return(nil).Once()
		nodeClient.EXPECT().RunCommand(n, "root", "sudo grep -E '^vm.max_map_count=262144' /etc/sysctl.conf >/dev/null 2>&1").Return(nil).Once()
		nodeClient.EXPECT().RunCommand(n, "root", "sudo sysctl -n vm.max_map_count | grep -q '^262144$'").Return(nil).Once()
		nodeClient.EXPECT().RunCommand(n, "root", "sudo grep -E '^net.ipv4.ip_unprivileged_port_start=80' /etc/sysctl.conf >/dev/null 2>&1").Return(errors.New("not configured")).Twice()
		nodeClient.EXPECT().RunCommand(n, "root", "echo 'net.ipv4.ip_unprivileged_port_start=80' | sudo tee -a /etc/sysctl.conf").Return(nil).Once()
		nodeClient.EXPECT().RunCommand(n, "root", "sudo sysctl -p").Return(nil).Once()
		nodeClient.EXPECT().RunCommand(n, "root", "mkdir -p /root/.kube /root/codesphere").Return(nil).Once()
		Expect(b.configureSingleVMHost()).To(Succeed())
	})
})
