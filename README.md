# ABLESTACK API

ABLESTACK API는 Cube 노드와 SCVM에서 실행되는 Go 기반 관리 API 서버입니다. 클러스터 구성, 호스트/디스크/NIC 조회, CCVM/SCVM lifecycle, PCS 제어, Glue 상태 조회, 라이선스, 보안 패치, DB/VM 백업 같은 Cube 운영 기능과 SCVM 전용 Glue API를 HTTP API로 제공합니다.

## What This API Does

| 영역 | 주요 기능 |
| --- | --- |
| Cluster | `cluster.json` 조회/적용, 호스트 추가/삭제, systemProfile 관리, 노드 health check |
| Host Inventory | `/etc/hosts`, 디스크, NIC, GFS 디스크/리소스 상태 조회 |
| Cloud Center VM | CCVM 상태 조회, 시작/정지/삭제/초기화, CPU/메모리 수정, secondary 용량 추가 |
| Storage Center VM | SCVM 상태 조회, 시작/정지/삭제/setup/reset, CPU/메모리 수정 |
| PCS/Glue | PCS cluster/resource 제어, Glue cluster 상태 조회, 유지보수 모드 설정/해제 |
| Glue API | SCVM Cockpit용 `/api/v1/glue` namespace, Glue status/hosts/version/pool/image/service, GlueFS/NFS/RGW 조회/쓰기 API, NVMe-oF 로컬 podman API, SMB/iSCSI/Mirror 로컬 실행 API |
| Backup | CCVM snapshot, CCVM 파일 백업/복구, CCVM DB dump/스케줄 관리 |
| Operations | 라이선스 등록/조회/role 기반 cluster fan-out, 올인원 배포 job, ABLESTACK 버전 업데이트, 보안 패치 실행 |

## Repository Structure

```text
cmd/apiserver/                 API 서버 진입점, Gin 라우팅, Swagger 노출
configs/                       API 서버 기본 설정 파일
docs/                          Swagger 산출물과 상세 운영 문서
internal/handler/              HTTP 요청 처리 계층
internal/model/                요청/응답 DTO, 캐시 모델, 상태 모델
internal/service/controller/   주기 실행 컨트롤러와 background job 실행
internal/service/clusterconfig/cluster.json 및 /etc/hosts 적용 로직
internal/service/glueservice/  SCVM 로컬 Ceph/RBD/orch 명령 실행 로직
internal/infra/utils/          공통 HTTP/SSH/error 유틸리티
shell/                         보안 패치 shell script
```

## Quick Start

서버 listen port는 `ABLESTACK_API_PORT`를 사용하며 기본값은 `18090`입니다.

```bash
go run ./cmd/apiserver
```

기본 생존 확인:

```bash
curl -sS http://127.0.0.1:18090/api/v1/cube/cluster/health
```

Swagger UI:

```text
http://<ablecube-ip>:18090/swagger/index.html
```

## Runtime Requirements

이 API는 단순 웹 API가 아니라 Cube 노드의 시스템 명령을 호출합니다. 운영 노드에는 기능별로 다음 명령/서비스가 필요할 수 있습니다.

| 기능 | 필요 요소 |
| --- | --- |
| libvirt 기반 VM 상태/제어 | `libvirt`, `virsh`, `qemu-guest-agent`, Go libvirt 빌드 환경 |
| PCS 제어 | `pcs`, `crm_mon`, pacemaker/corosync |
| Glue/Ceph 상태 및 스토리지 API | `ceph`, `rbd`, `radosgw-admin`, `rbd mirror`, Ceph dashboard API |
| Glue iSCSI | `ceph orch`, Ceph dashboard API, `podman`, iSCSI gateway container `gwcli` |
| Glue SMB | SCVM 로컬 `Samba-Execute.sh`, `smbpasswd`, `pdbedit`, Samba service, CephFS mount command |
| CCVM Wall 모니터링 구성 | `/usr/share/ablestack/ablestack-wall/python`의 기존 Wall Python 도구와 해당 Python 모듈 |
| 디스크/NIC 조회 | `lsblk`, `lspci`, `nmcli`, `ip` |
| DB dump | `/usr/bin/mysqldump`; 사용자 지정 반복/일회 일정에는 `crontab`, `at` |
| SSH key scan/security patch | `ssh`, `ssh-keyscan`, `ssh-keygen`, `security_patch.sh` |

macOS 개발 환경에서 `go test ./...`를 실행하려면 libvirt CGO 의존성 때문에 `pkg-config`와 libvirt 개발 패키지가 필요합니다.

RPM hard dependency는 host/SCVM/CCVM 공통 실행에 필요한 `systemd`, `bash`, `python3` 중심으로 유지합니다. Linux 계정 인증과 CCVM 자동 스냅샷·DB 백업은 Go에서 직접 처리하며 Python을 호출하지 않습니다. Python은 보안 증적, Samba 및 일부 shell helper 실행 때문에 유지합니다. Samba, Ceph, podman, pcs, virsh 같은 role-specific runtime command는 해당 role 이미지 또는 role 패키지에서 제공하고, API RPM 설치 자체를 막는 dependency로 두지 않습니다.

CCVM 자동 스냅샷과 DB dump는 API 프로세스 내부 스케줄러가 매일 01:00에 실행합니다. 기존 `create_ccvm_snap.py`, `backup_mysql.py` root cron은 API 시작 시 제거하며, DB dump API로 사용자가 등록한 `RegularBackup`/`DeleteOldBackup` 일정은 보존합니다.

## Configuration

| 변수 | 용도 |
| --- | --- |
| `ABLESTACK_CLUSTER_JSON` | `cluster.json` 절대 경로 override |
| `ABLESTACK_CONFIG_PATH` | ABLESTACK 설정 루트. 기본값은 `/etc/ablestack` |
| `ABLESTACK_STATE_PATH` | VM 설정 생성물 루트. 기본값은 `/etc/ablestack/vmconfig` |
| `ABLESTACK_API_SCHEME` | 노드 간 API 호출 scheme. 기본값 `http` |
| `ABLESTACK_API_PORT` | 서버 listen 및 노드 간 API 호출 대상 port. 기본값 `18090` |
| `ABLESTACK_SECURITY_PATCH_SCRIPT` | 보안 패치 스크립트 경로 override |
| `ABLESTACK_NODE_ROLE` | 노드 역할 override. Glue API route는 `scvm`일 때만 등록 |
| `ABLESTACK_NODE_ROLE_FILE` | 노드 역할 파일 override. 기본값 `/etc/ablestack/node-role` |
| `ABLESTACK_GLUE_DASHBOARD_URL` | iSCSI target/discovery용 Ceph dashboard URL override. 비어 있으면 `ceph mgr services -f json`에서 조회 |
| `ABLESTACK_GLUE_DASHBOARD_USER` | Ceph dashboard API 사용자. 기본값 `admin` |
| `ABLESTACK_GLUE_DASHBOARD_PASSWORD` | Ceph dashboard API password. iSCSI target/discovery 호출에 필요 |
| `ABLESTACK_GLUE_SMB_SCRIPT` | SCVM 로컬 SMB 실행 스크립트 경로. 기본값은 RPM 설치 경로 `/etc/ablestack/shell/Samba-Execute.sh` |

## RPM Build

```bash
./scripts/build-rpm.sh
```

RPM 버전은 루트의 `VERSION` 파일을 기준으로 결정합니다. `scripts/build-rpm.sh`는 `CHANGELOG.md`에 같은 버전의 릴리즈 섹션이 있는지도 확인합니다. 임시 버전은 `VERSION=0.1.5 RELEASE=1 ./scripts/build-rpm.sh`, API 포트는 `API_PORT=28090 ./scripts/build-rpm.sh`처럼 지정할 수 있습니다. 직접 `rpmbuild`를 실행할 때는 `--define "api_port 28090"`을 사용합니다. RPM에는 `README.md`, `CHANGELOG.md`, `VERSION`이 문서 파일로 포함됩니다.

빌드 결과는 `dist/rpm/rpmbuild/RPMS`와 `dist/rpm/rpmbuild/SRPMS` 아래에 생성됩니다. 신규 설치는 지정한 포트를 `/etc/ablestack/ablestack-api.env`에 반영하고, 업그레이드는 기존 env의 유효한 포트를 보존합니다. 포트 값이 없던 구버전 업그레이드는 기본값 `18090`으로 이전합니다. RPM은 새 포트를 runtime/permanent 방화벽에 먼저 추가한 뒤 서비스를 재시작하며, 새 포트 listen 확인이 끝난 경우에만 기존 API 포트 방화벽 규칙을 제거합니다. 실패하면 기존 포트 설정으로 복구하고 새 방화벽 규칙을 제거합니다.

설치 후 포트를 변경할 때는 다음 관리 명령을 사용합니다. 이 명령이 환경 파일 갱신, 새 방화벽 포트 추가, 서비스 재시작과 listen 확인, 기존 방화벽 포트 제거를 순서대로 처리합니다.

```bash
sudo /usr/libexec/ablestack-api/configure-api-port.sh 28090
```

환경 파일만 직접 수정하는 경우에는 `systemctl restart ablestack-api` 외에도 새 포트의 runtime/permanent 방화벽 규칙을 추가하고 기존 포트 규칙을 별도로 제거해야 합니다.

RPM 설치 경로:

| 항목 | 경로 |
| --- | --- |
| API binary | `/usr/bin/ablestack-api` |
| Cockpit auth helper | `/usr/bin/ablestack-auth-token` |
| systemd unit | `/usr/lib/systemd/system/ablestack-api.service` |
| service env | `/etc/ablestack/ablestack-api.env` |
| port configuration helper | `/usr/libexec/ablestack-api/configure-api-port.sh` |
| cluster properties | `/etc/ablestack/properties` |
| XML templates | `/etc/ablestack/xml-template` |
| shell resources | `/etc/ablestack/shell` |
| generated VM config | `/etc/ablestack/vmconfig` |

`/etc/ablestack` 아래 설정 파일은 RPM spec에서 `%config(noreplace)`로 관리합니다. 업데이트 시 기존 파일은 덮어쓰지 않고, `auth.json`과 `properties/cluster.json`은 새 RPM의 `.rpmnew`가 생기면 누락된 JSON key만 병합합니다. 기존 값과 운영 데이터는 유지됩니다.

라이선스 조회/등록 API는 Bearer 토큰 없이 호출할 수 있습니다. 활성 라이선스가 등록되면 API access token 서명값은 라이선스의 `license_key`에서 파생됩니다. 라이선스가 없거나 만료된 경우 라이선스 조회/등록과 Swagger를 제외한 API는 사용할 수 없습니다.

Cockpit UI에서는 로그인된 Linux 세션을 기준으로 `/usr/bin/ablestack-auth-token`을 실행해 Bearer 토큰을 자동 발급할 수 있습니다. 기본 JSON 출력의 `authorization` 값을 이후 API 요청의 `Authorization` 헤더에 사용하면 됩니다.

`cluster apply`는 insert 시 `security.internal_token`을 생성하고 `apply-local` payload에 포함해 각 호스트 `cluster.json`에 같은 값을 저장합니다. API access token 서명값은 같은 라이선스의 `license_key`에서 파생되므로 각 호스트에 동일한 라이선스가 등록되면 별도 인증 서명값 동기화 없이 같은 token을 검증합니다. 내부 호출용 token 교체는 `POST /api/v1/auth/internal-token/rotate`로 수행합니다.

`/cube/license/apply`는 `roles`가 비어 있으면 기존처럼 `hosts[].ablecube`에만 적용합니다. SCVM/CCVM은 VM 생성 후 API가 준비된 시점에 `roles:["scvm"]`, `roles:["ccvm"]` 또는 `roles:["all"]`로 후처리 등록합니다.

`/api/v1/health`와 `/health`는 라이선스 등록 전 bootstrap에서도 사용할 수 있는 public health check입니다. 올인원 배포에서는 `scvm_prepare`/`ccvm_prepare`가 VM 생성과 API health 확인을 담당하고, `scvm_bootstrap`/`ccvm_bootstrap`이 라이선스 자동 등록과 status 확인을 담당합니다.

## API Base URL

```text
http://<ablecube-ip>:18090/api/v1
```

예시:

```bash
curl -sS http://10.10.12.1:18090/api/v1/cube/disk?action=list
```

## Main API Groups

| Group | Endpoint 예시 | 설명 |
| --- | --- | --- |
| Cluster | `GET /cube/cluster/health`, `POST /cube/cluster/apply` | 클러스터 구성/상태 관리 |
| System Profile | `GET/POST /cube/system/config` | `cluster.json`의 `systemProfile` 조회/수정 |
| Host | `GET /cube/hosts` | `/etc/hosts`를 역할/네트워크 기준으로 조회 |
| Disk/NIC | `GET /cube/disk`, `GET /cube/nics` | 노드 디스크/NIC 인벤토리 |
| CCVM | `GET /cube/ccvm/status`, `POST /cube/ccvm/lifecycle` | Cloud Center VM 관리 |
| SCVM | `GET /cube/scvm/status`, `POST /cube/scvm/lifecycle` | Storage Center VM 관리 |
| PCS | `POST /cube/pcs/control` | Cloud Center PCS 리소스 제어 |
| Glue/GFS | `GET /cube/gluecluster/status`, `GET /cube/gfs/disk/status` | 스토리지 클러스터 상태 |
| Glue API | `GET /glue/status`, `GET/POST /glue/gluefs`, `GET/POST /glue/nfs`, `GET/POST /glue/rgw`, `GET/POST /glue/smb`, `GET/POST /glue/iscsi`, `GET/POST /glue/mirror` | SCVM 전용 Glue API namespace |
| Backup | `POST /cube/ccvm/snap`, `POST /cube/ccvm/backup`, `POST /cube/db/dump` | snapshot, 파일 백업, DB 백업 |
| Operations | `POST /cube/license`, `POST /cube/license/apply`, `POST /cube/deploy/run`, `POST /cube/version/update`, `POST /cube/security/patch`, `GET/POST /cube/security/evidence` | 운영/유지보수 및 보안 증적 작업 |

## Documentation

- [API 실행 및 상세 설명](docs/api-execution-and-description.md)
- [올인원 배포 실행 가이드](docs/deploy-run-guide.md)
- [SCVM SMB API 테스트 체크리스트](docs/scvm-smb-test-checklist.md)
- Swagger: `/swagger/index.html` (`Cube-*`, `Glue-*` 기능 tag 기준. host/CCVM은 Cube 중심, SCVM은 Glue와 라이선스 중심)

## Cluster Fan-out Model

일부 API는 요청을 받은 노드가 오케스트레이터 역할을 하고, `cluster.json`의 host 정보를 기준으로 다른 ablecube/scvm/ccvm 대상에 내부 API를 호출합니다.

대표 예:

```text
POST /api/v1/cube/cluster/apply
  -> 각 대상 노드의 /api/v1/cube/cluster/apply-local 호출
```

현재 구조를 유지하려면 모든 호스트 사이에 `ABLESTACK_API_PORT`로 설정한 TCP 포트 통신이 가능해야 합니다.
