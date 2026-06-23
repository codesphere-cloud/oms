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
		g.Config.Cluster.Certificates = files.ClusterCertificates{
			CA: files.CAConfig{
				Algorithm:   "RSA",
				KeySizeBits: 2048,
			},
		}
	}
	if g.Config.Cluster.Gateway.ServiceType == "" {
		g.Config.Cluster.Gateway = files.GatewayConfig{ServiceType: "LoadBalancer"}
	}
	if g.Config.Cluster.PublicGateway.ServiceType == "" {
		g.Config.Cluster.PublicGateway = files.GatewayConfig{ServiceType: "LoadBalancer"}
	}
	if g.Config.MetalLB == nil {
		g.Config.MetalLB = &files.MetalLBConfig{
			Enabled: false,
			Pools:   []files.MetalLBPoolDef{},
		}
	}
	if g.Config.Registry == nil {
		g.Config.Registry = &files.RegistryConfig{}
	}

	if g.Config.Codesphere.Domain == "" {
		g.Config.Codesphere.Domain = "codesphere.local"
	}
	if g.Config.Codesphere.WorkspaceHostingBaseDomain == "" {
		g.Config.Codesphere.WorkspaceHostingBaseDomain = "ws.local"
	}
	if g.Config.Codesphere.CustomDomains.CNameBaseDomain == "" {
		g.Config.Codesphere.CustomDomains.CNameBaseDomain = "custom.local"
	}
	if g.Config.Codesphere.DNSServers == nil {
		g.Config.Codesphere.DNSServers = []string{"8.8.8.8", "1.1.1.1"}
	}
	if g.Config.Codesphere.Experiments == nil {
		g.Config.Codesphere.Experiments = []string{}
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
		g.Config.ManagedServiceBackends = &files.ManagedServiceBackendsConfig{
			Postgres: &files.PgManagedServiceConfig{},
		}
	} else if g.Config.ManagedServiceBackends.Postgres == nil {
		g.Config.ManagedServiceBackends.Postgres = &files.PgManagedServiceConfig{}
	}
	if g.Config.Codesphere.ManagedServices == nil {
		pgBackups := &files.ManagedServiceBackups{
			ConfigSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"endpointUrl": map[string]any{
						"type":        "string",
						"format":      "uri",
						"description": `S3-compatible endpoint URL for the backup storage, e.g. "http://rgw-load-balancer.rook-ceph.svc.cluster.local"`,
					},
					"destinationPath": map[string]any{
						"type":        "string",
						"format":      "uri",
						"description": `S3 bucket URI where backups are stored. Must use the s3:// scheme, e.g. "s3://backup-test"`,
					},
					"accessKeyId": map[string]any{
						"type":        "string",
						"description": "S3 access key for authentication. The associated user must have write access to the destination bucket.",
					},
				},
				"required":             []string{"endpointUrl", "destinationPath", "accessKeyId"},
				"additionalProperties": false,
			},
			SecretsSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"secretKey": map[string]any{
						"type":        "string",
						"format":      "password",
						"description": "S3 secret key for authentication",
					},
				},
				"required":             []string{"secretKey"},
				"additionalProperties": false,
			},
		}
		postgresPlans := []files.ServicePlan{
			{
				ID:          0,
				Name:        "Small",
				Description: "0.5 vCPU / 500 MB Memory",
				Parameters: map[string]files.PlanParam{
					"storage": {PricedAs: "storage-mib", Schema: map[string]interface{}{"type": "integer", "default": 10240, "readOnly": false, "minimum": 512, "x-update-constraint": "increase-only"}},
					"cpu":     {PricedAs: "cpu-tenths", Schema: map[string]interface{}{"type": "number", "default": 5, "readOnly": true}},
					"memory":  {PricedAs: "ram-mib", Schema: map[string]interface{}{"type": "integer", "default": 512, "readOnly": true}},
				},
			},
			{
				ID:          1,
				Name:        "Medium",
				Description: "1 vCPU / 1 GB Memory",
				Parameters: map[string]files.PlanParam{
					"storage": {PricedAs: "storage-mib", Schema: map[string]interface{}{"type": "integer", "default": 25600, "readOnly": false, "minimum": 512}},
					"cpu":     {PricedAs: "cpu-tenths", Schema: map[string]interface{}{"type": "number", "default": 10, "readOnly": true}},
					"memory":  {PricedAs: "ram-mib", Schema: map[string]interface{}{"type": "integer", "default": 1024, "readOnly": true}},
				},
			},
			{
				ID:          2,
				Name:        "Medium High-Mem",
				Description: "1 vCPU / 2 GB Memory",
				Parameters: map[string]files.PlanParam{
					"storage": {PricedAs: "storage-mib", Schema: map[string]interface{}{"type": "integer", "default": 25600, "readOnly": false, "minimum": 512}},
					"cpu":     {PricedAs: "cpu-tenths", Schema: map[string]interface{}{"type": "number", "default": 10, "readOnly": true}},
					"memory":  {PricedAs: "ram-mib", Schema: map[string]interface{}{"type": "integer", "default": 2048, "readOnly": true}},
				},
			},
			{
				ID:          3,
				Name:        "Large",
				Description: "2 vCPU / 4 GB Memory",
				Parameters: map[string]files.PlanParam{
					"storage": {PricedAs: "storage-mib", Schema: map[string]interface{}{"type": "integer", "default": 51200, "readOnly": false, "minimum": 512}},
					"cpu":     {PricedAs: "cpu-tenths", Schema: map[string]interface{}{"type": "number", "default": 20, "readOnly": true}},
					"memory":  {PricedAs: "ram-mib", Schema: map[string]interface{}{"type": "integer", "default": 4096, "readOnly": true}},
				},
			},
			{
				ID:          4,
				Name:        "Extra Large",
				Description: "4 vCPU / 8 GB Memory",
				Parameters: map[string]files.PlanParam{
					"storage": {PricedAs: "storage-mib", Schema: map[string]interface{}{"type": "integer", "default": 153600, "readOnly": false, "minimum": 512}},
					"cpu":     {PricedAs: "cpu-tenths", Schema: map[string]interface{}{"type": "number", "default": 40, "readOnly": true}},
					"memory":  {PricedAs: "ram-mib", Schema: map[string]interface{}{"type": "integer", "default": 8192, "readOnly": true}},
				},
			},
		}
		ferretDbPlans := make([]files.ServicePlan, len(postgresPlans))
		for i, plan := range postgresPlans {
			params := make(map[string]files.PlanParam, len(plan.Parameters)+2)
			for k, v := range plan.Parameters {
				params[k] = v
			}
			params["ferretdbCpu"] = files.PlanParam{
				PricedAs: "cpu-tenths",
				Schema:   map[string]interface{}{"type": "number", "default": 3, "readOnly": false, "minimum": 1, "maximum": 10},
			}
			params["ferretdbMemory"] = files.PlanParam{
				PricedAs: "ram-mib",
				Schema:   map[string]interface{}{"type": "integer", "default": 128, "readOnly": false, "minimum": 128, "maximum": 1024},
			}
			ferretDbPlans[i] = files.ServicePlan{ID: plan.ID, Name: plan.Name, Description: plan.Description, Parameters: params}
		}
		g.Config.Codesphere.ManagedServices = []files.ManagedServiceConfig{
			{
				Name:    "postgres",
				Version: "v1",
				Author:  "Codesphere",
				Backend: files.ManagedServiceBackend{
					API: files.ManagedServiceAPI{
						Endpoint: "http://ms-backend-postgres.postgres-operator:3000/api/v1/postgres",
					},
				},
				Category:    "Database",
				Description: "Open-source database system tailored for efficient data management and scalability. Newest version of the Provider using the Cloud-Native Operator",
				DisplayName: "PostgreSQL",
				IconURL:     "/ide/assets/managed-services/postgresql.svg",
				Scope:       "global",
				Capabilities: &files.ManagedServiceCapabilities{
					Pause:               true,
					Backups:             true,
					PointInTimeRecovery: true,
				},
				ConfigSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"version": map[string]interface{}{
							"type":                "string",
							"description":         "Version of the Postgres DB. Includes pre-installed extensions compatible with this version. Extension versions are managed and cannot be customized.",
							"enum":                []string{"17.9", "17.6", "16.13", "16.10", "15.17", "15.14", "14.22", "14.19"},
							"default":             "17.9",
							"readOnly":            false,
							"x-update-constraint": "minor-upgrade-only",
						},
						"userName": map[string]interface{}{
							"type":                "string",
							"default":             "app",
							"pattern":             "^(?!postgres$)",
							"description":         `Cannot be "postgres" (reserved for the superuser).`,
							"x-update-constraint": "immutable",
						},
						"databaseName": map[string]interface{}{
							"type":                "string",
							"default":             "app",
							"x-update-constraint": "immutable",
						},
					},
					"required":             []string{},
					"additionalProperties": false,
				},
				DetailsSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"port":     map[string]interface{}{"type": "integer"},
						"hostname": map[string]interface{}{"type": "string", "format": "hostname"},
						"dsn":      map[string]interface{}{"type": "string", "format": "uri"},
						"ready":    map[string]interface{}{"type": "boolean"},
					},
					"required":             []string{"port", "hostname", "dsn", "ready"},
					"additionalProperties": false,
				},
				SecretsSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"userPassword":      map[string]interface{}{"type": "string", "format": "password", "x-update-constraint": "immutable"},
						"superuserPassword": map[string]interface{}{"type": "string", "format": "password"},
					},
					"required":             []string{"userPassword", "superuserPassword"},
					"additionalProperties": false,
				},
				Backups: pgBackups,
				Plans:   postgresPlans,
			},
			{
				Name:    "babelfish",
				Version: "v1",
				Author:  "Codesphere",
				Backend: files.ManagedServiceBackend{
					API: files.ManagedServiceAPI{
						Endpoint: "http://ms-backend-postgres.postgres-operator:3000/api/v1/babelfish",
					},
				},
				Category:    "Database",
				Description: "PostgreSQL instance with Babelfish extension to support applications requiring Microsoft TDS compatibility",
				DisplayName: "Babelfish (T-SQL compatible)",
				IconURL:     "https://codesphere.com/ide/assets/managed-services/babelfish.svg",
				Scope:       "global",
				Capabilities: &files.ManagedServiceCapabilities{
					Pause:               true,
					Backups:             true,
					PointInTimeRecovery: true,
				},
				ConfigSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"version": map[string]interface{}{
							"type":                "string",
							"description":         "Version of the Postgres DB and the corresponding version of Babelfish",
							"enum":                []string{"17.6-5.3.0", "16.10-4.7.0"},
							"default":             "17.6-5.3.0",
							"readOnly":            false,
							"x-update-constraint": "minor-upgrade-only",
						},
					},
					"required":             []string{},
					"additionalProperties": false,
				},
				DetailsSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"port":     map[string]interface{}{"type": "integer"},
						"hostname": map[string]interface{}{"type": "string", "format": "hostname"},
						"dsn":      map[string]interface{}{"type": "string", "format": "uri", "description": "TDS connection string for the superuser and master database. Use this to connect with full administrative privileges."},
						"ready":    map[string]interface{}{"type": "boolean"},
					},
					"required":             []string{"port", "hostname", "dsn", "ready"},
					"additionalProperties": false,
				},
				SecretsSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"superuserPassword": map[string]interface{}{"type": "string", "format": "password"},
					},
					"required":             []string{"superuserPassword"},
					"additionalProperties": false,
				},
				Backups: pgBackups,
				Plans:   postgresPlans,
			},
			{
				Name:    "ferretdb",
				Version: "v0",
				Author:  "Codesphere",
				Backend: files.ManagedServiceBackend{
					API: files.ManagedServiceAPI{
						Endpoint: "http://ms-backend-postgres.postgres-operator:3000/api/v1/ferretdb",
					},
				},
				Category:    "Database",
				Description: "FerretDB based provider for MongoDB-compatible document database workloads. Powered by PostgreSQL.",
				DisplayName: "Codesphere Document DB (MongoDB compatible)",
				IconURL:     "/ide/assets/managed-services/ferretdb.svg",
				Scope:       "global",
				Capabilities: &files.ManagedServiceCapabilities{
					Pause: true,
				},
				ConfigSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"version": map[string]interface{}{
							"type":                "string",
							"description":         "Version of the Postgres / DocumentDB extension / FerretDB",
							"enum":                []string{"17-0.107.0-ferretdb-2.7.0"},
							"default":             "17-0.107.0-ferretdb-2.7.0",
							"readOnly":            false,
							"x-update-constraint": "minor-upgrade-only",
						},
					},
					"required":             []string{},
					"additionalProperties": false,
				},
				DetailsSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"port":     map[string]interface{}{"type": "integer"},
						"hostname": map[string]interface{}{"type": "string", "format": "hostname"},
						"dsn":      map[string]interface{}{"type": "string", "format": "uri", "description": "MongoDB connection string for the admin user and admin database. Use this to connect with full administrative privileges."},
						"ready":    map[string]interface{}{"type": "boolean"},
					},
					"required":             []string{"port", "hostname", "dsn", "ready"},
					"additionalProperties": false,
				},
				SecretsSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"superuserPassword": map[string]interface{}{"type": "string", "format": "password"},
					},
					"required":             []string{"superuserPassword"},
					"additionalProperties": false,
				},
				Plans: ferretDbPlans,
			},
			{
				Name:    "s3",
				Version: "v1",
				Author:  "Codesphere",
				Backend: files.ManagedServiceBackend{
					API: files.ManagedServiceAPI{
						Endpoint: "http://ms-backend-s3.rook-ceph:3000/api/v1/s3",
					},
				},
				Category:    "Storage",
				Description: "S3-compatible object storage for persisting unstructured data artifacts",
				DisplayName: "Object Storage",
				IconURL:     "/ide/assets/managed-services/s3-bucket.svg",
				Scope:       "global",
				Capabilities: &files.ManagedServiceCapabilities{
					Pause:               false,
					Backups:             false,
					PointInTimeRecovery: false,
				},
				Backups: &files.ManagedServiceBackups{
					ConfigSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"endpointUrl": map[string]any{
								"type":        "string",
								"format":      "uri",
								"description": `S3-compatible endpoint URL for the backup storage, e.g. "http://rgw-load-balancer.rook-ceph.svc.cluster.local"`,
							},
							"accessKeyId": map[string]any{
								"type":        "string",
								"description": "S3 access key for authentication at the backup store.",
							},
							"path": map[string]any{
								"type":        "string",
								"description": `S3 path (bucket name with optional subpath), without s3://, e.g. "my-bucket/backups"`,
							},
						},
						"required":             []string{"endpointUrl", "accessKeyId", "path"},
						"additionalProperties": false,
					},
					SecretsSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"secretKey": map[string]any{
								"type":        "string",
								"format":      "password",
								"description": "S3 secret key for authentication at the backup store",
							},
						},
						"required":             []string{"secretKey"},
						"additionalProperties": false,
					},
				},
				ConfigSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"accessKey": map[string]interface{}{
							"type":                "string",
							"pattern":             "^[A-Z0-9]{20}$",
							"description":         "Has to be cluster-unique. Exactly 20 uppercase letters (A-Z) or digits (0-9).",
							"x-update-constraint": "immutable",
						},
						"userDisplayName": map[string]interface{}{
							"type":                "string",
							"readOnly":            false,
							"default":             "My S3 User",
							"x-update-constraint": "immutable",
						},
						"initialBucketName": map[string]interface{}{
							"type":                "string",
							"pattern":             `^(?!\.)(?!-)(?!.*\.-)(?!.*-\.)(?!.*\.\.)[a-z0-9][a-z0-9.-]{2,62}(?<!\.|-)$`,
							"x-update-constraint": "immutable",
							"description":         "Has to be cluster-unique. If the bucket name is already taken, no bucket will be created. You can still create more buckets later. Only lowercase letters, digits, hyphens, and periods are allowed. Must start and end with a letter or digit. Hyphens and periods cannot be adjacent to each other or repeated. Between 3 and 63 characters.",
						},
					},
					"required":             []string{"accessKey", "initialBucketName"},
					"additionalProperties": false,
				},
				DetailsSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url":    map[string]interface{}{"type": "string", "format": "uri"},
						"userId": map[string]interface{}{"type": "string"},
					},
					"required":             []string{"url", "userId"},
					"additionalProperties": false,
				},
				SecretsSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"secretKey": map[string]interface{}{
							"type":        "string",
							"format":      "password",
							"pattern":     "^[a-zA-Z0-9]{40}$",
							"description": "Exactly 40 alphanumeric characters (a-z, A-Z, 0-9).",
						},
					},
					"required":             []string{"secretKey"},
					"additionalProperties": false,
				},
				Plans: []files.ServicePlan{
					{
						ID:          0,
						Name:        "Generic",
						Description: "Allows all parameters to be freely modified",
						Parameters: map[string]files.PlanParam{
							"maxBuckets":        {Schema: map[string]interface{}{"description": "Maximum Number of Buckets", "type": "integer", "default": 50, "minimum": 1, "maximum": 1000, "readOnly": false, "x-update-constraint": "increase-only"}},
							"maxObjects":        {Schema: map[string]interface{}{"description": "Maximum Number of Objects", "type": "integer", "default": 100000, "minimum": 1, "maximum": 10000000, "readOnly": false, "x-update-constraint": "increase-only"}},
							"maxSizeKb":         {Schema: map[string]interface{}{"description": "Storage (KB)", "type": "integer", "default": 10000000, "minimum": 1, "maximum": 10000000000, "readOnly": false, "x-update-constraint": "increase-only"}},
							"maxReadOpsPerS":    {Schema: map[string]interface{}{"description": "Maximum Read Operations per Second", "type": "integer", "default": 1000, "minimum": 1, "maximum": 10000, "readOnly": false}},
							"maxWriteOpsPerS":   {Schema: map[string]interface{}{"description": "Maximum Write Operations per Second", "type": "integer", "default": 1000, "minimum": 1, "maximum": 10000, "readOnly": false}},
							"maxReadBytesPerS":  {Schema: map[string]interface{}{"description": "Maximum Download Speed in Bytes per Second", "type": "integer", "default": 100000000, "minimum": 1, "maximum": 10000000000, "readOnly": false}},
							"maxWriteBytesPerS": {Schema: map[string]interface{}{"description": "Maximum Upload Speed in Bytes per Second", "type": "integer", "default": 100000000, "minimum": 1, "maximum": 10000000000, "readOnly": false}},
						},
					},
				},
			},
			{
				Name:    "virtual-k8s",
				Version: "v1",
				Author:  "Codesphere",
				Backend: files.ManagedServiceBackend{
					API: files.ManagedServiceAPI{
						Endpoint: "",
					},
				},
				Category:    "Advanced Compute",
				Description: "Virtual k8s cluster running inside of host cluster. Basis for cloud native deployments inside of Codesphere.",
				DisplayName: "Virtual Kubernetes Cluster",
				IconURL:     "/ide/assets/managed-services/k8s.svg",
				Scope:       "global",
				Capabilities: &files.ManagedServiceCapabilities{
					Pause: false,
				},
				ConfigSchema: map[string]interface{}{
					"type":                 "object",
					"properties":           map[string]interface{}{},
					"required":             []string{},
					"additionalProperties": false,
				},
				DetailsSchema: map[string]interface{}{
					"type":                 "object",
					"properties":           map[string]interface{}{},
					"required":             []string{},
					"additionalProperties": false,
				},
				SecretsSchema: map[string]interface{}{
					"type":                 "object",
					"properties":           map[string]interface{}{},
					"required":             []string{},
					"additionalProperties": false,
				},
				Plans: []files.ServicePlan{
					{
						ID:          0,
						Name:        "Custom",
						Description: "Modify all parameters",
						Parameters: map[string]files.PlanParam{
							"cpu":              {PricedAs: "cpu-tenths", Schema: map[string]interface{}{"description": "vCPU Limit", "type": "integer", "minimum": 20, "default": 20, "maximum": 160, "readOnly": false}},
							"memory":           {PricedAs: "ram-mib", Schema: map[string]interface{}{"description": "RAM limit (MiB)", "type": "integer", "minimum": 5120, "default": 5120, "maximum": 32768, "readOnly": false}},
							"storage":          {PricedAs: "storage-mib", Schema: map[string]interface{}{"description": "Storage Limit (MiB)", "type": "integer", "minimum": 20000, "default": 20000, "maximum": 120000, "readOnly": false}},
							"ephemeralStorage": {PricedAs: "storage-mib", Schema: map[string]interface{}{"description": "Ephemeral Storage Limit (MiB)", "type": "integer", "minimum": 30000, "default": 30000, "maximum": 120000, "readOnly": false}},
						},
					},
				},
			},
		}
	}
	if g.Config.Secrets.BaseDir == "" {
		g.Config.Secrets.BaseDir = "/root/secrets"
	}
}

func (g *InstallConfig) applyProfileDev() error {
	if g.Config.Datacenter.Name == "" {
		g.Config.Datacenter.Name = "dev"
	}
	if g.Config.Cluster.Monitoring == nil {
		g.Config.Cluster.Monitoring = &files.MonitoringConfig{}
	}
	if g.Config.Cluster.Monitoring.Prometheus == nil {
		g.Config.Cluster.Monitoring.Prometheus = &files.PrometheusConfig{}
	}
	if g.Config.Cluster.Monitoring.Prometheus.RemoteWrite == nil {
		g.Config.Cluster.Monitoring.Prometheus.RemoteWrite = &files.RemoteWriteConfig{
			Enabled:     false,
			ClusterName: "dev",
		}
	}
	if g.Config.Cluster.Monitoring.Loki == nil {
		g.Config.Cluster.Monitoring.Loki = &files.LokiConfig{Enabled: false}
	}
	if g.Config.Cluster.Monitoring.Grafana == nil {
		g.Config.Cluster.Monitoring.Grafana = &files.GrafanaConfig{Enabled: false}
	}
	if g.Config.Cluster.Monitoring.GrafanaAlloy == nil {
		g.Config.Cluster.Monitoring.GrafanaAlloy = &files.GrafanaAlloyConfig{Enabled: false}
	}
	if err := ApplyResourceProfile(g.Config, ResourceProfileNoRequests); err != nil {
		return fmt.Errorf("applying resource profile: %w", err)
	}
	return nil
}

func (g *InstallConfig) applyProfileMinimal() error {
	if g.Config.Datacenter.Name == "" {
		g.Config.Datacenter.Name = "dev"
	}
	if g.Config.Cluster.Monitoring == nil {
		g.Config.Cluster.Monitoring = &files.MonitoringConfig{}
	}
	if g.Config.Cluster.Monitoring.Prometheus == nil {
		g.Config.Cluster.Monitoring.Prometheus = &files.PrometheusConfig{}
	}
	if g.Config.Cluster.Monitoring.Prometheus.RemoteWrite == nil {
		g.Config.Cluster.Monitoring.Prometheus.RemoteWrite = &files.RemoteWriteConfig{
			Enabled:     false,
			ClusterName: "dev",
		}
	}
	if g.Config.Cluster.Monitoring.Loki == nil {
		g.Config.Cluster.Monitoring.Loki = &files.LokiConfig{Enabled: true}
	}
	if g.Config.Cluster.Monitoring.Grafana == nil {
		g.Config.Cluster.Monitoring.Grafana = &files.GrafanaConfig{Enabled: true}
	}
	if g.Config.Cluster.Monitoring.GrafanaAlloy == nil {
		g.Config.Cluster.Monitoring.GrafanaAlloy = &files.GrafanaAlloyConfig{Enabled: true}
	}
	if g.Config.Codesphere.Plans.WorkspacePlans == nil {
		g.Config.Codesphere.Plans.WorkspacePlans = map[int]files.WorkspacePlan{
			1: {
				Name:          "Standard Developer",
				HostingPlanID: 1,
				MaxReplicas:   1,
				OnDemand:      true,
			},
		}
	}
	if g.Config.Cluster.BarmanCloudPlugin == nil {
		g.Config.Cluster.BarmanCloudPlugin = &files.BarmanCloudPluginConfig{
			Enabled: true,
		}
	}
	if g.Config.Cluster.PgOperator == nil {
		g.Config.Cluster.PgOperator = &files.PgOperatorConfig{
			Enabled: true,
		}
	}
	if g.Config.Cluster.RgwLoadBalancer == nil {
		g.Config.Cluster.RgwLoadBalancer = &files.RgwLoadBalancerConfig{
			Enabled: true,
		}
	}

	if err := ApplyResourceProfile(g.Config, ResourceProfileNoRequests); err != nil {
		return fmt.Errorf("applying resource profile: %w", err)
	}
	return nil
}

func (g *InstallConfig) applyProfileProd() error {
	if g.Config.Datacenter.Name == "" {
		g.Config.Datacenter.Name = "production"
	}
	if g.Config.Codesphere.Plans.WorkspacePlans == nil {
		g.Config.Codesphere.Plans.WorkspacePlans = map[int]files.WorkspacePlan{
			1: {
				Name:          "Standard Developer",
				HostingPlanID: 1,
				MaxReplicas:   3,
				OnDemand:      true,
			},
		}
	}
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
