// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package codespherevalues builds the Codesphere chart values from the install config, as the
// Codesphere installer does in createValues.
package codespherevalues

import (
	"fmt"
	"slices"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const (
	prodEnv                      = "prod"
	prodPriorityClass            = "prod-codesphere-deployment"
	workspaceNamespace           = "workspaces"
	codesphereDatabase           = "codesphere"
	internalPostgresPort         = 5432
	selfSignedIssuerName         = "self-signed"
	defaultManagedServicePlanID  = 20
	defaultCentralOtelEndpoint   = "https://otel.tools.codesphere.com"
	workspacePVCStorageFlag      = "workspace-pvc-storage"
	acmeEABMacKeyName            = "acmeEabMacKey"
	acmeCustomDomainsEABMacKey   = "acmeCustomDomainsEabMacKey"
	authServiceImage             = "auth-service"
	defaultKubernetesPodCIDR     = "100.96.0.0/11"
	defaultKubernetesServiceCIDR = "100.64.0.0/13"
)

// baseOAuthProviders are the built-in OAuth providers, all disabled in private clouds.
var baseOAuthProviders = []string{"bitbucket", "github", "gitlab", "google", "facebook"}

// serviceImages maps each service in the chart to its image in the BOM.
var serviceImages = map[string]string{
	"auth_service":       "auth-service",
	"deployment_service": "deployment-service",
	"error_page_server":  "error-page-server",
	"ide_frontend":       "ide-frontend",
	"ide_service":        "ide-service",
	"marketplace":        "marketplace",
	"payment_service":    "payment-service",
	"public_api_service": "public-api-service",
	"team_service":       "team-service",
	"workspace_proxy":    "workspace-proxy",
	"workspace_service":  "workspace-service",
}

// Build returns the Codesphere chart values for the data center the config installs, with external
// secrets enabled: the release Secret then comes from the vault, so the values hold no secrets.
func Build(cfg *files.RootConfig, b *bom.Config) (map[string]any, error) {
	images, err := newBOMImages(b, cfg.Registry)
	if err != nil {
		return nil, err
	}

	topology, err := resolveDataCenterTopology(cfg)
	if err != nil {
		return nil, err
	}

	global, err := globalValues(cfg, images, topology)
	if err != nil {
		return nil, err
	}

	authService, err := images.ref(authServiceImage)
	if err != nil {
		return nil, err
	}

	if authService.tag == "" {
		return nil, fmt.Errorf("image %s has no tag to deploy Codesphere with", authServiceImage)
	}

	values := mergeValues(map[string]any{"global": global}, cfg.Codesphere.Override)

	return mergeValues(values, map[string]any{
		"global": map[string]any{
			"imageTag": authService.tag,
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

func globalValues(cfg *files.RootConfig, images bomImages, topology dataCenterTopology) (map[string]any, error) {
	conf := cfg.Codesphere
	if err := validate(conf); err != nil {
		return nil, err
	}

	global := map[string]any{
		"env":                           prodEnv,
		"showPromotions":                false,
		"publicIP":                      conf.PublicIP,
		"certIssuer":                    certIssuerValues(conf.CertIssuer),
		"workspaceHostingBaseDomain":    conf.WorkspaceHostingBaseDomain,
		"sshWorkspaceProxy":             omitEmpty(map[string]any{"baseDomain": sshBaseDomain(conf.SSHWorkspaceProxy)}),
		"vpn":                           omitEmpty(map[string]any{"leftPeerId": conf.VPNLeftPeerID}),
		"dnsServers":                    dnsServers(conf.DNSServers),
		"hosts":                         []string{conf.Domain},
		"priorityClass":                 prodPriorityClass,
		"namespace":                     workspaceNamespace,
		"workspacePriorityClass":        map[string]any{"free": prodPriorityClass, "paid": "prod-paid-workspaces"},
		"customDomainIngressClass":      "workspace-proxy-ingress",
		"useDedicatedWorkspaceNodes":    false,
		"freeWorkspaceClusterLimit":     0,
		"poolDefinition":                poolValues(pool(conf.Plans.HostingPlans, func(files.HostingPlan) bool { return true })),
		"prePullImages":                 boolOr(conf.PrePullImages, true),
		"gitProviders":                  gitProvidersValues(conf.GitProviders),
		"experiments":                   experimentsValues(conf, workspacesGateway(cfg).enabled),
		"features":                      featuresValues(conf.Features),
		"deployCert":                    false,
		"customDomains":                 customDomainsValues(conf.CustomDomains, workspacesGateway(cfg).use),
		"metrics":                       map[string]any{"type": "log"},
		"ceph":                          cephValues(cfg.Ceph, enabledFlags(conf)),
		"networkPolicies":               networkPoliciesValues(cfg.Kubernetes),
		"oauth":                         oauthValues(conf.OAuth),
		"signUp":                        signUpValues(conf.SignUp),
		"mssqlDbServer":                 "",
		"stripePriceId":                 "",
		"managedServiceWorkspacePlanId": intOr(conf.ManagedServiceWorkspacePlanID, defaultManagedServicePlanID),
		"postgres":                      postgresValues(cfg.Postgres),
		"otel":                          otelValues(conf.TelemetryExport),
	}
	setDataCenterValues(global, topology)
	setOptionalValues(global, conf)

	if err := setImageValues(global, cfg, images); err != nil {
		return nil, err
	}

	if err := setIPServiceValues(global, cfg); err != nil {
		return nil, err
	}

	if err := setManagedServicesValues(global, conf.ManagedServices); err != nil {
		return nil, err
	}

	return global, nil
}

func validate(conf files.CodesphereConfig) error {
	if len(conf.DeployConfig.Images) > 0 {
		for _, plan := range conf.Plans.HostingPlans {
			if plan.PooledInstances != nil && *plan.PooledInstances != 0 {
				return fmt.Errorf("configuring .deployConfig AND .pooledInstances is ambiguous (remove the .pooledInstances config)")
			}
		}
	}

	export := conf.TelemetryExport
	if export != nil && export.Traces && strings.TrimSpace(export.TraceEndpoint) == "" {
		return fmt.Errorf("telemetryExport.traceEndpoint must be set if telemetryExport.traces is true")
	}

	return nil
}

func setDataCenterValues(global map[string]any, topology dataCenterTopology) {
	ids := make([]int, len(topology.available))

	dataCenters := make([]any, len(topology.available))
	for i, dc := range topology.available {
		ids[i] = dc.ID
		dataCenters[i] = map[string]any{"id": dc.ID, "name": dc.Name, "city": dc.City, "countryCode": dc.CountryCode}
	}

	global["dataCenterId"] = topology.current.ID
	global["dataCenterName"] = topology.current.Name
	global["defaultDataCenterId"] = topology.defaultID
	global["availableDataCenters"] = ids
	global["dataCenters"] = dataCenters
}

// setOptionalValues sets the values the installer passes through only when the config has them.
func setOptionalValues(global map[string]any, conf files.CodesphereConfig) {
	optional := map[string]any{
		"branding":              brandingValues(conf.Branding),
		"tlsCertificate":        customCertificateValues(conf.CustomCertificate),
		"underprovisionFactors": underprovisionFactorsValues(conf.UnderprovisionFactors),
		"extraWorkspaceFiles":   extraWorkspaceFilesValues(conf.ExtraWorkspaceFiles),
		"smtp":                  smtpValues(conf.Email),
		"openBao":               openBaoValues(conf.OpenBao),
		"openFga":               openFgaValues(conf.OpenFga),
	}
	if conf.ExtraCAPem != "" {
		optional["extraCaPem"] = conf.ExtraCAPem
	}

	if conf.ExtraWorkspaceEnvVars != nil {
		// As a plain map, so the override merges into it key by key.
		envVars := make(map[string]any, len(conf.ExtraWorkspaceEnvVars))
		for name, value := range conf.ExtraWorkspaceEnvVars {
			envVars[name] = value
		}

		optional["extraWorkspaceEnvVars"] = envVars
	}

	for key, value := range optional {
		if value != nil {
			global[key] = value
		}
	}
}

func setImageValues(global map[string]any, cfg *files.RootConfig, images bomImages) error {
	deployConfig, err := deployConfigValues(cfg, images)
	if err != nil {
		return err
	}

	workspaceImages, err := workspaceImagesValues(cfg.Codesphere.WorkspaceImages, images)
	if err != nil {
		return err
	}

	services, err := servicesValues(images)
	if err != nil {
		return err
	}

	workspaceRouter, err := workspaceRouterValues(images)
	if err != nil {
		return err
	}

	frontendGateway, err := frontendGatewayValues(images, cfg.Codesphere.CertIssuer, codesphereGateway(cfg).enabled)
	if err != nil {
		return err
	}

	workspaceObservability, err := workspaceObservabilityValues(images)
	if err != nil {
		return err
	}

	global["deployConfig"] = deployConfig
	global["workspaceImages"] = workspaceImages
	global["services"] = services
	global["workspaceRouter"] = workspaceRouter
	global["frontendGateway"] = frontendGateway
	global["workspaceObservability"] = workspaceObservability

	return nil
}

func setIPServiceValues(global map[string]any, cfg *files.RootConfig) error {
	ipService, err := ipServiceValues(cfg.Codesphere.IPService, cfg.Cluster.MetalLB)
	if err != nil {
		return err
	}

	if ipService != nil {
		global["ipService"] = ipService
	}

	return nil
}

func setManagedServicesValues(global map[string]any, services []files.ManagedServiceConfig) error {
	values, err := managedServicesValues(services)
	if err != nil {
		return err
	}

	if values != nil {
		global["managedServices"] = values
	}

	return nil
}

func certIssuerValues(issuer *files.CertIssuerConfig) map[string]any {
	acme := map[string]any{"enabled": false}
	if issuer == nil || issuer.Type != files.CertIssuerTypeACME || issuer.Acme == nil {
		return map[string]any{"acme": acme}
	}

	acme["enabled"] = true
	acme["server"] = issuer.Acme.Server

	acme["email"] = issuer.Acme.Email
	if keyID, macKeyName := customDomainsEAB(issuer.Acme); keyID != "" {
		acme["eabKeyId"] = keyID
		acme["eabMacKeyName"] = macKeyName
	}

	return map[string]any{"acme": acme}
}

// customDomainsEAB returns the EAB key the custom-domain issuer registers with and the vault entry of
// its MAC key. It falls back to the cluster issuer's key for configs without a separate one.
func customDomainsEAB(acme *files.ACMEConfig) (keyID, macKeyName string) {
	if acme.CustomDomainsEABKeyID != "" {
		return acme.CustomDomainsEABKeyID, acmeCustomDomainsEABMacKey
	}

	if acme.EABKeyID != "" {
		return acme.EABKeyID, acmeEABMacKeyName
	}

	return "", ""
}

func dnsServers(configured []string) []string {
	if configured == nil {
		return []string{"8.8.8.8"}
	}

	return configured
}

func gitProvidersValues(providers *files.GitProvidersConfig) map[string]any {
	if providers == nil {
		providers = &files.GitProvidersConfig{}
	}

	values := map[string]any{
		"azureDevOps": gitProviderValues(providers.AzureDevOps),
		"github":      gitProviderValues(providers.GitHub),
		"gitlab":      gitProviderValues(providers.GitLab),
		"bitbucket":   gitProviderValues(providers.Bitbucket),
	}

	scm := providers.SCMManager
	if scm == nil || !scm.Enabled {
		values["scmManager"] = map[string]any{"enabled": false}
		return values
	}

	scmValues := map[string]any{"enabled": true, "url": scm.URL, "api": map[string]any{}}
	if scm.API != nil {
		scmValues["api"] = map[string]any{"baseUrl": scm.API.BaseURL}
	}

	values["scmManager"] = scmValues

	return values
}

func gitProviderValues(provider *files.GitProviderConfig) map[string]any {
	if provider == nil || !provider.Enabled {
		return map[string]any{"enabled": false}
	}

	oauth := map[string]any{
		"issuer":                provider.OAuth.Issuer,
		"authorizationEndpoint": provider.OAuth.AuthorizationEndpoint,
		"tokenEndpoint":         provider.OAuth.TokenEndpoint,
	}

	optional := map[string]string{
		"redirectUri":      provider.OAuth.RedirectURI,
		"installationUri":  provider.OAuth.InstallationURI,
		"clientAuthMethod": provider.OAuth.ClientAuthMethod,
		"scope":            provider.OAuth.Scope,
	}
	for key, value := range optional {
		if value != "" {
			oauth[key] = value
		}
	}

	return map[string]any{
		"enabled": true,
		"url":     provider.URL,
		"api":     map[string]any{"baseUrl": provider.API.BaseURL},
		"oauth":   oauth,
	}
}

// enabledFlags are the flags the installer enables from the config's internal, deprecated
// experiments, preview and feature flags.
func enabledFlags(conf files.CodesphereConfig) map[string]bool {
	flags := map[string]bool{}
	for _, flag := range slices.Concat(conf.Internal, conf.Experiments) { //nolint:staticcheck // The installer still reads the deprecated experiments.
		flags[flag] = true
	}

	for _, toggles := range []map[string]bool{conf.Preview, conf.Features} {
		for flag, enabled := range toggles {
			if enabled {
				flags[flag] = true
			}
		}
	}

	return flags
}

func experimentsValues(conf files.CodesphereConfig, workspacesGatewayEnabled bool) map[string]any {
	experiments := map[string]any{
		"hermetic":       []string{prodEnv},
		"persistent-nix": []string{prodEnv},
	}
	if workspacesGatewayEnabled {
		experiments["gateway-domains"] = []string{prodEnv}
	}

	for _, flag := range slices.Concat(conf.Experiments, conf.Internal) { //nolint:staticcheck // The installer still reads the deprecated experiments.
		experiments[flag] = []string{prodEnv}
	}

	for flag, enabled := range conf.Preview {
		if enabled {
			experiments[flag] = []string{prodEnv}
		}
	}

	return experiments
}

func featuresValues(configured map[string]bool) map[string]any {
	features := map[string]any{
		"email-signup":     []string{prodEnv},
		"email-signin":     []string{prodEnv},
		"standalone-teams": []string{prodEnv},
		"billing":          []string{},
	}

	for feature, enabled := range configured {
		if enabled {
			features[feature] = []string{prodEnv}
		} else {
			features[feature] = []string{}
		}
	}

	return features
}

func customDomainsValues(conf files.CustomDomainsConfig, useGateway bool) map[string]any {
	values := map[string]any{
		"cNameBaseDomain": conf.CNameBaseDomain,
		"gatewayApi":      map[string]any{"useGateway": useGateway},
	}
	if conf.InsecureSkipDomainControlVerify != nil {
		values["insecureSkipDomainControlVerify"] = *conf.InsecureSkipDomainControlVerify
	}

	if conf.Cert != nil {
		cert := map[string]any{}
		if conf.Cert.Algorithm != "" {
			cert["algorithm"] = conf.Cert.Algorithm
		}

		if conf.Cert.Size != 0 {
			cert["size"] = conf.Cert.Size
		}

		values["cert"] = cert
	}

	return values
}

type gatewayAPI struct {
	enabled bool
	use     bool
}

func gatewayAPIOf(conf files.GatewayConfig) gatewayAPI {
	if conf.GatewayAPI == nil {
		return gatewayAPI{enabled: true, use: true}
	}

	return gatewayAPI{enabled: conf.GatewayAPI.Enabled, use: boolOr(conf.GatewayAPI.Use, true)}
}

// codesphereGateway serves Codesphere itself.
func codesphereGateway(cfg *files.RootConfig) gatewayAPI {
	return gatewayAPIOf(cfg.Cluster.Gateway)
}

// workspacesGateway serves workspaces and their custom domains.
func workspacesGateway(cfg *files.RootConfig) gatewayAPI {
	return gatewayAPIOf(cfg.Cluster.PublicGateway)
}

func cephValues(ceph files.CephConfig, flags map[string]bool) map[string]any {
	values := map[string]any{
		"credentialsSecretName":     "ceph-admin-credentials",
		"storageClass":              "codesphere",
		"monEndpointsConfigMapName": "ceph-mon-endpoints",
		"mdsNamespace":              "",
		"activeMds":                 "0",
	}
	if flags[workspacePVCStorageFlag] {
		return values
	}

	name, active := "codesphere", 7
	// Codesphere only uses one volume. The installer rejects configs with more.
	for volumeName, volume := range ceph.Volumes {
		name, active = volumeName, volume.MDS.Active
	}

	values["mdsNamespace"] = name
	values["activeMds"] = fmt.Sprint(active)

	return values
}

func networkPoliciesValues(kubernetes files.KubernetesConfig) map[string]any {
	podCIDR, serviceCIDR := kubernetes.PodCIDR, kubernetes.ServiceCIDR
	if kubernetes.ManagedByCodesphere {
		podCIDR = stringOr(podCIDR, defaultKubernetesPodCIDR)
		serviceCIDR = stringOr(serviceCIDR, defaultKubernetesServiceCIDR)
	}

	return map[string]any{
		"workspace": map[string]any{
			"namespace":     workspaceNamespace,
			"podSubnet":     podCIDR,
			"serviceSubnet": serviceCIDR,
		},
	}
}

func oauthValues(oauth *files.OAuthProvidersConfig) map[string]any {
	providers := map[string]any{}
	for _, id := range baseOAuthProviders {
		providers[id] = map[string]any{"enabled": false}
	}

	additional := map[string]any{}

	if oauth != nil && oauth.Oidc != nil {
		oidc := oauth.Oidc

		scopes := oidc.Scopes
		if scopes == nil {
			scopes = []string{"openid", "email", "profile"}
		}

		additional[oidc.Name] = map[string]any{
			"type":      oidc.Type,
			"enabled":   oidc.Enabled,
			"name":      oidc.Name,
			"issuerUrl": oidc.IssuerURL,
			"scopes":    scopes,
		}
	}

	return map[string]any{"providers": providers, "additionalProviders": additional}
}

func signUpValues(signUp *files.SignUpConfig) map[string]any {
	email := map[string]any{}
	if signUp != nil && signUp.Email != nil && signUp.Email.AllowedDomains != nil {
		email["allowedDomains"] = signUp.Email.AllowedDomains
	}

	return map[string]any{"email": email}
}

func postgresValues(postgres files.PostgresConfig) map[string]any {
	ssl := map[string]any{}
	if postgres.CACertPem != "" {
		ssl["ca"] = postgres.CACertPem
	}

	values := map[string]any{
		"database":           stringOr(postgres.Database, codesphereDatabase),
		"rejectUnauthorized": true,
		"ssl":                ssl,
	}

	if postgres.Primary != nil {
		values["host"] = postgres.Primary.IP
		values["port"] = internalPostgresPort
		ssl["serverName"] = postgres.Primary.Hostname

		return values
	}

	values["host"] = postgres.ServerAddress
	if postgres.Port != 0 {
		values["port"] = postgres.Port
	}

	ssl["serverName"] = stringOr(postgres.AltName, postgres.ServerAddress)

	return values
}

func otelValues(export *files.TelemetryExport) map[string]any {
	if export == nil {
		export = &files.TelemetryExport{}
	}

	traces := map[string]any{"enabled": export.Traces}
	if export.TraceEndpoint != "" {
		traces["endpoint"] = export.TraceEndpoint
	}

	return map[string]any{
		"centralOtelExport": map[string]any{
			"enabled":              export.RemoteExport,
			"telemetryCredentials": map[string]any{"secretName": "otel-collector-central-creds"},
			"otlpHttpEndpoint":     stringOr(export.RemoteEndpoint, defaultCentralOtelEndpoint),
		},
		"telemetryExport": map[string]any{
			"spanMetrics": map[string]any{"enabled": export.SpanMetrics},
			"traces":      traces,
		},
	}
}

func boolOr(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}

	return *value
}

func intOr(value *int, fallback int) int {
	if value == nil {
		return fallback
	}

	return *value
}

func stringOr(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}

// omitEmpty drops the empty strings from values, like JSON drops the installer's undefined fields.
func omitEmpty(values map[string]any) map[string]any {
	for key, value := range values {
		if value == "" {
			delete(values, key)
		}
	}

	return values
}
