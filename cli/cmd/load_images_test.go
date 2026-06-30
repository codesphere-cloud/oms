// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/installer"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeImageCopier struct {
	calls [][2]string
	err   error
}

func (r *fakeImageCopier) Copy(sourceRef string, destinationRef string) error {
	r.calls = append(r.calls, [2]string{sourceRef, destinationRef})
	return r.err
}

var _ = Describe("LoadImages", func() {
	var (
		tempDir    string
		newPackage func() installer.PackageManager
	)

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
		packagePath := filepath.Join(tempDir, "codesphere-test.tar.gz")
		Expect(createLoadImagesTestPackage(packagePath, "bom.json", `{
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
		}`)).To(Succeed())

		newPackage = func() installer.PackageManager {
			return installer.NewPackage(filepath.Join(tempDir, "oms-workdir"), packagePath)
		}
	})

	It("copies refs with fake copier", func() {
		copier := &fakeImageCopier{}
		c := &cmd.LoadImagesCmd{
			Opts:   &cmd.LoadImagesOpts{},
			Copier: copier,
		}

		err := c.LoadImagesFromPackage(context.TODO(), newPackage(), "registry.internal.example.com")
		Expect(err).NotTo(HaveOccurred())
		Expect(copier.calls).To(Equal([][2]string{
			{
				"ghcr.io/codesphere-cloud/charts/cluster-pki:0.1.6",
				"registry.internal.example.com/codesphere-cloud/charts/cluster-pki:0.1.6",
			},
			{
				"ghcr.io/codesphere-cloud/docker/alpine/kubectl:1.34.2",
				"registry.internal.example.com/codesphere-cloud/docker/alpine/kubectl:1.34.2",
			},
		}))
	})

	It("does not call the copier during dry-run", func() {
		copier := &fakeImageCopier{}
		c := &cmd.LoadImagesCmd{
			Opts:   &cmd.LoadImagesOpts{DryRun: true},
			Copier: copier,
		}

		err := c.LoadImagesFromPackage(context.TODO(), newPackage(), "registry.internal.example.com")
		Expect(err).NotTo(HaveOccurred())
		Expect(copier.calls).To(BeEmpty())
	})

	It("propagates copy errors", func() {
		copier := &fakeImageCopier{err: errors.New("copy failed")}
		c := &cmd.LoadImagesCmd{
			Opts:   &cmd.LoadImagesOpts{},
			Copier: copier,
		}

		err := c.LoadImagesFromPackage(context.TODO(), newPackage(), "registry.internal.example.com")
		Expect(err).To(MatchError(ContainSubstring("copy failed")))
	})
})

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
