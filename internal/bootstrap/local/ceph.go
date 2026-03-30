// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"fmt"
	"time"

	rookcephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	cephFilesystemName         = "codesphere"
	cephSubVolumeGroupName     = "workspace-volumes"
	cephFilesystemReadyTimeout = 10 * time.Minute
	cephClientReadyTimeout     = 5 * time.Minute
	cephReadyPollInterval      = 5 * time.Second
)

// CephUserCredentials holds the entity name and key for a Ceph auth user.
type CephUserCredentials struct {
	Entity string
	Key    string
}

// CephCredentials holds all Ceph credentials needed by Codesphere.
type CephCredentials struct {
	FSID                  string
	CephfsAdmin           CephUserCredentials
	CephfsAdminCodesphere CephUserCredentials
	CSIRBDNode            CephUserCredentials
	CSIRBDProvisioner     CephUserCredentials
	CSICephFSNode         CephUserCredentials
	CSICephFSProvisioner  CephUserCredentials
}

// cephClientDef defines a CephClient to create.
type cephClientDef struct {
	// name is the CephClient CR name (without the "client." prefix).
	name string
	// caps is the Ceph auth capabilities.
	caps map[string]string
}

// DeployCephFilesystem creates the CephFS filesystem "codesphere" via a CephFilesystem CRD.
func (b *LocalBootstrapper) DeployCephFilesystem() error {
	fs := &rookcephv1.CephFilesystem{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cephFilesystemName,
			Namespace: rookNamespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, fs, func() error {
		fs.Spec = rookcephv1.FilesystemSpec{
			MetadataPool: rookcephv1.NamedPoolSpec{
				PoolSpec: rookcephv1.PoolSpec{
					Replicated: rookcephv1.ReplicatedSpec{
						Size:                   1,
						RequireSafeReplicaSize: false,
					},
				},
			},
			DataPools: []rookcephv1.NamedPoolSpec{
				{
					PoolSpec: rookcephv1.PoolSpec{
						FailureDomain: "osd",
						Replicated: rookcephv1.ReplicatedSpec{
							Size:                   1,
							RequireSafeReplicaSize: false,
						},
					},
				},
			},
			PreserveFilesystemOnDelete: true,
			MetadataServer: rookcephv1.MetadataServerSpec{
				ActiveCount:   1,
				ActiveStandby: false,
				Resources:     corev1.ResourceRequirements{},
			},
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update CephFilesystem %q: %w", cephFilesystemName, err)
	}

	return b.waitForCephFilesystemReady()
}

// DeployCephFilesystemSubVolumeGroup creates the "workspace-volumes" SubVolumeGroup on the CephFS filesystem.
func (b *LocalBootstrapper) DeployCephFilesystemSubVolumeGroup() error {
	svg := &rookcephv1.CephFilesystemSubVolumeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cephSubVolumeGroupName,
			Namespace: rookNamespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, svg, func() error {
		svg.Spec = rookcephv1.CephFilesystemSubVolumeGroupSpec{
			FilesystemName: cephFilesystemName,
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update CephFilesystemSubVolumeGroup %q: %w", cephSubVolumeGroupName, err)
	}

	return nil
}

// DeployCephUsers creates CephClient CRDs for the Ceph auth users required by Codesphere.
func (b *LocalBootstrapper) DeployCephUsers() error {
	clients := []cephClientDef{
		{
			name: "cephfs-admin-blue",
			caps: map[string]string{
				"mds": "allow *",
				"mon": "allow *",
				"osd": "allow *",
				"mgr": "allow *",
			},
		},
		{
			name: "cephfs-codesphere-admin",
			caps: map[string]string{
				"mon": "allow r",
				"osd": fmt.Sprintf(
					"allow rwx pool=cephfs.%s.meta,allow rwx pool=cephfs.%s.data",
					cephFilesystemName, cephFilesystemName,
				),
			},
		},
	}

	for _, def := range clients {
		cc := &rookcephv1.CephClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      def.name,
				Namespace: rookNamespace,
			},
		}

		_, err := controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, cc, func() error {
			cc.Spec = rookcephv1.ClientSpec{
				Caps: def.caps,
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to create or update CephClient %q: %w", def.name, err)
		}

		if err := b.waitForCephClientReady(def.name); err != nil {
			return err
		}
	}

	return nil
}

// ReadCephCredentials reads all Ceph credentials from the cluster:
//   - FSID from CephCluster status
//   - Custom user keys from CephClient-generated K8s Secrets
//   - CSI user keys from Rook-managed K8s Secrets
func (b *LocalBootstrapper) ReadCephCredentials() (*CephCredentials, error) {
	fsid, err := b.readCephFSID()
	if err != nil {
		return nil, err
	}

	cephfsAdmin, err := b.readCephClientSecret("cephfs-admin-blue")
	if err != nil {
		return nil, err
	}

	cephfsAdminCodesphere, err := b.readCephClientSecret("cephfs-codesphere-admin")
	if err != nil {
		return nil, err
	}

	csiRBDNode, err := b.readCSISecret("rook-csi-rbd-node", "userID", "userKey")
	if err != nil {
		return nil, err
	}

	csiRBDProvisioner, err := b.readCSISecret("rook-csi-rbd-provisioner", "userID", "userKey")
	if err != nil {
		return nil, err
	}

	csiCephFSNode, err := b.readCSISecret("rook-csi-cephfs-node", "userID", "userKey")
	if err != nil {
		return nil, err
	}

	csiCephFSProvisioner, err := b.readCSISecret("rook-csi-cephfs-provisioner", "userID", "userKey")
	if err != nil {
		return nil, err
	}

	return &CephCredentials{
		FSID:                  fsid,
		CephfsAdmin:           *cephfsAdmin,
		CephfsAdminCodesphere: *cephfsAdminCodesphere,
		CSIRBDNode:            *csiRBDNode,
		CSIRBDProvisioner:     *csiRBDProvisioner,
		CSICephFSNode:         *csiCephFSNode,
		CSICephFSProvisioner:  *csiCephFSProvisioner,
	}, nil
}

// readCephFSID reads the Ceph FSID from the CephCluster status.
func (b *LocalBootstrapper) readCephFSID() (string, error) {
	cluster := &rookcephv1.CephCluster{}
	key := client.ObjectKey{Name: rookClusterName, Namespace: rookNamespace}
	if err := b.kubeClient.Get(b.ctx, key, cluster); err != nil {
		return "", fmt.Errorf("failed to get CephCluster %q: %w", rookClusterName, err)
	}

	if cluster.Status.CephStatus == nil || cluster.Status.CephStatus.FSID == "" {
		return "", fmt.Errorf("CephCluster %q does not have an FSID in its status yet", rookClusterName)
	}

	return cluster.Status.CephStatus.FSID, nil
}

// readCephClientSecret reads the key from the K8s Secret created by the Rook operator for a CephClient CR.
// The secret is named "rook-ceph-client-<name>" in the rook-ceph namespace.
func (b *LocalBootstrapper) readCephClientSecret(name string) (*CephUserCredentials, error) {
	secretName := "rook-ceph-client-" + name
	secret := &corev1.Secret{}
	key := client.ObjectKey{Name: secretName, Namespace: rookNamespace}
	if err := b.kubeClient.Get(b.ctx, key, secret); err != nil {
		return nil, fmt.Errorf("failed to get CephClient secret %q: %w", secretName, err)
	}

	userKey, ok := secret.Data[name]
	if !ok {
		return nil, fmt.Errorf("CephClient secret %q does not contain key %q", secretName, name)
	}

	return &CephUserCredentials{
		Entity: "client." + name,
		Key:    string(userKey),
	}, nil
}

// readCSISecret reads a Rook-managed CSI secret from the rook-ceph namespace.
func (b *LocalBootstrapper) readCSISecret(secretName, idKey, keyKey string) (*CephUserCredentials, error) {
	secret := &corev1.Secret{}
	key := client.ObjectKey{Name: secretName, Namespace: rookNamespace}
	if err := b.kubeClient.Get(b.ctx, key, secret); err != nil {
		return nil, fmt.Errorf("failed to get CSI secret %q: %w", secretName, err)
	}

	userID, ok := secret.Data[idKey]
	if !ok {
		return nil, fmt.Errorf("CSI secret %q does not contain key %q", secretName, idKey)
	}

	userKey, ok := secret.Data[keyKey]
	if !ok {
		return nil, fmt.Errorf("CSI secret %q does not contain key %q", secretName, keyKey)
	}

	return &CephUserCredentials{
		Entity: string(userID),
		Key:    string(userKey),
	}, nil
}

// waitForCephFilesystemReady polls until the CephFilesystem reaches the Ready phase.
func (b *LocalBootstrapper) waitForCephFilesystemReady() error {
	ctx, cancel := context.WithTimeout(b.ctx, cephFilesystemReadyTimeout)
	defer cancel()

	fsKey := client.ObjectKey{Name: cephFilesystemName, Namespace: rookNamespace}

	steps := int(cephFilesystemReadyTimeout / cephReadyPollInterval)
	if steps < 1 {
		steps = 1
	}

	backoff := wait.Backoff{
		Duration: cephReadyPollInterval,
		Factor:   1.0,
		Jitter:   0.1,
		Steps:    steps,
	}

	lastPhase := ""

	err := retry.OnError(backoff, isRetryableWaitError, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}

		fs := &rookcephv1.CephFilesystem{}
		if err := b.kubeClient.Get(ctx, fsKey, fs); err != nil {
			if apierrors.IsNotFound(err) {
				return &retryableWaitError{err: fmt.Errorf("CephFilesystem %q not found yet", cephFilesystemName)}
			}
			return err
		}

		if fs.Status != nil {
			lastPhase = string(fs.Status.Phase)
			if fs.Status.Phase == rookcephv1.ConditionReady {
				return nil
			}
		}

		return &retryableWaitError{err: fmt.Errorf(
			"CephFilesystem %q is not ready yet (phase=%q)",
			cephFilesystemName, lastPhase,
		)}
	})
	if err == nil {
		return nil
	}

	if isRetryableWaitError(err) {
		return fmt.Errorf("timed out waiting for CephFilesystem %q to become ready (phase=%q)", cephFilesystemName, lastPhase)
	}

	return fmt.Errorf("failed waiting for CephFilesystem %q: %w", cephFilesystemName, err)
}

// waitForCephClientReady polls until the CephClient reaches the Ready phase.
func (b *LocalBootstrapper) waitForCephClientReady(name string) error {
	ctx, cancel := context.WithTimeout(b.ctx, cephClientReadyTimeout)
	defer cancel()

	ccKey := client.ObjectKey{Name: name, Namespace: rookNamespace}

	steps := int(cephClientReadyTimeout / cephReadyPollInterval)
	if steps < 1 {
		steps = 1
	}

	backoff := wait.Backoff{
		Duration: cephReadyPollInterval,
		Factor:   1.0,
		Jitter:   0.1,
		Steps:    steps,
	}

	lastPhase := ""

	err := retry.OnError(backoff, isRetryableWaitError, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}

		cc := &rookcephv1.CephClient{}
		if err := b.kubeClient.Get(ctx, ccKey, cc); err != nil {
			if apierrors.IsNotFound(err) {
				return &retryableWaitError{err: fmt.Errorf("CephClient %q not found yet", name)}
			}
			return err
		}

		if cc.Status != nil {
			lastPhase = string(cc.Status.Phase)
			if cc.Status.Phase == rookcephv1.ConditionReady {
				return nil
			}
		}

		return &retryableWaitError{err: fmt.Errorf(
			"CephClient %q is not ready yet (phase=%q)",
			name, lastPhase,
		)}
	})
	if err == nil {
		return nil
	}

	if isRetryableWaitError(err) {
		return fmt.Errorf("timed out waiting for CephClient %q to become ready (phase=%q)", name, lastPhase)
	}

	return fmt.Errorf("failed waiting for CephClient %q: %w", name, err)
}
