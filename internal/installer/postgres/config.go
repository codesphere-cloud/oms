// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"fmt"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
)

// ClusterSpec validates and builds the database from explicit profile or user settings.
func (b *Installer) ClusterSpec() (cnpgv1.ClusterSpec, error) {
	c := b.Config
	if err := validateRequiredConfig(c); err != nil {
		return cnpgv1.ClusterSpec{}, err
	}

	instances, synchronous := *c.Instances, *c.SynchronousReplicas

	if instances < 1 || synchronous < 0 || synchronous >= instances {
		return cnpgv1.ClusterSpec{}, fmt.Errorf("postgres.k8sManagedConfig requires positive instances and synchronousReplicas between 0 and instances-1")
	}

	affinity := c.PodAntiAffinityType
	if affinity != "required" && affinity != "preferred" {
		return cnpgv1.ClusterSpec{}, fmt.Errorf("postgres.k8sManagedConfig.podAntiAffinityType must be required or preferred")
	}

	size := c.StorageSize

	quantity, err := resource.ParseQuantity(size)
	if err != nil || quantity.Sign() <= 0 {
		return cnpgv1.ClusterSpec{}, fmt.Errorf("invalid postgres.k8sManagedConfig.storageSize %q", size)
	}

	resources, err := resourceRequirements(c.Requests, c.Limits)
	if err != nil {
		return cnpgv1.ClusterSpec{}, err
	}

	spec := cnpgv1.ClusterSpec{
		ImageName:             fmt.Sprintf("ghcr.io/cloudnative-pg/postgresql:%s-system-trixie", c.Version),
		Instances:             instances,
		StorageConfiguration:  cnpgv1.StorageConfiguration{Size: size},
		EnableSuperuserAccess: ptr.To(true),
		ImagePullSecrets:      []cnpgv1.LocalObjectReference{{Name: cnpgImagePullSecretName}},
		Resources:             resources,
		Affinity:              cnpgv1.AffinityConfiguration{EnablePodAntiAffinity: ptr.To(true), PodAntiAffinityType: affinity, TopologyKey: "kubernetes.io/hostname"},
	}
	if storageClass := c.StorageClass; storageClass != "" {
		spec.StorageConfiguration.StorageClass = ptr.To(storageClass)
	}

	if synchronous > 0 {
		spec.PostgresConfiguration.Synchronous = &cnpgv1.SynchronousReplicaConfiguration{Method: "any", Number: synchronous, DataDurability: "required"}
	}

	return spec, nil
}

func validateRequiredConfig(c *files.K8sManagedConfig) error {
	if c == nil {
		return fmt.Errorf("postgres.k8sManagedConfig is required")
	}

	if c.Instances == nil {
		return fmt.Errorf("postgres.k8sManagedConfig.instances is required")
	}

	if c.SynchronousReplicas == nil {
		return fmt.Errorf("postgres.k8sManagedConfig.synchronousReplicas is required")
	}

	if c.Version == "" {
		return fmt.Errorf("postgres.k8sManagedConfig.version is required")
	}

	if c.Requests == nil {
		return fmt.Errorf("postgres.k8sManagedConfig.requests is required")
	}

	return nil
}

func resourceRequirements(requests, limits map[string]string) (corev1.ResourceRequirements, error) {
	result := corev1.ResourceRequirements{Requests: corev1.ResourceList{}, Limits: corev1.ResourceList{}}

	for name, values := range map[string]map[string]string{"requests": requests, "limits": limits} {
		for key, value := range values {
			quantity, err := resource.ParseQuantity(value)
			if err != nil || quantity.Sign() < 0 {
				return result, fmt.Errorf("invalid postgres.k8sManagedConfig.%s.%s %q", name, key, value)
			}

			if name == "requests" {
				result.Requests[corev1.ResourceName(key)] = quantity
			} else {
				result.Limits[corev1.ResourceName(key)] = quantity
			}
		}
	}

	for key, request := range result.Requests {
		if limit, ok := result.Limits[key]; ok && request.Cmp(limit) > 0 {
			return result, fmt.Errorf("postgres.k8sManagedConfig request %s exceeds its limit", key)
		}
	}

	return result, nil
}

// MigrationConfig connects local migrations through a temporary NodePort while
// verifying the database certificate against its in-cluster service identity.
func MigrationConfig(host string, port int) *files.MigrationConfig {
	return &files.MigrationConfig{Postgres: &files.MigrationPostgresConfig{
		Host: host, Port: port, Database: DatabaseName, AltName: ServiceAddress,
	}}
}

// ConnectionConfig adapts the existing cluster's TLS connection for installer
// bundles that expect an external PostgreSQL database.
func ConnectionConfig(ca string) files.PostgresConfig {
	return files.PostgresConfig{Mode: "external", ServerAddress: ServiceAddress,
		Port: files.PostgresPort, Database: DatabaseName, CACertPem: ca}
}
