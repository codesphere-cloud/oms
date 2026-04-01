// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"fmt"
	"strings"
	"sync"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/github"
	"github.com/codesphere-cloud/oms/internal/util"
	gh "github.com/google/go-github/v74/github"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

var _ = Describe("GCE", func() {

	Describe("IsNotFoundError", func() {
		Context("when error is nil", func() {
			It("should return false", func() {
				Expect(gcp.IsNotFoundError(nil)).To(BeFalse())
			})
		})

		Context("when error is a gRPC NotFound error", func() {
			It("should return true", func() {
				err := grpcstatus.Errorf(codes.NotFound, "not found")
				Expect(gcp.IsNotFoundError(err)).To(BeTrue())
			})
		})

		Context("when error is a Google API 404 error", func() {
			It("should return true", func() {
				err := &googleapi.Error{
					Code:    404,
					Message: "not found",
				}
				Expect(gcp.IsNotFoundError(err)).To(BeTrue())
			})
		})

		Context("when error is a Google API non-404 error", func() {
			It("should return false for 403 Forbidden", func() {
				err := &googleapi.Error{
					Code:    403,
					Message: "forbidden",
				}
				Expect(gcp.IsNotFoundError(err)).To(BeFalse())
			})

			It("should return false for 500 Internal Server Error", func() {
				err := &googleapi.Error{
					Code:    500,
					Message: "internal error",
				}
				Expect(gcp.IsNotFoundError(err)).To(BeFalse())
			})

			It("should return false for 401 Unauthorized", func() {
				err := &googleapi.Error{
					Code:    401,
					Message: "unauthorized",
				}
				Expect(gcp.IsNotFoundError(err)).To(BeFalse())
			})
		})

		Context("when error is a non-Google API error", func() {
			It("should return false", func() {
				err := fmt.Errorf("some other error")
				Expect(gcp.IsNotFoundError(err)).To(BeFalse())
			})
		})

		Context("when error wraps a Google API 404 error", func() {
			It("should return true for wrapped 404 errors", func() {
				innerErr := &googleapi.Error{
					Code:    404,
					Message: "not found",
				}
				wrappedErr := fmt.Errorf("failed to get record: %w", innerErr)
				Expect(gcp.IsNotFoundError(wrappedErr)).To(BeTrue())
			})

			It("should return false for wrapped non-404 errors", func() {
				innerErr := &googleapi.Error{
					Code:    403,
					Message: "forbidden",
				}
				wrappedErr := fmt.Errorf("failed to get record: %w", innerErr)
				Expect(gcp.IsNotFoundError(wrappedErr)).To(BeFalse())
			})
		})

		Context("when error wraps a gRPC NotFound error", func() {
			It("should return true", func() {
				innerErr := grpcstatus.Errorf(codes.NotFound, "not found")
				wrappedErr := fmt.Errorf("failed to get instance: %w", innerErr)
				Expect(gcp.IsNotFoundError(wrappedErr)).To(BeTrue())
			})
		})
	})

	Describe("IsSpotCapacityError", func() {
		It("returns false for nil error", func() {
			Expect(gcp.IsSpotCapacityError(nil)).To(BeFalse())
		})

		DescribeTable("detects known capacity errors",
			func(err error) { Expect(gcp.IsSpotCapacityError(err)).To(BeTrue()) },
			Entry("gRPC ResourceExhausted", grpcstatus.Errorf(codes.ResourceExhausted, "resource exhausted")),
			Entry("gRPC ResourceExhausted with detail", grpcstatus.Errorf(codes.ResourceExhausted, "spot VM pool exhausted in us-central1-a")),
			Entry("ZONE_RESOURCE_POOL_EXHAUSTED", fmt.Errorf("googleapi: Error 403: ZONE_RESOURCE_POOL_EXHAUSTED - the zone does not have enough resources")),
			Entry("UNSUPPORTED_OPERATION", fmt.Errorf("UNSUPPORTED_OPERATION: spot VMs not available in this zone")),
			Entry("stockout", fmt.Errorf("stockout in zone us-central1-a")),
			Entry("does not have enough resources", fmt.Errorf("the zone 'us-central1-a' does not have enough resources available to fulfill the request")),
		)

		DescribeTable("rejects non-capacity errors",
			func(err error) { Expect(gcp.IsSpotCapacityError(err)).To(BeFalse()) },
			Entry("NotFound", grpcstatus.Errorf(codes.NotFound, "not found")),
			Entry("PermissionDenied", grpcstatus.Errorf(codes.PermissionDenied, "denied")),
			Entry("Internal", grpcstatus.Errorf(codes.Internal, "internal")),
			Entry("Unavailable", grpcstatus.Errorf(codes.Unavailable, "service unavailable")),
			Entry("permission denied string", fmt.Errorf("permission denied")),
			Entry("invalid argument string", fmt.Errorf("invalid argument")),
			Entry("quota exceeded string", fmt.Errorf("quota exceeded")),
			Entry("network error string", fmt.Errorf("network error")),
		)
	})

	Describe("BuildSchedulingConfig", func() {
		var (
			bs    *gcp.GCPBootstrapper
			csEnv *gcp.CodesphereEnvironment
		)

		BeforeEach(func() {
			csEnv = &gcp.CodesphereEnvironment{
				ProjectName:  "test",
				Region:       "us-central1",
				Zone:         "us-central1-a",
				BaseDomain:   "example.com",
				DNSProjectID: "dns-project",
				DNSZoneName:  "test-zone",
				SecretsDir:   "/etc/codesphere/secrets",
				DatacenterID: 1,
				Experiments:  gcp.DefaultExperiments,
			}
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			bs = newTestBootstrapper(csEnv, gc)
		})

		It("returns spot scheduling when spot is enabled", func() {
			csEnv.SpotVMs = true
			sched := bs.BuildSchedulingConfig()
			Expect(*sched.ProvisioningModel).To(Equal("SPOT"))
			Expect(*sched.OnHostMaintenance).To(Equal("TERMINATE"))
			Expect(*sched.AutomaticRestart).To(BeFalse())
			Expect(*sched.InstanceTerminationAction).To(Equal("STOP"))
			Expect(sched.Preemptible).To(BeNil())
		})

		It("returns preemptible scheduling when preemptible is enabled", func() {
			csEnv.Preemptible = true
			sched := bs.BuildSchedulingConfig()
			Expect(*sched.Preemptible).To(BeTrue())
			Expect(sched.ProvisioningModel).To(BeNil())
			Expect(sched.OnHostMaintenance).To(BeNil())
			Expect(sched.AutomaticRestart).To(BeNil())
			Expect(sched.InstanceTerminationAction).To(BeNil())
		})

		It("returns empty scheduling when neither is enabled", func() {
			sched := bs.BuildSchedulingConfig()
			Expect(sched.ProvisioningModel).To(BeNil())
			Expect(sched.Preemptible).To(BeNil())
			Expect(sched.OnHostMaintenance).To(BeNil())
			Expect(sched.AutomaticRestart).To(BeNil())
			Expect(sched.InstanceTerminationAction).To(BeNil())
		})
	})

	Describe("validateVMProvisioningOptions", func() {
		var csEnv *gcp.CodesphereEnvironment

		BeforeEach(func() {
			csEnv = &gcp.CodesphereEnvironment{
				ProjectName:           "test",
				Region:                "us-central1",
				Zone:                  "us-central1-a",
				BaseDomain:            "example.com",
				DNSProjectID:          "dns-project",
				DNSZoneName:           "test-zone",
				SecretsDir:            "/etc/codesphere/secrets",
				DatacenterID:          1,
				Experiments:           gcp.DefaultExperiments,
				InstallConfigPath:     "fake-config",
				SecretsFilePath:       "fake-secrets",
				GitHubAppName:         "fake-app",
				GitHubAppClientID:     "fake-id",
				GitHubAppClientSecret: "fake-secret",
			}
		})

		DescribeTable("succeeds for valid combinations",
			func(spot, preemptible bool) {
				csEnv.SpotVMs = spot
				csEnv.Preemptible = preemptible
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				bs := newTestBootstrapper(csEnv, gc)
				err := bs.ValidateInput()
				Expect(err).NotTo(HaveOccurred())
			},
			Entry("only spot", true, false),
			Entry("only preemptible", false, true),
			Entry("neither", false, false),
		)

		It("fails when both spot and preemptible are set", func() {
			csEnv.SpotVMs = true
			csEnv.Preemptible = true
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			bs := newTestBootstrapper(csEnv, gc)
			err := bs.ValidateInput()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot specify both --spot-vms and --preemptible"))
			Expect(err.Error()).To(ContainSubstring("use --spot-vms for the newer spot VM model"))
		})
	})

	Describe("CreateInstanceWithFallback", func() {
		var (
			bs    *gcp.GCPBootstrapper
			csEnv *gcp.CodesphereEnvironment
			gc    *gcp.MockGCPClientManager
			logCh chan string
		)

		BeforeEach(func() {
			gc = gcp.NewMockGCPClientManager(GinkgoT())
			csEnv = &gcp.CodesphereEnvironment{
				ProjectName:  "test",
				ProjectID:    "test-pid",
				Region:       "us-central1",
				Zone:         "us-central1-a",
				BaseDomain:   "example.com",
				DNSProjectID: "dns-project",
				DNSZoneName:  "test-zone",
				SecretsDir:   "/etc/codesphere/secrets",
				DatacenterID: 1,
				Experiments:  gcp.DefaultExperiments,
			}
			logCh = make(chan string, 10)
			bs = newTestBootstrapper(csEnv, gc)
		})

		It("succeeds on first attempt", func() {
			instance := &computepb.Instance{Name: protoString("test-vm")}
			gc.EXPECT().CreateInstance("test-pid", "us-central1-a", instance).Return(nil)

			Expect(bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)).To(Succeed())
		})

		It("treats AlreadyExists as success", func() {
			instance := &computepb.Instance{Name: protoString("test-vm")}
			gc.EXPECT().CreateInstance("test-pid", "us-central1-a", instance).
				Return(grpcstatus.Errorf(codes.AlreadyExists, "already exists"))

			Expect(bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)).To(Succeed())
		})

		Context("when spot is enabled", func() {
			BeforeEach(func() {
				csEnv.SpotVMs = true
			})

			spotInstance := func(name string) *computepb.Instance {
				return &computepb.Instance{
					Name:       protoString(name),
					Scheduling: &computepb.Scheduling{ProvisioningModel: protoString("SPOT")},
				}
			}

			DescribeTable("falls back to standard VM on capacity errors",
				func(capacityErr error) {
					instance := spotInstance("test-vm")
					gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).Return(capacityErr).Once()
					gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).Return(nil).Once()

					Expect(bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)).To(Succeed())
					Expect(logCh).To(Receive(ContainSubstring("falling back to standard VM")))
				},
				Entry("gRPC ResourceExhausted", grpcstatus.Errorf(codes.ResourceExhausted, "exhausted")),
				Entry("ZONE_RESOURCE_POOL_EXHAUSTED", fmt.Errorf("ZONE_RESOURCE_POOL_EXHAUSTED")),
				Entry("UNSUPPORTED_OPERATION", fmt.Errorf("UNSUPPORTED_OPERATION")),
				Entry("stockout", fmt.Errorf("stockout in zone")),
				Entry("does not have enough resources", fmt.Errorf("zone does not have enough resources")),
			)

			It("clears scheduling config on fallback", func() {
				instance := spotInstance("test-vm")
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
					Return(fmt.Errorf("ZONE_RESOURCE_POOL_EXHAUSTED")).Once()
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.MatchedBy(func(inst *computepb.Instance) bool {
					return inst.Scheduling != nil &&
						inst.Scheduling.ProvisioningModel == nil &&
						inst.Scheduling.Preemptible == nil
				})).Return(nil).Once()

				Expect(bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)).To(Succeed())
			})

			It("returns error with context when fallback also fails", func() {
				instance := spotInstance("test-vm")
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
					Return(fmt.Errorf("ZONE_RESOURCE_POOL_EXHAUSTED")).Once()
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
					Return(fmt.Errorf("insufficient quota")).Once()

				err := bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fallback to standard VM"))
				Expect(err.Error()).To(ContainSubstring("insufficient quota"))
			})

			It("does NOT fall back on non-capacity errors", func() {
				instance := spotInstance("test-vm")
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
					Return(fmt.Errorf("permission denied")).Once()

				err := bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create instance test-vm"))
				Expect(err.Error()).To(ContainSubstring("permission denied"))
				Expect(logCh).To(BeEmpty())
			})

			It("succeeds when fallback retry returns AlreadyExists", func() {
				instance := spotInstance("test-vm")
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
					Return(grpcstatus.Errorf(codes.ResourceExhausted, "exhausted")).Once()
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
					Return(grpcstatus.Errorf(codes.AlreadyExists, "already exists")).Once()

				Expect(bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)).To(Succeed())
				Expect(logCh).To(Receive(ContainSubstring("falling back to standard VM")))
			})
		})

		Context("when spot is disabled", func() {
			It("does not fall back on capacity errors", func() {
				instance := &computepb.Instance{Name: protoString("test-vm")}
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", instance).
					Return(fmt.Errorf("ZONE_RESOURCE_POOL_EXHAUSTED"))

				err := bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create instance test-vm"))
			})

			It("returns nil for string-based already exists error", func() {
				instance := &computepb.Instance{Name: protoString("test-vm")}
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", instance).
					Return(fmt.Errorf("The resource 'test-vm' already exists"))

				Expect(bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)).To(Succeed())
			})
		})
	})

	Describe("extractInstanceIPs", func() {
		It("extracts both internal and external IPs", func() {
			inst := makeRunningInstance("10.0.0.1", "35.1.2.3")
			internalIP, externalIP := gcp.ExtractInstanceIPs(inst)
			Expect(internalIP).To(Equal("10.0.0.1"))
			Expect(externalIP).To(Equal("35.1.2.3"))
		})

		It("returns empty external IP when no access configs", func() {
			inst := &computepb.Instance{
				Status: protoString("RUNNING"),
				NetworkInterfaces: []*computepb.NetworkInterface{
					{NetworkIP: protoString("10.0.0.1")},
				},
			}
			internalIP, externalIP := gcp.ExtractInstanceIPs(inst)
			Expect(internalIP).To(Equal("10.0.0.1"))
			Expect(externalIP).To(BeEmpty())
		})

		It("returns empty IPs when no network interfaces", func() {
			inst := &computepb.Instance{
				Status: protoString("RUNNING"),
			}
			internalIP, externalIP := gcp.ExtractInstanceIPs(inst)
			Expect(internalIP).To(BeEmpty())
			Expect(externalIP).To(BeEmpty())
		})
	})

	Describe("isInstanceReady", func() {
		It("returns true for RUNNING instance with internal IP", func() {
			inst := makeRunningInstance("10.0.0.1", "")
			Expect(gcp.IsInstanceReady(inst, false)).To(BeTrue())
		})

		It("returns true for RUNNING instance with both IPs when external needed", func() {
			inst := makeRunningInstance("10.0.0.1", "35.1.2.3")
			Expect(gcp.IsInstanceReady(inst, true)).To(BeTrue())
		})

		It("returns false for RUNNING instance without external IP when needed", func() {
			inst := &computepb.Instance{
				Status: protoString("RUNNING"),
				NetworkInterfaces: []*computepb.NetworkInterface{
					{NetworkIP: protoString("10.0.0.1")},
				},
			}
			Expect(gcp.IsInstanceReady(inst, true)).To(BeFalse())
		})

		It("returns false for non-RUNNING instance", func() {
			inst := makeStoppedInstance("10.0.0.1", "35.1.2.3")
			Expect(gcp.IsInstanceReady(inst, false)).To(BeFalse())
		})

		It("returns false when no network interfaces", func() {
			inst := &computepb.Instance{
				Status: protoString("RUNNING"),
			}
			Expect(gcp.IsInstanceReady(inst, false)).To(BeFalse())
		})

		It("returns false when internal IP is empty", func() {
			inst := &computepb.Instance{
				Status: protoString("RUNNING"),
				NetworkInterfaces: []*computepb.NetworkInterface{
					{NetworkIP: protoString("")},
				},
			}
			Expect(gcp.IsInstanceReady(inst, false)).To(BeFalse())
		})
	})

	Describe("isAlreadyExistsError", func() {
		It("returns false for nil error", func() {
			Expect(gcp.IsAlreadyExistsError(nil)).To(BeFalse())
		})

		It("returns true for gRPC AlreadyExists error", func() {
			err := grpcstatus.Errorf(codes.AlreadyExists, "already exists")
			Expect(gcp.IsAlreadyExistsError(err)).To(BeTrue())
		})

		It("returns true for string-based already exists error", func() {
			err := fmt.Errorf("The resource 'my-vm' already exists")
			Expect(gcp.IsAlreadyExistsError(err)).To(BeTrue())
		})

		It("returns false for unrelated error", func() {
			err := fmt.Errorf("permission denied")
			Expect(gcp.IsAlreadyExistsError(err)).To(BeFalse())
		})

		It("returns false for gRPC NotFound error", func() {
			err := grpcstatus.Errorf(codes.NotFound, "not found")
			Expect(gcp.IsAlreadyExistsError(err)).To(BeFalse())
		})
	})

	Describe("readSSHKey", func() {
		var (
			bs *gcp.GCPBootstrapper
			fw *util.MockFileIO
		)

		BeforeEach(func() {
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			fw = util.NewMockFileIO(GinkgoT())
			csEnv := &gcp.CodesphereEnvironment{
				ProjectName:  "test",
				Region:       "us-central1",
				Zone:         "us-central1-a",
				BaseDomain:   "example.com",
				DNSProjectID: "dns-project",
				DNSZoneName:  "test-zone",
				SecretsDir:   "/etc/codesphere/secrets",
				DatacenterID: 1,
				Experiments:  gcp.DefaultExperiments,
			}
			bs = newTestBootstrapperWithFileIO(csEnv, gc, fw)
		})

		It("reads and trims SSH key", func() {
			fw.EXPECT().ReadFile(mock.Anything).Return([]byte("ssh-rsa AAAA...  \n"), nil)
			key, err := bs.ReadSSHKey("~/.ssh/id_rsa.pub")
			Expect(err).NotTo(HaveOccurred())
			Expect(key).To(Equal("ssh-rsa AAAA..."))
		})

		It("returns error when file read fails", func() {
			fw.EXPECT().ReadFile(mock.Anything).Return(nil, fmt.Errorf("no such file"))
			_, err := bs.ReadSSHKey("~/.ssh/missing.pub")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error reading SSH key"))
		})

		It("returns error when key file is empty", func() {
			fw.EXPECT().ReadFile(mock.Anything).Return([]byte("   \n  "), nil)
			_, err := bs.ReadSSHKey("~/.ssh/empty.pub")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("is empty"))
		})
	})

	Describe("EnsureComputeInstances", func() {
		var (
			bs               *gcp.GCPBootstrapper
			csEnv            *gcp.CodesphereEnvironment
			gc               *gcp.MockGCPClientManager
			fw               *util.MockFileIO
			mockGitHubClient *github.MockGitHubClient
		)

		BeforeEach(func() {
			gc = gcp.NewMockGCPClientManager(GinkgoT())
			fw = util.NewMockFileIO(GinkgoT())
			mockGitHubClient = github.NewMockGitHubClient(GinkgoT())
			csEnv = &gcp.CodesphereEnvironment{
				ProjectName:      "test-project",
				ProjectID:        "pid",
				Region:           "us-central1",
				Zone:             "us-central1-a",
				BaseDomain:       "example.com",
				DNSProjectID:     "dns-project",
				DNSZoneName:      "test-zone",
				SecretsDir:       "/etc/codesphere/secrets",
				DatacenterID:     1,
				SSHPublicKeyPath: "key.pub",
				Experiments:      gcp.DefaultExperiments,
				FeatureFlags:     []string{},
			}
			bs = newTestBootstrapperAll(csEnv, gc, fw, mockGitHubClient)
		})

		Describe("Valid EnsureComputeInstances", func() {
			It("creates all instances", func() {
				ipResp := makeRunningInstance("10.0.0.x", "1.2.3.x")
				mockGetInstanceNotFoundThenRunning(gc, csEnv.ProjectID, csEnv.Zone, ipResp, 8)

				fw.EXPECT().ReadFile(mock.Anything).Return([]byte("ssh-rsa AAA..."), nil).Times(8)
				gc.EXPECT().CreateInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(nil).Times(8)

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
					ipResp := makeRunningInstance("10.0.0.x", "1.2.3.x")
					mockGetInstanceNotFoundThenRunning(gc, csEnv.ProjectID, csEnv.Zone, ipResp, 8)
					fw.EXPECT().ReadFile(mock.Anything).Return([]byte("ssh-rsa AAA..."), nil).Times(8)
					gc.EXPECT().CreateInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(nil).Times(8)

					err := bs.EnsureComputeInstances()
					Expect(err).NotTo(HaveOccurred())
				})
				Context("When GitHub team org and slug are set", func() {
					BeforeEach(func() {
						csEnv.GitHubTeamSlug = "dev"
					})
					It("fetches GitHub team keys", func() {
						mockGitHubClient.EXPECT().ListTeamMembersBySlug(mock.Anything, csEnv.GitHubTeamOrg, csEnv.GitHubTeamSlug, mock.Anything).Return([]*gh.User{{Login: gh.Ptr("alice")}}, nil).Maybe()
						mockGitHubClient.EXPECT().ListUserKeys(mock.Anything, "alice").Return([]*gh.Key{{Key: gh.Ptr("ssh-rsa AAALICE...")}}, nil).Maybe()

						ipResp := makeRunningInstance("10.0.0.x", "1.2.3.x")
						mockGetInstanceNotFoundThenRunning(gc, csEnv.ProjectID, csEnv.Zone, ipResp, 8)
						fw.EXPECT().ReadFile(csEnv.SSHPublicKeyPath).Return([]byte("ssh-rsa AAA..."), nil).Times(8)
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

						err := bs.EnsureComputeInstances()
						Expect(err).NotTo(HaveOccurred())
					})

					It("fails when GitHub client fails to list team members", func() {
						gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(nil, grpcstatus.Errorf(codes.NotFound, "not found")).Maybe()
						mockGitHubClient.EXPECT().ListTeamMembersBySlug(mock.Anything, csEnv.GitHubTeamOrg, csEnv.GitHubTeamSlug, mock.Anything).Return(nil, fmt.Errorf("list members error")).Maybe()

						err := bs.EnsureComputeInstances()
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("failed to get SSH keys from GitHub team"))
					})
				})
			})
		})

		Describe("Invalid cases", func() {
			notFoundErr := grpcstatus.Errorf(codes.NotFound, "not found")

			It("fails when SSH key read fails", func() {
				gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(nil, notFoundErr).Maybe()
				fw.EXPECT().ReadFile(mock.Anything).Return(nil, fmt.Errorf("read error")).Maybe()

				err := bs.EnsureComputeInstances()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error reading SSH key from key.pub"))
			})

			It("fails when CreateInstance fails", func() {
				gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(nil, notFoundErr).Maybe()
				fw.EXPECT().ReadFile(mock.Anything).Return([]byte("ssh-rsa AAA..."), nil).Maybe()
				gc.EXPECT().CreateInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(fmt.Errorf("create error")).Maybe()

				err := bs.EnsureComputeInstances()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error ensuring compute instances"))
			})

			It("fails when GetInstance fails after creation", func() {
				instanceCalls := make(map[string]int)
				var mu sync.Mutex
				gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).RunAndReturn(
					func(projectID, zone, name string) (*computepb.Instance, error) {
						mu.Lock()
						defer mu.Unlock()
						instanceCalls[name]++
						if instanceCalls[name] == 1 {
							return nil, notFoundErr
						}
						return nil, fmt.Errorf("get error")
					},
				).Maybe()
				fw.EXPECT().ReadFile(mock.Anything).Return([]byte("ssh-rsa AAA..."), nil).Maybe()
				gc.EXPECT().CreateInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(nil).Maybe()

				err := bs.EnsureComputeInstances()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error ensuring compute instances"))
			})
		})

		Describe("Spot VM functionality", func() {
			It("creates spot VMs when spot flag is enabled", func() {
				csEnv.SpotVMs = true

				ipResp := makeRunningInstance("10.0.0.x", "1.2.3.x")
				mockGetInstanceNotFoundThenRunning(gc, csEnv.ProjectID, csEnv.Zone, ipResp, 8)

				fw.EXPECT().ReadFile(mock.Anything).Return([]byte("ssh-rsa AAA..."), nil).Times(8)

				// Verify CreateInstance is called with SPOT provisioning model
				gc.EXPECT().CreateInstance(csEnv.ProjectID, csEnv.Zone, mock.MatchedBy(func(instance *computepb.Instance) bool {
					return instance.Scheduling != nil &&
						instance.Scheduling.ProvisioningModel != nil &&
						*instance.Scheduling.ProvisioningModel == "SPOT"
				})).Return(nil).Times(8)

				err := bs.EnsureComputeInstances()
				Expect(err).NotTo(HaveOccurred())
			})

			It("falls back to standard VM when spot capacity is exhausted", func() {
				csEnv.SpotVMs = true

				ipResp := makeRunningInstance("10.0.0.x", "1.2.3.x")
				mockGetInstanceNotFoundThenRunning(gc, csEnv.ProjectID, csEnv.Zone, ipResp, 8)

				fw.EXPECT().ReadFile(mock.Anything).Return([]byte("ssh-rsa AAA..."), nil).Times(8)

				createCalls := make(map[string]int)
				var mu sync.Mutex
				gc.EXPECT().CreateInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).RunAndReturn(func(projectID, zone string, instance *computepb.Instance) error {
					mu.Lock()
					defer mu.Unlock()
					name := *instance.Name
					createCalls[name]++
					if createCalls[name] == 1 {
						return fmt.Errorf("ZONE_RESOURCE_POOL_EXHAUSTED")
					}
					return nil
				}).Times(16)

				err := bs.EnsureComputeInstances()
				Expect(err).NotTo(HaveOccurred())
			})

			It("restarts stopped VMs instead of creating new ones", func() {
				instanceCalls := make(map[string]int)
				var mu sync.Mutex
				stoppedResp := makeStoppedInstance("10.0.0.x", "1.2.3.x")
				runningResp := makeRunningInstance("10.0.0.x", "1.2.3.x")
				gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).RunAndReturn(func(projectID, zone, name string) (*computepb.Instance, error) {
					mu.Lock()
					defer mu.Unlock()
					instanceCalls[name]++
					if instanceCalls[name] == 1 {
						// First call, VM exists but is stopped
						return stoppedResp, nil
					}
					// After StartInstance, VM is running
					return runningResp, nil
				}).Times(16)

				gc.EXPECT().StartInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(nil).Times(8)

				err := bs.EnsureComputeInstances()
				Expect(err).NotTo(HaveOccurred())
			})

			It("uses existing running VMs without starting them", func() {
				runningResp := makeRunningInstance("10.0.0.x", "1.2.3.x")
				// 8 VMs × 2 GetInstance calls each (initial check + waitForInstanceRunning poll)
				gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(runningResp, nil).Times(16)

				err := bs.EnsureComputeInstances()
				Expect(err).NotTo(HaveOccurred())
			})

			It("handles VMs in intermediate states (STAGING/PROVISIONING)", func() {
				instanceCalls := make(map[string]int)
				var mu sync.Mutex
				stagingResp := makeInstance("STAGING", "10.0.0.x", "1.2.3.x")
				runningResp := makeRunningInstance("10.0.0.x", "1.2.3.x")
				gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).RunAndReturn(func(projectID, zone, name string) (*computepb.Instance, error) {
					mu.Lock()
					defer mu.Unlock()
					instanceCalls[name]++
					if instanceCalls[name] == 1 {
						// First call: instance exists but is still staging
						return stagingResp, nil
					}
					// Second call via waitForInstanceRunning: now running
					return runningResp, nil
				}).Times(16)

				err := bs.EnsureComputeInstances()
				Expect(err).NotTo(HaveOccurred())
			})

			It("fails when StartInstance returns an error", func() {
				stoppedResp := makeStoppedInstance("10.0.0.x", "1.2.3.x")
				// .Maybe() because VMs are created in parallel; not all may reach StartInstance.
				gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(stoppedResp, nil).Maybe()
				gc.EXPECT().StartInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(fmt.Errorf("start error")).Maybe()

				err := bs.EnsureComputeInstances()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to start stopped instance"))
			})

			It("fails when initial GetInstance returns a non-NotFound error", func() {
				gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(nil, fmt.Errorf("permission denied")).Maybe()

				err := bs.EnsureComputeInstances()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get instance"))
			})
		})
	})

	Describe("RestartVM", func() {
		var (
			gc    *gcp.MockGCPClientManager
			csEnv *gcp.CodesphereEnvironment
			bs    *gcp.GCPBootstrapper
		)

		BeforeEach(func() {
			gc = gcp.NewMockGCPClientManager(GinkgoT())
			csEnv = &gcp.CodesphereEnvironment{
				ProjectID: "test-project",
				Zone:      "us-central1-a",
			}
			bs = newTestBootstrapper(csEnv, gc)
		})

		It("returns error for unknown VM name", func() {
			err := bs.RestartVM("nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown VM name"))
			Expect(err.Error()).To(ContainSubstring("jumpbox"))
		})

		It("is a no-op when instance is already running", func() {
			runningInst := makeRunningInstance("10.0.0.1", "1.2.3.4")
			gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, "jumpbox").Return(runningInst, nil)

			err := bs.RestartVM("jumpbox")
			Expect(err).NotTo(HaveOccurred())
		})

		It("starts a TERMINATED instance and waits for it to be running", func() {
			stoppedInst := makeStoppedInstance("10.0.0.1", "1.2.3.4")
			runningInst := makeRunningInstance("10.0.0.1", "1.2.3.4")

			gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, "jumpbox").Return(stoppedInst, nil).Once()
			gc.EXPECT().StartInstance(csEnv.ProjectID, csEnv.Zone, "jumpbox").Return(nil)
			gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, "jumpbox").Return(runningInst, nil).Once()

			err := bs.RestartVM("jumpbox")
			Expect(err).NotTo(HaveOccurred())
		})

		It("starts a STOPPED instance", func() {
			stoppedInst := makeInstance("STOPPED", "10.0.0.1", "1.2.3.4")
			runningInst := makeRunningInstance("10.0.0.1", "1.2.3.4")

			gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, "postgres").Return(stoppedInst, nil).Once()
			gc.EXPECT().StartInstance(csEnv.ProjectID, csEnv.Zone, "postgres").Return(nil)
			gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, "postgres").Return(runningInst, nil).Once()

			err := bs.RestartVM("postgres")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error for SUSPENDED instance", func() {
			suspendedInst := makeInstance("SUSPENDED", "10.0.0.1", "")
			gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, "jumpbox").Return(suspendedInst, nil)

			err := bs.RestartVM("jumpbox")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("SUSPENDED"))
			Expect(err.Error()).To(ContainSubstring("manual resume"))
		})

		It("returns error when GetInstance fails", func() {
			gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, "jumpbox").Return(nil, fmt.Errorf("permission denied"))

			err := bs.RestartVM("jumpbox")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get instance"))
		})

		It("returns actionable error when instance is not found", func() {
			gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, "jumpbox").Return(nil, grpcstatus.Errorf(codes.NotFound, "not found"))

			err := bs.RestartVM("jumpbox")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not exist"))
			Expect(err.Error()).To(ContainSubstring("bootstrap first"))
		})

		It("returns error when StartInstance fails", func() {
			stoppedInst := makeStoppedInstance("10.0.0.1", "1.2.3.4")
			gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, "jumpbox").Return(stoppedInst, nil)
			gc.EXPECT().StartInstance(csEnv.ProjectID, csEnv.Zone, "jumpbox").Return(fmt.Errorf("quota exceeded"))

			err := bs.RestartVM("jumpbox")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to start instance"))
		})

		It("handles VM without external IP (ceph node)", func() {
			stoppedInst := makeStoppedInstance("10.0.0.5", "")
			runningInst := makeRunningInstance("10.0.0.5", "")

			gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, "ceph-1").Return(stoppedInst, nil).Once()
			gc.EXPECT().StartInstance(csEnv.ProjectID, csEnv.Zone, "ceph-1").Return(nil)
			gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, "ceph-1").Return(runningInst, nil).Once()

			err := bs.RestartVM("ceph-1")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("RestartVMs", func() {
		var (
			gc    *gcp.MockGCPClientManager
			csEnv *gcp.CodesphereEnvironment
			bs    *gcp.GCPBootstrapper
		)

		BeforeEach(func() {
			gc = gcp.NewMockGCPClientManager(GinkgoT())
			csEnv = &gcp.CodesphereEnvironment{
				ProjectID: "test-project",
				Zone:      "us-central1-a",
			}
			bs = newTestBootstrapper(csEnv, gc)
		})

		It("succeeds when all VMs are already running", func() {
			runningInst := makeRunningInstance("10.0.0.1", "1.2.3.4")
			gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(runningInst, nil).Times(8)

			err := bs.RestartVMs()
			Expect(err).NotTo(HaveOccurred())
		})

		It("starts stopped VMs and succeeds", func() {
			stoppedInst := makeStoppedInstance("10.0.0.1", "1.2.3.4")
			runningInst := makeRunningInstance("10.0.0.1", "1.2.3.4")

			callCounts := map[string]int{}
			gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).RunAndReturn(func(_, _, name string) (*computepb.Instance, error) {
				callCounts[name]++
				if callCounts[name] == 1 {
					return stoppedInst, nil
				}
				return runningInst, nil
			}).Times(16)
			gc.EXPECT().StartInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(nil).Times(8)

			err := bs.RestartVMs()
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns aggregated errors when some VMs fail", func() {
			gc.EXPECT().GetInstance(csEnv.ProjectID, csEnv.Zone, mock.Anything).Return(nil, fmt.Errorf("api error")).Times(8)

			err := bs.RestartVMs()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("errors restarting VMs"))
		})
	})
})
