#!/bin/bash

set -ex

echo "--- GCP K8S Test Cluster Deployment Script ---"

# --- 1. Define Variables and Defaults ---

# PROJECT_ID (Required)
# Check if the environment variable is set and non-empty.
if [[ -z "${PROJECT_NAME}" ]]; then
  read -p "Enter desired unique GCP Project Name (e.g., my-k8s-test-001): " PROJECT_NAME
  if [[ -z "${PROJECT_NAME}" ]]; then
    echo "Error: Project ID cannot be empty. Exiting."
    exit 1
  fi
fi

# BILLING_ACCOUNT (Required)
if [[ -z "${BILLING_ACCOUNT}" ]]; then
  read -p "Enter GCP Billing Account ID (e.g., ABCDEF-123456-GHIJKL): " BILLING_ACCOUNT
  if [[ -z "${BILLING_ACCOUNT}" ]]; then
    echo "Error: Billing Account ID cannot be empty. Exiting."
    exit 1
  fi
fi

if [[ -z "${BASE_DOMAIN}" ]]; then
  read -p "Enter base domain (e.g. my-codesphere-domain.com): " BASE_DOMAIN
  if [[ -z "${BASE_DOMAIN}" ]]; then
    echo "Error: Billing Account ID cannot be empty. Exiting."
    exit 1
  fi
fi

echo "Using base domain: $BASE_DOMAIN"

# GITHUB_APP_CLIENT_ID (Required)
if [[ -z "${GITHUB_APP_CLIENT_ID}" ]]; then
  read -p "Enter Github App Client ID: " GITHUB_APP_CLIENT_ID
  if [[ -z "${GITHUB_APP_CLIENT_ID}" ]]; then
    echo "Error: GitHub App Client ID cannot be empty. Exiting."
    exit 1
  fi
fi
#
# GITHUB_APP_CLIENT_SECRET (Required)
if [[ -z "${GITHUB_APP_CLIENT_SECRET}" ]]; then
  read -p "Enter Github App Client SECRET: " GITHUB_APP_CLIENT_SECRET
  if [[ -z "${GITHUB_APP_CLIENT_SECRET}" ]]; then
    echo "Error: GitHub App Client SECRET cannot be empty. Exiting."
    exit 1
  fi
fi

SECRETSDIR="${SECRETSDIR:-/etc/codesphere/secrets}"

# FOLDER_ID (Optional) - If not set, it defaults to null/empty and is ignored by Terraform.
FOLDER_ID="${FOLDER_ID:-}" # Use the value if set, otherwise leave empty.

# SSH_KEY_PATH (Optional) - Default to the standard home directory path.
# The ${VAR:-default} syntax uses the value of VAR, or 'default' if VAR is unset or null.
SSH_KEY_PATH="${SSH_KEY_PATH:-~/.ssh/id_rsa.pub}"
if [ ! -f "$SSH_KEY_PATH" ]; then
  echo "Error: SSH Public Key not found at $SSH_KEY_PATH. Please create it or set the SSH_KEY_PATH environment variable."
  exit 1
fi

# SCHEDULING_TYPE (Optional) - Default to the cheaper SPOT (Preemptive) option.
SCHEDULING_TYPE="${SCHEDULING_TYPE:-SPOT}"

# --- Summary of Configuration ---
echo ""
echo "Configuration Summary:"
echo "  Project Name:        $PROJECT_NAME"
echo "  Billing Account:   $BILLING_ACCOUNT"
echo "  Folder ID:         ${FOLDER_ID:-[NOT SET]}"
echo "  SSH Key Path:      $SSH_KEY_PATH"
echo "  VM Scheduling:     $SCHEDULING_TYPE"
echo "  Base Domain:     $BASE_DOMAIN"
echo ""

# --- Helper Function for API Waiting ---
wait_for_apis() {
  local PROJECT_ID="$1"
  local APIS=("compute.googleapis.com" "serviceusage.googleapis.com" "artifactregistry.googleapis.com")
  
  echo "--- Starting API Status Check in project $PROJECT_ID ---"
  
  for API in "${APIS[@]}"; do
    echo "Checking $API..."
    for i in {1..20}; do
      # Use gcloud services describe and filter for state
      STATUS=$(gcloud services list --enabled --project="$PROJECT_ID" | grep "$API" > /dev/null; echo $?)
      
      if [[ "$STATUS" == "0" ]]; then
        echo "✅ $API is enabled."
        break
      fi
      
      if [[ $i -eq 20 ]]; then
        echo "Error: $API did not become fully ENABLED after 20 attempts."
        return 1
      fi
      
      echo "Waiting for $API... (Current status: $STATUS). Attempt $i of 20. Sleeping 10s."
      sleep 10
    done
  done
  echo "--- All APIs are ready! ---"
}


echo -e "\n--- Starting Step 1: Project Bootstrap ---"
cd 1-project-bootstrap

terraform init

if [[ "$FOLDER_ID" == "" ]]; then
  terraform apply -state="${PROJECT_NAME}.tfstate" -var="project_name=$PROJECT_NAME" -var="billing_account=$BILLING_ACCOUNT"
else
  terraform apply -state="${PROJECT_NAME}.tfstate" -var="project_name=$PROJECT_NAME" -var="billing_account=$BILLING_ACCOUNT" -var="folder_id=$FOLDER_ID"
fi

PROJECT_ID=$(terraform output -state="${PROJECT_NAME}.tfstate" -raw project_id)

echo "Project created with ID: $PROJECT_ID"

wait_for_apis "$PROJECT_ID"

echo -e "\n--- Starting Step 2: Infrastructure Deployment ---"
cd ../2-cluster-infra

terraform init

terraform apply -state="${PROJECT_NAME}.tfstate" -var="ssh_public_key_path=$SSH_KEY_PATH" -var="vm_scheduling_type=$SCHEDULING_TYPE"

echo -e "\n✅ Deployment Complete! Outputs:"
terraform output -state="${PROJECT_NAME}.tfstate"

JUMPBOX_IP=$(terraform output -state="${PROJECT_NAME}.tfstate" -json | yq -r .external_ips.value.jumpbox)

echo -n "Waiting for SSH service to start on $JUMPBOX_IP..."

# --- Loop to Check SSH Port ---
for i in $(seq 1 10); do
    if nc -vz "$JUMPBOX_IP" 22 2>/dev/null; then
        break
    fi

    echo -n "."
    sleep 5s
done

echo "ok"

# --- Final Check and Error Handling ---
if [ $i -gt 9 ]; then
    echo "❌ ERROR: Failed waiting for SSH on $JUMPBOX_IP"
    exit 1
fi

echo -e "\n--- Starting Step 3: Cluster Configuration ---"

echo "configuring ssh access"
cat <<EOF > $HOME/.ssh/ssh_config_pc
Host $JUMPBOX_IP
  User ubuntu
  StrictHostKeyChecking=no
  UserKnownHostsFile=/dev/null
  ForwardAgent yes
  SendEnv OMS_PORTAL_API_KEY

EOF

chmod 600 $HOME/.ssh/ssh_config_pc

if ! grep -qF "Include $HOME/.ssh/ssh_config_pc" ~/.ssh/config; then
    sed -i "1iInclude $HOME/.ssh/ssh_config_pc" ~/.ssh/config
fi

ssh $JUMPBOX_IP "sudo sed -i 's/no-port-forwarding.*$//g' /root/.ssh/authorized_keys;echo -e 'PermitRootLogin yes\nAcceptEnv *\n' | sudo tee /etc/ssh/sshd_config.d/51-cs-ssh-settings.conf; sudo systemctl restart sshd"

ALL_PRIVATE_IPS=($(terraform output -state="${PROJECT_NAME}.tfstate" -json internal_vm_ips | yq -r '.[]'))
for VM_IP in "${ALL_PRIVATE_IPS[@]}"; do
  echo "Configuring ssh access on VM $VM_IP"
  cat <<EOF >> ~/.ssh/ssh_config_pc
Host $VM_IP
  StrictHostKeyChecking=no
  UserKnownHostsFile=/dev/null
  ProxyJump $JUMPBOX_IP
  ForwardAgent yes

EOF
  ssh ubuntu@$VM_IP "sudo sed -i 's/no-port-forwarding.*$//g' /root/.ssh/authorized_keys;echo -e 'PermitRootLogin yes\nAcceptEnv *\n' | sudo tee /etc/ssh/sshd_config.d/51-cs-ssh-settings.conf; sudo systemctl restart sshd"
done
echo "Setting inotify limits"
# Example SSH command structure for configuration:
K0S_IPS=($(terraform output -state="${PROJECT_NAME}.tfstate" -json internal_vm_ips | yq -r '."k0s-cp-1", ."k0s-cp-2", ."k0s-cp-3"'))


for K0S_IP in "${K0S_IPS[@]}"; do
  echo "Configuring inotify limits on k0s node $K0S_IP..."

  ssh ubuntu@$K0S_IP 'echo "fs.inotify.max_user_watches=524288" | sudo tee -a /etc/sysctl.conf; echo "fs.inotify.max_user_instances=8192" | sudo tee -a /etc/sysctl.conf; sudo sysctl -p'
  echo "inotify limits set on $K0S_IP."
done

echo "Ceph"
if [[ ! -f ./ceph_id_rsa ]]; then
  ssh-keygen -t rsa -b 4096 -C "ceph" -f ./ceph_id_rsa
fi

# Generate CA Key
if [ ! -f "ca.key" ]; then
  openssl genrsa -out ca.key 2048
  openssl rsa -in ca.key -outform PEM -pubout -out ca-pub.pem
fi

echo "Ingress"
# Generate CA Certificate
if [ ! -f "ca.pem" ]; then
  openssl req -x509 -new -nodes -key ca.key -sha256 -days 1068 \
    -outform PEM -out ca.pem \
    -subj '/CN=MyOrg Root CA/C=DE/L=KA/O=MyOrg'
fi

if [ ! -f "ingress.csr" ]; then
  # Create Certificate Signing Request (CSR) and new key for ingress
  openssl req -new -nodes -out ingress.csr -newkey rsa:4096 -keyout ingress.key \
    -subj "/CN=cs.$BASE_DOMAIN/O=MyOrg"

  # Sign the CSR with your existing CA
  openssl x509 -req -in ingress.csr -CA ca.pem -CAkey ca.key -CAcreateserial \
    -outform PEM -out ingress.pem \
    -days 730 -sha256

fi
echo "Postgres"

PRIMARY_PG_IP=$(terraform output -state="${PROJECT_NAME}.tfstate" -json | yq -r .internal_vm_ips.value.postgres)

# Allow override of PRIMARY_PG_IP with custom value
if [[ -z "${CUSTOM_PG_IP}" ]]; then
  read -p "Enter custom PostgreSQL IP (leave empty to use default $PRIMARY_PG_IP): " CUSTOM_PG_IP
fi

if [[ -n "${CUSTOM_PG_IP}" ]]; then
  echo "Overriding PRIMARY_PG_IP from $PRIMARY_PG_IP to $CUSTOM_PG_IP"
  PRIMARY_PG_IP="$CUSTOM_PG_IP"
else
  echo "Using default PRIMARY_PG_IP: $PRIMARY_PG_IP"
fi

if [ ! -f "pg_primary.csr" ]; then
  # Create CSR for Primary
  openssl req -new -nodes -out pg_primary.csr -newkey rsa:4096 -keyout pg_primary.key \
    -subj "/CN=$PRIMARY_PG_IP/O=MyOrg"
fi

# Create extensions file (primary.v3.ext)
if [ ! -f "pg_primary.v3.ext" ]; then
  cat > pg_primary.v3.ext << EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment
subjectAltName = @alt_names
[alt_names]
IP.0 = $PRIMARY_PG_IP
DNS.0 = postgres
EOF
fi

if [ -f "pg_primary.csr" ]; then
  # Sign Primary Certificate
  openssl x509 -req -in pg_primary.csr -CA ca.pem -CAkey ca.key -CAcreateserial \
    -outform PEM -out pg_primary.pem \
    -days 730 -sha256 -extfile pg_primary.v3.ext
fi

if [ ! -f "domain_auth_key.pem" ]; then
  echo "domain auth keys"
  openssl ecparam -name prime256v1 -genkey -noout -out domain_auth_key.pem
  openssl ec -in domain_auth_key.pem -pubout -out domain_auth_public.pem
fi

echo "generate secrets"
cat << EOF > prod.vault.yaml
secrets:
  - name: cephSshPrivateKey
    file:
      # Content of 'ceph_id_rsa' generated in section 3.1.1
      name: id_rsa
      content: |
$(sed 's/^/        /' ceph_id_rsa)
  - name: selfSignedCaKeyPem
    file:
      name: key.pem
      content: |
$(sed 's/^/        /' ca.key)
  - name: domainAuthPrivateKey
    file:
      name: key.pem
      # Content of 'domain_auth_key.pem' from section 3.1.4
      content: |
$(sed 's/^/        /' domain_auth_key.pem)
  - name: domainAuthPublicKey
    file:
      name: key.pem
      content: |
$(sed 's/^/        /' domain_auth_public.pem)
  - name: registryUsername
    fields:
      password: '_json_key_base64'
  - name: registryPassword
    fields:
      password: $(terraform output -state="${PROJECT_NAME}.tfstate" -raw ar_key_base64)
  - name: postgresPassword
    fields:
      # Generate a strong primary admin password (e.g., 25 characters)
      # password: $(openssl rand -base64 16)
      password: 1qrEiR1GUwHB6UdxmJsexw==
  - name: postgresPrimaryServerKeyPem
    file:
      name: primary.key
      content: |
$(sed 's/^/        /' pg_primary.key)
  - name: postgresReplicaPassword
    fields:
      # password: $(openssl rand -base64 16)
      password: 8rgfMxUiDaPsVxZpRYjO7g==
  - name: postgresReplicaServerKeyPem
    file:
      name: replica.key
      content: |
$(sed 's/^/        /' pg_primary.key)
  - name: postgresUserAuth
    fields:
      password: auth_blue
  - name: postgresUserDeployment
    fields:
      password: deployment_blue
  - name: postgresUserIde
    fields:
      password: ide_blue
  - name: postgresUserMarketplace
    fields:
      password: marketplace_blue
  - name: postgresUserPayment
    fields:
      password: payment_blue
  - name: postgresUserPublicApi
    fields:
      password: public_api_blue
  - name: postgresUserTeam
    fields:
      password: team_blue
  - name: postgresUserWorkspace
    fields:
      password: workspace_blue
  - name: postgresPasswordAuth
    fields:
      password: $(openssl rand -base64 16)
  - name: postgresPasswordDeployment
    fields:
      password: $(openssl rand -base64 16)
  - name: postgresPasswordIde
    fields:
      password: $(openssl rand -base64 16)
  - name: postgresPasswordMarketplace
    fields:
      password: $(openssl rand -base64 16)
  - name: postgresPasswordPayment
    fields:
      password: $(openssl rand -base64 16)
  - name: postgresPasswordPublicApi
    fields:
      password: $(openssl rand -base64 16)
  - name: postgresPasswordTeam
    fields:
      password: $(openssl rand -base64 16)
  - name: postgresPasswordWorkspace
    fields:
      password: $(openssl rand -base64 16)

  - name: githubAppsClientId
    fields:
      password: '$GITHUB_APP_CLIENT_ID'
  - name: githubAppsClientSecret
    fields:
      password: '$GITHUB_APP_CLIENT_SECRET'
EOF

# Generate PostgreSQL configuration based on DATACENTER_ID
if [[ "$DATACENTER_ID" == "1" ]]; then
  POSTGRES_CONFIG="
  # Option 1: Install New PostgreSQL (Refer to secrets file for passwords & keys)
  # CA certificate for PostgreSQL (pg_ca.pem from section 3.1.3)
  caCertPem: |
$(sed 's/^/    /' ca.pem)
  primary:
    sslConfig:
      serverCertPem: |
$(sed 's/^/        /' pg_primary.pem)
    ip: $PRIMARY_PG_IP
    hostname: postgres"
else
  POSTGRES_CONFIG="
  # Option 2: Use External PostgreSQL
  # CA certificate for PostgreSQL (pg_ca.pem from section 3.1.3)
  caCertPem: |
$(sed 's/^/    /' ca.pem)
  serverAddress: $PRIMARY_PG_IP"
fi

echo "generate config.yaml"
cat << EOF > config.yaml
dataCenters:
  - id: 1
    name: main
    city: Karlsruhe # Your datacenter city
    countryCode: DE # Your datacenter country code
  - id: 2
    name: second
    city: Karlsruhe # Your datacenter city
    countryCode: DE # Your datacenter country code
currentDataCenterId: $DATACENTER_ID
secrets:
  baseDir: $SECRETSDIR # Path to your secrets directory (where prod.vault.yaml is)

registry:
  server: $(terraform output -state="${PROJECT_NAME}.tfstate" -raw artifact_registry_fqdn)
  replaceImagesInBom: true # Optional, should be set true if using an external registry
  loadContainerImages: true # Optional, set to true if images should be loaded from the installer bundle

postgres:$POSTGRES_CONFIG

ceph:
  csiKubeletDir: /var/lib/k0s/kubelet
  cephAdmSshKey:
    # Public key part of 'ceph_id_rsa.pub' generated in section 3.1.1
    publicKey: >-
$(sed 's/^/      /' ceph_id_rsa.pub)
  nodesSubnet: 10.10.0.0/20
  hosts:
    - hostname: ceph-1 
      ipAddress: $(terraform output -state="${PROJECT_NAME}.tfstate" -json | yq -r .internal_vm_ips.value.ceph-1)
      isMaster: true
    - hostname: ceph-2
      ipAddress: $(terraform output -state="${PROJECT_NAME}.tfstate" -json | yq -r .internal_vm_ips.value.ceph-2)
      isMaster: false
    - hostname: ceph-3 
      ipAddress: $(terraform output -state="${PROJECT_NAME}.tfstate" -json | yq -r .internal_vm_ips.value.ceph-3)
      isMaster: false
    - hostname: ceph-4 
      ipAddress: $(terraform output -state="${PROJECT_NAME}.tfstate" -json | yq -r .internal_vm_ips.value.ceph-4)
      isMaster: false
  osds:
  - specId: default
    placement:
      host_pattern: '*' # Apply to all hosts defined above
    dataDevices: # Devices for storing data
      size: '500G:' # Disks 300GB or larger
      limit: 1      # Use up to 2 such disks per host for data
    dbDevices:   # Devices for BlueStore internal metadata (DB/WAL)
      size: '50G:100G' # Disks between 100GB and 200GB
      limit: 1          # Use 1 such disk per host for metadata-DB

# --- Kubernetes Configuration ---
# Choose one: "Install New Kubernetes" OR "Use External Kubernetes"
# kubernetes.managedByCodesphere should be set accordingly
kubernetes:
  managedByCodesphere: true
  apiServerHost:  $(terraform output -state="${PROJECT_NAME}.tfstate" -json | yq -r .internal_vm_ips.value.k0s-cp-1)
  controlPlanes:
    - ipAddress:  $(terraform output -state="${PROJECT_NAME}.tfstate" -json | yq -r .internal_vm_ips.value.k0s-cp-1)
  workers:
    - ipAddress:  $(terraform output -state="${PROJECT_NAME}.tfstate" -json | yq -r .internal_vm_ips.value.k0s-cp-1)
    - ipAddress:  $(terraform output -state="${PROJECT_NAME}.tfstate" -json | yq -r .internal_vm_ips.value.k0s-cp-2)
    - ipAddress:  $(terraform output -state="${PROJECT_NAME}.tfstate" -json | yq -r .internal_vm_ips.value.k0s-cp-3)

cluster:
  certificates: # CA for services accessed by Codesphere users
    ca:
      algorithm: RSA
      keySizeBits: 2048
      # Content of 'ca.pem' (or your Ingress CA cert) from section 3.1.2
      certPem: |
$(sed 's/^/        /' ca.pem)
  monitoring: 
    prometheus:  
      remoteWrite:
        enabled: false
        clusterName: my-cluster-name  
  gateway: # For Codesphere internal services
    serviceType: "ClusterIP"
    ipAddresses: [$(terraform output -state="${PROJECT_NAME}.tfstate" -json | yq .external_ips.value.k0s-cp-1)]

    # annotations: # Optional: for cloud provider specific LB config
      # Example Azure:
      # service.beta.kubernetes.io/azure-load-balancer-ipv4: <IP>
      # service.beta.kubernetes.io/azure-load-balancer-resource-group: <rg>
  publicGateway: # For user workspaces
    serviceType: "ClusterIP"
    ipAddresses: [$(terraform output -state="${PROJECT_NAME}.tfstate" -json | yq .external_ips.value.k0s-cp-2)]
    # annotations: {}
metallb:
  # This is the primary switch to enable or disable the MetalLB integration.
  # Set to 'true' for MetalLB to be installed and configured.
  enabled: false

# --- Codesphere Application Configuration ---
codesphere:
  domain: "cs.$BASE_DOMAIN" # Main domain for Codesphere UI/API
  workspaceHostingBaseDomain: "ws.$BASE_DOMAIN"
  # A primary public IP for workspaces (use one of the publicGateway). If assigned by a LoadBalancer and not known yet, 
  # leave blank and add later once known.
  publicIp: $(terraform output -state="${PROJECT_NAME}.tfstate" --json | yq .external_ips.value.k0s-cp-1)
  customDomains:
    cNameBaseDomain: "ws.$BASE_DOMAIN"
  dnsServers: ["1.1.1.1", "8.8.8.8"]
  experiments: [] # List of Codesphere experimental features to enable
  extraCaPem: "" # Optional: PEM of an extra custom root CA to be trusted by Codesphere services/workspaces
  extraWorkspaceEnvVars: {}
  extraWorkspaceFiles: []
  deployConfig:
    images:
      ubuntu-24.04:
        name: 'Ubuntu 24.04'
        supportedUntil: '2028-05-31'
        flavors:
          default: 
            image: 
              bomRef: 'workspace-agent-24.04'
            pool:   
              1: 1 # Number of warm instances to keep pooled
  plans:
    hostingPlans:
      1:
        cpuTenth: 10       # 1 CPU core (10 tenths)
        gpuParts: 0        # GPU resources
        memoryMb: 2048     # 2 GB RAM
        storageMb: 20480   # 20 GB persistent storage
        tempStorageMb: 1024 # Temporary storage
    workspacePlans:
      1:
        name: "Standard Developer" # Display name of the plan
        hostingPlanId: 1        # Maps to an ID from hostingPlans
        maxReplicas: 3          # Max concurrent replicas for a workspace
        onDemand: true          # Allow on-demand (start/stop) workspaces
  gitProviders:
    github:
      enabled: true
      url: "https://github.com"
      api:
        baseUrl: "https://api.github.com"
      oauth:
        issuer: "https://github.com"
        authorizationEndpoint: "https://github.com/login/oauth/authorize"
        tokenEndpoint: "https://github.com/login/oauth/access_token"
  managedServices:
    - name: postgres
      api:
        endpoint: "http://ms-backend-postgres.postgres-operator:3000/api/v1/postgres"
      author: Codesphere
      category: Database
      configSchema:
        type: object
        properties:
          version:
            type: string
            enum:
              - '17.6'
              - '16.10'
            default: '17.6'
            readOnly: false
          superuserName:
            type: string
            default: postgres
          userName:
            type: string
            default: app
          databaseName:
            type: string
            default: app
      detailsSchema:
        type: object
        properties:
          port:
            type: integer
          host:
            type: string
          dsn:
            type: string
      secretsSchema:
        type: object
        properties:
          userPassword:
            type: string
            format: password
          superuserPassword:
            type: string
            format: password
      description: >-
        Open-source database system tailored for efficient data management and 
        scalability. Deployed on Codesphere using the CNPG K8s Operator. 
      displayName: PostgreSQL
      iconUrl: 'https://www.vectorlogo.zone/logos/postgresql/postgresql-icon.svg'
      plans:
        - id: 0
          description: 0.5 vCPU / 500 MB Memory
          name: Small
          parameters:
            storage:
              pricedAs: storage-mb
              schema:
                description: Storage (MB)
                type: integer
                default: 10000
                readOnly: false
            cpu:
              pricedAs: cpu-tenths
              schema:
                description: CPU Tenths
                type: number
                default: 5
                readOnly: true
            memory:
              pricedAs: ram-mb
              schema:
                description: Memory (MB)
                type: integer
                default: 500
                readOnly: true
        - id: 1
          description: 1 vCPU / 1 GB Memory
          name: Medium
          parameters:
            storage:
              pricedAs: storage-mb
              schema:
                description: Storage (MB)
                type: integer
                default: 25000
                readOnly: false
            cpu:
              pricedAs: cpu-tenths
              schema:
                description: CPU Tenths
                type: number
                default: 10
                readOnly: true
            memory:
              pricedAs: ram-mb
              schema:
                description: Memory (MB)
                type: integer
                default: 1000
                readOnly: true
      version: v1
managedServiceBackends:
  postgres: {}
EOF

if [[ ! -f age_key.txt ]]; then
  age-keygen -o age_key.txt
fi
sops --encrypt --age $(age-keygen -y age_key.txt) --in-place prod.vault.yaml

ssh -o StrictHostKeyChecking=no ubuntu@$JUMPBOX_IP  "sudo mkdir -p $SECRETSDIR; sudo chown ubuntu:ubuntu $SECRETSDIR"
#ssh -o StrictHostKeyChecking=no -J root@$JUMPBOX_IP root@$K0S_IP "echo export OMS_PORTAL_API_KEY=$OMS_PORTAL_API_KEY >> ~/.bashrc"
ssh -o StrictHostKeyChecking=no ubuntu@$JUMPBOX_IP "wget -qO- 'https://api.github.com/repos/codesphere-cloud/oms/releases/latest' | jq -r '.assets[] | select(.name | match(\"oms-cli.*linux_amd64\")) | .browser_download_url' | xargs wget -O oms-cli"
ssh -o StrictHostKeyChecking=no ubuntu@$JUMPBOX_IP "chmod +x oms-cli; sudo mv oms-cli /usr/local/bin/"
ssh -o StrictHostKeyChecking=no ubuntu@$JUMPBOX_IP "curl -LO https://github.com/getsops/sops/releases/download/v3.11.0/sops-v3.11.0.linux.amd64; sudo mv sops-v3.11.0.linux.amd64 /usr/local/bin/sops; sudo chmod +x /usr/local/bin/sops"
ssh -o StrictHostKeyChecking=no ubuntu@$JUMPBOX_IP "wget https://dl.filippo.io/age/latest?for=linux/amd64 -O age.tar.gz; tar -xvf age.tar.gz; sudo mv age/age /usr/local/bin/"

: ssh -n -o StrictHostKeyChecking=no root@$JUMPBOX_IP '[[ -f config.yaml ]] && rm config.yaml'
scp config.yaml root@$JUMPBOX_IP:$SECRETSDIR/
: ssh -n -o StrictHostKeyChecking=no root@$JUMPBOX_IP '[[ -f prod.vault.yaml ]] && rm prod.vault.yaml'
scp prod.vault.yaml root@$JUMPBOX_IP:$SECRETSDIR/
: ssh -n -o StrictHostKeyChecking=no root@$JUMPBOX_IP '[[ -f age_key.txt ]] && rm age_key.txt'
scp age_key.txt root@$JUMPBOX_IP:$SECRETSDIR/

echo "All resources are contained in project: $PROJECT_ID"
echo "To shut down and delete the project (and ALL its contents), run:"
echo "gcloud projects delete $PROJECT_ID"
echo "start the Codesphere installation using OMS from the jumpbox host: ssh-add $SSH_KEY_PATH; ssh -o StrictHostKeyChecking=no -o ForwardAgent=yes -o SendEnv=OMS_PORTAL_API_KEY root@$JUMPBOX_IP"

echo "Please ensure that the necessary DNS records are configured for the domain $BASE_DOMAIN:"
echo "- A record for 'cs.$BASE_DOMAIN' pointing to the gateway IP $(terraform output -state="${PROJECT_NAME}.tfstate" --json | yq .external_ips.value.k0s-cp-1)."
echo "- A record for '*.cs.$BASE_DOMAIN' pointing to the public gateway IP $(terraform output -state="${PROJECT_NAME}.tfstate" --json | yq .external_ips.value.k0s-cp-1)."
echo "- A record for '*.ws.$BASE_DOMAIN' pointing to the public gateway IP $(terraform output -state="${PROJECT_NAME}.tfstate" --json | yq .external_ips.value.k0s-cp-2)."