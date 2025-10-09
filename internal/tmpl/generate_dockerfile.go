package tmpl

import (
	_ "embed"
	"fmt"
	"text/template"

	"github.com/codesphere-cloud/oms/internal/util"
)

//go:embed Dockerfile.agent
var dockerfileTemplate string

// TemplateData holds the dynamic values to be injected into the Dockerfile template.
type TemplateData struct {
	BaseImage string
}

// GenerateDockerfile creates a Dockerfile at the given path, extending from the baseImage,
// and injecting custom commands for bootstrapping a new baseimage.
func GenerateDockerfile(fileIo util.FileIO, outputPath, baseImage string) error {
	data := TemplateData{
		BaseImage: baseImage,
	}

	tmpl, err := template.New("dockerfile").Parse(dockerfileTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse dockerfile template: %w", err)
	}

	outFile, err := fileIo.Create(outputPath)
	if err != nil {
		return fmt.Errorf("error creating output file %s: %w", outputPath, err)
	}
	defer util.CloseFileIgnoreError(outFile)

	err = tmpl.Execute(outFile, data)
	if err != nil {
		return fmt.Errorf("error executing template and writing to file: %w", err)
	}

	return nil
}
