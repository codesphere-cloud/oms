#!/bin/bash
# Copyright (c) Codesphere Inc.
# SPDX-License-Identifier: Apache-2.0


set -eu
BRANCH=$(git branch --show-current)
if [[ "$BRANCH" != "main" ]]; then
  echo "This script should only run on main branch, not $BRANCH"
  exit 1
fi
TAG=$(git tag | sort -uV | tail -n 1 | xargs echo -n)
LIST="HEAD"
if [[ "$TAG" != "" ]]; then
  LIST="$TAG..HEAD"
fi
echo $LIST
LOG=$(git log "$LIST"  --pretty=format:%s)
BREAK=$(grep -e '!' >/dev/null <<< "$LOG"; echo $?)
FEAT=$(grep -e '^feat' >/dev/null <<< "$LOG"; echo $?)
FIX=$(grep -e '^fix' >/dev/null <<< "$LOG"; echo $?)

echo "Latest tag: $TAG"
echo "------"
echo "Relevant changes:"
echo "$LOG"
echo "------"

if [[ "$TAG" == "" ]]; then
  TAG=v0.0.0
fi
NEWTAG="$TAG"

if [[ $BREAK -eq 0 ]]; then
  echo "Breaking change! Increasing major."
  NEWTAG=$(sed -r 's/v([0-9]+)\.[0-9]+\.[0-9]+/echo "v$((\1+1)).0.0"/e' <<< "$TAG")
elif [[ $FEAT -eq 0 ]]; then
  echo "New feature! Increasing minor."
  NEWTAG=$(sed -r 's/v([0-9])+\.([0-9]+)\.[0-9]+/echo "v\1.$((\2+1)).0"/e' <<< "$TAG")
elif [[ $FIX -eq 0 ]]; then
  echo "New fix! Increasing patch."
  NEWTAG=$(sed -r 's/v([0-9])+\.([0-9]+)\.([0-9]+)/echo "v\1.\2.$((\3+1))"/e' <<< "$TAG")
fi

if [[ $NEWTAG == $TAG ]]; then
  echo "Nothing to tag."
  exit 0
fi

echo "Tagging $NEWTAG"
git tag "$NEWTAG"
git push origin "$NEWTAG"

echo "Triggering release of version $NEWTAG"
go install github.com/goreleaser/goreleaser/v2@latest
goreleaser release --clean
