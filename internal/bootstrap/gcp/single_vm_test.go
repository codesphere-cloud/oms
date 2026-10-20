// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"context"
	"errors"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"google.golang.org/protobuf/proto"
)

var _ = Describe("Single VM provisioning", func() {
	var (
		b      *gcp.GCPBootstrapper
		client *gcp.MockGCPClientManager
		fw     *util.MockFileIO
		ready  *computepb.Instance
	)

	BeforeEach(func() {
		client = gcp.NewMockGCPClientManager(GinkgoT())
		fw = util.NewMockFileIO(GinkgoT())

		var err error

		b, err = gcp.NewGCPBootstrapper(context.Background(), nil, bootstrap.NewStepLogger(true),
			&gcp.CodesphereEnvironment{ProjectID: "project", Zone: "zone", SSHPublicKeyPath: "/keys/id_rsa.pub"},
			nil, client, fw, nil, nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		ready = &computepb.Instance{
			Name: proto.String("codesphere"), Status: proto.String("RUNNING"),
			NetworkInterfaces: []*computepb.NetworkInterface{{
				NetworkIP:     proto.String("10.0.0.2"),
				AccessConfigs: []*computepb.AccessConfig{{NatIP: proto.String("192.0.2.1")}},
			}},
		}
	})

	It("preserves standard provisioning at package validation", func() {
		Expect(b.BootstrapSingleVM("", "", "", false)).To(HaveOccurred())
		Expect(b.Env.SpotVMs).To(BeFalse())
		Expect(b.Env.SpotOnly).To(BeFalse())
	})

	It("preserves Spot provisioning at package validation", func() {
		b.Env.SpotVMs = true
		Expect(b.BootstrapSingleVM("", "", "", false)).To(HaveOccurred())
		Expect(b.Env.SpotVMs).To(BeTrue())
		Expect(b.Env.SpotOnly).To(BeFalse())
	})

	It("creates a standard VM", func() {
		fw.EXPECT().ReadFile("/keys/id_rsa.pub").Return([]byte("ssh-rsa test\n"), nil)
		client.EXPECT().GetInstance("project", "zone", "codesphere").Return(nil, nil).Once()
		client.EXPECT().CreateInstance("project", "zone", mock.MatchedBy(func(instance *computepb.Instance) bool {
			return instance.GetScheduling().GetProvisioningModel() != "SPOT"
		})).Return(nil).Once()
		client.EXPECT().GetInstance("project", "zone", "codesphere").Return(ready, nil).Once()
		Expect(b.EnsureSingleVM()).To(Succeed())
		Expect(b.Env.Jumpbox.GetExternalIP()).To(Equal("192.0.2.1"))
	})

	It("creates a Spot VM when requested", func() {
		b.Env.SpotVMs = true

		fw.EXPECT().ReadFile("/keys/id_rsa.pub").Return([]byte("ssh-rsa test\n"), nil)
		client.EXPECT().GetInstance("project", "zone", "codesphere").Return(nil, nil).Once()
		client.EXPECT().CreateInstance("project", "zone", mock.MatchedBy(func(instance *computepb.Instance) bool {
			return instance.GetScheduling().GetProvisioningModel() == "SPOT"
		})).Return(nil).Once()
		client.EXPECT().GetInstance("project", "zone", "codesphere").Return(ready, nil).Once()
		Expect(b.EnsureSingleVM()).To(Succeed())
		Expect(b.Env.Jumpbox.GetExternalIP()).To(Equal("192.0.2.1"))
	})

	It("falls back to a standard VM when Spot capacity is exhausted", func() {
		b.Env.SpotVMs = true

		fw.EXPECT().ReadFile("/keys/id_rsa.pub").Return([]byte("ssh-rsa test\n"), nil)
		client.EXPECT().GetInstance("project", "zone", "codesphere").Return(nil, nil).Once()
		client.EXPECT().CreateInstance("project", "zone", mock.MatchedBy(func(instance *computepb.Instance) bool {
			return instance.GetScheduling().GetProvisioningModel() == "SPOT"
		})).Return(errors.New("ZONE_RESOURCE_POOL_EXHAUSTED")).Once()
		client.EXPECT().CreateInstance("project", "zone", mock.MatchedBy(func(instance *computepb.Instance) bool {
			return instance.GetScheduling().GetProvisioningModel() != "SPOT"
		})).Return(nil).Once()
		client.EXPECT().GetInstance("project", "zone", "codesphere").Return(ready, nil).Once()
		Expect(b.EnsureSingleVM()).To(Succeed())
	})

	It("reuses a running VM", func() {
		fw.EXPECT().ReadFile("/keys/id_rsa.pub").Return([]byte("ssh-rsa test\n"), nil)
		client.EXPECT().GetInstance("project", "zone", "codesphere").Return(ready, nil).Twice()
		Expect(b.EnsureSingleVM()).To(Succeed())
		Expect(b.Env.Jumpbox.GetExternalIP()).To(Equal("192.0.2.1"))
	})

	It("reuses a standard VM created by an earlier Spot fallback", func() {
		b.Env.SpotVMs = true

		fw.EXPECT().ReadFile("/keys/id_rsa.pub").Return([]byte("ssh-rsa test\n"), nil)
		client.EXPECT().GetInstance("project", "zone", "codesphere").Return(ready, nil).Twice()
		Expect(b.EnsureSingleVM()).To(Succeed())
	})

	It("defines one public VM with the requested machine type and a Ceph disk", func() {
		defs := gcp.VMDefsForEnv(&gcp.CodesphereEnvironment{SingleVM: true, SingleVMMachineType: "n2-standard-8"})
		Expect(defs).To(HaveLen(1))
		Expect(defs[0].Name).To(Equal("codesphere"))
		Expect(defs[0].MachineType).To(Equal("n2-standard-8"))
		Expect(defs[0].ExternalIP).To(BeTrue())
		Expect(defs[0].AdditionalDisks).To(Equal([]int64{100}))
	})

	It("does not fall back when Spot-only provisioning is required", func() {
		b.Env.SpotVMs = true
		b.Env.SpotOnly = true

		client.EXPECT().CreateInstance("project", "zone", mock.Anything).Return(errors.New("ZONE_RESOURCE_POOL_EXHAUSTED")).Once()
		Expect(b.CreateInstanceWithFallback("project", "zone", &computepb.Instance{}, "codesphere", make(chan string, 1))).To(HaveOccurred())
	})
})
