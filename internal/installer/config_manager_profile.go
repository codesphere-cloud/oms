// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

// prodReplicatedServices are the critical Codesphere services the prod profile runs with
// prodServiceReplicas, so the chart renders their PodDisruptionBudget and topology spread.
var prodReplicatedServices = []string{
	"auth_service",
	"deployment_service",
	"public_api_service",
	"team_service",
	"workspace_service",
}

const prodServiceReplicas = 2

const (
	PROFILE_DEV         = "dev"
	PROFILE_DEVELOPMENT = "development"
	PROFILE_PROD        = "prod"
	PROFILE_PRODUCTION  = "production"
	PROFILE_MINIMAL     = "minimal"
)

func (g *InstallConfig) applyCommonProperties() {
	if g.Config == nil {
		g.Config = &files.RootConfig{}
	}

	if g.Config.Datacenter.ID == 0 {
		g.Config.Datacenter.ID = 1
	}

	if g.Config.Datacenter.City == "" {
		g.Config.Datacenter.City = "Karlsruhe"
	}

	if g.Config.Datacenter.CountryCode == "" {
		g.Config.Datacenter.CountryCode = "DE"
	}

	if g.Config.Ceph.NodesSubnet == "" {
		g.Config.Ceph.NodesSubnet = "127.0.0.1/32"
	}

	if g.Config.Ceph.Hosts == nil {
		g.Config.Ceph.Hosts = []files.CephHost{{Hostname: "localhost", IPAddress: "127.0.0.1", IsMaster: true}}
	}

	if g.Config.Ceph.OSDs == nil {
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
	}

	if g.Config.Postgres.Mode == "" {
		g.Config.Postgres.Mode = "install"
	}

	if g.Config.Postgres.Primary == nil {
		g.Config.Postgres.Primary = &files.PostgresPrimaryConfig{
			IP:       "127.0.0.1",
			Hostname: "localhost",
		}
	}

	g.Config.Kubernetes.ManagedByCodesphere = true

	g.Config.Kubernetes.NeedsKubeConfig = false
	if g.Config.Kubernetes.APIServerHost == "" {
		g.Config.Kubernetes.APIServerHost = "127.0.0.1"
	}

	if g.Config.Kubernetes.ControlPlanes == nil {
		g.Config.Kubernetes.ControlPlanes = []files.K8sNode{{IPAddress: "127.0.0.1"}}
	}

	if g.Config.Kubernetes.Workers == nil {
		g.Config.Kubernetes.Workers = []files.K8sNode{{IPAddress: "127.0.0.1"}}
	}

	if g.Config.Cluster.Certificates.CA.Algorithm == "" {
		g.Config.Cluster.Certificates.CA = files.CAConfig{
			Algorithm:   "RSA",
			KeySizeBits: 2048,
		}
	}

	if g.Config.Cluster.Gateway.ServiceType == "" {
		g.Config.Cluster.Gateway.ServiceType = "LoadBalancer"
	}

	if g.Config.Cluster.PublicGateway.ServiceType == "" {
		g.Config.Cluster.PublicGateway.ServiceType = "LoadBalancer"
	}

	if g.Config.Cluster.MetalLB == nil {
		g.Config.Cluster.MetalLB = &files.MetalLBConfig{
			Enabled: false,
			Pools:   []files.MetalLBPoolDef{},
		}
	}

	if g.Config.Registry == nil {
		g.Config.Registry = &files.RegistryConfig{}
	}

	certIssuer := g.Config.Codesphere.EnsureCertIssuer()
	if certIssuer.Type == "" {
		certIssuer.Type = files.CertIssuerTypeSelfSigned
	}

	if g.Config.Codesphere.Domain == "" {
		g.Config.Codesphere.Domain = "codesphere.local"
	}

	if g.Config.Codesphere.WorkspaceHostingBaseDomain == "" {
		g.Config.Codesphere.WorkspaceHostingBaseDomain = "ws.local"
	}

	if g.Config.Codesphere.PublicIP == "" {
		g.Config.Codesphere.PublicIP = "127.0.0.1"
	}
	if g.Config.Codesphere.CustomDomains.CNameBaseDomain == "" {
		g.Config.Codesphere.CustomDomains.CNameBaseDomain = "custom.local"
	}

	if g.Config.Codesphere.DNSServers == nil {
		g.Config.Codesphere.DNSServers = []string{"8.8.8.8", "1.1.1.1"}
	}

	if g.Config.Codesphere.Internal == nil {
		g.Config.Codesphere.Internal = []string{}
	}

	if g.Config.Codesphere.Preview == nil {
		g.Config.Codesphere.Preview = map[string]bool{}
	}

	if g.Config.Codesphere.Features == nil {
		g.Config.Codesphere.Features = map[string]bool{}
	}

	if g.Config.Codesphere.WorkspaceImages == nil {
		g.Config.Codesphere.WorkspaceImages = &files.WorkspaceImagesConfig{
			Agent: &files.ImageRef{
				BomRef: "workspace-agent-24.04",
			},
		}
	} else if g.Config.Codesphere.WorkspaceImages.Agent == nil {
		g.Config.Codesphere.WorkspaceImages.Agent = &files.ImageRef{
			BomRef: "workspace-agent-24.04",
		}
	}

	if g.Config.Codesphere.DeployConfig.Images == nil {
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
	}

	if g.Config.Codesphere.Plans.HostingPlans == nil {
		g.Config.Codesphere.Plans.HostingPlans = map[int]files.HostingPlan{
			1: {
				CPUTenth:      10,
				GPUParts:      0,
				MemoryMb:      2048,
				StorageMb:     20480,
				TempStorageMb: 1024,
			},
		}
	}

	if g.Config.Codesphere.Plans.WorkspacePlans == nil {
		g.Config.Codesphere.Plans.WorkspacePlans = map[int]files.WorkspacePlan{
			1: {
				Name:          "Standard",
				HostingPlanID: 1,
				MaxReplicas:   3,
				OnDemand:      true,
			},
		}
	}

	if g.Config.ManagedServiceBackends == nil {
		g.Config.ManagedServiceBackends = &files.ManagedServiceBackendsConfig{}
	}

	if g.Config.ManagedServiceBackends.Postgres == nil {
		g.Config.ManagedServiceBackends.Postgres = &files.PgManagedServiceConfig{}
	}

	if g.Config.ManagedServiceBackends.BarmanCloudPlugin == nil {
		g.Config.ManagedServiceBackends.BarmanCloudPlugin = &files.BarmanCloudPluginConfig{Enabled: true}
	}

	if g.Config.Secrets.BaseDir == "" {
		g.Config.Secrets.BaseDir = "/root/secrets"
	}

	// OpenFGA's database is a CloudNativePG cluster, so every profile needs the operator.
	if g.Config.Cluster.PgOperator == nil {
		g.Config.Cluster.PgOperator = &files.PgOperatorConfig{Enabled: true}
	}
}

// ensureMonitoringDefaults populates the monitoring stack with the given enabled
// states, preserving any explicitly configured values.
func (g *InstallConfig) ensureMonitoringDefaults(clusterName string, loki, grafana, alloy bool) {
	if g.Config.Cluster.Monitoring == nil {
		g.Config.Cluster.Monitoring = &files.MonitoringConfig{}
	}

	if g.Config.Cluster.Monitoring.Prometheus == nil {
		g.Config.Cluster.Monitoring.Prometheus = &files.PrometheusConfig{}
	}

	if g.Config.Cluster.Monitoring.Prometheus.RemoteWrite == nil {
		g.Config.Cluster.Monitoring.Prometheus.RemoteWrite = &files.RemoteWriteConfig{
			Enabled:     false,
			ClusterName: clusterName,
		}
	}

	if g.Config.Cluster.Monitoring.Loki == nil {
		g.Config.Cluster.Monitoring.Loki = &files.LokiConfig{Enabled: loki}
	}

	if g.Config.Cluster.Monitoring.Grafana == nil {
		g.Config.Cluster.Monitoring.Grafana = &files.GrafanaConfig{Enabled: grafana}
	}

	if g.Config.Cluster.Monitoring.GrafanaAlloy == nil {
		g.Config.Cluster.Monitoring.GrafanaAlloy = &files.GrafanaAlloyConfig{Enabled: alloy}
	}
}

// ensureWorkspacePlans sets the default workspace plan for plan ID 1.
func (g *InstallConfig) ensureWorkspacePlans(name string, maxReplicas int) {
	g.Config.Codesphere.Plans.WorkspacePlans = map[int]files.WorkspacePlan{
		1: {
			Name:          name,
			HostingPlanID: 1,
			MaxReplicas:   maxReplicas,
			OnDemand:      true,
		},
	}
}

// ensureProdServiceReplicas runs the critical Codesphere services with prodServiceReplicas.
// Unlike the resource profiles, the existing override is merged over the defaults, so a
// replica count set in the config wins.
func (g *InstallConfig) ensureProdServiceReplicas() {
	services := map[string]any{}
	for _, service := range prodReplicatedServices {
		services[service] = map[string]any{"replicas": prodServiceReplicas}
	}

	defaults := map[string]any{
		"global": map[string]any{"services": services},
	}

	g.Config.Codesphere.Override = util.DeepMergeMaps(defaults, g.Config.Codesphere.Override)
}

// ensureKyvernoAuditPolicies keeps the kyverno-policies PDB policy in Audit mode for the
// single-replica profiles, so an Enforce default does not reject their critical workloads.
// The existing override is merged over the default, so a configured failureAction wins.
func (g *InstallConfig) ensureKyvernoAuditPolicies() {
	if g.Config.Cluster.Kyverno == nil {
		g.Config.Cluster.Kyverno = &files.KyvernoConfig{}
	}

	defaults := map[string]any{
		"pdbPolicy": map[string]any{"failureAction": "Audit"},
	}

	g.Config.Cluster.Kyverno.PoliciesOverride = util.DeepMergeMaps(defaults, g.Config.Cluster.Kyverno.PoliciesOverride)
}

func (g *InstallConfig) applyProfileDev() error {
	if g.Config.Datacenter.Name == "" {
		g.Config.Datacenter.Name = "dev"
	}

	g.ensureMonitoringDefaults("dev", false, false, false)
	g.ensureKyvernoAuditPolicies()

	if err := ApplyResourceProfile(g.Config, ResourceProfileNoRequestsSingleReplica); err != nil {
		return fmt.Errorf("applying resource profile: %w", err)
	}

	return nil
}

func (g *InstallConfig) applyProfileMinimal() error {
	if g.Config.Datacenter.Name == "" {
		g.Config.Datacenter.Name = "dev"
	}

	g.ensureMonitoringDefaults("dev", true, true, true)
	g.ensureWorkspacePlans("Standard Developer", 1)
	g.ensureKyvernoAuditPolicies()

	if g.Config.Cluster.RgwLoadBalancer == nil {
		g.Config.Cluster.RgwLoadBalancer = &files.RgwLoadBalancerConfig{
			Enabled: new(true),
		}
	}

	if err := ApplyResourceProfile(g.Config, ResourceProfileNoRequestsSingleReplica); err != nil {
		return fmt.Errorf("applying resource profile: %w", err)
	}

	return nil
}

func (g *InstallConfig) applyProfileProd() error {
	if g.Config.Datacenter.Name == "" {
		g.Config.Datacenter.Name = "production"
	}

	g.ensureWorkspacePlans("Standard Developer", 3)
	g.ensureProdServiceReplicas()
	g.Config.Cluster.Monitoring = &files.MonitoringConfig{
		Prometheus: &files.PrometheusConfig{
			RemoteWrite: &files.RemoteWriteConfig{
				Enabled:     false,
				ClusterName: "production",
			},
		},
		Loki:         &files.LokiConfig{Enabled: true},
		Grafana:      &files.GrafanaConfig{Enabled: true},
		GrafanaAlloy: &files.GrafanaAlloyConfig{Enabled: true},
	}

	return nil
}
