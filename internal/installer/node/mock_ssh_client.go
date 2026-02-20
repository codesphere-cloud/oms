// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"errors"
	"io"

	"golang.org/x/crypto/ssh"
)

// MockSSHClientFactory is a test implementation of SSHClientFactory.
type MockSSHClientFactory struct {
	DialFunc func(network, addr string, config *ssh.ClientConfig) (*ssh.Client, error)
}

func (m *MockSSHClientFactory) Dial(network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	if m.DialFunc != nil {
		return m.DialFunc(network, addr, config)
	}
	return nil, errors.New("mock SSH client factory not configured")
}

// MockSSHSession is a mock SSH session for testing.
type MockSSHSession struct {
	StartFunc  func(cmd string) error
	WaitFunc   func() error
	CloseFunc  func() error
	SetenvFunc func(name, value string) error
	Stdout     io.Writer
	Stderr     io.Writer
}

func (m *MockSSHSession) Start(cmd string) error {
	if m.StartFunc != nil {
		return m.StartFunc(cmd)
	}
	return nil
}

func (m *MockSSHSession) Wait() error {
	if m.WaitFunc != nil {
		return m.WaitFunc()
	}
	return nil
}

func (m *MockSSHSession) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

func (m *MockSSHSession) Setenv(name, value string) error {
	if m.SetenvFunc != nil {
		return m.SetenvFunc(name, value)
	}
	return nil
}
