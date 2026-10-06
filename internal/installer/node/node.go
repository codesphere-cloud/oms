// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package node manages the remote hosts of an installation over SSH.
package node

import (
	"bytes"
	"context"
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
	"golang.org/x/term"
)

type Node struct {
	FileIO util.FileIO `json:"-"`
	// If connecting via the Jumpbox
	Jumpbox *Node `json:"-"`
	// Config
	KeyPath      string     `json:"-"`
	Name         string     `json:"name"`
	ExternalIP   string     `json:"external_ip"`
	InternalIP   string     `json:"internal_ip"`
	cachedKey    any        `json:"-"`
	cachedSigner ssh.Signer `json:"-"`
	sshQuiet     bool       `json:"-"`

	NodeClient NodeClient `json:"-"`
	// SSH client cache: map[username]*ssh.Client
	clientCache map[string]*ssh.Client
	clientMu    sync.Mutex
}

//mockery:generate: true
type NodeClient interface {
	RunCommand(n *Node, username string, command string) error
	CopyFile(n *Node, src string, dst string) error
	DownloadFile(n *Node, src string, dst string) error
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
	_ = session.Setenv("OMS_PORTAL_API", os.Getenv("OMS_PORTAL_API"))
	_ = agent.RequestAgentForwarding(session) // Best effort, ignore errors

	var stderrBuf bytes.Buffer
	session.Stderr = &stderrBuf
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
		if r.Quiet && stderrBuf.Len() > 0 {
			return fmt.Errorf("command failed: %w\n%s", err, stderrBuf.String())
		}
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
		KeyPath:    util.ExpandPath(n.KeyPath),
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
		"wget -qO- 'https://api.github.com/repos/codesphere-cloud/oms/releases/latest' | jq -r '.assets[] | select(.name | match(\"oms.*linux_amd64\")) | .browser_download_url' | xargs wget -O oms",
		"chmod +x oms; sudo mv oms /usr/local/bin/",
	}
	for _, cmd := range remoteCommands {
		err := n.RunSSHCommand("root", cmd)
		if err != nil {
			return fmt.Errorf("failed to run remote command '%s': %w", cmd, err)
		}
	}
	return nil
}

// EnsureOmsDependencies installs the command-line tools used by remote OMS
// workflows independently of how the OMS binary itself was provisioned.
func (n *Node) EnsureOmsDependencies() error {
	dependencies := []struct {
		command string
		install string
	}{
		{
			command: "sops",
			install: "curl -LO https://github.com/getsops/sops/releases/download/v3.11.0/sops-v3.11.0.linux.amd64; sudo mv sops-v3.11.0.linux.amd64 /usr/local/bin/sops; sudo chmod +x /usr/local/bin/sops",
		},
		{
			command: "age-keygen",
			install: "wget https://dl.filippo.io/age/latest?for=linux/amd64 -O age.tar.gz; tar -xvf age.tar.gz; sudo mv age/age* /usr/local/bin/",
		},
	}

	for _, dependency := range dependencies {
		if n.HasCommand(dependency.command) {
			continue
		}
		if err := n.RunSSHCommand("root", dependency.install); err != nil {
			return fmt.Errorf("failed to install OMS dependency %s: %w", dependency.command, err)
		}
	}
	return nil
}

// HasAcceptEnvConfigured checks if AcceptEnv is configured
func (n *Node) HasAcceptEnvConfigured() bool {
	checkCommand := "sudo grep -qxF 'AcceptEnv OMS_PORTAL_API_KEY OMS_PORTAL_API' /etc/ssh/sshd_config >/dev/null 2>&1"
	err := n.RunSSHCommand("ubuntu", checkCommand)
	if err != nil {
		// If the command returns a NON-zero exit status, it means AcceptEnv is not configured
		return false
	}
	return true
}

// ConfigureAcceptEnv configures AcceptEnv for OMS_PORTAL_API_KEY and OMS_PORTAL_API
func (n *Node) ConfigureAcceptEnv() error {
	cmds := []string{
		"sudo sh -c \"grep -qxF 'AcceptEnv OMS_PORTAL_API_KEY OMS_PORTAL_API' /etc/ssh/sshd_config || printf '\\nAcceptEnv OMS_PORTAL_API_KEY OMS_PORTAL_API\\n' >> /etc/ssh/sshd_config\"",
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
	return n.hasSysctlLine("fs.inotify.max_user_watches=1048576") &&
		n.isSysctlActive("fs.inotify.max_user_watches", "1048576") &&
		n.hasSysctlLine("fs.inotify.max_user_instances=8192") &&
		n.isSysctlActive("fs.inotify.max_user_instances", "8192")
}

func (n *Node) ConfigureInotifyWatches() error {
	lines := []string{
		"fs.inotify.max_user_watches=1048576",
		"fs.inotify.max_user_instances=8192",
	}
	return n.configureSysctlLines(lines)
}

func (n *Node) HasMemoryMapConfigured() bool {
	return n.hasSysctlLine("vm.max_map_count=262144") && n.isSysctlActive("vm.max_map_count", "262144")
}

func (n *Node) ConfigureMemoryMap() error {
	return n.configureSysctlLines([]string{"vm.max_map_count=262144"})
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
	jumpBoxIP := ""
	nodeIP := n.ExternalIP
	if n.Jumpbox != nil {
		jumpBoxIP = n.Jumpbox.ExternalIP
		nodeIP = n.InternalIP
	}

	err := n.ensureDirectoryExists("root", filepath.Dir(dst))
	if err != nil {
		return fmt.Errorf("failed to ensure directory exists: %w", err)
	}

	return n.copyFile(jumpBoxIP, nodeIP, "root", src, dst)
}

// DownloadFile downloads a file from the remote node to the local system via SFTP
func (c *SSHNodeClient) DownloadFile(n *Node, src, dst string) error {
	jumpBoxIP := ""
	nodeIP := n.ExternalIP
	if n.Jumpbox != nil {
		jumpBoxIP = n.Jumpbox.ExternalIP
		nodeIP = n.InternalIP
	}

	return n.downloadFile(jumpBoxIP, nodeIP, "root", src, dst)
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

func (n *Node) isSysctlActive(key, expected string) bool {
	checkCommand := fmt.Sprintf("sudo sysctl -n %s | grep -q '^%s$'", key, expected)
	err := n.RunSSHCommand("root", checkCommand)
	return err == nil
}

// configureSysctlLine appends a specific line to /etc/sysctl.conf and applies the settings on the remote node via SSH
func (n *Node) configureSysctlLines(lines []string) error {
	for _, line := range lines {
		if !n.hasSysctlLine(line) {
			cmd := fmt.Sprintf("echo '%s' | sudo tee -a /etc/sysctl.conf", line)
			if err := n.RunSSHCommand("root", cmd); err != nil {
				return fmt.Errorf("failed to append to sysctl.conf: %w", err)
			}
		}
	}

	err := n.RunSSHCommand("root", "sudo sysctl -p")
	if err != nil {
		return fmt.Errorf("failed to run sysctl -p: %w", err)
	}

	return nil
}

// getOrCreateClient returns a cached SSH client or creates a new one if not cached.
func (n *Node) getOrCreateClient(jumpboxIp string, ip string, username string) (*ssh.Client, error) {
	n.clientMu.Lock()
	defer n.clientMu.Unlock()

	if n.clientCache == nil {
		n.clientCache = make(map[string]*ssh.Client)
	}

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
	authMethods, closeAgent, err := n.getAuthMethods()
	if err != nil {
		return nil, fmt.Errorf("failed to get authentication methods: %w", err)
	}
	defer closeAgent()

	if jumpboxIp != "" {
		// Use the Jumpbox's cached client if available
		jbClient, err := n.Jumpbox.getOrCreateClient("", jumpboxIp, jumpboxUser)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to jumpbox: %v", err)
		}

		finalTargetConfig := &ssh.ClientConfig{
			User:    username,
			Auth:    authMethods,
			Timeout: 10 * time.Second,
			// WARNING: This is INSECURE for production!
			// It tells the client to accept any host key.
			// For production, you should implement a proper HostKeyCallback
			// to verify the remote server's identity.
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}

		finalAddr := fmt.Sprintf("%s:22", ip)
		dialCtx, cancel := context.WithTimeout(context.Background(), finalTargetConfig.Timeout)
		defer cancel()
		jbConn, err := jbClient.DialContext(dialCtx, "tcp", finalAddr)
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
		User:    username,
		Auth:    authMethods,
		Timeout: 10 * time.Second,
		// WARNING: This is INSECURE for production!
		// It tells the client to accept any host key.
		// For production, you should implement a proper HostKeyCallback
		// to verify the remote server's identity.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
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

	sftpClient, err := sftp.NewClient(client,
		sftp.UseConcurrentWrites(true),
	)
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

// downloadFile downloads a file from the remote node to the local system via SFTP
func (n *Node) downloadFile(jumpboxIp, ip, username, src, dst string) error {
	client, err := n.getSFTPClient(jumpboxIp, ip, username)
	if err != nil {
		return fmt.Errorf("failed to get SSH client: %v", err)
	}
	defer util.IgnoreError(client.Close)

	srcFile, err := client.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %v", src, err)
	}
	defer util.IgnoreError(srcFile.Close)

	dstFile, err := n.FileIO.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file %s: %v", dst, err)
	}
	defer util.IgnoreError(dstFile.Close)

	_, err = dstFile.ReadFrom(srcFile)
	if err != nil {
		return fmt.Errorf("failed to download data from %s to %s: %v", src, dst, err)
	}

	return nil
}

// getAuthMethods constructs a slice of ssh.AuthMethod, prioritizing the SSH agent.
// The agent signers sign during the SSH handshake, so the caller must keep the agent
// connection open until the handshake is done and then call the returned close function.
func (n *Node) getAuthMethods() ([]ssh.AuthMethod, func(), error) {
	var signers []ssh.Signer

	// 1. Get Agent Signers
	localAgent, closeAgent, err := dialLocalAgent()
	if err != nil {
		log.Printf("Warning: failed to connect to SSH agent: %v", err)
	}

	if localAgent != nil {
		if s, err := localAgent.Signers(); err == nil {
			signers = append(signers, s...)
		}
	}

	// 2. Add Private Key (File) if needed
	if n.KeyPath != "" {
		shouldLoad := true

		// Use cached signer if available
		if n.cachedSigner != nil {
			signers = append(signers, n.cachedSigner)
			shouldLoad = false
		}

		// Check if key is already in agent (requires .pub file)
		if shouldLoad && localAgent != nil && agentHoldsKey(localAgent, n.KeyPath, n.FileIO) {
			shouldLoad = false
		}

		// Else load from file with passphrase prompt if needed
		if shouldLoad {
			if _, signer, err := n.loadKey(); err == nil {
				signers = append(signers, signer)
			} else {
				log.Printf("Warning: failed to load private key: %v\n", err)
			}
		}
	}

	if len(signers) == 0 {
		closeAgent()

		return nil, nil, fmt.Errorf("no valid authentication methods configured. Check SSH_AUTH_SOCK and private key path")
	}

	return []ssh.AuthMethod{ssh.PublicKeys(signers...)}, closeAgent, nil
}

// dialLocalAgent connects to the SSH agent at SSH_AUTH_SOCK. Without an agent it returns a
// nil agent. The returned close function is always safe to call.
func dialLocalAgent() (agent.ExtendedAgent, func(), error) {
	authSocket := os.Getenv("SSH_AUTH_SOCK")
	if authSocket == "" {
		return nil, func() {}, nil
	}

	conn, err := net.Dial("unix", authSocket)
	if err != nil {
		return nil, func() {}, fmt.Errorf("failed to dial SSH agent at %s: %w", authSocket, err)
	}

	return agent.NewClient(conn), func() { util.IgnoreError(conn.Close) }, nil
}

// loadKey returns the private key at KeyPath and its signer. The key is read only once, so an
// encrypted key prompts for its passphrase at most once.
func (n *Node) loadKey() (any, ssh.Signer, error) {
	if n.cachedKey != nil {
		return n.cachedKey, n.cachedSigner, nil
	}

	key, err := n.loadPrivateKey()
	if err != nil {
		return nil, nil, err
	}

	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create signer from private key: %v", err)
	}

	n.cachedKey = key
	n.cachedSigner = signer

	return key, signer, nil
}

// loadPrivateKey reads and parses the raw private key, prompting for passphrase if needed.
func (n *Node) loadPrivateKey() (any, error) {
	key, err := n.FileIO.ReadFile(n.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file %s: %v", n.KeyPath, err)
	}

	rawKey, err := ssh.ParseRawPrivateKey(key)
	if err == nil {
		return rawKey, nil
	}

	if _, ok := err.(*ssh.PassphraseMissingError); !ok {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	// Key is encrypted, prompt for passphrase
	log.Printf("Enter passphrase for key '%s': ", n.KeyPath)
	passphrase, err := term.ReadPassword(int(syscall.Stdin))
	log.Println()
	if err != nil {
		return nil, fmt.Errorf("failed to read passphrase: %v", err)
	}

	rawKey, err = ssh.ParseRawPrivateKeyWithPassphrase(key, passphrase)
	// Clear passphrase from memory
	for i := range passphrase {
		passphrase[i] = 0
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key with passphrase: %v", err)
	}

	return rawKey, nil
}

// setupAgentForwarding sets up SSH agent forwarding on the client (best effort). Tools on the
// remote host, such as k0sctl on the jumpbox, authenticate to further hosts through the
// forwarded agent, so it always offers the key at KeyPath, even when the local agent lacks it.
//
// The local agent connection stays open as long as the client, which serves the forwarded
// agent requests over it.
func (n *Node) setupAgentForwarding(client *ssh.Client) error {
	localAgent, closeAgent, err := dialLocalAgent()
	if err != nil {
		log.Printf("Warning: failed to connect to SSH agent: %v", err)
	}

	forwarded, err := n.forwardedAgent(localAgent)
	if err == nil && forwarded != nil {
		err = agent.ForwardToAgent(client, forwarded)
		if err == nil {
			go func() {
				_ = client.Wait()

				closeAgent()
			}()

			return nil
		}

		err = fmt.Errorf("failed to forward SSH agent: %w", err)
	}

	closeAgent()

	return err
}

// forwardedAgent returns the agent to forward to remote hosts: the local agent when it already
// holds the key at KeyPath, otherwise an agent that adds that key to the local agent's keys.
func (n *Node) forwardedAgent(localAgent agent.ExtendedAgent) (agent.Agent, error) {
	if n.KeyPath == "" || (localAgent != nil && agentHoldsKey(localAgent, n.KeyPath, n.FileIO)) {
		return localAgent, nil
	}

	key, signer, err := n.loadKey()
	if err != nil {
		if localAgent == nil {
			return nil, fmt.Errorf("no SSH agent to forward and failed to load private key: %w", err)
		}

		log.Printf("Warning: forwarding SSH agent without the key at %s: %v", n.KeyPath, err)

		return localAgent, nil
	}

	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: key, Comment: n.KeyPath}); err != nil {
		return nil, fmt.Errorf("failed to add private key to forwarded agent: %w", err)
	}

	if localAgent == nil {
		return keyring, nil
	}

	return &keyAddingAgent{
		ExtendedAgent: localAgent,
		keyring:       keyring.(agent.ExtendedAgent),
		keyBlob:       signer.PublicKey().Marshal(),
	}, nil
}

// agentHoldsKey reports whether the agent holds the private key whose public key is stored next
// to keyPath.
func agentHoldsKey(a agent.Agent, keyPath string, fileIO util.FileIO) bool {
	pubBytes, err := fileIO.ReadFile(keyPath + ".pub")
	if err != nil {
		return false
	}

	pub, _, _, _, err := ssh.ParseAuthorizedKey(pubBytes)
	if err != nil {
		return false
	}

	keys, err := a.List()
	if err != nil {
		return false
	}

	target := string(pub.Marshal())
	for _, k := range keys {
		if string(k.Marshal()) == target {
			return true
		}
	}

	return false
}

// keyAddingAgent serves the local agent with one extra key from a keyring. Signing requests for
// that key go to the keyring, everything else to the local agent.
type keyAddingAgent struct {
	agent.ExtendedAgent
	keyring agent.ExtendedAgent
	keyBlob []byte
}

func (a *keyAddingAgent) List() ([]*agent.Key, error) {
	return withExtra(a.keyring.List, a.ExtendedAgent.List)
}

func (a *keyAddingAgent) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	return a.SignWithFlags(key, data, 0)
}

func (a *keyAddingAgent) SignWithFlags(key ssh.PublicKey, data []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	signer := a.ExtendedAgent
	if bytes.Equal(key.Marshal(), a.keyBlob) {
		signer = a.keyring
	}

	signature, err := signer.SignWithFlags(key, data, flags)
	if err != nil {
		return nil, fmt.Errorf("failed to sign with SSH agent: %w", err)
	}

	return signature, nil
}

func (a *keyAddingAgent) Signers() ([]ssh.Signer, error) {
	return withExtra(a.keyring.Signers, a.ExtendedAgent.Signers)
}

// withExtra prepends the keyring's entries to the local agent's. A failing local agent is
// ignored, as it may be locked or broken while the extra key still works.
func withExtra[T any](extra, local func() ([]T, error)) ([]T, error) {
	extraItems, err := extra()
	if err != nil {
		return nil, err
	}

	localItems, err := local()
	if err != nil {
		return extraItems, nil
	}

	return append(extraItems, localItems...), nil
}
