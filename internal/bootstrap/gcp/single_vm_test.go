// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/stretchr/testify/mock"
	"google.golang.org/protobuf/proto"
)

func TestBootstrapSingleVMPreservesProvisioningOptions(t *testing.T) {
	for _, spot := range []bool{false, true} {
		t.Run(fmt.Sprintf("spot=%t", spot), func(t *testing.T) {
			env := &gcp.CodesphereEnvironment{SpotVMs: spot}
			b, err := gcp.NewGCPBootstrapper(context.Background(), nil, nil, env, nil, nil, nil, nil, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			// Stop at package validation, before any cloud resources are touched.
			if err := b.BootstrapSingleVM("", "", "", false); err == nil {
				t.Fatal("expected missing package error")
			}
			if env.SpotVMs != spot || env.SpotOnly {
				t.Fatalf("single VM bootstrap changed provisioning options: spot=%t, spotOnly=%t", env.SpotVMs, env.SpotOnly)
			}
		})
	}
}

func TestEnsureSingleVMProvisioning(t *testing.T) {
	for _, tc := range []struct {
		name     string
		spot     bool
		existing bool
		fallback bool
	}{
		{name: "standard"},
		{name: "spot", spot: true},
		{name: "spot capacity fallback", spot: true, fallback: true},
		{name: "reuse standard", existing: true},
		{name: "reuse standard after fallback", spot: true, existing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keyPath := filepath.Join(t.TempDir(), "id_rsa.pub")
			if err := os.WriteFile(keyPath, []byte("ssh-rsa test\n"), 0600); err != nil {
				t.Fatal(err)
			}
			ready := &computepb.Instance{
				Name: proto.String("codesphere"), Status: proto.String("RUNNING"),
				NetworkInterfaces: []*computepb.NetworkInterface{{
					NetworkIP:     proto.String("10.0.0.2"),
					AccessConfigs: []*computepb.AccessConfig{{NatIP: proto.String("192.0.2.1")}},
				}},
			}
			client := gcp.NewMockGCPClientManager(t)
			if tc.existing {
				client.EXPECT().GetInstance("project", "zone", "codesphere").Return(ready, nil).Twice()
			} else {
				client.EXPECT().GetInstance("project", "zone", "codesphere").Return(nil, nil).Once()
				client.EXPECT().CreateInstance("project", "zone", mock.Anything).RunAndReturn(func(_, _ string, instance *computepb.Instance) error {
					if got := instance.GetScheduling().GetProvisioningModel() == "SPOT"; got != tc.spot {
						t.Fatalf("expected spot=%t, got scheduling %+v", tc.spot, instance.GetScheduling())
					}
					if tc.fallback {
						return fmt.Errorf("ZONE_RESOURCE_POOL_EXHAUSTED")
					}
					return nil
				}).Once()
				if tc.fallback {
					client.EXPECT().CreateInstance("project", "zone", mock.Anything).RunAndReturn(func(_, _ string, instance *computepb.Instance) error {
						if instance.GetScheduling().GetProvisioningModel() == "SPOT" {
							t.Fatal("fallback must use standard scheduling")
						}
						return nil
					}).Once()
				}
				client.EXPECT().GetInstance("project", "zone", "codesphere").Return(ready, nil).Once()
			}
			b, err := gcp.NewGCPBootstrapper(context.Background(), nil, bootstrap.NewStepLogger(false),
				&gcp.CodesphereEnvironment{ProjectID: "project", Zone: "zone", SpotVMs: tc.spot, SSHPublicKeyPath: keyPath},
				nil, client, util.NewFilesystemWriter(), nil, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := b.EnsureSingleVM(); err != nil {
				t.Fatal(err)
			}
			if b.Env.Jumpbox.GetExternalIP() != "192.0.2.1" {
				t.Fatalf("unexpected VM external IP: %s", b.Env.Jumpbox.GetExternalIP())
			}
		})
	}
}

func TestSingleVMLayout(t *testing.T) {
	env := &gcp.CodesphereEnvironment{SingleVM: true, SingleVMMachineType: "n2-standard-8"}

	defs := gcp.VMDefsForEnv(env)
	if len(defs) != 1 {
		t.Fatalf("expected exactly one VM, got %d", len(defs))
	}

	vm := defs[0]
	if vm.Name != "codesphere" || vm.MachineType != "n2-standard-8" || !vm.ExternalIP {
		t.Fatalf("unexpected single VM definition: %+v", vm)
	}

	if len(vm.AdditionalDisks) != 1 || vm.AdditionalDisks[0] != 100 {
		t.Fatalf("expected one 100 GB Ceph disk, got %v", vm.AdditionalDisks)
	}
}

func TestSingleVMDoesNotFallBackToStandard(t *testing.T) {
	client := gcp.NewMockGCPClientManager(t)
	client.EXPECT().CreateInstance("project", "zone", mock.Anything).Return(fmt.Errorf("ZONE_RESOURCE_POOL_EXHAUSTED")).Once()

	b, err := gcp.NewGCPBootstrapper(context.Background(), nil, nil,
		&gcp.CodesphereEnvironment{SpotVMs: true, SpotOnly: true}, nil, client,
		nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	err = b.CreateInstanceWithFallback("project", "zone", &computepb.Instance{}, "codesphere", make(chan string, 1))
	if err == nil {
		t.Fatal("expected Spot capacity error without a standard VM fallback")
	}
}
