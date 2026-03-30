// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// installerComponentSteps lists the install-components.js steps executed
// locally (in order) instead of running the full private-cloud-installer.
var installerComponentSteps = []string{"setUpCluster", "codesphere"}

// installerArtifactFilename is the artifact to download from the OMS portal.
const installerArtifactFilename = "installer-lite.tar.gz"

const temporaryPostgresNodePortPrefix = "oms-masterdata-nodeport"

// DownloadInstallerPackage downloads the Codesphere installer package from the
// OMS portal, similar to how the GCP bootstrapper fetches it onto a jumpbox.
// The package is downloaded into the directory that contains the config/secrets
// files and its on-disk filename is returned.
func (b *LocalBootstrapper) DownloadInstallerPackage() (string, error) {
	version := b.Env.InstallVersion
	hash := b.Env.InstallHash

	if version == "" {
		return "", fmt.Errorf("install version is required to download from the portal")
	}
	if hash == "" {
		return "", fmt.Errorf("install hash must be set when install version is set")
	}

	log.Printf("Downloading Codesphere package %s (hash %s) from the OMS portal...", version, hash)

	p := portal.NewPortalClient()

	build, err := p.GetBuild(portal.CodesphereProduct, version, hash)
	if err != nil {
		return "", fmt.Errorf("failed to get build from portal: %w", err)
	}
	fullFilename := build.BuildPackageFilename(installerArtifactFilename)
	destPath := filepath.Join(b.Env.InstallDir, fullFilename)

	if b.fw.Exists(destPath) {
		log.Printf("Installer package already exists at %s, skipping download", destPath)
		return destPath, nil
	}

	download, err := build.GetBuildForDownload(installerArtifactFilename)
	if err != nil {
		return "", fmt.Errorf("artifact %q not found in build: %w", installerArtifactFilename, err)
	}

	// Support resuming a partial download.
	out, err := b.fw.OpenAppend(destPath)
	if err != nil {
		out, err = b.fw.Create(destPath)
		if err != nil {
			return "", fmt.Errorf("failed to create file %s: %w", destPath, err)
		}
	}
	defer util.CloseFileIgnoreError(out)

	fileSize := 0
	fileInfo, err := out.Stat()
	if err == nil {
		fileSize = int(fileInfo.Size())
	}

	err = p.DownloadBuildArtifact(portal.CodesphereProduct, download, out, fileSize, false)
	if err != nil {
		return "", fmt.Errorf("failed to download build artifact: %w", err)
	}

	// Verify integrity.
	verifyFile, err := b.fw.Open(destPath)
	if err != nil {
		return "", fmt.Errorf("failed to open downloaded file for verification: %w", err)
	}
	defer util.CloseFileIgnoreError(verifyFile)

	if err := p.VerifyBuildArtifactDownload(verifyFile, download); err != nil {
		return "", fmt.Errorf("artifact verification failed: %w", err)
	}

	return destPath, nil
}

// PrepareInstallerBundle resolves the installer package to a directory.
// It handles three cases:
//  1. Portal download: InstallVersion+InstallHash are set → download tar.gz, then extract.
//  2. Local tar.gz/tgz: InstallLocal points to an archive → extract.
//  3. Local directory: InstallLocal points to an already-unpacked directory → use as-is.
func (b *LocalBootstrapper) PrepareInstallerBundle() (string, error) {
	var bundlePath string

	switch {
	case b.Env.InstallVersion != "":
		// Download from portal.
		downloaded, err := b.DownloadInstallerPackage()
		if err != nil {
			return "", err
		}
		bundlePath = downloaded

	case b.Env.InstallLocal != "":
		bundlePath = b.Env.InstallLocal

	default:
		return "", fmt.Errorf("either --install-version or --install-local must be specified")
	}

	info, err := os.Stat(bundlePath)
	if err != nil {
		return "", fmt.Errorf("cannot access installer bundle %q: %w", bundlePath, err)
	}

	// Already an unpacked directory – use directly.
	if info.IsDir() {
		log.Printf("Installer bundle is a directory, using as-is: %s", bundlePath)
		return bundlePath, nil
	}

	// Treat as tar.gz archive – extract alongside the archive.
	if !strings.HasSuffix(bundlePath, ".tar.gz") && !strings.HasSuffix(bundlePath, ".tgz") {
		return "", fmt.Errorf("installer bundle %q is neither a directory nor a .tar.gz/.tgz archive", bundlePath)
	}

	destDir := strings.TrimSuffix(strings.TrimSuffix(bundlePath, ".gz"), ".tar")
	destDir = strings.TrimSuffix(destDir, ".tgz")
	if destDir == bundlePath {
		destDir = bundlePath + "-unpacked"
	}

	if b.fw.Exists(destDir) {
		log.Printf("Installer bundle is already extracted. Skipping extraction...\n")
		return destDir, nil
	}

	log.Printf("Extracting installer bundle %s → %s", bundlePath, destDir)
	if err := util.ExtractTarGz(b.fw, bundlePath, destDir); err != nil {
		return "", fmt.Errorf("failed to extract installer bundle: %w", err)
	}

	return destDir, nil
}

// symlinkLocalBinaries replaces bundled node, helm and kubectl binaries with
// symlinks to the locally installed versions. This is only done on non-Linux
// hosts because the bundled binaries are Linux x86_64 binaries that cannot run
// on macOS or other platforms.
func symlinkLocalBinaries(bundleDir string) error {
	if runtime.GOOS == "linux" {
		return nil
	}

	for _, name := range []string{"node", "helm", "kubectl"} {
		target := filepath.Join(bundleDir, name)
		if err := symlinkBinary(name, target); err != nil {
			return err
		}
	}

	return nil
}

// symlinkDepsBinaries replaces bundled dependency binaries inside the
// extracted deps directory with symlinks to locally installed versions.
// This covers tools like sops and age which live under <depsDir>/sops/files/.
func symlinkDepsBinaries(depsDir string) error {
	if runtime.GOOS == "linux" {
		return nil
	}

	// sops and age are resolved by install-components.js via
	// resolveFileDependency(dependenciesDir, "sops", "<binary>")
	// which maps to <depsDir>/sops/files/<binary>.
	sopsFilesDir := filepath.Join(depsDir, "sops", "files")
	for _, name := range []string{"sops", "age", "age-keygen"} {
		target := filepath.Join(sopsFilesDir, name)
		if err := symlinkBinary(name, target); err != nil {
			return err
		}
	}

	// sops and age are resolved by install-components.js via
	// resolveFileDependency(dependenciesDir, "installer", "<binary>")
	// which maps to <depsDir>/installer/files/<binary>.
	installerFilesDir := filepath.Join(depsDir, "installer", "files")
	for _, name := range []string{"kubectl", "helm", "node"} {
		target := filepath.Join(installerFilesDir, name)
		if err := symlinkBinary(name, target); err != nil {
			return err
		}
	}

	return nil
}

// symlinkBinary creates a symlink at target pointing to the locally installed
// binary identified by name (looked up via $PATH).
func symlinkBinary(name, target string) error {
	localPath, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("cannot find %q on the host system: %w", name, err)
	}

	// Resolve to an absolute path so the symlink is stable.
	localPath, err = filepath.Abs(localPath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path for %q: %w", name, err)
	}

	// Remove the bundled binary (or an existing symlink) if present.
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove bundled %q: %w", name, err)
	}

	if err := os.Symlink(localPath, target); err != nil {
		return fmt.Errorf("failed to symlink %q → %q: %w", target, localPath, err)
	}

	log.Printf("Symlinked %s → %s", target, localPath)
	return nil
}

func (b *LocalBootstrapper) createTemporaryPostgresNodePortEndpoint() (string, int32, func(), error) {
	masterdataSvc := &corev1.Service{}
	masterdataSvcKey := types.NamespacedName{Name: "masterdata-rw", Namespace: codesphereNamespace}
	if err := b.kubeClient.Get(b.ctx, masterdataSvcKey, masterdataSvc); err != nil {
		return "", 0, nil, fmt.Errorf("failed to get PostgreSQL service %s/%s: %w", codesphereNamespace, "masterdata-rw", err)
	}

	if len(masterdataSvc.Spec.Selector) == 0 {
		return "", 0, nil, fmt.Errorf("service %s/%s has no selector; cannot create NodePort proxy", codesphereNamespace, "masterdata-rw")
	}

	postgresPort, err := getPostgresServicePort(masterdataSvc)
	if err != nil {
		return "", 0, nil, err
	}

	nowNanos := time.Now().UnixNano()
	tmpServiceName := fmt.Sprintf("%s-%d", temporaryPostgresNodePortPrefix, nowNanos)

	selector := make(map[string]string, len(masterdataSvc.Spec.Selector))
	for k, v := range masterdataSvc.Spec.Selector {
		selector[k] = v
	}

	tmpService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tmpServiceName,
			Namespace: codesphereNamespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: selector,
			Ports: []corev1.ServicePort{{
				Name:       "postgres",
				Protocol:   corev1.ProtocolTCP,
				Port:       postgresPort.Port,
				TargetPort: postgresPort.TargetPort,
			}},
		},
	}

	if err := b.kubeClient.Create(b.ctx, tmpService); err != nil {
		return "", 0, nil, fmt.Errorf("failed to create temporary PostgreSQL NodePort service %s/%s: %w", codesphereNamespace, tmpServiceName, err)
	}

	cleanup := func() {
		if err := b.kubeClient.Delete(b.ctx, tmpService); err != nil && !apierrors.IsNotFound(err) {
			log.Printf("Warning: failed to delete temporary PostgreSQL NodePort service %s/%s: %v", codesphereNamespace, tmpServiceName, err)
		}
	}

	if len(tmpService.Spec.Ports) == 0 || tmpService.Spec.Ports[0].NodePort == 0 {
		if err := b.kubeClient.Get(b.ctx, types.NamespacedName{Name: tmpServiceName, Namespace: codesphereNamespace}, tmpService); err != nil {
			cleanup()
			return "", 0, nil, fmt.Errorf("failed to read temporary PostgreSQL NodePort service %s/%s: %w", codesphereNamespace, tmpServiceName, err)
		}
	}

	if len(tmpService.Spec.Ports) == 0 || tmpService.Spec.Ports[0].NodePort == 0 {
		cleanup()
		return "", 0, nil, fmt.Errorf("temporary PostgreSQL NodePort service %s/%s has no allocated nodePort", codesphereNamespace, tmpServiceName)
	}

	nodeIP, err := b.resolveNodeIPForNodePort()
	if err != nil {
		cleanup()
		return "", 0, nil, err
	}

	return nodeIP, tmpService.Spec.Ports[0].NodePort, cleanup, nil
}

func getPostgresServicePort(svc *corev1.Service) (corev1.ServicePort, error) {
	for _, port := range svc.Spec.Ports {
		if port.Port == 5432 {
			if port.TargetPort.Type == intstr.Int && port.TargetPort.IntValue() == 0 {
				port.TargetPort = intstr.FromInt(5432)
			}
			if port.TargetPort.Type == intstr.String && port.TargetPort.String() == "" {
				port.TargetPort = intstr.FromInt(5432)
			}
			return port, nil
		}
	}

	if len(svc.Spec.Ports) == 0 {
		return corev1.ServicePort{}, fmt.Errorf("service %s/%s has no ports", svc.Namespace, svc.Name)
	}

	port := svc.Spec.Ports[0]
	if port.TargetPort.Type == intstr.Int && port.TargetPort.IntValue() == 0 {
		port.TargetPort = intstr.FromInt(int(port.Port))
	}
	if port.TargetPort.Type == intstr.String && port.TargetPort.String() == "" {
		port.TargetPort = intstr.FromInt(int(port.Port))
	}

	return port, nil
}

func (b *LocalBootstrapper) resolveNodeIPForNodePort() (string, error) {
	nodeList := &corev1.NodeList{}
	if err := b.kubeClient.List(b.ctx, nodeList); err != nil {
		return "", fmt.Errorf("failed to list cluster nodes for NodePort endpoint: %w", err)
	}

	if len(nodeList.Items) == 0 {
		return "", fmt.Errorf("connected to Kubernetes cluster but no nodes are available")
	}

	for _, node := range nodeList.Items {
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP && addr.Address != "" {
				return addr.Address, nil
			}
		}
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeExternalIP && addr.Address != "" {
				return addr.Address, nil
			}
		}
	}

	return "", fmt.Errorf("failed to resolve node IP address for NodePort endpoint")
}

func (b *LocalBootstrapper) configurePostgresForMigration(host string, port int32) (func() error, error) {
	previousMigration := b.Env.InstallConfig.Codesphere.Migration
	b.Env.InstallConfig.Codesphere.Migration = &files.MigrationConfig{
		Postgres: &files.MigrationPostgresConfig{
			Host:     host,
			Port:     int(port),
			Database: b.Env.InstallConfig.Postgres.Database,
			AltName:  b.Env.InstallConfig.Postgres.ServerAddress,
		},
	}

	if err := b.icg.WriteInstallConfig(b.Env.InstallConfigPath, true); err != nil {
		b.Env.InstallConfig.Codesphere.Migration = previousMigration
		return nil, fmt.Errorf("failed to write migration config to install config: %w", err)
	}

	return func() error {
		b.Env.InstallConfig.Codesphere.Migration = previousMigration
		if err := b.icg.WriteInstallConfig(b.Env.InstallConfigPath, true); err != nil {
			return fmt.Errorf("failed to restore install config after installer run: %w", err)
		}
		return nil
	}, nil
}

// RunInstaller extracts the deps.tar.gz archive locally and then runs the
// install-components.js script directly on the local machine for each
// required component step (setUpCluster, codesphere), instead of running
// the private-cloud-installer.js which orchestrates remote nodes via SSH.
func (b *LocalBootstrapper) RunInstaller() (err error) {
	if b.Env.InstallVersion == "" && b.Env.InstallLocal == "" {
		log.Println("No installer package specified, skipping Codesphere installation.")
		return nil
	}

	bundleDir, err := b.PrepareInstallerBundle()
	if err != nil {
		return fmt.Errorf("failed to prepare installer bundle: %w", err)
	}

	// On non-Linux hosts the bundled binaries are Linux ELF executables that
	// cannot run natively. Replace them with symlinks to the host's versions.
	if err := symlinkLocalBinaries(bundleDir); err != nil {
		return fmt.Errorf("failed to symlink local binaries: %w", err)
	}

	// Extract deps.tar.gz locally so that install-components.js can find
	// all dependency binaries (helm charts, sops, etc.) on the local machine.
	archivePath := filepath.Join(bundleDir, "deps.tar.gz")
	depsDir := filepath.Join(bundleDir, "deps")

	if b.fw.Exists(depsDir) {
		log.Printf("deps directory already exists at %s, skipping extraction", depsDir)
	} else {
		log.Printf("Extracting deps.tar.gz → %s", depsDir)
		if err := util.ExtractTarGz(b.fw, archivePath, depsDir); err != nil {
			return fmt.Errorf("failed to extract deps.tar.gz: %w", err)
		}
	}

	// Symlink sops and age inside the extracted deps directory so that
	// install-components.js uses the locally installed versions.
	if err := symlinkDepsBinaries(depsDir); err != nil {
		return fmt.Errorf("failed to symlink deps binaries: %w", err)
	}

	nodePath := filepath.Join(bundleDir, "node")
	installerPath := filepath.Join(bundleDir, "install-components.js")

	// Resolve absolute paths so install-components.js finds them
	// regardless of its working directory.
	absDepsDir, err := filepath.Abs(depsDir)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute deps dir: %w", err)
	}

	configPath, err := filepath.Abs(b.Env.InstallConfigPath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute config path: %w", err)
	}

	privKeyPath := b.ageKeyPath
	if privKeyPath == "" {
		return fmt.Errorf("age key path is not set; cannot pass private key to installer")
	}
	privKeyPath, err = filepath.Abs(privKeyPath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute key path: %w", err)
	}

	// Create a temporary NodePort service for PostgreSQL so that
	// install-components.js can reach the database without a long-lived
	// kubectl port-forward session.
	dbHost, dbPort, cleanupNodePortSvc, err := b.createTemporaryPostgresNodePortEndpoint()
	if err != nil {
		return err
	}
	defer cleanupNodePortSvc()

	log.Printf("Temporary PostgreSQL NodePort service ready (%s:%d)", dbHost, dbPort)

	restoreMigrationConfig, err := b.configurePostgresForMigration(dbHost, dbPort)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, restoreMigrationConfig())
	}()

	// Run each component step locally via install-components.js.
	for _, component := range installerComponentSteps {
		cmdArgs := []string{
			installerPath,
			"--component", component,
			"--configDir", filepath.Join(b.Env.InstallDir, "config"),
			"--dependenciesDir", absDepsDir,
			"--config", configPath,
			"--privKey", privKeyPath,
		}

		log.Printf("Running install-components.js --component %s", component)
		log.Printf("  %s %s", nodePath, strings.Join(cmdArgs, " "))
		cmd := exec.Command(nodePath, cmdArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("install-components.js --component %s failed: %w", component, err)
		}

		log.Printf("Component %s installed successfully.", component)
	}

	log.Println("Codesphere installer finished successfully.")
	return nil
}
