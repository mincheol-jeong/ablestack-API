%global service_name ablestack-api
%global config_root %{_sysconfdir}/ablestack
%global state_root %{config_root}/vmconfig
%global log_root %{_localstatedir}/log/ablestack
%{!?api_port:%global api_port 18090}
%global debug_package %{nil}
%{!?_unitdir:%global _unitdir %{_prefix}/lib/systemd/system}

%bcond_without tests

Name:           ablestack-api
Version:        %{?rpm_version}%{!?rpm_version:0.1.1}
Release:        %{?rpm_release}%{!?rpm_release:1}%{?dist}
Summary:        ABLESTACK API server
License:        Apache-2.0
URL:            https://www.ablecloud.io
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  golang >= 1.26.2
BuildRequires:  libvirt-devel
BuildRequires:  pkgconfig
BuildRequires:  systemd-rpm-macros
Requires:       systemd
Requires:       bash
Requires:       python3
Requires:       openssh-clients
Recommends:     firewalld
# yescrypt and legacy crypt verification are statically linked Go modules and
# do not add runtime RPM dependencies. python3 remains for packaged evidence,
# Samba, and shell helper workflows, not Linux account authentication.
# Role-specific runtime commands and assets such as ceph, rbd, podman, samba,
# realmd, pcs, virsh, and the CCVM Wall Python runtime are intentionally not
# hard dependencies. The same API RPM is installed on host, SCVM, and CCVM,
# and those components are required only when the matching role API runs.
Requires(post): systemd
Requires(post): python3
Requires(preun): systemd
Requires(postun): systemd

%description
ABLESTACK API server and managed configuration files.

%prep
%autosetup -n %{name}-%{version}

%build
case "%{api_port}" in
    ''|*[!0-9]*) echo "invalid api_port: %{api_port}" >&2; exit 2 ;;
esac
if [ "%{api_port}" -lt 1 ] || [ "%{api_port}" -gt 65535 ]; then
    echo "api_port must be between 1 and 65535: %{api_port}" >&2
    exit 2
fi
export GO111MODULE=on
export CGO_ENABLED=1
if [ -d vendor ]; then
    export GOFLAGS="${GOFLAGS:-} -mod=vendor"
fi
go build -buildvcs=false -trimpath -ldflags "-s -w" -o %{name} ./cmd/apiserver
go build -buildvcs=false -trimpath -ldflags "-s -w" -o ablestack-auth-token ./cmd/authtoken

%check
%if %{with tests}
go vet ./...
go test ./...
%endif

%install
install -Dpm 0755 %{name} %{buildroot}%{_bindir}/%{name}
install -Dpm 0755 ablestack-auth-token %{buildroot}%{_bindir}/ablestack-auth-token
install -Dpm 0644 packaging/systemd/%{service_name}.service %{buildroot}%{_unitdir}/%{service_name}.service
install -Dpm 0755 packaging/scripts/merge-json-defaults.py %{buildroot}%{_libexecdir}/%{name}/merge-json-defaults.py
install -Dpm 0755 packaging/scripts/configure-api-port.sh %{buildroot}%{_libexecdir}/%{name}/configure-api-port.sh
install -d %{buildroot}%{_libexecdir}/%{name}/python/security_evidence
install -pm 0644 security-evidence/python/security_evidence/__init__.py %{buildroot}%{_libexecdir}/%{name}/python/security_evidence/
install -pm 0755 security-evidence/python/security_evidence/security_evidence.py %{buildroot}%{_libexecdir}/%{name}/python/security_evidence/
install -pm 0755 security-evidence/python/security_evidence/security_evidence_package.py %{buildroot}%{_libexecdir}/%{name}/python/security_evidence/
install -d %{buildroot}%{_libexecdir}/%{name}/tools/security_evidence
install -pm 0644 security-evidence/tools/security_evidence/checks.json %{buildroot}%{_libexecdir}/%{name}/tools/security_evidence/
install -Dpm 0755 shell/security_patch.sh %{buildroot}%{_libexecdir}/%{name}/shell/security_patch.sh

	install -d %{buildroot}%{config_root}
	install -Dpm 0600 configs/auth.json %{buildroot}%{config_root}/auth.json
	install -Dpm 0644 packaging/config/ablestack-api.env %{buildroot}%{config_root}/ablestack-api.env
	sed -i 's/^ABLESTACK_API_PORT=.*/ABLESTACK_API_PORT=%{api_port}/' %{buildroot}%{config_root}/ablestack-api.env

install -d %{buildroot}%{config_root}/properties
install -pm 0644 properties/* %{buildroot}%{config_root}/properties/

install -d %{buildroot}%{config_root}/xml-template
install -pm 0644 xml-template/* %{buildroot}%{config_root}/xml-template/

install -d %{buildroot}%{config_root}/shell
install -pm 0755 shell/* %{buildroot}%{config_root}/shell/

install -d %{buildroot}%{state_root}/ccvm
install -d %{buildroot}%{state_root}/scvm
install -d %{buildroot}%{log_root}/archive

%post
merge_json_if_rpmnew() {
    target="$1"
    defaults="${target}.rpmnew"
    if [ -f "$target" ] && [ -f "$defaults" ]; then
        %{_libexecdir}/%{name}/merge-json-defaults.py "$target" "$defaults" >/dev/null 2>&1 || :
        rm -f "$defaults" || :
	    fi
	}
	merge_json_if_rpmnew "%{config_root}/auth.json"
	merge_json_if_rpmnew "%{config_root}/properties/cluster.json"

remove_auth_legacy_secret() {
    target="%{config_root}/auth.json"
    [ -f "$target" ] || return 0
    python3 - "$target" <<'PY' >/dev/null 2>&1 || :
import json
import os
import sys

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as f:
    data = json.load(f)
if not isinstance(data, dict) or "access_token_secret" not in data:
    raise SystemExit(0)
data.pop("access_token_secret", None)
tmp = path + ".tmp"
with open(tmp, "w", encoding="utf-8") as f:
    json.dump(data, f, ensure_ascii=False, indent=2)
    f.write("\n")
os.chmod(tmp, 0o600)
os.replace(tmp, path)
PY
}
remove_auth_legacy_secret

desired_api_port="%{api_port}"
if [ "$1" -gt 1 ] && [ -f "%{config_root}/ablestack-api.env" ]; then
    configured_api_port="$(sed -n 's/^[[:space:]]*ABLESTACK_API_PORT[[:space:]]*=[[:space:]]*//p' "%{config_root}/ablestack-api.env" | tail -n 1)"
    configured_api_port="${configured_api_port%\"}"
    configured_api_port="${configured_api_port#\"}"
    configured_api_port="${configured_api_port%\'}"
    configured_api_port="${configured_api_port#\'}"
    case "$configured_api_port" in
        ''|*[!0-9]*) ;;
        *)
            if [ "$configured_api_port" -ge 1 ] && [ "$configured_api_port" -le 65535 ]; then
                desired_api_port="$configured_api_port"
            fi
            ;;
    esac
fi

%{_libexecdir}/%{name}/configure-api-port.sh \
    "%{config_root}/ablestack-api.env" \
    "%{service_name}.service" \
    "$desired_api_port" \
    "%{config_root}/.%{name}-port" \
    "$1"

%preun
if [ "$1" -eq 0 ] && command -v systemctl >/dev/null 2>&1; then
    systemctl disable --now %{service_name}.service >/dev/null 2>&1 || :
    if command -v firewall-cmd >/dev/null 2>&1; then
        api_port=""
        if [ -f "%{config_root}/.%{name}-port" ]; then
            IFS= read -r api_port < "%{config_root}/.%{name}-port" || :
        fi
        case "$api_port" in
            ''|*[!0-9]*) ;;
            *)
                firewall-cmd --permanent --remove-port="${api_port}/tcp" >/dev/null 2>&1 || :
                firewall-cmd --remove-port="${api_port}/tcp" >/dev/null 2>&1 || :
                ;;
        esac
    fi
    rm -f "%{config_root}/.%{name}-port" || :
fi

%postun
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || :
fi

%files
%license LICENSE
%doc README.md CHANGELOG.md VERSION
%{_bindir}/%{name}
%{_bindir}/ablestack-auth-token
%{_unitdir}/%{service_name}.service
	%{_libexecdir}/%{name}/merge-json-defaults.py
	%{_libexecdir}/%{name}/configure-api-port.sh
	%dir %{_libexecdir}/%{name}/python
	%dir %{_libexecdir}/%{name}/python/security_evidence
	%{_libexecdir}/%{name}/python/security_evidence/__init__.py
	%{_libexecdir}/%{name}/python/security_evidence/security_evidence.py
	%{_libexecdir}/%{name}/python/security_evidence/security_evidence_package.py
	%dir %{_libexecdir}/%{name}/tools
	%dir %{_libexecdir}/%{name}/tools/security_evidence
	%{_libexecdir}/%{name}/tools/security_evidence/checks.json
	%dir %{_libexecdir}/%{name}/shell
	%{_libexecdir}/%{name}/shell/security_patch.sh
	%dir %{config_root}
	%config(noreplace) %attr(0600,root,root) %{config_root}/auth.json
	%config(noreplace) %{config_root}/ablestack-api.env
%dir %{config_root}/properties
%config(noreplace) %{config_root}/properties/*
%dir %{config_root}/xml-template
%config(noreplace) %{config_root}/xml-template/*
%dir %{config_root}/shell
%config(noreplace) %attr(0755,root,root) %{config_root}/shell/*
%dir %{state_root}
%dir %{state_root}/ccvm
%dir %{state_root}/scvm
%dir %{log_root}
%dir %{log_root}/archive

%changelog
* Tue May 26 2026 ABLECLOUD <support@ablecloud.io> - 0.1.1-1
- Add Cockpit session token helper CLI.
- Add deployment status API for UI stage handling.
- Add dynamic PCS cluster target handling up to 16 hosts.
- Separate PCS validation rules by ABLESTACK deployment type.
- Improve PCS-based CloudCenter status, snapshot, and Glue config flows.
- Update API documentation and Swagger output for deployment status.

* Tue May 26 2026 ABLECLOUD <support@ablecloud.io> - 0.1.0-1
- Add RPM packaging for ABLESTACK API.
- Use VERSION as the RPM build version source.
- Include CHANGELOG and VERSION in package documentation.
