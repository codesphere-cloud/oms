// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package system

import "io/fs"

// hostProcFS returns nil on systems without procfs, so only the started process is signalled there.
func hostProcFS() fs.FS {
	return nil
}
