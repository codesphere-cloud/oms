// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"context"
	"fmt"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ = Describe("Spot VM", func() {

	Describe("isSpotCapacityError", func() {
		It("returns false for nil error", func() {
			Expect(gcp.IsSpotCapacityError(nil)).To(BeFalse())
		})

		It("detects gRPC ResourceExhausted status", func() {
			err := status.Errorf(codes.ResourceExhausted, "resource exhausted")
			Expect(gcp.IsSpotCapacityError(err)).To(BeTrue())
		})

		It("does not match non-ResourceExhausted gRPC codes", func() {
			Expect(gcp.IsSpotCapacityError(status.Errorf(codes.NotFound, "not found"))).To(BeFalse())
			Expect(gcp.IsSpotCapacityError(status.Errorf(codes.PermissionDenied, "denied"))).To(BeFalse())
			Expect(gcp.IsSpotCapacityError(status.Errorf(codes.Internal, "internal"))).To(BeFalse())
		})

		It("detects ZONE_RESOURCE_POOL_EXHAUSTED error string", func() {
			err := fmt.Errorf("googleapi: Error 403: ZONE_RESOURCE_POOL_EXHAUSTED - the zone does not have enough resources")
			Expect(gcp.IsSpotCapacityError(err)).To(BeTrue())
		})

		It("detects UNSUPPORTED_OPERATION error string", func() {
			err := fmt.Errorf("UNSUPPORTED_OPERATION: spot VMs not available in this zone")
			Expect(gcp.IsSpotCapacityError(err)).To(BeTrue())
		})

		It("detects stockout error string", func() {
			err := fmt.Errorf("stockout in zone us-central1-a")
			Expect(gcp.IsSpotCapacityError(err)).To(BeTrue())
		})

		It("detects does not have enough resources error string", func() {
			err := fmt.Errorf("the zone 'us-central1-a' does not have enough resources available to fulfill the request")
			Expect(gcp.IsSpotCapacityError(err)).To(BeTrue())
		})

		It("does not match unrelated error strings", func() {
			Expect(gcp.IsSpotCapacityError(fmt.Errorf("permission denied"))).To(BeFalse())
			Expect(gcp.IsSpotCapacityError(fmt.Errorf("invalid argument"))).To(BeFalse())
			Expect(gcp.IsSpotCapacityError(fmt.Errorf("quota exceeded"))).To(BeFalse())
			Expect(gcp.IsSpotCapacityError(fmt.Errorf("network error"))).To(BeFalse())
		})

		It("detects gRPC ResourceExhausted with additional message text", func() {
			err := status.Errorf(codes.ResourceExhausted, "spot VM pool exhausted in us-central1-a")
			Expect(gcp.IsSpotCapacityError(err)).To(BeTrue())
		})

		It("does not match gRPC Unavailable status", func() {
			err := status.Errorf(codes.Unavailable, "service unavailable")
			Expect(gcp.IsSpotCapacityError(err)).To(BeFalse())
		})
	})

	Describe("buildSchedulingConfig", func() {
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

			var err error
			bs, err = gcp.NewGCPBootstrapper(
				context.Background(),
				env.NewEnv(),
				bootstrap.NewStepLogger(false),
				csEnv,
				installer.NewMockInstallConfigManager(GinkgoT()),
				gcp.NewMockGCPClientManager(GinkgoT()),
				util.NewMockFileIO(GinkgoT()),
				node.NewMockNodeClient(GinkgoT()),
				portal.NewMockPortal(GinkgoT()),
				util.NewFakeTime(),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when spot is enabled", func() {
			BeforeEach(func() {
				csEnv.Spot = true
				csEnv.Preemptible = false
			})

			It("sets SPOT provisioning model", func() {
				sched := bs.BuildSchedulingConfig()
				Expect(sched.ProvisioningModel).NotTo(BeNil())
				Expect(*sched.ProvisioningModel).To(Equal("SPOT"))
			})

			It("sets OnHostMaintenance to TERMINATE", func() {
				sched := bs.BuildSchedulingConfig()
				Expect(sched.OnHostMaintenance).NotTo(BeNil())
				Expect(*sched.OnHostMaintenance).To(Equal("TERMINATE"))
			})

			It("sets AutomaticRestart to false", func() {
				sched := bs.BuildSchedulingConfig()
				Expect(sched.AutomaticRestart).NotTo(BeNil())
				Expect(*sched.AutomaticRestart).To(BeFalse())
			})

			It("sets InstanceTerminationAction to STOP", func() {
				sched := bs.BuildSchedulingConfig()
				Expect(sched.InstanceTerminationAction).NotTo(BeNil())
				Expect(*sched.InstanceTerminationAction).To(Equal("STOP"))
			})

			It("returns a complete scheduling config with all four fields set", func() {
				sched := bs.BuildSchedulingConfig()
				Expect(sched).NotTo(BeNil())
				Expect(*sched.ProvisioningModel).To(Equal("SPOT"))
				Expect(*sched.OnHostMaintenance).To(Equal("TERMINATE"))
				Expect(*sched.AutomaticRestart).To(BeFalse())
				Expect(*sched.InstanceTerminationAction).To(Equal("STOP"))
			})

			It("does not set Preemptible field", func() {
				sched := bs.BuildSchedulingConfig()
				Expect(sched.Preemptible).To(BeNil())
			})
		})

		Context("when preemptible is enabled", func() {
			BeforeEach(func() {
				csEnv.Spot = false
				csEnv.Preemptible = true
			})

			It("sets Preemptible to true", func() {
				sched := bs.BuildSchedulingConfig()
				Expect(sched.Preemptible).NotTo(BeNil())
				Expect(*sched.Preemptible).To(BeTrue())
			})

			It("does not set SPOT provisioning model", func() {
				sched := bs.BuildSchedulingConfig()
				Expect(sched.ProvisioningModel).To(BeNil())
			})

			It("does not set OnHostMaintenance", func() {
				sched := bs.BuildSchedulingConfig()
				Expect(sched.OnHostMaintenance).To(BeNil())
			})

			It("does not set AutomaticRestart", func() {
				sched := bs.BuildSchedulingConfig()
				Expect(sched.AutomaticRestart).To(BeNil())
			})

			It("does not set InstanceTerminationAction", func() {
				sched := bs.BuildSchedulingConfig()
				Expect(sched.InstanceTerminationAction).To(BeNil())
			})
		})

		Context("when neither spot nor preemptible is enabled", func() {
			BeforeEach(func() {
				csEnv.Spot = false
				csEnv.Preemptible = false
			})

			It("returns empty scheduling config", func() {
				sched := bs.BuildSchedulingConfig()
				Expect(sched.ProvisioningModel).To(BeNil())
				Expect(sched.Preemptible).To(BeNil())
				Expect(sched.OnHostMaintenance).To(BeNil())
				Expect(sched.AutomaticRestart).To(BeNil())
				Expect(sched.InstanceTerminationAction).To(BeNil())
			})
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
				GithubAppClientID:     "fake-id",
				GithubAppClientSecret: "fake-secret",
			}
		})

		newBootstrapper := func() *gcp.GCPBootstrapper {
			bs, err := gcp.NewGCPBootstrapper(
				context.Background(),
				env.NewEnv(),
				bootstrap.NewStepLogger(false),
				csEnv,
				installer.NewMockInstallConfigManager(GinkgoT()),
				gcp.NewMockGCPClientManager(GinkgoT()),
				util.NewMockFileIO(GinkgoT()),
				node.NewMockNodeClient(GinkgoT()),
				portal.NewMockPortal(GinkgoT()),
				util.NewFakeTime(),
			)
			Expect(err).NotTo(HaveOccurred())
			return bs
		}

		It("succeeds when only spot is set", func() {
			csEnv.Spot = true
			csEnv.Preemptible = false
			bs := newBootstrapper()
			err := bs.ValidateInput()
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds when only preemptible is set", func() {
			csEnv.Spot = false
			csEnv.Preemptible = true
			bs := newBootstrapper()
			err := bs.ValidateInput()
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds when neither is set", func() {
			csEnv.Spot = false
			csEnv.Preemptible = false
			bs := newBootstrapper()
			err := bs.ValidateInput()
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails when both spot and preemptible are set", func() {
			csEnv.Spot = true
			csEnv.Preemptible = true
			bs := newBootstrapper()
			err := bs.ValidateInput()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot specify both --spot and --preemptible"))
		})

		It("error message suggests using --spot over --preemptible", func() {
			csEnv.Spot = true
			csEnv.Preemptible = true
			bs := newBootstrapper()
			err := bs.ValidateInput()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("use --spot for the newer spot VM model"))
		})

		It("spot does not interfere with other validation (install version)", func() {
			csEnv.Spot = true
			csEnv.InstallVersion = "v1.0.0"
			csEnv.InstallHash = "abc123"
			csEnv.RegistryType = gcp.RegistryTypeGitHub

			mockPortal := portal.NewMockPortal(GinkgoT())
			mockPortal.EXPECT().GetBuild(portal.CodesphereProduct, "v1.0.0", "abc123").Return(portal.Build{
				Artifacts: []portal.Artifact{{Filename: "installer-lite.tar.gz"}},
				Hash:      "abc123",
				Version:   "v1.0.0",
			}, nil)

			bs, err := gcp.NewGCPBootstrapper(
				context.Background(),
				env.NewEnv(),
				bootstrap.NewStepLogger(false),
				csEnv,
				installer.NewMockInstallConfigManager(GinkgoT()),
				gcp.NewMockGCPClientManager(GinkgoT()),
				util.NewMockFileIO(GinkgoT()),
				node.NewMockNodeClient(GinkgoT()),
				mockPortal,
				util.NewFakeTime(),
			)
			Expect(err).NotTo(HaveOccurred())
			err = bs.ValidateInput()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("createInstanceWithFallback", func() {
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

			var err error
			bs, err = gcp.NewGCPBootstrapper(
				context.Background(),
				env.NewEnv(),
				bootstrap.NewStepLogger(false),
				csEnv,
				installer.NewMockInstallConfigManager(GinkgoT()),
				gc,
				util.NewMockFileIO(GinkgoT()),
				node.NewMockNodeClient(GinkgoT()),
				portal.NewMockPortal(GinkgoT()),
				util.NewFakeTime(),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when creation succeeds on first attempt", func() {
			It("returns nil", func() {
				instance := &computepb.Instance{Name: protoString("test-vm")}
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", instance).Return(nil)

				err := bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when the instance already exists", func() {
			It("returns nil (treats as success)", func() {
				instance := &computepb.Instance{Name: protoString("test-vm")}
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", instance).
					Return(status.Errorf(codes.AlreadyExists, "already exists"))

				err := bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when spot is enabled", func() {
			BeforeEach(func() {
				csEnv.Spot = true
			})

			DescribeTable("falls back to standard VM on capacity errors",
				func(capacityErr error) {
					instance := &computepb.Instance{
						Name: protoString("test-vm"),
						Scheduling: &computepb.Scheduling{
							ProvisioningModel: protoString("SPOT"),
						},
					}

					gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
						Return(capacityErr).Once()
					gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
						Return(nil).Once()

					err := bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)
					Expect(err).NotTo(HaveOccurred())

					Expect(logCh).To(Receive(ContainSubstring("falling back to standard VM")))
				},
				Entry("gRPC ResourceExhausted", status.Errorf(codes.ResourceExhausted, "exhausted")),
				Entry("ZONE_RESOURCE_POOL_EXHAUSTED string", fmt.Errorf("ZONE_RESOURCE_POOL_EXHAUSTED")),
				Entry("UNSUPPORTED_OPERATION string", fmt.Errorf("UNSUPPORTED_OPERATION")),
				Entry("stockout string", fmt.Errorf("stockout in zone")),
				Entry("does not have enough resources string", fmt.Errorf("zone does not have enough resources")),
			)

			It("clears scheduling config on fallback", func() {
				instance := &computepb.Instance{
					Name: protoString("test-vm"),
					Scheduling: &computepb.Scheduling{
						ProvisioningModel: protoString("SPOT"),
					},
				}

				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
					Return(fmt.Errorf("ZONE_RESOURCE_POOL_EXHAUSTED")).Once()
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.MatchedBy(func(inst *computepb.Instance) bool {
					return inst.Scheduling != nil &&
						inst.Scheduling.ProvisioningModel == nil &&
						inst.Scheduling.Preemptible == nil
				})).Return(nil).Once()

				err := bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns error when fallback also fails", func() {
				instance := &computepb.Instance{
					Name: protoString("test-vm"),
					Scheduling: &computepb.Scheduling{
						ProvisioningModel: protoString("SPOT"),
					},
				}

				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
					Return(fmt.Errorf("ZONE_RESOURCE_POOL_EXHAUSTED")).Once()
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
					Return(fmt.Errorf("quota exceeded")).Once()

				err := bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fallback to standard VM"))
			})

			It("does NOT fall back when error is not capacity-related", func() {
				instance := &computepb.Instance{
					Name: protoString("test-vm"),
					Scheduling: &computepb.Scheduling{
						ProvisioningModel: protoString("SPOT"),
					},
				}

				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
					Return(fmt.Errorf("permission denied")).Once()

				err := bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create instance test-vm"))
				Expect(err.Error()).To(ContainSubstring("permission denied"))
				Expect(logCh).To(BeEmpty())
			})

			It("succeeds when fallback retry returns AlreadyExists", func() {
				instance := &computepb.Instance{
					Name: protoString("test-vm"),
					Scheduling: &computepb.Scheduling{
						ProvisioningModel: protoString("SPOT"),
					},
				}

				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
					Return(status.Errorf(codes.ResourceExhausted, "exhausted")).Once()
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
					Return(status.Errorf(codes.AlreadyExists, "already exists")).Once()

				err := bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)
				Expect(err).NotTo(HaveOccurred())

				Expect(logCh).To(Receive(ContainSubstring("falling back to standard VM")))
			})

			It("logs the correct VM name in the fallback message", func() {
				instance := &computepb.Instance{
					Name: protoString("ceph-3"),
					Scheduling: &computepb.Scheduling{
						ProvisioningModel: protoString("SPOT"),
					},
				}

				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
					Return(fmt.Errorf("ZONE_RESOURCE_POOL_EXHAUSTED")).Once()
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
					Return(nil).Once()

				err := bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "ceph-3", logCh)
				Expect(err).NotTo(HaveOccurred())

				var msg string
				Expect(logCh).To(Receive(&msg))
				Expect(msg).To(ContainSubstring("ceph-3"))
			})

			It("wraps the original error when fallback fails", func() {
				instance := &computepb.Instance{
					Name: protoString("test-vm"),
					Scheduling: &computepb.Scheduling{
						ProvisioningModel: protoString("SPOT"),
					},
				}

				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
					Return(fmt.Errorf("stockout in zone")).Once()
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
					Return(fmt.Errorf("insufficient quota")).Once()

				err := bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fallback to standard VM"))
				Expect(err.Error()).To(ContainSubstring("insufficient quota"))
			})
		})

		Context("when spot is disabled", func() {
			BeforeEach(func() {
				csEnv.Spot = false
			})

			It("does not fall back on capacity errors", func() {
				instance := &computepb.Instance{Name: protoString("test-vm")}
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", instance).
					Return(fmt.Errorf("ZONE_RESOURCE_POOL_EXHAUSTED"))

				err := bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create instance test-vm"))
			})

			It("propagates non-capacity errors", func() {
				instance := &computepb.Instance{Name: protoString("test-vm")}
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", instance).
					Return(fmt.Errorf("permission denied"))

				err := bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create instance test-vm"))
				Expect(err.Error()).To(ContainSubstring("permission denied"))
			})

			It("returns nil for string-based already exists error", func() {
				instance := &computepb.Instance{Name: protoString("test-vm")}
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", instance).
					Return(fmt.Errorf("The resource 'test-vm' already exists"))

				err := bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("CodesphereEnvironment Spot/Preemptible fields", func() {
		It("defaults Spot to false", func() {
			csEnv := &gcp.CodesphereEnvironment{}
			Expect(csEnv.Spot).To(BeFalse())
		})

		It("defaults Preemptible to false", func() {
			csEnv := &gcp.CodesphereEnvironment{}
			Expect(csEnv.Preemptible).To(BeFalse())
		})

		It("can set Spot independently", func() {
			csEnv := &gcp.CodesphereEnvironment{Spot: true}
			Expect(csEnv.Spot).To(BeTrue())
			Expect(csEnv.Preemptible).To(BeFalse())
		})

		It("can set Preemptible independently", func() {
			csEnv := &gcp.CodesphereEnvironment{Preemptible: true}
			Expect(csEnv.Preemptible).To(BeTrue())
			Expect(csEnv.Spot).To(BeFalse())
		})
	})
})
