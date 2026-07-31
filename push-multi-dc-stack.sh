#!/usr/bin/env bash
# Force-push the rebased multi-dc stack (06..11) and retarget each PR onto its new base.
# The stack was rebased onto main after multi-dc-05 was squash-merged as #627.
#
# Usage:
#   ./push-multi-dc-stack.sh --dry-run   # verify and print what would happen
#   ./push-multi-dc-stack.sh             # push and retarget
#
# Written for bash 3.2 (the /bin/bash macOS ships), so no associative arrays.
set -eu

cd "$(dirname "$0")"

DRY_RUN=0
if [ "${1:-}" = "--dry-run" ]; then
  DRY_RUN=1
fi

run() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "    would run: $*"
  else
    "$@"
  fi
}

# branch:expected-tip:pr-base   — in stack order, bottom first.
STACK="
multi-dc-06-per-datacenter-dns:4923161:main
multi-dc-07-thread-datacenter-config:79b6465:multi-dc-06-per-datacenter-dns
multi-dc-08-shared-registry:d333fe2:multi-dc-07-thread-datacenter-config
multi-dc-09-multi-dc-bootstrap:2e8f44b:multi-dc-08-shared-registry
multi-dc-10-multi-dc-tests-docs:c170bc4:multi-dc-09-multi-dc-bootstrap
multi-dc-11-drop-legacy-env-mirror:0c170b3:multi-dc-10-multi-dc-tests-docs
"

echo "==> Verifying local state"
prev=main
echo "$STACK" | while IFS=: read -r branch want base; do
  [ -n "$branch" ] || continue

  have=$(git rev-parse --short "$branch" 2>/dev/null || echo MISSING)
  if [ "$have" != "$want" ]; then
    echo "ABORT: $branch is at $have, expected $want" >&2
    exit 1
  fi

  # Each branch must contain the one below it, or the PRs show each other's commits.
  if ! git merge-base --is-ancestor "$base" "$branch"; then
    echo "ABORT: $branch does not contain $base" >&2
    exit 1
  fi

  printf "    %-38s %s on %s\n" "$branch" "$have" "$base"
done

echo "==> Pushing (force-with-lease), bottom of the stack first"
echo "$STACK" | while IFS=: read -r branch want base; do
  [ -n "$branch" ] || continue
  echo "--- $branch"
  run git push --force-with-lease origin "$branch"
done

echo "==> Retargeting PR bases"
# multi-dc-05 is merged, so 06 now sits on main; the rest keep their predecessor.
echo "$STACK" | while IFS=: read -r branch want base; do
  [ -n "$branch" ] || continue
  echo "--- $branch -> base $base"
  run gh pr edit "$branch" --base "$base"
done

echo "==> Done"
if [ "$DRY_RUN" -eq 0 ]; then
  gh pr list --state open --search "multi-dc" \
    --json number,headRefName,baseRefName,mergeable \
    --template '{{range .}}{{printf "#%v  %s <- %s  (%s)\n" .number .baseRefName .headRefName .mergeable}}{{end}}'
fi
