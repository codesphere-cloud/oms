package installer

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
)

type K0sManager interface {
	BinaryExists() bool
	Download(force bool, quiet bool) error
	Install(configPath string, force bool) error
}

type K0s struct {
	Env        env.Env
	Http       portal.Http
	FileWriter util.FileIO
	Goos       string
	Goarch     string
}

func NewK0s(hw portal.Http, env env.Env, fw util.FileIO) K0sManager {
	return &K0s{
		Env:        env,
		Http:       hw,
		FileWriter: fw,
		Goos:       runtime.GOOS,
		Goarch:     runtime.GOARCH,
	}
}

func (k *K0s) BinaryExists() bool {
	workdir := k.Env.GetOmsWorkdir()
	k0sPath := filepath.Join(workdir, "k0s")
	return k.FileWriter.Exists(k0sPath)
}

func (k *K0s) Download(force bool, quiet bool) error {
	if k.Goos != "linux" || k.Goarch != "amd64" {
		return fmt.Errorf("codesphere installation is only supported on Linux amd64. Current platform: %s/%s", k.Goos, k.Goarch)
	}

	// Get the latest k0s version
	versionBytes, err := k.Http.Get("https://docs.k0sproject.io/stable.txt")
	if err != nil {
		return fmt.Errorf("failed to fetch version info: %w", err)
	}

	version := strings.TrimSpace(string(versionBytes))
	if version == "" {
		return fmt.Errorf("version info is empty, cannot proceed with download")
	}

	// Check if k0s binary already exists and create destination file
	workdir := k.Env.GetOmsWorkdir()
	k0sPath := filepath.Join(workdir, "k0s")
	if k.BinaryExists() && !force {
		return fmt.Errorf("k0s binary already exists at %s. Use --force to overwrite", k0sPath)
	}

	file, err := k.FileWriter.Create(k0sPath)
	if err != nil {
		return fmt.Errorf("failed to create k0s binary file: %w", err)
	}
	defer util.CloseFileIgnoreError(file)

	// Download using the portal Http wrapper with WriteCounter
	log.Printf("Downloading k0s version %s", version)

	downloadURL := fmt.Sprintf("https://github.com/k0sproject/k0s/releases/download/%s/k0s-%s-%s", version, version, k.Goarch)
	err = k.Http.Download(downloadURL, file, quiet)
	if err != nil {
		return fmt.Errorf("failed to download k0s binary: %w", err)
	}

	// Make the binary executable
	err = os.Chmod(k0sPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to make k0s binary executable: %w", err)
	}

	log.Printf("k0s binary downloaded and made executable at '%s'", k0sPath)

	return nil
}

func (k *K0s) Install(configPath string, force bool) error {
	if k.Goos != "linux" || k.Goarch != "amd64" {
		return fmt.Errorf("codesphere installation is only supported on Linux amd64. Current platform: %s/%s", k.Goos, k.Goarch)
	}

	workdir := k.Env.GetOmsWorkdir()
	k0sPath := filepath.Join(workdir, "k0s")
	if !k.BinaryExists() {
		return fmt.Errorf("k0s binary does not exist in '%s', please download first", k0sPath)
	}

	args := []string{"./k0s", "install", "controller"}
	if configPath != "" {
		args = append(args, "--config", configPath)
	} else {
		args = append(args, "--single")
	}

	if force {
		args = append(args, "--force")
	}

	err := util.RunCommand("sudo", args, workdir)
	if err != nil {
		return fmt.Errorf("failed to install k0s: %w", err)
	}

	log.Println("k0s installed successfully in single-node mode.")
	log.Printf("You can start it using 'sudo %v/k0s start'", workdir)
	log.Printf("You can check the status using 'sudo %v/k0s status'", workdir)

	return nil
}
