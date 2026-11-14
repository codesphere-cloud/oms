// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package files

import (
	"errors"
	"fmt"
	"net"
)

var (
	// ErrPublicIPRequired reports a missing codesphere.publicIp. Without it, Codesphere shows an
	// empty A record value and custom domains cannot be set up.
	ErrPublicIPRequired = errors.New("codesphere.publicIp is required (the IP custom domains' A records must point to; set it with --public-ip)")
	// ErrPublicIPInvalid reports a codesphere.publicIp that is not an IP address.
	ErrPublicIPInvalid = errors.New("invalid codesphere.publicIp")
	// ErrPublicIPUnreachable reports a codesphere.publicIp that no other machine can reach.
	ErrPublicIPUnreachable = errors.New("codesphere.publicIp is not reachable from other machines")
)

// ValidatePublicIP checks that the IP custom domains' A records point to is present and well-formed.
func (c CodesphereConfig) ValidatePublicIP() error {
	_, err := c.parsePublicIP()
	return err
}

// ValidateReachablePublicIP additionally rejects addresses that no other machine can reach, such
// as the 127.0.0.1 placeholder the config profiles default to. Private addresses stay allowed
// because on-premises and VM-based installs legitimately serve custom domains on them.
func (c CodesphereConfig) ValidateReachablePublicIP() error {
	ip, err := c.parsePublicIP()
	if err != nil {
		return err
	}

	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return fmt.Errorf("%w: %s; set it to the IP custom domains' A records must point to (e.g. the public gateway IP)", ErrPublicIPUnreachable, c.PublicIP)
	}

	return nil
}

func (c CodesphereConfig) parsePublicIP() (net.IP, error) {
	if c.PublicIP == "" {
		return nil, ErrPublicIPRequired
	}

	ip := net.ParseIP(c.PublicIP)
	if ip == nil {
		return nil, fmt.Errorf("%w: %s", ErrPublicIPInvalid, c.PublicIP)
	}

	return ip, nil
}
