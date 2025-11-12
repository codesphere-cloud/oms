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

	// TODO: remove triplets

	switch profile {
	case PROFILE_DEV, PROFILE_DEVELOPMENT:
		g.Config.Datacenter.ID = 1
		g.Config.Datacenter.Name = "dev"
		g.Config.Datacenter.City = "Karlsruhe"
		g.Config.Datacenter.CountryCode = "DE"
		// TODO: g.config.Postgres.Mode = "install"
		g.Config.Postgres.Primary.IP = "127.0.0.1"
		g.Config.Postgres.Primary.Hostname = "localhost"
		g.Config.Ceph.NodesSubnet = "127.0.0.1/32"
		g.Config.Ceph.Hosts = []files.CephHost{{Hostname: "localhost", IPAddress: "127.0.0.1", IsMaster: true}}
		g.Config.Kubernetes.ManagedByCodesphere = true
		g.Config.Kubernetes.APIServerHost = "127.0.0.1"
		g.Config.Kubernetes.ControlPlanes = []files.K8sNode{{IPAddress: "127.0.0.1"}}
		g.Config.Kubernetes.Workers = []files.K8sNode{{IPAddress: "127.0.0.1"}}
		g.Config.Cluster.Gateway = files.GatewayConfig{ServiceType: "LoadBalancer"}
		g.Config.Cluster.PublicGateway = files.GatewayConfig{ServiceType: "LoadBalancer"}
		g.Config.Codesphere.Domain = "codesphere.local"
		g.Config.Codesphere.WorkspaceHostingBaseDomain = "ws.local"
		g.Config.Codesphere.CustomDomains.CNameBaseDomain = "custom.local"
		g.Config.Codesphere.DNSServers = []string{"8.8.8.8", "1.1.1.1"}
		g.Config.Codesphere.WorkspaceImages.Agent.BomRef = "workspace-agent-24.04"
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
		g.Config.Secrets.BaseDir = "/root/secrets"
		fmt.Println("Applied 'dev' profile: single-node development setup")

	case PROFILE_PROD, PROFILE_PRODUCTION:
		g.Config.Datacenter.ID = 1
		g.Config.Datacenter.Name = "production"
		g.Config.Datacenter.City = "Karlsruhe"
		g.Config.Datacenter.CountryCode = "DE"
		// TODO: g.config.Postgres.Mode = "install"
		g.Config.Postgres.Primary.IP = "10.50.0.2"
		g.Config.Postgres.Primary.Hostname = "pg-primary"
		g.Config.Postgres.Replica.IP = "10.50.0.3"
		g.Config.Postgres.Replica.Name = "replica1"
		g.Config.Ceph.NodesSubnet = "10.53.101.0/24"
		g.Config.Ceph.Hosts = []files.CephHost{
			{Hostname: "ceph-node-0", IPAddress: "10.53.101.2", IsMaster: true},
			{Hostname: "ceph-node-1", IPAddress: "10.53.101.3", IsMaster: false},
			{Hostname: "ceph-node-2", IPAddress: "10.53.101.4", IsMaster: false},
		}
		g.Config.Kubernetes.ManagedByCodesphere = true
		g.Config.Kubernetes.APIServerHost = "10.50.0.2"
		g.Config.Kubernetes.ControlPlanes = []files.K8sNode{
			{IPAddress: "10.50.0.2"},
		}
		g.Config.Kubernetes.Workers = []files.K8sNode{
			{IPAddress: "10.50.0.2"},
			{IPAddress: "10.50.0.3"},
			{IPAddress: "10.50.0.4"},
		}
		g.Config.Cluster.Gateway = files.GatewayConfig{ServiceType: "LoadBalancer"}
		g.Config.Cluster.PublicGateway = files.GatewayConfig{ServiceType: "LoadBalancer"}
		g.Config.Codesphere.Domain = "codesphere.yourcompany.com"
		g.Config.Codesphere.WorkspaceHostingBaseDomain = "ws.yourcompany.com"
		g.Config.Codesphere.CustomDomains.CNameBaseDomain = "custom.yourcompany.com"
		g.Config.Codesphere.DNSServers = []string{"1.1.1.1", "8.8.8.8"}
		g.Config.Codesphere.WorkspaceImages.Agent.BomRef = "workspace-agent-24.04"
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
					Name:          "Standard Developer",
					HostingPlanID: 1,
					MaxReplicas:   3,
					OnDemand:      true,
				},
			},
		}
		g.Config.Secrets.BaseDir = "/root/secrets"
		fmt.Println("Applied 'production' profile: HA multi-node setup")

	case PROFILE_MINIMAL:
		g.Config.Datacenter.ID = 1
		g.Config.Datacenter.Name = "minimal"
		g.Config.Datacenter.City = "Karlsruhe"
		g.Config.Datacenter.CountryCode = "DE"
		// TODO: g.config.Postgres.Mode = "install"
		g.Config.Postgres.Primary.IP = "127.0.0.1"
		g.Config.Postgres.Primary.Hostname = "localhost"
		g.Config.Ceph.NodesSubnet = "127.0.0.1/32"
		g.Config.Ceph.Hosts = []files.CephHost{{Hostname: "localhost", IPAddress: "127.0.0.1", IsMaster: true}}
		g.Config.Kubernetes.ManagedByCodesphere = true
		g.Config.Kubernetes.APIServerHost = "127.0.0.1"
		g.Config.Kubernetes.ControlPlanes = []files.K8sNode{{IPAddress: "127.0.0.1"}}
		g.Config.Kubernetes.Workers = []files.K8sNode{}
		g.Config.Cluster.Gateway = files.GatewayConfig{ServiceType: "LoadBalancer"}
		g.Config.Cluster.PublicGateway = files.GatewayConfig{ServiceType: "LoadBalancer"}
		g.Config.Codesphere.Domain = "codesphere.local"
		g.Config.Codesphere.WorkspaceHostingBaseDomain = "ws.local"
		g.Config.Codesphere.CustomDomains.CNameBaseDomain = "custom.local"
		g.Config.Codesphere.DNSServers = []string{"8.8.8.8"}
		g.Config.Codesphere.WorkspaceImages.Agent.BomRef = "workspace-agent-24.04"
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
					Name:          "Standard Developer",
					HostingPlanID: 1,
					MaxReplicas:   1,
					OnDemand:      true,
				},
			},
		}
		g.Config.Secrets.BaseDir = "/root/secrets"
		fmt.Println("Applied 'minimal' profile: minimal single-node setup")

	default:
		return fmt.Errorf("unknown profile: %s, available profiles: dev, prod, minimal", profile)
	}

	return nil
}
