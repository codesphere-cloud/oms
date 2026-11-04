// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	synccommon "github.com/argoproj/argo-cd/gitops-engine/pkg/sync/common"
	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	applicationReadyTimeout      = 30 * time.Minute
	applicationReadyPollInterval = 5 * time.Second
	// pcAppsName is the name of the pc-applications chart, its BOM component, its Application and
	// its Helm release.
	pcAppsName = "pc-applications"
)

// WaitForAppFreshlySyncedAndHealthy waits until Argo CD has synced the Application's current
// source, values included, and reports it as healthy. A sync of an earlier source does not count.
// Resources that drift after a successful sync do not keep it waiting.
func WaitForAppFreshlySyncedAndHealthy(ctx context.Context, kubeClient client.Client, name string) error {
	log.Printf(
		"Waiting for ArgoCD Application %q to become freshly synced and healthy (timeout %s)",
		name,
		applicationReadyTimeout,
	)

	lastState := "not created"

	err := wait.PollUntilContextTimeout(
		ctx,
		applicationReadyPollInterval,
		applicationReadyTimeout,
		/* immediate */ true,
		func(ctx context.Context) (bool, error) {
			app := &argov1alpha1.Application{}
			if err := kubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: DefaultNamespace}, app); err != nil {
				if !apierrors.IsNotFound(err) {
					return false, fmt.Errorf("failed to read ArgoCD Application %q: %w", name, err)
				}

				log.Printf("Waiting for ArgoCD Application %q to be created", name)

				return false, nil
			}

			synced, err := freshlySynced(app)
			if err != nil {
				return false, fmt.Errorf("failed to compare the sources of ArgoCD Application %q: %w", name, err)
			}

			lastState = fmt.Sprintf(
				"synced current source: %t, health=%q, sync=%q",
				synced,
				app.Status.Health.Status,
				app.Status.Sync.Status,
			)

			if synced && app.Status.Health.Status == "Healthy" {
				log.Printf("ArgoCD Application %q is freshly synced and healthy", name)
				return true, nil
			}

			log.Printf("Waiting for ArgoCD Application %q (%s)", name, lastState)

			return false, nil
		},
	)
	if err != nil {
		return fmt.Errorf(
			"timed out waiting for ArgoCD Application %q to become freshly synced and healthy (%s): %w",
			name,
			lastState,
			err,
		)
	}

	return nil
}

// freshlySynced reports whether Argo CD has synced the Application's current source.
func freshlySynced(app *argov1alpha1.Application) (bool, error) {
	inSync, err := comparedSourceInSync(app)
	if err != nil || inSync {
		return inSync, err
	}

	return lastSyncSucceededFor(app)
}

// comparedSourceInSync reports whether the last comparison used the current source and found the
// cluster in sync. Spec.Source is what OMS writes: the chart, its version and all values, for an
// app of apps also its children's versions and values. Argo CD copies it to
// Status.Sync.ComparedTo.Source on every comparison with the cluster, so once the two match, the
// sync status describes the current source and not an earlier one. This also covers a source that
// renders the same manifests, for which Argo CD runs no sync operation.
func comparedSourceInSync(app *argov1alpha1.Application) (bool, error) {
	if app.Status.Sync.Status != argov1alpha1.SyncStatusCodeSynced {
		return false, nil
	}

	return sameSource(app.Spec.Source, &app.Status.Sync.ComparedTo.Source)
}

// lastSyncSucceededFor reports whether the last sync operation applied the current source and
// succeeded. A resource that changes right after the sync, e.g. replicas an autoscaler sets, turns
// the Application OutOfSync again, and without selfHeal Argo CD leaves it so. The deploy still
// worked, so this counts. A running or failed sync does not, although Argo CD may already record
// its source.
func lastSyncSucceededFor(app *argov1alpha1.Application) (bool, error) {
	operation := app.Status.OperationState
	if operation == nil || operation.Phase != synccommon.OperationSucceeded || operation.SyncResult == nil {
		return false, nil
	}

	return sameSource(app.Spec.Source, &operation.SyncResult.Source)
}

// sameSource compares two sources without their null values. Argo CD leaves nulls out of the
// sources it records in the status: spec values {"tlsSecret": null, "domain": "a"} come back as
// {"domain": "a"}, which would never match. sameSource only tells whether a recorded source is the
// current spec, so dropping nulls misses just one change: a key that only switches between null
// and missing, i.e. between removing and keeping a chart default. Callers that need it detected
// must change another value with it, as cs-applications does with its sync trigger on every deploy.
func sameSource(a, b *argov1alpha1.ApplicationSource) (bool, error) {
	a, err := withoutNullValues(a)
	if err != nil {
		return false, err
	}

	b, err = withoutNullValues(b)
	if err != nil {
		return false, err
	}

	return a.Equals(b), nil
}

func withoutNullValues(source *argov1alpha1.ApplicationSource) (*argov1alpha1.ApplicationSource, error) {
	if source == nil || source.Helm == nil || source.Helm.ValuesObject == nil {
		return source, nil
	}

	values := map[string]any{}
	if err := json.Unmarshal(source.Helm.ValuesObject.Raw, &values); err != nil {
		return nil, fmt.Errorf("failed to parse the Helm values: %w", err)
	}

	raw, err := json.Marshal(dropNulls(values))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal the Helm values: %w", err)
	}

	copied := source.DeepCopy()
	copied.Helm.ValuesObject = &runtime.RawExtension{Raw: raw}

	return copied, nil
}

func dropNulls(values map[string]any) map[string]any {
	for key, value := range values {
		if value == nil {
			delete(values, key)
			continue
		}

		dropNestedNulls(value)
	}

	return values
}

func dropNestedNulls(value any) {
	switch v := value.(type) {
	case map[string]any:
		dropNulls(v)
	case []any:
		for _, item := range v {
			dropNestedNulls(item)
		}
	}
}

// InstallerAPI is implemented by the concrete ArgoCD chart installer.
type InstallerAPI interface {
	Install() error
}

// AppInstallerConfig configures the shared ArgoCD, vault secret, and
// pc-applications app-of-apps workflow.
type AppInstallerConfig struct {
	Config       files.RootConfig
	Vault        *files.InstallVault
	RESTConfig   *rest.Config
	KubeClient   client.Client
	Installer    InstallerAPI
	PCAppsValues []string
}

// AppInstaller installs ArgoCD, syncs the vault secret, and registers the
// pc-applications app-of-apps from the installer BOM.
type AppInstaller struct {
	cfg AppInstallerConfig
}

// NewAppInstaller creates an ArgoCD-and-applications installer.
func NewAppInstaller(cfg AppInstallerConfig) *AppInstaller {
	return &AppInstaller{cfg: cfg}
}

// InstallArgoCD installs or upgrades ArgoCD using the configured installer.
func (i *AppInstaller) InstallArgoCD() error {
	if i.cfg.Installer == nil {
		return fmt.Errorf("ArgoCD installer is required")
	}

	if err := i.cfg.Installer.Install(); err != nil {
		return fmt.Errorf("failed to install ArgoCD: %w", err)
	}

	return nil
}

// SyncVaultSecret refreshes the service account tokens and creates or updates
// the Codesphere vault secret in Kubernetes.
func (i *AppInstaller) SyncVaultSecret(ctx context.Context) error {
	creator := vault.NewVaultSecretCreator(i.cfg.KubeClient)
	if err := creator.CreateSecretFromVault(ctx, i.cfg.Vault, vault.VaultSecretNamespace, vault.VaultSecretName); err != nil {
		return fmt.Errorf("failed to sync vault secret: %w", err)
	}

	return nil
}

// InstallPCApps creates or updates the pc-applications app-of-apps ArgoCD
// Application using the chart version from the supplied installer BOM.
func (i *AppInstaller) InstallPCApps(ctx context.Context, bomConfig *bom.Config) error {
	pcApps, err := installer.NewAppOfAppsFromBom(i.cfg.KubeClient, bomConfig, installer.AppOfAppsConfig{
		Component:      pcAppsName,
		Name:           pcAppsName,
		ReleaseName:    pcAppsName,
		Namespace:      DefaultNamespace,
		ValuesFiles:    i.cfg.PCAppsValues,
		ValuesOverride: i.getPcAppsValues(),
	})
	if err != nil {
		return fmt.Errorf("failed to initialize pc-apps installer: %w", err)
	}

	if err := pcApps.Install(ctx); err != nil {
		return fmt.Errorf("failed to install pc-apps: %w", err)
	}

	return nil
}

// getPcAppsValues collects the base values for some apps from the config (OpenFGA, Rook) and merges these with
// explicit pcApps block in the config.yaml.
// The pcApps block in the config.yaml and --pc-apps-values always win over derived values from the config.yaml.
// Returns a ChartValues map
func (i *AppInstaller) getPcAppsValues() files.ChartValues {
	openFGAppValues := pcapps.OpenFgaPcAppsValues(&i.cfg.Config, i.cfg.Vault)
	rookCephAppValues := pcapps.RookCephPcAppsValues(&i.cfg.Config)

	values := util.DeepMergeMaps(openFGAppValues, rookCephAppValues)
	values = util.DeepMergeMaps(values, i.cfg.Config.PcApps)

	return values
}
