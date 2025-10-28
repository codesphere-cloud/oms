// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

type Image struct {
	ctx context.Context
}

type ImageManager interface {
	LoadImage(imageTarPath string) error
	BuildImage(dockerfile string, tag string, buildContext string) error
	PushImage(tag string) error
}

func NewImage(ctx context.Context) *Image {
	return &Image{
		ctx: ctx,
	}
}

func isCommandAvailable(name string) bool {
	cmd := exec.Command("command", "-v", name)
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func (c *Image) LoadImage(imageTarPath string) error {
	var cmd *exec.Cmd
	if isCommandAvailable("docker") {
		cmd = exec.CommandContext(c.ctx, "docker", "load", "-i", imageTarPath)
	} else if isCommandAvailable("podman") {
		cmd = exec.CommandContext(c.ctx, "podman", "load", "-i", imageTarPath)
	} else {
		return fmt.Errorf("neither 'docker' nor 'podman' command is available")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("load failed with exit status %w", err)
	}

	return nil
}

func (c *Image) BuildImage(dockerfile string, tag string, buildContext string) error {
	var cmd *exec.Cmd
	if isCommandAvailable("docker") {
		cmd = exec.CommandContext(c.ctx, "docker", "build", "-f", dockerfile, "-t", tag, ".")
	} else if isCommandAvailable("podman") {
		cmd = exec.CommandContext(c.ctx, "podman", "build", "-f", dockerfile, "-t", tag, ".")
	} else {
		return fmt.Errorf("neither 'docker' nor 'podman' command is available")
	}

	cmd.Dir = buildContext
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("build failed with exit status %w", err)
	}

	return nil
}

func (c *Image) PushImage(tag string) error {
	var cmd *exec.Cmd
	if isCommandAvailable("docker") {
		cmd = exec.CommandContext(c.ctx, "docker", "push", tag)
	} else if isCommandAvailable("podman") {
		cmd = exec.CommandContext(c.ctx, "podman", "push", tag)
	} else {
		return fmt.Errorf("neither 'docker' nor 'podman' command is available")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("push failed with exit status %w", err)
	}

	return nil
}
