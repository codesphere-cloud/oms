// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

type Node struct {
	FileIO util.FileIO `json:"-"`
	// If connecting via the Jumpbox
	Jumpbox *Node `json:"-"`
	// Config
	keyPath      string     `json:"-"`
	Name         string     `json:"name"`
	ExternalIP   string     `json:"external_ip"`
	InternalIP   string     `json:"internal_ip"`
	cachedSigner ssh.Signer `json:"-"`
	sshQuiet     bool       `json:"-"`

	NodeClient NodeClient `json:"-"`
	// SSH client cache: map[username]*ssh.Client
	clientCache map[string]*ssh.Client
	clientMu    sync.Mutex
}

type NodeClient interface {
	RunCommand(n *Node, username string, command string) error
	CopyFile(n *Node, src string, dst string) error
	WaitReady(n *Node, timeout time.Duration) error
	HasFile(n *Node, filePath string) bool
}

type SSHNodeClient struct {
	Quiet bool
}

func NewSSHNodeClient(quiet bool) *SSHNodeClient {
	return &SSHNodeClient{
		Quiet: quiet,
	}
}

func (r *SSHNodeClient) RunCommand(n *Node, username string, command string) error {
	var jumpboxIp string
	var ip string
	if n.Jumpbox != nil {
		jumpboxIp = n.Jumpbox.ExternalIP
		ip = n.InternalIP
	} else {
		jumpboxIp = ""
		ip = n.ExternalIP
	}
	client, err := n.getOrCreateClient(jumpboxIp, ip, username)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}
	// Don't close the client - it's cached for reuse

	session, err := client.NewSession()
	if err != nil {
		// Connection might be stale, try to reconnect
		n.invalidateClient(username)
		client, err = n.getOrCreateClient(jumpboxIp, ip, username)
		if err != nil {
			return fmt.Errorf("failed to reconnect client: %w", err)
		}
		session, err = client.NewSession()
		if err != nil {
			return fmt.Errorf("failed to create session: %v", err)
		}
	}
	defer util.IgnoreError(session.Close)

	_ = session.Setenv("OMS_PORTAL_API_KEY", os.Getenv("OMS_PORTAL_API_KEY"))
	_ = agent.RequestAgentForwarding(session) // Best effort, ignore errors

	if !r.Quiet {
		session.Stdout = os.Stdout
		session.Stderr = os.Stderr
	}
	// Start the command
	if err := session.Start(command); err != nil {
		return fmt.Errorf("failed to start command: %v", err)
	}

	if err := session.Wait(); err != nil {
		// A non-zero exit status from the remote command is also considered an error
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}

const jumpboxUser = "ubuntu"

// NewNode creates a new Node with the given File
// CreateSubNode creates a Node object representing a node behind a jumpbox
func (n *Node) CreateSubNode(name string, externalIP string, internalIP string) *Node {
	return &Node{
		// Inherited from jumpbox
		FileIO:     n.FileIO,
		Jumpbox:    n,
		keyPath:    util.ExpandPath(n.keyPath),
		sshQuiet:   n.sshQuiet,
		NodeClient: n.NodeClient,

		// Custom
		Name:        name,
		ExternalIP:  externalIP,
		InternalIP:  internalIP,
		clientCache: make(map[string]*ssh.Client),
	}
}

// UpdateNode updates the node's name and IP addresses
func (n *Node) UpdateNode(name string, externalIP string, internalIP string) {
	n.Name = name
	n.ExternalIP = externalIP
	n.InternalIP = internalIP
}

// GetExternalIP returns the external IP of the node
func (n *Node) GetExternalIP() string {
	return n.ExternalIP
}

// GetInternalIP returns the internal IP of the node
func (n *Node) GetInternalIP() string {
	return n.InternalIP
}

// GetName returns the name of the node
func (n *Node) GetName() string {
	return n.Name
}

// getHostKeyCallback returns a host key callback that uses known_hosts file
// and auto-accepts new hosts for provisioning.
func (n *Node) getHostKeyCallback() (ssh.HostKeyCallback, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	knownHostsPath := filepath.Join(homeDir, ".ssh", "known_hosts")

	// Ensure known_hosts file exists
	sshDir := filepath.Join(homeDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	// Create file if it doesn't exist
	if _, err := os.Stat(knownHostsPath); os.IsNotExist(err) {
		f, err := os.OpenFile(knownHostsPath, os.O_CREATE|os.O_RDONLY, 0600)
		if err != nil {
			return nil, fmt.Errorf("failed to create known_hosts file: %w", err)
		}
		if err := f.Close(); err != nil {
			return nil, fmt.Errorf("failed to close known_hosts file: %w", err)
		}
	}

	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load known_hosts: %w", err)
	}

	// Wrap the callback to auto-accept new hosts for provisioning
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := hostKeyCallback(hostname, remote, key)
		if err == nil {
			// Host key is already known and valid
			return nil
		}

		var keyErr *knownhosts.KeyError
		if errors, ok := err.(*knownhosts.KeyError); ok {
			keyErr = errors
		}

		if keyErr != nil && len(keyErr.Want) == 0 {
			log.Printf("Warning: Adding new host %s to known_hosts (first connection)", hostname)

			// Append the new host key to known_hosts
			f, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				return fmt.Errorf("failed to open known_hosts for writing: %w", err)
			}
			defer func() {
				if err := f.Close(); err != nil {
					log.Printf("Warning: failed to close known_hosts file: %v", err)
				}
			}()

			// Format: hostname ssh-keytype base64-encoded-key
			normalizedHosts := []string{hostname}
			if host, port, splitErr := net.SplitHostPort(hostname); splitErr == nil {
				normalizedHosts = []string{net.JoinHostPort(host, port)}
			}
			line := knownhosts.Line(normalizedHosts, key)
			if _, err := f.WriteString(line + "\n"); err != nil {
				return fmt.Errorf("failed to write to known_hosts: %w", err)
			}

			return nil
		}

		return fmt.Errorf("host key verification failed: %w", err)
	}, nil
}

func (c *SSHNodeClient) WaitReady(node *Node, timeout time.Duration) error {
	start := time.Now()
	jumpboxIp := ""
	nodeIp := node.ExternalIP
	if node.Jumpbox != nil {
		jumpboxIp = node.Jumpbox.ExternalIP
		nodeIp = node.InternalIP
	}
	for {
		// Try to get or create a cached client
		_, err := node.getOrCreateClient(jumpboxIp, nodeIp, jumpboxUser)
		if err == nil {
			// Connection successful and cached
			return nil
		}
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout: %w", err)
		}
		time.Sleep(5 * time.Second)
	}
}

// RunSSHCommand connects to the node, executes a command and streams the output.
// If quiet is true, command output is not printed to stdout/stderr.
// The SSH client connection is cached and reused for subsequent commands.
func (n *Node) RunSSHCommand(username string, command string) error {
	return n.NodeClient.RunCommand(n, username, command)
}

// HasCommand checks if a command exists on the remote node via SSH
func (n *Node) HasCommand(command string) bool {
	checkCommand := fmt.Sprintf("command -v %s >/dev/null 2>&1", command)
	err := n.RunSSHCommand("root", checkCommand)
	if err != nil {
		// If the command returns a non-zero exit status, it means the command is not found
		return false
	}
	return true
}

// InstallOms installs the OMS CLI on the remote node via SSH
func (n *Node) InstallOms() error {
	remoteCommands := []string{
		"wget -qO- 'https://api.github.com/repos/codesphere-cloud/oms/releases/latest' | jq -r '.assets[] | select(.name | match(\"oms-cli.*linux_amd64\")) | .browser_download_url' | xargs wget -O oms-cli",
		"chmod +x oms-cli; sudo mv oms-cli /usr/local/bin/",
		"curl -LO https://github.com/getsops/sops/releases/download/v3.11.0/sops-v3.11.0.linux.amd64; sudo mv sops-v3.11.0.linux.amd64 /usr/local/bin/sops; sudo chmod +x /usr/local/bin/sops",
		"wget https://dl.filippo.io/age/latest?for=linux/amd64 -O age.tar.gz; tar -xvf age.tar.gz; sudo mv age/age* /usr/local/bin/",
	}
	for _, cmd := range remoteCommands {
		err := n.RunSSHCommand("root", cmd)
		if err != nil {
			return fmt.Errorf("failed to run remote command '%s': %w", cmd, err)
		}
	}
	return nil
}

// HasAcceptEnvConfigured checks if AcceptEnv is configured
func (n *Node) HasAcceptEnvConfigured() bool {
	checkCommand := "sudo grep -E '^AcceptEnv OMS_PORTAL_API_KEY' /etc/ssh/sshd_config >/dev/null 2>&1"
	err := n.RunSSHCommand("ubuntu", checkCommand)
	if err != nil {
		// If the command returns a NON-zero exit status, it means AcceptEnv is not configured
		return false
	}
	return true
}

// ConfigureAcceptEnv configures AcceptEnv for OMS_PORTAL_API_KEY
func (n *Node) ConfigureAcceptEnv() error {
	cmds := []string{
		"sudo sed -i 's/^#\\?AcceptEnv.*/AcceptEnv OMS_PORTAL_API_KEY/' /etc/ssh/sshd_config",
		"sudo systemctl restart sshd",
	}
	for _, cmd := range cmds {
		err := n.RunSSHCommand("ubuntu", cmd)
		if err != nil {
			return fmt.Errorf("failed to run command '%s': %w", cmd, err)
		}
	}
	return nil
}

// HasRootLoginEnabled checks if root login is enabled on the remote node via SSH
func (n *Node) HasRootLoginEnabled() bool {
	checkCommandPermit := "sudo grep -E '^PermitRootLogin yes' /etc/ssh/sshd_config >/dev/null 2>&1"
	err := n.RunSSHCommand("ubuntu", checkCommandPermit)
	if err != nil {
		// If the command returns a NON-zero exit status, it means root login is not permitted
		return false
	}
	checkCommandAuthorizedKeys := "sudo grep -E '^no-port-forwarding' /root/.ssh/authorized_keys >/dev/null 2>&1"
	err = n.RunSSHCommand("ubuntu", checkCommandAuthorizedKeys)
	if err == nil {
		// If the command returns a ZERO exit status, it means root login is prevented
		return false
	}
	return true
}

// EnableRootLogin enables root login on the remote node via SSH
func (n *Node) EnableRootLogin() error {
	cmds := []string{
		"sudo sed -i 's/^#\\?PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config",
		"sudo sed -i 's/no-port-forwarding.*$//g' /root/.ssh/authorized_keys",
		"sudo systemctl restart sshd",
	}
	for _, cmd := range cmds {
		err := n.RunSSHCommand("ubuntu", cmd)
		if err != nil {
			return fmt.Errorf("failed to run command '%s': %w", cmd, err)
		}
	}
	return nil
}

func (n *Node) HasInotifyWatchesConfigured() bool {
	return n.hasSysctlLine("fs.inotify.max_user_watches=1048576")
}

func (n *Node) ConfigureInotifyWatches() error {
	return n.configureSysctlLine("fs.inotify.max_user_watches=1048576")
}

func (n *Node) HasMemoryMapConfigured() bool {
	return n.hasSysctlLine("vm.max_map_count=262144")
}

func (n *Node) ConfigureMemoryMap() error {
	return n.configureSysctlLine("vm.max_map_count=262144")
}

// HasFile checks if a file exists on the remote node via SSH
func (c *SSHNodeClient) HasFile(n *Node, filePath string) bool {
	checkCommand := fmt.Sprintf("test -f '%s'", filePath)
	err := n.RunSSHCommand("ubuntu", checkCommand)
	if err != nil {
		// If the command returns a non-zero exit status, it means the file does not exist
		return false
	}
	return true
}

// CopyFile copies a file from the local system to the remote node via SFTP
func (c *SSHNodeClient) CopyFile(n *Node, src string, dst string) error {
	if n.Jumpbox == nil {
		err := n.ensureDirectoryExists("root", filepath.Dir(dst))
		if err != nil {
			return fmt.Errorf("failed to ensure directory exists: %w", err)
		}
		return n.copyFile("", n.ExternalIP, "root", src, dst)
	}
	err := n.ensureDirectoryExists("root", filepath.Dir(dst))
	if err != nil {
		return fmt.Errorf("failed to ensure directory exists: %w", err)
	}
	return n.copyFile(n.Jumpbox.ExternalIP, n.InternalIP, "root", src, dst)
}

// Helper functions

// hasSysctlLine checks if a specific line exists in /etc/sysctl.conf on the remote node via SSH
func (n *Node) hasSysctlLine(line string) bool {
	checkCommand := fmt.Sprintf("sudo grep -E '^%s' /etc/sysctl.conf >/dev/null 2>&1", line)
	err := n.RunSSHCommand("root", checkCommand)
	if err != nil {
		// If the command returns a NON-zero exit status, it means the setting is not configured
		return false
	}
	return true
}

// configureSysctlLine appends a specific line to /etc/sysctl.conf and applies the settings on the remote node via SSH
func (n *Node) configureSysctlLine(line string) error {
	cmds := []string{
		fmt.Sprintf("echo '%s' | sudo tee -a /etc/sysctl.conf", line),
		"sudo sysctl -p",
	}
	for _, cmd := range cmds {
		err := n.RunSSHCommand("root", cmd)
		if err != nil {
			return fmt.Errorf("failed to run command '%s': %w", cmd, err)
		}
	}
	return nil
}

// getOrCreateClient returns a cached SSH client or creates a new one if not cached.
func (n *Node) getOrCreateClient(jumpboxIp string, ip string, username string) (*ssh.Client, error) {
	if n.clientCache == nil {
		n.clientCache = make(map[string]*ssh.Client)
	}
	n.clientMu.Lock()
	defer n.clientMu.Unlock()

	if client, ok := n.clientCache[username]; ok {
		if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err == nil {
			return client, nil
		}
		util.IgnoreError(client.Close)
		delete(n.clientCache, username)
	}

	client, err := n.createClient(jumpboxIp, ip, username)
	if err != nil {
		return nil, err
	}

	// Set up agent forwarding (best effort, ignore errors)
	err = n.setupAgentForwarding(client)
	if err != nil {
		log.Printf("Warning: failed to set up agent forwarding: %v", err)
	}

	n.clientCache[username] = client
	return client, nil
}

// invalidateClient removes a cached client for the given username
func (n *Node) invalidateClient(username string) {
	n.clientMu.Lock()
	defer n.clientMu.Unlock()

	if client, ok := n.clientCache[username]; ok {
		util.IgnoreError(client.Close)
		delete(n.clientCache, username)
	}
}

// createClient creates and returns a new SSH client connected to the node (internal, no caching)
func (n *Node) createClient(jumpboxIp string, ip string, username string) (*ssh.Client, error) {
	authMethods, err := n.getAuthMethods()
	if err != nil {
		return nil, fmt.Errorf("failed to get authentication methods: %w", err)
	}

	hostKeyCallback, err := n.getHostKeyCallback()
	if err != nil {
		return nil, fmt.Errorf("failed to get host key callback: %w", err)
	}

	if jumpboxIp != "" {
		// Use the Jumpbox's cached client if available
		jbClient, err := n.Jumpbox.getOrCreateClient("", jumpboxIp, jumpboxUser)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to jumpbox: %v", err)
		}

		finalTargetConfig := &ssh.ClientConfig{
			User:            username,
			Auth:            authMethods,
			Timeout:         10 * time.Second,
			HostKeyCallback: hostKeyCallback,
		}

		finalAddr := fmt.Sprintf("%s:22", ip)
		jbConn, err := jbClient.Dial("tcp", finalAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to create connection through jumpbox: %v", err)
		}
		finalClient, channels, requests, err := ssh.NewClientConn(jbConn, finalAddr, finalTargetConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to perform SSH handshake through jumpbox: %v", err)
		}

		return ssh.NewClient(finalClient, channels, requests), nil
	}

	config := &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		Timeout:         10 * time.Second,
		HostKeyCallback: hostKeyCallback,
	}

	addr := fmt.Sprintf("%s:22", ip)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %v", err)
	}
	return client, nil
}

// getSFTPClient creates and returns an SFTP client connected to the node.
// Uses the cached SSH client for the connection.
func (n *Node) getSFTPClient(jumpboxIp string, ip string, username string) (*sftp.Client, error) {
	client, err := n.getOrCreateClient(jumpboxIp, ip, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get SSH client: %v", err)
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return nil, fmt.Errorf("failed to create SFTP client: %v", err)
	}

	return sftpClient, nil
}

// ensureDirectoryExists creates the directory on the remote node via SSH if it does not exist.
func (n *Node) ensureDirectoryExists(username string, dir string) error {
	cmd := fmt.Sprintf("mkdir -p '%s'", dir)
	return n.RunSSHCommand(username, cmd)
}

// copyFile copies a file from the local system to the remote node via SFTP.
func (n *Node) copyFile(jumpboxIp string, ip string, username string, src string, dst string) error {
	client, err := n.getSFTPClient(jumpboxIp, ip, username)
	if err != nil {
		return fmt.Errorf("failed to get SSH client: %v", err)
	}
	defer util.IgnoreError(client.Close)

	srcFile, err := n.FileIO.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %v", src, err)
	}
	defer util.IgnoreError(srcFile.Close)

	dstFile, err := client.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file %s: %v", dst, err)
	}
	defer util.IgnoreError(dstFile.Close)

	_, err = dstFile.ReadFrom(srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy data from %s to %s: %v", src, dst, err)
	}

	return nil
}

// getAuthMethods constructs a slice of ssh.AuthMethod, prioritizing the SSH agent.
func (n *Node) getAuthMethods() ([]ssh.AuthMethod, error) {
	var signers []ssh.Signer

	// 1. Get Agent Signers
	if authSocket := os.Getenv("SSH_AUTH_SOCK"); authSocket != "" {
		if conn, err := net.Dial("unix", authSocket); err == nil {
			agentClient := agent.NewClient(conn)
			if s, err := agentClient.Signers(); err == nil {
				signers = append(signers, s...)
			}
		}
	}

	// 2. Add Private Key (File) if needed
	if n.keyPath != "" {
		shouldLoad := true

		// Use cached signer if available
		if n.cachedSigner != nil {
			signers = append(signers, n.cachedSigner)
			shouldLoad = false
		}

		// Check if key is already in agent (requires .pub file)
		if shouldLoad && len(signers) > 0 {
			if pubBytes, err := n.FileIO.ReadFile(n.keyPath + ".pub"); err == nil {
				if targetPub, _, _, _, err := ssh.ParseAuthorizedKey(pubBytes); err == nil {
					targetMarshaled := string(targetPub.Marshal())
					for _, s := range signers {
						if string(s.PublicKey().Marshal()) == targetMarshaled {
							shouldLoad = false
							break
						}
					}
				}
			}
		}

		// Else load from file with passphrase prompt if needed
		if shouldLoad {
			if signer, err := n.loadPrivateKey(); err == nil {
				n.cachedSigner = signer
				signers = append(signers, signer)
			} else {
				log.Printf("Warning: failed to load private key: %v\n", err)
			}
		}
	}

	if len(signers) == 0 {
		return nil, fmt.Errorf("no valid authentication methods configured. Check SSH_AUTH_SOCK and private key path")
	}

	return []ssh.AuthMethod{ssh.PublicKeys(signers...)}, nil
}

// loadPrivateKey reads and parses the private key, prompting for passphrase if needed.
func (n *Node) loadPrivateKey() (ssh.Signer, error) {
	key, err := n.FileIO.ReadFile(n.keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file %s: %v", n.keyPath, err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err == nil {
		return signer, nil
	}

	if _, ok := err.(*ssh.PassphraseMissingError); !ok {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	// Key is encrypted, prompt for passphrase
	log.Printf("Enter passphrase for key '%s': ", n.keyPath)
	passphrase, err := term.ReadPassword(int(syscall.Stdin))
	log.Println()
	if err != nil {
		return nil, fmt.Errorf("failed to read passphrase: %v", err)
	}

	signer, err = ssh.ParsePrivateKeyWithPassphrase(key, passphrase)
	// Clear passphrase from memory
	for i := range passphrase {
		passphrase[i] = 0
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key with passphrase: %v", err)
	}

	return signer, nil
}

// setupAgentForwarding sets up SSH agent forwarding on the client (best effort)
func (n *Node) setupAgentForwarding(client *ssh.Client) error {
	authSocket := os.Getenv("SSH_AUTH_SOCK")
	if authSocket == "" {
		return nil
	}

	conn, err := net.Dial("unix", authSocket)
	if err != nil {
		return fmt.Errorf("failed to connect to SSH agent: %v", err)
	}

	return agent.ForwardToAgent(client, agent.NewClient(conn))
}
