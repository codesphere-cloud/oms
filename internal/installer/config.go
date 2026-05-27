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

//mockery:generate: true
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
	rootConfig := files.NewRootConfig()

	data, err := c.FileIO.ReadFile(configPath)
	if err != nil {
		return rootConfig, fmt.Errorf("failed to open config file: %w", err)
	}

	if err := rootConfig.Unmarshal(data); err != nil {
		return rootConfig, fmt.Errorf("failed to parse config.yaml: %w", err)
	}

	return rootConfig, nil
}
