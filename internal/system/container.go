package system

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
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
