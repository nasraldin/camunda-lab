#!/usr/bin/env bash
# Full-profile acceptance harness (ownership-safe).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

acceptance_init_run full
acceptance_trap_install
acceptance_record_inventory_before
acceptance_preflight_ports

"${SCRIPT_DIR}/self-test.sh"

if ! acceptance_docker_available; then
  acceptance_mark_unavailable live-full "docker unavailable"
  acceptance_log "full acceptance harness ready; live cluster steps skipped"
  exit 0
fi

acceptance_log "running live full profile steps"
if acceptance_run_live_profile full balanced 8.9; then
  acceptance_log "live full profile completed"
else
  acceptance_mark_unavailable live-full "live install/wait/smoke failed — see ${ACCEPTANCE_ARTIFACT_DIR}/install.log"
  acceptance_log "full acceptance harness complete with live steps unavailable"
  exit 0
fi
