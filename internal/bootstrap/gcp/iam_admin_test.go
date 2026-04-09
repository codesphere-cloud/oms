// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"context"
	"fmt"

	"cloud.google.com/go/resourcemanager/apiv3/resourcemanagerpb"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"google.golang.org/api/cloudbilling/v1"

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
)

var _ = Describe("IAM & Admin", func() {
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

	Describe("EnsureProject", func() {
		Describe("Valid EnsureProject", func() {
			It("uses existing project", func() {
				gc.EXPECT().GetProjectByName(csEnv.FolderID, csEnv.ProjectName).Return(&resourcemanagerpb.Project{ProjectId: "existing-id", Name: "existing-proj"}, nil)
				gc.EXPECT().UpdateProject("existing-id", mock.Anything).Return(nil)

				err := bs.EnsureProject()
				Expect(err).NotTo(HaveOccurred())
				Expect(bs.Env.ProjectID).To(Equal("existing-id"))
			})

			It("creates project when missing", func() {
				gc.EXPECT().GetProjectByName(csEnv.FolderID, csEnv.ProjectName).Return(nil, fmt.Errorf("project not found: %s", csEnv.ProjectName))
				gc.EXPECT().CreateProjectID(csEnv.ProjectName).Return("new-proj-id")
				gc.EXPECT().CreateProject(csEnv.FolderID, "new-proj-id", csEnv.ProjectName, mock.Anything).Return("", nil)

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
				gc.EXPECT().CreateProject("", "fake-id", csEnv.ProjectName, mock.Anything).Return("", fmt.Errorf("create error"))

				err := bs.EnsureProject()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create project"))
				Expect(err.Error()).To(ContainSubstring("create error"))
			})

			It("returns an error when UpdateProject fails", func() {
				gc.EXPECT().GetProjectByName(csEnv.FolderID, csEnv.ProjectName).Return(&resourcemanagerpb.Project{ProjectId: "existing-id", Name: "existing-proj"}, nil)
				gc.EXPECT().UpdateProject("existing-id", mock.Anything).Return(fmt.Errorf("failed to update project"))

				err := bs.EnsureProject()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to update project"))
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

})
