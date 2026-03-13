// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

type ResourceProfile string

const (
	ResourceProfileNoRequests ResourceProfile = "noRequests"
)

// ApplyResourceProfile mutates a RootConfig in-place to apply the requested
// resource profile overrides.
func ApplyResourceProfile(config *files.RootConfig, profile ResourceProfile) error {
	if config == nil {
		return fmt.Errorf("root config is nil")
	}

	switch profile {
	case ResourceProfileNoRequests:
		applyNoRequestsProfile(config)
		return nil
	default:
		return fmt.Errorf("unsupported resource profile %q", profile)
	}
}

func applyNoRequestsProfile(config *files.RootConfig) {
	if config.Cluster.CertManager == nil {
		config.Cluster.CertManager = &files.CertManagerConfig{}
	}
	config.Cluster.CertManager.Override = util.DeepMergeMaps(config.Cluster.CertManager.Override, map[string]any{
		"cert-manager": map[string]any{
			"resources": map[string]any{
				"requests": zeroRequests(),
			},
			"cainjector": map[string]any{
				"resources": map[string]any{
					"requests": zeroRequests(),
				},
			},
			"webhook": map[string]any{
				"replicaCount": 1,
				"resources": map[string]any{
					"requests": zeroRequests(),
				},
			},
			"startupapicheck": map[string]any{
				"resources": map[string]any{
					"requests": zeroRequests(),
				},
			},
		},
	})

	if config.Cluster.Monitoring == nil {
		config.Cluster.Monitoring = &files.MonitoringConfig{}
	}
	if config.Cluster.Monitoring.Prometheus == nil {
		config.Cluster.Monitoring.Prometheus = &files.PrometheusConfig{}
	}
	config.Cluster.Monitoring.Prometheus.Override = util.DeepMergeMaps(config.Cluster.Monitoring.Prometheus.Override, map[string]any{
		"kube-prometheus-stack": map[string]any{
			"prometheusOperator": map[string]any{
				"resources": map[string]any{
					"requests": zeroRequests(),
				},
			},
			"prometheus": map[string]any{
				"prometheusSpec": map[string]any{
					"resources": map[string]any{
						"requests": zeroRequests(),
					},
				},
			},
			"prometheus-node-exporter": map[string]any{
				"resources": map[string]any{
					"requests": zeroRequests(),
				},
			},
			"kube-state-metrics": map[string]any{
				"resources": map[string]any{
					"requests": zeroRequests(),
				},
			},
		},
	})

	if config.Cluster.Monitoring.BlackboxExporter == nil {
		config.Cluster.Monitoring.BlackboxExporter = &files.BlackboxExporterConfig{}
	}
	config.Cluster.Monitoring.BlackboxExporter.Override = util.DeepMergeMaps(config.Cluster.Monitoring.BlackboxExporter.Override, map[string]any{
		"prometheus-blackbox-exporter": map[string]any{
			"replicas": 1,
			"resources": map[string]any{
				"requests": zeroRequests(),
			},
		},
	})

	if config.Cluster.Monitoring.PushGateway == nil {
		config.Cluster.Monitoring.PushGateway = &files.PushGatewayConfig{}
	}
	config.Cluster.Monitoring.PushGateway.Override = util.DeepMergeMaps(config.Cluster.Monitoring.PushGateway.Override, map[string]any{
		"prometheus-pushgateway": map[string]any{
			"resources": map[string]any{
				"requests": zeroRequests(),
			},
		},
	})

	config.Cluster.Gateway.Override = util.DeepMergeMaps(config.Cluster.Gateway.Override, map[string]any{
		"ingress-nginx": map[string]any{
			"controller": map[string]any{
				"replicaCount": 1,
				"resources": map[string]any{
					"requests": zeroRequests(),
				},
			},
		},
	})

	config.Cluster.PublicGateway.Override = util.DeepMergeMaps(config.Cluster.PublicGateway.Override, map[string]any{
		"ingress-nginx": map[string]any{
			"controller": map[string]any{
				"replicaCount": 1,
				"resources": map[string]any{
					"requests": zeroRequests(),
				},
			},
		},
		"nginx": map[string]any{
			"replicaCount": 1,
			"resources": map[string]any{
				"requests": zeroRequests(),
			},
		},
	})

	if config.Codesphere.Override == nil {
		config.Codesphere.Override = map[string]any{}
	}

	serviceProfiles := map[string]any{}
	for _, serviceName := range []string{
		"auth_service",
		"deployment_service",
		"error_page_server",
		"ide_frontend",
		"ide_service",
		"marketplace",
		"payment_service",
		"public_api_service",
		"team_service",
		"workspace_proxy",
		"workspace_service",
	} {
		serviceProfiles[serviceName] = map[string]any{
			"requests": zeroRequests(),
		}
	}

	config.Codesphere.Override = util.DeepMergeMaps(config.Codesphere.Override, map[string]any{
		"global": map[string]any{
			"services": serviceProfiles,
		},
	})
}

func zeroRequests() map[string]int {
	return map[string]int{
		"cpu":    0,
		"memory": 0,
	}
}
