// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
)

type NodeManager interface {
	// Node
	CreateSubNode(name string, externalIP string, internalIP string) NodeManager
	UpdateNode(name string, externalIP string, internalIP string)
	GetExternalIP() string
	GetInternalIP() string
	GetName() string
	// SSH
	WaitForSSH(timeout time.Duration) error
	RunSSHCommand(username string, command string, quiet bool) error
	// OMS
	HasCommand(command string) bool
	InstallOms() error
	// AcceptEnv
	HasAcceptEnvConfigured() bool
	ConfigureAcceptEnv() error
	// Root Login
	HasRootLoginEnabled() bool
	EnableRootLogin() error
	// Host Config
	HasInotifyWatchesConfigured() bool
	ConfigureInotifyWatches() error
	HasMemoryMapConfigured() bool
	ConfigureMemoryMap() error
	// Files
	HasFile(filePath string) bool
	CopyFile(src string, dst string) error
}

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
}

const jumpboxUser = "ubuntu"

// NewNode creates a new Node with the given FileIO and SSH key path
func NewNode(fileIO util.FileIO, keyPath string) *Node {
	return &Node{
		FileIO:  fileIO,
		keyPath: util.ExpandPath(keyPath),
	}
}

// CreateSubNode creates a Node object representing a node behind a jumpbox
func (n *Node) CreateSubNode(name string, externalIP string, internalIP string) NodeManager {
	return &Node{
		FileIO:     n.FileIO,
		Jumpbox:    n,
		keyPath:    util.ExpandPath(n.keyPath),
		Name:       name,
		ExternalIP: externalIP,
		InternalIP: internalIP,
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

// WaitForSSH tries to connect to the node via SSH until timeout
func (n *Node) WaitForSSH(timeout time.Duration) error {
	start := time.Now()
	jumpboxIp := ""
	nodeIp := n.ExternalIP
	if n.Jumpbox != nil {
		jumpboxIp = n.Jumpbox.ExternalIP
		nodeIp = n.InternalIP
	}
	for {
		client, err := n.getClient(jumpboxIp, nodeIp, jumpboxUser)
		if err == nil {
			_ = client.Close()
			return nil
		}
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for SSH on node %s (%s)", n.Name, n.ExternalIP)
		}
		time.Sleep(5 * time.Second)
	}
}

// RunSSHCommand connects to the node, executes a command and streams the output.
// If quiet is true, command output is not printed to stdout/stderr.
func (n *Node) RunSSHCommand(username string, command string, quiet bool) error {
	var jumpboxIp string
	var ip string
	if n.Jumpbox != nil {
		jumpboxIp = n.Jumpbox.ExternalIP
		ip = n.InternalIP
	} else {
		jumpboxIp = ""
		ip = n.ExternalIP
	}

	client, err := n.getClient(jumpboxIp, ip, username)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}
	defer util.IgnoreError(client.Close)

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session on jumpbox: %v", err)
	}
	defer util.IgnoreError(session.Close)

	_ = session.Setenv("OMS_PORTAL_API_KEY", os.Getenv("OMS_PORTAL_API_KEY"))
	err = n.forwardAgent(client, session)
	if err != nil {
		log.Printf(" Warning: Agent forwarding setup failed on session: %v\n", err)
	}

	if !quiet {
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

// HasCommand checks if a command exists on the remote node via SSH
func (n *Node) HasCommand(command string) bool {
	checkCommand := fmt.Sprintf("command -v %s >/dev/null 2>&1", command)
	err := n.RunSSHCommand("root", checkCommand, true)
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
		err := n.RunSSHCommand("root", cmd, false)
		if err != nil {
			return fmt.Errorf("failed to run remote command '%s': %w", cmd, err)
		}
	}
	return nil
}

// HasAcceptEnvConfigured checks if AcceptEnv is configured
func (n *Node) HasAcceptEnvConfigured() bool {
	checkCommand := "sudo grep -E '^AcceptEnv OMS_PORTAL_API_KEY' /etc/ssh/sshd_config >/dev/null 2>&1"
	err := n.RunSSHCommand("ubuntu", checkCommand, true)
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
		err := n.RunSSHCommand("ubuntu", cmd, true)
		if err != nil {
			return fmt.Errorf("failed to run command '%s': %w", cmd, err)
		}
	}
	return nil
}

// HasRootLoginEnabled checks if root login is enabled on the remote node via SSH
func (n *Node) HasRootLoginEnabled() bool {
	checkCommandPermit := "sudo grep -E '^PermitRootLogin yes' /etc/ssh/sshd_config >/dev/null 2>&1"
	err := n.RunSSHCommand("ubuntu", checkCommandPermit, true)
	if err != nil {
		// If the command returns a NON-zero exit status, it means root login is not permitted
		return false
	}
	checkCommandAuthorizedKeys := "sudo grep -E '^no-port-forwarding' /root/.ssh/authorized_keys >/dev/null 2>&1"
	err = n.RunSSHCommand("ubuntu", checkCommandAuthorizedKeys, true)
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
		err := n.RunSSHCommand("ubuntu", cmd, true)
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
func (n *Node) HasFile(filePath string) bool {
	checkCommand := fmt.Sprintf("test -f '%s'", filePath)
	err := n.RunSSHCommand("ubuntu", checkCommand, true)
	if err != nil {
		// If the command returns a non-zero exit status, it means the file does not exist
		return false
	}
	return true
}

// CopyFile copies a file from the local system to the remote node via SFTP
func (n *Node) CopyFile(src string, dst string) error {
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
	err := n.RunSSHCommand("root", checkCommand, true)
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
		err := n.RunSSHCommand("root", cmd, true)
		if err != nil {
			return fmt.Errorf("failed to run command '%s': %w", cmd, err)
		}
	}
	return nil
}

// getClient creates and returns an SSH client connected to the node
func (n *Node) getClient(jumpboxIp string, ip string, username string) (*ssh.Client, error) {
	authMethods, err := n.getAuthMethods()
	if err != nil {
		return nil, fmt.Errorf("failed to get authentication methods: %w", err)
	}
	if jumpboxIp != "" {
		jbClient, err := n.connectToJumpbox(jumpboxIp, jumpboxUser)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to jumpbox: %v", err)
		}

		finalTargetConfig := &ssh.ClientConfig{
			User:            username,
			Auth:            authMethods,
			Timeout:         10 * time.Second,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
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

// getSFTPClient creates and returns an SFTP client connected to the node
func (n *Node) getSFTPClient(jumpboxIp string, ip string, username string) (*sftp.Client, error) {
	client, err := n.getClient(jumpboxIp, ip, username)
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
func (nm *Node) ensureDirectoryExists(username string, dir string) error {
	cmd := fmt.Sprintf("mkdir -p '%s'", dir)
	return nm.RunSSHCommand(username, cmd, true)
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

func (n *Node) connectToJumpbox(ip, username string) (*ssh.Client, error) {
	authMethods, err := n.getAuthMethods()
	if err != nil {
		return nil, fmt.Errorf("jumpbox authentication setup failed: %v", err)
	}

	config := &ssh.ClientConfig{
		User:    username,
		Auth:    authMethods,
		Timeout: 10 * time.Second,
		// WARNING: Still using InsecureIgnoreHostKey for simplicity. Use known_hosts in production.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	addr := fmt.Sprintf("%s:22", ip)
	jumpboxClient, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial jumpbox %s: %v", addr, err)
	}

	// Enable Agent Forwarding on the jumpbox connection
	if err := n.forwardAgent(jumpboxClient, nil); err != nil {
		fmt.Printf(" Warning: Agent forwarding setup failed on jumpbox: %v\n", err)
	}

	return jumpboxClient, nil
}

func (n *Node) forwardAgent(client *ssh.Client, session *ssh.Session) error {
	authSocket := os.Getenv("SSH_AUTH_SOCK")
	if authSocket == "" {
		log.Printf("SSH_AUTH_SOCK not set. Cannot perform agent forwarding")
	} else {
		// Connect to the local SSH Agent socket
		conn, err := net.Dial("unix", authSocket)
		if err != nil {
			log.Printf("failed to dial SSH agent socket: %v", err)
		} else {
			// Create an agent client for the local agent
			ag := agent.NewClient(conn)
			// This tells the remote server to proxy authentication requests back to us.
			if err := agent.ForwardToAgent(client, ag); err != nil {
				log.Printf("failed to forward agent to remote client: %v", err)
			}
			if session != nil {
				if err := agent.RequestAgentForwarding(session); err != nil {
					log.Printf("failed to request agent forwarding on session: %v", err)
				}
			}
		}

	}
	return nil
}
