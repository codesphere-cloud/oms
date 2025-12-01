package node

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/codesphere-cloud/oms/internal/util"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
)

type Node struct {
	ExternalIP string `json:"external_ip"`
	InternalIP string `json:"internal_ip"`
}

type NodeManager struct {
	FileIO  util.FileIO
	KeyPath string
}

// getAuthMethods constructs a slice of ssh.AuthMethod, prioritizing the SSH agent.
func (n *NodeManager) getAuthMethods() ([]ssh.AuthMethod, error) {
	var authMethods []ssh.AuthMethod

	// --- 1. Try SSH Agent Authentication ---
	if authSocket := os.Getenv("SSH_AUTH_SOCK"); authSocket != "" {
		// Connect to the SSH agent's socket
		conn, err := net.Dial("unix", authSocket)
		if err == nil {
			// Create an Agent client and use it for authentication
			agentClient := agent.NewClient(conn)
			authMethods = append(authMethods, ssh.PublicKeysCallback(agentClient.Signers))
			fmt.Println("SSH Agent authentication method added.")
		} else {
			// Log agent connection failure, but don't fail the whole program yet
			fmt.Printf("Could not connect to SSH Agent (%s): %v\n", authSocket, err)
		}
	}

	// --- 2. Fallback to Private Key File Authentication (with passphrase support) ---
	if n.KeyPath != "" {
		fmt.Println("Falling back to private key file authentication.")

		// This logic is copied from the previous answer
		key, err := n.FileIO.ReadFile(n.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key file %s: %v", n.KeyPath, err)
		}

		signer, err := ssh.ParsePrivateKey(key)
		if err == nil {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		} else if _, ok := err.(*ssh.PassphraseMissingError); ok {
			// Key is encrypted, prompt for passphrase
			fmt.Printf("Enter passphrase for key '%s': ", n.KeyPath)
			passphraseBytes, err := term.ReadPassword(int(syscall.Stdin))
			fmt.Println()

			if err != nil {
				return nil, fmt.Errorf("failed to read passphrase: %v", err)
			}

			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, passphraseBytes)
			// Clear passphrase from memory
			for i := range passphraseBytes {
				passphraseBytes[i] = 0
			}

			if err != nil {
				return nil, fmt.Errorf("failed to parse private key with passphrase: %v", err)
			}
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		} else {
			return nil, fmt.Errorf("failed to parse private key: %v", err)
		}
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no valid authentication methods configured. Check SSH_AUTH_SOCK and private key path")
	}

	return authMethods, nil
}

// RunSSHCommand connects to the node, executes a command and streams the output
func (n *NodeManager) RunSSHCommand(ip string, username string, command string) error {
	authMethods, err := n.getAuthMethods()
	if err != nil {
		return fmt.Errorf("failed to get authentication methods: %w", err)
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
		return fmt.Errorf("failed to dial: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

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

func (n *Node) InstallOms(nm *NodeManager) error {
	remoteCommands := []string{
		"wget -qO- 'https://api.github.com/repos/codesphere-cloud/oms/releases/latest' | jq -r '.assets[] | select(.name | match(\"oms-cli.*linux_amd64\")) | .browser_download_url' | xargs wget -O oms-cl",
		"chmod +x oms-cli; sudo mv oms-cli /usr/local/bin/",
		"curl -LO https://github.com/getsops/sops/releases/download/v3.11.0/sops-v3.11.0.linux.amd64; sudo mv sops-v3.11.0.linux.amd64 /usr/local/bin/sops; sudo chmod +x /usr/local/bin/sops",
		"wget https://dl.filippo.io/age/latest?for=linux/amd64 -O age.tar.gz; tar -xvf age.tar.gz; sudo mv age/age /usr/local/bin/",
	}
	for _, cmd := range remoteCommands {
		err := nm.RunSSHCommand(n.ExternalIP, "ubuntu", cmd)
		if err != nil {
			return fmt.Errorf("failed to run remote command '%s': %w", cmd, err)
		}
	}
	return nil
}
