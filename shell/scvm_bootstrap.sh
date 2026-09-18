#!/usr/bin/env bash
# Copyright (c) 2021 ABLECLOUD Co. Ltd.
# Bootstrap an ABLESTACK Ceph cluster without product-managed SSH/SCP calls.

set -Eeuo pipefail

readonly conffile="/root/ceph.conf"
readonly imagename="localhost:15000/glue/daemon:Diplo"
readonly action="${1:-bootstrap}"

scvm_records() {
  awk '
    /^[[:space:]]*#/ || NF < 2 { next }
    {
      for (i = 2; i <= NF; i++) {
        if ($i ~ /^scvm[0-9]+$/) {
          print $i, $1
          break
        }
      }
    }
  ' /etc/hosts | sort -V -k1,1 -u
}

master_mon_ip() {
  awk '
    /^[[:space:]]*#/ || NF < 2 { next }
    {
      for (i = 2; i <= NF; i++) {
        if ($i == "scvm") {
          print $1
          exit
        }
      }
    }
  ' /etc/hosts
}

cluster_network() {
  local ip
  ip=$(awk '
    /^[[:space:]]*#/ || NF < 2 { next }
    {
      for (i = 2; i <= NF; i++) {
        if ($i == "cn-scvm" || $i == "scvm-cn" || $i ~ /^scvm[0-9]+-cn$/) {
          print $1
          exit
        }
      }
    }
  ' /etc/hosts)
  [[ -n "$ip" ]] || return 1
  printf '%s.0/24\n' "${ip%.*}"
}

install_authorized_key() {
  local public_key="$1"
  [[ "$public_key" == ssh-* ]] || {
    echo "invalid Cephadm public key" >&2
    return 1
  }
  install -d -m 0700 /root/.ssh
  touch /root/.ssh/authorized_keys
  chmod 0600 /root/.ssh/authorized_keys
  grep -qxF -- "$public_key" /root/.ssh/authorized_keys || printf '%s\n' "$public_key" >> /root/.ssh/authorized_keys
}

prepare_local_scvm() {
  if [[ -x /usr/local/bin/ipcorrector ]]; then
    printf '%s\n' '* * * * * root /usr/local/bin/ipcorrector' > /etc/cron.d/ablestack-ipcorrector
    chmod 0644 /etc/cron.d/ablestack-ipcorrector
  fi

  local exporter_dir="/usr/share/ablestack/ablestack-wall/process-exporter"
  if [[ -f "$exporter_dir/scvm_process.yml" ]]; then
    install -m 0644 "$exporter_dir/scvm_process.yml" "$exporter_dir/process.yml"
  fi

  systemctl enable --now node-exporter
  systemctl enable --now process-exporter
  systemctl enable --now glue-api.service
  echo "SCVM local preparation completed on $(hostname)"
}

write_bootstrap_config() {
  local records mon_hosts=""
  records=$(scvm_records)
  [[ -n "$records" ]] || {
    echo "no SCVM public-network records found in /etc/hosts" >&2
    return 1
  }

  while read -r _ ip; do
    [[ -n "$ip" ]] || continue
    mon_hosts+="[v2:${ip}:3300/0,v1:${ip}:6789/0], "
  done <<< "$records"
  mon_hosts="${mon_hosts%, }"

  cat > "$conffile" <<EOF
[global]
\tmon_host = ${mon_hosts}
\tcontainer_image = ${imagename}
\tmgr/cephadm/container_image_base = ${imagename}
EOF
}

configure_ceph() {
  local image="$1"
  ceph config set global container_image "$image"
  ceph config set global mgr/cephadm/container_image_base "$image"
  ceph config set mgr mgr/cephadm/manage_etc_ceph_ceph_conf true
  ceph config set client rbd_cache_policy writeback
  ceph config set client rbd_cache_size 4294967296
  ceph config set client rbd_cache_max_dirty 3221225472
  ceph config set client rbd_cache_target_dirty 2147483648
  ceph config set client rbd_cache_max_dirty_age 5.0
  ceph config set mgr mgr/cephadm/container_image_alertmanager localhost:15000/prometheus/alertmanager:Diplo
  ceph config set mgr mgr/cephadm/container_image_grafana localhost:15000/glue/glue-grafana:Diplo
  ceph config set mgr mgr/cephadm/container_image_node_exporter localhost:15000/prometheus/node-exporter:Diplo
  ceph config set mgr mgr/cephadm/container_image_prometheus localhost:15000/prometheus/prometheus:Diplo
  ceph config set mgr mgr/cephadm/container_image_loki localhost:15000/grafana/loki:Diplo
  ceph config set mgr mgr/cephadm/container_image_promtail localhost:15000/grafana/promtail:Diplo
  ceph config set mgr mgr/cephadm/container_image_nvmeof-cli localhost:15000/glue/nvmeof-cli:Diplo
  ceph config set mgr mgr/cephadm/container_image_nvmeof localhost:15000/glue/nvmeof:Diplo
  ceph config set mgr mgr/cephadm/container_image_keepalived localhost:15000/glue/keepalived:2.2.4
  ceph config set mgr mgr/cephadm/container_image_haproxy localhost:15000/glue/haproxy:2.3
  ceph config set mon mon_warn_on_insecure_global_id_reclaim_allowed false
}

ceph_host_exists() {
  local hostname="$1"
  ceph orch host ls 2>/dev/null | awk 'NR > 1 {print $1}' | grep -qxF -- "$hostname"
}

ensure_ceph_hosts() {
  local records hostname ip count=0
  records=$(scvm_records)
  while read -r hostname ip; do
    [[ -n "$hostname" && -n "$ip" ]] || continue
    if ! ceph_host_exists "$hostname"; then
      ceph orch host add "$hostname" "$ip"
    fi
    count=$((count + 1))
  done <<< "$records"

  [[ "$count" -gt 0 ]] || {
    echo "no SCVM hosts available for Ceph orchestration" >&2
    return 1
  }
  if [[ "$count" -gt 3 ]]; then
    count=3
  fi
  ceph orch apply mon --placement="$count"
}

configure_dashboard_bindings() {
  local mgr scvm ip
  while read -r mgr; do
    [[ -n "$mgr" ]] || continue
    scvm="${mgr%%.*}"
    ip=$(awk -v name="${scvm}-mngt" '
      $0 !~ /^[[:space:]]*#/ {
        for (i = 2; i <= NF; i++) {
          if ($i == name) { print $1; exit }
        }
      }
    ' /etc/hosts)
    [[ -n "$ip" ]] && ceph config set mgr "mgr/dashboard/${mgr}/server_addr" "$ip"
  done < <(ceph orch ps | awk '/mgr/ {sub(/^mgr\./, "", $1); print $1}')

  ceph mgr module disable dashboard
  ceph mgr module enable dashboard
}

verify_cluster() {
  local hostname _
  while read -r hostname _; do
    [[ -n "$hostname" ]] || continue
    ceph_host_exists "$hostname" || {
      echo "Ceph host verification failed: ${hostname}" >&2
      return 1
    }
  done < <(scvm_records)
  ceph orch host ls
  ceph orch ps
  ceph health detail
}

remove_ceph_host() {
  local hostname="${1:-}" force="${2:-false}"
	local attempts="${ABLESTACK_CEPH_DRAIN_ATTEMPTS:-90}"
	local interval="${ABLESTACK_CEPH_DRAIN_INTERVAL:-20}"
  [[ -n "$hostname" ]] || {
    echo "Ceph host name is required" >&2
    return 1
  }
  if ! ceph_host_exists "$hostname"; then
    echo "Ceph host ${hostname} is already absent"
    return 0
  fi
  ceph orch host drain "$hostname" --zap-osd-devices
  if [[ "$force" == "true" ]]; then
    ceph orch host rm "$hostname" --force --offline
  else
	local attempt daemon_count
	for ((attempt = 1; attempt <= attempts; attempt++)); do
	  daemon_count=$(ceph orch ps --hostname "$hostname" --format json 2>/dev/null | grep -c '"daemon_name"' || true)
	  if [[ "$daemon_count" -eq 0 ]]; then
	    break
	  fi
	  echo "Waiting for Ceph host drain: ${hostname} (${attempt}/${attempts}, daemons=${daemon_count})"
	  sleep "$interval"
	done
	daemon_count=$(ceph orch ps --hostname "$hostname" --format json 2>/dev/null | grep -c '"daemon_name"' || true)
	if [[ "$daemon_count" -ne 0 ]]; then
	  echo "Ceph host drain did not complete: ${hostname} still has ${daemon_count} daemon(s)" >&2
	  ceph orch ps --hostname "$hostname"
	  return 1
	fi
    ceph orch host rm "$hostname"
  fi
  ceph_host_exists "$hostname" && {
    echo "Ceph host ${hostname} still exists after removal" >&2
    return 1
  }
  ceph orch host ls
}

bootstrap_master() {
  local image mon_ip network
  prepare_local_scvm
  write_bootstrap_config
  podman start registry >/dev/null 2>&1 || podman inspect registry >/dev/null
  image=$(/bin/podman inspect --format '{{.ID}},{{.RepoDigests}}' "$imagename" | cut -d ',' -f 2 | sed 's/^\[//;s/\]$//')
  [[ -n "$image" ]] || {
    echo "failed to resolve Ceph container image digest" >&2
    return 1
  }

  if ! ceph status >/dev/null 2>&1; then
    mon_ip=$(master_mon_ip)
    network=$(cluster_network)
    [[ -n "$mon_ip" ]] || {
      echo "master SCVM public-network IP not found in /etc/hosts" >&2
      return 1
    }
    cephadm --image "$image" bootstrap \
      --initial-dashboard-user admin \
      --initial-dashboard-password password \
      --no-minimize-config \
      --skip-pull \
      --allow-overwrite \
      --mon-ip "$mon_ip" \
      --cluster-network "$network" \
      --config "$conffile"
  fi

  configure_ceph "$image"
}

export_cephadm_public_key() {
  local public_key
  public_key=$(ceph cephadm get-pub-key)
  [[ "$public_key" == ssh-* ]] || {
    echo "Cephadm public key is unavailable" >&2
    return 1
  }
  printf 'CEPHADM_PUBLIC_KEY_BASE64=%s\n' "$(printf '%s' "$public_key" | base64 -w 0)"
}

install_cephadm_public_key() {
  local encoded_key="${1:-}" public_key
  [[ -n "$encoded_key" ]] || {
    echo "Cephadm public key payload is required" >&2
    return 1
  }
  public_key=$(printf '%s' "$encoded_key" | base64 -d)
  install_authorized_key "$public_key"
  echo "Cephadm public key installed on $(hostname)"
}

finalize_cluster() {
  ensure_ceph_hosts
  configure_dashboard_bindings
  verify_cluster
}

case "$action" in
  prepare)
    prepare_local_scvm
    ;;
  bootstrap)
    bootstrap_master
    ;;
  export-key)
    export_cephadm_public_key
    ;;
  install-key)
    install_cephadm_public_key "${2:-}"
    ;;
  finalize)
    finalize_cluster
    ;;
  verify)
    verify_cluster
    ;;
  remove-host)
    remove_ceph_host "${2:-}" "${3:-false}"
    ;;
  *)
    echo "unsupported SCVM bootstrap action: $action" >&2
    exit 2
    ;;
esac
