#!/usr/bin/env bash
# Copyright (c) Codesphere Inc.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

echo "Running 'make docs'"
make docs

echo "Running 'make generate-license'"
make generate-license

make generate

echo "Done!"
