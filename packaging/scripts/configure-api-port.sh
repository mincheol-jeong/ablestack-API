#!/usr/bin/env bash
set -euo pipefail

if [[ $# -eq 1 ]]; then
    ENV_FILE="/etc/ablestack/ablestack-api.env"
    SERVICE_NAME="ablestack-api.service"
    NEW_PORT="$1"
    STATE_FILE="/etc/ablestack/.ablestack-api-port"
    INSTALL_COUNT="1"
else
    ENV_FILE="${1:-/etc/ablestack/ablestack-api.env}"
    SERVICE_NAME="${2:-ablestack-api.service}"
    NEW_PORT="${3:-18090}"
    STATE_FILE="${4:-/etc/ablestack/.ablestack-api-port}"
    INSTALL_COUNT="${5:-1}"
fi
HEALTH_ATTEMPTS="${ABLESTACK_API_PORT_HEALTH_ATTEMPTS:-30}"

is_valid_port() {
    [[ "$1" =~ ^[0-9]+$ ]] && (( 10#$1 >= 1 && 10#$1 <= 65535 ))
}

read_env_port() {
    local value=""
    if [[ -f "$ENV_FILE" ]]; then
        value="$(sed -n 's/^[[:space:]]*ABLESTACK_API_PORT[[:space:]]*=[[:space:]]*//p' "$ENV_FILE" | tail -n 1)"
        value="${value%\"}"
        value="${value#\"}"
        value="${value%\'}"
        value="${value#\'}"
    fi
    printf '%s' "$value"
}

read_old_port() {
    local value=""
    value="$(read_env_port)"
    if ! is_valid_port "$value" && [[ -f "$STATE_FILE" ]]; then
        IFS= read -r value < "$STATE_FILE" || true
    fi
    if is_valid_port "$value"; then
        printf '%s' "$((10#$value))"
    fi
}

write_env_port() {
    local port="$1"
    local dir tmp
    dir="$(dirname "$ENV_FILE")"
    mkdir -p "$dir"
    tmp="$(mktemp "${dir}/.ablestack-api.env.XXXXXX")"
    if [[ -f "$ENV_FILE" ]]; then
        awk -v port="$port" '
            BEGIN { updated = 0 }
            /^[[:space:]]*ABLESTACK_API_PORT[[:space:]]*=/ {
                if (!updated) print "ABLESTACK_API_PORT=" port
                updated = 1
                next
            }
            { print }
            END { if (!updated) print "ABLESTACK_API_PORT=" port }
        ' "$ENV_FILE" > "$tmp"
    else
        printf 'ABLESTACK_API_PORT=%s\n' "$port" > "$tmp"
    fi
    chmod 0644 "$tmp"
    mv -f "$tmp" "$ENV_FILE"
}

add_firewall_port() {
    local port="$1"
    command -v firewall-cmd >/dev/null 2>&1 || return 0
    systemctl enable --now firewalld.service >/dev/null 2>&1 || true
    firewall-cmd --permanent --add-port="${port}/tcp" >/dev/null
    firewall-cmd --add-port="${port}/tcp" >/dev/null
}

remove_firewall_port() {
    local port="$1"
    command -v firewall-cmd >/dev/null 2>&1 || return 0
    firewall-cmd --permanent --remove-port="${port}/tcp" >/dev/null 2>&1 || true
    firewall-cmd --remove-port="${port}/tcp" >/dev/null 2>&1 || true
}

wait_for_api() {
    local port="$1"
    local attempt
    for attempt in $(seq 1 "$HEALTH_ATTEMPTS"); do
        if systemctl is-active --quiet "$SERVICE_NAME" && (exec 9<>"/dev/tcp/127.0.0.1/${port}") 2>/dev/null; then
            exec 9>&-
            exec 9<&-
            return 0
        fi
        sleep 1
    done
    return 1
}

if ! is_valid_port "$NEW_PORT"; then
    echo "invalid API port: $NEW_PORT (expected 1-65535)" >&2
    exit 2
fi
if [[ ! "$HEALTH_ATTEMPTS" =~ ^[0-9]+$ ]] || (( HEALTH_ATTEMPTS < 1 )); then
    echo "ABLESTACK_API_PORT_HEALTH_ATTEMPTS must be a positive integer" >&2
    exit 2
fi
NEW_PORT="$((10#$NEW_PORT))"
OLD_PORT="$(read_old_port)"
if ! is_valid_port "$OLD_PORT" && [[ "$INSTALL_COUNT" =~ ^[0-9]+$ ]] && (( INSTALL_COUNT > 1 )); then
    # Releases before ABLESTACK_API_PORT was introduced listened on 8090
    # without persisting the managed firewall port.
    OLD_PORT="8090"
fi

add_firewall_port "$NEW_PORT"
write_env_port "$NEW_PORT"
systemctl daemon-reload
systemctl enable "$SERVICE_NAME" >/dev/null
if systemctl is-active --quiet "$SERVICE_NAME"; then
    systemctl restart "$SERVICE_NAME"
else
    systemctl start "$SERVICE_NAME"
fi

if ! wait_for_api "$NEW_PORT"; then
    echo "${SERVICE_NAME} did not listen on ${NEW_PORT}/tcp" >&2
    if is_valid_port "$OLD_PORT" && [[ "$OLD_PORT" != "$NEW_PORT" ]]; then
        write_env_port "$OLD_PORT"
        systemctl restart "$SERVICE_NAME" >/dev/null 2>&1 || true
    fi
    if [[ "$OLD_PORT" != "$NEW_PORT" ]]; then
        remove_firewall_port "$NEW_PORT"
    fi
    exit 1
fi

mkdir -p "$(dirname "$STATE_FILE")"
printf '%s\n' "$NEW_PORT" > "$STATE_FILE"
chmod 0644 "$STATE_FILE"

if is_valid_port "$OLD_PORT" && [[ "$OLD_PORT" != "$NEW_PORT" ]]; then
    remove_firewall_port "$OLD_PORT"
fi

echo "${SERVICE_NAME} is listening on ${NEW_PORT}/tcp"
