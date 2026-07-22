#!/usr/bin/env bash
# Ownership-validated cleanup for a single acceptance run manifest.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

usage() {
  cat <<EOF
Usage: $(basename "$0") --manifest PATH

Remove only resources listed in an acceptance ownership manifest.
Refuses cleanup when the marker, temp prefix, PID list, or compose project checks fail.
EOF
}

MANIFEST=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --manifest)
      MANIFEST="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      acceptance_die "unknown argument: $1"
      ;;
  esac
done

[[ -n "${MANIFEST}" ]] || {
  usage
  exit 2
}

acceptance_cleanup_owned_resources "${MANIFEST}"
acceptance_log "cleanup.sh finished for ${MANIFEST}"
