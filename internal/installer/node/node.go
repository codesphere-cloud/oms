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

		fmt.Printf("Successfully read %d bytes from key file\\n", len(key))

		signer, err := ssh.ParsePrivateKey(key)
		if err == nil {
			fmt.Printf("Successfully parsed private key (type: %s)\\n", signer.PublicKey().Type())
			authMethods = append(authMethods, ssh.PublicKeys(signer))
			return authMethods, nil
		}

		fmt.Printf("Failed to parse private key: %v\\n", err)
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
	defer func() { _ = client.Close() }()
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session on target node (%s): %v", ip, err)
	}
	defer func() { _ = session.Close() }()

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
		jbClient, err := nm.connectToJumpbox(jumpboxIp, username)
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
	defer func() { _ = client.Close() }()

	srcFile, err := nm.FileIO.Open(src)
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

func (n *Node) CopyFile(nm *NodeManager, src string, dst string) error {
	user := n.User
	if user == "" {
		user = "root"
	}

	err := nm.EnsureDirectoryExists("", n.ExternalIP, user, filepath.Dir(dst))
	if err != nil {
		return fmt.Errorf("failed to ensure directory exists: %w", err)
	}
	return nm.CopyFile("", n.ExternalIP, user, src, dst)
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

func (n *Node) InstallK0s(nm *NodeManager, k0sBinaryPath string, k0sConfigPath string, force bool) error {
	remoteK0sDir := "/usr/local/bin"
	remoteK0sBinary := filepath.Join(remoteK0sDir, "k0s")
	remoteConfigPath := "/etc/k0s/k0s.yaml"

	user := n.User
	if user == "" {
		user = "root"
	}

	// Copy k0s binary to temp location first, then move with sudo
	tmpK0sBinary := "/tmp/k0s"
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
		tmpConfigPath := "/tmp/k0s-config.yaml"
		log.Printf("Copying k0s config to %s", tmpConfigPath)
		if err := nm.CopyFile("", n.ExternalIP, user, k0sConfigPath, tmpConfigPath); err != nil {
			return fmt.Errorf("failed to copy k0s config to temp: %w", err)
		}

		// Create /etc/k0s directory and move config with sudo
		log.Printf("Moving k0s config to %s", remoteConfigPath)
		setupConfigCmd := fmt.Sprintf("sudo mkdir -p /etc/k0s && sudo mv '%s' '%s' && sudo chmod 644 '%s'",
			shellEscape(tmpConfigPath), shellEscape(remoteConfigPath), shellEscape(remoteConfigPath))
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
