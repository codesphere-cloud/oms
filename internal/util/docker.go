// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

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

	// Regex to match FROM statements that contain workspace-agent
	fromRegex := regexp.MustCompile(`(?i)(.*FROM\s+).*workspace-agent[^\s]*(.*)`)

	lines := strings.Split(string(content), "\n")
	lastMatchIndex := -1

	for i, line := range lines {
		if fromRegex.MatchString(line) {
			lastMatchIndex = i
		}
	}
	if lastMatchIndex == -1 {
		return "", fmt.Errorf("no FROM statement with workspace-agent found in dockerfile")
	}

	newLine := fromRegex.ReplaceAllString(lines[lastMatchIndex], "${1}"+baseImage+"${2}")
	lines[lastMatchIndex] = strings.TrimRight(newLine, " \t")

	return strings.Join(lines, "\n"), nil
}
