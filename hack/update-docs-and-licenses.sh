#!/usr/bin/env bash
# Copyright (c) Codesphere Inc.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail
IFS=$'\n\t'

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$here/.." && pwd)

echo "Working directory: $root"

cd "$root"

echo "1/2: Generating docs"
if command -v make >/dev/null 2>&1; then
    echo "Running 'make docs'"
    make docs
    echo "Docs generated into: $root/docs"
else
    echo "ERROR: 'make' not found in PATH. Install make and retry." >&2
    exit 2
fi

echo "2/2: Generating licenses via Makefile target 'generate-license'"

if command -v make >/dev/null 2>&1; then
    echo "Running 'make generate-license'"
    make generate-license
    echo "NOTICE and license headers generated/updated"
else
    echo "ERROR: 'make' not found in PATH. Install make and retry." >&2
    exit 2
fi

echo "Done!"
