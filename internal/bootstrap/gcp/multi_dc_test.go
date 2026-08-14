// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"context"
	"fmt"
	"os"
	"strings"

	"cloud.google.com/go/compute/apiv1/computepb"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"google.golang.org/api/cloudbilling/v1"

	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/github"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
)

// realConfigManager returns an install config manager that keeps its files in memory, so tests
// can inspect the config and vault a data center would actually be installed with.
func realConfigManager(writes map[string][]byte) installer.InstallConfigManager {
	icm := installer.NewInstallConfigManager()
	icm.(*installer.InstallConfig).SetFileIO(&recordingFileIO{writes: writes})

	return icm
}

// recordingFileIO records written files and reports every other path as missing, so a bootstrap
// run behaves as if nothing existed locally beforehand.
type recordingFileIO struct {
	util.FilesystemWriter
	writes map[string][]byte
}

func (f *recordingFileIO) CreateAndWrite(path string, content []byte, _ string) error {
	f.writes[path] = content
	return nil
}

var _ = Describe("Multi-DC bootstrap", func() {
	var (
		nodeClient *node.MockNodeClient
		csEnv      *gcp.CodesphereEnvironment
		gc         *gcp.MockGCPClientManager
		fw         *util.MockFileIO
		writes     map[string][]byte
		primaryICG installer.InstallConfigManager

		bs *gcp.GCPBootstrapper
	)

	BeforeEach(func() {
		nodeClient = node.NewMockNodeClient(GinkgoT())
		gc = gcp.NewMockGCPClientManager(GinkgoT())
		fw = util.NewMockFileIO(GinkgoT())
		writes = map[string][]byte{}
		primaryICG = realConfigManager(writes)

		csEnv = &gcp.CodesphereEnvironment{
			MultiDC:           true,
			ProjectName:       "test-project",
			BillingAccount:    "test-billing-account",
			BaseDomain:        "example.com",
			Region:            "us-central1",
			Zone:              "us-central1-a",
			DNSProjectID:      "dns-project",
			DNSZoneName:       "test-zone",
			ProjectTTL:        "1h",
			SecretsDir:        "/etc/codesphere/secrets",
			DatacenterName:    "multidc",
			InstallConfigPath: "config.yaml",
			SecretsFilePath:   "prod.vault.yaml",
			WriteConfig:       true,
			RegistryType:      gcp.RegistryTypeGitHub,
			RegistryUser:      "registry-user",
			GitHubPAT:         "fake-pat",
			SSHPublicKeyPath:  "key.pub",
			RootDiskSize:      50,
			InternalFlags:     gcp.DefaultInternalFlags,
			PreviewFlags:      gcp.DefaultPreviewFlags,
			FeatureFlags:      gcp.DefaultFeatureFlags,
		}
	})

	JustBeforeEach(func() {
		var err error

		bs, err = gcp.NewGCPBootstrapper(
			context.Background(),
			env.NewEnv(),
			bootstrap.NewStepLogger(false),
			csEnv,
			primaryICG,
			gc,
			fw,
			nodeClient,
			portal.NewMockPortal(GinkgoT()),
			util.NewFakeTime(),
			github.NewMockGitHubClient(GinkgoT()),
		)
		Expect(err).NotTo(HaveOccurred())

		bs.NewConfigManager = func() installer.InstallConfigManager { return realConfigManager(writes) }
	})

	// expectBootstrapMocks sets up the GCP, SSH and file mocks for a full two-data-center run.
	expectBootstrapMocks := func(projectID string) {
		const vmCount = 14

		fw.EXPECT().Exists(mock.Anything).Return(false)
		fw.EXPECT().MkdirAll(mock.Anything, os.FileMode(0755)).Return(nil)
		fw.EXPECT().WriteFile(mock.Anything, mock.Anything, mock.Anything).Return(nil)
		// The SSH keys are read once and handed to every VM.
		fw.EXPECT().ReadFile(mock.Anything).Return([]byte("ssh-rsa AAA..."), nil).Once()

		// EnsureProject only creates a project when the lookup fails with this exact message.
		gc.EXPECT().GetProjectByName(mock.Anything, "test-project").Return(nil, fmt.Errorf("project not found: test-project"))
		gc.EXPECT().CreateProjectID("test-project").Return(projectID)
		gc.EXPECT().CreateProject(mock.Anything, mock.Anything, "test-project", mock.Anything).Return(mock.Anything, nil)
		gc.EXPECT().GetBillingInfo(projectID).Return(&cloudbilling.ProjectBillingInfo{BillingEnabled: false}, nil)
		gc.EXPECT().EnableBilling(projectID, "test-billing-account").Return(nil)
		gc.EXPECT().EnableAPIs(projectID, mock.Anything).Return(nil)
		gc.EXPECT().CreateServiceAccount(projectID, "cloud-controller", "cloud-controller").Return("cc@p.iam.gserviceaccount.com", false, nil)
		gc.EXPECT().AssignIAMRole(projectID, "cloud-controller", projectID, []string{"roles/compute.admin"}).Return(nil)
		gc.EXPECT().AssignIAMRole("dns-project", "cloud-controller", projectID, []string{"roles/dns.admin"}).Return(nil)
		gc.EXPECT().CreateVPC(projectID, "us-central1", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gc.EXPECT().CreateFirewallRule(projectID, mock.Anything).Return(nil).Times(5)

		mockGetInstanceNotFoundThenRunning(gc, projectID, "us-central1-a", makeRunningInstance("10.10.0.2", "1.2.3.4"), vmCount)
		gc.EXPECT().CreateInstance(projectID, "us-central1-a", mock.Anything).Return(nil).Times(vmCount)

		// Two data centers reserve three addresses each, suffixed for the second one.
		for _, name := range []string{
			"gateway", "public-gateway", "ssh-proxy",
			"gateway-dc2", "public-gateway-dc2", "ssh-proxy-dc2",
		} {
			ip := fmt.Sprintf("203.0.113.%d", len(name))
			gc.EXPECT().GetAddress(projectID, "us-central1", name).Return(nil, fmt.Errorf("not found")).Once()
			gc.EXPECT().CreateAddress(projectID, "us-central1", mock.MatchedBy(func(addr *computepb.Address) bool {
				return addr.GetName() == name
			})).Return(ip, nil).Once()
		}

		gc.EXPECT().EnsureDNSManagedZone("dns-project", "test-zone", "example.com.", mock.Anything).Return(nil)
		gc.EXPECT().EnsureDNSRecordSets("dns-project", "test-zone", mock.Anything).Return(nil)

		nodeClient.EXPECT().WaitReady(mock.Anything, mock.Anything).Return(nil)
		nodeClient.EXPECT().HasFile(mock.Anything, mock.Anything).Return(false)
		nodeClient.EXPECT().RunCommand(mock.Anything, mock.Anything, mock.Anything).Return(nil)
		nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil)
	}

	It("bootstraps two data centers sharing one database", func() {
		expectBootstrapMocks("test-project-12345")

		Expect(bs.Bootstrap()).To(Succeed())

		Expect(bs.Env.DataCenters).To(HaveLen(2))
		primary, secondary := bs.Env.DataCenters[0], bs.Env.DataCenters[1]

		By("giving each data center its own ceph and k0s nodes")
		Expect(vmNamesOfNodes(primary.CephNodes)).To(Equal([]string{"ceph-1", "ceph-2", "ceph-3"}))
		Expect(vmNamesOfNodes(primary.ControlPlaneNodes)).To(Equal([]string{"k0s-1", "k0s-2", "k0s-3"}))
		Expect(vmNamesOfNodes(secondary.CephNodes)).To(Equal([]string{"ceph-1-dc2", "ceph-2-dc2", "ceph-3-dc2"}))
		Expect(vmNamesOfNodes(secondary.ControlPlaneNodes)).To(Equal([]string{"k0s-1-dc2", "k0s-2-dc2", "k0s-3-dc2"}))

		By("sharing the platform domain and scoping the workspace domain")
		Expect(primary.InstallConfig.Codesphere.Domain).To(Equal("cs.example.com"))
		Expect(secondary.InstallConfig.Codesphere.Domain).To(Equal("cs.example.com"))
		Expect(primary.InstallConfig.Codesphere.WorkspaceHostingBaseDomain).To(Equal("1.ws.example.com"))
		Expect(secondary.InstallConfig.Codesphere.WorkspaceHostingBaseDomain).To(Equal("2.ws.example.com"))
		Expect(secondary.InstallConfig.Codesphere.CustomDomains.CNameBaseDomain).To(Equal("2.ws.example.com"))

		By("giving each data center a distinct ID and name")
		Expect(primary.InstallConfig.Datacenter.ID).To(Equal(1))
		Expect(secondary.InstallConfig.Datacenter.ID).To(Equal(2))
		// The k0s cluster is named codesphere-<datacenter name>, so the names must differ.
		Expect(primary.InstallConfig.Datacenter.Name).To(Equal("multidc"))
		Expect(secondary.InstallConfig.Datacenter.Name).To(Equal("multidc-dc2"))

		By("installing postgres in the primary data center and reusing it in the secondary")
		Expect(primary.InstallConfig.Postgres.Mode).To(Equal("install"))
		Expect(primary.InstallConfig.Postgres.Primary.IP).To(Equal("10.10.0.2"))
		Expect(secondary.InstallConfig.Postgres.Mode).To(Equal("external"))
		// The server certificate carries only an IP SAN, so the address must be the IP.
		Expect(secondary.InstallConfig.Postgres.ServerAddress).To(Equal("10.10.0.2"))
		Expect(secondary.InstallConfig.Postgres.Primary).To(BeNil())
		Expect(secondary.InstallConfig.Postgres.Replica).To(BeNil())
		Expect(secondary.InstallConfig.Postgres.CACertPem).To(Equal(primary.InstallConfig.Postgres.CACertPem))
		Expect(secondary.InstallConfig.Postgres.CACertPem).NotTo(BeEmpty())

		By("skipping the postgres install step in the secondary data center only")
		Expect(secondary.InstallConfig.Operations.Skip).To(ContainElement("postgres"))
		// Both data centers install their own ceph and kubernetes.
		Expect(secondary.InstallConfig.Operations.Skip).NotTo(ContainElement("ceph"))
		Expect(secondary.InstallConfig.Operations.Skip).NotTo(ContainElement("kubernetes"))

		if primary.InstallConfig.Operations != nil {
			Expect(primary.InstallConfig.Operations.Skip).NotTo(ContainElement("postgres"))
		}

		By("telling both data centers about every data center of the installation")
		// The installer defaults dataCenters to the local one, so without the list each data
		// center renders as a single-data-center instance.
		topology := []files.DatacenterConfig{
			{ID: 1, Name: "multidc", City: "Karlsruhe", CountryCode: "DE"},
			{ID: 2, Name: "multidc-dc2", City: "Karlsruhe", CountryCode: "DE"},
		}
		for _, dc := range bs.Env.DataCenters {
			Expect(dc.InstallConfig.DataCenters).To(Equal(topology), "data center %d", dc.ID)
			Expect(dc.InstallConfig.DefaultDataCenterID).To(Equal(1), "data center %d", dc.ID)
			// The local data center is still the one this config installs.
			Expect(dc.InstallConfig.Datacenter.ID).To(Equal(dc.ID))
		}

		By("giving each data center its own vault directory on the shared jumpbox")
		Expect(primary.InstallConfig.Secrets.BaseDir).To(Equal("/etc/codesphere/secrets"))
		Expect(secondary.InstallConfig.Secrets.BaseDir).To(Equal("/etc/codesphere/secrets-dc2"))
		Expect(writes).To(HaveKey("config.yaml"))
		Expect(writes).To(HaveKey("config-dc2.yaml"))
		Expect(writes).To(HaveKey("prod.vault.yaml"))
		Expect(writes).To(HaveKey("prod-dc2.vault.yaml"))

		primaryVault := primary.ConfigManager.GetVault()
		secondaryVault := secondary.ConfigManager.GetVault()

		By("sharing every secret both data centers need to agree on")

		shared := []string{
			files.SecretPostgresPassword,
			files.SecretPostgresReplicaPassword,
			files.SecretPostgresCaKeyPem,
			files.SecretTokenPrivateKey,
			files.SecretTokenPublicKey,
			files.SecretDomainAuthPrivateKey,
			files.SecretMounterHmacSecret,
			files.SecretMongoDbPasswordEncryptionKey,
		}
		for _, svc := range codesphere.PostgresServices {
			shared = append(shared, files.PostgresUserSecretName(svc.Name), files.PostgresPasswordSecretName(svc.Name))
		}

		for _, name := range shared {
			Expect(primaryVault.GetSecret(name)).NotTo(BeNil(), "primary should have %s", name)
			Expect(secondaryVault.GetSecret(name)).To(Equal(primaryVault.GetSecret(name)), "%s must be shared", name)
		}

		By("regenerating the per-cluster secrets for the secondary data center")

		for _, name := range []string{files.SecretSelfSignedCaKeyPem, files.SecretCephSshPrivateKey} {
			Expect(secondaryVault.GetSecret(name)).NotTo(BeNil(), "secondary should have %s", name)
			Expect(secondaryVault.GetSecret(name).File.Content).
				NotTo(Equal(primaryVault.GetSecret(name).File.Content), "%s must be per data center", name)
		}

		Expect(secondary.InstallConfig.Cluster.Certificates.CA.CertPem).
			NotTo(Equal(primary.InstallConfig.Cluster.Certificates.CA.CertPem))
		Expect(secondary.InstallConfig.Ceph.CephAdmSSHKey.PublicKey).
			NotTo(Equal(primary.InstallConfig.Ceph.CephAdmSSHKey.PublicKey))

		By("recording the DNS records it created so cleanup can delete them")
		Expect(bs.Env.DNSRecords).To(Equal(gcp.DataCenterDNSRecordNames("example.com", bs.Env.DataCenters)))

		By("pointing each data center's install command at its own config and vault")
		Expect(bs.InstallCommand(primary, "pkg.tar.gz")).To(ContainSubstring("-c /etc/codesphere/config.yaml"))
		Expect(bs.InstallCommand(primary, "pkg.tar.gz")).To(ContainSubstring("--vault /etc/codesphere/secrets/prod.vault.yaml"))
		Expect(bs.InstallCommand(secondary, "pkg.tar.gz")).To(ContainSubstring("-c /etc/codesphere/config-dc2.yaml"))
		Expect(bs.InstallCommand(secondary, "pkg.tar.gz")).To(ContainSubstring("--vault /etc/codesphere/secrets-dc2/prod.vault.yaml"))

		By("producing postgres blocks the installer's validation accepts")

		for _, dc := range bs.Env.DataCenters {
			for _, problem := range dc.ConfigManager.ValidateInstallConfig() {
				Expect(problem).NotTo(ContainSubstring("postgres"), "data center %d", dc.ID)
			}
		}
	})

	It("installs a separate k0s cluster per data center", func() {
		csEnv.Jumpbox = &node.Node{NodeClient: nodeClient, FileIO: fw}
		for _, dc := range []struct{ config, secretsDir string }{
			{"/etc/codesphere/config.yaml", "/etc/codesphere/secrets"},
			{"/etc/codesphere/config-dc2.yaml", "/etc/codesphere/secrets-dc2"},
		} {
			nodeClient.EXPECT().RunCommand(csEnv.Jumpbox, "root", mock.MatchedBy(func(command string) bool {
				return strings.HasPrefix(command, "oms install k0s ") &&
					strings.Contains(command, "--install-config "+dc.config+" ") &&
					strings.Contains(command, "--vault "+dc.secretsDir+"/prod.vault.yaml ") &&
					strings.Contains(command, "--vault-priv-key "+dc.secretsDir+"/age_key.txt")
			})).Return(nil).Once()
		}

		Expect(bs.InstallK0s()).To(Succeed())
	})

	Describe("validateMultiDC", func() {
		DescribeTable("rejects flag combinations it cannot satisfy",
			func(mutate func(), wantErr string) {
				mutate()
				Expect(bs.ValidateInput()).To(MatchError(ContainSubstring(wantErr)))
			},
			Entry("without write-config", func() { csEnv.WriteConfig = false }, "multi-dc requires write-config"),
			Entry("with an explicit datacenter ID", func() { csEnv.DatacenterIDExplicit = true }, "datacenter-id cannot be combined with multi-dc"),
			Entry("without a datacenter name", func() { csEnv.DatacenterName = "" }, "datacenter-name is required with multi-dc"),
			Entry("without a config path", func() { csEnv.InstallConfigPath = "" }, "cannot derive a per-data-center path"),
			Entry("without a secrets path", func() { csEnv.SecretsFilePath = "" }, "cannot derive a per-data-center path"),
		)

		It("accepts the default multi-dc flags", func() {
			Expect(bs.ValidateInput()).To(Succeed())
		})

		It("does not constrain a single-data-center bootstrap", func() {
			csEnv.MultiDC = false
			csEnv.WriteConfig = false
			csEnv.DatacenterName = ""
			csEnv.DatacenterIDExplicit = true

			Expect(bs.ValidateInput()).To(Succeed())
		})
	})
})

// vmNamesOfNodes returns the names of the given nodes, in order.
func vmNamesOfNodes(nodes []*node.Node) []string {
	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.GetName()
	}

	return names
}
