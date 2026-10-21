// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package values builds the values of the cs-applications app of apps from the install config. For
// now that is the Codesphere chart values, as the Codesphere installer builds them in createValues.
package values

import (
	"fmt"

	"maps"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/ceph"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/cluster"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/datacenters"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/images"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/kubernetes"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/platform"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/postgres"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// Build returns the Codesphere chart values for the data center the config installs, with external
// secrets enabled: the release Secret then comes from the vault, so the values hold no secrets.
func Build(cfg *files.RootConfig, b *bom.Config) (map[string]any, error) {
	imgs, err := images.New(b, cfg.Registry)
	if err != nil {
		return nil, fmt.Errorf("failed to read the BOM images: %w", err)
	}

	topology, err := datacenters.Resolve(cfg)
	if err != nil {
		return nil, fmt.Errorf("invalid data centers: %w", err)
	}

	global, err := globalValues(cfg, imgs, topology)
	if err != nil {
		return nil, err
	}

	imageTag, err := releaseImageTag(imgs)
	if err != nil {
		return nil, fmt.Errorf("failed to read the release image tag: %w", err)
	}

	values := mergeValues(map[string]any{"global": global}, cfg.Codesphere.Override)

	return mergeValues(values, map[string]any{
		"global": map[string]any{
			"imageTag": imageTag,
			"hostName": hostName(cfg.Codesphere),
		},
	}), nil
}

// hostName prefers the override's hostName: the installer sets it after the override, so it would
// otherwise replace the override's.
func hostName(conf files.CodesphereConfig) string {
	if global, ok := conf.Override["global"].(map[string]any); ok {
		if name, ok := global["hostName"].(string); ok {
			return name
		}
	}

	return conf.Domain
}

const authServiceImage = "auth-service"

// releaseImageTag is the tag of all Codesphere service images in the BOM. The chart pins every
// service to it; like the old pc-installer, it is read from the auth-service image.
func releaseImageTag(imgs images.Images) (string, error) {
	authService, err := imgs.Ref(authServiceImage)
	if err != nil {
		return "", fmt.Errorf("failed to resolve the %s image: %w", authServiceImage, err)
	}

	if authService.Tag == "" {
		return "", fmt.Errorf("image %s has no tag to deploy Codesphere with", authServiceImage)
	}

	return authService.Tag, nil
}

func globalValues(
	cfg *files.RootConfig,
	imgs images.Images,
	topology datacenters.Topology,
) (map[string]any, error) {
	codesphereValues, err := codesphere.Values(
		cfg.Codesphere,
		imgs,
		cluster.GatewayAPIs(cfg.Cluster),
		cfg.Cluster.MetalLB,
		cfg.Registry,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build the codesphere values: %w", err)
	}

	platformValues, err := platform.Values(imgs)
	if err != nil {
		return nil, fmt.Errorf("failed to build the platform image values: %w", err)
	}

	pvcStorage := codesphere.EnabledFlags(cfg.Codesphere)[codesphere.WorkspacePVCStorageFlag]
	sections := []map[string]any{
		fixedValues(),
		datacenters.Values(topology),
		postgres.Values(cfg.Postgres),
		ceph.Values(cfg.Ceph, pvcStorage),
		kubernetes.Values(cfg.Kubernetes),
		platformValues,
		codesphereValues,
	}

	global := map[string]any{}
	for _, section := range sections {
		maps.Copy(global, section)
	}

	return global, nil
}

// fixedValues are the values the installer sets the same for every private cloud.
func fixedValues() map[string]any {
	return map[string]any{
		"env":            "prod",
		"showPromotions": false,
		"priorityClass":  platform.PriorityClass,
		"namespace":      kubernetes.WorkspaceNamespace,
		"workspacePriorityClass": map[string]any{
			"free": platform.PriorityClass,
			"paid": "prod-paid-workspaces",
		},
		"customDomainIngressClass":   "workspace-proxy-ingress",
		"useDedicatedWorkspaceNodes": false,
		"freeWorkspaceClusterLimit":  0,
		"deployCert":                 false,
		"metrics":                    map[string]any{"type": "log"},
		"mssqlDbServer":              "",
		"stripePriceId":              "",
	}
}
