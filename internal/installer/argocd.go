// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type ArgoCDManager interface {
	PreInstall() error
	Install() error
	PostInstall() error
}

type ArgoCD struct {
	Version string
}

func NewArgoCD() ArgoCDManager {
	return &ArgoCD{
		Version: "9.1.4",
	}
}

func createArgocdNamespace() error {
	home, _ := os.UserHomeDir()
	kubeconfigPath := filepath.Join(home, ".kube", "config")

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{CurrentContext: ""}, // Empty string means the current select context
	).ClientConfig()

	if err != nil {
		return fmt.Errorf("Error loading current context: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	namespace := "argocd"
	_, err = clientset.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})

	if err == nil {
		fmt.Printf("Namespace %s already exists\n", namespace)
		return nil
	}

	if errors.IsNotFound(err) {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})

		if err != nil {
			return fmt.Errorf("Error: %v\n", err)
		} else {
			log.Println("Created namespace 'argocd' using the active context.")
		}
	}
	return err
}

// Install resources needed by ArgoCD
func (a *ArgoCD) PreInstall() error {
	err := createArgocdNamespace()
	if err != nil {
		return fmt.Errorf("Error creating namespace argocd: %v", err)
	}

	// TODO: argocd secret
	return nil
}

// PostInstall implements ArgoCDManager.
func (a *ArgoCD) PostInstall() error {
	panic("unimplemented")
}

// Install the ArgoCD chart
func (a *ArgoCD) Install() error {
	log.Println("Installing ArgoCD helm chart version %s", a.Version)
	settings := cli.New()
	ctx := context.Background()

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), os.Getenv("HELM_DRIVER")); err != nil {
		log.Fatalf("Init failed: %v", err)
	}

	client := action.NewInstall(actionConfig)
	client.ReleaseName = "argocd"
	client.Namespace = "argocd"
	client.CreateNamespace = true
	client.DryRunStrategy = "none"
	client.WaitStrategy = "watcher"
	client.Version = a.Version

	client.ChartPathOptions.RepoURL = "https://argoproj.github.io/argo-helm"

	chartPath, err := client.ChartPathOptions.LocateChart("argo-cd", settings)
	if err != nil {
		log.Fatalf("LocateChart failed: %v", err)
	}

	chartRequested, err := loader.Load(chartPath)
	if err != nil {
		log.Fatalf("Load failed: %v", err)
	}

	vals := map[string]interface{}{
		"dex": map[string]interface{}{
			"enabled": false,
		},
		"configs": map[string]interface{}{
			"secret": map[string]interface{}{
				"createSecret": true,
			},
		},
	}

	_, err = client.RunWithContext(ctx, chartRequested, vals)
	if err != nil {
		log.Fatalf("Install failed: %v", err)
	}

	fmt.Printf("Successfully installed Argo CD %s (Chart version: %s)\n", client.ReleaseName, client.Version)
	return nil
}
