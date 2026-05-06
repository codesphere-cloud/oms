// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"go.yaml.in/yaml/v3"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const lts177InstallVersion = "codesphere-lts-v1.77.2"

// lts177CodesphereRemotePath is the path where the separate codesphere config
// file will be placed on the jumpbox for the LTS 1.77.2 installer.
const lts177CodesphereRemotePath = "/etc/codesphere/codesphere.yaml"

// lts177UnsupportedExperiments are experiments that did not exist in the LTS 1.77.2 release
// and therefore must be removed from the config before passing it to the LTS installer.
var lts177UnsupportedExperiments = []string{"secret-management", "sub-path-mount"}

// IsLTS177 reports whether the given installVersion is the LTS 1.77.2 release.
func IsLTS177(installVersion string) bool {
	return installVersion == lts177InstallVersion
}

// SetLTS177CodesphereInRoot applies LTS 1.77.2 compatibility to the RootConfig:
//  1. Filters unsupported experiments from codesphere.experiments.
//  2. Strips each ManagedServiceConfig down to {Name, Version} only.
//  3. Sets CodesphereConfigPath
func SetLTS177CodesphereInRoot(root *files.RootConfig) ([]byte, error) {
	ApplyLTS177Compat(&root.Codesphere)
	root.CodesphereConfigPath = lts177CodesphereRemotePath

	data, err := yaml.Marshal(root.Codesphere)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal LTS 1.77.2 codesphere config: %w", err)
	}
	return data, nil
}

// LTS177LocalCodesphereConfigPath derives the local path for the separate codesphere
// config file from the main config path (same directory, filename "codesphere.yaml").
func LTS177LocalCodesphereConfigPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "codesphere.yaml")
}

// LTS177LocalJumpboxConfigPath derives the local path for the jumpbox-specific config.yaml
// from the main config path (same directory, filename "config-jumpbox.yaml").
func LTS177LocalJumpboxConfigPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "config-jumpbox.yaml")
}

// GenerateLTS177JumpboxFiles generates the two files needed on the jumpbox for LTS 1.77.2
// without modifying the original root config:
//   - jumpboxConfigBytes: config.yaml with inline compat-stripped codesphere object
//   - codesphereBytes: standalone codesphere.yaml with the same compat-stripped codesphere config
func GenerateLTS177JumpboxFiles(root *files.RootConfig) (jumpboxConfigBytes, codesphereBytes []byte, err error) {
	csCopy := root.Codesphere
	ApplyLTS177Compat(&csCopy)

	codesphereBytes, err = yaml.Marshal(csCopy)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal LTS 1.77.2 codesphere config: %w", err)
	}

	// Generate the jumpbox config with the compat-stripped codesphere inlined.
	rootCopy := *root
	rootCopy.Codesphere = csCopy

	jumpboxConfigBytes, err = rootCopy.Marshal()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal LTS 1.77.2 jumpbox config: %w", err)
	}

	return jumpboxConfigBytes, codesphereBytes, nil
}

// ApplyLTS177Compat adjusts the CodesphereConfig to be compatible with the LTS 1.77.2 installer:
//
//  1. Experiments that did not exist at the 1.77.2 release are removed.
//  2. ManagedServices is cleared: the LTS 1.77.2 schema requires full provider definitions
//     which are not stored in config.yaml. Setting the field to nil causes it to be omitted from the YAML,
//     which passes the toUndefOr validator in the LTS 1.77.2 private-cloud-installer.js.
func ApplyLTS177Compat(cfg *files.CodesphereConfig) {
	cfg.Experiments = FilterExperiments(cfg.Experiments, lts177UnsupportedExperiments)
	cfg.ManagedServices = nil
}

// FilterExperiments returns a new slice of experiments with the unsupported ones removed.
func FilterExperiments(experiments, unsupported []string) []string {
	unsupportedSet := make(map[string]struct{}, len(unsupported))
	for _, u := range unsupported {
		unsupportedSet[u] = struct{}{}
	}

	filtered := make([]string, 0, len(experiments))
	for _, exp := range experiments {
		if _, remove := unsupportedSet[exp]; !remove {
			filtered = append(filtered, exp)
		}
	}
	return filtered
}

// BuildOmsLinuxBinary returns the path to an OMS binary built for linux/amd64.
func BuildOmsLinuxBinary() (path string, cleanup func(), err error) {
	noop := func() {}

	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		execPath, err := os.Executable()
		if err != nil {
			return "", noop, fmt.Errorf("failed to locate current OMS binary: %w", err)
		}
		return execPath, noop, nil
	}

	// Cross-compile for linux/amd64 from the current working directory (project root).
	cwd, err := os.Getwd()
	if err != nil {
		return "", noop, fmt.Errorf("failed to determine project directory: %w", err)
	}

	outFile, err := os.CreateTemp("", "oms-linux-amd64-*")
	if err != nil {
		return "", noop, fmt.Errorf("failed to create temp file for OMS binary: %w", err)
	}
	if err = outFile.Close(); err != nil {
		return "", noop, fmt.Errorf("failed to close temp file for OMS binary: %w", err)
	}
	outPath := outFile.Name()
	rmCleanup := func() { _ = os.Remove(outPath) }

	cmd := exec.Command("go", "build", "-o", outPath, "./cli")
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")
	if output, cmdErr := cmd.CombinedOutput(); cmdErr != nil {
		rmCleanup()
		return "", noop, fmt.Errorf("failed to cross-compile OMS binary for linux/amd64: %w\n%s", cmdErr, output)
	}

	return outPath, rmCleanup, nil
}
