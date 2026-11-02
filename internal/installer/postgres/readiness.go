// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"fmt"
	"log"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WaitForDatabaseReady waits for the current database configuration, all instance
// pods, and the primary service to be ready.
func (b *Installer) WaitForDatabaseReady() error {
	_, err := b.waitForDatabaseReady(b.Context)
	return err
}

func (b *Installer) waitForDatabaseReady(ctx context.Context) (string, error) {
	var (
		primary string
		lastErr error
	)

	err := wait.PollUntilContextTimeout(ctx, cnpgReadyPollInterval, cnpgReadyTimeout, true, func(ctx context.Context) (bool, error) {
		primary, lastErr = b.CheckDatabaseReady(ctx)
		if isRetryableWaitError(lastErr) {
			log.Printf("Waiting for PostgreSQL cluster %q: %v", b.clusterName(), lastErr)
			return false, nil
		}

		return lastErr == nil, lastErr
	})
	if err != nil {
		return "", fmt.Errorf("wait for PostgreSQL cluster %q to become ready (last observation: %v): %w", b.clusterName(), lastErr, err)
	}

	return primary, nil
}

// CheckDatabaseReady checks the live Cluster and instance pods and returns the
// ready primary selected by the read-write service. It does not wait or mutate resources.
func (b *Installer) CheckDatabaseReady(ctx context.Context) (string, error) {
	cluster := &cnpgv1.Cluster{}
	if err := b.KubeClient.Get(ctx, client.ObjectKey{Namespace: b.namespace(), Name: b.clusterName()}, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return "", &retryableWaitError{err: fmt.Errorf("cluster %q not found yet", b.clusterName())}
		}

		return "", fmt.Errorf("get PostgreSQL cluster: %w", err)
	}

	if err := clusterReady(cluster); err != nil {
		return "", &retryableWaitError{err: err}
	}

	pods := &corev1.PodList{}
	if err := b.KubeClient.List(ctx, pods, client.InNamespace(b.namespace()), client.MatchingLabels{"cnpg.io/cluster": cluster.Name, "cnpg.io/podRole": "instance"}); err != nil {
		return "", fmt.Errorf("list PostgreSQL instance pods: %w", err)
	}

	if len(pods.Items) != cluster.Spec.Instances {
		return "", &retryableWaitError{err: fmt.Errorf("found %d instance pods, want %d", len(pods.Items), cluster.Spec.Instances)}
	}

	for i := range pods.Items {
		if err := instanceReady(cluster, &pods.Items[i]); err != nil {
			return "", &retryableWaitError{err: err}
		}
	}

	return b.readyPrimary(ctx, cluster, pods.Items)
}

func clusterReady(cluster *cnpgv1.Cluster) error {
	ready := apimeta.FindStatusCondition(cluster.Status.Conditions, string(cnpgv1.ConditionClusterReady))
	if ready == nil || ready.Status != metav1.ConditionTrue {
		return fmt.Errorf("cluster Ready condition is not true (phase=%q)", cluster.Status.Phase)
	}
	// CNPG 1.30 does not populate observedGeneration on the Ready condition. When
	// available, respect it; otherwise validate the live pod configuration below.
	if ready.ObservedGeneration > 0 && ready.ObservedGeneration < cluster.Generation {
		return fmt.Errorf("cluster Ready condition observes generation %d, want %d", ready.ObservedGeneration, cluster.Generation)
	}

	if cluster.Status.Phase != cnpgv1.PhaseHealthy || cluster.Status.Instances != cluster.Spec.Instances || cluster.Status.ReadyInstances != cluster.Spec.Instances {
		return fmt.Errorf("cluster is not reconciled (phase=%q, readyInstances=%d, instances=%d, desired=%d)", cluster.Status.Phase, cluster.Status.ReadyInstances, cluster.Status.Instances, cluster.Spec.Instances)
	}

	if cluster.Status.CurrentPrimary == "" || cluster.Status.CurrentPrimary != cluster.Status.TargetPrimary {
		return fmt.Errorf("cluster primary is not settled (current=%q, target=%q)", cluster.Status.CurrentPrimary, cluster.Status.TargetPrimary)
	}

	return nil
}

func instanceReady(cluster *cnpgv1.Cluster, pod *corev1.Pod) error {
	if !metav1.IsControlledBy(pod, cluster) {
		return fmt.Errorf("pod %q does not belong to the current cluster", pod.Name)
	}

	if !podReady(pod) {
		return fmt.Errorf("pod %q is not ready (phase=%q, waiting=%s)", pod.Name, pod.Status.Phase, podWaitingReason(pod))
	}

	for _, secret := range cluster.Spec.ImagePullSecrets {
		if !hasPullSecret(pod.Spec.ImagePullSecrets, secret.Name) {
			return fmt.Errorf("pod %q is missing image pull secret %q", pod.Name, secret.Name)
		}
	}

	for _, container := range pod.Spec.Containers {
		if container.Name != "postgres" {
			continue
		}

		if container.Image != cluster.Spec.ImageName {
			return fmt.Errorf("pod %q uses image %q, want %q", pod.Name, container.Image, cluster.Spec.ImageName)
		}

		if !resourceListMatches(cluster.Spec.Resources.Requests, container.Resources.Requests) || !resourceListMatches(cluster.Spec.Resources.Limits, container.Resources.Limits) {
			return fmt.Errorf("pod %q resources do not match the current cluster specification", pod.Name)
		}

		return nil
	}

	return fmt.Errorf("pod %q has no postgres container", pod.Name)
}

func hasPullSecret(secrets []corev1.LocalObjectReference, name string) bool {
	for _, secret := range secrets {
		if secret.Name == name {
			return true
		}
	}

	return false
}

func resourceListMatches(desired, actual corev1.ResourceList) bool {
	for name, quantity := range desired {
		if quantity.Cmp(actual[name]) != 0 {
			return false
		}
	}

	return true
}

func podWaitingReason(pod *corev1.Pod) string {
	for _, status := range append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...) {
		if status.State.Waiting != nil {
			return status.State.Waiting.Reason
		}
	}

	return "none"
}

func (b *Installer) readyPrimary(ctx context.Context, cluster *cnpgv1.Cluster, pods []corev1.Pod) (string, error) {
	service := &corev1.Service{}
	if err := b.KubeClient.Get(ctx, client.ObjectKey{Namespace: b.namespace(), Name: b.serviceName()}, service); err != nil {
		if apierrors.IsNotFound(err) {
			return "", &retryableWaitError{err: fmt.Errorf("read-write service not found yet")}
		}

		return "", fmt.Errorf("get PostgreSQL read-write service: %w", err)
	}

	if len(service.Spec.Selector) > 0 {
		selector := labels.SelectorFromSet(service.Spec.Selector)
		for _, pod := range pods {
			if pod.Name == cluster.Status.CurrentPrimary && selector.Matches(labels.Set(pod.Labels)) {
				return pod.Name, nil
			}
		}
	}

	return "", &retryableWaitError{err: fmt.Errorf("no ready primary pod backs service %s/%s", b.namespace(), b.serviceName())}
}

func podReady(pod *corev1.Pod) bool {
	if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
		return false
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}

	return false
}
