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
	UpdateFromStatement(dockerfile string, baseImage string) error
}

type Dockerfile struct {
	fileIO FileIO
}

// NewDockerfileManager creates a new instance of DockerfileManager
func NewDockerfileManager() DockerfileManager {
	return &Dockerfile{
		fileIO: NewFilesystemWriter(),
	}
}

// UpdateFromStatement updates the FROM statement in a Dockerfile with a new base image
func (dm *Dockerfile) UpdateFromStatement(dockerfile string, baseImage string) error {
	file, err := dm.fileIO.Open(dockerfile)
	if err != nil {
		return fmt.Errorf("failed to open dockerfile %s: %w", dockerfile, err)
	}
	defer CloseFileIgnoreError(file)

	content, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("error reading dockerfile: %w", err)
	}

	// Regex to match FROM statements that contain workspace-agent
	fromRegex := regexp.MustCompile(`(?i)(.*FROM\s+).*workspace-agent[^\s]*(.*)`)

	updated := false
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if fromRegex.MatchString(line) {
			lines[i] = fromRegex.ReplaceAllString(line, "${1}"+baseImage+"${2}")
			updated = true
		}
	}

	if !updated {
		return fmt.Errorf("no FROM statement with workspace-agent found in dockerfile")
	}

	err = dm.fileIO.WriteFile(dockerfile, []byte(strings.Join(lines, "\n")), 0644)
	if err != nil {
		return fmt.Errorf("failed to write updated dockerfile: %w", err)
	}

	return nil
}
