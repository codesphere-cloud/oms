package installer

import (
	"fmt"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

type Config struct {
	FileIO util.FileIO
}

type ConfigManager interface {
	ParseConfigYaml(configPath string) (files.RootConfig, error)
	ExtractOciImageIndex(imagefile string) (files.OCIImageIndex, error)
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

// ExtractOciImageIndex extracts and parses the OCI image index from the given image file path.
func (c *Config) ExtractOciImageIndex(imagefile string) (files.OCIImageIndex, error) {
	var ociImageIndex files.OCIImageIndex
	err := util.ExtractTarSingleFile(c.FileIO, imagefile, "index.json", filepath.Dir(imagefile))
	if err != nil {
		return ociImageIndex, fmt.Errorf("failed to extract index.json: %w", err)
	}

	err = ociImageIndex.ParseOCIImageConfig(imagefile)
	if err != nil {
		return ociImageIndex, fmt.Errorf("failed to parse OCI image config: %w", err)
	}

	return ociImageIndex, nil
}
