#!/usr/bin/env bash
# Copyright (c) Codesphere Inc.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

repo_root=${1:-$(pwd)}

if [[ ! -f "$repo_root/Makefile" ]]; then
    echo "ERROR: Makefile not found in $repo_root." >&2
    exit 2
fi

echo "Working directory: $repo_root"
cd "$repo_root"

if ! command -v make >/dev/null 2>&1; then
    echo "ERROR: 'make' not found in PATH. Install make and retry." >&2
    exit 2
fi

echo "Running 'make docs'"
make docs

echo "Running 'make generate-license'"
make generate-license

echo "Done!"
