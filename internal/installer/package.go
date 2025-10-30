// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

const depsDir = "deps"

type PackageManager interface {
	FileIO() util.FileIO
	GetWorkDir() string
	GetDependencyPath(filename string) string
	Extract(force bool) error
	ExtractDependency(file string, force bool) error
	ExtractOciImageIndex(imagefile string) (files.OCIImageIndex, error)
	GetImagePathAndName(baseimage string, force bool) (string, string, error)
	GetCodesphereVersion() (string, error)
}

type Package struct {
	OmsWorkdir string
	Filename   string
	fileIO     util.FileIO
}

func NewPackage(omsWorkdir, filename string) *Package {
	return &Package{
		Filename:   filename,
		OmsWorkdir: omsWorkdir,
		fileIO:     &util.FilesystemWriter{},
	}
}

// FileIO returns the FileIO interface used by the package.
func (p *Package) FileIO() util.FileIO {
	return p.fileIO
}

// GetWorkDir returns the working directory path for the package
// by joining the OmsWorkdir and the filename (without the .tar.gz extension).
func (p *Package) GetWorkDir() string {
	return path.Join(p.OmsWorkdir, strings.ReplaceAll(p.Filename, ".tar.gz", ""))
}

// GetDependencyPath returns the full path to a dependency file within the package's deps directory.
func (p *Package) GetDependencyPath(filename string) string {
	workDir := p.GetWorkDir()
	return path.Join(workDir, depsDir, filename)
}

// alreadyExtracted checks if the package has already been extracted to the given directory.
func (p *Package) alreadyExtracted(dir string) (bool, error) {
	if !p.fileIO.Exists(dir) {
		return false, nil
	}
	isDir, err := p.fileIO.IsDirectory(dir)
	if err != nil {
		return false, fmt.Errorf("failed to determine if %s is a folder: %w", dir, err)
	}
	return isDir, nil
}

// Extract extracts the package tar.gz file into its working directory.
// If force is true, it will overwrite existing files.
func (p *Package) Extract(force bool) error {
	workDir := p.GetWorkDir()
	err := os.MkdirAll(p.OmsWorkdir, 0755)
	if err != nil {
		return fmt.Errorf("failed to ensure workdir exists: %w", err)
	}

	alreadyExtracted, err := p.alreadyExtracted(workDir)
	if err != nil {
		return fmt.Errorf("failed to figure out if package %s is already extracted in %s: %w", p.Filename, workDir, err)
	}
	if alreadyExtracted && !force {
		log.Println("Skipping extraction, package already unpacked. Use force option to overwrite.")
		return nil
	}

	err = util.ExtractTarGz(p.fileIO, p.Filename, workDir)
	if err != nil {
		return fmt.Errorf("failed to extract package %s to %s: %w", p.Filename, workDir, err)
	}

	return nil
}

// ExtractDependency extracts a specific dependency file from the deps.tar.gz archive within the package.
func (p *Package) ExtractDependency(file string, force bool) error {
	err := p.Extract(force)
	if err != nil {
		return fmt.Errorf("failed to extract package: %w", err)
	}
	workDir := p.GetWorkDir()

	if p.fileIO.Exists(p.GetDependencyPath(file)) && !force {
		log.Println("Skipping extraction, dependency already unpacked. Use force option to overwrite.")
		return nil
	}

	err = util.ExtractTarGzSingleFile(p.fileIO, path.Join(workDir, "deps.tar.gz"), file, path.Join(workDir, depsDir))
	if err != nil {
		return fmt.Errorf("failed to extract dependency %s from deps archive to %s: %w", file, workDir, err)
	}

	return err
}

// ExtractOciImageIndex extracts and parses the OCI image index from the given image file path.
func (p *Package) ExtractOciImageIndex(imagefile string) (files.OCIImageIndex, error) {
	var ociImageIndex files.OCIImageIndex
	err := util.ExtractTarSingleFile(p.fileIO, imagefile, "index.json", filepath.Dir(imagefile))
	if err != nil {
		return ociImageIndex, fmt.Errorf("failed to extract index.json: %w", err)
	}

	err = ociImageIndex.ParseOCIImageConfig(imagefile)
	if err != nil {
		return ociImageIndex, fmt.Errorf("failed to parse OCI image config: %w", err)
	}

	return ociImageIndex, nil
}

const baseimagePath = "./codesphere/images"
const defaultBaseimage = "workspace-agent-24.04.tar"

func (p *Package) GetImagePathAndName(baseimage string, force bool) (string, string, error) {
	if baseimage == "" {
		baseimage = defaultBaseimage
	}

	baseImageTarPath := path.Join(baseimagePath, defaultBaseimage)
	err := p.ExtractDependency(baseImageTarPath, force)
	if err != nil {
		return "", "", fmt.Errorf("failed to extract package to workdir: %w", err)
	}

	baseimagePath := p.GetDependencyPath(baseImageTarPath)
	index, err := p.ExtractOciImageIndex(baseimagePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to extract OCI image index: %w", err)
	}

	imagenames, err := index.ExtractImageNames()
	if err != nil || len(imagenames) == 0 {
		return "", "", fmt.Errorf("failed to read image tags: %w", err)
	}

	log.Printf("Extracted image names: %s", strings.Join(imagenames, ", "))

	baseimageName := imagenames[0]

	return baseimagePath, baseimageName, nil
}

func (p *Package) GetCodesphereVersion() (string, error) {
	_, imageName, err := p.GetImagePathAndName("", false)
	if err != nil {
		return "", fmt.Errorf("failed to get Codesphere version from package: %w", err)
	}

	parts := strings.Split(imageName, ":")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid image name format: %s", imageName)
	}

	return parts[len(parts)-1], nil
}
