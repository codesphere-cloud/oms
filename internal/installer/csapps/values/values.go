// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package values builds the values of the cs-applications app of apps from the install config. For
// now that is the Codesphere chart values, as the Codesphere installer builds them in createValues.
package values

import (
	"fmt"

	"go.yaml.in/yaml/v3"

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
	imgs, err := images.New(b)
	if err != nil {
		return nil, fmt.Errorf("failed to read the BOM images: %w", err)
	}

	built, err := globalValues(cfg, imgs)
	if err != nil {
		return nil, err
	}

	global, err := toMap(built)
	if err != nil {
		return nil, fmt.Errorf("failed to convert the global values: %w", err)
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
	tag, err := imgs.Tag(authServiceImage)
	if err != nil {
		return "", fmt.Errorf("failed to resolve the %s image: %w", authServiceImage, err)
	}

	return tag, nil
}

// global are the chart's global values the installer builds, before the override.
type global struct {
	Fixed       fixed                   `yaml:",inline"`
	DataCenters datacenters.DataCenters `yaml:",inline"`
	Postgres    postgres.Postgres       `yaml:"postgres"`
	Ceph        ceph.Ceph               `yaml:"ceph"`
	Kubernetes  kubernetes.Kubernetes   `yaml:",inline"`
	Platform    platform.Platform       `yaml:",inline"`
	Codesphere  codesphere.Codesphere   `yaml:",inline"`
}

func globalValues(cfg *files.RootConfig, imgs images.Images) (global, error) {
	codesphereValues, err := codesphere.Values(
		cfg.Codesphere,
		imgs,
		cluster.GatewayAPIs(cfg.Cluster),
		cfg.Cluster.MetalLB,
		cfg.Registry,
	)
	if err != nil {
		return global{}, fmt.Errorf("failed to build the codesphere values: %w", err)
	}

	platformValues, err := platform.Values(imgs)
	if err != nil {
		return global{}, fmt.Errorf("failed to build the platform image values: %w", err)
	}

	pvcStorage := codesphere.EnabledFlags(cfg.Codesphere)[codesphere.WorkspacePVCStorageFlag]

	cephValues, err := ceph.Values(cfg.Ceph, pvcStorage)
	if err != nil {
		return global{}, fmt.Errorf("failed to build the ceph values: %w", err)
	}

	dataCenterValues, err := datacenters.Values(cfg)
	if err != nil {
		return global{}, fmt.Errorf("invalid data centers: %w", err)
	}

	return global{
		Fixed:       fixedValues(),
		DataCenters: dataCenterValues,
		Postgres:    postgres.Values(cfg.Postgres),
		Ceph:        cephValues,
		Kubernetes:  kubernetes.Values(cfg.Kubernetes),
		Platform:    platformValues,
		Codesphere:  codesphereValues,
	}, nil
}

// fixed are the values the installer sets the same for every private cloud.
type fixed struct {
	Env                        string                 `yaml:"env"`
	ShowPromotions             bool                   `yaml:"showPromotions"`
	PriorityClass              string                 `yaml:"priorityClass"`
	Namespace                  string                 `yaml:"namespace"`
	WorkspacePriorityClass     workspacePriorityClass `yaml:"workspacePriorityClass"`
	CustomDomainIngressClass   string                 `yaml:"customDomainIngressClass"`
	UseDedicatedWorkspaceNodes bool                   `yaml:"useDedicatedWorkspaceNodes"`
	FreeWorkspaceClusterLimit  int                    `yaml:"freeWorkspaceClusterLimit"`
	DeployCert                 bool                   `yaml:"deployCert"`
	Metrics                    metrics                `yaml:"metrics"`
	StripePriceID              string                 `yaml:"stripePriceId"`
}

type workspacePriorityClass struct {
	Free string `yaml:"free"`
	Paid string `yaml:"paid"`
}

type metrics struct {
	Type string `yaml:"type"`
}

func fixedValues() fixed {
	return fixed{
		Env:                        "prod",
		ShowPromotions:             false,
		PriorityClass:              platform.PriorityClass,
		Namespace:                  kubernetes.WorkspaceNamespace,
		WorkspacePriorityClass:     workspacePriorityClass{Free: platform.PriorityClass, Paid: "prod-paid-workspaces"},
		CustomDomainIngressClass:   "workspace-proxy-ingress",
		UseDedicatedWorkspaceNodes: false,
		FreeWorkspaceClusterLimit:  0,
		DeployCert:                 false,
		Metrics:                    metrics{Type: "log"},
		StripePriceID:              "",
	}
}

// toMap converts the built values to the plain maps the override merges into, keeping their yaml
// keys and shapes.
func toMap(values global) (map[string]any, error) {
	data, err := yaml.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: %w", err)
	}

	var plain map[string]any
	if err := yaml.Unmarshal(data, &plain); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	return plain, nil
}
