// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	synccommon "github.com/argoproj/argo-cd/gitops-engine/pkg/sync/common"
	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
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
)

// LogFunc records formatted application readiness messages.
type LogFunc func(format string, args ...interface{})

// WaitForApplicationHealthy waits until Argo CD has compared the requested target revision
// and reports the Application as healthy and synced.
func WaitForApplicationHealthy(ctx context.Context, kubeClient client.Client, name, targetRevision string, logf LogFunc) error {
	if logf == nil {
		logf = func(string, ...interface{}) {}
	}

	logf("Waiting for ArgoCD Application %q to become healthy and synced (timeout %s)", name, applicationReadyTimeout)

	lastHealth, lastSync, lastTargetRevision := "", "", ""

	err := wait.PollUntilContextTimeout(ctx, applicationReadyPollInterval, applicationReadyTimeout, true, func(ctx context.Context) (bool, error) {
		app := &argov1alpha1.Application{}
		if err := kubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: DefaultNamespace}, app); err != nil {
			if !apierrors.IsNotFound(err) {
				return false, fmt.Errorf("failed to read ArgoCD Application %q: %w", name, err)
			}

			logf("Waiting for ArgoCD Application %q: failed to read status: %v", name, err)

			return false, nil
		}

		lastHealth, lastSync = string(app.Status.Health.Status), string(app.Status.Sync.Status)

		lastTargetRevision = app.Status.Sync.ComparedTo.Source.TargetRevision
		if lastTargetRevision == targetRevision && app.Status.Health.Status == "Healthy" && app.Status.Sync.Status == argov1alpha1.SyncStatusCodeSynced {
			logf("ArgoCD Application %q is healthy and synced", name)
			return true, nil
		}

		logf("Waiting for ArgoCD Application %q (target-revision=%q/%q, health=%q, sync=%q)", name, lastTargetRevision, targetRevision, lastHealth, lastSync)

		return false, nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for ArgoCD Application %q to become healthy and synced at target revision %q (observed target revision=%q, health=%q, sync=%q): %w", name, targetRevision, lastTargetRevision, lastHealth, lastSync, err)
	}

	return nil
}

// WaitForApplicationSynced waits until Argo CD has synced the Application's current source, values
// included, and reports it as healthy. A sync of an earlier source does not count. Resources that
// drift after a successful sync do not keep it waiting.
func WaitForApplicationSynced(ctx context.Context, kubeClient client.Client, name string, logf LogFunc) error {
	if logf == nil {
		logf = func(string, ...interface{}) {}
	}

	logf("Waiting for ArgoCD Application %q to become healthy and synced (timeout %s)", name, applicationReadyTimeout)

	lastHealth, lastSync := "", ""

	err := wait.PollUntilContextTimeout(ctx, applicationReadyPollInterval, applicationReadyTimeout, true, func(ctx context.Context) (bool, error) {
		app := &argov1alpha1.Application{}
		if err := kubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: DefaultNamespace}, app); err != nil {
			if !apierrors.IsNotFound(err) {
				return false, fmt.Errorf("failed to read ArgoCD Application %q: %w", name, err)
			}

			logf("Waiting for ArgoCD Application %q to be created", name)

			return false, nil
		}

		lastHealth, lastSync = string(app.Status.Health.Status), string(app.Status.Sync.Status)

		synced, err := syncedCurrentSource(app)
		if err != nil {
			return false, fmt.Errorf("failed to compare the sources of ArgoCD Application %q: %w", name, err)
		}

		if synced && app.Status.Health.Status == "Healthy" {
			logf("ArgoCD Application %q is healthy and synced", name)
			return true, nil
		}

		logf("Waiting for ArgoCD Application %q (synced current source=%t, health=%q, sync=%q)", name, synced, lastHealth, lastSync)

		return false, nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for ArgoCD Application %q to become healthy and synced (health=%q, sync=%q): %w", name, lastHealth, lastSync, err)
	}

	return nil
}

// syncedCurrentSource reports whether the Application is in sync with its current source, or the
// last sync of that source succeeded.
func syncedCurrentSource(app *argov1alpha1.Application) (bool, error) {
	if app.Status.Sync.Status == argov1alpha1.SyncStatusCodeSynced {
		compared, err := sameSource(app.Spec.Source, &app.Status.Sync.ComparedTo.Source)
		if err != nil || compared {
			return compared, err
		}
	}

	operation := app.Status.OperationState
	if operation == nil || operation.Phase != synccommon.OperationSucceeded || operation.SyncResult == nil {
		return false, nil
	}

	return sameSource(app.Spec.Source, &operation.SyncResult.Source)
}

// sameSource compares two sources ignoring null values, which Argo CD drops from the sources it
// records in the status.
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
	if err := secrets.EnsureServiceAccountTokens(i.cfg.Vault); err != nil {
		return fmt.Errorf("failed to ensure service account tokens: %w", err)
	}

	creator := vault.NewVaultSecretCreator(i.cfg.KubeClient)
	if err := creator.CreateSecretFromVault(ctx, i.cfg.Vault, vault.VaultSecretNamespace, vault.VaultSecretName); err != nil {
		return fmt.Errorf("failed to sync vault secret: %w", err)
	}

	return nil
}

// InstallPCApps creates or updates the pc-applications app-of-apps ArgoCD
// Application using the chart version from the supplied installer BOM.
func (i *AppInstaller) InstallPCApps(ctx context.Context, bomConfig *bom.Config) error {
	pcApps, err := installer.NewPcAppsFromBom(
		i.cfg.KubeClient,
		bomConfig,
		DefaultNamespace,
		i.cfg.PCAppsValues,
		i.getPcAppsValues(),
	)
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
