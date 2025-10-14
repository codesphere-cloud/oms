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

if command -v go >/dev/null 2>&1; then
    go run ./hack/gendocs/main.go
    echo "Docs generated into: $root/docs"
else
    echo "ERROR: 'go' binary not found in PATH. Install Go and retry." >&2
    exit 2
fi

echo "2/2: Updating licenses"

echo "Checking license tooling: go-licenses + addlicense"

export GOBIN="$(go env GOBIN 2>/dev/null || echo "$HOME/go/bin")"
export PATH="$GOBIN:$PATH"

need_install=()
if ! command -v go-licenses >/dev/null 2>&1; then
    need_install+=("github.com/google/go-licenses@latest")
fi
if ! command -v addlicense >/dev/null 2>&1; then
    need_install+=("github.com/google/addlicense@latest")
fi

if [ ${#need_install[@]} -ne 0 ]; then
    echo "Installing missing tools: ${need_install[*]}"
    for pkg in "${need_install[@]}"; do
        if command -v go >/dev/null 2>&1; then
            go install "$pkg"
        else
            echo "ERROR: 'go' binary not found; cannot install $pkg" >&2
            exit 2
        fi
    done
fi

echo "Generating NOTICE via go-licenses"
if command -v go-licenses >/dev/null 2>&1; then

    if ! go-licenses report --template .NOTICE.template ./... > NOTICE 2> >(grep -v "module .* has empty version, defaults to HEAD" >&2); then
        echo "go-licenses report failed" >&2
    fi
    echo "NOTICE generated/updated"
else
    echo "go-licenses not available; skipping NOTICE generation" >&2
fi

echo "Done."
