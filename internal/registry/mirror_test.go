// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package registry_test

import (
	"errors"
	"testing"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/registry"
	"github.com/stretchr/testify/require"
)

type fakeCopier struct {
	calls [][2]string
	err   error
}

func (c *fakeCopier) CopyAll(sourceRef string, destinationRef string) error {
	c.calls = append(c.calls, [2]string{sourceRef, destinationRef})
	return c.err
}

func TestTargetImageRef(t *testing.T) {
	target, err := registry.TargetImageRef(
		"ghcr.io/codesphere-cloud/charts/gateway:0.13.3",
		"https://registry.internal.example.com/mirror/",
	)
	require.NoError(t, err)
	require.Equal(
		t,
		"registry.internal.example.com/mirror/codesphere-cloud/charts/gateway:0.13.3",
		target,
	)

	_, err = registry.TargetImageRef("quay.io/prometheus/prometheus:v2.51.0", "registry.internal.example.com")
	require.Error(t, err)

	_, err = registry.TargetImageRef("ghcr.io/codesphere-cloud/charts/gateway:0.13.3", "")
	require.Error(t, err)
}

func TestMirrorGHCRImages(t *testing.T) {
	config := &bom.Config{
		Components: map[string]bom.ComponentConfig{
			"cluster-pki": {
				ContainerImages: map[string]string{
					"cronjob": "ghcr.io/codesphere-cloud/docker/alpine/kubectl:1.34.2",
				},
			},
		},
	}

	t.Run("copies all refs", func(t *testing.T) {
		copier := &fakeCopier{}
		mirror := &registry.Mirror{Copier: copier}

		count, err := mirror.MirrorGHCRImages(config, "registry.internal.example.com")
		require.NoError(t, err)
		require.Equal(t, 1, count)
		require.Equal(t, [][2]string{{
			"ghcr.io/codesphere-cloud/docker/alpine/kubectl:1.34.2",
			"registry.internal.example.com/codesphere-cloud/docker/alpine/kubectl:1.34.2",
		}}, copier.calls)
	})

	t.Run("does not copy on dry run", func(t *testing.T) {
		copier := &fakeCopier{}
		mirror := &registry.Mirror{Copier: copier, DryRun: true}

		count, err := mirror.MirrorGHCRImages(config, "registry.internal.example.com")
		require.NoError(t, err)
		require.Equal(t, 1, count)
		require.Empty(t, copier.calls)
	})

	t.Run("propagates copy errors", func(t *testing.T) {
		mirror := &registry.Mirror{Copier: &fakeCopier{err: errors.New("copy failed")}}

		_, err := mirror.MirrorGHCRImages(config, "registry.internal.example.com")
		require.Error(t, err)
		require.Contains(t, err.Error(), "copy failed")
	})
}
