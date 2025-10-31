package util

import (
	"fmt"
	"io"
	"regexp"
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

	// Regex to match FROM statements and capture parts separately
	// Group 1: whitespace + FROM + whitespace
	// Group 2: image name until AS or end of line (also replaces --platform if present)
	// Group 3: AS + alias (optional)
	fromRegex := regexp.MustCompile(`(?i)^(\s*FROM\s+)\S+(\s+AS\s+\S+)?(.*)$`)

	updated := false
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		newLine := fromRegex.ReplaceAllString(line, fmt.Sprintf("${1}%s${2}", baseImage))
		if newLine != line {
			lines[i] = newLine
			updated = true
			break
		}
	}

	if !updated {
		return "", fmt.Errorf("no FROM statement found in dockerfile")
	}

	return strings.Join(lines, "\n"), nil
}
