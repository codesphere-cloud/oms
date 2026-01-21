// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"io"

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
	rootConfig := files.NewRootConfig()

	file, err := c.FileIO.Open(configPath)
	if err != nil {
		return rootConfig, fmt.Errorf("failed to open config file: %w", err)
	}
	defer util.CloseFileIgnoreError(file)

	data, err := io.ReadAll(file)
	if err != nil {
		return rootConfig, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := rootConfig.Unmarshal(data); err != nil {
		return rootConfig, fmt.Errorf("failed to parse config.yaml: %w", err)
	}

	return rootConfig, nil
}
