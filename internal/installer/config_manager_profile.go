// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const (
	PROFILE_DEV         = "dev"
	PROFILE_DEVELOPMENT = "development"
	PROFILE_PROD        = "prod"
	PROFILE_PRODUCTION  = "production"
	PROFILE_MINIMAL     = "minimal"
)

func (g *InstallConfig) ApplyProfile(profile string) error {
	if g.Config == nil {
		g.Config = &files.RootConfig{}
	}

	g.Config.Ceph.NodesSubnet = "127.0.0.1/32"
	g.Config.Ceph.Hosts = []files.CephHost{{Hostname: "localhost", IPAddress: "127.0.0.1", IsMaster: true}}
	g.Config.Ceph.OSDs = []files.CephOSD{
		{
			SpecID: "default",
			Placement: files.CephPlacement{
				HostPattern: "*",
			},
			DataDevices: files.CephDataDevices{
				Size:  "240G:300G",
				Limit: 1,
			},
			DBDevices: files.CephDBDevices{
				Size:  "120G:150G",
				Limit: 1,
			},
		},
	}

	g.Config.Datacenter.ID = 1
	g.Config.Datacenter.City = "Karlsruhe"
	g.Config.Datacenter.CountryCode = "DE"

	g.Config.Postgres.Mode = "install"
	g.Config.Postgres.Primary = &files.PostgresPrimaryConfig{
		IP:       "127.0.0.1",
		Hostname: "localhost",
	}

	g.Config.Kubernetes.ManagedByCodesphere = true
	g.Config.Kubernetes.NeedsKubeConfig = false
	g.Config.Kubernetes.APIServerHost = "127.0.0.1"
	g.Config.Kubernetes.ControlPlanes = []files.K8sNode{{IPAddress: "127.0.0.1"}}
	g.Config.Kubernetes.Workers = []files.K8sNode{{IPAddress: "127.0.0.1"}}

	g.Config.Cluster.Certificates = files.ClusterCertificates{
		CA: files.CAConfig{
			Algorithm:   "RSA",
			KeySizeBits: 2048,
		},
	}
	g.Config.Cluster.Gateway = files.GatewayConfig{ServiceType: "LoadBalancer"}
	g.Config.Cluster.PublicGateway = files.GatewayConfig{ServiceType: "LoadBalancer"}
	g.Config.MetalLB = &files.MetalLBConfig{
		Enabled: false,
		Pools:   []files.MetalLBPoolDef{},
	}
	g.Config.Registry = &files.RegistryConfig{}

	g.Config.Codesphere.Domain = "codesphere.local"
	g.Config.Codesphere.WorkspaceHostingBaseDomain = "ws.local"
	g.Config.Codesphere.CustomDomains.CNameBaseDomain = "custom.local"
	g.Config.Codesphere.DNSServers = []string{"8.8.8.8", "1.1.1.1"}
	g.Config.Codesphere.Experiments = []string{}
	g.Config.Codesphere.WorkspaceImages = &files.WorkspaceImagesConfig{
		Agent: &files.ImageRef{
			BomRef: "workspace-agent-24.04",
		},
	}
	g.Config.Codesphere.DeployConfig = files.DeployConfig{
		Images: map[string]files.ImageConfig{
			"ubuntu-24.04": {
				Name:           "Ubuntu 24.04",
				SupportedUntil: "2028-05-31",
				Flavors: map[string]files.FlavorConfig{
					"default": {
						Image: files.ImageRef{
							BomRef: "workspace-agent-24.04",
						},
						Pool: map[int]int{1: 1},
					},
				},
			},
		},
	}
	g.Config.Codesphere.Plans = files.PlansConfig{
		HostingPlans: map[int]files.HostingPlan{
			1: {
				CPUTenth:      10,
				GPUParts:      0,
				MemoryMb:      2048,
				StorageMb:     20480,
				TempStorageMb: 1024,
			},
		},
		WorkspacePlans: map[int]files.WorkspacePlan{
			1: {
				Name:          "Standard",
				HostingPlanID: 1,
				MaxReplicas:   3,
				OnDemand:      true,
			},
		},
	}
	g.Config.ManagedServiceBackends = &files.ManagedServiceBackendsConfig{
		Postgres: make(map[string]interface{}),
	}
	g.Config.Codesphere.ManagedServices = []files.ManagedServiceConfig{
		{
			Name:    "postgres",
			Version: "v1",
		},
		{
			Name:    "babelfish",
			Version: "v1",
		},
		{
			Name:    "s3",
			Version: "v1",
		},
		{
			Name:    "virtual-k8s",
			Version: "v1",
		},
	}
	g.Config.Secrets.BaseDir = "/root/secrets"

	switch profile {
	case PROFILE_DEV, PROFILE_DEVELOPMENT:
		g.Config.Datacenter.Name = "dev"
		if err := ApplyResourceProfile(g.Config, ResourceProfileNoRequests); err != nil {
			return fmt.Errorf("applying resource profile: %w", err)
		}
		g.Config.Cluster.Monitoring = &files.MonitoringConfig{
			Prometheus: &files.PrometheusConfig{
				RemoteWrite: &files.RemoteWriteConfig{
					Enabled:     false,
					ClusterName: "local-test",
				},
			},
			Loki:         &files.LokiConfig{Enabled: false},
			Grafana:      &files.GrafanaConfig{Enabled: false},
			GrafanaAlloy: &files.GrafanaAlloyConfig{Enabled: false},
		}

	case PROFILE_PROD, PROFILE_PRODUCTION:
		g.Config.Datacenter.Name = "production"
		g.Config.Codesphere.Plans.WorkspacePlans = map[int]files.WorkspacePlan{
			1: {
				Name:          "Standard Developer",
				HostingPlanID: 1,
				MaxReplicas:   3,
				OnDemand:      true,
			},
		}

	case PROFILE_MINIMAL:
		g.Config.Datacenter.Name = "minimal"
		g.Config.Codesphere.Plans.WorkspacePlans = map[int]files.WorkspacePlan{
			1: {
				Name:          "Standard Developer",
				HostingPlanID: 1,
				MaxReplicas:   1,
				OnDemand:      true,
			},
		}

	default:
		return fmt.Errorf("unknown profile: %s, available profiles: dev, prod, minimal", profile)
	}

	return nil
}
