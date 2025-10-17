#!/usr/bin/env bash
# Copyright (c) Codesphere Inc.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

repo_root=${1:-$(pwd)}

echo "Working directory: $repo_root"
cd "$repo_root"

echo "Running 'make docs'"
make docs

echo "Running 'make generate-license'"
make generate-license

echo "Done!"
