// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	_ "embed"
	"fmt"
	"log"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

//go:embed manifests/argocd/app-projects.yaml
var appProjectsYAML []byte

//go:embed manifests/argocd/cluster-local.yaml.tpl
var localClusterTpl []byte

//go:embed manifests/argocd/repo-helm-oci.yaml.tpl
var helmRegistryTpl []byte

//go:embed manifests/argocd/repo-creds-git.yaml.tpl
var gitRepoTpl []byte

func applyAppProjects(ctx context.Context, dynClient dynamic.Interface) error {
	log.Println("Applying AppProjects... ")
	objects, err := decodeMultiDocYAML(appProjectsYAML)
	if err != nil {
		return fmt.Errorf("decoding app projects yaml: %w", err)
	}

	for _, obj := range objects {
		gvr, err := gvrForUnstructured(obj)
		if err != nil {
			return err
		}
		if err := applyUnstructured(ctx, dynClient, gvr, obj); err != nil {
			return fmt.Errorf("applying app project %q: %w", obj.GetName(), err)
		}
	}
	return nil
}

func applyLocalCluster(ctx context.Context, clientset kubernetes.Interface, dcNumber string) error {
	log.Println("Applying local cluster secret... ")
	rendered, err := renderTemplate(localClusterTpl, map[string]string{
		"DC_NUMBER": dcNumber,
	})
	if err != nil {
		return fmt.Errorf("rendering local cluster template: %w", err)
	}

	return applySecretFromYAML(ctx, clientset, rendered)
}

func applyHelmRegistrySecret(ctx context.Context, clientset kubernetes.Interface, ociReadPassword string) error {
	log.Println("Applying helm registry secret... ")
	rendered, err := renderTemplate(helmRegistryTpl, map[string]string{
		"SECRET_CODESPHERE_OCI_READ": ociReadPassword,
	})
	if err != nil {
		return fmt.Errorf("rendering helm registry template: %w", err)
	}

	return applySecretFromYAML(ctx, clientset, rendered)
}

func applyGitRepoSecret(ctx context.Context, clientset kubernetes.Interface, reposReadPassword string) error {
	log.Println("Applying git repo secret... ")
	rendered, err := renderTemplate(gitRepoTpl, map[string]string{
		"SECRET_CODESPHERE_REPOS_READ": reposReadPassword,
	})
	if err != nil {
		return fmt.Errorf("rendering git repo template: %w", err)
	}

	return applySecretFromYAML(ctx, clientset, rendered)
}
