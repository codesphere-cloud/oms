// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package system_test

import (
	"testing/fstest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/system"
)

// procStat returns a procfs stat file for a process with the given command name and parent PID.
func procStat(comm string, ppid string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("1 (" + comm + ") S " + ppid + " 1 1 0 -1 4194560 0 0 0 0")}
}

var _ = Describe("DescendantPIDs", func() {
	It("returns the children and grandchildren of the installer, but not unrelated processes", func() {
		proc := fstest.MapFS{
			"1/stat":   procStat("systemd", "0"),
			"100/stat": procStat("oms", "1"),
			"200/stat": procStat("node", "100"),
			"300/stat": procStat("ssh", "200"),
			"301/stat": procStat("helm", "200"),
			"400/stat": procStat("helm-plugin", "301"),
			"500/stat": procStat("unrelated", "1"),
		}

		Expect(system.DescendantPIDs(proc, 200)).To(ConsistOf(300, 301, 400))
	})

	It("reads the parent PID after command names containing spaces and parentheses", func() {
		proc := fstest.MapFS{
			"200/stat": procStat("node", "100"),
			"300/stat": procStat("weird ) name (1)", "200"),
		}

		Expect(system.DescendantPIDs(proc, 200)).To(ConsistOf(300))
	})

	It("skips non-process entries and unparsable stat files of processes that exited mid-read", func() {
		proc := fstest.MapFS{
			"self/stat":    procStat("oms", "1"),
			"cpuinfo":      &fstest.MapFile{Data: []byte("processor: 0")},
			"300/stat":     procStat("ssh", "200"),
			"301/stat":     &fstest.MapFile{Data: []byte("")},
			"302/cmdline":  &fstest.MapFile{Data: []byte("helm")},
			"303/stat":     procStat("kubectl", "200"),
			"304/stat":     &fstest.MapFile{Data: []byte("304 (truncated")},
			"305/stat":     &fstest.MapFile{Data: []byte("305 (sh) S")},
			"306/stat":     &fstest.MapFile{Data: []byte("306 (sh) S notapid")},
			"200/children": &fstest.MapFile{Data: []byte("")},
		}

		Expect(system.DescendantPIDs(proc, 200)).To(ConsistOf(300, 303))
	})

	It("returns no descendants without procfs, as on macOS and Windows", func() {
		Expect(system.DescendantPIDs(nil, 200)).To(BeEmpty())
	})
})
