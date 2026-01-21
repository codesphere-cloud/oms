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

type Node struct {
	Name       string `json:"name"`
	ExternalIP string `json:"external_ip"`
	InternalIP string `json:"internal_ip"`
}

type NodeManager struct {
	FileIO       util.FileIO
	KeyPath      string
	cachedSigner ssh.Signer // cached signer to avoid repeated passphrase prompts
}

// getAuthMethods constructs a slice of ssh.AuthMethod, prioritizing the SSH agent.
func (n *NodeManager) getAuthMethods() ([]ssh.AuthMethod, error) {
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
	if n.KeyPath != "" {
		shouldLoad := true

		// Use cached signer if available
		if n.cachedSigner != nil {
			signers = append(signers, n.cachedSigner)
			shouldLoad = false
		}

		// Check if key is already in agent (requires .pub file)
		if shouldLoad && len(signers) > 0 {
			if pubBytes, err := n.FileIO.ReadFile(n.KeyPath + ".pub"); err == nil {
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
				fmt.Printf("Warning: failed to load private key: %v\n", err)
			}
		}
	}

	if len(signers) == 0 {
		return nil, fmt.Errorf("no valid authentication methods configured. Check SSH_AUTH_SOCK and private key path")
	}

	return []ssh.AuthMethod{ssh.PublicKeys(signers...)}, nil
}

// loadPrivateKey reads and parses the private key, prompting for passphrase if needed.
func (n *NodeManager) loadPrivateKey() (ssh.Signer, error) {
	key, err := n.FileIO.ReadFile(n.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file %s: %v", n.KeyPath, err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err == nil {
		return signer, nil
	}

	if _, ok := err.(*ssh.PassphraseMissingError); !ok {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	// Key is encrypted, prompt for passphrase
	fmt.Printf("Enter passphrase for key '%s': ", n.KeyPath)
	passphrase, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
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

func (n *NodeManager) connectToJumpbox(ip, username string) (*ssh.Client, error) {
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

func (n *NodeManager) forwardAgent(client *ssh.Client, session *ssh.Session) error {
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

const jumpboxUser = "ubuntu"

// RunSSHCommand connects to the node, executes a command and streams the output
func (n *NodeManager) RunSSHCommand(jumpboxIp string, ip string, username string, command string) error {
	client, err := n.GetClient(jumpboxIp, ip, username)
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
		fmt.Printf(" Warning: Agent forwarding setup failed on session: %v\n", err)
	}

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
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

func (n *NodeManager) GetClient(jumpboxIp string, ip string, username string) (*ssh.Client, error) {
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

func (n *NodeManager) GetSFTPClient(jumpboxIp string, ip string, username string) (*sftp.Client, error) {
	client, err := n.GetClient(jumpboxIp, ip, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get SSH client: %v", err)
	}
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return nil, fmt.Errorf("failed to create SFTP client: %v", err)
	}
	return sftpClient, nil
}

// EnsureDirectoryExists creates the directory on the remote node via SSH if it does not exist.
func (nm *NodeManager) EnsureDirectoryExists(jumpboxIp string, ip string, username string, dir string) error {
	cmd := fmt.Sprintf("mkdir -p '%s'", dir)
	return nm.RunSSHCommand(jumpboxIp, ip, username, cmd)
}

func (n *NodeManager) CopyFile(jumpboxIp string, ip string, username string, src string, dst string) error {
	client, err := n.GetSFTPClient(jumpboxIp, ip, username)
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

func (n *Node) HasCommand(nm *NodeManager, command string) bool {
	checkCommand := fmt.Sprintf("command -v %s >/dev/null 2>&1", command)
	err := nm.RunSSHCommand("", n.ExternalIP, "root", checkCommand)
	if err != nil {
		// If the command returns a non-zero exit status, it means the command is not found
		return false
	}
	return true
}

func (n *Node) InstallOms(nm *NodeManager) error {
	remoteCommands := []string{
		"wget -qO- 'https://api.github.com/repos/codesphere-cloud/oms/releases/latest' | jq -r '.assets[] | select(.name | match(\"oms-cli.*linux_amd64\")) | .browser_download_url' | xargs wget -O oms-cli",
		"chmod +x oms-cli; sudo mv oms-cli /usr/local/bin/",
		"curl -LO https://github.com/getsops/sops/releases/download/v3.11.0/sops-v3.11.0.linux.amd64; sudo mv sops-v3.11.0.linux.amd64 /usr/local/bin/sops; sudo chmod +x /usr/local/bin/sops",
		"wget https://dl.filippo.io/age/latest?for=linux/amd64 -O age.tar.gz; tar -xvf age.tar.gz; sudo mv age/age* /usr/local/bin/",
	}
	for _, cmd := range remoteCommands {
		err := nm.RunSSHCommand("", n.ExternalIP, "root", cmd)
		if err != nil {
			return fmt.Errorf("failed to run remote command '%s': %w", cmd, err)
		}
	}
	return nil
}

func (n *Node) CopyFile(jumpbox *Node, nm *NodeManager, src string, dst string) error {
	if jumpbox == nil {
		err := nm.EnsureDirectoryExists("", n.ExternalIP, "root", filepath.Dir(dst))
		if err != nil {
			return fmt.Errorf("failed to ensure directory exists: %w", err)
		}
		return nm.CopyFile("", n.ExternalIP, "root", src, dst)
	}
	err := nm.EnsureDirectoryExists(jumpbox.ExternalIP, n.InternalIP, "root", filepath.Dir(dst))
	if err != nil {
		return fmt.Errorf("failed to ensure directory exists: %w", err)
	}
	return nm.CopyFile(jumpbox.ExternalIP, n.InternalIP, "root", src, dst)
}

// HasAcceptEnvConfigured checks if AcceptEnv is configured
func (n *Node) HasAcceptEnvConfigured(jumpbox *Node, nm *NodeManager) bool {
	checkCommand := "sudo grep -E '^AcceptEnv OMS_PORTAL_API_KEY' /etc/ssh/sshd_config >/dev/null 2>&1"
	err := n.RunSSHCommand(jumpbox, nm, "ubuntu", checkCommand)
	if err != nil {
		// If the command returns a NON-zero exit status, it means AcceptEnv is not configured
		return false
	}
	return true
}

func (n *Node) ConfigureAcceptEnv(jumpbox *Node, nm *NodeManager) error {
	cmds := []string{
		"sudo sed -i 's/^#\\?AcceptEnv.*/AcceptEnv OMS_PORTAL_API_KEY/' /etc/ssh/sshd_config",
		"sudo systemctl restart sshd",
	}
	for _, cmd := range cmds {
		err := n.RunSSHCommand(jumpbox, nm, "ubuntu", cmd)
		if err != nil {
			return fmt.Errorf("failed to run command '%s': %w", cmd, err)
		}
	}
	return nil
}

func (n *Node) HasRootLoginEnabled(jumpbox *Node, nm *NodeManager) bool {
	checkCommandPermit := "sudo grep -E '^PermitRootLogin yes' /etc/ssh/sshd_config >/dev/null 2>&1"
	err := n.RunSSHCommand(jumpbox, nm, "ubuntu", checkCommandPermit)
	if err != nil {
		// If the command returns a NON-zero exit status, it means root login is not permitted
		return false
	}
	checkCommandAuthorizedKeys := "sudo grep -E '^no-port-forwarding' /root/.ssh/authorized_keys >/dev/null 2>&1"
	err = n.RunSSHCommand(jumpbox, nm, "ubuntu", checkCommandAuthorizedKeys)
	if err == nil {
		// If the command returns a ZERO exit status, it means root login is prevented
		return false
	}
	return true
}

func (n *Node) HasFile(jumpbox *Node, nm *NodeManager, filePath string) bool {
	checkCommand := fmt.Sprintf("test -f '%s'", filePath)
	err := n.RunSSHCommand(jumpbox, nm, "ubuntu", checkCommand)
	if err != nil {
		// If the command returns a non-zero exit status, it means the file does not exist
		return false
	}
	return true
}

func (n *Node) RunSSHCommand(jumpbox *Node, nm *NodeManager, username string, command string) error {
	if jumpbox == nil {
		return nm.RunSSHCommand("", n.ExternalIP, username, command)
	}

	return nm.RunSSHCommand(jumpbox.ExternalIP, n.InternalIP, username, command)
}

func (n *Node) EnableRootLogin(jumpbox *Node, nm *NodeManager) error {
	cmds := []string{
		"sudo sed -i 's/^#\\?PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config",
		"sudo sed -i 's/no-port-forwarding.*$//g' /root/.ssh/authorized_keys",
		"sudo systemctl restart sshd",
	}
	for _, cmd := range cmds {
		err := n.RunSSHCommand(jumpbox, nm, "ubuntu", cmd)
		if err != nil {
			return fmt.Errorf("failed to run command '%s': %w", cmd, err)
		}
	}
	return nil
}

func (n *Node) WaitForSSH(jumpbox *Node, nm *NodeManager, timeout time.Duration) error {
	start := time.Now()
	jumpboxIp := ""
	nodeIp := n.ExternalIP
	if jumpbox != nil {
		jumpboxIp = jumpbox.ExternalIP
		nodeIp = n.InternalIP
	}
	for {
		client, err := nm.GetClient(jumpboxIp, nodeIp, jumpboxUser)
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

func (n *Node) HasInotifyWatchesConfigured(jumpbox *Node, nm *NodeManager) bool {
	return n.HasSysctlLine(jumpbox, "fs.inotify.max_user_watches=1048576", nm)
}

func (n *Node) ConfigureInotifyWatches(jumpbox *Node, nm *NodeManager) error {
	return n.ConfigureSysctlLine(jumpbox, "fs.inotify.max_user_watches=1048576", nm)
}

func (n *Node) HasMemoryMapConfigured(jumpbox *Node, nm *NodeManager) bool {
	return n.HasSysctlLine(jumpbox, "vm.max_map_count=262144", nm)
}

func (n *Node) ConfigureMemoryMap(jumpbox *Node, nm *NodeManager) error {
	return n.ConfigureSysctlLine(jumpbox, "vm.max_map_count=262144", nm)
}

func (n *Node) HasSysctlLine(jumpbox *Node, line string, nm *NodeManager) bool {
	checkCommand := fmt.Sprintf("sudo grep -E '^%s' /etc/sysctl.conf >/dev/null 2>&1", line)
	err := n.RunSSHCommand(jumpbox, nm, "root", checkCommand)
	if err != nil {
		// If the command returns a NON-zero exit status, it means the setting is not configured
		return false
	}
	return true
}

func (n *Node) ConfigureSysctlLine(jumpbox *Node, line string, nm *NodeManager) error {
	cmds := []string{
		fmt.Sprintf("echo '%s' | sudo tee -a /etc/sysctl.conf", line),
		"sudo sysctl -p",
	}
	for _, cmd := range cmds {
		err := n.RunSSHCommand(jumpbox, nm, "root", cmd)
		if err != nil {
			return fmt.Errorf("failed to run command '%s': %w", cmd, err)
		}
	}
	return nil
}
