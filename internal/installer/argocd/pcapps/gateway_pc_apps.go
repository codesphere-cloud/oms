// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/chartvalues"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

// GatewayPcAppsValues transfers each gateway to ArgoCD once its legacy TS chart
// is absent from the package. Chart defaults supply image references; the
// registry admission policy rewrites them when Pods are created.
func GatewayPcAppsValues(config *files.RootConfig, packageBOM *bom.Config) files.ChartValues {
	if packageBOM == nil {
		return nil
	}

	gateway := util.DeepMergeMaps(
		commonGatewayValues(config.Cluster.Gateway),
		gatewayOverrides(config.Cluster.Gateway),
	)
	publicGateway := util.DeepMergeMaps(
		commonGatewayValues(config.Cluster.PublicGateway),
		publicGatewayValues(config),
	)

	applications := util.DeepMergeMaps(
		gatewayApplicationValues("gateway", packageBOM, gateway, config.Cluster.Gateway.Override),
		gatewayApplicationValues("public-gateway", packageBOM, publicGateway, config.Cluster.PublicGateway.Override),
	)

	if len(applications) == 0 {
		return nil
	}

	return files.ChartValues{"applications": applications}
}

func gatewayApplicationValues(name string, packageBOM *bom.Config, values files.ChartValues, override files.ChartOverride) files.ChartValues {
	if _, legacy := packageBOM.Components[name].Files["chart"]; legacy {
		return nil
	}

	return files.ChartValues{name: files.ChartValues{
		"enabled":      true,
		"valuesObject": util.DeepMergeMaps(values, override),
	}}
}

func commonGatewayValues(config files.GatewayConfig) files.ChartValues {
	enabled := true
	if config.GatewayAPI != nil {
		enabled = config.GatewayAPI.Enabled
	}

	useGatewayAPI := gatewayAPIInUse(config)

	controller := files.ChartValues{
		"config":  privateGatewayControllerConfig(),
		"service": files.ChartValues{"enabled": !useGatewayAPI},
		"metrics": files.ChartValues{"serviceMonitor": files.ChartValues{"enabled": false}},
	}
	if config.NodeSelector != nil {
		controller["nodeSelector"] = config.NodeSelector
	}

	service := gatewayServiceValues(config)

	controller["service"] = util.DeepMergeMaps(controller["service"].(files.ChartValues), service)
	proxy := files.ChartValues{"imagePullSecrets": chartvalues.RegistryPullSecrets(), "service": files.ChartValues{"type": "ClusterIP"}}

	if useGatewayAPI {
		proxyService := gatewayServiceValues(config)
		delete(proxyService, "externalIPs")

		proxy["service"] = proxyService
		if config.IPAddresses != nil {
			proxy["externalIPs"] = config.IPAddresses
		}
	}

	values := files.ChartValues{
		"ingress-nginx": files.ChartValues{"imagePullSecrets": chartvalues.RegistryPullSecrets(), "controller": controller},
		"gateway":       files.ChartValues{"enabled": enabled, "overrideIngressNginxService": useGatewayAPI, "proxy": proxy},
	}

	return values
}

func gatewayServiceValues(config files.GatewayConfig) files.ChartValues {
	values := files.ChartValues{}
	if config.ServiceType != "" {
		values["type"] = config.ServiceType
	}

	if config.Annotations != nil {
		values["annotations"] = config.Annotations
	}

	if config.IPAddresses != nil {
		values["externalIPs"] = config.IPAddresses
	}

	return values
}

func gatewayOverrides(config files.GatewayConfig) files.ChartValues {
	replicas := 3
	if gatewayAPIInUse(config) {
		replicas = 0
	}

	return files.ChartValues{
		"global": files.ChartValues{"imagePullSecrets": chartvalues.RegistryPullSecrets()},
		"ingress-nginx": files.ChartValues{"controller": files.ChartValues{
			"replicaCount": replicas,
			"config":       files.ChartValues{"worker-processes": "2"},
			"resources": files.ChartValues{
				"requests": files.ChartValues{"cpu": "400m", "memory": "1Gi"},
				"limits":   files.ChartValues{"cpu": "2", "memory": "3Gi"},
			},
		}},
	}
}

func gatewayAPIInUse(config files.GatewayConfig) bool {
	if config.GatewayAPI == nil || config.GatewayAPI.Use == nil {
		return true
	}

	return *config.GatewayAPI.Use
}

// These are the values-private-cloud.yaml defaults previously loaded by TS.
func privateGatewayControllerConfig() files.ChartValues {
	return files.ChartValues{"service": files.ChartValues{"annotations": files.ChartValues{
		"service.beta.kubernetes.io/port_80_health-probe_request-path":  "/healthz",
		"service.beta.kubernetes.io/port_443_health-probe_request-path": "/healthz",
		"service.beta.kubernetes.io/port_443_health-probe_protocol":     "https",
	}}}
}

func publicGatewayValues(config *files.RootConfig) files.ChartValues {
	replicas := 2
	if gatewayAPIInUse(config.Cluster.PublicGateway) {
		// Retained during the nginx-to-Gateway-API migration, as in the TS installer.
		replicas = 1
	}

	wildcard := "*." + config.Codesphere.WorkspaceHostingBaseDomain

	nginx := files.ChartValues{
		"ingress": files.ChartValues{
			"issuer": certIssuerName(config), "hostname": wildcard,
			"extraTls": []any{files.ChartValues{"hosts": []string{wildcard}, "secretName": "workspace-reverse-proxy-tls"}},
			"annotations": files.ChartValues{
				"nginx.ingress.kubernetes.io/auth-signin": "https://" + config.Codesphere.Domain + "/ide/signin?devDomain=$scheme://$host$request_uri",
				"nginx.ingress.kubernetes.io/auth-url":    "http://auth-service.codesphere.svc.cluster.local:3010/public/authForDevDomain?devDomain=$scheme://$host$request_uri",
			},
		},
		"extraVolumes": []any{}, "extraVolumeMounts": []any{},
		"extraEnvVars": []any{
			chartvalues.EnvVar("DOMAIN_NAME", strings.ReplaceAll(config.Codesphere.WorkspaceHostingBaseDomain, ".", `\\.`)),
			chartvalues.EnvVar("PLATFORM_DOMAIN", config.Codesphere.Domain),
			chartvalues.EnvVar("ERROR_PAGE_SERVER_HOSTNAME", "error-page-server"),
			chartvalues.EnvVar("IDE_SERVICE_HOSTNAME", "ide-service"),
			files.ChartValues{
				"name": "CPU_LIMIT",
				"valueFrom": files.ChartValues{
					"resourceFieldRef": files.ChartValues{"resource": "limits.cpu", "divisor": "1"},
				},
			},
		},
		"image": files.ChartValues{"pullSecrets": chartvalues.RegistryPullSecretNames()},
		"resources": files.ChartValues{
			"requests": files.ChartValues{"cpu": "400m", "memory": "256Mi"},
			"limits":   files.ChartValues{"cpu": "1000m", "memory": "256Mi"},
		},
	}
	if certificate := config.Codesphere.WorkspaceHostingCustomCertificate; certificate != nil {
		nginx["certificate"] = files.ChartValues{"cert": certificate.Cert, "key": certificate.Key}
	}

	return files.ChartValues{
		"ingress-nginx":        files.ChartValues{"controller": files.ChartValues{"replicaCount": replicas}},
		"workspacesDevGateway": files.ChartValues{"name": "workspaces-dev-gateway", "host": files.ChartValues{"name": wildcard, "tlsSecretName": "workspace-reverse-proxy-tls"}, "extraHosts": []any{}},
		"nginx":                nginx,
	}
}
