// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"errors"
	"fmt"
	"strings"

	"github.com/codesphere-cloud/oms/internal/bootstrap/datacenter"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"golang.org/x/crypto/ssh"
)

// EnsureK0s executed all steps to ensure a k0s cluster in gcp for every data center.
// Only executing the config script needs to be done after installing codesphere, as crucial parts are still in the ts-installer.
// k0s is installed only into data centers where it is not configured yet. It is downloaded to the
// jumpbox before installing, airgapped bootstraps also download the airgap bundle and generate an
// airgap k0sctl config first.
// Returns an error if k0s could not be ensured.
func (b *GCPBootstrapper) EnsureK0s() error {
	err := b.GenerateK0sConfigScript()
	if err != nil {
		return fmt.Errorf("failed to generate k0s config script: %w", err)
	}

	unconfigured, err := b.dataCentersWithoutConfiguredK0s()
	if err != nil {
		return err
	}

	if len(unconfigured) > 0 {
		err = b.stlog.Step("Download k0s", b.DownloadK0s)
		if err != nil {
			return fmt.Errorf("failed to download k0s: %w", err)
		}
	}

	if b.Env.Airgapped {
		err = b.GenerateK0sAirgapConfig(unconfigured)
		if err != nil {
			return err
		}
	}

	err = b.InstallK0s(unconfigured)
	if err != nil {
		return fmt.Errorf("failed to install k0s: %w", err)
	}

	err = b.WaitForK0sNodes()
	if err != nil {
		return fmt.Errorf("failed waiting for k0s nodes to get ready: %w", err)
	}

	return nil
}

// dataCentersWithoutConfiguredK0s returns the data centers k0s has to be installed into. A data
// center whose k0s is already configured is left out: k0sctl would take the cloud provider flag
// the config script adds for changed install flags, reinstall the k0s service and restart it.
func (b *GCPBootstrapper) dataCentersWithoutConfiguredK0s() ([]*datacenter.DataCenter, error) {
	var unconfigured []*datacenter.DataCenter

	for _, dc := range b.Env.DataCenters {
		var reinstallReason string

		err := b.stlog.Step(dc.StepName("Check running k0s"), func() error {
			var err error

			reinstallReason, err = b.k0sReinstallReason(dc)
			if err != nil {
				return err
			}

			if reinstallReason == "" {
				b.stlog.Logf("k0s %s already runs with the cloud provider enabled, leaving it running", b.Env.K0sVersion)
			} else {
				b.stlog.Logf("Installing k0s %s: %s", b.Env.K0sVersion, reinstallReason)
			}

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to check the running k0s (data center %d): %w", dc.ID, err)
		}

		if reinstallReason != "" {
			unconfigured = append(unconfigured, dc)
		}
	}

	return unconfigured, nil
}

// k0sReinstallReason returns why k0s has to be installed into the data center, or "" when its
// vault holds the kubeconfig and every k0s node runs the requested k0s version with the cloud
// provider enabled, which the k0s config script does last. Only "oms install k0s" adds the
// kubeconfig the Codesphere install reads, and a vault kept locally from the first bootstrap lacks
// it but still replaces the jumpbox's copy. Workers are checked too, since recovering a project
// recreates a missing VM without k0s. A node failing the check needs k0s; a node that cannot be
// checked at all is an error, so an unreachable node never leads to reinstalling k0s.
func (b *GCPBootstrapper) k0sReinstallReason(dc *datacenter.DataCenter) (string, error) {
	vault := dc.ConfigManager.GetVault()
	if vault == nil || vault.GetSecret(files.SecretKubeConfig) == nil {
		return "the vault has no kubeconfig", nil
	}

	for i, n := range dc.ControlPlaneNodes {
		service := k0sServiceName(i)
		err := n.RunSSHCommand("root", k0sConfiguredCheckCmd(service, b.Env.K0sVersion))

		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Sprintf("%s on %s is not active, runs another k0s version or lacks the cloud provider flag", service, n.Name), nil
		}

		if err != nil {
			return "", fmt.Errorf("failed to check k0s on %s: %w", n.Name, err)
		}
	}

	return "", nil
}

// k0sServiceName returns the k0s systemd service of a data center's i-th control plane node: the
// first one runs the controller with an enabled worker, the others are dedicated workers.
func k0sServiceName(i int) string {
	if i == 0 {
		return "k0scontroller"
	}

	return "k0sworker"
}

// k0sConfiguredCheckCmd returns a shell command that succeeds only on a node whose k0s service is
// active, runs the given version and was started with the cloud provider flag the k0s config
// script adds.
func k0sConfiguredCheckCmd(service, version string) string {
	return fmt.Sprintf("systemctl is-active --quiet %s && [ \"$(k0s version)\" = %s ] && %s",
		service, shellQuote(version), cloudProviderRunningCheck(service))
}

func k0sUnitPath(service string) string {
	return "/etc/systemd/system/" + service + ".service"
}

// cloudProviderRunningCheck returns a shell command that succeeds when the running k0s service was
// started with --enable-cloud-provider. It reads the process, not the unit, so a unit that was
// patched but never reloaded or restarted does not count.
func cloudProviderRunningCheck(service string) string {
	return fmt.Sprintf("tr '\\0' ' ' < /proc/$(systemctl show --property MainPID --value %s)/cmdline"+
		" | grep -q -- ' --enable-cloud-provider'", service)
}

// cloudProviderInUnitCheck returns a shell command that succeeds when the k0s unit starts k0s
// with --enable-cloud-provider.
func cloudProviderInUnitCheck(unit string) string {
	return "grep -q '^ExecStart=.*--enable-cloud-provider' " + unit
}

// GenerateK0sConfigScript writes and uploads the k0s cloud-provider configuration script of
// every data center to that data center's first control plane node.
// Returns an error if a script can't be generated, written, copied.
func (b *GCPBootstrapper) GenerateK0sConfigScript() error {
	if err := b.ensureDataCenters(); err != nil {
		return err
	}

	for _, dc := range b.Env.DataCenters {
		if err := b.generateK0sConfigScript(dc); err != nil {
			return err
		}
	}

	return nil
}

func (b *GCPBootstrapper) generateK0sConfigScript(dc *datacenter.DataCenter) error {
	var enableWorkerDaemonsCmds strings.Builder

	// The first control plane node runs k0scontroller with an enabled worker, the others are
	// dedicated workers running k0sworker. ssh joins its arguments and the remote shell parses
	// them again, so the whole remote command is passed as a single quoted word.
	for i := 1; i < len(dc.ControlPlaneNodes); i++ {
		internalIP := dc.ControlPlaneNodes[i].GetInternalIP()
		fmt.Fprintf(&enableWorkerDaemonsCmds, "ssh -o StrictHostKeyChecking=no root@%s %s\n",
			internalIP, shellQuote(enableCloudProviderCmd("k0sworker")))
	}

	script := fmt.Sprintf(`#!/bin/bash

set -eo pipefail

cat <<EOF > cloud.conf
[Global]
project-id = "%s"
EOF

cat <<EOF > cc-deployment.yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: cloud-controller-manager
  namespace: kube-system
  labels:
    component: cloud-controller-manager
spec:
  selector:
    matchLabels:
      component: cloud-controller-manager
  template:
    metadata:
      labels:
        component: cloud-controller-manager
    spec:
      serviceAccountName: cloud-controller-manager
      containers:
      - name: cloud-controller-manager
        image: k8scloudprovidergcp/cloud-controller-manager:latest
        command:
        - /usr/local/bin/cloud-controller-manager
        args:
        - --v=5
        - --cloud-provider=gce
        - --cloud-config=/etc/gce/cloud.conf
        - --leader-elect-resource-name=k0s-gcp-ccm
        - --use-service-account-credentials=true
        - --controllers=cloud-node,cloud-node-lifecycle,service
        - --allocate-node-cidrs=false
        - --configure-cloud-routes=false
        volumeMounts:
        - name: cloud-config-volume
          mountPath: /etc/gce
          readOnly: true
      volumes:
      - name: cloud-config-volume
        configMap:
          name: cloud-config
      tolerations:
      - key: node.cloudprovider.kubernetes.io/uninitialized
        value: "true"
        effect: NoSchedule
      - key: node-role.kubernetes.io/master
        effect: NoSchedule
      - key: node-role.kubernetes.io/control-plane
        effect: NoSchedule
EOF

KUBECTL="k0s kubectl"
$KUBECTL create configmap cloud-config --from-file=cloud.conf -n kube-system --dry-run=client -o yaml | $KUBECTL apply -f -
echo alias kubectl=\"$KUBECTL\" >> /root/.bashrc
echo alias k=\"$KUBECTL\" >> /root/.bashrc

$KUBECTL apply -f https://raw.githubusercontent.com/kubernetes/cloud-provider-gcp/refs/tags/providers/v0.28.2/deploy/packages/default/manifest.yaml

$KUBECTL apply -f cc-deployment.yaml

# set loadBalancerIP for public-gateway-controller and gateway-controller
$KUBECTL patch svc public-gateway-controller -n codesphere -p '{"spec": {"loadBalancerIP": "'%s'"}}'
$KUBECTL patch svc gateway-controller -n codesphere -p '{"spec": {"loadBalancerIP": "'%s'"}}'

%s
%s
`, b.Env.ProjectID, dc.PublicGatewayIP, dc.GatewayIP, enableWorkerDaemonsCmds.String(), enableCloudProviderCmd("k0scontroller"))

	localScript := dc.K0sConfigScriptPath()

	err := b.fw.WriteFile(localScript, []byte(script), 0755)
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", localScript, err)
	}

	controller := dc.ControlPlaneNodes[0]

	err = controller.NodeClient.CopyFile(controller, localScript, remoteK0sConfigScriptPath)
	if err != nil {
		return fmt.Errorf("failed to copy %s to control plane node: %w", localScript, err)
	}

	err = controller.RunSSHCommand("root", "chmod +x "+remoteK0sConfigScriptPath)
	if err != nil {
		return fmt.Errorf("failed to make %s executable: %w", localScript, err)
	}

	return nil
}

// enableCloudProviderCmd returns a shell command that adds --enable-cloud-provider to the k0s
// systemd service's ExecStart line and restarts the service. The unit starts k0s as
// "k0s worker ..." or "k0s controller ...", so the flag is appended to the end of that line.
// A service already running with the flag is neither changed nor restarted, so rerunning the
// script keeps a configured k0s running, while a unit patched by an interrupted run is still
// reloaded and restarted. The command fails if the flag is not in the unit afterwards.
func enableCloudProviderCmd(service string) string {
	unit := k0sUnitPath(service)

	return fmt.Sprintf("%s || { sed -i '/^ExecStart=/{/--enable-cloud-provider/!s/$/ --enable-cloud-provider/}' %s"+
		" && %s && systemctl daemon-reload && systemctl restart %s; }",
		cloudProviderRunningCheck(service), unit, cloudProviderInUnitCheck(unit), service)
}

// RunK0sConfigScript runs every data center's k0s configuration script on its first control
// plane node. It requires that data center's Codesphere install to have completed, since the
// script patches the gateway services the install creates.
func (b *GCPBootstrapper) RunK0sConfigScript() error {
	if err := b.ensureDataCenters(); err != nil {
		return err
	}

	for _, dc := range b.Env.DataCenters {
		err := b.stlog.Step(dc.StepName("Run k0s config script"), func() error {
			return b.runK0sConfigScript(dc)
		})
		if err != nil {
			return fmt.Errorf("failed to run k0s config script (data center %d): %w", dc.ID, err)
		}
	}

	return nil
}

func (b *GCPBootstrapper) runK0sConfigScript(dc *datacenter.DataCenter) error {
	err := dc.ControlPlaneNodes[0].RunSSHCommand("root", remoteK0sConfigScriptPath)
	if err != nil {
		return fmt.Errorf("failed to configure k0s in data center %d: %w", dc.ID, err)
	}

	return nil
}

// DownloadK0s caches the k0s binary on the jumpbox, together with its airgap image bundle for
// airgapped bootstraps. All data centers install the same k0s version, so they share the cache.
func (b *GCPBootstrapper) DownloadK0s() error {
	downloadCmd := fmt.Sprintf("oms download k0s --version %s", b.Env.K0sVersion)
	if b.Env.Airgapped {
		downloadCmd += " --airgapped"
	}

	if err := b.Env.Jumpbox.RunSSHCommand("root", downloadCmd); err != nil {
		return fmt.Errorf("failed to download k0s on jumpbox: %w", err)
	}

	return nil
}

// GenerateK0sAirgapConfig generates the given data centers' airgap k0sctl configs on the jumpbox
// from the cached airgap bundle, without installing them yet.
func (b *GCPBootstrapper) GenerateK0sAirgapConfig(dataCenters []*datacenter.DataCenter) error {
	for _, dc := range dataCenters {
		err := b.stlog.Step(dc.StepName("Generate k0s airgap config"), func() error {
			return b.generateK0sAirgapConfig(dc)
		})
		if err != nil {
			return fmt.Errorf("failed to generate k0s airgap config (data center %d): %w", dc.ID, err)
		}
	}

	return nil
}

func (b *GCPBootstrapper) generateK0sAirgapConfig(dc *datacenter.DataCenter) error {
	generateCmd := fmt.Sprintf("oms init k0s-config --version %s --install-config %s --airgapped --output %s --yes",
		b.Env.K0sVersion, dc.RemoteConfigPath, dc.RemoteK0sctlConfigPath())
	if err := b.Env.Jumpbox.RunSSHCommand("root", generateCmd); err != nil {
		return fmt.Errorf("failed to generate k0s airgap config on jumpbox (data center %d): %w", dc.ID, err)
	}

	return nil
}

// InstallK0s deploys k0s into the given data centers with the native OMS installer. Each data
// center gets its own cluster, so every run stores its kubeconfig in that data center's encrypted
// install vault for the remaining installer steps. Airgapped bootstraps apply the airgap
// k0sctl config generated by GenerateK0sAirgapConfig.
func (b *GCPBootstrapper) InstallK0s(dataCenters []*datacenter.DataCenter) error {
	for _, dc := range dataCenters {
		err := b.stlog.Step(dc.StepName("Install k0s"), func() error {
			return b.installK0s(dc)
		})
		if err != nil {
			return fmt.Errorf("failed to install k0s (data center %d): %w", dc.ID, err)
		}
	}

	return nil
}

func (b *GCPBootstrapper) installK0s(dc *datacenter.DataCenter) error {
	// Reuse matching cached binaries and let k0sctl reconcile normally. Without
	// --force, an unchanged cluster remains untouched on bootstrap retries. --yes regenerates
	// the k0sctl config of an earlier run, as there is no one to confirm it.
	configArgs := fmt.Sprintf("--version %s --install-config %s --yes", b.Env.K0sVersion, dc.RemoteConfigPath)
	if b.Env.Airgapped {
		configArgs = "--config " + dc.RemoteK0sctlConfigPath()
	}

	installCmd := fmt.Sprintf("oms install k0s %s --vault %s --vault-priv-key %s",
		configArgs, dc.RemoteVaultPath(), dc.RemoteAgeKeyPath())
	if err := b.Env.Jumpbox.RunSSHCommand("root", installCmd); err != nil {
		return fmt.Errorf("failed to install k0s from jumpbox (data center %d): %w", dc.ID, err)
	}

	return nil
}

// WaitForK0sNodes restores the readiness barrier from the TypeScript
// Kubernetes setup. k0sctl apply completing is not sufficient for the
// Codesphere charts: all schedulable nodes must be Ready before gateway
// controllers and their admission webhooks are installed.
func (b *GCPBootstrapper) WaitForK0sNodes() error {
	if err := b.ensureDataCenters(); err != nil {
		return err
	}

	for _, dc := range b.Env.DataCenters {
		err := b.stlog.Step(dc.StepName("Wait for k0s nodes"), func() error {
			return b.waitForK0sNodes(dc)
		})
		if err != nil {
			return fmt.Errorf("failed waiting for k0s nodes (data center %d): %w", dc.ID, err)
		}
	}

	return nil
}

func (b *GCPBootstrapper) waitForK0sNodes(dc *datacenter.DataCenter) error {
	const command = "k0s kubectl wait --for=condition=Ready nodes --all --timeout=30m"
	if err := dc.ControlPlaneNodes[0].RunSSHCommand("root", command); err != nil {
		return fmt.Errorf("k0s nodes did not become ready (data center %d): %w", dc.ID, err)
	}

	return nil
}
