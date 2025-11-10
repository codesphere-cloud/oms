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
	BuildAndPushImage(dockerfile string, tag string, buildContext string) error
}

func NewImage(ctx context.Context) *Image {
	return &Image{
		ctx: ctx,
	}
}

func isCommandAvailable(name string) bool {
	cmd := exec.Command(name, "-v")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func (i *Image) LoadImage(imageTarPath string) error {
	err := i.runCommand("", "load", "-i", imageTarPath)
	if err != nil {
		return fmt.Errorf("load failed: %w", err)
	}
	return nil
}

func (i *Image) BuildImage(dockerfile string, tag string, buildContext string) error {
	err := i.runCommand(buildContext, "build", "-f", dockerfile, "-t", tag, ".")
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}
	return nil
}

func (i *Image) PushImage(tag string) error {
	err := i.runCommand("", "push", tag)
	if err != nil {
		return fmt.Errorf("push failed: %w", err)
	}
	return nil
}

func (i *Image) BuildAndPushImage(dockerfile string, tag string, buildContext string) error {
	err := i.BuildImage(dockerfile, tag, buildContext)
	if err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}

	err = i.PushImage(tag)
	if err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}

	return nil
}

func (i *Image) runCommand(cmdDir string, args ...string) error {
	var cmd *exec.Cmd
	if isCommandAvailable("docker") {
		cmd = exec.CommandContext(i.ctx, "docker", args...)
	} else if isCommandAvailable("podman") {
		cmd = exec.CommandContext(i.ctx, "podman", args...)
	} else {
		return fmt.Errorf("neither 'docker' nor 'podman' command is available")
	}

	if cmdDir != "" {
		cmd.Dir = cmdDir
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("command failed with exit status %w", err)
	}

	return nil
}
