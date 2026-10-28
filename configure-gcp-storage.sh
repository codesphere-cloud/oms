#!/bin/bash
set -euo pipefail

KUBECTL="k0s kubectl"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

# Helm does not inherit the embedded kubeconfig used by the k0s kubectl command.
# Generate a short-lived admin kubeconfig rather than depending on k0s's internal file layout.
export KUBECONFIG="$workdir/kubeconfig"
k0s kubeconfig admin >"$KUBECONFIG"
chmod 0600 "$KUBECONFIG"

if ! command -v helm >/dev/null 2>&1 || [[ "$(helm version --template '{{.Version}}')" != "v3.17.3"* ]]; then
  archive="$(mktemp)"
  curl -fsSL "https://get.helm.sh/helm-v3.17.3-linux-amd64.tar.gz" -o "$archive"
  echo "ee88b3c851ae6466a3de507f7be73fe94d54cbf2987cbaa3d1a3832ea331f2cd  $archive" | sha256sum --check --status
  tar -xzf "$archive" -C /tmp linux-amd64/helm
  install -m 0755 /tmp/linux-amd64/helm /usr/local/bin/helm
  rm -f "$archive" /tmp/linux-amd64/helm
  rmdir /tmp/linux-amd64 2>/dev/null || true
fi

pd_archive="$workdir/gce-pd-csi-driver.tar.gz"
mkdir "$workdir/driver"
curl -fsSL "https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/archive/refs/tags/v1.13.9.tar.gz" -o "$pd_archive"
echo "1538ede8ef07292b0e1ecffbd37a58741b357779477d0de6c663a4e18311febc  $pd_archive" | sha256sum --check --status
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
  name: gce-pd-rwo
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

$KUBECTL create namespace rwx-csi-system --dry-run=client -o yaml | $KUBECTL apply -f -
$KUBECTL -n rwx-csi-system create secret docker-registry rwox-csi-ghcr \
  --docker-server=ghcr.io \
  --docker-username="$RWX_REGISTRY_USER" \
  --docker-password="$RWX_REGISTRY_PASSWORD" \
  --dry-run=client -o yaml | $KUBECTL apply -f -

# The rwox repository currently publishes commit-tagged container images but no OCI chart.
# Fetch the chart from the exact source commit matching the image so bootstrap is reproducible.
rwox_archive="$workdir/rwox-csi.tar.gz"
mkdir "$workdir/rwox-csi"
curl -fsSL \
  "https://api.github.com/repos/codesphere-cloud/rwox-csi/tarball/07b9e650c564848d06e4fc031080697dd854f431" \
  -o "$rwox_archive"
tar -xzf "$rwox_archive" -C "$workdir/rwox-csi" --strip-components=1

cat <<'EOF' >/root/rwox-k0s-post-renderer.sh
#!/bin/sh
sed 's#/var/lib/kubelet#/var/lib/k0s/kubelet#g'
EOF
chmod 0755 /root/rwox-k0s-post-renderer.sh

helm upgrade --install rwox-csi "$workdir/rwox-csi/chart/rwox-csi" \
  --namespace rwx-csi-system \
  --create-namespace \
  --post-renderer /root/rwox-k0s-post-renderer.sh \
  --set-string storageClass.name=codesphere \
  --set-string storageClass.backingStorageClass=gce-pd-rwo \
  --set-string image.tag=sha-07b9e65 \
  --set-string 'imagePullSecrets[0].name=rwox-csi-ghcr' \
  --wait --timeout 10m
