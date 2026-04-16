// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"
	"net"
	"strings"
)

// SplitMonitorEndpointEntries splits a raw Ceph monitor endpoint string by
// commas while respecting bracket-delimited groups (e.g. msgr2 format).
//
// For example, given the Rook monitor endpoint string:
//
//	"a=10.0.0.1:6789,b=[v2:10.0.0.2:3300/0,v1:10.0.0.2:6789/0]"
//
// it returns:
//
//	["a=10.0.0.1:6789", "b=[v2:10.0.0.2:3300/0,v1:10.0.0.2:6789/0]"]
func SplitMonitorEndpointEntries(rawEndpoints string) []string {
	entries := []string{}
	var current strings.Builder
	bracketDepth := 0

	for _, r := range rawEndpoints {
		switch r {
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case ',':
			if bracketDepth == 0 {
				entries = append(entries, current.String())
				current.Reset()
				continue
			}
		}

		current.WriteRune(r)
	}

	if current.Len() > 0 {
		entries = append(entries, current.String())
	}

	return entries
}

// ParseMonitorEndpointHost extracts the host (and optional port) from a single
// Ceph monitor endpoint entry. It handles plain host:port, msgr2 bracket
// notation, and name=value prefixes.
//
// Examples:
//
//	ParseMonitorEndpointHost("a=10.0.0.10:6789")                                  // "10.0.0.10:6789"
//	ParseMonitorEndpointHost("a=[v2:10.0.0.10:3300/0,v1:10.0.0.10:6789/0]")       // "10.0.0.10:3300"
//	ParseMonitorEndpointHost("a=rook-ceph-mon-a.rook-ceph.svc:6789")              // "rook-ceph-mon-a.rook-ceph.svc:6789"
func ParseMonitorEndpointHost(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", fmt.Errorf("empty endpoint")
	}

	if separator := strings.Index(endpoint, "="); separator >= 0 {
		endpoint = strings.TrimSpace(endpoint[separator+1:])
	}

	if strings.HasPrefix(endpoint, "[") && strings.HasSuffix(endpoint, "]") {
		for _, candidate := range strings.Split(endpoint[1:len(endpoint)-1], ",") {
			host, err := ParseMonitorEndpointHost(candidate)
			if err == nil {
				return host, nil
			}
		}
		return "", fmt.Errorf("no valid monitor host found in %q", endpoint)
	}

	endpoint = strings.TrimPrefix(endpoint, "v1:")
	endpoint = strings.TrimPrefix(endpoint, "v2:")
	if slash := strings.Index(endpoint, "/"); slash >= 0 {
		endpoint = endpoint[:slash]
	}

	if host, port, err := net.SplitHostPort(endpoint); err == nil {
		host = strings.Trim(host, "[]")
		if host == "" {
			return "", fmt.Errorf("endpoint %q does not contain a valid host", endpoint)
		}
		if port == "" {
			return host, nil
		}
		return net.JoinHostPort(host, port), nil
	}

	trimmed := strings.Trim(endpoint, "[]")
	if trimmed == "" {
		return "", fmt.Errorf("endpoint %q does not contain a valid host", endpoint)
	}

	if ip := net.ParseIP(trimmed); ip != nil {
		return ip.String(), nil
	}

	if strings.Contains(trimmed, ":") {
		return "", fmt.Errorf("endpoint %q contains an unparseable host:port", endpoint)
	}

	return trimmed, nil
}
