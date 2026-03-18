// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"fmt"
	"sync"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/stretchr/testify/mock"
)

func protoString(s string) *string { return &s }

// makeInstance creates a computepb.Instance with the given status and IPs.
func makeInstance(status, internalIP, externalIP string) *computepb.Instance {
	inst := &computepb.Instance{
		Status: protoString(status),
		NetworkInterfaces: []*computepb.NetworkInterface{
			{
				NetworkIP: protoString(internalIP),
			},
		},
	}
	if externalIP != "" {
		inst.NetworkInterfaces[0].AccessConfigs = []*computepb.AccessConfig{
			{NatIP: protoString(externalIP)},
		}
	}
	return inst
}

// makeRunningInstance creates a RUNNING instance with both IPs assigned.
func makeRunningInstance(internalIP, externalIP string) *computepb.Instance {
	return makeInstance("RUNNING", internalIP, externalIP)
}

// makeStoppedInstance creates a TERMINATED instance with IPs from its last run.
func makeStoppedInstance(internalIP, externalIP string) *computepb.Instance {
	return makeInstance("TERMINATED", internalIP, externalIP)
}

// mockGetInstanceNotFoundThenRunning sets up a GetInstance mock where the first call per VM
// returns "not found" and subsequent calls return the given running instance.
// It sets the expected total call count to 2 × numVMs.
func mockGetInstanceNotFoundThenRunning(gc *gcp.MockGCPClientManager, projectID, zone string, runningResp *computepb.Instance, numVMs int) {
	instanceCalls := make(map[string]int)
	var mu sync.Mutex
	gc.EXPECT().GetInstance(projectID, zone, mock.Anything).RunAndReturn(func(projectID, zone, name string) (*computepb.Instance, error) {
		mu.Lock()
		defer mu.Unlock()
		instanceCalls[name]++
		if instanceCalls[name] == 1 {
			return nil, fmt.Errorf("not found")
		}
		return runningResp, nil
	}).Times(numVMs * 2)
}
