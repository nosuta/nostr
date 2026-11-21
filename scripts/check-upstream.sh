#!/bin/bash
# List upstream changes that may matter to this client-only fork.
#
# The fork intentionally keeps server-side code out and diverges from
# upstream in build-tagged files, so a full merge is never the goal. This
# script turns "upstream sync" into a short review of client-side commits
# since UPSTREAM_BASE; cherry-pick only what is actually needed.
set -euo pipefail

cd "$(dirname "$0")/.."

BASE=$(cat UPSTREAM_BASE)
REMOTE=${UPSTREAM_REMOTE:-gitnostr.com}
REF=${UPSTREAM_REF:-${REMOTE}/master}

# Client-side paths we still carry. Server-side packages were removed on
# purpose and are excluded so they never show up in the review.
CLIENT_PATHS=(
    '.'
    ':(exclude)khatru/**'
    ':(exclude)eventstore/**'
    ':(exclude)sdk/**'
    ':(exclude)schema/**'
    ':(exclude)nip86/**'
)

if [[ "${1:-}" == "--fetch" ]]; then
    git fetch "$REMOTE" master
fi

if ! git rev-parse --verify --quiet "${REF}^{commit}" >/dev/null; then
    echo "check-upstream: no ref $REF; run: git fetch $REMOTE master" >&2
    exit 1
fi
if ! git rev-parse --verify --quiet "${BASE}^{commit}" >/dev/null; then
    echo "check-upstream: UPSTREAM_BASE $BASE is not in this repository" >&2
    exit 1
fi
if ! git merge-base --is-ancestor "$BASE" "$REF"; then
    echo "check-upstream: UPSTREAM_BASE is not an ancestor of $REF." >&2
    echo "  Fetch a newer upstream, or set UPSTREAM_REF to a ref containing $BASE." >&2
    exit 1
fi

echo "Upstream baseline: $BASE"
echo "Upstream ref:      $REF ($(git rev-parse --short "$REF"))"
echo
echo "Client-side commits since the baseline (server-side packages excluded):"
git log --oneline --no-merges "${BASE}..${REF}" -- "${CLIENT_PATHS[@]}"
echo
echo "Diffstat of those changes:"
git diff --stat "${BASE}..${REF}" -- "${CLIENT_PATHS[@]}"
echo
echo "Next steps: cherry-pick only the needed commits, re-apply the import"
echo "rewrite (fiatjaf.com/nostr -> github.com/nosuta/nostr), then run the"
echo "verification commands in FORK_MAINTENANCE.md. When the review is done,"
echo "update UPSTREAM_BASE to the new upstream head."
