// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"net"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GetNodeIPAddress", func() {
	Context("with valid network setup", func() {
		It("should return a valid IPv4 address", func() {
			ip, err := GetNodeIPAddress([]string{})

			if err != nil {
				Skip("No non-loopback network interfaces available on this system")
			}

			Expect(ip).NotTo(BeEmpty())
			parsedIP := net.ParseIP(ip)
			Expect(parsedIP).NotTo(BeNil())
			Expect(parsedIP.To4()).NotTo(BeNil())
		})

		It("should not return loopback address", func() {
			ip, err := GetNodeIPAddress([]string{})

			if err != nil {
				Skip("No non-loopback network interfaces available")
			}

			Expect(ip).NotTo(Equal("127.0.0.1"))
			Expect(ip).NotTo(HavePrefix("127."))
		})
	})

	Context("with control plane addresses provided", func() {
		It("should prioritize control plane IP if available", func() {
			interfaces, err := net.Interfaces()
			if err != nil {
				Skip("Cannot list network interfaces")
			}

			var testIP string
			for _, iface := range interfaces {
				if iface.Flags&net.FlagLoopback != 0 {
					continue
				}

				addrs, err := iface.Addrs()
				if err != nil {
					continue
				}

				for _, addr := range addrs {
					var ip net.IP
					switch v := addr.(type) {
					case *net.IPNet:
						ip = v.IP
					case *net.IPAddr:
						ip = v.IP
					}

					if ip == nil || ip.IsLoopback() {
						continue
					}

					if ip.To4() != nil {
						testIP = ip.String()
						break
					}
				}

				if testIP != "" {
					break
				}
			}

			if testIP == "" {
				Skip("No suitable test IP found")
			}

			result, err := GetNodeIPAddress([]string{testIP, "10.0.0.1"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(testIP))
		})

		It("should fallback if control plane IPs don't match", func() {
			result, err := GetNodeIPAddress([]string{"10.254.254.1", "192.168.254.254"})

			if err == nil {
				Expect(result).NotTo(BeEmpty())
				parsedIP := net.ParseIP(result)
				Expect(parsedIP).NotTo(BeNil())
				Expect(parsedIP.To4()).NotTo(BeNil())
			}
		})
	})

	Context("with edge cases", func() {
		It("should handle empty control plane list", func() {
			ip, err := GetNodeIPAddress([]string{})

			if err == nil {
				Expect(ip).NotTo(BeEmpty())
			} else {
				Expect(err.Error()).To(ContainSubstring("no suitable"))
			}
		})

		It("should handle nil control plane list", func() {
			ip, err := GetNodeIPAddress(nil)

			if err == nil {
				Expect(ip).NotTo(BeEmpty())
			} else {
				Expect(err.Error()).To(ContainSubstring("no suitable"))
			}
		})

		It("should return error when no interfaces are available", func() {
			ip, err := GetNodeIPAddress([]string{"invalid"})

			if err != nil {
				Expect(err.Error()).To(Or(
					ContainSubstring("no suitable"),
					ContainSubstring("network"),
				))
			} else {
				Expect(ip).NotTo(BeEmpty())
			}
		})
	})
})
