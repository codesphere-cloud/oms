// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"encoding/base64"
	"fmt"

	"github.com/codesphere-cloud/oms/internal/bootstrap/datacenter"
)

const (
	gcePDCSIDriverVersion      = "v1.13.9"
	gcePDCSIDriverSHA256       = "1538ede8ef07292b0e1ecffbd37a58741b357779477d0de6c663a4e18311febc"
	gcePDRWOStorageClassName   = "gce-pd-rwo"
	helmDownloadSHA256         = "ee88b3c851ae6466a3de507f7be73fe94d54cbf2987cbaa3d1a3832ea331f2cd"
	helmVersion                = "v3.17.3"
	remoteGCPStorageScriptPath = "/root/configure-gcp-storage.sh"
	rwoxCSICommit              = "1ab0c853f47e1c2254a4c8e5b093f51b6ee99b07"
	rwoxCSIImageTag            = "sha-1ab0c85"
	rwoxCSINamespace           = "rwx-csi-system"
	rwoxCSIRegistrySecretName  = "rwox-csi-ghcr"
	rwoxStorageClassName       = "codesphere"
)

// EnsureGCPStorage installs one GCE PD CSI driver and rwox-csi release in every data center.
// It runs on the first control-plane node because the Kubernetes API is private to the VPC.
func (b *GCPBootstrapper) EnsureGCPStorage() error {
	if err := b.ensureDataCenters(); err != nil {
		return err
	}

	for _, dc := range b.Env.DataCenters {
		err := b.stlog.Step(dc.StepName("Install GCP storage"), func() error {
			if err := b.generateGCPStorageScript(dc); err != nil {
				return err
			}

			command := fmt.Sprintf("RWX_REGISTRY_USER=%s RWX_REGISTRY_PASSWORD=%s %s",
				shellQuote(b.Env.RegistryUser), shellQuote(b.Env.GitHubPAT), remoteGCPStorageScriptPath)
			if err := dc.ControlPlaneNodes[0].RunSSHCommand("root", command); err != nil {
				return fmt.Errorf("failed to run GCP storage setup in data center %d: %w", dc.ID, err)
			}

			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to install GCP storage in data center %d: %w", dc.ID, err)
		}
	}

	return nil
}

func (b *GCPBootstrapper) generateGCPStorageScript(dc *datacenter.DataCenter) error {
	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

KUBECTL="k0s kubectl"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

# Helm does not inherit the embedded kubeconfig used by the k0s kubectl command.
# Generate a short-lived admin kubeconfig rather than depending on k0s's internal file layout.
export KUBECONFIG="$workdir/kubeconfig"
k0s kubeconfig admin >"$KUBECONFIG"
chmod 0600 "$KUBECONFIG"

if ! command -v helm >/dev/null 2>&1 || [[ "$(helm version --template '{{.Version}}')" != "%s"* ]]; then
  archive="$(mktemp)"
  curl -fsSL "https://get.helm.sh/helm-%s-linux-amd64.tar.gz" -o "$archive"
  echo "%s  $archive" | sha256sum --check --status
  tar -xzf "$archive" -C /tmp linux-amd64/helm
  install -m 0755 /tmp/linux-amd64/helm /usr/local/bin/helm
  rm -f "$archive" /tmp/linux-amd64/helm
  rmdir /tmp/linux-amd64 2>/dev/null || true
fi

pd_archive="$workdir/gce-pd-csi-driver.tar.gz"
mkdir "$workdir/driver"
curl -fsSL "https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/archive/refs/tags/%s.tar.gz" -o "$pd_archive"
echo "%s  $pd_archive" | sha256sum --check --status
tar -xzf "$pd_archive" -C "$workdir/driver" --strip-components=1

# The noauth overlay uses the VM's attached service account through the GCE metadata server,
# avoiding a long-lived JSON service-account key. The upstream manifests use the standard
# kubelet path, so rewrite every host path and registrar argument for k0s before applying them.
$KUBECTL create namespace gce-pd-csi-driver --dry-run=client -o yaml | $KUBECTL apply -f -
$KUBECTL kustomize "$workdir/driver/deploy/kubernetes/overlays/noauth" |
  sed 's#/var/lib/kubelet#/var/lib/k0s/kubelet#g' |
  $KUBECTL apply -f -

cat <<'EOF' | $KUBECTL apply -f -
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: %s
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
provisioner: pd.csi.storage.gke.io
parameters:
  type: pd-balanced
  csi.storage.k8s.io/fstype: ext4
reclaimPolicy: Delete
allowVolumeExpansion: true
volumeBindingMode: WaitForFirstConsumer
EOF

$KUBECTL -n gce-pd-csi-driver rollout status deployment/csi-gce-pd-controller --timeout=10m
$KUBECTL -n gce-pd-csi-driver rollout status daemonset/csi-gce-pd-node --timeout=10m

if [[ -z "${RWX_REGISTRY_USER:-}" || -z "${RWX_REGISTRY_PASSWORD:-}" ]]; then
  echo "rwox-csi requires RWX_REGISTRY_USER and RWX_REGISTRY_PASSWORD" >&2
  exit 1
fi

$KUBECTL create namespace %s --dry-run=client -o yaml | $KUBECTL apply -f -
$KUBECTL -n %s create secret docker-registry %s \
  --docker-server=ghcr.io \
  --docker-username="$RWX_REGISTRY_USER" \
  --docker-password="$RWX_REGISTRY_PASSWORD" \
  --dry-run=client -o yaml | $KUBECTL apply -f -

# The rwox repository currently publishes commit-tagged container images but no OCI chart.
# Fetch the chart from the exact source commit matching the image so bootstrap is reproducible.
rwox_archive="$workdir/rwox-csi.tar.gz"
mkdir "$workdir/rwox-csi"
curl -fsSL \
  "https://api.github.com/repos/codesphere-cloud/rwox-csi/tarball/%s" \
  -o "$rwox_archive"
tar -xzf "$rwox_archive" -C "$workdir/rwox-csi" --strip-components=1

# Helm installs CRDs but does not upgrade them. Apply the schema explicitly so
# new per-volume ownership fields are accepted on both fresh and existing clusters.
$KUBECTL apply -f "$workdir/rwox-csi/chart/rwox-csi/crds/storage.example.io_rwxvolumes.yaml"

cat <<'EOF' >/root/rwox-k0s-post-renderer.sh
#!/bin/sh
sed 's#/var/lib/kubelet#/var/lib/k0s/kubelet#g'
EOF
chmod 0755 /root/rwox-k0s-post-renderer.sh

# StorageClass parameters are immutable. A retry or upgrade from an older rwox-csi
# configuration must recreate the class when its managed parameters differ.
if $KUBECTL get storageclass %s >/dev/null 2>&1; then
  current_backing="$($KUBECTL get storageclass %s -o jsonpath='{.parameters.backingStorageClass}')"
  current_mode="$($KUBECTL get storageclass %s -o jsonpath='{.parameters.directoryMode}')"
  current_uid="$($KUBECTL get storageclass %s -o jsonpath='{.parameters.directoryUID}')"
  current_gid="$($KUBECTL get storageclass %s -o jsonpath='{.parameters.directoryGID}')"
  current_directory="$($KUBECTL get storageclass %s -o jsonpath='{.parameters.backingDirectory}')"
  if [[ "$current_backing" != %s || "$current_mode" != "0777" || "$current_uid" != "1501" || "$current_gid" != "1010" || "$current_directory" != "export" ]]; then
    echo "Recreating StorageClass %s because its immutable parameters differ"
    $KUBECTL delete storageclass %s
  fi
fi

helm upgrade --install rwox-csi "$workdir/rwox-csi/chart/rwox-csi" \
  --namespace %s \
  --create-namespace \
  --post-renderer /root/rwox-k0s-post-renderer.sh \
  --set-string storageClass.name=%s \
  --set-string storageClass.backingStorageClass=%s \
  --set-string storageClass.directoryMode=0777 \
  --set-string storageClass.directoryUID=1501 \
  --set-string storageClass.directoryGID=1010 \
  --set-string storageClass.backingDirectory=export \
  --set-string image.tag=%s \
  --set-string 'imagePullSecrets[0].name=%s' \
  --wait --timeout 10m
`, helmVersion, helmVersion, helmDownloadSHA256, gcePDCSIDriverVersion, gcePDCSIDriverSHA256,
		gcePDRWOStorageClassName, rwoxCSINamespace, rwoxCSINamespace, rwoxCSIRegistrySecretName,
		rwoxCSICommit, rwoxStorageClassName, rwoxStorageClassName, rwoxStorageClassName,
		rwoxStorageClassName, rwoxStorageClassName, rwoxStorageClassName, gcePDRWOStorageClassName, rwoxStorageClassName, rwoxStorageClassName,
		rwoxCSINamespace, rwoxStorageClassName,
		gcePDRWOStorageClassName, rwoxCSIImageTag, rwoxCSIRegistrySecretName)

	controller := dc.ControlPlaneNodes[0]
	encodedScript := base64.StdEncoding.EncodeToString([]byte(script))
	uploadCommand := fmt.Sprintf("printf '%%s' %s | base64 -d > %s && chmod 0755 %s",
		encodedScript, remoteGCPStorageScriptPath, remoteGCPStorageScriptPath)
	if err := controller.RunSSHCommand("root", uploadCommand); err != nil {
		return fmt.Errorf("failed to upload GCP storage setup script to control plane node: %w", err)
	}

	return nil
}
