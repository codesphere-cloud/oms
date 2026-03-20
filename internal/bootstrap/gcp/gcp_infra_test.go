// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/artifactregistry/apiv1/artifactregistrypb"
	"cloud.google.com/go/compute/apiv1/computepb"
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
	gh "github.com/google/go-github/v74/github"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"google.golang.org/api/cloudbilling/v1"
	"google.golang.org/api/dns/v1"
)

var _ = Describe("GCP Infrastructure", func() {
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

		csEnv = NewTestCodesphereEnvironment(nodeClient)
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
				gc.EXPECT().CreateProject(csEnv.FolderID, "new-proj-id", csEnv.ProjectName, time.Hour).Return("", nil)

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
				gc.EXPECT().CreateProject("", "fake-id", csEnv.ProjectName, time.Hour).Return("", fmt.Errorf("create error"))

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
				// Mock ReadFile for SSH key
				fw.EXPECT().ReadFile(csEnv.SSHPublicKeyPath).Return([]byte("ssh-rsa AAA..."), nil).Times(1)

				// Mock CreateInstance (8 times)
				gc.EXPECT().CreateInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(nil).Times(8)

				// Mock GetInstance (8 times)
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
				gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(ipResp, nil).Times(8)

				err := bs.EnsureComputeInstances()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(bs.Env.ControlPlaneNodes)).To(Equal(3))
				Expect(len(bs.Env.CephNodes)).To(Equal(3))
				Expect(bs.Env.PostgreSQLNode).NotTo(BeNil())
				Expect(bs.Env.Jumpbox).NotTo(BeNil())
			})

			Context("When github org is set", func() {
				BeforeEach(func() {
					csEnv.GitHubTeamOrg = "someorg"
					csEnv.GitHubTeamSlug = ""
					csEnv.GitHubPAT = "pat"
				})
				It("does not fetch GitHub org keys when GitHub team org is set without slug", func() {
					fw.EXPECT().ReadFile(csEnv.SSHPublicKeyPath).Return([]byte("ssh-rsa AAA..."), nil).Times(1)
					gc.EXPECT().CreateInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(nil).Times(8)
					ipResp := &computepb.Instance{
						NetworkInterfaces: []*computepb.NetworkInterface{{
							NetworkIP:     protoString("10.0.0.x"),
							AccessConfigs: []*computepb.AccessConfig{{NatIP: protoString("1.2.3.x")}},
						}},
					}
					gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(ipResp, nil).Times(8)

					err := bs.EnsureComputeInstances()
					Expect(err).NotTo(HaveOccurred())
				})
				Context("When GitHub team org and slug are set", func() {
					BeforeEach(func() {
						csEnv.GitHubTeamSlug = "dev"
					})
					It("fetches GitHub team keys", func() {
						mockGitHubClient.EXPECT().ListTeamMembersBySlug(mock.Anything, csEnv.GitHubTeamOrg, csEnv.GitHubTeamSlug, mock.Anything).Return([]*gh.User{{Login: gh.Ptr("alice")}}, nil).Once()
						mockGitHubClient.EXPECT().ListUserKeys(mock.Anything, "alice").Return([]*gh.Key{{Key: gh.Ptr("ssh-rsa AAALICE...")}}, nil).Once()

						fw.EXPECT().ReadFile(csEnv.SSHPublicKeyPath).Return([]byte("ssh-rsa AAA..."), nil).Times(1)
						gc.EXPECT().CreateInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).RunAndReturn(func(projectID, zone string, instance *computepb.Instance) error {
							sshMetadata := ""
							for _, item := range instance.GetMetadata().GetItems() {
								if item.GetKey() == "ssh-keys" {
									sshMetadata = item.GetValue()
								}
							}
							if !strings.Contains(sshMetadata, "AAALICE...") {
								return fmt.Errorf("expected ssh metadata to include team user key")
							}
							return nil
						}).Times(8)

						ipResp := &computepb.Instance{
							NetworkInterfaces: []*computepb.NetworkInterface{{
								NetworkIP:     protoString("10.0.0.x"),
								AccessConfigs: []*computepb.AccessConfig{{NatIP: protoString("1.2.3.x")}},
							}},
						}
						gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(ipResp, nil).Times(8)

						err := bs.EnsureComputeInstances()
						Expect(err).NotTo(HaveOccurred())
					})

					It("fails when GitHub client fails to list team members", func() {
						mockGitHubClient.EXPECT().ListTeamMembersBySlug(mock.Anything, csEnv.GitHubTeamOrg, csEnv.GitHubTeamSlug, mock.Anything).Return(nil, fmt.Errorf("list members error")).Once()

						err := bs.EnsureComputeInstances()
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("failed to get SSH keys from GitHub team"))
					})
				})
			})
		})

		Describe("Invalid cases", func() {
			It("fails when SSH key read fails", func() {
				fw.EXPECT().ReadFile(csEnv.SSHPublicKeyPath).Return(nil, fmt.Errorf("read error")).Maybe()

				err := bs.EnsureComputeInstances()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error reading SSH key from key.pub"))
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
})
