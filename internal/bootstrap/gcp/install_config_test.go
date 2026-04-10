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
			BaseDomain:            "example.com",
			DNSProjectID:          "dns-project",
			DNSZoneName:           "test-zone",
			SSHPublicKeyPath:      "key.pub",
			ProjectID:             "pid",
			Experiments:           gcp.DefaultExperiments,
			FeatureFlags:          map[string]bool{},
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
				Expect(bs.Env.InstallConfig.Codesphere.Features).To(Equal(map[string]bool{}))
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

				Expect(bs.Env.InstallConfig.Codesphere.OpenBao).To(BeNil())
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

					Expect(bs.Env.InstallConfig.Codesphere.Experiments).To(Equal(csEnv.Experiments))
				})
			})
			Context("When feature flags are set in CodesphereEnvironment", func() {
				BeforeEach(func() {
					csEnv.FeatureFlags = map[string]bool{"fake-flag1": true, "fake-flag2": true}
				})
				It("uses those feature flags", func() {
					icg.EXPECT().GenerateSecrets().Return(nil)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)

					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Codesphere.Features).To(Equal(csEnv.FeatureFlags))
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
					Expect(bs.Env.InstallConfig.Codesphere.OpenBao.Password).To(Equal("fake-password"))
					Expect(bs.Env.InstallConfig.Codesphere.OpenBao.User).To(Equal("fake-username"))
					Expect(bs.Env.InstallConfig.Codesphere.OpenBao.Engine).To(Equal("fake-engine"))
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
			})

			Context("with unchanged IP and existing key", func() {
				BeforeEach(func() {
					caKey, caCert, err := installer.GenerateCA("Test CA", "DE", "Berlin", "TestOrg")
					Expect(err).NotTo(HaveOccurred())

					key, cert, err := installer.GenerateServerCertificate(caKey, caCert, "postgres", []string{"10.0.0.1"})
					Expect(err).NotTo(HaveOccurred())

					csEnv.InstallConfig.Postgres.CaCertPrivateKey = caKey
					csEnv.InstallConfig.Postgres.CACertPem = caCert
					csEnv.InstallConfig.Postgres.Primary.IP = "10.0.0.1"
					csEnv.InstallConfig.Postgres.Primary.Hostname = "postgres"
					csEnv.InstallConfig.Postgres.Primary.PrivateKey = key
					csEnv.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem = cert
				})

				It("preserves existing cert/key without regeneration", func() {
					origKey := csEnv.InstallConfig.Postgres.Primary.PrivateKey
					origCert := csEnv.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem

					icg.EXPECT().GenerateSecrets().Return(nil).Times(0)
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Postgres.Primary.PrivateKey).To(Equal(origKey))
					Expect(bs.Env.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem).To(Equal(origCert))
				})
			})

			Context("with changed IP", func() {
				BeforeEach(func() {
					caKey, caCert, err := installer.GenerateCA("Test CA", "DE", "Berlin", "TestOrg")
					Expect(err).NotTo(HaveOccurred())

					key, cert, err := installer.GenerateServerCertificate(caKey, caCert, "postgres", []string{"10.0.0.99"})
					Expect(err).NotTo(HaveOccurred())

					csEnv.InstallConfig.Postgres.CaCertPrivateKey = caKey
					csEnv.InstallConfig.Postgres.CACertPem = caCert
					csEnv.InstallConfig.Postgres.Primary.IP = "10.0.0.99"
					csEnv.InstallConfig.Postgres.Primary.Hostname = "postgres"
					csEnv.InstallConfig.Postgres.Primary.PrivateKey = key
					csEnv.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem = cert
				})

				It("regenerates cert/key for the new IP", func() {
					origKey := csEnv.InstallConfig.Postgres.Primary.PrivateKey

					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					// IP should be updated to the PostgreSQLNode's InternalIP ("10.0.0.1" from fakeNode)
					Expect(bs.Env.InstallConfig.Postgres.Primary.IP).To(Equal("10.0.0.1"))
					// Key should be regenerated
					Expect(bs.Env.InstallConfig.Postgres.Primary.PrivateKey).NotTo(Equal(origKey))
					Expect(bs.Env.InstallConfig.Postgres.Primary.PrivateKey).NotTo(BeEmpty())
					// New cert/key should match
					err = installer.ValidateCertKeyPair(
						bs.Env.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem,
						bs.Env.InstallConfig.Postgres.Primary.PrivateKey,
					)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("with empty PrivateKey (not loaded from vault)", func() {
				BeforeEach(func() {
					caKey, caCert, err := installer.GenerateCA("Test CA", "DE", "Berlin", "TestOrg")
					Expect(err).NotTo(HaveOccurred())

					csEnv.InstallConfig.Postgres.CaCertPrivateKey = caKey
					csEnv.InstallConfig.Postgres.CACertPem = caCert
					csEnv.InstallConfig.Postgres.Primary.IP = "10.0.0.1"
					csEnv.InstallConfig.Postgres.Primary.Hostname = "postgres"
					csEnv.InstallConfig.Postgres.Primary.PrivateKey = ""
				})

				It("generates new cert/key pair", func() {
					icg.EXPECT().WriteInstallConfig("fake-config-file", true).Return(nil)
					icg.EXPECT().WriteVault("fake-secret", true).Return(nil)
					nodeClient.EXPECT().CopyFile(mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

					err := bs.UpdateInstallConfig()
					Expect(err).NotTo(HaveOccurred())

					Expect(bs.Env.InstallConfig.Postgres.Primary.PrivateKey).NotTo(BeEmpty())
					Expect(bs.Env.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem).NotTo(BeEmpty())
					err = installer.ValidateCertKeyPair(
						bs.Env.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem,
						bs.Env.InstallConfig.Postgres.Primary.PrivateKey,
					)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("with missing CA cert (cert generation fails)", func() {
				BeforeEach(func() {
					csEnv.InstallConfig.Postgres.CaCertPrivateKey = ""
					csEnv.InstallConfig.Postgres.CACertPem = ""
					csEnv.InstallConfig.Postgres.Primary.IP = "10.0.0.1"
					csEnv.InstallConfig.Postgres.Primary.Hostname = "postgres"
					csEnv.InstallConfig.Postgres.Primary.PrivateKey = ""
				})

				It("returns an error", func() {
					err := bs.UpdateInstallConfig()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to generate primary server certificate"))
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
