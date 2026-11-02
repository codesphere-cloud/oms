// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const migrationServicePrefix = "oms-masterdata-nodeport-"

// MigrationEndpoint exposes PostgreSQL for the lifetime of a local installation.
type MigrationEndpoint struct {
	Host    string
	Port    int
	kube    client.Client
	service *corev1.Service
}

// Close deletes the temporary Service even when installation was canceled.
func (e *MigrationEndpoint) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := e.kube.Delete(ctx, e.service); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete temporary PostgreSQL NodePort service %s/%s: %w", e.service.Namespace, e.service.Name, err)
	}

	return nil
}

// CreateMigrationEndpoint uses a NodePort so closing a PostgreSQL TLS session
// cannot tear down the shared client-go port-forward. See README.md for the
// upstream issues and why reconnecting a port-forward is insufficient.
func (b *Installer) CreateMigrationEndpoint(ctx context.Context) (*MigrationEndpoint, error) {
	if err := b.cleanupMigrationEndpoints(ctx); err != nil {
		return nil, err
	}

	if _, err := b.waitForDatabaseReady(ctx); err != nil {
		return nil, err
	}

	source := &corev1.Service{}
	if err := b.KubeClient.Get(ctx, client.ObjectKey{Namespace: b.namespace(), Name: b.serviceName()}, source); err != nil {
		return nil, fmt.Errorf("get PostgreSQL read-write service for migrations: %w", err)
	}

	if len(source.Spec.Selector) == 0 {
		return nil, fmt.Errorf("PostgreSQL service %s/%s has no selector", source.Namespace, source.Name)
	}

	port, err := postgresServicePort(source)
	if err != nil {
		return nil, err
	}

	host, err := b.nodePortHost(ctx)
	if err != nil {
		return nil, err
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{GenerateName: migrationServicePrefix, Namespace: b.namespace()},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort, Selector: source.Spec.Selector,
			Ports: []corev1.ServicePort{{Name: "postgres", Protocol: corev1.ProtocolTCP, Port: port.Port, TargetPort: port.TargetPort}},
		},
	}
	if err := b.KubeClient.Create(ctx, service); err != nil {
		return nil, fmt.Errorf("create temporary PostgreSQL NodePort service: %w", err)
	}

	endpoint := &MigrationEndpoint{Host: host, kube: b.KubeClient, service: service}
	if len(service.Spec.Ports) != 1 || service.Spec.Ports[0].NodePort == 0 {
		return nil, errors.Join(fmt.Errorf("temporary PostgreSQL NodePort service %s/%s has no allocated nodePort", service.Namespace, service.Name), endpoint.Close())
	}

	endpoint.Port = int(service.Spec.Ports[0].NodePort)

	return endpoint, nil
}

// Recover endpoints left behind when the installer process could not run its
// deferred cleanup. The reserved prefix also covers Services from older OMS runs.
func (b *Installer) cleanupMigrationEndpoints(ctx context.Context) error {
	services := &corev1.ServiceList{}
	if err := b.KubeClient.List(ctx, services, client.InNamespace(b.namespace())); err != nil {
		return fmt.Errorf("list temporary PostgreSQL NodePort services: %w", err)
	}

	for i := range services.Items {
		service := &services.Items[i]
		if !strings.HasPrefix(service.Name, migrationServicePrefix) || service.Spec.Type != corev1.ServiceTypeNodePort || service.Spec.Selector["cnpg.io/cluster"] != b.clusterName() {
			continue
		}

		if err := b.KubeClient.Delete(ctx, service); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete stale PostgreSQL NodePort service %s/%s: %w", service.Namespace, service.Name, err)
		}
	}

	return nil
}

func postgresServicePort(service *corev1.Service) (corev1.ServicePort, error) {
	for _, port := range service.Spec.Ports {
		if port.Port != 5432 {
			continue
		}

		if port.TargetPort == (intstr.IntOrString{}) {
			port.TargetPort = intstr.FromInt32(port.Port)
		}

		return port, nil
	}

	return corev1.ServicePort{}, fmt.Errorf("PostgreSQL service %s/%s has no port 5432", service.Namespace, service.Name)
}

func (b *Installer) nodePortHost(ctx context.Context) (string, error) {
	nodes := &corev1.NodeList{}
	if err := b.KubeClient.List(ctx, nodes); err != nil {
		return "", fmt.Errorf("list cluster nodes for PostgreSQL NodePort endpoint: %w", err)
	}

	for _, node := range nodes.Items {
		for _, address := range node.Status.Addresses {
			if address.Type == corev1.NodeInternalIP && address.Address != "" {
				return address.Address, nil
			}
		}

		for _, address := range node.Status.Addresses {
			if address.Type == corev1.NodeExternalIP && address.Address != "" {
				return address.Address, nil
			}
		}
	}

	return "", fmt.Errorf("no node IP address available for PostgreSQL NodePort endpoint")
}
