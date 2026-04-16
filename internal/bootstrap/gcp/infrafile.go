// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"encoding/json"
	"fmt"

	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/util"
)

func GetInfraFilePath() string {
	workdir := env.NewEnv().GetOmsWorkdir()
	return fmt.Sprintf("%s/gcp-infra.json", workdir)
}

// LoadInfraFile reads and parses the GCP infrastructure file from the specified path.
// Returns the environment, whether the file exists, and any error.
// If the file doesn't exist, returns an empty environment with exists=false and nil error.
func LoadInfraFile(fw util.FileIO, infraFilePath string) (CodesphereEnvironment, bool, error) {
	if !fw.Exists(infraFilePath) {
		return CodesphereEnvironment{}, false, nil
	}

	content, err := fw.ReadFile(infraFilePath)
	if err != nil {
		return CodesphereEnvironment{}, true, fmt.Errorf("failed to read gcp infra file: %w", err)
	}

	var env CodesphereEnvironment
	if err := json.Unmarshal(content, &env); err != nil {
		return CodesphereEnvironment{}, true, fmt.Errorf("failed to unmarshal gcp infra file: %w", err)
	}
	return env, true, nil
}

// WriteInfraFile writes details about the bootstrapped codesphere environment into a file.
func (b *GCPBootstrapper) WriteInfraFile() error {
	envBytes, err := json.MarshalIndent(b.Env, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal codesphere env: %w", err)
	}

	workdir := env.NewEnv().GetOmsWorkdir()
	fw := util.NewFilesystemWriter()

	err = fw.MkdirAll(workdir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create workdir %w", err)
	}

	infraFilePath := GetInfraFilePath()
	err = fw.WriteFile(infraFilePath, envBytes, 0644)
	if err != nil {
		return fmt.Errorf("failed to write gcp bootstrap env file: %w", err)
	}

	b.stlog.Logf("Infrastructure details written to %s", infraFilePath)

	return nil
}
