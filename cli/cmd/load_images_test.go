// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/stretchr/testify/require"
)

type fakeImageCopier struct {
	calls [][2]string
	err   error
}

func (r *fakeImageCopier) CopyAll(sourceRef string, destinationRef string) error {
	r.calls = append(r.calls, [2]string{sourceRef, destinationRef})
	return r.err
}

func TestLoadImages(t *testing.T) {
	tempDir := t.TempDir()
	packagePath := filepath.Join(tempDir, "codesphere-test.tar.gz")
	require.NoError(t, createLoadImagesTestPackage(packagePath, "bom.json", `{
	  "components": {
	    "clusterPki": {
	      "files": {
	        "chart": {
	          "ociRef": "ghcr.io/codesphere-cloud/charts/cluster-pki:0.1.6"
	        }
	      },
	      "containerImages": {
	        "cronjob": "ghcr.io/codesphere-cloud/docker/alpine/kubectl:1.34.2"
	      }
	    }
	  }
	}`))

	newPackage := func() installer.PackageManager {
		return installer.NewPackage(filepath.Join(tempDir, "oms-workdir"), packagePath)
	}

	t.Run("copies refs with fake copier", func(t *testing.T) {
		copier := &fakeImageCopier{}
		c := &cmd.LoadImagesCmd{
			Opts:   &cmd.LoadImagesOpts{},
			Copier: copier,
		}

		err := c.LoadImagesFromPackage(newPackage(), "registry.internal.example.com")
		require.NoError(t, err)
		require.Equal(t, [][2]string{
			{
				"ghcr.io/codesphere-cloud/charts/cluster-pki:0.1.6",
				"registry.internal.example.com/codesphere-cloud/charts/cluster-pki:0.1.6",
			},
			{
				"ghcr.io/codesphere-cloud/docker/alpine/kubectl:1.34.2",
				"registry.internal.example.com/codesphere-cloud/docker/alpine/kubectl:1.34.2",
			},
		}, copier.calls)
	})

	t.Run("dry-run does not call runner", func(t *testing.T) {
		copier := &fakeImageCopier{}
		c := &cmd.LoadImagesCmd{
			Opts:   &cmd.LoadImagesOpts{DryRun: true},
			Copier: copier,
		}

		err := c.LoadImagesFromPackage(newPackage(), "registry.internal.example.com")
		require.NoError(t, err)
		require.Empty(t, copier.calls)
	})

	t.Run("propagates copy errors", func(t *testing.T) {
		copier := &fakeImageCopier{err: errors.New("copy failed")}
		c := &cmd.LoadImagesCmd{
			Opts:   &cmd.LoadImagesOpts{},
			Copier: copier,
		}

		err := c.LoadImagesFromPackage(newPackage(), "registry.internal.example.com")
		require.Error(t, err)
		require.Contains(t, err.Error(), "copy failed")
	})
}

func createLoadImagesTestPackage(filename string, bomName string, bomContent string) error {
	depsContent, err := createLoadImagesDepsArchive(map[string]string{
		bomName: bomContent,
	})
	if err != nil {
		return err
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gzw := gzip.NewWriter(file)
	defer func() { _ = gzw.Close() }()

	tw := tar.NewWriter(gzw)
	defer func() { _ = tw.Close() }()

	header := &tar.Header{
		Name: "deps.tar.gz",
		Mode: 0o600,
		Size: int64(len(depsContent)),
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err = tw.Write(depsContent)
	return err
}

func createLoadImagesDepsArchive(files map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0o600,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(header); err != nil {
			return nil, err
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gzw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
