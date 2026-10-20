// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"slices"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// K0sPackageBinaryPath is where the Codesphere package places the k0s binary on the nodes.
const K0sPackageBinaryPath = "/etc/codesphere/deps/kubernetes/files/k0s"

// K0sRebootCommand schedules a reboot and returns immediately, so the SSH session that runs it
// is not torn down by the reboot itself.
const K0sRebootCommand = "systemctl reboot --no-block || (nohup sh -c 'sleep 2; reboot' >/dev/null 2>&1 &)"

// K0sCleanupNodeIPs returns the unique IPs of all control plane and worker nodes of the
// install-config, control planes first.
func K0sCleanupNodeIPs(installConfig *files.RootConfig) []string {
	var ips []string

	nodes := append(slices.Clone(installConfig.Kubernetes.ControlPlanes), installConfig.Kubernetes.Workers...)
	for _, n := range nodes {
		if n.IPAddress != "" && !slices.Contains(ips, n.IPAddress) {
			ips = append(ips, n.IPAddress)
		}
	}

	return ips
}

// K0sCleanupScript returns a shell script that removes k0s from a node: it stops and disables
// the k0s services, runs k0s reset and deletes the k0s service units, data and config.
// Controllers (also with --enable-worker) run k0scontroller, dedicated workers run k0sworker,
// so both are handled. Missing services, binaries or files are tolerated, so the script also
// cleans up partially installed or corrupted nodes and can be rerun safely.
//
// If k0s reset fails, workload volumes may still be mounted below the k0s directories. The
// script unmounts them and exits with an error instead of deleting through a remaining mount,
// which would wipe the volume contents. Failed deletions also make the script fail, so the
// node is not reported as cleaned. A reboot is required afterwards, see K0sRebootCommand.
func K0sCleanupScript() string {
	return fmt.Sprintf(`for svc in k0sworker k0scontroller; do
  systemctl stop "$svc.service" 2>/dev/null || true
  systemctl disable "$svc.service" 2>/dev/null || true
done
K0S="$(command -v k0s || true)"
if [ -z "$K0S" ] && [ -x %[1]s ]; then K0S=%[1]s; fi
if [ -n "$K0S" ]; then
  "$K0S" reset || echo "warning: k0s reset failed, continuing cleanup"
else
  echo "k0s binary not found, skipping k0s reset"
fi
k0s_mounts() { awk '$2 ~ "^/(var/lib/k0s|etc/k0s)(/|$)" { print $2 }' /proc/mounts | sort -r; }
for mnt in $(k0s_mounts); do umount "$mnt" || true; done
if [ -n "$(k0s_mounts)" ]; then
  echo "error: still mounted below /var/lib/k0s or /etc/k0s, not deleting them:" >&2
  k0s_mounts >&2
  echo "reboot the node and run the cleanup again" >&2
  exit 1
fi
rm -f /etc/systemd/system/k0sworker.service /etc/systemd/system/k0scontroller.service || exit 1
rm -rf --one-file-system /var/lib/k0s /etc/k0s || exit 1
systemctl daemon-reload
`, K0sPackageBinaryPath)
}
