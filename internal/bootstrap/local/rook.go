// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/codesphere-cloud/oms/internal/installer"
	rookcephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	rookRepoURL             = "https://charts.rook.io/release"
	rookReleaseName         = "rook-ceph"
	rookNamespace           = "rook-ceph"
	rookClusterName         = "rook-ceph"
	rookCephImage           = "quay.io/ceph/ceph:v19.2.3"
	rookCephDataDirHostPath = "/var/lib/rook"
	rookReadyTimeout        = 30 * time.Minute
	rookReadyPollInterval   = 5 * time.Second

	cephBlockPoolName      = "codesphere-rbd"
	cephStorageClassName   = "codesphere-rbd"
	cephRBDProvisionerName = "rook-ceph.rbd.csi.ceph.com"
)

// csiResourceEntry represents a single container resource definition for Rook CSI drivers.
type csiResourceEntry struct {
	Name     string                 `json:"name"`
	Resource map[string]interface{} `json:"resource"`
}

// buildRookHelmValues constructs the Helm values for the Rook operator chart.
// It configures the operator and CSI drivers without resource requests
// but preserves the default memory limits for each container.
//
// Config is based on https://github.com/rook/rook/blob/master/deploy/charts/rook-ceph/values.yaml#L233
func (b *LocalBootstrapper) buildRookHelmValues() (map[string]interface{}, error) {
	limitOnly := func(memory string) map[string]interface{} {
		return map[string]interface{}{
			"limits":   map[string]string{"memory": memory},
			"requests": map[string]string{"cpu": "0", "memory": "0"},
		}
	}

	marshalCSIResources := func(entries []csiResourceEntry) (string, error) {
		b, err := json.Marshal(entries)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	rbdProvisioner, err := marshalCSIResources([]csiResourceEntry{
		{"csi-provisioner", limitOnly("256Mi")},
		{"csi-resizer", limitOnly("256Mi")},
		{"csi-attacher", limitOnly("256Mi")},
		{"csi-snapshotter", limitOnly("256Mi")},
		{"csi-rbdplugin", limitOnly("1Gi")},
		{"liveness-prometheus", limitOnly("256Mi")},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal csiRBDProvisionerResource: %w", err)
	}

	rbdPlugin, err := marshalCSIResources([]csiResourceEntry{
		{"driver-registrar", limitOnly("256Mi")},
		{"csi-rbdplugin", limitOnly("1Gi")},
		{"liveness-prometheus", limitOnly("256Mi")},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal csiRBDPluginResource: %w", err)
	}

	cephfsProvisioner, err := marshalCSIResources([]csiResourceEntry{
		{"csi-provisioner", limitOnly("256Mi")},
		{"csi-resizer", limitOnly("256Mi")},
		{"csi-attacher", limitOnly("256Mi")},
		{"csi-snapshotter", limitOnly("256Mi")},
		{"csi-cephfsplugin", limitOnly("1Gi")},
		{"liveness-prometheus", limitOnly("256Mi")},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal csiCephFSProvisionerResource: %w", err)
	}

	cephfsPlugin, err := marshalCSIResources([]csiResourceEntry{
		{"driver-registrar", limitOnly("256Mi")},
		{"csi-cephfsplugin", limitOnly("1Gi")},
		{"liveness-prometheus", limitOnly("256Mi")},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal csiCephFSPluginResource: %w", err)
	}

	kubeletDir := "/var/lib/kubelet"
	if b.Env.K0s {
		kubeletDir = "/var/lib/k0s/kubelet"
	}

	values := map[string]interface{}{
		"priorityClassName": "system-cluster-critical",
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"cpu":    "0",
				"memory": "0",
			},
		},
		"csi": map[string]interface{}{
			"kubeletDirPath":               kubeletDir,
			"csiRBDProvisionerResource":    rbdProvisioner,
			"csiRBDPluginResource":         rbdPlugin,
			"csiCephFSProvisionerResource": cephfsProvisioner,
			"csiCephFSPluginResource":      cephfsPlugin,
		},
	}

	return values, nil
}

func (b *LocalBootstrapper) InstallRookHelmChart() error {
	helmValues, err := b.buildRookHelmValues()
	if err != nil {
		return fmt.Errorf("failed to build Helm values: %w", err)
	}

	if err := b.helm.UpgradeChart(b.ctx, installer.ChartConfig{
		ReleaseName:     rookReleaseName,
		ChartName:       "rook-ceph",
		RepoURL:         rookRepoURL,
		Namespace:       rookNamespace,
		CreateNamespace: true,
		Values:          helmValues,
	}, installer.UpgradeChartOptions{InstallIfNotExist: true}); err != nil {
		return fmt.Errorf("failed to deploy Helm chart %q: %w", rookReleaseName, err)
	}

	return nil
}

func (b *LocalBootstrapper) DeployTestCephCluster() error {
	// Ceph test cluster config from https://github.com/rook/rook/blob/0e05c6afff25a4e03649dd2092a5a10c3349fd9c/deploy/examples/cluster-test.yaml
	cephCluster := &rookcephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rookClusterName,
			Namespace: rookNamespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, cephCluster, func() error {
		cephCluster.Spec = rookcephv1.ClusterSpec{
			CephVersion: rookcephv1.CephVersionSpec{
				Image: rookCephImage,
			},
			DataDirHostPath: rookCephDataDirHostPath,
			Mon: rookcephv1.MonSpec{
				Count:                1,
				AllowMultiplePerNode: true,
			},
			Mgr: rookcephv1.MgrSpec{
				Count:                1,
				AllowMultiplePerNode: true,
			},
			Storage: rookcephv1.StorageScopeSpec{
				// TODO: make configurable.
				UseAllNodes: true,
				Selection: rookcephv1.Selection{
					UseAllDevices: ptr.To(true),
				},
				AllowDeviceClassUpdate:    true,
				AllowOsdCrushWeightUpdate: false,
			},
			PriorityClassNames: rookcephv1.PriorityClassNamesSpec{
				"all": "system-node-critical",
				"mgr": "system-cluster-critical",
			},
			HealthCheck: rookcephv1.CephClusterHealthCheckSpec{
				DaemonHealth: rookcephv1.DaemonHealthSpec{
					Monitor: rookcephv1.HealthCheckSpec{
						Interval: &metav1.Duration{Duration: 45 * time.Second},
						Timeout:  "600s",
					},
				},
			},
			Dashboard: rookcephv1.DashboardSpec{
				Enabled: true,
			},
			DisruptionManagement: rookcephv1.DisruptionManagementSpec{
				ManagePodBudgets: true,
			},
			CrashCollector: rookcephv1.CrashCollectorSpec{
				Disable: true,
			},
			Monitoring: rookcephv1.MonitoringSpec{
				Enabled: false,
			},
			CephConfig: map[string]map[string]string{
				"global": {
					"osd_pool_default_size":          "1",
					"mon_warn_on_pool_no_redundancy": "false",
					"bdev_flock_retry":               "20",
					"bluefs_buffered_io":             "false",
					"mon_data_avail_warn":            "10",
				},
			},
			Resources: rookcephv1.ResourceSpec{
				"mon":            {},
				"mgr":            {},
				"osd":            {},
				"mgr-sidecar":    {},
				"crashcollector": {},
				"cleanup":        {},
			},
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update Ceph cluster %q: %w", rookClusterName, err)
	}

	if err := b.WaitForTestCephClusterReady(); err != nil {
		return err
	}

	return nil
}

func (b *LocalBootstrapper) WaitForTestCephClusterReady() error {
	ctx, cancel := context.WithTimeout(b.ctx, rookReadyTimeout)
	defer cancel()

	clusterKey := client.ObjectKey{
		Name:      rookClusterName,
		Namespace: rookNamespace,
	}

	steps := int(rookReadyTimeout / rookReadyPollInterval)
	if steps < 1 {
		steps = 1
	}

	backoff := wait.Backoff{
		Duration: rookReadyPollInterval,
		Factor:   1.0,
		Jitter:   0.1,
		Steps:    steps,
	}

	lastPhase := ""
	lastState := ""
	lastMessage := ""

	err := retry.OnError(backoff, isRetryableWaitError, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}

		cluster := &rookcephv1.CephCluster{}
		err := b.kubeClient.Get(ctx, clusterKey, cluster)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return &retryableWaitError{err: fmt.Errorf("ceph cluster %q not found yet", rookClusterName)}
			}

			return err
		}

		lastPhase = string(cluster.Status.Phase)
		lastState = string(cluster.Status.State)
		lastMessage = cluster.Status.Message

		if isRookCephClusterReady(cluster) {
			return nil
		}

		return &retryableWaitError{err: fmt.Errorf(
			"ceph cluster is not ready yet (phase=%q, state=%q, message=%q)",
			lastPhase,
			lastState,
			lastMessage,
		)}
	})
	if err == nil {
		return nil
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || isRetryableWaitError(err) {
		return fmt.Errorf(
			"timed out waiting for Ceph cluster %q to become ready (phase=%q, state=%q, message=%q, error=%v)",
			rookClusterName,
			lastPhase,
			lastState,
			lastMessage,
			err,
		)
	}

	return fmt.Errorf("failed to fetch Ceph cluster %q: %w", rookClusterName, err)
}

func (b *LocalBootstrapper) DeployCephBlockPoolAndStorageClass() error {
	// Create CephBlockPool
	blockPool := &rookcephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cephBlockPoolName,
			Namespace: rookNamespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, blockPool, func() error {
		blockPool.Spec = rookcephv1.NamedBlockPoolSpec{
			PoolSpec: rookcephv1.PoolSpec{
				FailureDomain: "osd",
				Replicated: rookcephv1.ReplicatedSpec{
					Size:                   1,
					RequireSafeReplicaSize: false,
				},
			},
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update CephBlockPool %q: %w", cephBlockPoolName, err)
	}

	// Create StorageClass
	reclaimPolicy := corev1.PersistentVolumeReclaimDelete
	volumeBindingMode := storagev1.VolumeBindingImmediate
	storageClass := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: cephStorageClassName,
		},
	}

	_, err = controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, storageClass, func() error {
		storageClass.Provisioner = cephRBDProvisionerName
		storageClass.Parameters = map[string]string{
			"clusterID":     rookNamespace,
			"pool":          cephBlockPoolName,
			"imageFormat":   "2",
			"imageFeatures": "layering",
			"csi.storage.k8s.io/provisioner-secret-name":            "rook-csi-rbd-provisioner",
			"csi.storage.k8s.io/provisioner-secret-namespace":       rookNamespace,
			"csi.storage.k8s.io/controller-expand-secret-name":      "rook-csi-rbd-provisioner",
			"csi.storage.k8s.io/controller-expand-secret-namespace": rookNamespace,
			"csi.storage.k8s.io/node-stage-secret-name":             "rook-csi-rbd-node",
			"csi.storage.k8s.io/node-stage-secret-namespace":        rookNamespace,
			"csi.storage.k8s.io/fstype":                             "ext4",
		}
		storageClass.ReclaimPolicy = &reclaimPolicy
		storageClass.VolumeBindingMode = &volumeBindingMode
		storageClass.AllowVolumeExpansion = ptr.To(true)
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update StorageClass %q: %w", cephStorageClassName, err)
	}

	return nil
}

func isRookCephClusterReady(cluster *rookcephv1.CephCluster) bool {
	if cluster == nil {
		return false
	}

	for _, condition := range cluster.Status.Conditions {
		if condition.Type == rookcephv1.ConditionReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}

	return cluster.Status.Phase == rookcephv1.ConditionReady
}
