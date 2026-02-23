// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"context"
	"fmt"
	"strings"

	"os"

	"cloud.google.com/go/artifactregistry/apiv1/artifactregistrypb"
	"cloud.google.com/go/compute/apiv1/computepb"
	"cloud.google.com/go/resourcemanager/apiv3/resourcemanagerpb"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"google.golang.org/api/cloudbilling/v1"
	"google.golang.org/api/dns/v1"
)

func protoString(s string) *string { return &s }

func jumpbboxMatcher(node *node.Node) bool {
	return node.GetName() == "jumpbox"
}

var _ = Describe("GCP Bootstrapper", func() {
	var (
		nodeClient *node.MockNodeClient
		csEnv      *gcp.CodesphereEnvironment
		ctx        context.Context
		e          env.Env

		icg              *installer.MockInstallConfigManager
		gc               *gcp.MockGCPClientManager
		fw               *util.MockFileIO
		stlog            *bootstrap.StepLogger
		mockPortalClient *portal.MockPortal

		bs *gcp.GCPBootstrapper
	)

	JustBeforeEach(func() {
		var err error
		bs, err = gcp.NewGCPBootstrapper(ctx, e, stlog, csEnv, icg, gc, fw, nodeClient, mockPortalClient)
		Expect(err).NotTo(HaveOccurred())
	})

	BeforeEach(func() {
		nodeClient = node.NewMockNodeClient(GinkgoT())
		ctx = context.Background()
		e = env.NewEnv()
		icg = installer.NewMockInstallConfigManager(GinkgoT())
		gc = gcp.NewMockGCPClientManager(GinkgoT())
		fw = util.NewMockFileIO(GinkgoT())
		mockPortalClient = portal.NewMockPortal(GinkgoT())
		stlog = bootstrap.NewStepLogger(false)

		csEnv = &gcp.CodesphereEnvironment{
			GitHubAppName:         "fake-app",
			GithubAppClientID:     "fake-client-id",
			GithubAppClientSecret: "fake-secret",
			InstallConfigPath:     "fake-config-file",
			SecretsFilePath:       "fake-secret",
			ProjectName:           "test-project",
			SecretsDir:            "/etc/codesphere/secrets",
			BillingAccount:        "test-billing-account",
			Region:                "us-central1",
			Zone:                  "us-central1-a",
			DatacenterID:          1,
			BaseDomain:            "example.com",
			DNSProjectID:          "dns-project",
			DNSZoneName:           "test-zone",
			SSHPublicKeyPath:      "key.pub",
			ProjectID:             "pid",
			Experiments:           gcp.DefaultExperiments,
			FeatureFlags:          []string{},
			InstallConfig: &files.RootConfig{
				Registry: &files.RegistryConfig{},
				Postgres: files.PostgresConfig{
					Primary: &files.PostgresPrimaryConfig{},
				},
				Cluster: files.ClusterConfig{},
			},
			Jumpbox:           fakeNode("jumpbox", nodeClient),
			PostgreSQLNode:    fakeNode("postgres", nodeClient),
			ControlPlaneNodes: []*node.Node{fakeNode("k0s-1", nodeClient), fakeNode("k0s-2", nodeClient), fakeNode("k0s-3", nodeClient)},
			CephNodes:         []*node.Node{fakeNode("ceph-1", nodeClient), fakeNode("ceph-2", nodeClient), fakeNode("ceph-3", nodeClient), fakeNode("ceph-4", nodeClient)},
		}
	})
	Describe("NewGCPBootstrapper", func() {
		It("creates a valid GCPBootstrapper", func() {
			csEnv = &gcp.CodesphereEnvironment{}

			Expect(bs).NotTo(BeNil())
		})
	})

	Describe("Bootstrap", func() {
		BeforeEach(func() {
			csEnv.InstallConfig = &files.RootConfig{Registry: &files.RegistryConfig{}}
			csEnv.ControlPlaneNodes = []*node.Node{}
			csEnv.CephNodes = []*node.Node{}
			csEnv.PostgreSQLNode = nil
			csEnv.Jumpbox = nil
			csEnv.ProjectID = ""
		})

		It("runs bootstrap successfully", func() {
			bs.Env.RegistryType = gcp.RegistryTypeArtifactRegistry
			bs.Env.WriteConfig = true

			// 1. EnsureInstallConfig
			fw.EXPECT().Exists("fake-config-file").Return(false)
			icg.EXPECT().ApplyProfile("dev").Return(nil)
			// Returning a real install config to avoid nil pointer dereferences later
			icg.EXPECT().GetInstallConfig().RunAndReturn(func() *files.RootConfig {
				realIcm := installer.NewInstallConfigManager()
				_ = realIcm.ApplyProfile("dev")
				return realIcm.GetInstallConfig()
			})

			projectId := "test-project-12345"

			// 2. EnsureSecrets
			fw.EXPECT().Exists("fake-secret").Return(false)
			icg.EXPECT().GetVault().Return(&files.InstallVault{})

			// 3. EnsureProject
			gc.EXPECT().GetProjectByName(mock.Anything, "test-project").Return(nil, fmt.Errorf("project not found: test-project"))
			gc.EXPECT().CreateProjectID("test-project").Return(projectId)
			gc.EXPECT().CreateProject(mock.Anything, mock.Anything, "test-project").Return(mock.Anything, nil)

			// 4. EnsureBilling
			gc.EXPECT().GetBillingInfo(projectId).Return(&cloudbilling.ProjectBillingInfo{BillingEnabled: false}, nil)
			gc.EXPECT().EnableBilling(projectId, "test-billing-account").Return(nil)

			// 5. EnsureAPIsEnabled
			gc.EXPECT().EnableAPIs(projectId, mock.Anything).Return(nil)

			// 6. EnsureArtifactRegistry
			gc.EXPECT().GetArtifactRegistry(projectId, "us-central1", "codesphere-registry").Return(nil, fmt.Errorf("not found"))
			gc.EXPECT().CreateArtifactRegistry(projectId, "us-central1", "codesphere-registry").Return(&artifactregistrypb.Repository{Name: "codesphere-registry"}, nil)

			// 7. EnsureServiceAccounts
			gc.EXPECT().CreateServiceAccount(projectId, "cloud-controller", "cloud-controller").Return("cloud-controller@p.iam.gserviceaccount.com", false, nil)
			gc.EXPECT().CreateServiceAccount(projectId, "artifact-registry-writer", "artifact-registry-writer").Return("writer@p.iam.gserviceaccount.com", true, nil)
			gc.EXPECT().CreateServiceAccountKey(projectId, "writer@p.iam.gserviceaccount.com").Return("fake-key", nil)

			// 8. EnsureIAMRoles
			gc.EXPECT().AssignIAMRole(projectId, "artifact-registry-writer", projectId, []string{"roles/artifactregistry.writer"}).Return(nil)
			gc.EXPECT().AssignIAMRole(projectId, "cloud-controller", projectId, []string{"roles/compute.admin"}).Return(nil)
			gc.EXPECT().AssignIAMRole(csEnv.DNSProjectID, "cloud-controller", projectId, []string{"roles/dns.admin"}).Return(nil)

			// 9. EnsureVPC
			gc.EXPECT().CreateVPC(projectId, "us-central1", projectId+"-vpc", projectId+"-us-central1-subnet", projectId+"-router", projectId+"-nat-gateway").Return(nil)

			// 10. EnsureFirewallRules (5 times)
			gc.EXPECT().CreateFirewallRule(projectId, mock.Anything).Return(nil).Times(5)

			// 11. EnsureComputeInstances
			gc.EXPECT().CreateInstance(projectId, "us-central1-a", mock.Anything).Return(nil).Times(9)
			// GetInstance calls to retrieve IPs
			ipResp := &computepb.Instance{
				NetworkInterfaces: []*computepb.NetworkInterface{
					{
						NetworkIP: protoString("10.0.0.1"),
						AccessConfigs: []*computepb.AccessConfig{
							{NatIP: protoString("1.2.3.4")},
						},
					},
				},
			}

			gc.EXPECT().GetInstance(projectId, "us-central1-a", mock.Anything).Return(ipResp, nil).Times(9)
			fw.EXPECT().ReadFile(mock.Anything).Return([]byte("fake-key"), nil).Times(9)

			// 12. EnsureGatewayIPAddresses
			gc.EXPECT().GetAddress(projectId, "us-central1", "gateway").Return(nil, fmt.Errorf("not found"))
			gc.EXPECT().CreateAddress(projectId, "us-central1", mock.MatchedBy(func(addr *computepb.Address) bool { return *addr.Name == "gateway" })).Return("1.1.1.1", nil)
			gc.EXPECT().GetAddress(projectId, "us-central1", "gateway").Return(nil, fmt.Errorf("not found"))
			gc.EXPECT().GetAddress(projectId, "us-central1", "public-gateway").Return(nil, fmt.Errorf("not found"))
			gc.EXPECT().CreateAddress(projectId, "us-central1", mock.MatchedBy(func(addr *computepb.Address) bool { return *addr.Name == "public-gateway" })).Return("2.2.2.2", nil)
			gc.EXPECT().GetAddress(projectId, "us-central1", "public-gateway").Return(&computepb.Address{Address: protoString("2.2.2.2")}, nil)

			// 16. UpdateInstallConfig
			icg.EXPECT().GenerateSecrets().Return(nil)
			icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
			nodeClient.EXPECT().CopyFile(mock.Anything, "fake-config-file", "/etc/codesphere/config.yaml").Return(nil)
			nodeClient.EXPECT().CopyFile(mock.Anything, "fake-secret", "/etc/codesphere/secrets/prod.vault.yaml").Return(nil)
			icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

			// Enable Root Login
			nodeClient.EXPECT().WaitReady(mock.Anything, mock.Anything).Return(nil).Return(nil)
			nodeClient.EXPECT().RunCommand(mock.Anything, mock.Anything, mock.Anything).Return(nil)

			// 17. EnsureAgeKey
			nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "mkdir -p /etc/codesphere/secrets; age-keygen -o /etc/codesphere/secrets/age_key.txt").Return(nil)
			nodeClient.EXPECT().HasFile(mock.MatchedBy(jumpbboxMatcher), "/etc/codesphere/secrets/age_key.txt").Return(false)

			// 18. EncryptVault
			nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "cp /etc/codesphere/secrets/prod.vault.yaml{,.bak}").Return(nil)
			nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "sops --encrypt")
			})).Return(nil)

			// 19. EnsureDNSRecords
			gc.EXPECT().EnsureDNSManagedZone(csEnv.DNSProjectID, "test-zone", "example.com.", mock.Anything).Return(nil)
			gc.EXPECT().EnsureDNSRecordSets(csEnv.DNSProjectID, "test-zone", mock.MatchedBy(func(records []*dns.ResourceRecordSet) bool {
				return len(records) == 4
			})).Return(nil)

			// 20. GenerateK0sConfigScript
			fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(nil)
			nodeClient.EXPECT().CopyFile(mock.Anything, "configure-k0s.sh", "/root/configure-k0s.sh").Return(nil)
			nodeClient.EXPECT().RunCommand(mock.Anything, "root", "chmod +x /root/configure-k0s.sh").Return(nil)

			err := bs.Bootstrap()
			Expect(err).NotTo(HaveOccurred())
			Expect(bs.Env).NotTo(BeNil())
			Expect(bs.Env.ProjectID).To(HavePrefix("test-project-"))

			// Verify nodes are properly set in the environment
			Expect(bs.Env.Jumpbox).NotTo(BeNil(), "Jumpbox should be created")
			Expect(bs.Env.PostgreSQLNode).NotTo(BeNil(), "PostgreSQL node should be created")
			Expect(bs.Env.CephNodes).To(HaveLen(4), "Should have 4 Ceph nodes")
			Expect(bs.Env.ControlPlaneNodes).To(HaveLen(3), "Should have 3 K0s control plane nodes")

			// Verify mock returns expected values
			Expect(bs.Env.Jumpbox.GetName()).To(Equal("jumpbox"))
			Expect(bs.Env.Jumpbox.GetExternalIP()).To(Equal("1.2.3.4"))
			Expect(bs.Env.Jumpbox.GetInternalIP()).To(Equal("10.0.0.1"))

			Expect(bs.Env.PostgreSQLNode.GetName()).To(Equal("postgres"))
			Expect(bs.Env.PostgreSQLNode.GetExternalIP()).To(Equal("1.2.3.4"))
			Expect(bs.Env.PostgreSQLNode.GetInternalIP()).To(Equal("10.0.0.1"))

			for _, cephNode := range bs.Env.CephNodes {
				Expect(cephNode.GetName()).To(MatchRegexp("ceph-\\d+"))
				Expect(cephNode.GetExternalIP()).To(Equal("1.2.3.4"))
				Expect(cephNode.GetInternalIP()).To(Equal("10.0.0.1"))
			}

			for _, cpNode := range bs.Env.ControlPlaneNodes {
				Expect(cpNode.GetName()).To(MatchRegexp("k0s-\\d+"))
				Expect(cpNode.GetExternalIP()).To(Equal("1.2.3.4"))
				Expect(cpNode.GetInternalIP()).To(Equal("10.0.0.1"))
			}
		})
	})

	Describe("ValidateInput", func() {
		var (
			artifacts []portal.Artifact
		)
		BeforeEach(func() {
			mockPortalClient = portal.NewMockPortal(GinkgoT())
			csEnv.InstallVersion = "v1.2.3"
			csEnv.InstallHash = "abc123"
			artifacts = []portal.Artifact{
				{Filename: "installer-lite.tar.gz"},
			}
		})
		JustBeforeEach(func() {
			mockPortalClient.EXPECT().GetBuild(portal.CodesphereProduct, bs.Env.InstallVersion, bs.Env.InstallHash).Return(portal.Build{
				Artifacts: artifacts,
				Hash:      csEnv.InstallHash,
				Version:   csEnv.InstallVersion,
			}, nil)
		})

		Context("when GitHub arguments are partially set", func() {
			BeforeEach(func() {
				csEnv.GitHubAppName = ""
			})
			It("fails", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(MatchRegexp("GitHub app credentials are not fully specified")))
			})
		})

		Context("when GHCR registry is used", func() {
			BeforeEach(func() {
				csEnv.RegistryType = gcp.RegistryTypeGitHub
			})

			It("succeeds when package exists and has the lite package", func() {
				err := bs.ValidateInput()
				Expect(err).NotTo(HaveOccurred())
			})

			Context("when package exists but does not have the lite package", func() {
				BeforeEach(func() {
					artifacts[0].Filename = "installer.tar.gz"
				})
				It("fails", func() {
					err := bs.ValidateInput()
					Expect(err).To(MatchError(MatchRegexp("artifact installer-lite\\.tar\\.gz")))
				})
			})
		})

		Context("when non-GHCR registry is used", func() {
			BeforeEach(func() {
				csEnv.RegistryType = gcp.RegistryTypeArtifactRegistry
			})

			Context("when build exists and has the full package", func() {
				BeforeEach(func() {
					artifacts[0].Filename = "installer.tar.gz"
				})
				It("succeeds", func() {
					err := bs.ValidateInput()
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when package exists but does not have the full package", func() {
				BeforeEach(func() {
					artifacts[0].Filename = "installer-lite.tar.gz"
				})
				It("fails", func() {
					err := bs.ValidateInput()
					Expect(err).To(MatchError(MatchRegexp("artifact installer\\.tar\\.gz")))
				})
			})
		})
	})

	Describe("EnsureInstallConfig", func() {
		Describe("Valid EnsureInstallConfig", func() {
			BeforeEach(func() {
			})
			It("uses existing when config file exists", func() {
				fw.EXPECT().Exists(csEnv.InstallConfigPath).Return(true)
				icg.EXPECT().LoadInstallConfigFromFile(csEnv.InstallConfigPath).Return(nil)
				icg.EXPECT().GetInstallConfig().Return(&files.RootConfig{})

				err := bs.EnsureInstallConfig()
				Expect(err).NotTo(HaveOccurred())
			})

			It("creates install config when missing", func() {
				fw.EXPECT().Exists(csEnv.InstallConfigPath).Return(false)
				icg.EXPECT().ApplyProfile("dev").Return(nil)
				icg.EXPECT().GetInstallConfig().Return(&files.RootConfig{})

				err := bs.EnsureInstallConfig()
				Expect(err).NotTo(HaveOccurred())
				Expect(bs.Env.InstallConfig).NotTo(BeNil())
			})
		})

		Describe("Invalid cases", func() {
			It("returns error when config file exists but fails to load", func() {
				fw.EXPECT().Exists(csEnv.InstallConfigPath).Return(true)
				icg.EXPECT().LoadInstallConfigFromFile(csEnv.InstallConfigPath).Return(fmt.Errorf("bad format"))

				err := bs.EnsureInstallConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to load config file"))
				Expect(err.Error()).To(ContainSubstring("bad format"))
			})

			It("returns error when config file missing and applying profile fails", func() {
				fw.EXPECT().Exists(csEnv.InstallConfigPath).Return(false)
				icg.EXPECT().ApplyProfile("dev").Return(fmt.Errorf("profile error"))

				err := bs.EnsureInstallConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to apply profile"))
				Expect(err.Error()).To(ContainSubstring("profile error"))
			})
		})
	})

	Describe("EnsureSecrets", func() {
		Describe("Valid EnsureSecrets", func() {
			It("loads existing secrets file", func() {
				fw.EXPECT().Exists(csEnv.SecretsFilePath).Return(true)
				icg.EXPECT().LoadVaultFromFile(csEnv.SecretsFilePath).Return(nil)
				icg.EXPECT().MergeVaultIntoConfig().Return(nil)
				icg.EXPECT().GetVault().Return(&files.InstallVault{})

				err := bs.EnsureSecrets()
				Expect(err).NotTo(HaveOccurred())
			})

			It("skips when secrets file missing", func() {
				fw.EXPECT().Exists(csEnv.SecretsFilePath).Return(false)
				icg.EXPECT().GetVault().Return(&files.InstallVault{})

				err := bs.EnsureSecrets()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("returns error when secrets file load fails", func() {
				fw.EXPECT().Exists(csEnv.SecretsFilePath).Return(true)
				icg.EXPECT().LoadVaultFromFile(csEnv.SecretsFilePath).Return(fmt.Errorf("load error"))

				err := bs.EnsureSecrets()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to load vault file"))
				Expect(err.Error()).To(ContainSubstring("load error"))
			})

			It("returns error when merge fails", func() {
				fw.EXPECT().Exists(csEnv.SecretsFilePath).Return(true)
				icg.EXPECT().LoadVaultFromFile(csEnv.SecretsFilePath).Return(nil)
				icg.EXPECT().MergeVaultIntoConfig().Return(fmt.Errorf("merge error"))

				err := bs.EnsureSecrets()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to merge vault into config"))
				Expect(err.Error()).To(ContainSubstring("merge error"))
			})
		})
	})

	Describe("EnsureProject", func() {
		Describe("Valid EnsureProject", func() {
			It("uses existing project", func() {
				gc.EXPECT().GetProjectByName(csEnv.FolderID, csEnv.ProjectName).Return(&resourcemanagerpb.Project{ProjectId: "existing-id", Name: "existing-proj"}, nil)

				err := bs.EnsureProject()
				Expect(err).NotTo(HaveOccurred())
				Expect(bs.Env.ProjectID).To(Equal("existing-id"))
			})

			It("creates project when missing", func() {
				gc.EXPECT().GetProjectByName(csEnv.FolderID, csEnv.ProjectName).Return(nil, fmt.Errorf("project not found: %s", csEnv.ProjectName))
				gc.EXPECT().CreateProjectID(csEnv.ProjectName).Return("new-proj-id")
				gc.EXPECT().CreateProject(csEnv.FolderID, "new-proj-id", csEnv.ProjectName).Return("", nil)

				err := bs.EnsureProject()
				Expect(err).NotTo(HaveOccurred())
				Expect(bs.Env.ProjectID).To(Equal("new-proj-id"))
			})
		})

		Describe("Invalid cases", func() {
			It("returns error when GetProjectByName fails unexpectedly", func() {
				gc.EXPECT().GetProjectByName("", csEnv.ProjectName).Return(nil, fmt.Errorf("api error"))
				err := bs.EnsureProject()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get project"))
				Expect(err.Error()).To(ContainSubstring("api error"))
			})

			It("returns error when CreateProject fails", func() {
				gc.EXPECT().GetProjectByName("", csEnv.ProjectName).Return(nil, fmt.Errorf("project not found: %s", csEnv.ProjectName))
				gc.EXPECT().CreateProjectID(csEnv.ProjectName).Return("fake-id")
				gc.EXPECT().CreateProject("", "fake-id", csEnv.ProjectName).Return("", fmt.Errorf("create error"))

				err := bs.EnsureProject()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create project"))
				Expect(err.Error()).To(ContainSubstring("create error"))
			})
		})
	})

	Describe("EnsureBilling", func() {
		Describe("Valid EnsureBilling", func() {
			It("does nothing if billing already enabled correctly", func() {
				bi := &cloudbilling.ProjectBillingInfo{
					BillingEnabled:     true,
					BillingAccountName: csEnv.BillingAccount,
				}
				gc.EXPECT().GetBillingInfo(csEnv.ProjectID).Return(bi, nil)
				err := bs.EnsureBilling()
				Expect(err).NotTo(HaveOccurred())
			})

			It("enables billing if not enabled", func() {
				bi := &cloudbilling.ProjectBillingInfo{
					BillingEnabled: false,
				}
				gc.EXPECT().GetBillingInfo(csEnv.ProjectID).Return(bi, nil)
				gc.EXPECT().EnableBilling(csEnv.ProjectID, csEnv.BillingAccount).Return(nil)

				err := bs.EnsureBilling()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when GetBillingInfo fails", func() {
				gc.EXPECT().GetBillingInfo(csEnv.ProjectID).Return(nil, fmt.Errorf("billing info error"))

				err := bs.EnsureBilling()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get billing info"))
				Expect(err.Error()).To(ContainSubstring("billing info error"))
			})

			It("fails when EnableBilling fails", func() {
				bi := &cloudbilling.ProjectBillingInfo{
					BillingEnabled: false,
				}
				gc.EXPECT().GetBillingInfo(csEnv.ProjectID).Return(bi, nil)
				gc.EXPECT().EnableBilling(csEnv.ProjectID, csEnv.BillingAccount).Return(fmt.Errorf("enable error"))

				err := bs.EnsureBilling()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to enable billing"))
				Expect(err.Error()).To(ContainSubstring("enable error"))
			})
		})
	})

	Describe("EnsureAPIsEnabled", func() {
		Describe("Valid EnsureAPIsEnabled", func() {
			It("enables default APIs", func() {
				gc.EXPECT().EnableAPIs(csEnv.ProjectID, []string{
					"compute.googleapis.com",
					"serviceusage.googleapis.com",
					"artifactregistry.googleapis.com",
					"dns.googleapis.com",
				}).Return(nil)

				err := bs.EnsureAPIsEnabled()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when EnableAPIs fails", func() {
				gc.EXPECT().EnableAPIs(csEnv.ProjectID, mock.Anything).Return(fmt.Errorf("api error"))

				err := bs.EnsureAPIsEnabled()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to enable APIs"))
				Expect(err.Error()).To(ContainSubstring("api error"))
			})
		})
	})

	Describe("EnsureArtifactRegistry", func() {
		Describe("Valid EnsureArtifactRegistry", func() {
			It("uses existing registry if present", func() {
				repo := &artifactregistrypb.Repository{Name: "projects/" + csEnv.ProjectID + "/locations/" + csEnv.Region + "/repositories/codesphere-registry"}
				gc.EXPECT().GetArtifactRegistry(csEnv.ProjectID, csEnv.Region, "codesphere-registry").Return(repo, nil)

				err := bs.EnsureArtifactRegistry()
				Expect(err).NotTo(HaveOccurred())
			})

			It("creates registry if missing", func() {
				gc.EXPECT().GetArtifactRegistry(csEnv.ProjectID, csEnv.Region, "codesphere-registry").Return(nil, fmt.Errorf("not found"))

				createdRepo := &artifactregistrypb.Repository{Name: "projects/" + csEnv.ProjectID + "/locations/" + csEnv.Region + "/repositories/codesphere-registry"}
				gc.EXPECT().CreateArtifactRegistry(csEnv.ProjectID, csEnv.Region, "codesphere-registry").Return(createdRepo, nil)

				err := bs.EnsureArtifactRegistry()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when CreateArtifactRegistry fails", func() {
				gc.EXPECT().GetArtifactRegistry(csEnv.ProjectID, csEnv.Region, "codesphere-registry").Return(nil, fmt.Errorf("not found"))
				gc.EXPECT().CreateArtifactRegistry(csEnv.ProjectID, csEnv.Region, "codesphere-registry").Return(nil, fmt.Errorf("create error"))

				err := bs.EnsureArtifactRegistry()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create artifact registry"))
				Expect(err.Error()).To(ContainSubstring("create error"))
			})
		})
	})

	Describe("EnsureLocalContainerRegistry", func() {
		Describe("Valid EnsureLocalContainerRegistry", func() {
			It("installs local registry", func() {
				// Setup mocked node
				// Check if running - return error to simulate not running
				nodeClient.EXPECT().RunCommand(bs.Env.PostgreSQLNode, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "podman ps")
				})).Return(fmt.Errorf("not running"))

				// Install commands (8 commands) + scp/update-ca/docker commands (3 per 4 nodes = 12)
				nodeClient.EXPECT().RunCommand(mock.Anything, "root", mock.Anything).Return(nil).Times(8 + 12)

				bs.Env.ControlPlaneNodes = []*node.Node{fakeNode("k0s-1", nodeClient), fakeNode("k0s-2", nodeClient)}
				bs.Env.CephNodes = []*node.Node{fakeNode("ceph-1", nodeClient), fakeNode("ceph-2", nodeClient)}

				err := bs.EnsureLocalContainerRegistry()
				Expect(err).NotTo(HaveOccurred())
				Expect(bs.Env.InstallConfig.Registry.Username).To(Equal("custom-registry"))
			})
		})

		Describe("Invalid cases", func() {
			BeforeEach(func() {
				ctx = context.Background()
				csEnv = &gcp.CodesphereEnvironment{
					ProjectID: "pid",
					InstallConfig: &files.RootConfig{
						Registry: &files.RegistryConfig{},
					},
					PostgreSQLNode:    fakeNode("postgres", nodeClient),
					ControlPlaneNodes: []*node.Node{fakeNode("k0s-1", nodeClient), fakeNode("k0s-2", nodeClient)},
					CephNodes:         []*node.Node{fakeNode("ceph-1", nodeClient), fakeNode("ceph-2", nodeClient)},
				}

				icg = installer.NewMockInstallConfigManager(GinkgoT())
				gc = gcp.NewMockGCPClientManager(GinkgoT())
				fw = util.NewMockFileIO(GinkgoT())

			})

			It("fails when the 8th install command fails", func() {
				// First check - registry not running
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "podman ps")
				})).Return(fmt.Errorf("not running"))

				// First 7 install commands succeed
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.Anything).Return(nil).Times(7)

				// 8th install command fails
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.Anything).Return(fmt.Errorf("ssh error")).Once()

				err := bs.EnsureLocalContainerRegistry()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("ssh error"))
			})

			It("fails when the first scp command fails", func() {
				// First check - registry not running
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "podman ps")
				})).Return(fmt.Errorf("not running"))

				// All 8 install commands succeed
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.Anything).Return(nil).Times(8)

				// First scp command fails
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.HasPrefix(cmd, "scp ")
				})).Return(fmt.Errorf("scp error")).Once()

				err := bs.EnsureLocalContainerRegistry()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to copy registry certificate"))
			})

			It("fails when update-ca-certificates fails", func() {
				// Override node setup for this test
				bs.Env.ControlPlaneNodes = []*node.Node{fakeNode("k0s-1", nodeClient)}
				bs.Env.CephNodes = []*node.Node{}

				// First check - registry not running
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "podman ps")
				})).Return(fmt.Errorf("not running"))

				// All 8 install commands succeed
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.Anything).Return(nil).Times(8)
				// scp succeeds
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.HasPrefix(cmd, "scp ")
				})).Return(nil).Once()

				// update-ca-certificates fails
				nodeClient.EXPECT().RunCommand(bs.Env.ControlPlaneNodes[0], "root", "update-ca-certificates").Return(fmt.Errorf("ca update error")).Once()

				err := bs.EnsureLocalContainerRegistry()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to update CA certificates"))
			})

			It("fails when docker restart fails", func() {
				// Override node setup for this test
				bs.Env.ControlPlaneNodes = []*node.Node{fakeNode("k0s-1", nodeClient)}
				bs.Env.CephNodes = []*node.Node{}

				// First check - registry not running
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "podman ps")
				})).Return(fmt.Errorf("not running"))

				// All 8 install commands succeed
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.Anything).Return(nil).Times(8)

				// scp succeeds
				nodeClient.EXPECT().RunCommand(csEnv.PostgreSQLNode, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.HasPrefix(cmd, "scp ")
				})).Return(nil).Once()

				// update-ca-certificates succeeds
				nodeClient.EXPECT().RunCommand(bs.Env.ControlPlaneNodes[0], "root", "update-ca-certificates").Return(nil).Once()

				// docker restart fails
				nodeClient.EXPECT().RunCommand(bs.Env.ControlPlaneNodes[0], "root", "systemctl restart docker.service || true").Return(fmt.Errorf("docker restart error")).Once()

				err := bs.EnsureLocalContainerRegistry()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to restart docker service"))
			})
		})
	})

	Describe("EnsureGitHubAccessConfigured", func() {
		BeforeEach(func() {
			csEnv.GitHubPAT = "fake-pat"
			csEnv.RegistryUser = "custom-registry"
		})
		It("sets configuration options in installconfig", func() {
			err := bs.EnsureGitHubAccessConfigured()
			Expect(err).NotTo(HaveOccurred())
			Expect(bs.Env.InstallConfig.Registry.Server).To(Equal("ghcr.io"))
			Expect(bs.Env.InstallConfig.Registry.Username).To(Equal(csEnv.RegistryUser))
			Expect(bs.Env.InstallConfig.Registry.Password).To(Equal(csEnv.GitHubPAT))
			Expect(bs.Env.InstallConfig.Registry.LoadContainerImages).To(BeFalse())
			Expect(bs.Env.InstallConfig.Registry.ReplaceImagesInBom).To(BeFalse())
		})

		Context("When GitHub PAT is missing", func() {
			BeforeEach(func() {
				csEnv.GitHubPAT = ""
			})
			It("returns an error", func() {
				err := bs.EnsureGitHubAccessConfigured()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("GitHub PAT is not set"))
			})
		})
	})

	Describe("EnsureServiceAccounts", func() {
		Describe("Valid EnsureServiceAccounts", func() {
			Context("When using local container registry", func() {
				BeforeEach(func() {
					csEnv.RegistryType = gcp.RegistryTypeLocalContainer
				})
				It("creates cloud-controller and skips writer", func() {
					gc.EXPECT().CreateServiceAccount(csEnv.ProjectID, "cloud-controller", "cloud-controller").Return("email@sa", false, nil)

					err := bs.EnsureServiceAccounts()
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("When using artifact registry", func() {
				BeforeEach(func() {
					csEnv.RegistryType = gcp.RegistryTypeArtifactRegistry
				})

				It("creates both accounts", func() {
					csEnv = &gcp.CodesphereEnvironment{
						ProjectID:    "pid",
						RegistryType: gcp.RegistryTypeArtifactRegistry,
						InstallConfig: &files.RootConfig{
							Registry: &files.RegistryConfig{},
						},
					}

					gc.EXPECT().CreateServiceAccount(csEnv.ProjectID, "cloud-controller", "cloud-controller").Return("email@sa", false, nil)
					gc.EXPECT().CreateServiceAccount(csEnv.ProjectID, "artifact-registry-writer", "artifact-registry-writer").Return("writer@sa", true, nil)
					gc.EXPECT().CreateServiceAccountKey(csEnv.ProjectID, "writer@sa").Return("key-content", nil)
					err := bs.EnsureServiceAccounts()
					Expect(err).NotTo(HaveOccurred())
					Expect(bs.Env.InstallConfig.Registry.Password).To(Equal("key-content"))
				})
			})
		})

		Describe("Invalid cases", func() {
			It("fails when cloud-controller creation fails", func() {
				gc.EXPECT().CreateServiceAccount(csEnv.ProjectID, "cloud-controller", "cloud-controller").Return("", false, fmt.Errorf("create error"))

				err := bs.EnsureServiceAccounts()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("create error"))
			})
		})
	})

	Describe("EnsureIAMRoles", func() {
		Describe("Valid EnsureIAMRoles", func() {
			BeforeEach(func() {
				csEnv.RegistryType = gcp.RegistryTypeArtifactRegistry
			})
			It("assigns roles correctly", func() {
				gc.EXPECT().AssignIAMRole(csEnv.ProjectID, "cloud-controller", csEnv.ProjectID, []string{"roles/compute.admin"}).Return(nil)
				gc.EXPECT().AssignIAMRole(csEnv.DNSProjectID, "cloud-controller", csEnv.ProjectID, []string{"roles/dns.admin"}).Return(nil)
				gc.EXPECT().AssignIAMRole(csEnv.ProjectID, "artifact-registry-writer", csEnv.ProjectID, []string{"roles/artifactregistry.writer"}).Return(nil)

				err := bs.EnsureIAMRoles()
				Expect(err).NotTo(HaveOccurred())
			})

			Context("When DNS project is unset", func() {
				BeforeEach(func() {
					csEnv.DNSProjectID = ""
				})
				It("assigns DNS role to cloud-controller in main project", func() {
					gc.EXPECT().AssignIAMRole(csEnv.ProjectID, "cloud-controller", csEnv.ProjectID, []string{"roles/compute.admin"}).Return(nil)
					gc.EXPECT().AssignIAMRole(csEnv.ProjectID, "cloud-controller", csEnv.ProjectID, []string{"roles/dns.admin"}).Return(nil)
					gc.EXPECT().AssignIAMRole(csEnv.ProjectID, "artifact-registry-writer", csEnv.ProjectID, []string{"roles/artifactregistry.writer"}).Return(nil)

					err := bs.EnsureIAMRoles()
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})

		Describe("Invalid cases", func() {
			It("fails when AssignIAMRole fails", func() {
				gc.EXPECT().AssignIAMRole(csEnv.ProjectID, "cloud-controller", csEnv.ProjectID, []string{"roles/compute.admin"}).Return(fmt.Errorf("iam error"))

				err := bs.EnsureIAMRoles()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("iam error"))
			})
		})
	})

	Describe("EnsureVPC", func() {
		Describe("Valid EnsureVPC", func() {
			It("creates VPC, subnet, router, and nat", func() {
				gc.EXPECT().CreateVPC(csEnv.ProjectID, csEnv.Region, "pid-vpc", "pid-us-central1-subnet", "pid-router", "pid-nat-gateway").Return(nil)

				err := bs.EnsureVPC()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when CreateVPC fails", func() {
				gc.EXPECT().CreateVPC(csEnv.ProjectID, csEnv.Region, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("vpc error"))

				err := bs.EnsureVPC()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to ensure VPC"))
				Expect(err.Error()).To(ContainSubstring("vpc error"))
			})
		})
	})

	Describe("EnsureFirewallRules", func() {
		Describe("Valid EnsureFirewallRules", func() {
			It("creates required firewall rules", func() {
				// Expect 4 rules: allow-ssh-ext, allow-internal, allow-all-egress, allow-ingress-web, allow-ingress-postgres
				// Wait, code showed 5 blocks? ssh, internal, egress, web, postgres.
				gc.EXPECT().CreateFirewallRule(csEnv.ProjectID, mock.MatchedBy(func(r *computepb.Firewall) bool {
					return *r.Name == "allow-ssh-ext"
				})).Return(nil)
				gc.EXPECT().CreateFirewallRule(csEnv.ProjectID, mock.MatchedBy(func(r *computepb.Firewall) bool {
					return *r.Name == "allow-internal"
				})).Return(nil)
				gc.EXPECT().CreateFirewallRule(csEnv.ProjectID, mock.MatchedBy(func(r *computepb.Firewall) bool {
					return *r.Name == "allow-all-egress"
				})).Return(nil)
				gc.EXPECT().CreateFirewallRule(csEnv.ProjectID, mock.MatchedBy(func(r *computepb.Firewall) bool {
					return *r.Name == "allow-ingress-web"
				})).Return(nil)
				gc.EXPECT().CreateFirewallRule(csEnv.ProjectID, mock.MatchedBy(func(r *computepb.Firewall) bool {
					return *r.Name == "allow-ingress-postgres"
				})).Return(nil)

				err := bs.EnsureFirewallRules()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when first firewall rule creation fails", func() {
				gc.EXPECT().CreateFirewallRule(csEnv.ProjectID, mock.Anything).Return(fmt.Errorf("firewall error")).Once()

				err := bs.EnsureFirewallRules()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create jumpbox ssh firewall rule"))
			})
		})
	})

	Describe("EnsureComputeInstances", func() {
		BeforeEach(func() {
			csEnv.ControlPlaneNodes = []*node.Node{}
			csEnv.CephNodes = []*node.Node{}
		})
		Describe("Valid EnsureComputeInstances", func() {
			It("creates all instances", func() {
				// Mock ReadFile for SSH key (called 9 times in parallel)
				fw.EXPECT().ReadFile(csEnv.SSHPublicKeyPath).Return([]byte("ssh-rsa AAA..."), nil).Times(9)

				// Mock CreateInstance (9 times)
				gc.EXPECT().CreateInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(nil).Times(9)

				// Mock GetInstance (9 times)
				ipResp := &computepb.Instance{
					NetworkInterfaces: []*computepb.NetworkInterface{
						{
							NetworkIP: protoString("10.0.0.x"),
							AccessConfigs: []*computepb.AccessConfig{
								{NatIP: protoString("1.2.3.x")},
							},
						},
					},
				}
				gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(ipResp, nil).Times(9)

				err := bs.EnsureComputeInstances()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(bs.Env.ControlPlaneNodes)).To(Equal(3))
				Expect(len(bs.Env.CephNodes)).To(Equal(4))
				Expect(bs.Env.PostgreSQLNode).NotTo(BeNil())
				Expect(bs.Env.Jumpbox).NotTo(BeNil())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when SSH key read fails", func() {
				fw.EXPECT().ReadFile(csEnv.SSHPublicKeyPath).Return(nil, fmt.Errorf("read error")).Maybe()

				err := bs.EnsureComputeInstances()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error ensuring compute instances"))
			})

			It("fails when CreateInstance fails", func() {
				fw.EXPECT().ReadFile(csEnv.SSHPublicKeyPath).Return([]byte("ssh-rsa AAA..."), nil).Maybe()
				gc.EXPECT().CreateInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(fmt.Errorf("create error")).Maybe()

				err := bs.EnsureComputeInstances()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error ensuring compute instances"))
			})

			It("fails when GetInstance fails", func() {
				fw.EXPECT().ReadFile(csEnv.SSHPublicKeyPath).Return([]byte("ssh-rsa AAA..."), nil).Maybe()
				gc.EXPECT().CreateInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(nil).Maybe()
				gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(nil, fmt.Errorf("get error")).Maybe()

				err := bs.EnsureComputeInstances()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error ensuring compute instances"))
			})
		})
	})

	Describe("EnsureGatewayIPAddresses", func() {
		Describe("Valid EnsureGatewayIPAddresses", func() {
			It("creates two addresses", func() {
				// Gateway
				gc.EXPECT().GetAddress(csEnv.ProjectID, csEnv.Region, "gateway").Return(nil, fmt.Errorf("not found"))
				gc.EXPECT().CreateAddress(csEnv.ProjectID, csEnv.Region, mock.MatchedBy(func(a *computepb.Address) bool {
					return *a.Name == "gateway"
				})).Return("1.1.1.1", nil)

				// Public Gateway
				gc.EXPECT().GetAddress(csEnv.ProjectID, csEnv.Region, "public-gateway").Return(nil, fmt.Errorf("not found"))
				gc.EXPECT().CreateAddress(csEnv.ProjectID, csEnv.Region, mock.MatchedBy(func(a *computepb.Address) bool {
					return *a.Name == "public-gateway"
				})).Return("2.2.2.2", nil)

				err := bs.EnsureGatewayIPAddresses()
				Expect(err).NotTo(HaveOccurred())
				Expect(bs.Env.GatewayIP).To(Equal("1.1.1.1"))
				Expect(bs.Env.PublicGatewayIP).To(Equal("2.2.2.2"))
			})
		})

		Describe("Invalid cases", func() {
			It("fails when gateway IP creation fails", func() {
				gc.EXPECT().GetAddress(csEnv.ProjectID, csEnv.Region, "gateway").Return(nil, fmt.Errorf("not found"))
				gc.EXPECT().CreateAddress(csEnv.ProjectID, csEnv.Region, mock.Anything).Return("", fmt.Errorf("create error"))

				err := bs.EnsureGatewayIPAddresses()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to ensure gateway IP"))
			})

			It("fails when public gateway IP creation fails", func() {
				gc.EXPECT().GetAddress(csEnv.ProjectID, csEnv.Region, "gateway").Return(nil, fmt.Errorf("not found"))
				gc.EXPECT().CreateAddress(csEnv.ProjectID, csEnv.Region, mock.MatchedBy(func(a *computepb.Address) bool {
					return *a.Name == "gateway"
				})).Return("1.1.1.1", nil)
				gc.EXPECT().GetAddress(csEnv.ProjectID, csEnv.Region, "public-gateway").Return(nil, fmt.Errorf("not found"))
				gc.EXPECT().CreateAddress(csEnv.ProjectID, csEnv.Region, mock.MatchedBy(func(a *computepb.Address) bool {
					return *a.Name == "public-gateway"
				})).Return("", fmt.Errorf("create error"))

				err := bs.EnsureGatewayIPAddresses()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to ensure public gateway IP"))
			})
		})
	})

	Describe("EnsureRootLoginEnabled", func() {
		Context("When WaitReady times out", func() {
			It("fails", func() {
				nodeClient.EXPECT().WaitReady(mock.Anything, mock.Anything).Return(fmt.Errorf("TIMEOUT!"))

				err := bs.EnsureRootLoginEnabled()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("timed out waiting for SSH service"))
			})
		})
		Context("When WaitReady succeeds", func() {
			BeforeEach(func() {
				nodeClient.EXPECT().WaitReady(mock.Anything, mock.Anything).Return(nil)
			})
			Describe("Valid EnsureRootLoginEnabled", func() {
				It("enables root login on all nodes", func() {
					nodeClient.EXPECT().RunCommand(mock.Anything, "ubuntu", mock.Anything).Return(nil)

					// Setup nodes

					err := bs.EnsureRootLoginEnabled()
					Expect(err).NotTo(HaveOccurred())
				})
			})

			It("fails when EnableRootLogin fails", func() {
				nodeClient.EXPECT().RunCommand(mock.Anything, "ubuntu", mock.Anything).Return(fmt.Errorf("ouch"))

				err := bs.EnsureRootLoginEnabled()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to enable root login"))
			})
		})
	})

	Describe("EnsureJumpboxConfigured", func() {
		Describe("Valid EnsureJumpboxConfigured", func() {
			It("configures jumpbox", func() {
				// Setup jumpbox node requires some commands to run
				nodeClient.EXPECT().RunCommand(mock.Anything, mock.Anything, mock.Anything).Return(nil)

				err := bs.EnsureJumpboxConfigured()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when ConfigureAcceptEnv fails", func() {
				// Setup jumpbox node requires some commands to run
				nodeClient.EXPECT().RunCommand(mock.Anything, "ubuntu", mock.Anything).Return(fmt.Errorf("ouch")).Twice()

				err := bs.EnsureJumpboxConfigured()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to configure AcceptEnv"))
			})

			It("fails when InstallOms fails", func() {
				nodeClient.EXPECT().RunCommand(mock.Anything, "ubuntu", mock.Anything).Return(nil)
				nodeClient.EXPECT().RunCommand(mock.Anything, "root", mock.Anything).Return(fmt.Errorf("outch"))

				err := bs.EnsureJumpboxConfigured()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to install OMS"))
			})
		})
	})

	Describe("EnsureHostsConfigured", func() {
		Describe("Valid EnsureHostsConfigured", func() {
			It("configures hosts", func() {
				nodeClient.EXPECT().RunCommand(mock.Anything, "root", mock.Anything).Return(nil)

				err := bs.EnsureHostsConfigured()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when ConfigureInotifyWatches fails", func() {
				nodeClient.EXPECT().RunCommand(mock.Anything, "root", mock.Anything).Return(fmt.Errorf("ouch"))

				err := bs.EnsureHostsConfigured()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to configure inotify watches"))
			})

			It("fails when ConfigureMemoryMap fails", func() {
				mock.InOrder(
					nodeClient.EXPECT().RunCommand(mock.Anything, "root", mock.Anything).Return(nil).Times(1),                // for inotify
					nodeClient.EXPECT().RunCommand(mock.Anything, "root", mock.Anything).Return(fmt.Errorf("ouch")).Times(2), // for memory map
				)

				err := bs.EnsureHostsConfigured()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to configure memory map"))
			})
		})
	})

	Describe("UpdateInstallConfig", func() {
		BeforeEach(func() {
			csEnv.GitHubAppName = "fake-app-name"
		})
		Describe("Valid UpdateInstallConfig", func() {
			It("updates config and writes files", func() {
				icg.EXPECT().GenerateSecrets().Return(nil)
				icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
				icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

				nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

				err := bs.UpdateInstallConfig()
				Expect(err).NotTo(HaveOccurred())

				Expect(bs.Env.InstallConfig.Datacenter.ID).To(Equal(1))
				Expect(bs.Env.InstallConfig.Codesphere.Domain).To(Equal("cs.example.com"))
				Expect(bs.Env.InstallConfig.Codesphere.Features).To(Equal([]string{}))
				Expect(bs.Env.InstallConfig.Codesphere.Experiments).To(Equal(gcp.DefaultExperiments))

				expectedInstallURI := "https://github.com/apps/" + bs.Env.GitHubAppName + "/installations/new"
				Expect(bs.Env.InstallConfig.Codesphere.GitProviders.GitHub.OAuth.InstallationURI).To(Equal(expectedInstallURI))
				expectedRedirectURI := "https://cs." + bs.Env.BaseDomain + "/ide/auth/github/callback"
				Expect(bs.Env.InstallConfig.Codesphere.GitProviders.GitHub.OAuth.RedirectURI).To(Equal(expectedRedirectURI))
				Expect(bs.Env.InstallConfig.Codesphere.GitProviders.GitHub.OAuth.ClientAuthMethod).To(Equal("client_secret_post"))

				issuers := bs.Env.InstallConfig.Cluster.Certificates.Override["issuers"].(map[string]interface{})
				httpIssuer := issuers["letsEncryptHttp"].(map[string]interface{})
				Expect(httpIssuer["enabled"]).To(Equal(true))

				acme := issuers["acme"].(map[string]interface{})
				dnsIssuer := acme["dnsSolver"].(map[string]interface{})
				dnsConfig := dnsIssuer["config"].(map[string]interface{})
				cloudDns := dnsConfig["cloudDNS"].(map[string]interface{})
				Expect(cloudDns["project"]).To(Equal(bs.Env.DNSProjectID))
			})
			Context("When Experiments are set in CodesphereEnvironment", func() {
				BeforeEach(func() {
					csEnv.Experiments = []string{"fake-exp1", "fake-exp2"}
				})
				It("uses those experiments instead of defaults", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.Experiments).To(Equal([]string{"fake-exp1", "fake-exp2"}))
				})
			})
			Context("When feature flags are set in CodesphereEnvironment", func() {
				BeforeEach(func() {
					csEnv.FeatureFlags = []string{"fake-flag1", "fake-flag2"}
				})
				It("uses those feature flags", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.Features).To(Equal([]string{"fake-flag1", "fake-flag2"}))
				})
			})
			Context("When GitHub App name is not set ", func() {
				BeforeEach(func() {
					csEnv.GitHubAppName = ""
				})
				It("skips setting GitHub OAuth configuration", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.GitHub).To(BeNil())
				})

			})
		})

		Describe("Invalid cases", func() {
			It("fails when GenerateSecrets fails", func() {
				icg.EXPECT().GenerateSecrets().Return(fmt.Errorf("generate error"))

				err := bs.UpdateInstallConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to generate secrets"))
			})

			It("fails when WriteInstallConfig fails", func() {
				icg.EXPECT().GenerateSecrets().Return(nil)
				icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(fmt.Errorf("write error"))

				err := bs.UpdateInstallConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to write config file"))
			})

			It("fails when WriteVault fails", func() {
				icg.EXPECT().GenerateSecrets().Return(nil)
				icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
				icg.EXPECT().WriteVault("fake-secret", true).Return(fmt.Errorf("vault write error"))

				err := bs.UpdateInstallConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to write vault file"))
			})

			It("fails when CopyFile config fails", func() {
				icg.EXPECT().GenerateSecrets().Return(nil)
				icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
				icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

				nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("copy error")).Once()

				err := bs.UpdateInstallConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to copy install config to jumpbox"))
			})

			It("fails when CopyFile secrets fails", func() {
				icg.EXPECT().GenerateSecrets().Return(nil)
				icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
				icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

				nodeClient.EXPECT().CopyFile(mock.Anything, "fake-config-file", mock.Anything).Return(nil).Once()
				nodeClient.EXPECT().CopyFile(mock.Anything, "fake-secret", mock.Anything).Return(fmt.Errorf("copy error")).Once()

				err := bs.UpdateInstallConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to copy secrets file to jumpbox"))
			})
		})
	})

	Describe("EnsureAgeKey", func() {
		Describe("Valid EnsureAgeKey", func() {
			It("generates key if missing", func() {
				nodeClient.EXPECT().HasFile(mock.MatchedBy(jumpbboxMatcher), "/etc/codesphere/secrets/age_key.txt").Return(false)
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "mkdir -p /etc/codesphere/secrets; age-keygen -o /etc/codesphere/secrets/age_key.txt").Return(nil)

				err := bs.EnsureAgeKey()
				Expect(err).NotTo(HaveOccurred())
			})

			It("skips if key exists", func() {
				nodeClient.EXPECT().HasFile(mock.MatchedBy(jumpbboxMatcher), "/etc/codesphere/secrets/age_key.txt").Return(true)

				err := bs.EnsureAgeKey()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when age-keygen command fails", func() {

				nodeClient.EXPECT().HasFile(mock.MatchedBy(jumpbboxMatcher), "/etc/codesphere/secrets/age_key.txt").Return(false)
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "mkdir -p /etc/codesphere/secrets; age-keygen -o /etc/codesphere/secrets/age_key.txt").Return(fmt.Errorf("ouch"))

				err := bs.EnsureAgeKey()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to generate age key on jumpbox"))
			})
		})
	})

	Describe("EncryptVault", func() {
		Describe("Valid EncryptVault", func() {
			It("encrypts vault using sops", func() {
				nodeClient.EXPECT().RunCommand(bs.Env.Jumpbox, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.HasPrefix(cmd, "cp ")
				})).Return(nil)

				nodeClient.EXPECT().RunCommand(bs.Env.Jumpbox, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "sops --encrypt")
				})).Return(nil)

				err := bs.EncryptVault()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when backup vault command fails", func() {
				nodeClient.EXPECT().RunCommand(bs.Env.Jumpbox, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.HasPrefix(cmd, "cp ")
				})).Return(fmt.Errorf("backup error"))

				err := bs.EncryptVault()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed backup vault on jumpbox"))
			})

			It("fails when sops encrypt command fails", func() {
				nodeClient.EXPECT().RunCommand(bs.Env.Jumpbox, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.HasPrefix(cmd, "cp ")
				})).Return(nil)

				nodeClient.EXPECT().RunCommand(bs.Env.Jumpbox, "root", mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "sops --encrypt")
				})).Return(fmt.Errorf("encrypt error"))

				err := bs.EncryptVault()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to encrypt vault on jumpbox"))
			})
		})
	})

	Describe("EnsureDNSRecords", func() {
		Describe("Valid EnsureDNSRecords", func() {
			It("ensures DNS records", func() {
				gc.EXPECT().EnsureDNSManagedZone(csEnv.DNSProjectID, csEnv.DNSZoneName, csEnv.BaseDomain+".", mock.Anything).Return(nil)
				gc.EXPECT().EnsureDNSRecordSets(csEnv.DNSProjectID, csEnv.DNSZoneName, mock.MatchedBy(func(records []*dns.ResourceRecordSet) bool {
					// Expect 4 records: *.ws, *.cs, cs, ws
					return len(records) == 4
				})).Return(nil)

				err := bs.EnsureDNSRecords()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when EnsureDNSManagedZone fails", func() {
				gc.EXPECT().EnsureDNSManagedZone(csEnv.DNSProjectID, csEnv.DNSZoneName, csEnv.BaseDomain+".", mock.Anything).Return(fmt.Errorf("zone error"))

				err := bs.EnsureDNSRecords()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to ensure DNS managed zone"))
			})

			It("fails when EnsureDNSRecordSets fails", func() {
				gc.EXPECT().EnsureDNSManagedZone(csEnv.DNSProjectID, csEnv.DNSZoneName, csEnv.BaseDomain+".", mock.Anything).Return(nil)
				gc.EXPECT().EnsureDNSRecordSets(csEnv.DNSProjectID, csEnv.DNSZoneName, mock.Anything).Return(fmt.Errorf("record error"))

				err := bs.EnsureDNSRecords()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to ensure DNS record sets"))
			})
		})
	})

	Describe("InstallCodesphere", func() {
		BeforeEach(func() {
			csEnv.InstallVersion = "v1.2.3"
			csEnv.InstallHash = "abc1234567890"
		})
		Describe("Valid InstallCodesphere", func() {
			Context("Direct GitHub access", func() {
				BeforeEach(func() {
					csEnv.GitHubPAT = "fake-pat"
					csEnv.RegistryUser = "fake-user"
					csEnv.RegistryType = "github"

				})
				It("downloads and installs lite package", func() {
					mockPortalClient.EXPECT().GetBuild(portal.CodesphereProduct, "v1.2.3", "abc1234567890").Return(portal.Build{
						Version: "v1.2.3",
						Hash:    "abc1234567890",
					}, nil)

					// Expect download package
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "oms-cli download package -f installer-lite.tar.gz -H abc1234567890 v1.2.3").Return(nil)

					// Expect install codesphere
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root",
						"oms-cli install codesphere -c /etc/codesphere/config.yaml -k /etc/codesphere/secrets/age_key.txt -p v1.2.3-abc1234567890-installer-lite.tar.gz -s load-container-images").Return(nil)

					err := bs.InstallCodesphere()
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("without explicit hash", func() {
				BeforeEach(func() {
					csEnv.InstallHash = ""
				})
				It("downloads and installs codesphere", func() {
					mockPortalClient.EXPECT().GetBuild(portal.CodesphereProduct, "v1.2.3", "").Return(portal.Build{
						Version: "v1.2.3",
						Hash:    "def9876543210",
					}, nil)

					// Expect download package
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "oms-cli download package -f installer.tar.gz v1.2.3").Return(nil)

					// Expect install codesphere
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "oms-cli install codesphere -c /etc/codesphere/config.yaml -k /etc/codesphere/secrets/age_key.txt -p v1.2.3-def9876543210-installer.tar.gz").Return(nil)

					err := bs.InstallCodesphere()
					Expect(err).NotTo(HaveOccurred())
				})
			})

			It("downloads and installs codesphere with hash", func() {
				mockPortalClient.EXPECT().GetBuild(portal.CodesphereProduct, "v1.2.3", "abc1234567890").Return(portal.Build{
					Version: "v1.2.3",
					Hash:    "abc1234567890",
				}, nil)

				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "oms-cli download package -f installer.tar.gz -H abc1234567890 v1.2.3").Return(nil)
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "oms-cli install codesphere -c /etc/codesphere/config.yaml -k /etc/codesphere/secrets/age_key.txt -p v1.2.3-abc1234567890-installer.tar.gz").Return(nil)

				err := bs.InstallCodesphere()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when GetBuild fails", func() {
				mockPortalClient.EXPECT().GetBuild(portal.CodesphereProduct, "v1.2.3", "abc1234567890").Return(portal.Build{}, fmt.Errorf("portal error"))

				err := bs.InstallCodesphere()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get build info"))
			})

			It("fails when download package fails", func() {
				mockPortalClient.EXPECT().GetBuild(portal.CodesphereProduct, "v1.2.3", "abc1234567890").Return(portal.Build{
					Version: "v1.2.3",
					Hash:    "abc1234567890",
				}, nil)
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "oms-cli download package -f installer.tar.gz -H abc1234567890 v1.2.3").Return(fmt.Errorf("download error"))

				err := bs.InstallCodesphere()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to download Codesphere package from jumpbox"))
			})

			It("fails when install codesphere fails", func() {
				mockPortalClient.EXPECT().GetBuild(portal.CodesphereProduct, "v1.2.3", "abc1234567890").Return(portal.Build{
					Version: "v1.2.3",
					Hash:    "abc1234567890",
				}, nil)
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "oms-cli download package -f installer.tar.gz -H abc1234567890 v1.2.3").Return(nil).Once()
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpbboxMatcher), "root", "oms-cli install codesphere -c /etc/codesphere/config.yaml -k /etc/codesphere/secrets/age_key.txt -p v1.2.3-abc1234567890-installer.tar.gz").Return(fmt.Errorf("install error")).Once()

				err := bs.InstallCodesphere()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to install Codesphere from jumpbox"))
			})
		})
	})

	Describe("GenerateK0sConfigScript", func() {
		Describe("Valid GenerateK0sConfigScript", func() {
			It("generates script", func() {
				fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(nil)
				nodeClient.EXPECT().CopyFile(bs.Env.ControlPlaneNodes[0], "configure-k0s.sh", "/root/configure-k0s.sh").Return(nil)
				nodeClient.EXPECT().RunCommand(bs.Env.ControlPlaneNodes[0], "root", "chmod +x /root/configure-k0s.sh").Return(nil)

				err := bs.GenerateK0sConfigScript()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when WriteFile fails", func() {
				fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(fmt.Errorf("write error"))

				err := bs.GenerateK0sConfigScript()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to write configure-k0s.sh"))
			})

			It("fails when CopyFile fails", func() {
				fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(nil)
				nodeClient.EXPECT().CopyFile(mock.Anything, "configure-k0s.sh", "/root/configure-k0s.sh").Return(fmt.Errorf("copy error"))

				err := bs.GenerateK0sConfigScript()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to copy configure-k0s.sh to control plane node"))
			})

			It("fails when RunSSHCommand chmod fails", func() {
				fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(nil)

				nodeClient.EXPECT().CopyFile(bs.Env.ControlPlaneNodes[0], "configure-k0s.sh", "/root/configure-k0s.sh").Return(nil)
				nodeClient.EXPECT().RunCommand(bs.Env.ControlPlaneNodes[0], "root", "chmod +x /root/configure-k0s.sh").Return(fmt.Errorf("chmod error"))

				err := bs.GenerateK0sConfigScript()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to make configure-k0s.sh executable"))
			})
		})
	})

})

func fakeNode(name string, commandRunner node.NodeClient) *node.Node {
	return &node.Node{
		Name:       name,
		ExternalIP: "1.2.3.4",
		InternalIP: "10.0.0.1",

		NodeClient: commandRunner,
	}
}
