// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"

	k8s "github.com/codesphere-cloud/oms/internal/util"
	"k8s.io/client-go/kubernetes"
)

//mockery:generate: true
type ArgoCDResources interface {
	ApplyAll(ctx context.Context) error
}

// defaultOciUsername is the user of the Codesphere registry on GHCR, which is where the Helm
// charts are pulled from unless an installation mirrors them into a registry of its own.
const defaultOciUsername = "github"

type argoCDResources struct {
	clientset kubernetes.Interface

	DatacenterID   string
	OciUsername    string
	OciPassword    string
	OciRegistryURL string
	GitPassword    string
}

// NewArgoCDResources collects the Codesphere-managed resources an ArgoCD installation needs, above
// all the repository credentials it pulls the Helm charts with. An empty ociUsername falls back to
// the GHCR user.
func NewArgoCDResources(clientset kubernetes.Interface, dataCenterID string, ociUsername string, ociPassword string, ociRegistryURL string, gitPassword string) (ArgoCDResources, error) {
	if ociUsername == "" {
		ociUsername = defaultOciUsername
	}

	return &argoCDResources{
		clientset:      clientset,
		DatacenterID:   dataCenterID,
		OciUsername:    ociUsername,
		OciPassword:    ociPassword,
		OciRegistryURL: ociRegistryURL,
		GitPassword:    gitPassword,
	}, nil
}

//go:embed manifests/cluster-local.yaml.tpl
var localClusterTpl []byte

//go:embed manifests/repo-helm-oci.yaml.tpl
var helmRegistryTpl []byte

//go:embed manifests/repo-creds-git.yaml.tpl
var gitRepoTpl []byte

func (a *argoCDResources) ApplyAll(ctx context.Context) error {
	if a.DatacenterID != "" {
		if err := a.applyLocalCluster(ctx); err != nil {
			return fmt.Errorf("applying local cluster secret: %w", err)
		}
	}

	if a.OciPassword == "" {
		return errors.New("OCI registry password is required but not set")
	}

	if err := a.applyHelmRegistrySecret(ctx); err != nil {
		return fmt.Errorf("applying helm registry secret: %w", err)
	}

	if a.GitPassword != "" {
		if err := a.applyGitRepoSecret(ctx); err != nil {
			return fmt.Errorf("applying git repo secret: %w", err)
		}
	}

	return nil
}

func (a *argoCDResources) applyLocalCluster(ctx context.Context) error {
	log.Println("Applying local cluster secret... ")
	rendered, err := k8s.RenderTemplate(localClusterTpl, map[string]string{
		"DC_NUMBER": a.DatacenterID,
	})
	if err != nil {
		return fmt.Errorf("rendering local cluster template: %w", err)
	}

	return k8s.ApplySecretFromYAML(ctx, a.clientset, rendered)
}

func (a *argoCDResources) applyHelmRegistrySecret(ctx context.Context) error {
	log.Println("Applying helm registry secret... ")
	rendered, err := k8s.RenderTemplate(helmRegistryTpl, map[string]string{
		"SECRET_CODESPHERE_OCI_READ": a.OciPassword,
		"OCI_REGISTRY_URL":           a.OciRegistryURL,
		"OCI_USERNAME":               a.OciUsername,
	})
	if err != nil {
		return fmt.Errorf("rendering helm registry template: %w", err)
	}

	return k8s.ApplySecretFromYAML(ctx, a.clientset, rendered)
}

func (a *argoCDResources) applyGitRepoSecret(ctx context.Context) error {
	log.Println("Applying git repo secret... ")
	rendered, err := k8s.RenderTemplate(gitRepoTpl, map[string]string{
		"SECRET_CODESPHERE_REPOS_READ": a.GitPassword,
	})
	if err != nil {
		return fmt.Errorf("rendering git repo template: %w", err)
	}

	return k8s.ApplySecretFromYAML(ctx, a.clientset, rendered)
}
