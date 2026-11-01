// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"go.yaml.in/yaml/v3"
	appsv1 "k8s.io/api/apps/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const databaseSetupNamespace = "codesphere"

func (ci *CodesphereInstaller) runDatabaseSetupJob(pm PackageManager, cfg files.RootConfig) error {
	if ci.VaultSecretName == "" {
		ci.VaultSecretName = vault.VaultSecretName
	}

	if ci.VaultType == "" {
		ci.VaultType = string(vault.TypeSOPS)
	}

	packageBOM, err := bom.Parse(pm.GetDependencyPath("bom.json"))
	if err != nil {
		return fmt.Errorf("read database setup artifacts from BOM: %w", err)
	}

	chart, ok := packageBOM.GetChart("database-setup")
	if !ok {
		return fmt.Errorf("database-setup chart with a tagged OCI reference is required in BOM")
	}

	ctx := ci.Context
	if ctx == nil {
		ctx = context.Background()
	}

	_, restConfig, err := VaultAndRESTConfig(ci.VaultPath, ci.PrivKey, ci.VaultType, cfg)
	if err != nil {
		return fmt.Errorf("load Kubernetes credentials: %w", err)
	}

	releaseName := cfg.Codesphere.ReleaseName
	if releaseName == "" {
		releaseName = "codesphere"
	}

	helm, err := NewHelmClientWithRESTConfig(databaseSetupNamespace, restConfig)
	if err != nil {
		return err
	}

	installed, err := helm.FindRelease(databaseSetupNamespace, releaseName)
	if err != nil {
		return fmt.Errorf("find installed Codesphere chart: %w", err)
	}

	installedVersion := ""
	maintenancePrepared := false

	if installed != nil {
		installedVersion = installed.InstalledVersion
		if installedVersion == "" {
			return fmt.Errorf("installed Codesphere chart has no version")
		}

		if cfg.Codesphere.MaintenanceMode == nil || *cfg.Codesphere.MaintenanceMode {
			client, err := NewKubeClient(restConfig)
			if err != nil {
				return err
			}

			if err := prepareCodesphereMaintenance(ctx, client); err != nil {
				return fmt.Errorf("prepare Codesphere maintenance: %w", err)
			}
		}

		maintenancePrepared = true
	}

	values, err := databaseSetupValues(cfg, ci.VaultSecretName, installedVersion, maintenancePrepared)
	if err != nil {
		return err
	}

	chartRepository := path.Dir(chart.Name())
	if cfg.Registry != nil && cfg.Registry.Server != "" {
		chartRepository, err = ChartRegistryURL(cfg.Registry.Server)
		if err != nil {
			return fmt.Errorf("resolve database setup chart registry: %w", err)
		}
	}

	chartConfig := ChartConfig{
		ReleaseName: "database-setup",
		ChartName:   "oci://" + path.Join(chartRepository, path.Base(chart.Name())),
		Namespace:   databaseSetupNamespace,
		Version:     chart.Tag(),
		Values:      values,
		Timeout:     30 * time.Minute,
	}
	if err := helm.UpgradeChart(ctx, chartConfig, UpgradeChartOptions{InstallIfNotExist: true}); err != nil {
		return fmt.Errorf("run database setup chart %q: %w", path.Base(chart.Name()), err)
	}

	return nil
}

func databaseSetupValues(cfg files.RootConfig, vaultSecretName, installedVersion string, maintenancePrepared bool) (map[string]interface{}, error) {
	plansYaml, err := yaml.Marshal(cfg.Codesphere.Plans)
	if err != nil {
		return nil, fmt.Errorf("encode database setup plans: %w", err)
	}

	var plans map[string]interface{}
	if err := yaml.Unmarshal(plansYaml, &plans); err != nil {
		return nil, fmt.Errorf("decode database setup plans: %w", err)
	}

	pg := cfg.Postgres
	host := pg.ServerAddress

	serverName := pg.AltName
	if pg.Primary != nil {
		host = pg.Primary.IP
		serverName = pg.Primary.Hostname
	}

	if host == "" {
		return nil, fmt.Errorf("PostgreSQL address is required for database setup Job")
	}

	port := pg.Port
	if port == 0 {
		port = files.PostgresPort
	}

	database := pg.Database
	if database == "" {
		database = "codesphere"
	}

	releaseName := cfg.Codesphere.ReleaseName
	if releaseName == "" {
		releaseName = "codesphere"
	}

	if pg.UseSSL == nil || *pg.UseSSL {
		if serverName == "" {
			serverName = host
		}
	} else {
		serverName = ""
	}

	return map[string]interface{}{
		"codesphere": map[string]interface{}{
			"releaseName": releaseName, "installedVersion": installedVersion, "maintenanceMode": cfg.Codesphere.MaintenanceMode,
			"maintenancePrepared": maintenancePrepared, "plans": plans,
		},
		"postgres": map[string]interface{}{
			"host": host, "port": port, "database": database, "rolesPrefix": pg.RolesPrefix,
			"tls": map[string]interface{}{"serverName": serverName, "caPem": pg.CACertPem},
		},
		"vault": map[string]interface{}{"sourceSecretName": vaultSecretName},
	}, nil
}

func prepareCodesphereMaintenance(ctx context.Context, client ctrlclient.Client) error {
	deployments := &appsv1.DeploymentList{}
	if err := client.List(ctx, deployments, ctrlclient.InNamespace(databaseSetupNamespace), ctrlclient.MatchingLabels{
		"app.kubernetes.io/part-of": "codesphere",
	}); err != nil {
		return fmt.Errorf("list Codesphere deployments: %w", err)
	}

	var ide *appsv1.Deployment

	for i := range deployments.Items {
		if deployments.Items[i].Labels["app"] == "ide-service" {
			ide = &deployments.Items[i]
			break
		}
	}

	if ide == nil {
		return fmt.Errorf("ide-service deployment not found for maintenance mode")
	}

	if len(ide.Spec.Template.Spec.Containers) == 0 {
		return fmt.Errorf("ide-service deployment has no container")
	}

	flagFound := false

	for i := range ide.Spec.Template.Spec.Containers[0].Env {
		env := &ide.Spec.Template.Spec.Containers[0].Env[i]
		if env.Name != "CODESPHERE_EXPERIMENTS" {
			continue
		}

		flags := strings.Split(env.Value, ",")
		for _, flag := range flags {
			if flag == "maintenance-mode" {
				flagFound = true
				break
			}
		}

		if !flagFound {
			env.Value = strings.Join(append(flags, "maintenance-mode"), ",")
		}

		if err := client.Update(ctx, ide); err != nil {
			return fmt.Errorf("update ide-service maintenance mode: %w", err)
		}

		flagFound = true

		break
	}

	if !flagFound {
		return fmt.Errorf("ide-service has no CODESPHERE_EXPERIMENTS environment variable")
	}

	for i := range deployments.Items {
		deployment := &deployments.Items[i]
		if deployment.Labels["app"] == "ide-service" || deployment.Labels["app"] == "ide-frontend" {
			continue
		}

		zero := int32(0)

		deployment.Spec.Replicas = &zero
		if err := client.Update(ctx, deployment); err != nil {
			return fmt.Errorf("scale deployment %q for maintenance: %w", deployment.Name, err)
		}
	}

	return nil
}
