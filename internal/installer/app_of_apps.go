// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/distribution/reference"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/cli/values"
	"helm.sh/helm/v4/pkg/getter"
)

const (
	// ociCredentialSecretName is the K8s Secret created by "oms beta install argocd"
	// that stores OCI registry credentials for pulling Codesphere charts.
	ociCredentialSecretName = "argocd-codesphere-oci-read"
	// ociCredentialNamespace is the namespace where the credential secret lives.
	// It is also the namespace ArgoCD watches for Application resources.
	ociCredentialNamespace = "argocd"
	// pcAppsName is the name of the pc-applications chart, its BOM component and its Application.
	pcAppsName = "pc-applications"
	// argoAppProject is the ArgoCD project the app-of-apps belongs to.
	argoAppProject = "default"
	// argoDestinationServer is the in-cluster API server address, matching the
	// cluster secret applied by "oms beta install argocd --deploy-dc-config".
	argoDestinationServer = "https://kubernetes.default.svc"
)

// AppOfApps holds the configuration for creating the ArgoCD Application of an app of apps
// chart in a private OCI registry. ArgoCD pulls and reconciles the chart; OMS only owns the
// Application resource.
type AppOfApps struct {
	name           string // name of the Application and of its Helm release
	chart          string
	version        string // chart version, used as the Application targetRevision (required)
	namespace      string // destination namespace of the app-of-apps
	valuesFiles    []string
	valuesOverride map[string]interface{}
	forceConflicts bool
	finalizers     []string
	client         client.Client
}

// DeleteResourcesWithApplication makes deleting the Application delete everything ArgoCD
// deployed from it.
func (p *AppOfApps) DeleteResourcesWithApplication() {
	p.finalizers = []string{argov1alpha1.ResourcesFinalizerName}
}

// NewPCApps creates a new pc-applications installer. It validates that required fields
// are non-empty but does not apply defaults — defaults live on the CLI flag
// declarations only.
func NewPCApps(c client.Client, version, namespace string, valuesFiles []string, valuesOverride map[string]interface{}, forceConflicts bool) (*AppOfApps, error) {
	if version == "" {
		return nil, errors.New("version is required")
	}
	if namespace == "" {
		return nil, errors.New("namespace is required")
	}
	if err := checkArgoCDScheme(c); err != nil {
		return nil, err
	}

	return &AppOfApps{
		name:           pcAppsName,
		chart:          pcAppsName,
		version:        version,
		namespace:      namespace,
		valuesFiles:    valuesFiles,
		valuesOverride: valuesOverride,
		forceConflicts: forceConflicts,
		client:         c,
	}, nil
}

// NewPcAppsFromBom creates a pc-applications installer from the pc-applications entry in a BOM.
func NewPcAppsFromBom(c client.Client, bomConfig *bom.Config, namespace string, valuesFiles []string, valuesOverride map[string]interface{}) (*AppOfApps, error) {
	pcApps, err := NewAppOfAppsFromBom(c, bomConfig, pcAppsName, pcAppsName, namespace, valuesOverride)
	if err != nil {
		return nil, err
	}

	pcApps.valuesFiles = valuesFiles

	return pcApps, nil
}

// NewAppOfAppsFromBom creates an installer for the chart in the BOM component, applied as the
// Application name.
func NewAppOfAppsFromBom(
	c client.Client,
	bomConfig *bom.Config,
	component, name, namespace string,
	values map[string]interface{},
) (*AppOfApps, error) {
	if err := checkArgoCDScheme(c); err != nil {
		return nil, err
	}

	if bomConfig == nil {
		return nil, fmt.Errorf("BOM is required")
	}

	chart, ok := bomConfig.GetChart(component)
	if !ok {
		return nil, fmt.Errorf("%s chart not found in BOM", component)
	}

	return &AppOfApps{
		name:           name,
		chart:          path.Base(reference.Path(chart)),
		version:        chart.Tag(),
		namespace:      namespace,
		valuesOverride: values,
		client:         c,
	}, nil
}

// checkArgoCDScheme fails early if the client cannot encode ArgoCD Applications,
// instead of letting the apply fail with a "no kind is registered" error. Call
// sites build their client scheme with argov1alpha1.AddToScheme.
func checkArgoCDScheme(c client.Client) error {
	if c == nil || c.Scheme() == nil {
		return errors.New("kubernetes client is required")
	}
	if !c.Scheme().Recognizes(argov1alpha1.ApplicationSchemaGroupVersionKind) {
		return fmt.Errorf(
			"kubernetes client scheme does not recognize %s; register it with argov1alpha1.AddToScheme",
			argov1alpha1.ApplicationSchemaGroupVersionKind,
		)
	}
	return nil
}

// ResolveChartsRegistry reads the OCI registry base URL from the K8s Secret created by
// "oms beta install argocd --deploy-dc-config". An Application's repoURL must
// match that secret's url verbatim, otherwise ArgoCD cannot match the
// credentials to the repository.
func ResolveChartsRegistry(ctx context.Context, c client.Client) (string, error) {
	secret := &corev1.Secret{}
	key := client.ObjectKey{Name: ociCredentialSecretName, Namespace: ociCredentialNamespace}
	if err := c.Get(ctx, key, secret); err != nil {
		return "", fmt.Errorf(
			"K8s secret %q not found in namespace %q: %w\n"+
				"Run 'oms beta install argocd --deploy-dc-config' first to create registry credentials",
			ociCredentialSecretName, ociCredentialNamespace, err,
		)
	}

	baseURL := string(secret.Data["url"])
	if baseURL == "" {
		return "", fmt.Errorf(
			"K8s secret %q in namespace %q is missing required field (url)",
			ociCredentialSecretName, ociCredentialNamespace,
		)
	}

	log.Printf("Using OCI registry %q from K8s secret %q\n", baseURL, ociCredentialSecretName)
	return baseURL, nil
}

// createApplication renders the app-of-apps ArgoCD Application for the given repo URL
// and merged Helm values.
func (p *AppOfApps) createApplication(repoURL string, vals map[string]interface{}) (*argov1alpha1.Application, error) {
	helm := &argov1alpha1.ApplicationSourceHelm{
		ReleaseName: p.name,
	}
	if len(vals) > 0 {
		raw, err := json.Marshal(vals)
		if err != nil {
			return nil, fmt.Errorf("marshaling helm values: %w", err)
		}
		helm.ValuesObject = &runtime.RawExtension{Raw: raw}
	}

	return &argov1alpha1.Application{
		TypeMeta: metav1.TypeMeta{
			APIVersion: argov1alpha1.SchemeGroupVersion.String(),
			Kind:       argov1alpha1.ApplicationSchemaGroupVersionKind.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       p.name,
			Namespace:  ociCredentialNamespace,
			Finalizers: p.finalizers,
		},
		Spec: argov1alpha1.ApplicationSpec{
			Project: argoAppProject,
			Source: &argov1alpha1.ApplicationSource{
				RepoURL:        repoURL,
				Chart:          p.chart,
				TargetRevision: p.version,
				Helm:           helm,
			},
			Destination: argov1alpha1.ApplicationDestination{
				Server:    argoDestinationServer,
				Namespace: p.namespace,
			},
			SyncPolicy: &argov1alpha1.SyncPolicy{
				Automated: &argov1alpha1.SyncPolicyAutomated{
					Prune: ptr.To(true),
				},
				SyncOptions: argov1alpha1.SyncOptions{
					"CreateNamespace=true",
					"ServerSideApply=true",
				},
			},
		},
	}, nil
}

// Install creates or updates the app-of-apps ArgoCD Application that references
// the chart. Rolling the chart out is left to ArgoCD.
func (p *AppOfApps) Install(ctx context.Context) error {
	// Validate values files before any cluster calls so local errors fail fast.
	valueOpts := values.Options{ValueFiles: p.valuesFiles}
	fileVals, err := valueOpts.MergeValues(getter.All(cli.New()))
	if err != nil {
		return fmt.Errorf("loading values files: %w", err)
	}
	vals := util.DeepMergeMaps(map[string]any{}, p.valuesOverride)
	vals = util.DeepMergeMaps(vals, fileVals)

	repoURL, err := ResolveChartsRegistry(ctx, p.client)
	if err != nil {
		return fmt.Errorf("failed to resolve repo url: %w", err)
	}

	app, err := p.createApplication(repoURL, vals)
	if err != nil {
		return fmt.Errorf("failed to create argo appliaction: %w", err)
	}

	log.Printf("Applying ArgoCD Application %q (chart %s, version %s) in namespace %s\n",
		p.name, p.chart, p.version, ociCredentialNamespace)
	current := &argov1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, p.client, current, func() error {
		current.TypeMeta = app.TypeMeta
		current.Spec = app.Spec
		for _, finalizer := range app.Finalizers {
			controllerutil.AddFinalizer(current, finalizer)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("applying ArgoCD Application %q failed: %w", p.name, err)
	}

	log.Printf("Successfully applied ArgoCD Application %q; ArgoCD will sync the chart\n", p.name)
	return nil
}

// NewPCAppsForTesting creates a pc-applications installer with injected dependencies for
// use in tests. This avoids exporting struct fields solely for test access.
func NewPCAppsForTesting(c client.Client, version, namespace string, valuesFiles []string, valuesOverride map[string]interface{}, forceConflicts bool) *AppOfApps {
	return &AppOfApps{
		name:           pcAppsName,
		chart:          pcAppsName,
		version:        version,
		namespace:      namespace,
		valuesFiles:    valuesFiles,
		valuesOverride: valuesOverride,
		forceConflicts: forceConflicts,
		client:         c,
	}
}
