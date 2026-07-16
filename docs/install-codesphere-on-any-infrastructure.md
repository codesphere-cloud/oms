# Prepare infrastructure and install Codesphere

This guide describes how to prepare infrastructure for Codesphere without depending on a specific cloud provider.

> Choose the POC or production topology below, then increase capacity for the expected users, workspaces, storage consumption, and failure tolerance.

## What the installation builds

The installation uses a management host as the single entry point into a private network. From there, OMS installs and configures PostgreSQL, Ceph, k0s, and the Codesphere platform.

```text
administrator
    |
    | SSH
    v
jumpbox
    |
    +-- PostgreSQL node
    +-- 3 or more Ceph nodes
    `-- 3 or more k0s nodes

Internet
    |
    +-- platform gateway IP       -> cs.<base-domain> and *.cs.<base-domain>
    +-- workspace gateway IP      -> ws.<base-domain> and *.ws.<base-domain>
    `-- workspace SSH proxy IP    -> *.ssh.cs.<base-domain>
```

The installer can manage PostgreSQL, Ceph, and Kubernetes on these machines. An external PostgreSQL database or an existing Kubernetes cluster can also be described in the install configuration, but those alternatives are outside the topology covered by this guide.

## 1. Collect the required inputs

Decide on the following before provisioning anything:

- A base domain whose DNS zone you can change.
- A datacenter ID, name, city, and country code.
- Stable private IP addresses for every machine.
- A public IP address for the jumpbox. This is separate from the service addresses and should accept SSH only from trusted administrator networks.
- Three stable, externally reachable service IP addresses: platform gateway, workspace gateway, and workspace SSH proxy.
- GitHub Container Registry credentials. Codesphere nodes pull images directly from `ghcr.io`, so provide a GitHub username and a personal access token with at least `read:packages` access to the Codesphere packages.
- Access to the Codesphere package portal.
- An initial cluster administrator email, if desired.
- OAuth credentials for each Git provider or OIDC provider that users will use.
- An SSH key with administrative access to all hosts.

Git provider credentials must be supplied as complete sets. For example, a GitHub App needs its app name, client ID, and client secret; an OIDC provider needs its issuer URL, client ID, and client secret.

## 2. Provision the machines

Provision x86-64 machines running Ubuntu 22.04 LTS or an equivalent supported operating system. Choose a topology based on the purpose of the installation.

### POC topology

Use this compact topology for proofs of concept, evaluation, and other non-production environments:

| Role | Count | Per machine | Storage | Public IP |
| --- | ---: | --- | --- | --- |
| Jumpbox | 1 | 2 vCPU, 4 GiB RAM | 50 GiB SSD root disk | Yes |
| PostgreSQL | 1 | 2 vCPU, 8 GiB RAM | 200 GiB SSD root disk | Not required |
| Ceph | **3** | 8 vCPU, 32 GiB RAM | 50 GiB SSD root, one 10 GiB DB/WAL disk, and one 250 GiB data disk | No |
| Combined k0s control plane and worker | **3** | 8 vCPU, 32 GiB RAM | 200 GiB SSD root disk | No |

All three k0s nodes participate in the control plane and also provide workload capacity. This topology keeps the machine count low, but it does not provide the same maintenance and failure headroom as the production topology. Do not use it for production workloads.

### Production baseline

Use separate control-plane and worker pools in production. Start with:

| Role | Starting count | Per machine | Storage | Public IP |
| --- | ---: | --- | --- | --- |
| Jumpbox | 1 | 2 vCPU, 4 GiB RAM | 50 GiB SSD root disk | Yes |
| PostgreSQL primary | 1 | 2 vCPU, 8 GiB RAM | 200 GiB SSD root disk | No |
| PostgreSQL replica | 1 recommended | 2 vCPU, 8 GiB RAM | 200 GiB SSD root disk | No |
| Ceph | **4 or more** | 16 vCPU, 64 GiB RAM | 50 GiB SSD root, one 10 GiB DB/WAL disk, and one or more 500 GiB data disks | No |
| Dedicated k0s control plane | **3** | 4 vCPU, 8 GiB RAM | 50 GiB SSD root disk | No |
| Dedicated k0s worker | **3 or more** | 8 vCPU, 32 GiB RAM | 200 GiB SSD root disk | No |

Three control-plane nodes provide control-plane quorum and high availability. Scale the separate worker pool for workload CPU, memory, scheduling headroom, and the number of simultaneous node failures the platform must tolerate. Scale Ceph beyond four nodes and add data disks based on usable-capacity targets, replication overhead, recovery headroom, and growth.

Use 250 GiB Ceph data disks as the POC minimum and 500 GiB data disks as the production minimum. Present Ceph DB/WAL and data disks as distinct, unused block devices; do not format or mount them. For production, validate disk performance and size the DB/WAL device for the selected data devices and workload.

Use dedicated PostgreSQL storage and a replica when database availability is required. The POC topology keeps PostgreSQL on its root disk and has no replica.

Record the hostname and private IP of every host. Hostnames must be unique and resolvable consistently, either through internal DNS or `/etc/hosts`.

## 3. Create the private network

Place all machines on one private routed network. For example, use `10.10.0.0/20`. Any non-overlapping private CIDR is suitable if the same CIDR is recorded as `ceph.nodesSubnet` in the install configuration.

Provide unrestricted communication between Codesphere hosts on the private network. Kubernetes, Ceph, PostgreSQL, container registry, and SSH traffic all cross this network. If internal firewalls must be restrictive, derive and test an explicit port matrix for the selected Kubernetes and Ceph versions before installation.

Hosts without public addresses need outbound access through NAT or an HTTP proxy. They must be able to reach package repositories, the selected container registry, certificate endpoints, and any source URLs referenced by the installer.

Avoid CIDR overlap between the host network, Kubernetes pod network, Kubernetes service network, connected corporate networks, and VPNs.

## 4. Configure external access and firewalls

Reserve three stable external addresses and connect them to Kubernetes `LoadBalancer` services using the infrastructure's load-balancer implementation:

1. The platform gateway serves HTTP and HTTPS.
2. The public workspace gateway serves workspace HTTP and HTTPS traffic.
3. The workspace SSH proxy serves SSH traffic for workspaces.

On a public cloud, install and configure that provider's Kubernetes cloud controller or load-balancer integration. On bare metal or other infrastructure without a native implementation, configure MetalLB with an address pool containing the reserved addresses. The addresses must remain stable across service and node restarts.

Apply these boundary firewall rules:

| Source | Destination | Ports | Purpose |
| --- | --- | --- | --- |
| Trusted administrator networks | Jumpbox | TCP 22 | Administration |
| Internet | Platform gateway | TCP 80, 443 | Codesphere UI and API; ACME HTTP-01 when used |
| Internet | Workspace gateway | TCP 80, 443 | Hosted workspaces and custom domains |
| Internet | Workspace SSH proxy | TCP 22 | Workspace SSH |
| Codesphere private network | All Codesphere hosts | All required internal traffic | Kubernetes, Ceph, PostgreSQL, registry, and SSH |
| Codesphere hosts | Internet or approved proxies | Required outbound traffic | Packages, images, certificates, and integrations |

Do not expose SSH or PostgreSQL to `0.0.0.0/0`. PostgreSQL TCP 5432 normally needs to be reachable only from Codesphere hosts and approved administration or monitoring networks.

## 5. Establish administrative SSH access

Configure the jumpbox as the only public SSH entry point. The jumpbox must be able to connect as an administrative user to every private host, and the hosts must be able to connect to one another where required by Ceph and k0s installation.

The installation workflow uses direct root SSH and SSH agent forwarding. If organizational policy forbids root SSH, provide an equivalent privileged automation path and verify it with the installer before proceeding. Protect the private key, restrict ingress to trusted source ranges, and use host-key verification in a production workflow.

From the jumpbox, verify every host:

```bash
ssh root@<postgres-private-ip> hostname
ssh root@<ceph-private-ip> hostname
ssh root@<k0s-private-ip> hostname
```

## 6. Tune every installation host

Apply and persist the following kernel settings on the PostgreSQL, Ceph, and k0s hosts:

```bash
cat >/etc/sysctl.d/99-codesphere.conf <<'EOF'
fs.inotify.max_user_watches=1048576
fs.inotify.max_user_instances=8192
vm.max_map_count=262144
EOF

sysctl --system
```

Verify the active values:

```bash
sysctl fs.inotify.max_user_watches
sysctl fs.inotify.max_user_instances
sysctl vm.max_map_count
```

Keep time synchronized on every host. Confirm that hostnames, private addresses, attached disks, DNS resolution, outbound connectivity, and SSH access survive a reboot before installing Codesphere.

## 7. Prepare the jumpbox

Install OMS on the jumpbox together with `sops` and `age`. Pin and verify approved versions so the environment is repeatable.

The jumpbox must have enough free space for the installer package and extracted installation dependencies. Create a root-only secrets directory:

```bash
install -d -m 0700 /etc/codesphere/secrets
```

Installer packages are always browsed and downloaded from the Codesphere package portal. Set the portal API key in the environment of the user that will run OMS on the jumpbox:

```bash
export OMS_PORTAL_API_KEY='<portal-api-key>'
```

The API key must remain set for subsequent OMS portal operations. Supply it through an approved secret manager or protected session setup, and do not commit it to a shell profile, image, or source repository.

## 8. Prepare access to GitHub Container Registry

Codesphere images are pulled directly from GitHub Container Registry. Every k0s node must have outbound HTTPS access to `ghcr.io` and the related GitHub package endpoints.

Create or select a GitHub personal access token with at least the `read:packages` scope and access to the Codesphere packages. If the organization enforces SSO, authorize the token for that organization. Use a dedicated machine account where possible so image pulls do not depend on an employee account.

Keep the token ready for the configuration-generation step, but do not write it into shell history, an Ansible inventory, or `config.yaml`. Unauthenticated pulls or a token without package access will cause the platform installation to fail.

## 9. Generate the install configuration and secrets

The OMS command for configuration generation is `oms init install-config`. It creates both inputs required by the installer:

- `config.yaml` describes the infrastructure, network, storage, registry, domains, authentication providers, plans, and enabled features.
- `prod.vault.yaml` contains generated passwords, private keys, certificates, and provider credentials. It is plaintext when generated and must be encrypted before it is stored or copied through an untrusted system.

Run the interactive generator from a trusted workstation or the jumpbox:

```bash
oms init install-config \
  --profile production \
  --config config.yaml \
  --vault prod.vault.yaml \
  --with-comments
```

The profile supplies initial values and the wizard prompts for the final settings. Use `minimal` for the POC topology or `production` for the production baseline. The selected profile configures software defaults; it does not provision or resize machines.

An Ansible inventory can prepopulate the Kubernetes and Ceph host lists. For example:

```yaml
k8s-cp:
  hosts:
    k0s-1:
      private_ip: 10.10.0.11
    k0s-2:
      private_ip: 10.10.0.12
    k0s-3:
      private_ip: 10.10.0.13
k8s-workers:
  hosts:
    k0s-4:
      private_ip: 10.10.0.14
    k0s-5:
      private_ip: 10.10.0.15
    k0s-6:
      private_ip: 10.10.0.16
ceph:
  hosts:
    ceph-1:
      private_ip: 10.10.0.21
    ceph-2:
      private_ip: 10.10.0.22
    ceph-3:
      private_ip: 10.10.0.23
    ceph-4:
      private_ip: 10.10.0.24
```

Pass it to the generator and review the imported values in the wizard:

```bash
oms init install-config \
  --profile production \
  --config config.yaml \
  --vault prod.vault.yaml \
  --ansible-inventory inventory.yaml \
  --with-comments
```

The supported inventory groups are `k8s-cp`, `k8s-workers`, and `ceph`, with hosts represented as dictionary keys and each host containing `private_ip`.

Interactive mode is enabled by default. In that mode, profile and inventory values seed the wizard, while individual non-interactive configuration flags are not applied. For automation, pass `--interactive=false` and provide every required value through flags, the profile, and the inventory. Treat a non-interactive validation failure as a missing or inconsistent input rather than bypassing it.

After generation, set the registry section in `config.yaml` to:

```yaml
registry:
  server: ghcr.io
  replaceImagesInBom: false
  loadContainerImages: false
```

Set the existing registry entries in `prod.vault.yaml` to the GitHub username and personal access token:

```yaml
secrets:
  - name: registryUsername
    fields:
      password: <github-username>
  - name: registryPassword
    fields:
      password: <github-personal-access-token>
```

The username is intentionally stored in the `password` field used by the installer secret format. Keep both values in the vault and never place the token directly in `config.yaml`.

Review the generated files and ensure they describe the actual infrastructure:

- `dataCenter` contains the intended ID, name, city, and country code.
- `secrets.baseDir` is `/etc/codesphere/secrets` when following this layout.
- `postgres.primary` contains the PostgreSQL hostname and private IP, or `postgres.mode` describes the external database.
- `ceph.nodesSubnet` matches the private host network.
- `ceph.hosts` lists three hosts for a POC or at least four hosts for production, with exactly one initial master.
- `ceph.csiKubeletDir` is `/var/lib/k0s/kubelet` when Codesphere manages k0s.
- Each Ceph OSD definition selects only the intended, empty data and DB/WAL devices.
- `kubernetes.apiServerHost` is the first control-plane node's private IP, or a stable load-balancer address or DNS name when using multiple control-plane nodes.
- `kubernetes.controlPlanes` and `kubernetes.workers` assign every k0s node to its intended role. The production baseline uses three dedicated control-plane nodes and a separate worker pool:

  ```yaml
  kubernetes:
    managedByCodesphere: true
    apiServerHost: 10.10.0.11
    controlPlanes:
      - ipAddress: 10.10.0.11
      - ipAddress: 10.10.0.12
      - ipAddress: 10.10.0.13
    workers:
      - ipAddress: 10.10.0.14
      - ipAddress: 10.10.0.15
      - ipAddress: 10.10.0.16
  ```

  Always list at least three addresses under `controlPlanes` for a highly available control plane. Dedicated worker addresses go under `workers`; increase that list for the expected workload CPU, memory, and failure tolerance.

  The interactive `oms init install-config` wizard asks separately for the comma-separated control-plane and worker IPs. With an Ansible inventory, put controller hosts under `k8s-cp` and worker hosts under `k8s-workers`.

  k0s supports combined control-plane/worker nodes. However, the current OMS k0sctl configuration generator treats an IP present in both lists as control-plane-only and ignores the duplicate worker entry. Until combined-role generation is supported by OMS, use dedicated control-plane and worker entries in this workflow.
- The platform and public gateways use `LoadBalancer`, or `ExternalIP` where that is the chosen integration.
- `codesphere.domain` is `cs.<base-domain>`.
- `codesphere.workspaceHostingBaseDomain` and the custom-domain CNAME base are `ws.<base-domain>`.
- The workspace SSH proxy application is enabled and assigned its reserved address.
- ACME or another certificate issuer is configured for the selected DNS and load-balancer design.
- Git provider and OIDC redirect URLs use the final `https://cs.<base-domain>` address.
- The registry server is `ghcr.io`, `replaceImagesInBom` and `loadContainerImages` are `false`, and the vault contains valid `registryUsername` and `registryPassword` entries.

Use only annotations supported by the selected load-balancer implementation, or omit annotations when the implementation honors `loadBalancerIP` directly.

## 10. Encrypt and place the secrets

Generate an age identity on the jumpbox and make it readable only by root:

```bash
age-keygen -o /etc/codesphere/secrets/age_key.txt
chmod 0600 /etc/codesphere/secrets/age_key.txt
```

Copy the generated files to the jumpbox, then encrypt the vault there:

```bash
install -m 0600 config.yaml /etc/codesphere/config.yaml
install -m 0600 prod.vault.yaml /etc/codesphere/secrets/prod.vault.yaml

sops --encrypt --in-place \
  --age "$(age-keygen -y /etc/codesphere/secrets/age_key.txt)" \
  /etc/codesphere/secrets/prod.vault.yaml
```

Back up `config.yaml`, the encrypted vault, and the age identity to an approved secret store. The age identity is required to recover or update the installation. Never commit the plaintext vault or age identity to source control.

## 11. Configure DNS

Create these records after the three stable addresses are allocated:

| Record | Target |
| --- | --- |
| `cs.<base-domain>` | Platform gateway IP |
| `*.cs.<base-domain>` | Platform gateway IP |
| `ws.<base-domain>` | Workspace gateway IP |
| `*.ws.<base-domain>` | Workspace gateway IP |
| `*.ssh.cs.<base-domain>` | Workspace SSH proxy IP |

Use a short TTL such as 300 seconds during initial setup. Depending on the DNS provider and network design, the records may be A/AAAA, alias, or load-balancer records. Ensure the certificate solver can update DNS when using DNS-01, or that TCP 80 reaches the platform gateway when using HTTP-01.

Verify every record from outside the private network before continuing.

## 12. Obtain the installer package

Browse the available Codesphere packages and download the selected lite installer build on the jumpbox:

```bash
oms list packages
oms download package --version <version> --file installer-lite.tar.gz
```

`oms list packages` shows the versions and available builds visible to the configured portal account. If multiple builds share a version, pass the selected hash explicitly:

```bash
oms download package \
  --version <version> \
  --hash <hash> \
  --file installer-lite.tar.gz
```

Verify the artifact's provenance and checksum according to the release process before executing it.

## 13. Install Codesphere

Codesphere needs a secrets directory, but `oms install codesphere` does not have a separate `--secrets-dir` flag. Set the directory in `/etc/codesphere/config.yaml`:

```yaml
secrets:
  baseDir: /etc/codesphere/secrets
```

The configured directory should match the directory containing the file passed through `--vault`. Before installing, confirm that the directory and required files are present and readable only by root:

```bash
install -d -m 0700 /etc/codesphere/secrets
chmod 0600 /etc/codesphere/secrets/age_key.txt
chmod 0600 /etc/codesphere/secrets/prod.vault.yaml
test -r /etc/codesphere/secrets/age_key.txt
test -r /etc/codesphere/secrets/prod.vault.yaml
```

Run the installation from the jumpbox. The lite package does not contain the platform container images, so skip `load-container-images`; the cluster pulls them directly from GHCR using the credentials in the vault:

```bash
oms install codesphere \
  --config /etc/codesphere/config.yaml \
  --priv-key /etc/codesphere/secrets/age_key.txt \
  --vault /etc/codesphere/secrets/prod.vault.yaml \
  --package <downloaded-installer-lite-package>.tar.gz \
  --skip-steps load-container-images
```

The combined command installs in this order:

1. Copies and extracts dependencies.
2. Skips loading bundled container images because the nodes pull them from GHCR.
3. Installs SOPS and the container runtime dependencies.
4. Installs or configures PostgreSQL.
5. Installs and configures Ceph.
6. Installs and configures k0s when Kubernetes is Codesphere-managed.
7. Installs Argo CD and the Codesphere platform.

The installation can also be run as the separate `infra`, `dependencies`, and `platform` phases when operational change control requires distinct checkpoints.

## 14. Complete the infrastructure integration

If the selected infrastructure needs a Kubernetes cloud controller, install its supported provider integration and enable external cloud-provider mode on the k0s controller and workers. Use only manifests and service annotations intended for that infrastructure.

Confirm that the three services receive the reserved addresses:

```bash
/etc/codesphere/deps/kubernetes/files/k0s kubectl \
  get services --all-namespaces
```

The platform gateway, public workspace gateway, and workspace SSH proxy must retain their intended addresses. If the load-balancer implementation does not assign them from `config.yaml`, patch or annotate the services using that implementation's supported mechanism and then make the change persistent in the install configuration.

## 15. Verify the installation

Complete these checks before handing over the environment:

1. All k0s nodes are `Ready`.
2. Ceph reports healthy and all expected OSDs are present on all three POC hosts or at least four production hosts.
3. The three load-balancer services have the reserved external addresses.
4. `https://cs.<base-domain>` presents a trusted certificate and loads Codesphere.
5. A workspace can be created, reached over HTTPS, and reached through the workspace SSH proxy.

Run the Codesphere smoke test when credentials for the environment are available:

```bash
oms smoketest codesphere --help
```
