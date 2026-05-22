// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	_ "embed"
	"fmt"
	"log"

	k8s "github.com/codesphere-cloud/oms/internal/util"
	"k8s.io/client-go/kubernetes"
)

// ArgoCDRepoSecretConfig holds the parameters for creating an ArgoCD repository secret.
type ArgoCDRepoSecretConfig struct {
	Name       string // metadata.name of the Secret
	URL        string // repository URL (e.g. ghcr.io/codesphere-cloud/charts)
	RepoName   string // display name for ArgoCD (stringData.name)
	Type       string // repo type: "helm" or "git"
	Username   string // auth username
	Password   string // auth password/token
	EnableOCI  bool   // whether to set enableOCI: "true"
	SecretType string // argocd.argoproj.io/secret-type label value (default: "repository")
}

// ArgoCDRepoSecret handles creating/updating ArgoCD repository secrets.
type ArgoCDRepoSecret struct {
	Config    ArgoCDRepoSecretConfig
	Clientset kubernetes.Interface
}

//go:embed manifests/argocd/repo-secret.yaml.tpl
var repoSecretTpl []byte

// NewArgoCDRepoSecret creates a new ArgoCDRepoSecret installer.
func NewArgoCDRepoSecret(cfg ArgoCDRepoSecretConfig) (*ArgoCDRepoSecret, error) {
	clientset, _, err := k8s.NewClients()
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes clients: %w", err)
	}

	return &ArgoCDRepoSecret{
		Config:    cfg,
		Clientset: clientset,
	}, nil
}

// Apply renders the template and creates or updates the ArgoCD repository secret.
func (r *ArgoCDRepoSecret) Apply(ctx context.Context) error {
	log.Printf("Applying ArgoCD repository secret %q...\n", r.Config.Name)

	enableOCI := "false"
	if r.Config.EnableOCI {
		enableOCI = "true"
	}

	rendered, err := k8s.RenderTemplate(repoSecretTpl, map[string]string{
		"SECRET_NAME":       r.Config.Name,
		"SECRET_TYPE":       r.Config.SecretType,
		"REPO_TYPE":         r.Config.Type,
		"REPO_URL":          r.Config.URL,
		"REPO_DISPLAY_NAME": r.Config.RepoName,
		"USERNAME":          r.Config.Username,
		"PASSWORD":          r.Config.Password,
		"ENABLE_OCI":        enableOCI,
	})
	if err != nil {
		return fmt.Errorf("rendering repo secret template: %w", err)
	}

	if err := k8s.ApplySecretFromYAML(ctx, r.Clientset, rendered); err != nil {
		return fmt.Errorf("applying repo secret %q: %w", r.Config.Name, err)
	}

	fmt.Printf("Successfully applied ArgoCD repository secret %q\n", r.Config.Name)
	return nil
}
