// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

type Config struct {
	FileIO util.FileIO
}

type ConfigManager interface {
	ParseConfigYaml(configPath string) (files.RootConfig, error)
}

func NewConfig() *Config {
	return &Config{
		FileIO: &util.FilesystemWriter{},
	}
}

// ParseConfigYaml reads and parses the configuration YAML file at the given path.
func (c *Config) ParseConfigYaml(configPath string) (files.RootConfig, error) {
	var rootConfig files.RootConfig
	err := rootConfig.ParseConfig(configPath)
	if err != nil {
		return rootConfig, fmt.Errorf("failed to parse config.yaml: %w", err)
	}

	return rootConfig, nil
}
