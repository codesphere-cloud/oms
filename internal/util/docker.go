package util

import (
	"fmt"
	"io"
	"strings"
)

// DockerfileManager provides functionality to parse and modify Dockerfiles
type DockerfileManager interface {
	UpdateFromStatement(dockerfile io.Reader, baseImage string) (string, error)
}

type Dockerfile struct{}

// NewDockerfileManager creates a new instance of DockerfileManager
func NewDockerfileManager() DockerfileManager {
	return &Dockerfile{}
}

// UpdateFromStatement updates the FROM statement in a Dockerfile with a new base image
func (dm *Dockerfile) UpdateFromStatement(dockerfile io.Reader, baseImage string) (string, error) {
	content, err := io.ReadAll(dockerfile)
	if err != nil {
		return "", fmt.Errorf("error reading dockerfile: %w", err)
	}

	lines := strings.Split(string(content), "\n")

	// Find and update the first FROM line
	updated := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(trimmed), "FROM ") {
			// Preserve original indentation
			indent := ""
			for _, char := range line {
				if char == ' ' || char == '\t' {
					indent += string(char)
				} else {
					break
				}
			}

			// Check for platform flag
			platformFlag := ""
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 && strings.HasPrefix(parts[1], "--platform=") {
				platformFlag = parts[1] + " "
			}

			// Update the line
			lines[i] = fmt.Sprintf("%sFROM %s%s", indent, platformFlag, baseImage)
			updated = true
			break
		}
	}

	if !updated {
		return "", fmt.Errorf("no FROM statement found in dockerfile")
	}

	// Join lines back together
	return strings.Join(lines, "\n"), nil
}
