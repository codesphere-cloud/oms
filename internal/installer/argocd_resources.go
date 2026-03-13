// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	_ "embed"
	"fmt"
	"log"

	k8s "github.com/codesphere-cloud/oms/internal/util"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type ArgoCDResources interface {
	ApplyAll(ctx context.Context) error
}

type argoCDResources struct {
	clientset kubernetes.Interface
	dynClient dynamic.Interface

	DatacenterId string
	OciPassword  string
	GitPassword  string
}

func NewArgoCDResources(dataCenterId string, ociPassword string, gitPassword string) (ArgoCDResources, error) {
	clientset, dynClient, err := k8s.NewClients()
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes clients: %w", err)
	}

	return &argoCDResources{
		clientset:    clientset,
		dynClient:    dynClient,
		DatacenterId: dataCenterId,
		OciPassword:  ociPassword,
		GitPassword:  gitPassword,
	}, nil
}

//go:embed manifests/argocd/app-projects.yaml
var appProjectsYAML []byte

//go:embed manifests/argocd/cluster-local.yaml.tpl
var localClusterTpl []byte

//go:embed manifests/argocd/repo-helm-oci.yaml.tpl
var helmRegistryTpl []byte

//go:embed manifests/argocd/repo-creds-git.yaml.tpl
var gitRepoTpl []byte

func (a *argoCDResources) ApplyAll(ctx context.Context) error {
	if err := a.applyAppProjects(ctx); err != nil {
		return fmt.Errorf("applying app projects: %w", err)
	}

	if err := a.applyLocalCluster(ctx); err != nil {
		return fmt.Errorf("applying local cluster secret: %w", err)
	}

	if err := a.applyHelmRegistrySecret(ctx); err != nil {
		return fmt.Errorf("applying helm registry secret: %w", err)
	}

	if err := a.applyGitRepoSecret(ctx); err != nil {
		return fmt.Errorf("applying git repo secret: %w", err)
	}

	return nil
}

func (a *argoCDResources) applyAppProjects(ctx context.Context) error {
	log.Println("Applying AppProjects... ")
	objects, err := k8s.DecodeMultiDocYAML(appProjectsYAML)
	if err != nil {
		return fmt.Errorf("decoding app projects yaml: %w", err)
	}

	for _, obj := range objects {
		gvr, err := k8s.GvrForUnstructured(obj)
		if err != nil {
			return err
		}
		if err := k8s.ApplyUnstructured(ctx, a.dynClient, gvr, obj); err != nil {
			return fmt.Errorf("applying app project %q: %w", obj.GetName(), err)
		}
	}
	return nil
}

func (a *argoCDResources) applyLocalCluster(ctx context.Context) error {
	log.Println("Applying local cluster secret... ")
	rendered, err := k8s.RenderTemplate(localClusterTpl, map[string]string{
		"DC_NUMBER": a.DatacenterId,
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
