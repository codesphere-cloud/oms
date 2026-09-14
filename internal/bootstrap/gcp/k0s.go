// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"
	"strings"

	"github.com/codesphere-cloud/oms/internal/bootstrap/datacenter"
	"github.com/codesphere-cloud/oms/internal/installer"
)

// EnsureK0s executed all steps to ensure a k0s cluster in gcp for every data center.
// Only executing the config script needs to be done after installing codesphere, as crucial parts are still in the ts-installer.
// Returns an error if k0s could not be ensured.
func (b *GCPBootstrapper) EnsureK0s() error {
	err := b.GenerateK0sConfigScript()
	if err != nil {
		return fmt.Errorf("failed to generate k0s config script: %w", err)
	}

	err = b.InstallK0s()
	if err != nil {
		return fmt.Errorf("failed to install k0s: %w", err)
	}

	err = b.WaitForK0sNodes()
	if err != nil {
		return fmt.Errorf("failed waiting for k0s nodes to get ready: %w", err)
	}

	return nil
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

	for i := 1; i < len(dc.ControlPlaneNodes); i++ {
		internalIP := dc.ControlPlaneNodes[i].GetInternalIP()
		fmt.Fprintf(&enableWorkerDaemonsCmds, "ssh -o StrictHostKeyChecking=no root@%s sed -i 's/k0sworker/k0sworker --enable-cloud-provider/g' /etc/systemd/system/k0sworker.service; systemctl daemon-reload; systemctl restart k0sworker", internalIP)
		fmt.Fprint(&enableWorkerDaemonsCmds, "\n")
	}

	script := fmt.Sprintf(`#!/bin/bash

cat <<EOF > cloud.conf
[Global]
project-id = "$PROJECT_ID"
EOF

cat <<EOF >> cc-deployment.yaml
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

KUBECTL="/etc/codesphere/deps/kubernetes/files/k0s kubectl"
$KUBECTL create configmap cloud-config --from-file=cloud.conf -n kube-system
echo alias kubectl=\"$KUBECTL\" >> /root/.bashrc
echo alias k=\"$KUBECTL\" >> /root/.bashrc

$KUBECTL apply -f https://raw.githubusercontent.com/kubernetes/cloud-provider-gcp/refs/tags/providers/v0.28.2/deploy/packages/default/manifest.yaml

$KUBECTL apply -f cc-deployment.yaml

# set loadBalancerIP for public-gateway-controller and gateway-controller
$KUBECTL patch svc public-gateway-controller -n codesphere -p '{"spec": {"loadBalancerIP": "'%s'"}}'
$KUBECTL patch svc gateway-controller -n codesphere -p '{"spec": {"loadBalancerIP": "'%s'"}}'

%s

sed -i 's/k0scontroller/k0scontroller --enable-cloud-provider/g' /etc/systemd/system/k0scontroller.service
systemctl daemon-reload
systemctl restart k0scontroller
`, dc.PublicGatewayIP, dc.GatewayIP, enableWorkerDaemonsCmds.String())

	// Probably we need to enable the cloud provider plugin in k0s configuration.
	// --enable-cloud-provider on worker nodes systemd file /etc/systemd/system/k0sworker.service
	// in addition on the first node: /etc/systemd/system/k0scontroller.service the flag --enable-cloud-provider

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

// InstallK0s deploys k0s into every data center with the native OMS installer. Each data center
// gets its own cluster, so every run stores its kubeconfig in that data center's encrypted
// install vault for the remaining installer steps.
func (b *GCPBootstrapper) InstallK0s() error {
	if err := b.ensureDataCenters(); err != nil {
		return err
	}

	for _, dc := range b.Env.DataCenters {
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
	// --force, an unchanged cluster remains untouched on bootstrap retries.
	installCmd := fmt.Sprintf("oms install k0s --version %s --install-config %s --vault %s --vault-priv-key %s",
		installer.DefaultK0sVersion, dc.RemoteConfigPath, dc.RemoteVaultPath(), dc.RemoteAgeKeyPath())
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
