package installer

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/codesphere-cloud/oms/internal/util"
)

const depsDir = "deps"

type Package struct {
	OmsWorkdir string
	Filename   string
	FileIO     util.FileIO
}

func NewPackage(omsWorkdir, filename string) *Package {
	return &Package{
		Filename:   filename,
		OmsWorkdir: omsWorkdir,
		FileIO:     &util.FilesystemWriter{},
	}
}

func (p *Package) Extract(force bool) error {
	workDir := p.getWorkDir()
	err := os.MkdirAll(p.OmsWorkdir, 0755)
	if err != nil {
		return fmt.Errorf("failed to ensure workdir exists: %w", err)
	}

	alreadyExtracted, err := p.alreadyExtracted(workDir)
	if err != nil {
		return fmt.Errorf("failed to figure out if package %s is already extracted in %s: %w", p.Filename, workDir, err)
	}
	if alreadyExtracted && !force {
		fmt.Println("skipping extraction, package already unpacked. Use force option to overwrite.")
		return nil
	}

	err = util.ExtractTarGz(p.FileIO, p.Filename, workDir)
	if err != nil {
		return fmt.Errorf("failed to extract package %s to %s: %w", p.Filename, workDir, err)
	}

	return nil
}

func (p *Package) ExtractDependency(file string, force bool) error {
	err := p.Extract(force)
	if err != nil {
		return fmt.Errorf("failed to extract package: %w", err)
	}
	workDir := p.getWorkDir()

	if p.FileIO.Exists(p.GetDependencyPath(file)) && !force {
		fmt.Println("skipping extraction, dependency already unpacked. Use force option to overwrite.")
		return nil
	}

	err = util.ExtractTarGzSingleFile(p.FileIO, path.Join(workDir, "deps.tar.gz"), file, path.Join(workDir, depsDir))
	if err != nil {
		return fmt.Errorf("failed to extract dependency %s from deps archive to %s: %w", file, workDir, err)
	}

	return err
}

func (p *Package) alreadyExtracted(dir string) (bool, error) {
	if !p.FileIO.Exists(dir) {
		return false, nil
	}
	isDir, err := p.FileIO.IsDirectory(dir)
	if err != nil {
		return false, fmt.Errorf("failed to determine if %s is a folder: %w", dir, err)
	}
	return isDir, nil
}

func (p *Package) getWorkDir() string {
	return path.Join(p.OmsWorkdir, strings.ReplaceAll(p.Filename, ".tar.gz", ""))
}

func (p *Package) GetDependencyPath(filename string) string {
	workDir := p.getWorkDir()
	return path.Join(workDir, depsDir, filename)
}
