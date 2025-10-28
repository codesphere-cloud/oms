// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"github.com/codesphere-cloud/oms/internal/util"
)

const depsDir = "deps"

type PackageManager interface {
	FileIO() util.FileIO
	Extract(force bool) error
	ExtractDependency(file string, force bool) error
	GetWorkDir() string
	GetDependencyPath(filename string) string
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
		log.Println("skipping extraction, package already unpacked. Use force option to overwrite.")
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
		log.Println("skipping extraction, dependency already unpacked. Use force option to overwrite.")
		return nil
	}

	err = util.ExtractTarGzSingleFile(p.fileIO, path.Join(workDir, "deps.tar.gz"), file, path.Join(workDir, depsDir))
	if err != nil {
		return fmt.Errorf("failed to extract dependency %s from deps archive to %s: %w", file, workDir, err)
	}

	return err
}

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
