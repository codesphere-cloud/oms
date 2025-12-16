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
}

type NodeManager struct {
	FileIO  util.FileIO
	KeyPath string
}

func shellEscape(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

func (n *NodeManager) getHostKeyCallback() (ssh.HostKeyCallback, error) {
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

func (n *NodeManager) getAuthMethods() ([]ssh.AuthMethod, error) {
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

	if n.KeyPath != "" {
		fmt.Println("Falling back to private key file authentication.")

		key, err := n.FileIO.ReadFile(n.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key file %s: %v", n.KeyPath, err)
		}

		signer, err := ssh.ParsePrivateKey(key)
		if err == nil {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
			return authMethods, nil
		}
		if _, ok := err.(*ssh.PassphraseMissingError); ok {
			fmt.Printf("Enter passphrase for key '%s': ", n.KeyPath)
			passphraseBytes, err := term.ReadPassword(int(syscall.Stdin))
			fmt.Println()

			if err != nil {
				return nil, fmt.Errorf("failed to read passphrase: %v", err)
			}

			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, passphraseBytes)
			for i := range passphraseBytes {
				passphraseBytes[i] = 0
			}

			if err != nil {
				return nil, fmt.Errorf("failed to parse private key with passphrase: %v", err)
			}
			authMethods = append(authMethods, ssh.PublicKeys(signer))
			return authMethods, nil
		}
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no valid authentication methods configured. Check SSH_AUTH_SOCK and private key path")
	}

	return authMethods, nil
}

func (n *NodeManager) connectToJumpbox(ip, username string) (*ssh.Client, error) {
	authMethods, err := n.getAuthMethods()
	if err != nil {
		return nil, fmt.Errorf("jumpbox authentication setup failed: %v", err)
	}

	hostKeyCallback, err := n.getHostKeyCallback()
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

func (n *NodeManager) RunSSHCommand(jumpboxIp string, ip string, username string, command string) error {
	client, err := n.GetClient(jumpboxIp, ip, username)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}
	defer func() { _ = client.Close() }()
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session on jumpbox: %v", err)
	}
	defer func() { _ = session.Close() }()

	if err := n.forwardAgent(client, session); err != nil {
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

func (n *NodeManager) GetClient(jumpboxIp string, ip string, username string) (*ssh.Client, error) {

	authMethods, err := n.getAuthMethods()
	if err != nil {
		return nil, fmt.Errorf("failed to get authentication methods: %w", err)
	}

	hostKeyCallback, err := n.getHostKeyCallback()
	if err != nil {
		return nil, fmt.Errorf("failed to get host key callback: %w", err)
	}

	if jumpboxIp != "" {
		jbClient, err := n.connectToJumpbox(jumpboxIp, "ubuntu")
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

func (nm *NodeManager) EnsureDirectoryExists(ip string, username string, dir string) error {
	cmd := fmt.Sprintf("mkdir -p '%s'", shellEscape(dir))
	return nm.RunSSHCommand("", ip, username, cmd)
}

func (n *NodeManager) CopyFile(jumpboxIp string, ip string, username string, src string, dst string) error {
	client, err := n.GetSFTPClient(jumpboxIp, ip, username)
	if err != nil {
		return fmt.Errorf("failed to get SSH client: %v", err)
	}
	defer func() { _ = client.Close() }()

	srcFile, err := n.FileIO.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %v", src, err)
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := client.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file %s: %v", dst, err)
	}
	defer func() { _ = dstFile.Close() }()

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

func (n *Node) CopyFile(nm *NodeManager, src string, dst string) error {
	err := nm.EnsureDirectoryExists(n.ExternalIP, "root", filepath.Dir(dst))
	if err != nil {
		return fmt.Errorf("failed to ensure directory exists: %w", err)
	}
	return nm.CopyFile("", n.ExternalIP, "root", src, dst)
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

	return nm.RunSSHCommand(jumpbox.ExternalIP, n.InternalIP, "ubuntu", command)
}

func (n *Node) EnableRootLogin(jumpbox *Node, nm *NodeManager) error {
	cmds := []string{
		"sudo sed-i 's/^#\\?PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config",
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

func (n *Node) InstallK0s(nm *NodeManager, k0sBinaryPath string, k0sConfigPath string, force bool) error {
	remoteK0sDir := "/usr/local/bin"
	remoteK0sBinary := filepath.Join(remoteK0sDir, "k0s")
	remoteConfigPath := "/etc/k0s/k0s.yaml"

	log.Printf("Copying k0s binary to %s:%s", n.ExternalIP, remoteK0sBinary)
	if err := n.CopyFile(nm, k0sBinaryPath, remoteK0sBinary); err != nil {
		return fmt.Errorf("failed to copy k0s binary: %w", err)
	}

	log.Printf("Making k0s binary executable on %s", n.ExternalIP)
	chmodCmd := fmt.Sprintf("chmod +x '%s'", shellEscape(remoteK0sBinary))
	if err := nm.RunSSHCommand("", n.ExternalIP, "root", chmodCmd); err != nil {
		return fmt.Errorf("failed to make k0s binary executable: %w", err)
	}

	if k0sConfigPath != "" {
		log.Printf("Copying k0s config to %s:%s", n.ExternalIP, remoteConfigPath)
		if err := nm.EnsureDirectoryExists(n.ExternalIP, "root", "/etc/k0s"); err != nil {
			return fmt.Errorf("failed to create /etc/k0s directory: %w", err)
		}
		if err := nm.CopyFile("", n.ExternalIP, "root", k0sConfigPath, remoteConfigPath); err != nil {
			return fmt.Errorf("failed to copy k0s config: %w", err)
		}
	}

	installCmd := fmt.Sprintf("sudo '%s' install controller", shellEscape(remoteK0sBinary))
	if k0sConfigPath != "" {
		installCmd += fmt.Sprintf(" --config '%s'", shellEscape(remoteConfigPath))
	} else {
		installCmd += " --single"
	}
	if force {
		installCmd += " --force"
	}

	log.Printf("Installing k0s on %s", n.ExternalIP)
	if err := nm.RunSSHCommand("", n.ExternalIP, "root", installCmd); err != nil {
		return fmt.Errorf("failed to install k0s: %w", err)
	}

	log.Printf("k0s successfully installed on %s", n.ExternalIP)
	log.Printf("You can start it using: ssh root@%s 'sudo %s start'", n.ExternalIP, shellEscape(remoteK0sBinary))
	log.Printf("You can check the status using: ssh root@%s 'sudo %s status'", n.ExternalIP, shellEscape(remoteK0sBinary))

	return nil
}
