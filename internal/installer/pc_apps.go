// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/util"
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
	// pcAppsAppName is the name of the app-of-apps ArgoCD Application.
	pcAppsAppName = "pc-applications"
	// pcAppsChartName is the chart name referenced by the ArgoCD Application source.
	pcAppsChartName = "pc-applications"
	// argoAppProject is the ArgoCD project the app-of-apps belongs to.
	argoAppProject = "default"
	// argoDestinationServer is the in-cluster API server address, matching the
	// cluster secret applied by "oms beta install argocd --deploy-dc-config".
	argoDestinationServer = "https://kubernetes.default.svc"
)

// PCApps holds the configuration for creating the ArgoCD app-of-apps
// Application that references the pc-applications chart in a private OCI
// registry. ArgoCD pulls and reconciles the chart; OMS only owns the
// Application resource.
type PCApps struct {
	version        string // chart version, used as the Application targetRevision (required)
	namespace      string // destination namespace of the app-of-apps
	valuesFiles    []string
	valuesOverride map[string]interface{}
	forceConflicts bool
	client         client.Client
}

// NewPCApps creates a new PCApps installer. It validates that required fields
// are non-empty but does not apply defaults — defaults live on the CLI flag
// declarations only.
func NewPCApps(c client.Client, version, namespace string, valuesFiles []string, valuesOverride map[string]interface{}, forceConflicts bool) (*PCApps, error) {
	if version == "" {
		return nil, errors.New("version is required")
	}
	if namespace == "" {
		return nil, errors.New("namespace is required")
	}
	if err := checkArgoCDScheme(c); err != nil {
		return nil, err
	}

	return &PCApps{
		version:        version,
		namespace:      namespace,
		valuesFiles:    valuesFiles,
		valuesOverride: valuesOverride,
		forceConflicts: forceConflicts,
		client:         c,
	}, nil
}

func NewPcAppsFromBom(c client.Client, bomConfig *bom.Config, namespace string, valuesFiles []string, valuesOverride map[string]interface{}) (*PCApps, error) {
	if err := checkArgoCDScheme(c); err != nil {
		return nil, err
	}
	if bomConfig == nil {
		return nil, fmt.Errorf("BOM is required")
	}

	pcApps, ok := bomConfig.GetPCApps()
	if !ok {
		return nil, fmt.Errorf("pc-applications component not found in BOM")
	}

	return &PCApps{
		version:        pcApps.Tag(),
		namespace:      namespace,
		valuesFiles:    valuesFiles,
		valuesOverride: valuesOverride,
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

// resolveRepoURL reads the OCI registry base URL from the K8s Secret created by
// "oms beta install argocd --deploy-dc-config". The Application's repoURL must
// match that secret's url verbatim, otherwise ArgoCD cannot match the
// credentials to the repository.
func (p *PCApps) resolveRepoURL(ctx context.Context) (string, error) {
	secret := &corev1.Secret{}
	key := client.ObjectKey{Name: ociCredentialSecretName, Namespace: ociCredentialNamespace}
	if err := p.client.Get(ctx, key, secret); err != nil {
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
func (p *PCApps) createApplication(repoURL string, vals map[string]interface{}) (*argov1alpha1.Application, error) {
	helm := &argov1alpha1.ApplicationSourceHelm{
		ReleaseName: pcAppsAppName,
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
			Name:      pcAppsAppName,
			Namespace: ociCredentialNamespace,
		},
		Spec: argov1alpha1.ApplicationSpec{
			Project: argoAppProject,
			Source: &argov1alpha1.ApplicationSource{
				RepoURL:        repoURL,
				Chart:          pcAppsChartName,
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
// the pc-applications chart. Rolling the chart out is left to ArgoCD.
func (p *PCApps) Install(ctx context.Context) error {
	// Validate values files before any cluster calls so local errors fail fast.
	valueOpts := values.Options{ValueFiles: p.valuesFiles}
	fileVals, err := valueOpts.MergeValues(getter.All(cli.New()))
	if err != nil {
		return fmt.Errorf("loading values files: %w", err)
	}
	vals := util.DeepMergeMaps(map[string]any{}, p.valuesOverride)
	vals = util.DeepMergeMaps(vals, fileVals)

	repoURL, err := p.resolveRepoURL(ctx)
	if err != nil {
		return fmt.Errorf("failed to resolve repo url: %w", err)
	}

	app, err := p.createApplication(repoURL, vals)
	if err != nil {
		return fmt.Errorf("failed to create argo appliaction: %w", err)
	}

	log.Printf("Applying ArgoCD Application %q (chart %s, version %s) in namespace %s\n",
		pcAppsAppName, pcAppsChartName, p.version, ociCredentialNamespace)
	current := &argov1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, p.client, current, func() error {
		current.TypeMeta = app.TypeMeta
		current.Spec = app.Spec
		return nil
	}); err != nil {
		return fmt.Errorf("applying ArgoCD Application %q failed: %w", pcAppsAppName, err)
	}

	log.Printf("Successfully applied ArgoCD Application %q; ArgoCD will sync the chart\n", pcAppsAppName)
	return nil
}

// NewPCAppsForTesting creates a PCApps instance with injected dependencies for
// use in tests. This avoids exporting struct fields solely for test access.
func NewPCAppsForTesting(c client.Client, version, namespace string, valuesFiles []string, valuesOverride map[string]interface{}, forceConflicts bool) *PCApps {
	return &PCApps{
		version:        version,
		namespace:      namespace,
		valuesFiles:    valuesFiles,
		valuesOverride: valuesOverride,
		forceConflicts: forceConflicts,
		client:         c,
	}
}
