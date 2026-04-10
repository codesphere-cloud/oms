// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	rookcephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	cephFilesystemName          = "codesphere"
	cephSubVolumeGroupName      = "workspace-volumes"
	cephMonEndpointsConfigMap   = "rook-ceph-mon-endpoints"
	cephMonSecretName           = "rook-ceph-mon"
	cephFilesystemReadyTimeout  = 10 * time.Minute
	cephClientReadyTimeout      = 5 * time.Minute
	cephObjectStoreReadyTimeout = 10 * time.Minute
	cephObjectUserReadyTimeout  = 5 * time.Minute
	cephReadyPollInterval       = 5 * time.Second
	rgwObjectStoreName          = "s3-ms-provider"
	rgwAdminUserName            = "rgw-admin-ops-user"
	rgwAdminUserCaps            = "buckets=*;users=*;ratelimit=*;usage=read;metadata=read;zone=read"
	rgwRealmName                = "s3-ms-provider-realm"
	rgwZoneGroupName            = "s3-ms-provider-zonegroup-default"
	rgwZoneName                 = "s3-ms-provider-zone-default"
)

// CephUserCredentials holds the entity name and key for a Ceph auth user.
type CephUserCredentials struct {
	Entity string
	Key    string
}

type RGWUserCredentials struct {
	AccessKey string
	SecretKey string
}

// CephCredentials holds all Ceph credentials needed by Codesphere.
type CephCredentials struct {
	FSID                  string
	CephfsAdmin           CephUserCredentials
	CephfsAdminCodesphere CephUserCredentials
	RGWAdmin              RGWUserCredentials
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

// EnsureCephUsers creates the Ceph users required by Codesphere and returns
// all resulting Ceph and RGW credentials.
func (b *LocalBootstrapper) EnsureCephUsers() (*CephCredentials, error) {
	b.stlog.Logf("Ensuring Ceph users and collecting credentials")

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
		b.stlog.Logf("Reconciling CephClient %q", def.name)
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
			return nil, fmt.Errorf("failed to create or update CephClient %q: %w", def.name, err)
		}

		if err := b.waitForCephClientReady(def.name); err != nil {
			return nil, err
		}
		b.stlog.Logf("CephClient %q is ready", def.name)
	}

	b.stlog.Logf("Reading Ceph cluster FSID")
	fsid, err := b.readCephFSID()
	if err != nil {
		return nil, err
	}

	b.stlog.Logf("Ensuring RGW admin user %q", rgwAdminUserName)
	rgwAdmin, err := b.EnsureRGWAdminUser()
	if err != nil {
		return nil, err
	}

	b.stlog.Logf("Reading Ceph client secrets")
	cephfsAdmin, err := b.readCephClientSecret("cephfs-admin-blue")
	if err != nil {
		return nil, err
	}

	cephfsAdminCodesphere, err := b.readCephClientSecret("cephfs-codesphere-admin")
	if err != nil {
		return nil, err
	}

	b.stlog.Logf("Reading Rook CSI secrets")
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

	b.stlog.Logf("Ceph users and credentials are ready")
	return &CephCredentials{
		FSID:                  fsid,
		CephfsAdmin:           *cephfsAdmin,
		CephfsAdminCodesphere: *cephfsAdminCodesphere,
		RGWAdmin:              *rgwAdmin,
		CSIRBDNode:            *csiRBDNode,
		CSIRBDProvisioner:     *csiRBDProvisioner,
		CSICephFSNode:         *csiCephFSNode,
		CSICephFSProvisioner:  *csiCephFSProvisioner,
	}, nil
}

// ReadCephMonHosts reads the Rook object store status and converts the
// insecure RGW endpoints into Ceph hosts for the internal install config.
func (b *LocalBootstrapper) ReadCephMonHosts() ([]files.CephHost, error) {
	store := &rookcephv1.CephObjectStore{}
	key := client.ObjectKey{Name: rgwObjectStoreName, Namespace: rookNamespace}
	if err := b.kubeClient.Get(b.ctx, key, store); err != nil {
		return nil, fmt.Errorf("failed to get CephObjectStore %q: %w", key.Name, err)
	}

	return cephHostsFromObjectStore(store)
}

func cephHostsFromObjectStore(store *rookcephv1.CephObjectStore) ([]files.CephHost, error) {
	if store.Status == nil || len(store.Status.Endpoints.Insecure) == 0 {
		return nil, fmt.Errorf("CephObjectStore %q does not contain insecure endpoints", store.Name)
	}

	hosts := make([]files.CephHost, 0, len(store.Status.Endpoints.Insecure))
	seenHosts := make(map[string]struct{}, len(store.Status.Endpoints.Insecure))

	for _, endpoint := range store.Status.Endpoints.Insecure {
		host, err := parseObjectStoreEndpointHost(endpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to parse object store endpoint %q: %w", endpoint, err)
		}

		if _, exists := seenHosts[host]; exists {
			continue
		}

		hosts = append(hosts, files.CephHost{
			Hostname:  host,
			IPAddress: host,
			IsMaster:  len(hosts) == 0,
		})
		seenHosts[host] = struct{}{}
	}

	if len(hosts) == 0 {
		return nil, fmt.Errorf("CephObjectStore %q does not contain any valid insecure endpoints", store.Name)
	}

	return hosts, nil
}

func parseObjectStoreEndpointHost(endpoint string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", err
	}

	if parsed.Hostname() == "" {
		return "", fmt.Errorf("endpoint %q does not contain a hostname", endpoint)
	}

	return parsed.Hostname(), nil
}

func parseMonitorEndpointHost(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", fmt.Errorf("empty endpoint")
	}

	if separator := strings.Index(endpoint, "="); separator >= 0 {
		endpoint = strings.TrimSpace(endpoint[separator+1:])
	}

	if strings.HasPrefix(endpoint, "[") && strings.HasSuffix(endpoint, "]") {
		for _, candidate := range strings.Split(endpoint[1:len(endpoint)-1], ",") {
			host, err := parseMonitorEndpointHost(candidate)
			if err == nil {
				return host, nil
			}
		}
		return "", fmt.Errorf("no valid monitor host found in %q", endpoint)
	}

	endpoint = strings.TrimPrefix(endpoint, "v1:")
	endpoint = strings.TrimPrefix(endpoint, "v2:")
	if slash := strings.Index(endpoint, "/"); slash >= 0 {
		endpoint = endpoint[:slash]
	}

	if host, port, err := net.SplitHostPort(endpoint); err == nil {
		host = strings.Trim(host, "[]")
		if host == "" {
			return "", fmt.Errorf("endpoint %q does not contain a valid host", endpoint)
		}
		if port == "" {
			return host, nil
		}
		return net.JoinHostPort(host, port), nil
	}

	trimmed := strings.Trim(endpoint, "[]")
	if trimmed == "" {
		return "", fmt.Errorf("endpoint %q does not contain a valid host", endpoint)
	}

	if ip := net.ParseIP(trimmed); ip != nil {
		return ip.String(), nil
	}

	if strings.Contains(trimmed, ":") {
		return "", fmt.Errorf("endpoint %q contains an unparseable host:port", endpoint)
	}

	return trimmed, nil
}

func (b *LocalBootstrapper) DeployRGWGateway() error {
	realm := &rookcephv1.CephObjectRealm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rgwRealmName,
			Namespace: rookNamespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, realm, func() error {
		realm.Spec = rookcephv1.ObjectRealmSpec{
			DefaultRealm: true,
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update CephObjectRealm %q: %w", rgwRealmName, err)
	}

	zoneGroup := &rookcephv1.CephObjectZoneGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rgwZoneGroupName,
			Namespace: rookNamespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, zoneGroup, func() error {
		zoneGroup.Spec = rookcephv1.ObjectZoneGroupSpec{
			Realm: rgwRealmName,
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update CephObjectZoneGroup %q: %w", rgwZoneGroupName, err)
	}

	zone := &rookcephv1.CephObjectZone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rgwZoneName,
			Namespace: rookNamespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, zone, func() error {
		zone.Spec = rookcephv1.ObjectZoneSpec{
			ZoneGroup: rgwZoneGroupName,
			MetadataPool: rookcephv1.PoolSpec{
				Replicated: rookcephv1.ReplicatedSpec{
					Size:                   1,
					RequireSafeReplicaSize: false,
				},
				Application: "rgw",
			},
			DataPool: rookcephv1.PoolSpec{
				Replicated: rookcephv1.ReplicatedSpec{
					Size:                   1,
					RequireSafeReplicaSize: false,
				},
				Application: "rgw",
			},
			PreservePoolsOnDelete: true,
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update CephObjectZone %q: %w", rgwZoneName, err)
	}

	store := &rookcephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rgwObjectStoreName,
			Namespace: rookNamespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, store, func() error {
		store.Spec = rookcephv1.ObjectStoreSpec{
			MetadataPool: rookcephv1.PoolSpec{
				Replicated: rookcephv1.ReplicatedSpec{
					Size:                   1,
					RequireSafeReplicaSize: false,
				},
				Application: "rgw",
			},
			DataPool: rookcephv1.PoolSpec{
				Replicated: rookcephv1.ReplicatedSpec{
					Size:                   1,
					RequireSafeReplicaSize: false,
				},
				Application: "rgw",
			},
			PreservePoolsOnDelete: true,
			Gateway: rookcephv1.GatewaySpec{
				Instances: 1,
				Port:      80,
			},
			Zone: rookcephv1.ZoneSpec{
				Name: rgwZoneName,
			},
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update CephObjectStore %q: %w", rgwObjectStoreName, err)
	}

	return b.waitForCephObjectStoreReady(rgwObjectStoreName)
}

// EnsureRGWAdminUser creates the RGW admin-ops user used by the S3 backend.
//
// We intentionally do not use the CephObjectStoreUser CRD here. While that CRD
// can provision regular object-store users with capabilities, in practice it
// does not yield credentials that can successfully call the RGW Admin Ops API
// endpoints used by Codesphere, such as /admin/ratelimit. We keep the original
// behavior of the private cloud installer and pass
// explicit monitor and admin-auth settings to avoid relying on pod-local Ceph
// config files or keyrings.
func (b *LocalBootstrapper) EnsureRGWAdminUser() (*RGWUserCredentials, error) {
	b.stlog.Logf("Creating or reconciling RGW admin user %q via radosgw-admin", rgwAdminUserName)

	createArgs := []string{
		"user", "create",
		"--uid", rgwAdminUserName,
		"--display-name", rgwAdminUserName,
		"--caps", rgwAdminUserCaps,
		"--rgw-realm", rgwRealmName,
		"--rgw-zonegroup", rgwZoneGroupName,
		"--rgw-zone", rgwZoneName,
		"--format", "json",
	}
	createArgs, err := b.withCephMonitorArgs(createArgs)
	if err != nil {
		return nil, err
	}
	stdout, stderr, err := b.execRadosGWAdmin(createArgs)
	if err != nil {
		errorText := strings.ToLower(stderr + "\n" + err.Error())
		if !strings.Contains(errorText, "exist") {
			return nil, fmt.Errorf("failed to create RGW admin user %q: %w: %s", rgwAdminUserName, err, strings.TrimSpace(stderr))
		}
		b.stlog.Logf("RGW admin user %q already exists, reading credentials", rgwAdminUserName)
	} else {
		creds, parseErr := rgwUserCredentialsFromAdminJSON(stdout)
		if parseErr == nil {
			return creds, nil
		}
		b.stlog.Logf("Failed to parse RGW admin create output for %q, falling back to user info: %v", rgwAdminUserName, parseErr)
	}

	infoArgs := []string{
		"user", "info",
		"--uid", rgwAdminUserName,
		"--rgw-realm", rgwRealmName,
		"--rgw-zonegroup", rgwZoneGroupName,
		"--rgw-zone", rgwZoneName,
		"--format", "json",
	}
	infoArgs, err = b.withCephMonitorArgs(infoArgs)
	if err != nil {
		return nil, err
	}
	stdout, stderr, err = b.execRadosGWAdmin(infoArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to read RGW admin user %q: %w: %s", rgwAdminUserName, err, strings.TrimSpace(stderr))
	}

	creds, err := rgwUserCredentialsFromAdminJSON(stdout)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RGW admin user %q info output: %w", rgwAdminUserName, err)
	}
	return creds, nil
}

func (b *LocalBootstrapper) withCephMonitorArgs(args []string) ([]string, error) {
	monHosts, err := b.readCephMonitorHosts()
	if err != nil {
		return nil, err
	}
	adminUser, adminSecret, err := b.readCephAdminAuth()
	if err != nil {
		return nil, err
	}
	return append([]string{
		"--mon-host", monHosts,
		"--no-mon-config",
		"--name", adminUser,
		"--key", adminSecret,
	}, args...), nil
}

func (b *LocalBootstrapper) readCephMonitorHosts() (string, error) {
	cm := &corev1.ConfigMap{}
	key := client.ObjectKey{Name: cephMonEndpointsConfigMap, Namespace: rookNamespace}
	if err := b.kubeClient.Get(b.ctx, key, cm); err != nil {
		return "", fmt.Errorf("failed to get Ceph monitor endpoints ConfigMap %q: %w", cephMonEndpointsConfigMap, err)
	}

	rawEndpoints := strings.TrimSpace(cm.Data["data"])
	if rawEndpoints == "" {
		return "", fmt.Errorf("ceph monitor endpoints ConfigMap %q does not contain data", cephMonEndpointsConfigMap)
	}

	var monHosts []string
	seen := map[string]struct{}{}
	for _, entry := range splitMonitorEndpointEntries(rawEndpoints) {
		monHost, err := parseMonitorEndpointHost(entry)
		if err != nil {
			b.stlog.Logf("Skipping invalid Ceph monitor endpoint entry %q: %v", entry, err)
			continue
		}
		if _, ok := seen[monHost]; ok {
			continue
		}
		seen[monHost] = struct{}{}
		monHosts = append(monHosts, monHost)
	}

	if len(monHosts) == 0 {
		return "", fmt.Errorf("ceph monitor endpoints ConfigMap %q does not contain any valid monitor addresses", cephMonEndpointsConfigMap)
	}

	return strings.Join(monHosts, ","), nil
}

func (b *LocalBootstrapper) readCephAdminAuth() (string, string, error) {
	secret := &corev1.Secret{}
	key := client.ObjectKey{Name: cephMonSecretName, Namespace: rookNamespace}
	if err := b.kubeClient.Get(b.ctx, key, secret); err != nil {
		return "", "", fmt.Errorf("failed to get Ceph monitor secret %q: %w", cephMonSecretName, err)
	}

	username, err := getSecretDataValue(secret, "ceph-username")
	if err != nil {
		return "", "", fmt.Errorf("failed to read Ceph admin username from secret %q: %w", cephMonSecretName, err)
	}

	cephSecret, err := getSecretDataValue(secret, "ceph-secret", "admin-secret", "mon-secret")
	if err != nil {
		return "", "", fmt.Errorf("failed to read Ceph admin secret from secret %q: %w", cephMonSecretName, err)
	}

	return username, cephSecret, nil
}

func (b *LocalBootstrapper) waitForRGWPod() (*corev1.Pod, error) {
	ctx, cancel := context.WithTimeout(b.ctx, cephObjectUserReadyTimeout)
	defer cancel()

	steps := int(cephObjectUserReadyTimeout / cephReadyPollInterval)
	if steps < 1 {
		steps = 1
	}

	backoff := wait.Backoff{
		Duration: cephReadyPollInterval,
		Factor:   1.0,
		Jitter:   0.1,
		Steps:    steps,
	}

	var pod *corev1.Pod

	err := retry.OnError(backoff, isRetryableWaitError, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}

		currentPod, err := b.getRGWPod()
		if err != nil {
			if isRetryableWaitError(err) {
				return err
			}
			return &retryableWaitError{err: err}
		}
		pod = currentPod
		return nil
	})
	if err == nil {
		return pod, nil
	}

	if isRetryableWaitError(err) {
		return nil, fmt.Errorf("timed out waiting for an RGW pod for object store %q", rgwObjectStoreName)
	}

	return nil, fmt.Errorf("failed waiting for an RGW pod for object store %q: %w", rgwObjectStoreName, err)
}

func (b *LocalBootstrapper) getRGWPod() (*corev1.Pod, error) {
	serviceName := "rook-ceph-rgw-" + rgwObjectStoreName
	service := &corev1.Service{}
	if err := b.kubeClient.Get(b.ctx, client.ObjectKey{Name: serviceName, Namespace: rookNamespace}, service); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, &retryableWaitError{err: fmt.Errorf("RGW service %q not found yet", serviceName)}
		}
		return nil, fmt.Errorf("failed to get RGW service %q: %w", serviceName, err)
	}

	if len(service.Spec.Selector) == 0 {
		return nil, &retryableWaitError{err: fmt.Errorf("RGW service %q does not have selectors yet", serviceName)}
	}

	pods := &corev1.PodList{}
	if err := b.kubeClient.List(b.ctx, pods, client.InNamespace(rookNamespace), client.MatchingLabels(service.Spec.Selector)); err != nil {
		return nil, fmt.Errorf("failed to list RGW pods for service %q: %w", serviceName, err)
	}

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if len(pod.Spec.Containers) == 0 {
			continue
		}
		return pod, nil
	}

	return nil, &retryableWaitError{err: fmt.Errorf("no running RGW pod found for service %q", serviceName)}
}

func (b *LocalBootstrapper) execRadosGWAdmin(args []string) (string, string, error) {
	pod, err := b.waitForRGWPod()
	if err != nil {
		return "", "", err
	}

	command := append([]string{"radosgw-admin"}, args...)
	return b.execInPod(pod.Namespace, pod.Name, pod.Spec.Containers[0].Name, command)
}

func (b *LocalBootstrapper) execInPod(namespace, podName, containerName string, command []string) (string, string, error) {
	clientset, err := kubernetes.NewForConfig(b.restConfig)
	if err != nil {
		return "", "", fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec")

	req.VersionedParams(&corev1.PodExecOptions{
		Container: containerName,
		Command:   command,
		Stdout:    true,
		Stderr:    true,
	}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(b.restConfig, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("failed to create pod exec executor for %s/%s: %w", namespace, podName, err)
	}

	var stdout, stderr bytes.Buffer
	err = executor.StreamWithContext(b.ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	return stdout.String(), stderr.String(), err
}

func rgwUserCredentialsFromAdminJSON(raw string) (*RGWUserCredentials, error) {
	type rgwAdminKey struct {
		AccessKey string `json:"access_key"`
		SecretKey string `json:"secret_key"`
	}
	type rgwAdminUserInfo struct {
		Keys []rgwAdminKey `json:"keys"`
	}

	var info rgwAdminUserInfo
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		return nil, fmt.Errorf("failed to unmarshal RGW admin JSON: %w", err)
	}
	if len(info.Keys) == 0 {
		return nil, fmt.Errorf("RGW admin JSON does not contain any keys")
	}
	if info.Keys[0].AccessKey == "" || info.Keys[0].SecretKey == "" {
		return nil, fmt.Errorf("RGW admin JSON does not contain a complete access/secret key pair")
	}

	return &RGWUserCredentials{
		AccessKey: info.Keys[0].AccessKey,
		SecretKey: info.Keys[0].SecretKey,
	}, nil
}

func rgwUserCredentialsFromSecret(secret *corev1.Secret) (*RGWUserCredentials, error) {
	accessKey, err := getSecretDataValue(secret, "AccessKey", "accessKey", "AWS_ACCESS_KEY_ID")
	if err != nil {
		return nil, err
	}

	secretKey, err := getSecretDataValue(secret, "SecretKey", "secretKey", "AWS_SECRET_ACCESS_KEY")
	if err != nil {
		return nil, err
	}

	return &RGWUserCredentials{
		AccessKey: accessKey,
		SecretKey: secretKey,
	}, nil
}

func getSecretDataValue(secret *corev1.Secret, keys ...string) (string, error) {
	for _, key := range keys {
		if value, ok := secret.Data[key]; ok {
			return string(value), nil
		}
	}

	return "", fmt.Errorf("secret %q does not contain any of the expected keys %q", secret.Name, strings.Join(keys, ", "))
}

func splitMonitorEndpointEntries(rawEndpoints string) []string {
	entries := []string{}
	var current strings.Builder
	bracketDepth := 0

	for _, r := range rawEndpoints {
		switch r {
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case ',':
			if bracketDepth == 0 {
				entries = append(entries, current.String())
				current.Reset()
				continue
			}
		}

		current.WriteRune(r)
	}

	if current.Len() > 0 {
		entries = append(entries, current.String())
	}

	return entries
}

func (b *LocalBootstrapper) waitForCephObjectStoreReady(name string) error {
	ctx, cancel := context.WithTimeout(b.ctx, cephObjectStoreReadyTimeout)
	defer cancel()

	storeKey := client.ObjectKey{Name: name, Namespace: rookNamespace}
	steps := int(cephObjectStoreReadyTimeout / cephReadyPollInterval)
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

		store := &rookcephv1.CephObjectStore{}
		if err := b.kubeClient.Get(ctx, storeKey, store); err != nil {
			if apierrors.IsNotFound(err) {
				return &retryableWaitError{err: fmt.Errorf("CephObjectStore %q not found yet", name)}
			}
			return err
		}

		if store.Status != nil {
			lastPhase = string(store.Status.Phase)
			if store.Status.Phase == rookcephv1.ConditionReady {
				return nil
			}
		}

		return &retryableWaitError{err: fmt.Errorf("CephObjectStore %q is not ready yet (phase=%q)", name, lastPhase)}
	})
	if err == nil {
		return nil
	}

	if isRetryableWaitError(err) {
		return fmt.Errorf("timed out waiting for CephObjectStore %q to become ready (phase=%q)", name, lastPhase)
	}

	return fmt.Errorf("failed waiting for CephObjectStore %q: %w", name, err)
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
