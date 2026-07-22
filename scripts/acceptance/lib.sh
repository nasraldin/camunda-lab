#!/usr/bin/env bash
# Ownership-safe acceptance harness library.
# Never run broad docker prune; only touch resources recorded in the ownership manifest.

set -euo pipefail

: "${ACCEPTANCE_OWNERSHIP_MARKER:=camunda-lab-acceptance-v1}"
: "${ACCEPTANCE_TEMP_PREFIX:=${TMPDIR:-/tmp}/camunda-lab-acceptance-}"
: "${ACCEPTANCE_COMPOSE_PROJECT_PREFIX:=camunda-lab-acc-}"
export ACCEPTANCE_OWNERSHIP_MARKER ACCEPTANCE_TEMP_PREFIX ACCEPTANCE_COMPOSE_PROJECT_PREFIX

ACCEPTANCE_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ACCEPTANCE_REPO_ROOT="$(cd "${ACCEPTANCE_LIB_DIR}/../.." && pwd)"

ACCEPTANCE_CLEANUP_RAN=0
ACCEPTANCE_INVENTORY_BEFORE=""
ACCEPTANCE_INVENTORY_AFTER=""

acceptance_log() {
  printf '[acceptance] %s\n' "$*" >&2
}

acceptance_die() {
  acceptance_log "ERROR: $*"
  exit 1
}

acceptance_require_cmd() {
  command -v "$1" >/dev/null 2>&1 || acceptance_die "required command not found: $1"
}

acceptance_normalize_path() {
  local path="$1"
  if [[ -z "${path}" ]]; then
    printf ''
    return 0
  fi
  local dir base
  dir="$(dirname "${path}")"
  base="$(basename "${path}")"
  if [[ -d "${dir}" ]]; then
    (cd "${dir}" && printf '%s/%s\n' "$(pwd -P)" "${base}")
  else
    printf '%s\n' "${path}"
  fi
}

acceptance_is_under_temp_prefix() {
  local path="$1"
  local normalized prefix
  normalized="$(acceptance_normalize_path "${path}")"
  prefix="$(acceptance_normalize_path "${ACCEPTANCE_TEMP_PREFIX}")"
  [[ "${normalized}" == "${prefix}"* ]] && return 0
  [[ "${normalized}" == /tmp/camunda-lab-acceptance-* ]] && return 0
  [[ "${normalized}" == /private/tmp/camunda-lab-acceptance-* ]] && return 0
  return 1
}

acceptance_manifest_path() {
  printf '%s/ownership.json\n' "${ACCEPTANCE_ARTIFACT_DIR}"
}

acceptance_json_field() {
  local file="$1"
  local field="$2"
  python3 - "$file" "$field" <<'PY'
import json, sys
path, field = sys.argv[1], sys.argv[2]
with open(path, encoding="utf-8") as fh:
    data = json.load(fh)
cur = data
for part in field.split("."):
    if isinstance(cur, dict):
        cur = cur.get(part)
    else:
        cur = None
        break
if cur is None:
    sys.exit(2)
if isinstance(cur, bool):
    print("true" if cur else "false")
elif isinstance(cur, (int, float)):
    print(cur)
elif isinstance(cur, list):
    print(json.dumps(cur))
else:
    print(cur)
PY
}

acceptance_write_manifest() {
  mkdir -p "${ACCEPTANCE_ARTIFACT_DIR}"
  python3 - <<PY
import json, os, time
manifest = {
    "marker": os.environ["ACCEPTANCE_OWNERSHIP_MARKER"],
    "run_id": os.environ["ACCEPTANCE_RUN_ID"],
    "profile": os.environ.get("ACCEPTANCE_PROFILE", ""),
    "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    "camunda_lab_home": os.environ["ACCEPTANCE_CAMUNDA_LAB_HOME"],
    "project_root": os.environ["ACCEPTANCE_PROJECT_ROOT"],
    "compose_project": os.environ["ACCEPTANCE_COMPOSE_PROJECT"],
    "ui_port": int(os.environ["ACCEPTANCE_UI_PORT"]),
    "devtools_port": int(os.environ["ACCEPTANCE_DEVTOOLS_PORT"]),
    "chrome_profile": os.environ["ACCEPTANCE_CHROME_PROFILE"],
    "pids": json.loads(os.environ.get("ACCEPTANCE_PIDS_JSON", "[]")),
}
with open(os.environ["ACCEPTANCE_MANIFEST_FILE"], "w", encoding="utf-8") as fh:
    json.dump(manifest, fh, indent=2)
    fh.write("\n")
PY
}

acceptance_validate_manifest_core() {
  local manifest_file="${1:-$(acceptance_manifest_path)}"
  local marker home project compose pids_json

  [[ -f "${manifest_file}" ]] || acceptance_die "ownership manifest missing: ${manifest_file}"

  marker="$(acceptance_json_field "${manifest_file}" marker 2>/dev/null || true)"
  [[ "${marker}" == "${ACCEPTANCE_OWNERSHIP_MARKER}" ]] || acceptance_die "ownership marker mismatch or missing in ${manifest_file}"

  home="$(acceptance_json_field "${manifest_file}" camunda_lab_home)"
  project="$(acceptance_json_field "${manifest_file}" project_root)"
  compose="$(acceptance_json_field "${manifest_file}" compose_project)"

  acceptance_is_under_temp_prefix "${home}" || acceptance_die "camunda_lab_home outside accepted temp prefix: ${home}"
  acceptance_is_under_temp_prefix "${project}" || acceptance_die "project_root outside accepted temp prefix: ${project}"

  [[ "${compose}" == "${ACCEPTANCE_COMPOSE_PROJECT_PREFIX}"* ]] || acceptance_die "compose_project outside harness prefix ${ACCEPTANCE_COMPOSE_PROJECT_PREFIX}: ${compose}"

  if [[ -n "${ACCEPTANCE_EXPECT_COMPOSE_PROJECT:-}" ]]; then
    [[ "${compose}" == "${ACCEPTANCE_EXPECT_COMPOSE_PROJECT}" ]] || acceptance_die "compose project mismatch: ${compose} != ${ACCEPTANCE_EXPECT_COMPOSE_PROJECT}"
  fi

  pids_json="$(acceptance_json_field "${manifest_file}" pids 2>/dev/null || printf '[]')"
  if [[ -n "${ACCEPTANCE_EXPECT_PIDS_JSON:-}" && "${pids_json}" != "${ACCEPTANCE_EXPECT_PIDS_JSON}" ]]; then
    acceptance_die "tracked PID list mismatch"
  fi

  acceptance_validate_manifest_pids "${pids_json}"

  if [[ -n "${ACCEPTANCE_EXPECT_PID:-}" ]]; then
    local found=0 pid pattern
    while IFS=$'\t' read -r pid pattern; do
      if [[ "${pid}" == "${ACCEPTANCE_EXPECT_PID}" ]]; then
        found=1
        [[ "${pattern}" == "${ACCEPTANCE_EXPECT_PID_PATTERN:-acceptance-selftest}" ]] || acceptance_die "expected PID pattern mismatch for ${ACCEPTANCE_EXPECT_PID}"
        acceptance_verify_pid_cmdline "${ACCEPTANCE_EXPECT_PID}" "${pattern}"
        break
      fi
    done < <(acceptance_iter_manifest_pids "${pids_json}")
    [[ "${found}" -eq 1 ]] || acceptance_die "expected PID ${ACCEPTANCE_EXPECT_PID} not listed in manifest"
  fi
}

acceptance_validate_manifest_file() {
  local manifest_file="${1:-$(acceptance_manifest_path)}"
  acceptance_validate_manifest_core "${manifest_file}"
}

acceptance_find_free_port() {
  python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
}

acceptance_port_in_use() {
  local port="$1"
  python3 - "$port" <<'PY'
import socket, sys
port = int(sys.argv[1])
s = socket.socket()
try:
    s.bind(("127.0.0.1", port))
except OSError:
    sys.exit(0)
else:
    sys.exit(1)
finally:
    s.close()
PY
}

acceptance_preflight_ports() {
  local port
  for port in "${ACCEPTANCE_UI_PORT}" "${ACCEPTANCE_DEVTOOLS_PORT}"; do
    if acceptance_port_in_use "${port}"; then
      acceptance_die "preflight failed: port ${port} is already in use (will not stop foreign processes)"
    fi
  done
  acceptance_log "preflight ports free: ui=${ACCEPTANCE_UI_PORT} devtools=${ACCEPTANCE_DEVTOOLS_PORT}"
}

acceptance_generate_run_id() {
  printf '%s-%s' "$(date +%Y%m%d%H%M%S)" "$$"
}

acceptance_init_run() {
  local profile="${1:-light}"
  ACCEPTANCE_PROFILE="${profile}"
  ACCEPTANCE_RUN_ID="${ACCEPTANCE_RUN_ID:-$(acceptance_generate_run_id)}"
  ACCEPTANCE_CAMUNDA_LAB_HOME="${ACCEPTANCE_TEMP_PREFIX}${ACCEPTANCE_RUN_ID}/lab-home"
  ACCEPTANCE_PROJECT_ROOT="${ACCEPTANCE_TEMP_PREFIX}${ACCEPTANCE_RUN_ID}/project"
  ACCEPTANCE_COMPOSE_PROJECT="camunda-lab-acc-${ACCEPTANCE_RUN_ID}"
  ACCEPTANCE_UI_PORT="${ACCEPTANCE_UI_PORT:-$(acceptance_find_free_port)}"
  ACCEPTANCE_DEVTOOLS_PORT="${ACCEPTANCE_DEVTOOLS_PORT:-$(acceptance_find_free_port)}"
  ACCEPTANCE_CHROME_PROFILE="${ACCEPTANCE_TEMP_PREFIX}${ACCEPTANCE_RUN_ID}/chrome"
  ACCEPTANCE_ARTIFACT_DIR="${ACCEPTANCE_TEMP_PREFIX}${ACCEPTANCE_RUN_ID}/artifacts"
  ACCEPTANCE_PIDS_JSON="${ACCEPTANCE_PIDS_JSON:-[]}"
  export ACCEPTANCE_PROFILE ACCEPTANCE_RUN_ID ACCEPTANCE_CAMUNDA_LAB_HOME
  export ACCEPTANCE_PROJECT_ROOT ACCEPTANCE_COMPOSE_PROJECT ACCEPTANCE_UI_PORT
  export ACCEPTANCE_DEVTOOLS_PORT ACCEPTANCE_CHROME_PROFILE ACCEPTANCE_ARTIFACT_DIR
  export ACCEPTANCE_PIDS_JSON

  mkdir -p "${ACCEPTANCE_CAMUNDA_LAB_HOME}" "${ACCEPTANCE_PROJECT_ROOT}" "${ACCEPTANCE_CHROME_PROFILE}" "${ACCEPTANCE_ARTIFACT_DIR}"
  export ACCEPTANCE_MANIFEST_FILE
  ACCEPTANCE_MANIFEST_FILE="$(acceptance_manifest_path)"
  acceptance_write_manifest
  acceptance_log "run ${ACCEPTANCE_RUN_ID} profile=${profile} home=${ACCEPTANCE_CAMUNDA_LAB_HOME} compose=${ACCEPTANCE_COMPOSE_PROJECT}"
}

acceptance_iter_manifest_pids() {
  local pids_json="$1"
  python3 - "${pids_json}" <<'PY'
import json, sys
for entry in json.loads(sys.argv[1] or "[]"):
    if isinstance(entry, dict):
        pid = entry.get("pid")
        pattern = entry.get("pattern", "")
    else:
        pid = entry
        pattern = ""
    if pid is None:
        continue
    print(f"{int(pid)}\t{pattern}")
PY
}

acceptance_validate_manifest_pids() {
  local pids_json="$1"
  local pid pattern
  while IFS=$'\t' read -r pid pattern; do
    [[ -n "${pattern}" ]] || acceptance_die "PID ${pid} missing cmdline pattern in ownership manifest"
  done < <(acceptance_iter_manifest_pids "${pids_json}")
}

acceptance_register_pid() {
  local pid="$1"
  local pattern="${2:-}"
  [[ -n "${pattern}" ]] || acceptance_die "PID ${pid} registration requires cmdline pattern"
  ACCEPTANCE_PIDS_JSON="$(python3 - <<PY
import json, os
pids = json.loads(os.environ.get("ACCEPTANCE_PIDS_JSON", "[]"))
pid = int("${pid}")
pattern = """${pattern}"""
updated = [entry for entry in pids if (entry.get("pid") if isinstance(entry, dict) else entry) != pid]
updated.append({"pid": pid, "pattern": pattern})
print(json.dumps(updated))
PY
)"
  export ACCEPTANCE_PIDS_JSON
  acceptance_write_manifest
}

acceptance_verify_pid_cmdline() {
  local pid="$1"
  local pattern="$2"
  local cmdline=""
  if [[ ! -d "/proc/${pid}" && "$(uname -s)" != "Linux" ]]; then
    cmdline="$(ps -p "${pid}" -o command= 2>/dev/null || true)"
  else
    cmdline="$(ps -p "${pid}" -o args= 2>/dev/null || true)"
  fi
  [[ -n "${cmdline}" ]] || acceptance_die "PID ${pid} is not running"
  [[ "${cmdline}" == *"${pattern}"* ]] || acceptance_die "PID ${pid} cmdline does not match expected pattern '${pattern}': ${cmdline}"
}

acceptance_inventory_snapshot() {
  local label="$1"
  local out="${ACCEPTANCE_ARTIFACT_DIR}/inventory-${label}.txt"
  {
    printf 'timestamp=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    printf 'compose_project=%s\n' "${ACCEPTANCE_COMPOSE_PROJECT:-}"
    if command -v docker >/dev/null 2>&1; then
      docker ps -a --filter "label=com.docker.compose.project=${ACCEPTANCE_COMPOSE_PROJECT:-__none__}" --format '{{.ID}}\t{{.Names}}\t{{.Status}}' 2>/dev/null || true
    fi
  } >"${out}"
  printf '%s' "${out}"
}

acceptance_record_inventory_before() {
  ACCEPTANCE_INVENTORY_BEFORE="$(acceptance_inventory_snapshot before)"
}

acceptance_record_inventory_after() {
  ACCEPTANCE_INVENTORY_AFTER="$(acceptance_inventory_snapshot after)"
}

acceptance_mark_unavailable() {
  local step="$1"
  local reason="$2"
  local marker="${ACCEPTANCE_ARTIFACT_DIR}/unavailable-${step}.txt"
  printf 'step=%s\nreason=%s\nrun_id=%s\n' "${step}" "${reason}" "${ACCEPTANCE_RUN_ID}" >"${marker}"
  acceptance_log "UNAVAILABLE: ${step} — ${reason}"
}

acceptance_docker_available() {
  command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1
}

acceptance_cleanup_compose_project() {
  local project="$1"
  [[ -n "${project}" ]] || return 0
  command -v docker >/dev/null 2>&1 || return 0
  local ids
  ids="$(docker ps -aq --filter "label=com.docker.compose.project=${project}" 2>/dev/null || true)"
  if [[ -n "${ids}" ]]; then
    acceptance_log "stopping compose project containers: ${project}"
    # shellcheck disable=SC2086
    docker stop ${ids} >/dev/null 2>&1 || true
    # shellcheck disable=SC2086
    docker rm ${ids} >/dev/null 2>&1 || true
  fi
  local vols
  vols="$(docker volume ls -q --filter "label=com.docker.compose.project=${project}" 2>/dev/null || true)"
  if [[ -n "${vols}" ]]; then
    acceptance_log "removing compose project volumes: ${project}"
    # shellcheck disable=SC2086
    docker volume rm ${vols} >/dev/null 2>&1 || true
  fi
}

acceptance_cleanup_pids() {
  local pids_json="$1"
  local pid pattern
  acceptance_validate_manifest_pids "${pids_json}"
  while IFS=$'\t' read -r pid pattern; do
    if kill -0 "${pid}" 2>/dev/null; then
      acceptance_verify_pid_cmdline "${pid}" "${pattern}"
      acceptance_log "terminating owned PID ${pid}"
      kill "${pid}" 2>/dev/null || true
    fi
  done < <(acceptance_iter_manifest_pids "${pids_json}")
}

acceptance_cleanup_owned_resources() {
  local manifest_file="${1:-$(acceptance_manifest_path)}"
  [[ "${ACCEPTANCE_CLEANUP_RAN}" -eq 1 ]] && return 0
  ACCEPTANCE_CLEANUP_RAN=1

  [[ -f "${manifest_file}" ]] || {
    acceptance_log "cleanup skipped: no manifest at ${manifest_file}"
    return 0
  }

  acceptance_validate_manifest_core "${manifest_file}"

  local compose pids_json home project chrome
  compose="$(acceptance_json_field "${manifest_file}" compose_project)"
  pids_json="$(acceptance_json_field "${manifest_file}" pids 2>/dev/null || printf '[]')"
  home="$(acceptance_json_field "${manifest_file}" camunda_lab_home)"
  project="$(acceptance_json_field "${manifest_file}" project_root)"
  chrome="$(acceptance_json_field "${manifest_file}" chrome_profile 2>/dev/null || true)"

  acceptance_cleanup_pids "${pids_json}"
  acceptance_cleanup_compose_project "${compose}"

  if acceptance_is_under_temp_prefix "${home}" && [[ -d "${home}" ]]; then
    rm -rf "${home}"
  fi
  if acceptance_is_under_temp_prefix "${project}" && [[ -d "${project}" ]]; then
    rm -rf "${project}"
  fi
  if [[ -n "${chrome}" ]] && acceptance_is_under_temp_prefix "${chrome}" && [[ -d "${chrome}" ]]; then
    rm -rf "${chrome}"
  fi

  acceptance_log "cleanup complete for run manifest ${manifest_file}"
}

acceptance_on_exit() {
  local code=$?
  if [[ -n "${ACCEPTANCE_ARTIFACT_DIR:-}" ]]; then
    acceptance_record_inventory_after >/dev/null 2>&1 || true
  fi
  if [[ -f "$(acceptance_manifest_path 2>/dev/null || echo /dev/null)" ]]; then
    acceptance_cleanup_owned_resources "$(acceptance_manifest_path)" || true
  fi
  exit "${code}"
}

acceptance_on_signal() {
  local sig="$1"
  acceptance_log "received signal ${sig}; running owned cleanup"
  acceptance_on_exit
}

acceptance_trap_install() {
  trap 'acceptance_on_signal INT' INT
  trap 'acceptance_on_signal TERM' TERM
  trap 'acceptance_on_exit' EXIT
}

acceptance_build_camunda() {
  (cd "${ACCEPTANCE_REPO_ROOT}" && go build -o bin/camunda ./cmd/camunda)
}

acceptance_camunda_bin() {
  printf '%s/bin/camunda\n' "${ACCEPTANCE_REPO_ROOT}"
}

acceptance_write_lab_config() {
  local version="$1"
  local profile="$2"
  local resources="$3"
  mkdir -p "${ACCEPTANCE_CAMUNDA_LAB_HOME}"
  cat >"${ACCEPTANCE_CAMUNDA_LAB_HOME}/config.yaml" <<EOF
version: "${version}"
profile: ${profile}
resources: ${resources}
host: localhost
compose_project: ${ACCEPTANCE_COMPOSE_PROJECT}
ai:
  enabled: false
EOF
}

acceptance_run_live_profile() {
  local profile="$1"
  local resources="${2:-balanced}"
  local version="${3:-8.8}"
  local bin
  bin="$(acceptance_camunda_bin)"

  export CAMUNDA_LAB_HOME="${ACCEPTANCE_CAMUNDA_LAB_HOME}"
  export CAMUNDA_LAB_UI_PORT="${ACCEPTANCE_UI_PORT}"

  acceptance_build_camunda
  mkdir -p "${ACCEPTANCE_PROJECT_ROOT}"
  acceptance_write_lab_config "${version}" "${profile}" "${resources}"
  (cd "${ACCEPTANCE_PROJECT_ROOT}" && "${bin}" init --yes >/dev/null 2>&1 || true)

  "${bin}" install --version "${version}" --profile "${profile}" --resources "${resources}" --yes \
    >"${ACCEPTANCE_ARTIFACT_DIR}/install.log" 2>&1 || return 1
  "${bin}" wait --timeout 15m >>"${ACCEPTANCE_ARTIFACT_DIR}/install.log" 2>&1 || return 1
  "${bin}" smoke >>"${ACCEPTANCE_ARTIFACT_DIR}/install.log" 2>&1 || return 1
  "${bin}" down >>"${ACCEPTANCE_ARTIFACT_DIR}/install.log" 2>&1 || return 1
}
