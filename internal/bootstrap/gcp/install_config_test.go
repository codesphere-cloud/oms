// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/resourcemanager/apiv3/resourcemanagerpb"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/github"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

var _ = Describe("Installconfig & Secrets", func() {
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
			RecoverConfig:         false,
			ProjectName:           "test-project",
			ProjectTTL:            "1h",
			SecretsDir:            "/etc/codesphere/secrets",
			BillingAccount:        "test-billing-account",
			Region:                "us-central1",
			Zone:                  "us-central1-a",
			DatacenterID:          1,
			DatacenterName:        "dev",
			BaseDomain:            "example.com",
			DNSProjectID:          "dns-project",
			DNSZoneName:           "test-zone",
			SSHPublicKeyPath:      "key.pub",
			ProjectID:             "pid",
			InternalFlags:         gcp.DefaultInternalFlags,
			PreviewFlags:          gcp.DefaultPreviewFlags,
			FeatureFlags:          gcp.DefaultFeatureFlags,
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

			Describe("Config Recovery from Jumpbox", func() {
				JustBeforeEach(func() {
					csEnv.RecoverConfig = true
					gc.EXPECT().GetProjectByName(mock.Anything, mock.Anything).Return(&resourcemanagerpb.Project{ProjectId: csEnv.ProjectID, Name: "existing-proj"}, nil)

					runningResp := makeRunningInstance("10.0.0.x", "1.2.3.x")
					gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, "jumpbox").Return(runningResp, nil)

					nodeClient.EXPECT().DownloadFile(mock.Anything, mock.Anything, mock.Anything).Return(nil)
					nodeClient.EXPECT().RunCommand(mock.Anything, mock.Anything, mock.Anything).Return(nil)
				})

				It("overwrites an existing config", func() {
					fw.EXPECT().Exists(csEnv.InstallConfigPath).Return(true)
					fw.EXPECT().Exists(csEnv.SecretsFilePath).Return(false)
					icg.EXPECT().LoadInstallConfigFromFile(csEnv.InstallConfigPath).Return(nil)
					icg.EXPECT().GetInstallConfig().Return(&files.RootConfig{})

					err := bs.EnsureInstallConfig()
					Expect(err).ToNot(HaveOccurred())
				})
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

			Describe("returns an error when config recovery fails", func() {
				JustBeforeEach(func() {
					csEnv.RecoverConfig = true
				})

				It("return an error when project for recovery is not found", func() {
					gc.EXPECT().GetProjectByName(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("project not found"))

					err := bs.EnsureInstallConfig()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to find gcp project for config recovery"))
					Expect(err.Error()).To(ContainSubstring("project not found"))
				})

				It("return an error when jumpbox for recovery is not found", func() {
					gc.EXPECT().GetProjectByName(mock.Anything, mock.Anything).Return(&resourcemanagerpb.Project{ProjectId: csEnv.ProjectID, Name: "existing-proj"}, nil)
					gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, "jumpbox").Return(nil, grpcstatus.Errorf(codes.NotFound, "not found"))

					err := bs.EnsureInstallConfig()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to find jumpbox node for config recovery"))
					Expect(err.Error()).To(ContainSubstring("not found"))
				})

				It("return an error when config download fails from the jumpbox for recovery", func() {
					gc.EXPECT().GetProjectByName(mock.Anything, mock.Anything).Return(&resourcemanagerpb.Project{ProjectId: csEnv.ProjectID, Name: "existing-proj"}, nil)

					runningResp := makeRunningInstance("10.0.0.x", "1.2.3.x")
					gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, "jumpbox").Return(runningResp, nil)

					nodeClient.EXPECT().DownloadFile(mock.Anything, mock.Anything, csEnv.InstallConfigPath).Return(fmt.Errorf("failed"))

					err := bs.EnsureInstallConfig()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to download install config from jumpbox"))
				})

				It("return an error when decrypting the secrets on the jumpbox for recovery", func() {
					gc.EXPECT().GetProjectByName(mock.Anything, mock.Anything).Return(&resourcemanagerpb.Project{ProjectId: csEnv.ProjectID, Name: "existing-proj"}, nil)

					runningResp := makeRunningInstance("10.0.0.x", "1.2.3.x")
					gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, "jumpbox").Return(runningResp, nil)

					nodeClient.EXPECT().DownloadFile(mock.Anything, mock.Anything, csEnv.InstallConfigPath).Return(nil)
					nodeClient.EXPECT().RunCommand(mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("failed"))

					err := bs.EnsureInstallConfig()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to create decrypted vault for recovery"))
				})

				It("return an error when secrets download fails from the jumpbox for recovery", func() {
					gc.EXPECT().GetProjectByName(mock.Anything, mock.Anything).Return(&resourcemanagerpb.Project{ProjectId: csEnv.ProjectID, Name: "existing-proj"}, nil)

					runningResp := makeRunningInstance("10.0.0.x", "1.2.3.x")
					gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, "jumpbox").Return(runningResp, nil)

					nodeClient.EXPECT().DownloadFile(mock.Anything, mock.Anything, csEnv.InstallConfigPath).Return(nil)
					nodeClient.EXPECT().RunCommand(mock.Anything, mock.Anything, mock.Anything).Return(nil)
					nodeClient.EXPECT().DownloadFile(mock.Anything, mock.Anything, csEnv.SecretsFilePath).Return(fmt.Errorf("failed"))

					err := bs.EnsureInstallConfig()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to download secrets file from jumpbox"))
				})

				It("returns error when recovery is successful, but config file fails to load", func() {
					gc.EXPECT().GetProjectByName(mock.Anything, mock.Anything).Return(&resourcemanagerpb.Project{ProjectId: csEnv.ProjectID, Name: "existing-proj"}, nil)

					runningResp := makeRunningInstance("10.0.0.x", "1.2.3.x")
					gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, "jumpbox").Return(runningResp, nil)

					nodeClient.EXPECT().DownloadFile(mock.Anything, mock.Anything, mock.Anything).Return(nil)
					nodeClient.EXPECT().RunCommand(mock.Anything, mock.Anything, mock.Anything).Return(nil)

					fw.EXPECT().Exists(csEnv.InstallConfigPath).Return(true)
					fw.EXPECT().Exists(csEnv.SecretsFilePath).Return(false)
					icg.EXPECT().LoadInstallConfigFromFile(csEnv.InstallConfigPath).Return(fmt.Errorf("bad format"))

					err := bs.EnsureInstallConfig()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to load config file"))
					Expect(err.Error()).To(ContainSubstring("bad format"))
				})
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

	Describe("UpdateInstallConfig", func() {
		var vault *files.InstallVault
		BeforeEach(func() {
			vault = &files.InstallVault{}
			icg.EXPECT().GetVault().Return(vault).Maybe()
			csEnv.GitHubAppName = "fake-app-name"
		})
		Describe("Valid UpdateInstallConfig", func() {
			It("updates config and writes files", func() {
				csEnv.SshProxyIP = "3.3.3.3"

				icg.EXPECT().GenerateSecrets().Return(nil)
				icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
				icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

				nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

				err := bs.UpdateInstallConfig()
				Expect(err).NotTo(HaveOccurred())

				applications := bs.Env.InstallConfig.PcApps["applications"].(map[string]interface{})
				sshProxy := applications["ssh-workspace-proxy"].(map[string]interface{})
				Expect(sshProxy["enabled"]).To(Equal(true))
				sshProxyValues := sshProxy["valuesObject"].(map[string]interface{})
				sshProxyService := sshProxyValues["service"].(map[string]interface{})
				Expect(sshProxyService["enabled"]).To(Equal(true))
				Expect(sshProxyService["type"]).To(Equal("LoadBalancer"))
				Expect(sshProxyService["loadBalancerIP"]).To(Equal("3.3.3.3"))
				sshProxyAnnotations := sshProxyService["annotations"].(map[string]interface{})
				Expect(sshProxyAnnotations["cloud.google.com/load-balancer-ipv4"]).To(Equal("3.3.3.3"))

				Expect(bs.Env.InstallConfig.Datacenter.ID).To(Equal(1))
				Expect(bs.Env.InstallConfig.Datacenter.Name).To(Equal("dev"))
				Expect(bs.Env.InstallConfig.Codesphere.Domain).To(Equal("cs.example.com"))
				Expect(bs.Env.InstallConfig.Codesphere.Features).To(Equal(map[string]bool{}))
				Expect(bs.Env.InstallConfig.Codesphere.Internal).To(Equal(gcp.DefaultInternalFlags))
				Expect(bs.Env.InstallConfig.Codesphere.Preview).To(Equal(util.StringSliceToBoolMap(gcp.DefaultPreviewFlags)))

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

				Expect(bs.Env.InstallConfig.Codesphere.OpenBao).To(BeNil())
			})
			It("uses the configured datacenter name", func() {
				csEnv.DatacenterName = "staging"

				icg.EXPECT().GenerateSecrets().Return(nil)
				icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
				icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

				nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

				err := bs.UpdateInstallConfig()
				Expect(err).NotTo(HaveOccurred())

				Expect(bs.Env.InstallConfig.Datacenter.Name).To(Equal("staging"))
			})
			Context("When internal flags are set in CodesphereEnvironment", func() {
				BeforeEach(func() {
					csEnv.InternalFlags = []string{"fake-exp1", "fake-exp2"}
				})
				It("uses those internal flags instead of defaults", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.Internal).To(Equal(csEnv.InternalFlags))
				})
			})
			Context("When preview flags are set in CodesphereEnvironment", func() {
				BeforeEach(func() {
					csEnv.PreviewFlags = []string{"fake-preview1", "fake-preview2"}
				})
				It("uses those preview flags instead of defaults", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.Preview).To(Equal(util.StringSliceToBoolMap(csEnv.PreviewFlags)))
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

					Expect(bs.Env.InstallConfig.Codesphere.Features).To(Equal(util.StringSliceToBoolMap(csEnv.FeatureFlags)))
				})
			})
			Context("When cluster admin email is set", func() {
				BeforeEach(func() {
					csEnv.ClusterAdminEmail = "admin@codesphere.com"
				})
				It("writes the email to the install config", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.ClusterAdminEmail).To(Equal("admin@codesphere.com"))
				})
			})
			Context("When cluster admin email is not set", func() {
				BeforeEach(func() {
					csEnv.InstallConfig.Codesphere.ClusterAdminEmail = "existing@codesphere.com"
				})
				It("keeps the value of an existing config", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.ClusterAdminEmail).To(Equal("existing@codesphere.com"))
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

			Context("When GitLab credentials are set", func() {
				BeforeEach(func() {
					csEnv.GitLabAppClientID = "gitlab-app-client-id"
					csEnv.GitLabAppClientSecret = "gitlab-app-client-secret"
				})
				It("sets GitLab OAuth configuration", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.GitLab).NotTo(BeNil())
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.GitLab.Enabled).To(BeTrue())
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.GitLab.URL).To(Equal("https://gitlab.com"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.GitLab.API.BaseURL).To(Equal("https://gitlab.com"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.GitLab.OAuth.Issuer).To(Equal("https://gitlab.com"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.GitLab.OAuth.AuthorizationEndpoint).To(Equal("https://gitlab.com/oauth/authorize"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.GitLab.OAuth.TokenEndpoint).To(Equal("https://gitlab.com/oauth/token"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.GitLab.OAuth.ClientAuthMethod).To(Equal("client_secret_post"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.GitLab.OAuth.RedirectURI).To(Equal("https://cs.example.com/ide/auth/gitlab/callback"))
					Expect(vault.GetSecret(files.SecretGitlabAppClientId).Fields.Password).To(Equal("gitlab-app-client-id"))
					Expect(vault.GetSecret(files.SecretGitlabAppClientSecret).Fields.Password).To(Equal("gitlab-app-client-secret"))
				})
			})

			Context("When GitLab credentials are not set", func() {
				It("skips setting GitLab OAuth configuration", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.GitLab).To(BeNil())
				})
			})

			Context("When Bitbucket credentials are set", func() {
				BeforeEach(func() {
					csEnv.BitbucketAppClientID = "bitbucket-app-client-id"
					csEnv.BitbucketAppClientSecret = "bitbucket-app-client-secret"
				})
				It("sets Bitbucket OAuth configuration", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.Bitbucket).NotTo(BeNil())
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.Bitbucket.Enabled).To(BeTrue())
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.Bitbucket.URL).To(Equal("https://bitbucket.org"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.Bitbucket.API.BaseURL).To(Equal("https://api.bitbucket.org/2.0"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.Bitbucket.OAuth.Issuer).To(Equal("https://bitbucket.org"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.Bitbucket.OAuth.AuthorizationEndpoint).To(Equal("https://bitbucket.org/site/oauth2/authorize"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.Bitbucket.OAuth.TokenEndpoint).To(Equal("https://bitbucket.org/site/oauth2/access_token"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.Bitbucket.OAuth.ClientAuthMethod).To(Equal("client_secret_post"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.Bitbucket.OAuth.RedirectURI).To(Equal("https://cs.example.com/ide/auth/bitbucket/callback"))
					Expect(vault.GetSecret(files.SecretBitbucketAppsClientId).Fields.Password).To(Equal("bitbucket-app-client-id"))
					Expect(vault.GetSecret(files.SecretBitbucketAppsClientSecret).Fields.Password).To(Equal("bitbucket-app-client-secret"))
				})
			})

			Context("When Bitbucket credentials are not set", func() {
				It("skips setting Bitbucket OAuth configuration", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.Bitbucket).To(BeNil())
				})
			})

			Context("When Azure DevOps credentials are set", func() {
				BeforeEach(func() {
					csEnv.AzureDevOpsAppClientID = "azure-devops-app-client-id"
					csEnv.AzureDevOpsAppClientSecret = "azure-devops-app-client-secret"
				})
				It("sets Azure DevOps OAuth configuration", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.AzureDevOps).NotTo(BeNil())
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.AzureDevOps.Enabled).To(BeTrue())
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.AzureDevOps.URL).To(Equal("https://dev.azure.com"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.AzureDevOps.API.BaseURL).To(Equal("https://dev.azure.com"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.AzureDevOps.OAuth.Issuer).To(Equal("https://login.microsoftonline.com/common/v2.0"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.AzureDevOps.OAuth.AuthorizationEndpoint).To(Equal("https://login.microsoftonline.com/common/oauth2/v2.0/authorize"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.AzureDevOps.OAuth.TokenEndpoint).To(Equal("https://login.microsoftonline.com/common/oauth2/v2.0/token"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.AzureDevOps.OAuth.ClientAuthMethod).To(Equal("client_secret_post"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.AzureDevOps.OAuth.Scope).To(Equal("openid offline_access https://app.vssps.visualstudio.com/vso.code_full"))
					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.AzureDevOps.OAuth.RedirectURI).To(Equal("https://cs.example.com/ide/auth/azure-dev-ops/callback"))
					Expect(vault.GetSecret(files.SecretAzureDevOpsAppClientId).Fields.Password).To(Equal("azure-devops-app-client-id"))
					Expect(vault.GetSecret(files.SecretAzureDevOpsAppClientSecret).Fields.Password).To(Equal("azure-devops-app-client-secret"))
				})
			})

			Context("When Azure DevOps credentials are not set", func() {
				It("skips setting Azure DevOps OAuth configuration", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.GitProviders.AzureDevOps).To(BeNil())
				})
			})

			Context("When OIDC OAuth provider is fully configured", func() {
				BeforeEach(func() {
					csEnv.OidcIssuerURL = "https://dev-idp.example.com"
					csEnv.OidcClientID = "oidc-client-id"
					csEnv.OidcClientSecret = "oidc-client-secret"
					csEnv.OidcProviderName = "MyIDP"
				})
				It("sets OIDC OAuth configuration", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.OAuth).NotTo(BeNil())
					Expect(bs.Env.InstallConfig.Codesphere.OAuth.Oidc).NotTo(BeNil())
					Expect(bs.Env.InstallConfig.Codesphere.OAuth.Oidc.Type).To(Equal("oidc"))
					Expect(bs.Env.InstallConfig.Codesphere.OAuth.Oidc.Enabled).To(BeTrue())
					Expect(bs.Env.InstallConfig.Codesphere.OAuth.Oidc.Name).To(Equal("MyIDP"))
					Expect(bs.Env.InstallConfig.Codesphere.OAuth.Oidc.IssuerURL).To(Equal("https://dev-idp.example.com"))
					Expect(bs.Env.InstallConfig.Codesphere.OAuth.Oidc.Scopes).To(Equal([]string{"openid", "profile", "email"}))
					Expect(vault.GetSecret(files.SecretOidcClientId).Fields.Password).To(Equal("oidc-client-id"))
					Expect(vault.GetSecret(files.SecretOidcClientSecret).Fields.Password).To(Equal("oidc-client-secret"))
				})
			})

			Context("When OIDC OAuth provider name is not set", func() {
				BeforeEach(func() {
					csEnv.OidcIssuerURL = "https://dev-idp.example.com"
					csEnv.OidcClientID = "oidc-client-id"
					csEnv.OidcClientSecret = "oidc-client-secret"
				})
				It("defaults OIDC provider name to OIDC", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.OAuth.Oidc.Name).To(Equal("OIDC"))
				})
			})

			Context("When OIDC OAuth provider is not configured", func() {
				It("skips setting OIDC OAuth configuration", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.OAuth).To(BeNil())
				})
			})

			Context("When CentralOtel credentials are fully set", func() {
				BeforeEach(func() {
					csEnv.CentralOtelUsername = "otel-user"
					csEnv.CentralOtelPassword = "otel-password"
				})
				It("sets CentralOtel credentials in install config with Enabled true", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Cluster.Monitoring).NotTo(BeNil())
					centralOtel := bs.Env.InstallConfig.Cluster.Monitoring.CentralOtelExport
					Expect(centralOtel).NotTo(BeNil())
					Expect(centralOtel.Enabled).To(BeTrue())
					Expect(centralOtel.Username).To(Equal("otel-user"))
					Expect(centralOtel.Password).To(Equal("otel-password"))
				})
				It("stores CentralOtel username and password in the vault", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					secret := vault.GetSecret(files.SecretCentralOtelCreds)
					Expect(secret).NotTo(BeNil())
					Expect(secret.Fields).NotTo(BeNil())
					Expect(secret.Fields.Username).To(Equal("otel-user"))
					Expect(secret.Fields.Password).To(Equal("otel-password"))
				})
			})

			Context("When only CentralOtel username is set", func() {
				BeforeEach(func() {
					csEnv.CentralOtelUsername = "otel-user"
					csEnv.CentralOtelPassword = ""
				})
				It("skips setting CentralOtel credentials", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Cluster.Monitoring).To(BeNil())
				})
			})

			Context("When only CentralOtel password is set", func() {
				BeforeEach(func() {
					csEnv.CentralOtelUsername = ""
					csEnv.CentralOtelPassword = "otel-password"
				})
				It("skips setting CentralOtel credentials", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Cluster.Monitoring).To(BeNil())
				})
			})

			Context("When CentralOtel credentials are not set", func() {
				It("leaves Monitoring.CentralOtel nil", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Cluster.Monitoring).To(BeNil())
				})
			})

			Context("When Google ACME issuer is enabled", func() {
				BeforeEach(func() {
					csEnv.GoogleACMEIssuer = true
				})
				It("requests EAB credentials from Public CA and uses them in the ACME config", func() {
					gc.EXPECT().CreatePublicCAExternalAccountKey(mock.Anything).Return("fake-eab-key-id", "fake-eab-mac-key", nil)
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.CertIssuer.Acme.Server).To(Equal("https://dv.acme-v02.api.pki.goog/directory"))
					Expect(bs.Env.InstallConfig.Codesphere.CertIssuer.Acme.EABKeyID).To(Equal("fake-eab-key-id"))
					Expect(vault.GetSecret(files.SecretAcmeEabMacKey).Fields.Password).To(Equal("fake-eab-mac-key"))

					issuers := bs.Env.InstallConfig.Cluster.Certificates.Override["issuers"].(map[string]interface{})
					httpIssuer := issuers["letsEncryptHttp"].(map[string]interface{})
					Expect(httpIssuer["enabled"]).To(Equal(false))
				})
				It("returns an error when the publicca API call fails", func() {
					gc.EXPECT().CreatePublicCAExternalAccountKey(mock.Anything).Return("", "", fmt.Errorf("api boom"))

					err := bs.UpdateInstallConfig()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to obtain Google Public CA EAB credentials"))
				})
			})

			Context("When ACME staging is enabled", func() {
				BeforeEach(func() {
					csEnv.ACMEStaging = true
				})

				It("uses the Let's Encrypt staging directory", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())
					Expect(bs.Env.InstallConfig.Codesphere.CertIssuer.Acme.Server).To(Equal("https://acme-staging-v02.api.letsencrypt.org/directory"))
				})
			})

			Context("When OpenBao config is set", func() {
				BeforeEach(func() {
					csEnv.OpenBaoURI = "https://openbao.example.com"
					csEnv.OpenBaoPassword = "fake-password"
					csEnv.OpenBaoUser = "fake-username"
					csEnv.OpenBaoEngine = "fake-engine"
				})
				It("sets OpenBao config in install config", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.OpenBao.URI).To(Equal("https://openbao.example.com"))
					Expect(vault.GetSecret(files.SecretOpenBaoPassword).Fields.Password).To(Equal("fake-password"))
					Expect(bs.Env.InstallConfig.Codesphere.OpenBao.User).To(Equal("fake-username"))
					Expect(bs.Env.InstallConfig.Codesphere.OpenBao.Engine).To(Equal("fake-engine"))
				})
			})
			Context("When external Loki config is set", func() {
				BeforeEach(func() {
					csEnv.ExternalLokiEndpoint = "https://loki.example.com/loki/api/v1/push"
					csEnv.ExternalLokiSecret = "fake-loki-password"
					csEnv.ExternalLokiUser = "fake-loki-user"
				})
				It("sets Grafana Alloy Loki config", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					loki := bs.Env.InstallConfig.Cluster.Monitoring.GrafanaAlloy.Loki
					Expect(bs.Env.InstallConfig.Cluster.Monitoring.GrafanaAlloy.Enabled).To(BeTrue())
					Expect(loki.Endpoint).To(Equal("https://loki.example.com/loki/api/v1/push"))
					Expect(loki.Password).To(Equal("fake-loki-password"))
					Expect(loki.User).To(Equal("fake-loki-user"))
				})
				It("stores External Loki credentials in the vault", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					secret := vault.GetSecret(files.SecretLokiGatewayBasicAuthPassword)
					Expect(secret).NotTo(BeNil())
					Expect(secret.Fields).NotTo(BeNil())
					Expect(secret.Fields.Password).To(Equal("fake-loki-password"))
				})
			})
			Context("When only external Loki endpoint is set", func() {
				BeforeEach(func() {
					csEnv.ExternalLokiEndpoint = "https://loki.example.com/loki/api/v1/push"
				})
				It("sets Grafana Alloy Loki config without a password secret", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					loki := bs.Env.InstallConfig.Cluster.Monitoring.GrafanaAlloy.Loki
					Expect(loki.Endpoint).To(Equal("https://loki.example.com/loki/api/v1/push"))
					Expect(loki.Password).To(BeEmpty())
					Expect(loki.User).To(BeEmpty())
				})
			})

			Context("When Prometheus remote write is fully configured", func() {
				BeforeEach(func() {
					csEnv.PrometheusRemoteWriteURL = "https://prometheus.example.com/api/v1/write"
					csEnv.PrometheusRemoteWriteUser = "prom-user"
					csEnv.PrometheusRemoteWritePassword = "prom-password"
				})
				It("sets Prometheus remote write config", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					rw := bs.Env.InstallConfig.Cluster.Monitoring.Prometheus.RemoteWrite
					Expect(rw).NotTo(BeNil())
					Expect(rw.Enabled).To(BeTrue())
					Expect(rw.Url).To(Equal("https://prometheus.example.com/api/v1/write"))
					Expect(rw.ClusterName).To(Equal(csEnv.DatacenterName))
					Expect(rw.Username).To(Equal("prom-user"))
					Expect(vault.GetSecret("promRemoteWritePassword").Fields.Password).To(Equal("prom-password"))
				})
			})

			Context("When Prometheus remote write URL is not set", func() {
				It("leaves Prometheus remote write nil", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Cluster.Monitoring).To(BeNil())
				})
			})

			Context("When CentralOtelPassword is set", func() {
				BeforeEach(func() {
					csEnv.CentralOtelPassword = "otel-secret"
					csEnv.CentralOtelEndpoint = "https://otel.example.com"
					csEnv.CentralOtelSpanMetrics = true
				})
				It("sets TelemetryExport with RemoteExport true", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					te := bs.Env.InstallConfig.Codesphere.TelemetryExport
					Expect(te).NotTo(BeNil())
					Expect(te.RemoteEndpoint).To(Equal("https://otel.example.com"))
					Expect(te.RemoteExport).To(BeTrue())
					Expect(te.Traces).To(BeFalse())
					Expect(te.SpanMetrics).To(BeTrue())
				})
			})

			Context("When LocalTraceEndpoint is set (no password)", func() {
				BeforeEach(func() {
					csEnv.LocalTraceEndpoint = "http://localhost:4318"
					csEnv.CentralOtelEndpoint = "https://otel.example.com"
				})
				It("sets TelemetryExport with Traces true and RemoteExport false", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					te := bs.Env.InstallConfig.Codesphere.TelemetryExport
					Expect(te).NotTo(BeNil())
					Expect(te.RemoteEndpoint).To(Equal("https://otel.example.com"))
					Expect(te.RemoteExport).To(BeFalse())
					Expect(te.Traces).To(BeTrue())
					Expect(te.TraceEndpoint).To(Equal("http://localhost:4318"))
					Expect(te.SpanMetrics).To(BeFalse())
				})
			})

			Context("When both CentralOtelPassword and CentralOtelEndpoint are set", func() {
				BeforeEach(func() {
					csEnv.CentralOtelPassword = "otel-secret"
					csEnv.CentralOtelEndpoint = "https://otel.example.com"
					csEnv.CentralOtelSpanMetrics = true
				})
				It("sets TelemetryExport with both RemoteExport and Traces true", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					te := bs.Env.InstallConfig.Codesphere.TelemetryExport
					Expect(te).NotTo(BeNil())
					Expect(te.RemoteEndpoint).To(Equal("https://otel.example.com"))
					Expect(te.RemoteExport).To(BeTrue())
					Expect(te.Traces).To(BeFalse())
					Expect(te.SpanMetrics).To(BeTrue())
				})
			})

			Context("When neither CentralOtelPassword nor LocalTraceEndpoint are set", func() {
				It("leaves TelemetryExport nil", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.TelemetryExport).To(BeNil())
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

		Describe("ExistingConfigUsed", func() {
			BeforeEach(func() {
				csEnv.ExistingConfigUsed = true
				icg.EXPECT().GenerateSecrets().Return(nil)
			})

			Context("with unchanged IP and existing key", func() {
				BeforeEach(func() {
					caKey, caCert, err := secrets.GenerateCA("Test CA", "DE", "Berlin", "TestOrg")
					Expect(err).NotTo(HaveOccurred())

					key, cert, err := secrets.GenerateServerCertificate(caKey, caCert, "postgres", []string{"10.0.0.1"})
					Expect(err).NotTo(HaveOccurred())

					vault.SetSecret(files.SecretEntry{Name: files.SecretPostgresCaKeyPem, File: &files.SecretFile{Name: "ca.key", Content: caKey}})
					vault.SetSecret(files.SecretEntry{Name: files.SecretPostgresPrimaryServerKeyPem, File: &files.SecretFile{Name: "primary.key", Content: key}})
					csEnv.InstallConfig.Postgres.CACertPem = caCert
					csEnv.InstallConfig.Postgres.Primary.IP = "10.0.0.1"
					csEnv.InstallConfig.Postgres.Primary.Hostname = "postgres"
					csEnv.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem = cert
				})

				It("preserves existing cert/key without regeneration", func() {
					origKey := vault.GetSecret(files.SecretPostgresPrimaryServerKeyPem).File.Content
					origCert := csEnv.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem

					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(vault.GetSecret(files.SecretPostgresPrimaryServerKeyPem).File.Content).To(Equal(origKey))
					Expect(bs.Env.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem).To(Equal(origCert))
				})
			})

			Context("with changed IP", func() {
				BeforeEach(func() {
					caKey, caCert, err := secrets.GenerateCA("Test CA", "DE", "Berlin", "TestOrg")
					Expect(err).NotTo(HaveOccurred())

					key, cert, err := secrets.GenerateServerCertificate(caKey, caCert, "postgres", []string{"10.0.0.99"})
					Expect(err).NotTo(HaveOccurred())

					vault.SetSecret(files.SecretEntry{Name: files.SecretPostgresCaKeyPem, File: &files.SecretFile{Name: "ca.key", Content: caKey}})
					vault.SetSecret(files.SecretEntry{Name: files.SecretPostgresPrimaryServerKeyPem, File: &files.SecretFile{Name: "primary.key", Content: key}})
					csEnv.InstallConfig.Postgres.CACertPem = caCert
					csEnv.InstallConfig.Postgres.Primary.IP = "10.0.0.99"
					csEnv.InstallConfig.Postgres.Primary.Hostname = "postgres"
					csEnv.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem = cert
				})

				It("regenerates cert/key for the new IP", func() {
					origKey := vault.GetSecret(files.SecretPostgresPrimaryServerKeyPem).File.Content

					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					// IP should be updated to the PostgreSQLNode's InternalIP ("10.0.0.1" from fakeNode)
					Expect(bs.Env.InstallConfig.Postgres.Primary.IP).To(Equal("10.0.0.1"))
					// Key should be regenerated in vault
					newKey := vault.GetSecret(files.SecretPostgresPrimaryServerKeyPem).File.Content
					Expect(newKey).NotTo(Equal(origKey))
					Expect(newKey).NotTo(BeEmpty())
					// New cert/key should match
					err = secrets.ValidateCertKeyPair(
						bs.Env.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem,
						newKey,
					)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("with empty PrivateKey (not loaded from vault)", func() {
				BeforeEach(func() {
					caKey, caCert, err := secrets.GenerateCA("Test CA", "DE", "Berlin", "TestOrg")
					Expect(err).NotTo(HaveOccurred())

					vault.SetSecret(files.SecretEntry{Name: files.SecretPostgresCaKeyPem, File: &files.SecretFile{Name: "ca.key", Content: caKey}})
					csEnv.InstallConfig.Postgres.CACertPem = caCert
					csEnv.InstallConfig.Postgres.Primary.IP = "10.0.0.1"
					csEnv.InstallConfig.Postgres.Primary.Hostname = "postgres"
				})

				It("generates new cert/key pair", func() {
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					newKey := vault.GetSecret(files.SecretPostgresPrimaryServerKeyPem)
					Expect(newKey).NotTo(BeNil())
					Expect(newKey.File.Content).NotTo(BeEmpty())
					Expect(bs.Env.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem).NotTo(BeEmpty())
					err = secrets.ValidateCertKeyPair(
						bs.Env.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem,
						newKey.File.Content,
					)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("with missing CA cert (cert generation fails)", func() {
				BeforeEach(func() {
					csEnv.InstallConfig.Postgres.CACertPem = ""
					csEnv.InstallConfig.Postgres.Primary.IP = "10.0.0.1"
					csEnv.InstallConfig.Postgres.Primary.Hostname = "postgres"
				})

				It("returns an error", func() {
					err := bs.UpdateInstallConfig()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("postgres CA key not found in vault"))
				})
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
})
