package system

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/util"
)

type DockerEngine struct {
}

type ContainerEngine interface {
	LoadLocalContainerImage(filename string) error
	BuildImage(dockerfile string) error
}

func NewDockerEngine() *DockerEngine {
	return &DockerEngine{}
}

func (d *DockerEngine) LoadLocalContainerImage(imagefile string) error {
	err := d.RunCommand([]string{"load", "--input", imagefile})

	if err != nil {
		return fmt.Errorf("failed to load image %s: %w", imagefile, err)
	}

	return nil
}

// OCIImageIndex represents the top-level structure of an OCI Image Index (manifest list).
type OCIImageIndex struct {
	SchemaVersion int             `json:"schemaVersion"`
	MediaType     string          `json:"mediaType"`
	Manifests     []ManifestEntry `json:"manifests"`
}

// ManifestEntry represents a single manifest entry within the index.
type ManifestEntry struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Annotations map[string]string `json:"annotations,omitempty"` // Use omitempty just in case, though usually present
}

func (d *DockerEngine) GetImageNames(fileIo util.FileIO, imagefile string) ([]string, error) {
	names := []string{}
	err := util.ExtractTarSingleFile(fileIo, imagefile, "index.json", filepath.Dir(imagefile))

	if err != nil {
		return names, fmt.Errorf("failed to extract index.json: %w", err)
	}

	indexfile := filepath.Join(filepath.Dir(imagefile), "index.json")
	file, err := os.Open(indexfile)
	if err != nil {
		return names, fmt.Errorf("failed to open file %s: %w", indexfile, err)
	}
	defer util.CloseFileIgnoreError(file)

	var index OCIImageIndex
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&index)

	if err != nil {
		return names, fmt.Errorf("failed to decode file %s: %w", indexfile, err)
	}

	for _, manifest := range index.Manifests {
		fmt.Println(manifest)
		name := manifest.Annotations["io.containerd.image.name"]
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

func (d *DockerEngine) RunCommand(dockerCmd []string) error {
	err := RunCommandAndStreamOutput("docker", dockerCmd...)
	if err != nil {
		return fmt.Errorf("failed to run docker command `docker \"%s\"`: %w", strings.Join(dockerCmd, "\" \""), err)
	}

	return nil
}

func RunCommandAndStreamOutput(name string, args ...string) error {
	cmd := exec.Command(name, args...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to run command: %w", err)
	}
	return nil
}
