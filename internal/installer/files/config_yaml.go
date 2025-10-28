package files

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// RootConfig represents the relevant parts of the configuration file
type RootConfig struct {
	Registry   RegistryConfig   `yaml:"registry"`
	Codesphere CodesphereConfig `yaml:"codesphere"`
}

type RegistryConfig struct {
	Server string `yaml:"server"`
}

type CodesphereConfig struct {
	DeployConfig DeployConfig `yaml:"deployConfig"`
}

type DeployConfig struct {
	Images map[string]ImageConfig `yaml:"images"`
}

type ImageConfig struct {
	Name           string                  `yaml:"name"`
	SupportedUntil string                  `yaml:"supportedUntil"`
	Flavors        map[string]FlavorConfig `yaml:"flavors"`
}

type FlavorConfig struct {
	Image ImageRef    `yaml:"image"`
	Pool  map[int]int `yaml:"pool"`
}

type ImageRef struct {
	BomRef     string `yaml:"bomRef"`
	Dockerfile string `yaml:"dockerfile"`
}

func (c *RootConfig) ParseConfig(filePath string) error {
	configData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	err = yaml.Unmarshal(configData, c)
	if err != nil {
		return fmt.Errorf("failed to parse YAML config: %w", err)
	}

	return nil
}

func (c *RootConfig) ExtractBomRefs() []string {
	var bomRefs []string
	for _, imageConfig := range c.Codesphere.DeployConfig.Images {
		for _, flavor := range imageConfig.Flavors {
			if flavor.Image.BomRef != "" {
				bomRefs = append(bomRefs, flavor.Image.BomRef)
			}
		}
	}

	return bomRefs
}

func (c *RootConfig) ExtractWorkspaceDockerfiles() map[string]string {
	dockerfiles := make(map[string]string)
	for _, imageConfig := range c.Codesphere.DeployConfig.Images {
		for _, flavor := range imageConfig.Flavors {
			if flavor.Image.Dockerfile != "" {
				dockerfiles[flavor.Image.Dockerfile] = flavor.Image.BomRef
			}
		}
	}
	return dockerfiles
}
