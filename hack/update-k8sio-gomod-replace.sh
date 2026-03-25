#!/bin/bash
# Copyright (c) Codesphere Inc.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

# Script adapted from https://github.com/rzetelskik/local-csi-driver/blob/268dc56cad74645fab84bae1bd3c1d49cb97ab2b/hack/update-k8sio-gomod-replace.sh.
#
# This script resolves k8s.io staging module versions to use with k8s.io/kubernetes.
#
# NOTE: This script queries for the exact staging module version matching the
# kubernetes release (e.g., k8s.io/kubernetes v1.34.1 → k8s.io/api v0.34.1).
# This assumes Kubernetes follows its standard versioning policy where staging modules
# are versioned consistently with the main release (v1.X.Y → v0.X.Y for staging).
#
# If Kubernetes publishes hotfix patches to specific staging modules independently,
# this approach ensures reproducibility by pinning to the version tagged at release time,
# not the latest available patch.

VERSION=$(cat go.mod | grep "k8s.io/kubernetes v" | sed "s/^.*v\([0-9.]*\).*/\1/")
echo "Updating k8s.io go.mod replace directives for k8s.io/kubernetes@v$VERSION"

MODS=($(
    curl -sS https://raw.githubusercontent.com/kubernetes/kubernetes/v${VERSION}/go.mod |
    sed -n 's|.*k8s.io/\(.*\) => ./staging/src/k8s.io/.*|k8s.io/\1|p'
))

# Set concurrency level to the number of available CPU cores
CONCURRENCY=$(nproc)
export VERSION

# Staging modules use 0.X.Y version scheme (k8s.io/kubernetes v1.34.1 → staging modules v0.34.1)
STAGING_VERSION="0.${VERSION#*.}"

# Create an empty array to store replace directives
declare -a REPLACE_COMMANDS

# Function to generate replace directive for a module
generate_replace_command() {
    local MOD=$1
    local STAGING_VERSION=$2

    echo "-replace=${MOD}=${MOD}@v${STAGING_VERSION}"
}

# Export function for access in subshells run by xargs
export -f generate_replace_command

# Run in parallel to collect replace directives
echo "Collecting replace directives for ${#MODS[@]} modules concurrently (N=${CONCURRENCY})"
REPLACE_COMMANDS=($(printf "%s\n" "${MODS[@]}" | xargs -P "$CONCURRENCY" -n 1 -I {} bash -c 'generate_replace_command "$@"' _ {} "$STAGING_VERSION"))

# Apply each replace directive serially
for CMD in "${REPLACE_COMMANDS[@]}"; do
    echo "Applying go.mod $CMD"
    go mod edit "$CMD"
done

go get "k8s.io/kubernetes@v${VERSION}"
