#!/usr/bin/env bash
# Shell self-tests for ownership-safe acceptance cleanup.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

FIXTURES="${SCRIPT_DIR}/fixtures"
PASS=0
FAIL=0

assert_fails() {
  local name="$1"
  shift
  if ( "$@" ); then
    printf 'FAIL %s (expected rejection)\n' "${name}" >&2
    FAIL=$((FAIL + 1))
  else
    printf 'PASS %s\n' "${name}"
    PASS=$((PASS + 1))
  fi
}

assert_passes() {
  local name="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    printf 'PASS %s\n' "${name}"
    PASS=$((PASS + 1))
  else
    printf 'FAIL %s (expected success)\n' "${name}" >&2
    FAIL=$((FAIL + 1))
  fi
}

test_reject_missing_marker() {
  ACCEPTANCE_EXPECT_COMPOSE_PROJECT=""
  ACCEPTANCE_EXPECT_PIDS_JSON=""
  ACCEPTANCE_EXPECT_PID=""
  acceptance_validate_manifest_file "${FIXTURES}/missing-marker-manifest.json"
}

test_reject_outside_prefix() {
  ACCEPTANCE_EXPECT_COMPOSE_PROJECT=""
  ACCEPTANCE_EXPECT_PID=""
  acceptance_validate_manifest_file "${FIXTURES}/outside-prefix-manifest.json"
}

test_reject_mismatched_compose() {
  acceptance_validate_manifest_file "${FIXTURES}/valid-manifest.json"
}

test_reject_mismatched_pid() {
  acceptance_validate_manifest_file "${FIXTURES}/valid-manifest.json"
}

test_valid_manifest_and_cleanup() {
  local tmp manifest home project
  tmp="$(mktemp -d "${ACCEPTANCE_TEMP_PREFIX}selftest-XXXXXX")"
  home="${tmp}/lab-home"
  project="${tmp}/project"
  manifest="${tmp}/ownership.json"
  mkdir -p "${home}" "${project}"
  python3 - "${manifest}" "${home}" "${project}" <<'PY'
import json, os, sys
manifest, home, project = sys.argv[1:4]
data = {
    "marker": os.environ["ACCEPTANCE_OWNERSHIP_MARKER"],
    "run_id": "selftest-valid",
    "profile": "selftest",
    "created_at": "2026-07-22T00:00:00Z",
    "camunda_lab_home": home,
    "project_root": project,
    "compose_project": "camunda-lab-acc-selftest-valid",
    "ui_port": 0,
    "devtools_port": 0,
    "chrome_profile": f"{home}/chrome",
    "pids": [],
}
with open(manifest, "w", encoding="utf-8") as fh:
    json.dump(data, fh)
PY
  ACCEPTANCE_EXPECT_COMPOSE_PROJECT=""
  ACCEPTANCE_EXPECT_PID=""
  acceptance_validate_manifest_file "${manifest}"
  "${SCRIPT_DIR}/cleanup.sh" --manifest "${manifest}"
  [[ ! -d "${home}" && ! -d "${project}" ]]
}

test_pid_cmdline_check() {
  local tmp pid
  tmp="$(mktemp -d "${ACCEPTANCE_TEMP_PREFIX}selftest-pid-XXXXXX")"
  (sleep 60) &
  pid=$!
  acceptance_verify_pid_cmdline "${pid}" "sleep 60"
  kill "${pid}" 2>/dev/null || true
  wait "${pid}" 2>/dev/null || true
  rm -rf "${tmp}"
}

test_reject_missing_pid_pattern() {
  local tmp manifest home project
  tmp="$(mktemp -d "${ACCEPTANCE_TEMP_PREFIX}selftest-pidpat-XXXXXX")"
  home="${tmp}/lab-home"
  project="${tmp}/project"
  manifest="${tmp}/ownership.json"
  mkdir -p "${home}" "${project}"
  python3 - "${manifest}" "${home}" "${project}" <<'PY'
import json, os, sys
manifest, home, project = sys.argv[1:4]
data = {
    "marker": os.environ["ACCEPTANCE_OWNERSHIP_MARKER"],
    "run_id": "selftest-missing-pid-pattern",
    "profile": "selftest",
    "created_at": "2026-07-22T00:00:00Z",
    "camunda_lab_home": home,
    "project_root": project,
    "compose_project": "camunda-lab-acc-selftest-missing-pid-pattern",
    "ui_port": 0,
    "devtools_port": 0,
    "chrome_profile": f"{home}/chrome",
    "pids": [99999],
}
with open(manifest, "w", encoding="utf-8") as fh:
    json.dump(data, fh)
PY
  ACCEPTANCE_EXPECT_COMPOSE_PROJECT=""
  ACCEPTANCE_EXPECT_PID=""
  acceptance_validate_manifest_file "${manifest}"
  rm -rf "${tmp}"
}

test_cleanup_rejects_pid_cmdline_mismatch() {
  local tmp manifest home project pid
  tmp="$(mktemp -d "${ACCEPTANCE_TEMP_PREFIX}selftest-pidkill-XXXXXX")"
  home="${tmp}/lab-home"
  project="${tmp}/project"
  manifest="${tmp}/ownership.json"
  mkdir -p "${home}" "${project}"
  (sleep 60) &
  pid=$!
  python3 - "${manifest}" "${home}" "${project}" "${pid}" <<'PY'
import json, os, sys
manifest, home, project, pid = sys.argv[1:5]
data = {
    "marker": os.environ["ACCEPTANCE_OWNERSHIP_MARKER"],
    "run_id": "selftest-pidkill",
    "profile": "selftest",
    "created_at": "2026-07-22T00:00:00Z",
    "camunda_lab_home": home,
    "project_root": project,
    "compose_project": "camunda-lab-acc-selftest-pidkill",
    "ui_port": 0,
    "devtools_port": 0,
    "chrome_profile": f"{home}/chrome",
    "pids": [{"pid": int(pid), "pattern": "definitely-not-this-cmdline"}],
}
with open(manifest, "w", encoding="utf-8") as fh:
    json.dump(data, fh)
PY
  acceptance_cleanup_owned_resources "${manifest}"
  kill "${pid}" 2>/dev/null || true
  wait "${pid}" 2>/dev/null || true
  rm -rf "${tmp}"
  return 1
}

test_reject_compose_prefix() {
  local tmp manifest home project
  tmp="$(mktemp -d "${ACCEPTANCE_TEMP_PREFIX}selftest-compose-XXXXXX")"
  home="${tmp}/lab-home"
  project="${tmp}/project"
  manifest="${tmp}/ownership.json"
  mkdir -p "${home}" "${project}"
  python3 - "${manifest}" "${home}" "${project}" <<'PY'
import json, os, sys
manifest, home, project = sys.argv[1:4]
data = {
    "marker": os.environ["ACCEPTANCE_OWNERSHIP_MARKER"],
    "run_id": "selftest-compose-prefix",
    "profile": "selftest",
    "created_at": "2026-07-22T00:00:00Z",
    "camunda_lab_home": home,
    "project_root": project,
    "compose_project": "not-our-compose-project",
    "ui_port": 0,
    "devtools_port": 0,
    "chrome_profile": f"{home}/chrome",
    "pids": [],
}
with open(manifest, "w", encoding="utf-8") as fh:
    json.dump(data, fh)
PY
  ACCEPTANCE_EXPECT_COMPOSE_PROJECT=""
  ACCEPTANCE_EXPECT_PID=""
  acceptance_validate_manifest_file "${manifest}"
  rm -rf "${tmp}"
}

test_no_docker_system_prune() {
  local found=0 file
  for file in "${SCRIPT_DIR}/lib.sh" "${SCRIPT_DIR}/cleanup.sh" \
    "${SCRIPT_DIR}/platform-toolkit-light.sh" "${SCRIPT_DIR}/platform-toolkit-full.sh"; do
    if grep -q "docker system prune" "${file}" 2>/dev/null; then
      found=1
    fi
  done
  [[ "${found}" -eq 0 ]]
}

main() {
  acceptance_require_cmd python3
  acceptance_log "running ownership self-tests"

  assert_fails reject-missing-marker test_reject_missing_marker
  assert_fails reject-outside-prefix test_reject_outside_prefix
  ACCEPTANCE_EXPECT_COMPOSE_PROJECT="camunda-lab-acc-expected"
  assert_fails reject-mismatched-compose test_reject_mismatched_compose
  ACCEPTANCE_EXPECT_COMPOSE_PROJECT=""
  ACCEPTANCE_EXPECT_PID="999999999"
  ACCEPTANCE_EXPECT_PID_PATTERN="acceptance-selftest"
  assert_fails reject-mismatched-pid test_reject_mismatched_pid
  ACCEPTANCE_EXPECT_PID=""
  assert_fails reject-missing-pid-pattern test_reject_missing_pid_pattern
  assert_fails reject-compose-prefix test_reject_compose_prefix
  assert_fails cleanup-rejects-pid-cmdline-mismatch test_cleanup_rejects_pid_cmdline_mismatch

  assert_passes valid-manifest-and-cleanup test_valid_manifest_and_cleanup
  assert_passes pid-cmdline-check test_pid_cmdline_check
  assert_passes no-docker-system-prune test_no_docker_system_prune

  printf '\nself-tests: %s passed, %s failed\n' "${PASS}" "${FAIL}"
  [[ "${FAIL}" -eq 0 ]]
}

main "$@"
