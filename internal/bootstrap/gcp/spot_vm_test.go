// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"fmt"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ = Describe("Spot VM", func() {

	Describe("IsSpotCapacityError", func() {
		It("returns false for nil error", func() {
			Expect(gcp.IsSpotCapacityError(nil)).To(BeFalse())
		})

		DescribeTable("detects known capacity errors",
			func(err error) { Expect(gcp.IsSpotCapacityError(err)).To(BeTrue()) },
			Entry("gRPC ResourceExhausted", status.Errorf(codes.ResourceExhausted, "resource exhausted")),
			Entry("gRPC ResourceExhausted with detail", status.Errorf(codes.ResourceExhausted, "spot VM pool exhausted in us-central1-a")),
			Entry("ZONE_RESOURCE_POOL_EXHAUSTED", fmt.Errorf("googleapi: Error 403: ZONE_RESOURCE_POOL_EXHAUSTED - the zone does not have enough resources")),
			Entry("UNSUPPORTED_OPERATION", fmt.Errorf("UNSUPPORTED_OPERATION: spot VMs not available in this zone")),
			Entry("stockout", fmt.Errorf("stockout in zone us-central1-a")),
			Entry("does not have enough resources", fmt.Errorf("the zone 'us-central1-a' does not have enough resources available to fulfill the request")),
		)

		DescribeTable("rejects non-capacity errors",
			func(err error) { Expect(gcp.IsSpotCapacityError(err)).To(BeFalse()) },
			Entry("NotFound", status.Errorf(codes.NotFound, "not found")),
			Entry("PermissionDenied", status.Errorf(codes.PermissionDenied, "denied")),
			Entry("Internal", status.Errorf(codes.Internal, "internal")),
			Entry("Unavailable", status.Errorf(codes.Unavailable, "service unavailable")),
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
			csEnv.Spot = true
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
				csEnv.Spot = spot
				csEnv.Preemptible = preemptible
				gc := gcp.NewMockGCPClientManager(GinkgoT())
				bs := newTestBootstrapper(csEnv, gc)
				Expect(bs.ValidateInput()).NotTo(HaveOccurred())
			},
			Entry("only spot", true, false),
			Entry("only preemptible", false, true),
			Entry("neither", false, false),
		)

		It("fails when both spot and preemptible are set", func() {
			csEnv.Spot = true
			csEnv.Preemptible = true
			gc := gcp.NewMockGCPClientManager(GinkgoT())
			bs := newTestBootstrapper(csEnv, gc)
			err := bs.ValidateInput()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot specify both --spot and --preemptible"))
			Expect(err.Error()).To(ContainSubstring("use --spot for the newer spot VM model"))
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
				Return(status.Errorf(codes.AlreadyExists, "already exists"))

			Expect(bs.CreateInstanceWithFallback("test-pid", "us-central1-a", instance, "test-vm", logCh)).To(Succeed())
		})

		Context("when spot is enabled", func() {
			BeforeEach(func() {
				csEnv.Spot = true
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
				Entry("gRPC ResourceExhausted", status.Errorf(codes.ResourceExhausted, "exhausted")),
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
					Return(status.Errorf(codes.ResourceExhausted, "exhausted")).Once()
				gc.EXPECT().CreateInstance("test-pid", "us-central1-a", mock.Anything).
					Return(status.Errorf(codes.AlreadyExists, "already exists")).Once()

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
})
