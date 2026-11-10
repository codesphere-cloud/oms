// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package system

import (
	"io/fs"
	"os"
)

// hostProcFS returns the host's procfs, which lists the running processes and their parents.
func hostProcFS() fs.FS {
	return os.DirFS("/proc")
}
