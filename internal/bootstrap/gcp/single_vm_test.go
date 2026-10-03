// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"context"
	"fmt"
	"testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/stretchr/testify/mock"
)

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
