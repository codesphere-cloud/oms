// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package system

import (
	"io/fs"
	"path"
	"strconv"
	"strings"
)

// descendantPIDs returns the PIDs of all children, grandchildren and further descendants of root,
// read from a procfs-style file system (one directory per PID with a "stat" file). It is best effort:
// processes that exit while the tree is read, or whose stat cannot be parsed, are skipped. A nil
// procFS, as on systems without procfs, yields no descendants.
func descendantPIDs(procFS fs.FS, root int) []int {
	if procFS == nil {
		return nil
	}

	children := childrenByParent(procFS)

	var descendants []int

	queue := []int{root}
	seen := map[int]bool{root: true}

	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]

		for _, child := range children[pid] {
			if seen[child] {
				continue
			}

			seen[child] = true
			descendants = append(descendants, child)
			queue = append(queue, child)
		}
	}

	return descendants
}

// childrenByParent maps every parent PID to the PIDs of its direct children.
func childrenByParent(procFS fs.FS) map[int][]int {
	entries, err := fs.ReadDir(procFS, ".")
	if err != nil {
		return nil
	}

	children := map[int][]int{}

	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || !entry.IsDir() {
			continue
		}

		stat, err := fs.ReadFile(procFS, path.Join(entry.Name(), "stat"))
		if err != nil {
			continue
		}

		if ppid, ok := parentPID(string(stat)); ok {
			children[ppid] = append(children[ppid], pid)
		}
	}

	return children
}

// parentPID extracts the parent PID from the content of /proc/<pid>/stat. The command name in the
// second field is wrapped in parentheses and may itself contain spaces and parentheses, so the
// fields are read after its last closing parenthesis: state, then the parent PID.
func parentPID(stat string) (int, bool) {
	end := strings.LastIndexByte(stat, ')')
	if end < 0 {
		return 0, false
	}

	fields := strings.Fields(stat[end+1:])
	if len(fields) < 2 {
		return 0, false
	}

	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, false
	}

	return ppid, true
}
