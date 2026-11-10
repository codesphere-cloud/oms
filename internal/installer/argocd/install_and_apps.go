// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	synccommon "github.com/argoproj/argo-cd/gitops-engine/pkg/sync/common"
	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/util"
	jsonpatch "github.com/evanphx/json-patch/v5"
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
				// The deadline can run out during the read, which the client reports as its own error.
				if ctx.Err() != nil {
					return false, ctx.Err()
				}

				if !apierrors.IsNotFound(err) {
					return false, fmt.Errorf("failed to read ArgoCD Application %q: %w", name, err)
				}

				log.Printf("Waiting for ArgoCD Application %q to be created", name)

				return false, nil
			}

			deployed, err := currentSourceDeployed(app)
			if err != nil {
				return false, fmt.Errorf("failed to compare the sources of ArgoCD Application %q: %w", name, err)
			}

			lastState = fmt.Sprintf(
				"current source deployed: %t, health=%q, sync=%q",
				deployed,
				app.Status.Health.Status,
				app.Status.Sync.Status,
			)

			if deployed && app.Status.Health.Status == "Healthy" {
				log.Printf("ArgoCD Application %q is freshly synced and healthy", name)
				return true, nil
			}

			log.Printf("Waiting for ArgoCD Application %q (%s)", name, lastState)

			return false, nil
		},
	)
	if wait.Interrupted(err) {
		return fmt.Errorf(
			"timed out waiting for ArgoCD Application %q to become freshly synced and healthy (%s): %w",
			name,
			lastState,
			err,
		)
	}

	if err != nil {
		return fmt.Errorf("failed waiting for ArgoCD Application %q: %w", name, err)
	}

	return nil
}

// WaitForSubAppsFreshlySyncedAndHealthy waits in parallel for every Application the parent deployed, so a failure
// reports all sub applications that did not come up.
func WaitForSubAppsFreshlySyncedAndHealthy(ctx context.Context, kubeClient client.Client, parentAppName string) error {
	names, err := subAppNames(ctx, kubeClient, parentAppName)
	if err != nil {
		return err
	}

	waitErrs := make([]error, len(names))

	var wg sync.WaitGroup
	for i, name := range names {
		wg.Go(func() {
			if err := WaitForAppFreshlySyncedAndHealthy(ctx, kubeClient, name); err != nil {
				waitErrs[i] = fmt.Errorf("failed to wait for ArgoCD Application %q: %w", name, err)
			}
		})
	}

	wg.Wait()

	return errors.Join(waitErrs...)
}

func subAppNames(ctx context.Context, kubeClient client.Client, parentAppName string) ([]string, error) {
	parent := &argov1alpha1.Application{}
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: parentAppName, Namespace: DefaultNamespace}, parent); err != nil {
		return nil, fmt.Errorf("failed to read ArgoCD Application %q: %w", parentAppName, err)
	}

	var names []string

	for _, resource := range parent.Status.Resources {
		if resource.Group == argov1alpha1.SchemeGroupVersion.Group && resource.Kind == argov1alpha1.ApplicationSchemaGroupVersionKind.Kind {
			names = append(names, resource.Name)
		}
	}

	return names, nil
}

// currentSourceDeployed reports whether Argo CD has deployed the Application's current source.
// Spec.Source is what OMS writes: the chart, its version and all values, for an app of apps also its
// children's versions and values.
func currentSourceDeployed(app *argov1alpha1.Application) (bool, error) {
	// Argo CD writes Status.Sync.ComparedTo.Source on every comparison with the
	// cluster (and sets it to Spec.Source). Once they match we know the sync status describes the current source.
	freshlyCompared, err := sameSource(app.Spec.Source, &app.Status.Sync.ComparedTo.Source)
	if err != nil {
		return false, err
	}

	if !freshlyCompared {
		return false, nil
	}

	// Happy case: Argo CD compared and synced (or a sync was not necessary).
	if app.Status.Sync.Status == argov1alpha1.SyncStatusCodeSynced {
		return true, nil
	}

	// Drift case: Argo CD compared and synced, but the resources in the cluster changed afterwards
	// (so app.Status.Sync.Status is not "Synced"). Example: an autoscaler changes replicas. The
	// deploy still worked, so this is fine. The sync must be of the current source: until the new
	// sync starts, OperationState still holds the previous deploy's succeeded sync.
	operation := app.Status.OperationState
	if operation == nil || operation.SyncResult == nil {
		return false, nil
	}

	freshlySynced, err := sameSource(app.Spec.Source, &operation.SyncResult.Source)
	if err != nil {
		return false, err
	}

	if !freshlySynced || operation.Phase != synccommon.OperationSucceeded {
		return false, nil
	}

	// Sync and Health status come from Argo CD's last comparison, done at ReconciledAt, not from the
	// sync. Until a comparison runs after the sync, they still describe the old resources, e.g. Healthy
	// for the pods being replaced.
	reconciledAt, finishedAt := app.Status.ReconciledAt, operation.FinishedAt

	return reconciledAt != nil && finishedAt != nil && reconciledAt.After(finishedAt.Time), nil
}

// sameSource compares two sources the way Argo CD may record them in the status. Argo CD writes the
// status as a JSON merge patch, in which null deletes a key, so spec values {"metrics": null} come
// back as {} in Status.Sync.ComparedTo. OperationState.SyncResult keeps them. Applying the same
// merge patch to both sides drops those keys before comparing, whichever copy it is. That
// misses a change in which a key only switches between null and missing. Callers that need it
// detected must change another value with it, as cs-applications does with its sync trigger on
// every deploy.
//
// Possible alternative: passing the values as a YAML string (helm.values) would keep the nulls,
// but makes them hard to read and diff in the Argo CD UI.
func sameSource(a, b *argov1alpha1.ApplicationSource) (bool, error) {
	a, err := asStoredInStatus(a)
	if err != nil {
		return false, err
	}

	b, err = asStoredInStatus(b)
	if err != nil {
		return false, err
	}

	return a.Equals(b), nil
}

func asStoredInStatus(source *argov1alpha1.ApplicationSource) (*argov1alpha1.ApplicationSource, error) {
	if source == nil || source.Helm == nil || source.Helm.ValuesObject == nil {
		return source, nil
	}

	raw, err := jsonpatch.MergePatch([]byte("{}"), source.Helm.ValuesObject.Raw)
	if err != nil {
		return nil, fmt.Errorf("failed to apply the Helm values as a merge patch: %w", err)
	}

	copied := source.DeepCopy()
	copied.Helm.ValuesObject = &runtime.RawExtension{Raw: raw}

	return copied, nil
}

// InstallerAPI is implemented by the concrete ArgoCD chart installer.
//
//mockery:generate: true
type InstallerAPI interface {
	Install(ctx context.Context) error
}

// AppInstallerConfig configures the shared ArgoCD, vault secret, and
// pc-applications app-of-apps workflow.
type AppInstallerConfig struct {
	Config       files.RootConfig
	Vault        *files.InstallVault
	RESTConfig   *rest.Config
	KubeClient   client.Client
	HelmClient   installer.HelmClient
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
func (i *AppInstaller) InstallArgoCD(ctx context.Context) error {
	if i.cfg.Installer == nil {
		return fmt.Errorf("ArgoCD installer is required")
	}

	if err := i.cfg.Installer.Install(ctx); err != nil {
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
	values, err := i.getPcAppsValues()
	if err != nil {
		return err
	}

	pcApps, err := installer.NewAppOfAppsFromBom(i.cfg.KubeClient, bomConfig, installer.AppOfAppsConfig{
		Component:      pcAppsName,
		Name:           pcAppsName,
		ReleaseName:    pcAppsName,
		Namespace:      DefaultNamespace,
		ValuesFiles:    i.cfg.PCAppsValues,
		ValuesOverride: values,
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
func (i *AppInstaller) getPcAppsValues() (files.ChartValues, error) {
	clusterConfigValues, err := pcapps.ClusterConfigPcAppsValues(&i.cfg.Config, i.cfg.Vault)
	if err != nil {
		return nil, fmt.Errorf("configuring cluster-config application: %w", err)
	}
	openFGAppValues := pcapps.OpenFgaPcAppsValues(&i.cfg.Config, i.cfg.Vault)
	rookCephAppValues := pcapps.RookCephPcAppsValues(&i.cfg.Config)

	values := util.DeepMergeMaps(openFGAppValues, rookCephAppValues)
	values = util.DeepMergeMaps(values, clusterConfigValues)
	values = util.DeepMergeMaps(values, i.cfg.Config.PcApps)

	return values, nil
}
