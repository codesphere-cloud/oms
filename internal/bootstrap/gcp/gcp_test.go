// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"context"
	"fmt"
	"os"
	"strings"

	"cloud.google.com/go/artifactregistry/apiv1/artifactregistrypb"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/github"
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

func jumpboxMatcher(node *node.Node) bool {
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
		mockGitHubClient *github.MockGitHubClient

		bs *gcp.GCPBootstrapper
	)

	JustBeforeEach(func() {
		var err error
		bs, err = gcp.NewGCPBootstrapper(
			ctx,
			e,
			stlog,
			csEnv,
			icg,
			gc,
			fw,
			nodeClient,
			mockPortalClient,
			util.NewFakeTime(),
			mockGitHubClient,
		)
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
		mockGitHubClient = github.NewMockGitHubClient(GinkgoT())
		stlog = bootstrap.NewStepLogger(false)

		csEnv = &gcp.CodesphereEnvironment{
			GitHubAppName:         "fake-app",
			GitHubAppClientID:     "fake-client-id",
			GitHubAppClientSecret: "fake-secret",
			InstallConfigPath:     "fake-config-file",
			SecretsFilePath:       "fake-secret",
			ProjectName:           "test-project",
			ProjectTTL:            "1h",
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
			FeatureFlags:          map[string]bool{},
			RootDiskSize:          50,
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
			CephNodes:         []*node.Node{fakeNode("ceph-1", nodeClient), fakeNode("ceph-2", nodeClient), fakeNode("ceph-3", nodeClient)},
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

			// EnsureInstallConfig
			fw.EXPECT().Exists("fake-config-file").Return(false)
			icg.EXPECT().ApplyProfile("minimal").Return(nil)
			// Returning a real install config to avoid nil pointer dereferences later
			icg.EXPECT().GetInstallConfig().RunAndReturn(func() *files.RootConfig {
				realIcm := installer.NewInstallConfigManager()
				_ = realIcm.ApplyProfile("minimal")
				return realIcm.GetInstallConfig()
			})

			projectId := "test-project-12345"

			// EnsureSecrets
			fw.EXPECT().Exists("fake-secret").Return(false)
			icg.EXPECT().GetVault().Return(&files.InstallVault{})

			// EnsureProject
			gc.EXPECT().GetProjectByName(mock.Anything, "test-project").Return(nil, fmt.Errorf("project not found: test-project"))
			gc.EXPECT().CreateProjectID("test-project").Return(projectId)
			gc.EXPECT().CreateProject(mock.Anything, mock.Anything, "test-project", mock.Anything).Return(mock.Anything, nil)

			// WriteInfraFile
			fw.EXPECT().MkdirAll(mock.Anything, os.FileMode(0755)).Return(nil)
			fw.EXPECT().WriteFile(mock.Anything, mock.Anything, os.FileMode(0644)).Return(nil)

			// EnsureBilling
			gc.EXPECT().GetBillingInfo(projectId).Return(&cloudbilling.ProjectBillingInfo{BillingEnabled: false}, nil)
			gc.EXPECT().EnableBilling(projectId, "test-billing-account").Return(nil)

			// EnsureAPIsEnabled
			gc.EXPECT().EnableAPIs(projectId, mock.Anything).Return(nil)

			// EnsureArtifactRegistry
			gc.EXPECT().GetArtifactRegistry(projectId, "us-central1", "codesphere-registry").Return(nil, fmt.Errorf("not found"))
			gc.EXPECT().CreateArtifactRegistry(projectId, "us-central1", "codesphere-registry").Return(&artifactregistrypb.Repository{Name: "codesphere-registry"}, nil)

			// EnsureServiceAccounts
			gc.EXPECT().CreateServiceAccount(projectId, "cloud-controller", "cloud-controller").Return("cloud-controller@p.iam.gserviceaccount.com", false, nil)
			gc.EXPECT().CreateServiceAccount(projectId, "artifact-registry-writer", "artifact-registry-writer").Return("writer@p.iam.gserviceaccount.com", true, nil)
			gc.EXPECT().CreateServiceAccountKey(projectId, "writer@p.iam.gserviceaccount.com").Return("fake-key", nil)

			// EnsureIAMRoles
			gc.EXPECT().AssignIAMRole(projectId, "artifact-registry-writer", projectId, []string{"roles/artifactregistry.writer"}).Return(nil)
			gc.EXPECT().AssignIAMRole(projectId, "cloud-controller", projectId, []string{"roles/compute.admin"}).Return(nil)
			gc.EXPECT().AssignIAMRole(csEnv.DNSProjectID, "cloud-controller", projectId, []string{"roles/dns.admin"}).Return(nil)

			// EnsureVPC
			gc.EXPECT().CreateVPC(projectId, "us-central1", projectId+"-vpc", projectId+"-us-central1-subnet", projectId+"-router", projectId+"-nat-gateway").Return(nil)

			// EnsureFirewallRules (5 times)
			gc.EXPECT().CreateFirewallRule(projectId, mock.Anything).Return(nil).Times(5)

			// EnsureComputeInstances
			ipResp := makeRunningInstance("10.0.0.1", "1.2.3.4")
			mockGetInstanceNotFoundThenRunning(gc, projectId, "us-central1-a", ipResp, 8)
			fw.EXPECT().ReadFile(mock.Anything).Return([]byte("fake-key"), nil).Times(8)
			gc.EXPECT().CreateInstance(projectId, "us-central1-a", mock.Anything).Return(nil).Times(8)

			// EnsureGatewayIPAddresses
			gc.EXPECT().GetAddress(projectId, "us-central1", "gateway").Return(nil, fmt.Errorf("not found"))
			gc.EXPECT().CreateAddress(projectId, "us-central1", mock.MatchedBy(func(addr *computepb.Address) bool { return *addr.Name == "gateway" })).Return("1.1.1.1", nil)
			gc.EXPECT().GetAddress(projectId, "us-central1", "gateway").Return(nil, fmt.Errorf("not found"))
			gc.EXPECT().GetAddress(projectId, "us-central1", "public-gateway").Return(nil, fmt.Errorf("not found"))
			gc.EXPECT().CreateAddress(projectId, "us-central1", mock.MatchedBy(func(addr *computepb.Address) bool { return *addr.Name == "public-gateway" })).Return("2.2.2.2", nil)
			gc.EXPECT().GetAddress(projectId, "us-central1", "public-gateway").Return(&computepb.Address{Address: protoString("2.2.2.2")}, nil)

			// UpdateInstallConfig
			icg.EXPECT().GenerateSecrets().Return(nil)
			icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
			nodeClient.EXPECT().CopyFile(mock.Anything, "fake-config-file", "/etc/codesphere/config.yaml").Return(nil)
			nodeClient.EXPECT().CopyFile(mock.Anything, "fake-secret", "/etc/codesphere/secrets/prod.vault.yaml").Return(nil)
			icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

			// Enable Root Login
			nodeClient.EXPECT().WaitReady(mock.Anything, mock.Anything).Return(nil).Return(nil)
			nodeClient.EXPECT().RunCommand(mock.Anything, mock.Anything, mock.Anything).Return(nil)

			// EnsureAgeKey
			nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root", "mkdir -p /etc/codesphere/secrets; age-keygen -o /etc/codesphere/secrets/age_key.txt").Return(nil)
			nodeClient.EXPECT().HasFile(mock.MatchedBy(jumpboxMatcher), "/etc/codesphere/secrets/age_key.txt").Return(false)

			// EncryptVault
			nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root", "cp /etc/codesphere/secrets/prod.vault.yaml{,.bak}").Return(nil)
			nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "sops --encrypt")
			})).Return(nil)

			// EnsureDNSRecords
			gc.EXPECT().EnsureDNSManagedZone(csEnv.DNSProjectID, "test-zone", "example.com.", mock.Anything).Return(nil)
			gc.EXPECT().EnsureDNSRecordSets(csEnv.DNSProjectID, "test-zone", mock.MatchedBy(func(records []*dns.ResourceRecordSet) bool {
				return len(records) == 4
			})).Return(nil)

			// GenerateK0sConfigScript
			fw.EXPECT().WriteFile("configure-k0s.sh", mock.Anything, os.FileMode(0755)).Return(nil)
			nodeClient.EXPECT().CopyFile(mock.Anything, "configure-k0s.sh", "/root/configure-k0s.sh").Return(nil)
			nodeClient.EXPECT().RunCommand(mock.Anything, "root", "chmod +x /root/configure-k0s.sh").Return(nil)

			err := bs.Bootstrap()
			Expect(err).NotTo(HaveOccurred())
			Expect(bs.Env).NotTo(BeNil())
			Expect(bs.Env.ProjectID).To(HavePrefix("test-project-"))

			// Verify nodes are properly set in the environment
			Expect(bs.Env.Jumpbox).NotTo(BeNil())
			Expect(bs.Env.PostgreSQLNode).NotTo(BeNil())
			Expect(bs.Env.CephNodes).To(HaveLen(3))
			Expect(bs.Env.ControlPlaneNodes).To(HaveLen(3))

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

			Expect(len(bs.Env.InstallConfig.Codesphere.ManagedServices)).To(Equal(5))
		})
	})

	Describe("ValidateInput", func() {
		var artifacts []portal.Artifact
		Context("When GitHub team and org is set", func() {
			BeforeEach(func() {
				csEnv.GitHubTeamOrg = "codesphere-cloud"
				csEnv.GitHubTeamSlug = "dev"
			})
			Context("when GitHub PAT is set", func() {
				BeforeEach(func() {
					csEnv.GitHubPAT = "pat"
				})
				It("passes validation", func() {
					err := bs.ValidateInput()
					Expect(err).NotTo(HaveOccurred())
				})

				Context("when GitHub arguments are partially set", func() {
					BeforeEach(func() {
						csEnv.GitHubTeamOrg = ""
					})
					It("fails", func() {
						err := bs.ValidateInput()
						Expect(err).To(MatchError(MatchRegexp("GitHub team parameters are not fully specified")))
					})
				})
			})

			Context("when GitHub PAT is not set", func() {
				BeforeEach(func() {
					csEnv.GitHubPAT = ""
				})
				It("fails", func() {
					err := bs.ValidateInput()
					Expect(err).To(MatchError(MatchRegexp("GitHub PAT is required to extract public keys of GitHub team members")))
				})
			})
		})
		Context("When a version and hash are specified", func() {
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

			Context("when GHCR registry is used", func() {
				BeforeEach(func() {
					csEnv.RegistryType = gcp.RegistryTypeGitHub
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

		Context("When a local package is specified", func() {
			BeforeEach(func() {
				csEnv.InstallLocal = "fake-installer-lite.tar.gz"
			})

			Context("when a version is also specified", func() {
				BeforeEach(func() {
					csEnv.InstallVersion = "v1.2.3"
				})
				It("fails", func() {
					err := bs.ValidateInput()
					Expect(err).To(MatchError(MatchRegexp("cannot specify both install-local and install-version/install-hash")))
				})
			})
			Context("when a hash is also specified", func() {
				BeforeEach(func() {
					csEnv.InstallHash = "abc123"
				})
				It("fails", func() {
					err := bs.ValidateInput()
					Expect(err).To(MatchError(MatchRegexp("cannot specify both install-local and install-version/install-hash")))
				})
			})
			Context("when no version or hash is specified", func() {
				Context("when the local file does not exist", func() {
					BeforeEach(func() {
						fw.EXPECT().Exists(csEnv.InstallLocal).Return(false)
					})
					It("fails", func() {
						err := bs.ValidateInput()
						Expect(err).To(MatchError(MatchRegexp("local installer package not found at path: " + csEnv.InstallLocal)))
					})
				})
				Context("when the local file exists", func() {
					BeforeEach(func() {
						fw.EXPECT().Exists(csEnv.InstallLocal).Return(true)
					})
					It("succeeds", func() {
						err := bs.ValidateInput()
						Expect(err).NotTo(HaveOccurred())
					})
				})
			})
		})
	})

	Describe("ValidateGitProviderParams", func() {
		Context("When GitLab client ID is set but secret is missing", func() {
			BeforeEach(func() {
				csEnv.GitLabAppClientID = "some-id"
				csEnv.GitLabAppClientSecret = ""
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("GitLab client ID is set but client secret is missing")))
			})
		})
		Context("When GitLab client secret is set but ID is missing", func() {
			BeforeEach(func() {
				csEnv.GitLabAppClientID = ""
				csEnv.GitLabAppClientSecret = "some-secret"
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("GitLab client secret is set but client ID is missing")))
			})
		})
		Context("When Bitbucket client ID is set but secret is missing", func() {
			BeforeEach(func() {
				csEnv.BitbucketAppClientID = "some-id"
				csEnv.BitbucketAppClientSecret = ""
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("Bitbucket client ID is set but client secret is missing")))
			})
		})
		Context("When Bitbucket client secret is set but ID is missing", func() {
			BeforeEach(func() {
				csEnv.BitbucketAppClientID = ""
				csEnv.BitbucketAppClientSecret = "some-secret"
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("Bitbucket client secret is set but client ID is missing")))
			})
		})
		Context("When Azure DevOps client ID is set but secret is missing", func() {
			BeforeEach(func() {
				csEnv.AzureDevOpsAppClientID = "some-id"
				csEnv.AzureDevOpsAppClientSecret = ""
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("Azure DevOps client ID is set but client secret is missing")))
			})
		})
		Context("When Azure DevOps client secret is set but ID is missing", func() {
			BeforeEach(func() {
				csEnv.AzureDevOpsAppClientID = ""
				csEnv.AzureDevOpsAppClientSecret = "some-secret"
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("Azure DevOps client secret is set but client ID is missing")))
			})
		})
		Context("When all providers have both ID and secret set", func() {
			BeforeEach(func() {
				csEnv.GitLabAppClientID = "gl-id"
				csEnv.GitLabAppClientSecret = "gl-secret"
				csEnv.BitbucketAppClientID = "bb-id"
				csEnv.BitbucketAppClientSecret = "bb-secret"
				csEnv.AzureDevOpsAppClientID = "az-id"
				csEnv.AzureDevOpsAppClientSecret = "az-secret"
			})
			It("succeeds", func() {
				err := bs.ValidateInput()
				Expect(err).NotTo(HaveOccurred())
			})
		})
		Context("When no provider credentials are set", func() {
			It("succeeds", func() {
				err := bs.ValidateInput()
				Expect(err).NotTo(HaveOccurred())
			})
		})
		Context("When OIDC issuer URL is set but client ID and secret are missing", func() {
			BeforeEach(func() {
				csEnv.OidcIssuerURL = "https://idp.example.com"
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("OIDC OAuth provider credentials are not fully specified")))
			})
		})
		Context("When OIDC client ID and secret are set but issuer URL is missing", func() {
			BeforeEach(func() {
				csEnv.OidcClientID = "oidc-id"
				csEnv.OidcClientSecret = "oidc-secret"
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("OIDC OAuth provider credentials are not fully specified")))
			})
		})
		Context("When only OIDC client ID is set", func() {
			BeforeEach(func() {
				csEnv.OidcClientID = "oidc-id"
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("OIDC OAuth provider credentials are not fully specified")))
			})
		})
		Context("When all OIDC params are set", func() {
			BeforeEach(func() {
				csEnv.OidcIssuerURL = "https://idp.example.com"
				csEnv.OidcClientID = "oidc-id"
				csEnv.OidcClientSecret = "oidc-secret"
			})
			It("succeeds", func() {
				err := bs.ValidateInput()
				Expect(err).NotTo(HaveOccurred())
			})
		})
		Context("When external Loki endpoint is set", func() {
			BeforeEach(func() {
				csEnv.ExternalLokiEndpoint = "https://loki.example.com/loki/api/v1/push"
				csEnv.ExternalLokiSecret = "loki-password"
				csEnv.ExternalLokiUser = "loki-user"
			})
			It("succeeds", func() {
				err := bs.ValidateInput()
				Expect(err).NotTo(HaveOccurred())
			})
		})
		Context("When external Loki secret is set without endpoint", func() {
			BeforeEach(func() {
				csEnv.ExternalLokiSecret = "loki-password"
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("external Loki endpoint is required")))
			})
		})
		Context("When external Loki user is set without endpoint", func() {
			BeforeEach(func() {
				csEnv.ExternalLokiUser = "loki-user"
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("external Loki endpoint is required")))
			})
		})

		Context("When Prometheus remote write is fully configured", func() {
			BeforeEach(func() {
				csEnv.PrometheusRemoteWriteURL = "https://prometheus.example.com/api/v1/write"
				csEnv.PrometheusRemoteWriteUser = "prom-user"
				csEnv.PrometheusRemoteWritePassword = "prom-password"
			})
			It("succeeds", func() {
				err := bs.ValidateInput()
				Expect(err).NotTo(HaveOccurred())
			})
		})
		Context("When Prometheus remote write URL is set but credentials are missing", func() {
			BeforeEach(func() {
				csEnv.PrometheusRemoteWriteURL = "https://prometheus.example.com/api/v1/write"
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("prometheus remote write username and password must both be set when remote write URL is specified")))
			})
		})
		Context("When Prometheus remote write URL is set but only username is missing", func() {
			BeforeEach(func() {
				csEnv.PrometheusRemoteWriteURL = "https://prometheus.example.com/api/v1/write"
				csEnv.PrometheusRemoteWritePassword = "prom-password"
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("prometheus remote write username and password must both be set when remote write URL is specified")))
			})
		})
		Context("When Prometheus remote write URL is set but only password is missing", func() {
			BeforeEach(func() {
				csEnv.PrometheusRemoteWriteURL = "https://prometheus.example.com/api/v1/write"
				csEnv.PrometheusRemoteWriteUser = "prom-user"
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("prometheus remote write username and password must both be set when remote write URL is specified")))
			})
		})
		Context("When Prometheus remote write credentials are set but URL is missing", func() {
			BeforeEach(func() {
				csEnv.PrometheusRemoteWriteUser = "prom-user"
				csEnv.PrometheusRemoteWritePassword = "prom-password"
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("prometheus remote write URL is required when remote write username or password is set")))
			})
		})

		Context("When central OTel endpoint is set but password is missing", func() {
			BeforeEach(func() {
				csEnv.CentralOtelEndpoint = "https://otel.example.com"
				csEnv.CentralOtelUsername = "otel-user"
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("central OTel password is required when central OTel endpoint is set")))
			})
		})

		Context("When central OTel username is set but password is missing", func() {
			BeforeEach(func() {
				csEnv.CentralOtelUsername = "otel-user"
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("central OTel username is set but password is missing")))
			})
		})

		Context("When central OTel password is set but username is missing", func() {
			BeforeEach(func() {
				csEnv.CentralOtelPassword = "otel-secret"
				csEnv.CentralOtelEndpoint = "https://otel.example.com"
			})
			It("returns an error", func() {
				err := bs.ValidateInput()
				Expect(err).To(MatchError(ContainSubstring("central OTel password is set but username is missing")))
			})
		})
	})

	Describe("EnsureInstallConfig", func() {
		Describe("Valid EnsureInstallConfig", func() {
			BeforeEach(func() {
			})
			It("uses existing when config file exists", func() {
				fw.EXPECT().Exists(csEnv.InstallConfigPath).Return(true)
				fw.EXPECT().Exists(csEnv.SecretsFilePath).Return(false)
				icg.EXPECT().LoadInstallConfigFromFile(csEnv.InstallConfigPath).Return(nil)
				icg.EXPECT().GetInstallConfig().Return(&files.RootConfig{})

				err := bs.EnsureInstallConfig()
				Expect(err).NotTo(HaveOccurred())
			})

			It("loads existing vault before existing config for templating", func() {
				fw.EXPECT().Exists(csEnv.InstallConfigPath).Return(true)
				fw.EXPECT().Exists(csEnv.SecretsFilePath).Return(true)
				icg.EXPECT().LoadVaultFromFile(csEnv.SecretsFilePath).Return(nil)
				icg.EXPECT().LoadInstallConfigFromFile(csEnv.InstallConfigPath).Return(nil)
				icg.EXPECT().GetInstallConfig().Return(&files.RootConfig{})

				err := bs.EnsureInstallConfig()
				Expect(err).NotTo(HaveOccurred())
			})

			It("creates install config when missing", func() {
				fw.EXPECT().Exists(csEnv.InstallConfigPath).Return(false)
				icg.EXPECT().ApplyProfile("minimal").Return(nil)
				icg.EXPECT().GetInstallConfig().Return(&files.RootConfig{})

				err := bs.EnsureInstallConfig()
				Expect(err).NotTo(HaveOccurred())
				Expect(bs.Env.InstallConfig).NotTo(BeNil())
			})
		})

		Describe("Invalid cases", func() {
			It("returns error when config file exists but fails to load", func() {
				fw.EXPECT().Exists(csEnv.InstallConfigPath).Return(true)
				fw.EXPECT().Exists(csEnv.SecretsFilePath).Return(false)
				icg.EXPECT().LoadInstallConfigFromFile(csEnv.InstallConfigPath).Return(fmt.Errorf("bad format"))

				err := bs.EnsureInstallConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to load config file"))
				Expect(err.Error()).To(ContainSubstring("bad format"))
			})

			It("returns error when config file missing and applying profile fails", func() {
				fw.EXPECT().Exists(csEnv.InstallConfigPath).Return(false)
				icg.EXPECT().ApplyProfile("minimal").Return(fmt.Errorf("profile error"))

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
				vault := &files.InstallVault{}
				icg.EXPECT().GetVault().Return(vault)

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
				Expect(vault.GetSecret(files.SecretRegistryUsername).Fields.Password).To(Equal("custom-registry"))
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
				icg.EXPECT().GetVault().Return(&files.InstallVault{})
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
			vault := &files.InstallVault{}
			icg.EXPECT().GetVault().Return(vault)

			err := bs.EnsureGitHubAccessConfigured()
			Expect(err).NotTo(HaveOccurred())
			Expect(bs.Env.InstallConfig.Registry.Server).To(Equal("ghcr.io"))
			Expect(vault.GetSecret(files.SecretRegistryUsername).Fields.Password).To(Equal(csEnv.RegistryUser))
			Expect(vault.GetSecret(files.SecretRegistryPassword).Fields.Password).To(Equal(csEnv.GitHubPAT))
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
					// HasInotifyWatchesConfigured: all 4 checks pass → skip ConfigureInotifyWatches
					nodeClient.EXPECT().RunCommand(mock.Anything, "root", "sudo grep -E '^fs.inotify.max_user_watches=1048576' /etc/sysctl.conf >/dev/null 2>&1").Return(nil).Once(),
					nodeClient.EXPECT().RunCommand(mock.Anything, "root", "sudo sysctl -n fs.inotify.max_user_watches | grep -q '^1048576$'").Return(nil).Once(),
					nodeClient.EXPECT().RunCommand(mock.Anything, "root", "sudo grep -E '^fs.inotify.max_user_instances=8192' /etc/sysctl.conf >/dev/null 2>&1").Return(nil).Once(),
					nodeClient.EXPECT().RunCommand(mock.Anything, "root", "sudo sysctl -n fs.inotify.max_user_instances | grep -q '^8192$'").Return(nil).Once(),

					// HasMemoryMapConfigured: line not found → returns false → call ConfigureMemoryMap
					nodeClient.EXPECT().RunCommand(mock.Anything, "root", "sudo grep -E '^vm.max_map_count=262144' /etc/sysctl.conf >/dev/null 2>&1").Return(fmt.Errorf("not found")).Once(),

					// ConfigureMemoryMap → configureSysctlLine: line not found → tee fails
					nodeClient.EXPECT().RunCommand(mock.Anything, "root", "sudo grep -E '^vm.max_map_count=262144' /etc/sysctl.conf >/dev/null 2>&1").Return(fmt.Errorf("not found")).Once(),
					nodeClient.EXPECT().RunCommand(mock.Anything, "root", "echo 'vm.max_map_count=262144' | sudo tee -a /etc/sysctl.conf").Return(fmt.Errorf("ouch")).Once(),
				)

				err := bs.EnsureHostsConfigured()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to configure memory map"))
			})
		})
	})

	Describe("EnsureAgeKey", func() {
		Describe("Valid EnsureAgeKey", func() {
			It("generates key if missing", func() {
				nodeClient.EXPECT().HasFile(mock.MatchedBy(jumpboxMatcher), "/etc/codesphere/secrets/age_key.txt").Return(false)
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root", "mkdir -p /etc/codesphere/secrets; age-keygen -o /etc/codesphere/secrets/age_key.txt").Return(nil)

				err := bs.EnsureAgeKey()
				Expect(err).NotTo(HaveOccurred())
			})

			It("skips if key exists", func() {
				nodeClient.EXPECT().HasFile(mock.MatchedBy(jumpboxMatcher), "/etc/codesphere/secrets/age_key.txt").Return(true)

				err := bs.EnsureAgeKey()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Invalid cases", func() {
			It("fails when age-keygen command fails", func() {
				nodeClient.EXPECT().HasFile(mock.MatchedBy(jumpboxMatcher), "/etc/codesphere/secrets/age_key.txt").Return(false)
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root", "mkdir -p /etc/codesphere/secrets; age-keygen -o /etc/codesphere/secrets/age_key.txt").Return(fmt.Errorf("ouch"))

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
			icg.EXPECT().GetSecretFilePath().Return("/etc/codesphere/secrets/prod.vault.yaml").Maybe()
		})
		Describe("Valid InstallCodesphere", func() {
			Context("Direct GitHub access", func() {
				BeforeEach(func() {
					csEnv.GitHubPAT = "fake-pat"
					csEnv.RegistryUser = "fake-user"
					csEnv.RegistryType = "github"
				})
				It("downloads and installs lite package", func() {
					// Expect download package
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root", "oms download package -f installer-lite.tar.gz -H abc1234567890 v1.2.3").Return(nil)

					// Expect install codesphere
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root",
						"oms install codesphere -c /etc/codesphere/config.yaml -k /etc/codesphere/secrets/age_key.txt --vault /etc/codesphere/secrets/prod.vault.yaml -p v1.2.3-abc1234567890-installer-lite.tar.gz -s load-container-images").Return(nil)

					err := bs.InstallCodesphere()
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("without explicit hash", func() {
				BeforeEach(func() {
					// Simulate that ValidateInput has populated the hash
					csEnv.InstallHash = "def9876543210"
				})
				It("downloads and installs codesphere", func() {
					// Expect download package
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root", "oms download package -f installer.tar.gz -H def9876543210 v1.2.3").Return(nil)

					// Expect install codesphere
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root", "oms install codesphere -c /etc/codesphere/config.yaml -k /etc/codesphere/secrets/age_key.txt --vault /etc/codesphere/secrets/prod.vault.yaml -p v1.2.3-def9876543210-installer.tar.gz").Return(nil)

					err := bs.InstallCodesphere()
					Expect(err).NotTo(HaveOccurred())
				})
			})

			It("downloads and installs codesphere with hash", func() {
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root", "oms download package -f installer.tar.gz -H abc1234567890 v1.2.3").Return(nil)
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root", "oms install codesphere -c /etc/codesphere/config.yaml -k /etc/codesphere/secrets/age_key.txt --vault /etc/codesphere/secrets/prod.vault.yaml -p v1.2.3-abc1234567890-installer.tar.gz").Return(nil)

				err := bs.InstallCodesphere()
				Expect(err).NotTo(HaveOccurred())
			})

			Context("LTS 1.77.2", func() {
				BeforeEach(func() {
					csEnv.InstallVersion = "codesphere-lts-v1.77.2"
				})
				JustBeforeEach(func() {
					// Inject a stub binary builder so tests don't invoke `go build`.
					bs.OmsBinaryBuilder = func() (string, func(), error) {
						f, err := os.CreateTemp("", "oms-test-binary-*")
						Expect(err).NotTo(HaveOccurred())
						Expect(f.Close()).To(Succeed())
						return f.Name(), func() { Expect(os.Remove(f.Name())).To(Succeed()) }, nil
					}
					// Create a fake SSH private key file for the jumpbox key copy.
					sshKeyFile, err := os.CreateTemp("", "oms-test-ssh-key-*")
					Expect(err).NotTo(HaveOccurred())
					_, err = sshKeyFile.WriteString("fake-ssh-private-key")
					Expect(err).NotTo(HaveOccurred())
					Expect(sshKeyFile.Close()).To(Succeed())
					csEnv.SSHPrivateKeyPath = sshKeyFile.Name()
					DeferCleanup(func() { Expect(os.Remove(sshKeyFile.Name())).To(Succeed()) })
				})
				It("downloads package, updates OMS binary, and installs codesphere", func() {
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root",
						"oms download package -f installer.tar.gz -H abc1234567890 codesphere-lts-v1.77.2").Return(nil)
					nodeClient.EXPECT().CopyFile(mock.MatchedBy(jumpboxMatcher), mock.Anything, "/tmp/oms-new").Return(nil)
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root",
						"chmod +x /tmp/oms-new && mv /tmp/oms-new /usr/local/bin/oms").Return(nil)
					// Phase 1: Infra (skip codesphere + SSH-needing steps).
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root",
						"oms install codesphere infra -c /etc/codesphere/config.yaml -k /etc/codesphere/secrets/age_key.txt --vault /etc/codesphere/secrets/prod.vault.yaml -p codesphere-lts-v1.77.2-abc1234567890-installer.tar.gz -s codesphere,set-up-cluster,ms-backends,argocd").Return(nil)
					// Phase 2: Dependencies (skip SSH-needing steps).
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root",
						"oms install codesphere dependencies -c /etc/codesphere/config.yaml -k /etc/codesphere/secrets/age_key.txt --vault /etc/codesphere/secrets/prod.vault.yaml -p codesphere-lts-v1.77.2-abc1234567890-installer.tar.gz -s set-up-cluster,ms-backends,argocd").Return(nil)
					// SSH key setup for platform phase.
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root",
						"mkdir -p /root/.ssh && chmod 700 /root/.ssh").Return(nil)
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root",
						"cat > /root/.ssh/id_rsa << 'OMSEOF'\nfake-ssh-private-key\nOMSEOF\nchmod 600 /root/.ssh/id_rsa").Return(nil)
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root",
						"cat > /root/.ssh/config << 'OMSEOF'\nHost *\n  IdentityFile /root/.ssh/id_rsa\n  StrictHostKeyChecking no\n  UserKnownHostsFile /dev/null\nOMSEOF\nchmod 600 /root/.ssh/config").Return(nil)
					// Phase 3: Platform (codesphere runs with SSH).
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root",
						"oms install codesphere platform -c /etc/codesphere/config.yaml -k /etc/codesphere/secrets/age_key.txt --vault /etc/codesphere/secrets/prod.vault.yaml -p codesphere-lts-v1.77.2-abc1234567890-installer.tar.gz -s set-up-cluster,ms-backends,argocd").Return(nil)

					err := bs.InstallCodesphere()
					Expect(err).NotTo(HaveOccurred())
				})

				It("fails when OmsBinaryBuilder fails", func() {
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root",
						"oms download package -f installer.tar.gz -H abc1234567890 codesphere-lts-v1.77.2").Return(nil)
					bs.OmsBinaryBuilder = func() (string, func(), error) {
						return "", func() {}, fmt.Errorf("build failed")
					}

					err := bs.InstallCodesphere()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to update OMS binary on jumpbox for codesphere-lts-v1.77.2"))
				})

				It("fails when copying binary to jumpbox fails", func() {
					nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root",
						"oms download package -f installer.tar.gz -H abc1234567890 codesphere-lts-v1.77.2").Return(nil)
					nodeClient.EXPECT().CopyFile(mock.MatchedBy(jumpboxMatcher), mock.Anything, "/tmp/oms-new").Return(fmt.Errorf("copy failed"))

					err := bs.InstallCodesphere()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to update OMS binary on jumpbox for codesphere-lts-v1.77.2"))
				})
			})

			Context("with local package", func() {
				BeforeEach(func() {
					csEnv.InstallLocal = "fake-installer-lite.tar.gz"
					csEnv.InstallVersion = ""
					csEnv.InstallHash = ""
				})
				Context("using the github registry", func() {
					BeforeEach(func() {
						csEnv.RegistryType = gcp.RegistryTypeGitHub
					})
					It("installs codesphere from local package", func() {
						nodeClient.EXPECT().CopyFile(mock.Anything, csEnv.InstallLocal, "/root/local-installer-lite.tar.gz").Return(nil)
						nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root",
							"oms install codesphere -c /etc/codesphere/config.yaml -k /etc/codesphere/secrets/age_key.txt --vault /etc/codesphere/secrets/prod.vault.yaml -p local-installer-lite.tar.gz -s load-container-images").Return(nil)

						err := bs.InstallCodesphere()
						Expect(err).NotTo(HaveOccurred())
					})
				})
				Context("using the local registry", func() {
					BeforeEach(func() {
						csEnv.RegistryType = gcp.RegistryTypeLocalContainer
						csEnv.InstallLocal = "fake-installer-lite.tar.gz"
					})
					It("installs codesphere from local package", func() {
						nodeClient.EXPECT().CopyFile(mock.Anything, csEnv.InstallLocal, "/root/local-installer.tar.gz").Return(nil)
						nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root",
							"oms install codesphere -c /etc/codesphere/config.yaml -k /etc/codesphere/secrets/age_key.txt --vault /etc/codesphere/secrets/prod.vault.yaml -p local-installer.tar.gz").Return(nil)

						err := bs.InstallCodesphere()
						Expect(err).NotTo(HaveOccurred())
					})
				})
			})
		})

		Describe("Invalid cases", func() {
			Context("without explicit hash", func() {
				BeforeEach(func() {
					// Simulate that ValidateInput has not populated the hash
					csEnv.InstallHash = ""
				})
				It("fails", func() {
					err := bs.InstallCodesphere()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("install hash must be set when install version is set"))
				})
			})

			Context("neither local nor install version specified", func() {
				BeforeEach(func() {
					csEnv.InstallVersion = ""
					csEnv.InstallHash = ""
				})
				It("fails", func() {
					err := bs.InstallCodesphere()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("either install version or a local package must be specified"))
				})
			})

			It("fails when download package fails", func() {
				downloadCmd := "oms download package -f installer.tar.gz -H abc1234567890 v1.2.3"
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root", downloadCmd).Return(fmt.Errorf("download error")).Once()

				err := bs.InstallCodesphere()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to download Codesphere package from jumpbox"))
			})

			It("fails when install codesphere fails", func() {
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root", "oms download package -f installer.tar.gz -H abc1234567890 v1.2.3").Return(nil).Once()
				nodeClient.EXPECT().RunCommand(mock.MatchedBy(jumpboxMatcher), "root", "oms install codesphere -c /etc/codesphere/config.yaml -k /etc/codesphere/secrets/age_key.txt --vault /etc/codesphere/secrets/prod.vault.yaml -p v1.2.3-abc1234567890-installer.tar.gz").Return(fmt.Errorf("install error")).Once()

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
