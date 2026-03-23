// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"context"
	"sync"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/github"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
// returns a 404 "not found" error and subsequent calls return the given running instance.
// The expected total call count is 2 × numVMs.
func mockGetInstanceNotFoundThenRunning(gc *gcp.MockGCPClientManager, projectID, zone string, runningResp *computepb.Instance, numVMs int) {
	instanceCalls := make(map[string]int)
	var mu sync.Mutex
	gc.EXPECT().GetInstance(projectID, zone, mock.Anything).RunAndReturn(func(projectID, zone, name string) (*computepb.Instance, error) {
		mu.Lock()
		defer mu.Unlock()
		instanceCalls[name]++
		if instanceCalls[name] == 1 {
			return nil, status.Errorf(codes.NotFound, "not found")
		}
		return runningResp, nil
	}).Times(numVMs * 2)
}

// newTestBootstrapper creates a GCPBootstrapper with the given environment and GCP client mock.
// All other dependencies use fresh mocks.
func newTestBootstrapper(csEnv *gcp.CodesphereEnvironment, gc gcp.GCPClientManager) *gcp.GCPBootstrapper {
	return newTestBootstrapperWithFileIO(csEnv, gc, util.NewMockFileIO(GinkgoT()))
}

// newTestBootstrapperWithFileIO creates a GCPBootstrapper with a specific FileIO mock,
// allowing tests to set expectations on file operations.
func newTestBootstrapperWithFileIO(csEnv *gcp.CodesphereEnvironment, gc gcp.GCPClientManager, fw util.FileIO) *gcp.GCPBootstrapper {
	bs, err := gcp.NewGCPBootstrapper(
		context.Background(),
		env.NewEnv(),
		bootstrap.NewStepLogger(false),
		csEnv,
		installer.NewMockInstallConfigManager(GinkgoT()),
		gc,
		fw,
		node.NewMockNodeClient(GinkgoT()),
		portal.NewMockPortal(GinkgoT()),
		util.NewFakeTime(),
		github.NewMockGitHubClient(GinkgoT()),
	)
	if err != nil {
		panic("newTestBootstrapper: " + err.Error())
	}
	return bs
}
