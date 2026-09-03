package gcp

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer"
)

// EnsureK0s executed all steps to ensure a k0s cluster in gcp.
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

// GenerateK0sConfigScript creates a script to confire k0s in a gcp VM that will be executed on the control plane
// Returns an error if the script can't be generated, written, copied.
func (b *GCPBootstrapper) GenerateK0sConfigScript() error {
	var enableWorkerDaemonsCmds strings.Builder

	for i := 1; i < len(b.Env.ControlPlaneNodes); i++ {
		internalIP := b.Env.ControlPlaneNodes[i].GetInternalIP()
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
`, b.Env.PublicGatewayIP, b.Env.GatewayIP, enableWorkerDaemonsCmds.String())

	// Probably we need to enable the cloud provider plugin in k0s configuration.
	// --enable-cloud-provider on worker nodes systemd file /etc/systemd/system/k0sworker.service
	// in addition on the first node: /etc/systemd/system/k0scontroller.service the flag --enable-cloud-provider

	err := b.fw.WriteFile("configure-k0s.sh", []byte(script), 0755)
	if err != nil {
		return fmt.Errorf("failed to write configure-k0s.sh: %w", err)
	}

	err = b.Env.ControlPlaneNodes[0].NodeClient.CopyFile(b.Env.ControlPlaneNodes[0], "configure-k0s.sh", "/root/configure-k0s.sh")
	if err != nil {
		return fmt.Errorf("failed to copy configure-k0s.sh to control plane node: %w", err)
	}

	err = b.Env.ControlPlaneNodes[0].RunSSHCommand("root", "chmod +x /root/configure-k0s.sh")
	if err != nil {
		return fmt.Errorf("failed to make configure-k0s.sh executable on control plane node: %w", err)
	}

	return nil
}

// RunK0sConfigScript executed the configure script for k0s on the control-plane
// Return an error if executing the ssh command fails
func (b *GCPBootstrapper) RunK0sConfigScript() error {
	err := b.Env.ControlPlaneNodes[0].RunSSHCommand("root", "/root/configure-k0s.sh")
	if err != nil {
		return fmt.Errorf("failed to configure k0s on the control-plane: %w", err)
	}

	return nil
}

// InstallK0s deploys k0s with the native OMS installer and stores its
// kubeconfig in the encrypted install vault for the remaining installer steps.
func (b *GCPBootstrapper) InstallK0s() error {
	// Reuse matching cached binaries and let k0sctl reconcile normally. Without
	// --force, an unchanged cluster remains untouched on bootstrap retries.
	installCmd := fmt.Sprintf("oms install k0s --version %s --install-config /etc/codesphere/config.yaml --vault %s --vault-priv-key %s/age_key.txt",
		installer.DefaultK0sVersion, filepath.Join(b.Env.SecretsDir, "prod.vault.yaml"), b.Env.SecretsDir)
	if err := b.Env.Jumpbox.RunSSHCommand("root", installCmd); err != nil {
		return fmt.Errorf("failed to install k0s from jumpbox: %w", err)
	}

	return nil
}

// WaitForK0sNodes restores the readiness barrier from the TypeScript
// Kubernetes setup. k0sctl apply completing is not sufficient for the
// Codesphere charts: all schedulable nodes must be Ready before gateway
// controllers and their admission webhooks are installed.
func (b *GCPBootstrapper) WaitForK0sNodes() error {
	const command = "k0s kubectl wait --for=condition=Ready nodes --all --timeout=30m"
	if err := b.Env.ControlPlaneNodes[0].RunSSHCommand("root", command); err != nil {
		return fmt.Errorf("k0s nodes did not become ready: %w", err)
	}

	return nil
}
