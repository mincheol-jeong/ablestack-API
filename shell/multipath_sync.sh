#!/usr/bin/env bash
#########################################
# Copyright (c) 2025 ABLECLOUD Co. Ltd.
#
# Multipath sync API wrapper.
# 각 호스트 작업은 ablestack-api가 대상 노드에서 로컬로 수행한다.
#########################################
set -euo pipefail

ABLESTACK_CONFIG_PATH="${ABLESTACK_CONFIG_PATH:-/etc/ablestack}"
CLUSTER_JSON="${ABLESTACK_CLUSTER_JSON:-${ABLESTACK_CONFIG_PATH}/properties/cluster.json}"
ABLESTACK_API_SCHEME="${ABLESTACK_API_SCHEME:-http}"
ABLESTACK_API_HOST="${ABLESTACK_API_HOST:-127.0.0.1}"
ABLESTACK_API_PORT="${ABLESTACK_API_PORT:-18090}"
ABLESTACK_API_BASE_URL="${ABLESTACK_API_BASE_URL:-${ABLESTACK_API_SCHEME}://${ABLESTACK_API_HOST}:${ABLESTACK_API_PORT}}"
ABLESTACK_MULTIPATH_SYNC_URL="${ABLESTACK_MULTIPATH_SYNC_URL:-${ABLESTACK_API_BASE_URL}/api/v1/cube/multipath/sync}"

usage() {
    cat <<EOF
Usage: $0 [sync|rescan] [target...]

Actions:
  sync      Rescan SCSI and synchronize multipath bindings/wwids through the API.
  rescan    Rescan SCSI through the API.

Optional targets are hostnames or IPs. Hostnames are sent as target_hostnames,
and IP addresses are sent as targets.

Environment:
  ABLESTACK_MULTIPATH_SYNC_URL  Override API endpoint.
  ABLESTACK_AUTHORIZATION       Full Authorization header value.
  ABLESTACK_ACCESS_TOKEN        Bearer token without the "Bearer " prefix.
  ABLESTACK_INTERNAL_TOKEN      Internal token override.
  CUBE_INTERNAL_TOKEN           Internal token override.
EOF
}

action="${1:-sync}"
case "$action" in
    sync|rescan)
        shift || true
        ;;
    -h|--help|help)
        usage
        exit 0
        ;;
    *)
        echo "unsupported action: ${action}" >&2
        usage >&2
        exit 2
        ;;
esac

require_command() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "required command not found: $1" >&2
        exit 1
    fi
}

json_string_array() {
    python3 - "$@" <<'PY'
import json
import sys

print(json.dumps(sys.argv[1:], ensure_ascii=False))
PY
}

is_ip_address() {
    python3 - "$1" <<'PY'
import ipaddress
import sys

try:
    ipaddress.ip_address(sys.argv[1])
except ValueError:
    raise SystemExit(1)
PY
}

cluster_internal_token() {
    if [ -n "${CUBE_INTERNAL_TOKEN:-}" ]; then
        printf '%s\n' "$CUBE_INTERNAL_TOKEN"
        return 0
    fi
    if [ -n "${ABLESTACK_INTERNAL_TOKEN:-}" ]; then
        printf '%s\n' "$ABLESTACK_INTERNAL_TOKEN"
        return 0
    fi
    if [ ! -f "$CLUSTER_JSON" ]; then
        return 1
    fi
    python3 - "$CLUSTER_JSON" <<'PY'
import json
import sys

try:
    with open(sys.argv[1], "r", encoding="utf-8") as f:
        data = json.load(f)
except Exception:
    raise SystemExit(1)

token = data.get("security", {}).get("internal_token", "")
if not isinstance(token, str) or not token.strip():
    raise SystemExit(1)
print(token.strip())
PY
}

authorization_header() {
    if [ -n "${ABLESTACK_AUTHORIZATION:-}" ]; then
        printf '%s\n' "$ABLESTACK_AUTHORIZATION"
        return 0
    fi
    if [ -n "${ABLESTACK_ACCESS_TOKEN:-}" ]; then
        printf 'Bearer %s\n' "$ABLESTACK_ACCESS_TOKEN"
        return 0
    fi
    if command -v ablestack-auth-token >/dev/null 2>&1; then
        ablestack-auth-token --plain 2>/dev/null || true
    fi
}

build_payload() {
    local targets_json target_hostnames_json
    targets_json="$(json_string_array "${target_ips[@]}")"
    target_hostnames_json="$(json_string_array "${target_hostnames[@]}")"
    python3 - "$action" "$targets_json" "$target_hostnames_json" <<'PY'
import json
import sys

payload = {"action": sys.argv[1]}
targets = json.loads(sys.argv[2])
target_hostnames = json.loads(sys.argv[3])
if targets:
    payload["targets"] = targets
if target_hostnames:
    payload["target_hostnames"] = target_hostnames
print(json.dumps(payload, separators=(",", ":")))
PY
}

require_command curl
require_command python3

target_ips=()
target_hostnames=()
for target in "$@"; do
    if is_ip_address "$target"; then
        target_ips+=("$target")
    else
        target_hostnames+=("$target")
    fi
done

payload="$(build_payload)"
curl_args=(
    -sS
    -X POST
    "$ABLESTACK_MULTIPATH_SYNC_URL"
    -H "Content-Type: application/json"
    -d "$payload"
)

if token="$(cluster_internal_token 2>/dev/null)"; then
    curl_args+=(-H "X-Cube-Internal-Token: ${token}")
else
    auth="$(authorization_header)"
    if [ -n "$auth" ]; then
        curl_args+=(-H "Authorization: ${auth}")
    fi
fi

curl "${curl_args[@]}"
echo
