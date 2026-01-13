// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
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
	Name       string `json:"name"`
	ExternalIP string `json:"external_ip"`
	InternalIP string `json:"internal_ip"`
	User       string `json:"user,omitempty"`
}

type NodeManager struct {
	FileIO  util.FileIO
	KeyPath string
}

const (
	jumpboxUser     = "ubuntu"
	remoteK0sDir    = "/usr/local/bin"
	remoteK0sConfig = "/etc/k0s/k0s.yaml"
	tmpK0sBinary    = "/tmp/k0s"
	tmpK0sConfig    = "/tmp/k0s-config.yaml"
)

func shellEscape(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

func (nm *NodeManager) getHostKeyCallback() (ssh.HostKeyCallback, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	knownHostsPath := filepath.Join(homeDir, ".ssh", "known_hosts")
	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		sshDir := filepath.Join(homeDir, ".ssh")
		if err := os.MkdirAll(sshDir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create .ssh directory: %w", err)
		}
		if _, err := os.Create(knownHostsPath); err != nil {
			return nil, fmt.Errorf("failed to create known_hosts file: %w", err)
		}
		hostKeyCallback, err = knownhosts.New(knownHostsPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load known_hosts: %w", err)
		}
	}

	return hostKeyCallback, nil
}

func (nm *NodeManager) getAuthMethods() ([]ssh.AuthMethod, error) {
	var authMethods []ssh.AuthMethod

	if authSocket := os.Getenv("SSH_AUTH_SOCK"); authSocket != "" {
		conn, err := net.Dial("unix", authSocket)
		if err == nil {
			agentClient := agent.NewClient(conn)
			authMethods = append(authMethods, ssh.PublicKeysCallback(agentClient.Signers))
			return authMethods, nil
		}
		fmt.Printf("Could not connect to SSH Agent (%s): %v\n", authSocket, err)
	}

	if nm.KeyPath != "" {
		fmt.Printf("Falling back to private key file authentication (key: %s).\n", nm.KeyPath)

		key, err := nm.FileIO.ReadFile(nm.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key file %s: %v", nm.KeyPath, err)
		}

		log.Printf("Successfully read %d bytes from key file", len(key))

		signer, err := ssh.ParsePrivateKey(key)
		if err == nil {
			log.Printf("Successfully parsed private key (type: %s)", signer.PublicKey().Type())
			authMethods = append(authMethods, ssh.PublicKeys(signer))
			return authMethods, nil
		}

		log.Printf("Failed to parse private key: %v", err)
		if _, ok := err.(*ssh.PassphraseMissingError); ok {
			// Check if we're in an interactive terminal
			if !term.IsTerminal(int(syscall.Stdin)) {
				return nil, fmt.Errorf("passphrase-protected key requires interactive terminal. Use ssh-agent or an unencrypted key for automated scenarios")
			}

			fmt.Printf("Enter passphrase for key '%s': ", nm.KeyPath)

			// Read passphrase with a timeout using a channel
			type result struct {
				password []byte
				err      error
			}
			resultChan := make(chan result, 1)
			go func() {
				passphraseBytes, err := term.ReadPassword(int(syscall.Stdin))
				resultChan <- result{password: passphraseBytes, err: err}
			}()

			// Wait for passphrase input with 30 second timeout
			select {
			case res := <-resultChan:
				fmt.Println()
				if res.err != nil {
					return nil, fmt.Errorf("failed to read passphrase: %v", res.err)
				}

				defer func() {
					for i := range res.password {
						res.password[i] = 0
					}
				}()

				signer, err = ssh.ParsePrivateKeyWithPassphrase(key, res.password)
				if err != nil {
					return nil, fmt.Errorf("failed to parse private key with passphrase: %v", err)
				}
				authMethods = append(authMethods, ssh.PublicKeys(signer))
				return authMethods, nil

			case <-time.After(30 * time.Second):
				fmt.Println()
				return nil, fmt.Errorf("passphrase input timeout after 30 seconds")
			}
		}
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no valid authentication methods configured. Check SSH_AUTH_SOCK and private key path")
	}

	return authMethods, nil
}

func (nm *NodeManager) connectToJumpbox(ip, username string) (*ssh.Client, error) {
	authMethods, err := nm.getAuthMethods()
	if err != nil {
		return nil, fmt.Errorf("jumpbox authentication setup failed: %v", err)
	}

	hostKeyCallback, err := nm.getHostKeyCallback()
	if err != nil {
		return nil, fmt.Errorf("failed to get host key callback: %w", err)
	}

	config := &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		Timeout:         10 * time.Second,
		HostKeyCallback: hostKeyCallback,
	}

	addr := fmt.Sprintf("%s:22", ip)
	jumpboxClient, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial jumpbox %s: %v", addr, err)
	}

	if err := nm.forwardAgent(jumpboxClient, nil); err != nil {
		fmt.Printf(" Warning: Agent forwarding setup failed on jumpbox: %v\n", err)
	}

	return jumpboxClient, nil
}

func (nm *NodeManager) forwardAgent(client *ssh.Client, session *ssh.Session) error {
	authSocket := os.Getenv("SSH_AUTH_SOCK")
	if authSocket == "" {
		log.Printf("SSH_AUTH_SOCK not set. Cannot perform agent forwarding")
	} else {
		conn, err := net.Dial("unix", authSocket)
		if err != nil {
			log.Printf("failed to dial SSH agent socket: %v", err)
		} else {
			ag := agent.NewClient(conn)
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

func (nm *NodeManager) RunSSHCommand(jumpboxIp string, ip string, username string, command string) error {
	client, err := nm.GetClient(jumpboxIp, ip, username)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}
	defer util.IgnoreError(client.Close)
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session on target node (%s): %v", ip, err)
	}
	defer util.IgnoreError(session.Close)

	_ = session.Setenv("OMS_PORTAL_API_KEY", os.Getenv("OMS_PORTAL_API_KEY"))

	if err := nm.forwardAgent(client, session); err != nil {
		fmt.Printf(" Warning: Agent forwarding setup failed on session: %v\n", err)
	}

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	if err := session.Start(command); err != nil {
		return fmt.Errorf("failed to start command: %v", err)
	}

	if err := session.Wait(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}

	return nil
}

func (nm *NodeManager) GetClient(jumpboxIp string, ip string, username string) (*ssh.Client, error) {

	authMethods, err := nm.getAuthMethods()
	if err != nil {
		return nil, fmt.Errorf("failed to get authentication methods: %w", err)
	}

	hostKeyCallback, err := nm.getHostKeyCallback()
	if err != nil {
		return nil, fmt.Errorf("failed to get host key callback: %w", err)
	}

	if jumpboxIp != "" {
		jbClient, err := nm.connectToJumpbox(jumpboxIp, jumpboxUser)
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

func (nm *NodeManager) GetSFTPClient(jumpboxIp string, ip string, username string) (*sftp.Client, error) {
	client, err := nm.GetClient(jumpboxIp, ip, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get SSH client: %v", err)
	}
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return nil, fmt.Errorf("failed to create SFTP client: %v", err)
	}
	return sftpClient, nil
}

func (nm *NodeManager) EnsureDirectoryExists(jumpboxIp string, ip string, username string, dir string) error {
	cmd := fmt.Sprintf("mkdir -p '%s'", shellEscape(dir))
	return nm.RunSSHCommand(jumpboxIp, ip, username, cmd)
}

func (nm *NodeManager) CopyFile(jumpboxIp string, ip string, username string, src string, dst string) error {
	client, err := nm.GetSFTPClient(jumpboxIp, ip, username)
	if err != nil {
		return fmt.Errorf("failed to get SSH client: %v", err)
	}
	defer util.IgnoreError(client.Close)

	srcFile, err := nm.FileIO.Open(src)
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
	checkCommand := fmt.Sprintf("command -v '%s' >/dev/null 2>&1", shellEscape(command))
	err := nm.RunSSHCommand("", n.ExternalIP, "root", checkCommand)
	return err == nil
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
	user := n.User
	if user == "" {
		user = "root"
	}

	if jumpbox == nil {
		err := nm.EnsureDirectoryExists("", n.ExternalIP, user, filepath.Dir(dst))
		if err != nil {
			return fmt.Errorf("failed to ensure directory exists: %w", err)
		}
		return nm.CopyFile("", n.ExternalIP, user, src, dst)
	}
	err := nm.EnsureDirectoryExists(jumpbox.ExternalIP, n.InternalIP, user, filepath.Dir(dst))
	if err != nil {
		return fmt.Errorf("failed to ensure directory exists: %w", err)
	}
	return nm.CopyFile(jumpbox.ExternalIP, n.InternalIP, user, src, dst)
}

func (n *Node) HasAcceptEnvConfigured(jumpbox *Node, nm *NodeManager) bool {
	checkCommand := "sudo grep -E '^AcceptEnv OMS_PORTAL_API_KEY' /etc/ssh/sshd_config >/dev/null 2>&1"
	err := n.RunSSHCommand(jumpbox, nm, "ubuntu", checkCommand)
	return err == nil
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
		return false
	}
	checkCommandAuthorizedKeys := "sudo grep -E '^no-port-forwarding' /root/.ssh/authorized_keys >/dev/null 2>&1"
	err = n.RunSSHCommand(jumpbox, nm, "ubuntu", checkCommandAuthorizedKeys)
	return err != nil
}

func (n *Node) HasFile(jumpbox *Node, nm *NodeManager, filePath string) bool {
	checkCommand := fmt.Sprintf("test -f '%s'", shellEscape(filePath))
	err := n.RunSSHCommand(jumpbox, nm, "ubuntu", checkCommand)
	return err == nil
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
	checkCommand := "sudo grep -E '^fs.inotify.max_user_watches=1048576' /etc/sysctl.conf >/dev/null 2>&1"
	err := n.RunSSHCommand(jumpbox, nm, "root", checkCommand)
	return err == nil
}

func (n *Node) ConfigureInotifyWatches(jumpbox *Node, nm *NodeManager) error {
	cmds := []string{
		"echo 'fs.inotify.max_user_watches=1048576' | sudo tee -a /etc/sysctl.conf",
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

func (n *Node) InstallK0s(nm *NodeManager, k0sBinaryPath string, k0sConfigPath string, force bool, nodeIP string) error {
	remoteK0sBinary := filepath.Join(remoteK0sDir, "k0s")
	remoteConfigPath := remoteK0sConfig

	user := n.User
	if user == "" {
		user = "root"
	}

	// Copy k0s binary to temp location first, then move with sudo
	log.Printf("Copying k0s binary to %s:%s", n.ExternalIP, tmpK0sBinary)
	if err := nm.CopyFile("", n.ExternalIP, user, k0sBinaryPath, tmpK0sBinary); err != nil {
		return fmt.Errorf("failed to copy k0s binary to temp: %w", err)
	}

	// Move to final location and make executable with sudo
	log.Printf("Moving k0s binary to %s", remoteK0sBinary)
	moveCmd := fmt.Sprintf("sudo mv '%s' '%s' && sudo chmod +x '%s'",
		shellEscape(tmpK0sBinary), shellEscape(remoteK0sBinary), shellEscape(remoteK0sBinary))
	if err := nm.RunSSHCommand("", n.ExternalIP, user, moveCmd); err != nil {
		return fmt.Errorf("failed to move and chmod k0s binary: %w", err)
	}

	if k0sConfigPath != "" {
		// Copy config to temp location first
		log.Printf("Copying k0s config to %s", tmpK0sConfig)
		if err := nm.CopyFile("", n.ExternalIP, user, k0sConfigPath, tmpK0sConfig); err != nil {
			return fmt.Errorf("failed to copy k0s config to temp: %w", err)
		}

		// Create /etc/k0s directory and move config with sudo
		log.Printf("Moving k0s config to %s", remoteConfigPath)
		setupConfigCmd := fmt.Sprintf("sudo mkdir -p /etc/k0s && sudo mv '%s' '%s' && sudo chmod 644 '%s'",
			shellEscape(tmpK0sConfig), shellEscape(remoteConfigPath), shellEscape(remoteConfigPath))
		if err := nm.RunSSHCommand("", n.ExternalIP, user, setupConfigCmd); err != nil {
			return fmt.Errorf("failed to setup k0s config: %w", err)
		}
	}

	installCmd := fmt.Sprintf("sudo '%s' install controller", shellEscape(remoteK0sBinary))
	if k0sConfigPath != "" {
		installCmd += fmt.Sprintf(" --config '%s'", shellEscape(remoteConfigPath))
	} else {
		installCmd += " --single"
	}

	installCmd += " --enable-worker"
	installCmd += " --no-taints"
	installCmd += fmt.Sprintf(" --kubelet-extra-args='--node-ip=%s'", shellEscape(nodeIP))

	if force {
		installCmd += " --force"
	}

	log.Printf("Installing k0s on %s", n.ExternalIP)
	if err := nm.RunSSHCommand("", n.ExternalIP, user, installCmd); err != nil {
		return fmt.Errorf("failed to install k0s: %w", err)
	}

	log.Printf("k0s successfully installed on %s", n.ExternalIP)
	log.Printf("You can start it using: ssh %s@%s 'sudo %s start'", user, n.ExternalIP, shellEscape(remoteK0sBinary))
	log.Printf("You can check the status using: ssh %s@%s 'sudo %s status'", user, n.ExternalIP, shellEscape(remoteK0sBinary))

	return nil
}
