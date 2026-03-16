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
		})
	})
})
