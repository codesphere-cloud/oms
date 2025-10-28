package files

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/util"
)

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

func (o *OCIImageIndex) ParseOCIImageConfig(filePath string) error {
	indexfile := filepath.Join(filepath.Dir(filePath), "index.json")
	file, err := os.Open(indexfile)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", indexfile, err)
	}
	defer util.CloseFileIgnoreError(file)

	decoder := json.NewDecoder(file)
	err = decoder.Decode(o)
	if err != nil {
		return fmt.Errorf("failed to decode file %s: %w", indexfile, err)
	}

	return nil
}

// ExtractImageNames extracts the image names from the OCI image index file.
func (o *OCIImageIndex) ExtractImageNames() ([]string, error) {
	var names []string
	for _, manifest := range o.Manifests {
		name := manifest.Annotations["io.containerd.image.name"]
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}
