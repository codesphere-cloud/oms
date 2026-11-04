#!/usr/bin/env bash
# Copyright (c) Codesphere Inc.
# SPDX-License-Identifier: Apache-2.0

# Sync the Docusaurus output of hack/gendocs into the docs repository.
set -euo pipefail
shopt -s nullglob

if [[ $# -ne 2 ]]; then
  echo "Usage: $0 <docusaurus-docs> <docs-repo>" >&2
  exit 1
fi
source_dir=$1
reference=$2/docs/Private_Cloud/reference
target=$reference/oms-cli

if [[ ! -f "$source_dir/oms.md" || ! -f "$source_dir/_category_.json" ]]; then
  echo "No Docusaurus CLI reference found; run make docs-docusaurus first" >&2
  exit 1
fi
if [[ ! -d "$reference" ]]; then
  echo "Missing Private Cloud reference directory: $reference" >&2
  exit 1
fi

mkdir -p "$target"
# Remove obsolete commands only from the generator-owned section.
for stale in "$target"/oms*.md; do
  if [[ ! -f "$source_dir/${stale##*/}" ]]; then
    rm -- "$stale"
  fi
done
cp -- "$source_dir"/oms*.md "$source_dir/_category_.json" "$target/"

if [[ -f "$reference/oms-commands.mdx" ]]; then
  sed -i 's|\[github.com/codesphere-cloud/oms/tree/main/docs\](https://github.com/codesphere-cloud/oms/tree/main/docs)|[OMS CLI reference](./oms-cli/oms.md)|g' \
    "$reference/oms-commands.mdx"
fi
