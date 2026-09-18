# ABLESTACK API 실행 및 상세 설명

작성일: 2026-05-22

이 문서는 현재 Go 코드의 실제 route, RPM 패키징 구성, `/etc/ablestack` 설정 경로 정책을 기준으로 정리했다. Swagger 예시는 `/swagger/index.html`에서 확인할 수 있고, 이 문서는 운영자가 설치, 설정, API 호출 흐름을 한 번에 파악하는 것을 목표로 한다.

## 기본 정보

| 항목 | 값 |
| --- | --- |
| 기본 URL | `http://<ablecube-ip>:18090/api/v1` |
| Swagger | `http://<ablecube-ip>:18090/swagger/index.html` |
| API 서버 진입점 | `cmd/apiserver/main.go` |
| 주요 handler | `internal/handler` |
| 주요 model | `internal/model` |
| 클러스터 설정 파일 | `/etc/ablestack/properties/cluster.json` |
| 서비스 환경 파일 | `/etc/ablestack/ablestack-api.env` |
| VM 설정 생성물 | `/etc/ablestack/vmconfig` |
| XML 템플릿 | `/etc/ablestack/xml-template` |

`ABLESTACK_API_PORT`는 서버 listen port와 노드 간 원격 API URL에 함께 사용되며 기본값은 `18090`이다. RPM 포트를 변경하면 새 포트를 방화벽에 먼저 추가하고 서비스를 재시작한 뒤 listen 상태를 확인하며, 성공한 경우에만 기존 포트의 runtime/permanent 방화벽 규칙을 제거한다.

Swagger tag는 `Cube-License`, `Cube-Nic`, `Glue-RGW`, `Glue-GlueFS`처럼 기능 단위로 나뉜다. host/CCVM에서 Swagger `doc.json`을 요청하면 Glue path와 Glue model definition을 런타임에 제거한다. SCVM에서 요청하면 Glue 중심 화면이 되도록 Cube 운영/내부 통신 API path/tag를 숨기고, 인증, health, version, license 계열 API만 남긴다. 실제 route는 제거하지 않으므로 내부 fan-out 통신은 그대로 동작한다.

## RPM 설치 및 경로 정책

RPM은 `cmd/apiserver/main.go`를 `/usr/bin/ablestack-api`로 빌드하고 systemd 서비스로 설치한다. 설치 후 `%post` 스크립트에서 설정 병합, 방화벽 포트 오픈, 서비스 enable/start를 수행한다.

### 빌드

```bash
./scripts/build-rpm.sh
```

기본 RPM 버전은 루트의 `VERSION` 파일에서 읽는다. `scripts/build-rpm.sh`는 `CHANGELOG.md`에 같은 버전의 릴리즈 섹션이 있는지도 확인한다. 릴리즈 번호는 기본값 `1`을 사용하고, 필요하면 `RELEASE=2 ./scripts/build-rpm.sh`처럼 override한다. 포트 변경 빌드는 `API_PORT=28090 ./scripts/build-rpm.sh`를 사용하고 직접 `rpmbuild`할 때는 `--define "api_port 28090"`을 지정한다.

생성 위치:

```text
dist/rpm/rpmbuild/RPMS/<arch>/ablestack-api-<version>-<release>.el9.<arch>.rpm
dist/rpm/rpmbuild/SRPMS/ablestack-api-<version>-<release>.el9.src.rpm
```

### 설치/업데이트

로컬 RPM은 `rpm -Uvh`보다 `dnf install` 또는 `dnf upgrade`로 설치한다. `dnf`를 사용해야 저장소에 있는 필수 패키지를 함께 해결할 수 있다.

```bash
dnf install "dist/rpm/rpmbuild/RPMS/x86_64/ablestack-api-$(cat VERSION)-1.el9.x86_64.rpm"
```

업데이트:

```bash
dnf upgrade "dist/rpm/rpmbuild/RPMS/x86_64/ablestack-api-$(cat VERSION)-1.el9.x86_64.rpm"
```

설치 후 운영 포트를 바꿀 때는 RPM에 포함된 helper를 실행한다.

```bash
sudo /usr/libexec/ablestack-api/configure-api-port.sh 28090
```

helper는 새 runtime/permanent 방화벽 규칙 추가, `/etc/ablestack/ablestack-api.env` 갱신, 서비스 재시작, 새 포트 listen 확인, 기존 방화벽 규칙 제거를 순서대로 수행한다. 새 포트가 열리지 않으면 기존 env와 서비스 포트로 복구하고 실패한 새 방화벽 규칙을 제거한다. env를 직접 수정하면 방화벽은 자동 변경되지 않으므로 helper 사용을 기본 운영 절차로 한다. 이후 RPM 업그레이드는 env에 저장된 유효한 운영 포트를 보존하며, 포트 값이 없는 구버전 설치본만 기본값 `18090`으로 이전한다.

### RPM 설치 시 자동 처리

| 처리 | 설명 |
| --- | --- |
| Binary 설치 | `/usr/bin/ablestack-api` |
| systemd unit 설치 | `/usr/lib/systemd/system/ablestack-api.service` |
| 서비스 환경 파일 설치 | `/etc/ablestack/ablestack-api.env` |
| 설정 파일 설치 | `/etc/ablestack/auth.json`, `/etc/ablestack/properties/cluster.json` |
| 템플릿 설치 | `/etc/ablestack/xml-template/*` |
| shell 리소스 설치 | `/etc/ablestack/shell/*` |
| VM 설정 디렉터리 생성 | `/etc/ablestack/vmconfig/ccvm`, `/etc/ablestack/vmconfig/scvm` |
| 설정 병합 | 기존 JSON 값은 유지하고 누락된 key만 추가 |
| 방화벽 처리 | `firewall-cmd`가 있으면 `firewalld enable --now`, `18090/tcp` open |
| API 서비스 처리 | `systemctl enable/start ablestack-api.service`, 업데이트 시 새 포트로 restart 및 listen 확인 |

### RPM dependency 기준

설치 자체를 막는 hard dependency는 최소화한다.

| 구분 | 패키지/명령 | 설명 |
| --- | --- | --- |
| Hard dependency | `systemd`, `bash`, `python3` | 서비스 등록과 보안 증적·Samba·shell helper 실행에 필요. Linux 인증과 CCVM 자동 백업은 Go에서 처리 |
| Build dependency | `golang >= 1.26.2`, `libvirt-devel`, `pkgconfig` | RPM 빌드 서버에서만 필요 |
| Recommended | `firewalld` | 있으면 설치 시 18090/tcp 오픈 |
| Runtime command | `pcs`, `virsh`, `qemu-img`, `genisoimage`, `ssh/scp`, `ceph`, `rbd`, `radosgw-admin`, `podman`, `smbpasswd`, `pdbedit`, `realm`, `lsblk`, `nmcli` | 해당 기능 실행 시 필요. RPM 설치를 막지는 않음 |

예를 들어 `pcs`가 없는 저장소에서도 RPM 설치는 가능해야 한다. 단, PCS 관련 API를 호출하면 실제 명령이 없으므로 해당 API에서 실패한다.

SCVM 전용 Glue API가 사용하는 Ceph, podman, Samba, realmd 계열 명령은 SCVM 이미지 또는 SCVM role 패키지에서 제공한다. `ablestack-api` RPM은 host/SCVM/CCVM 공통 패키지이므로 이러한 role-specific 명령을 hard dependency로 추가하지 않는다.

CCVM Wall 모니터링 구성은 `/usr/share/ablestack/ablestack-wall/python`의 `host_ping_test.py`, `config_netdive.py`, `start_services.py`, `config_wall.py`, `config_loki.py`, `config_smtp.py`와 해당 Python 모듈을 사용한다. 이 자산은 CCVM Wall 이미지에서 제공해야 하며 Host와 SCVM의 공통 API RPM 의존성으로 강제하지 않는다. 스크립트나 모듈이 없는 CCVM에서는 API가 해당 실행 단계를 구체적인 원인과 함께 실패 처리한다.

### 업데이트 시 설정 보존

`/etc/ablestack` 아래 설정 파일은 RPM spec에서 `%config(noreplace)`로 관리한다. 따라서 업데이트 시 기존 운영 파일을 덮어쓰지 않는다.

`auth.json`과 `properties/cluster.json`은 새 RPM에서 `.rpmnew`가 생기면 `merge-json-defaults.py`가 기존 파일에 없는 key만 추가한다. 기존 값은 유지된다. 기존 `cluster.json`이 0바이트라면 운영 데이터가 없다고 보고 기본 스켈레톤을 채운다.

초기 `cluster.json`은 값 없이 구조만 제공한다.

```json
{
  "clusterConfig": {
    "type": "",
    "backup_path": "",
    "ccvm": {
      "ip": ""
    },
    "mngtNic": {
      "cidr": "",
      "gw": "",
      "dns": ""
    },
    "pcsCluster": {
      "hostname1": "",
      "hostname2": "",
      "hostname3": "",
      "hostname4": "..."
    },
    "hosts": [
      {
        "index": "",
        "hostname": "",
        "ablecube": "",
        "scvmMngt": "",
        "ablecubePn": "",
        "scvm": "",
        "scvmCn": ""
      }
    ],
    "external_timeserver": "",
    "storage_network": ""
  },
  "systemProfile": {
    "bootstrap": {
      "scvm": "",
      "ccvm": "",
      "wall": "",
      "gfs_configure": "",
      "local_configure": ""
    },
    "license": {
      "status": "",
      "type": ""
    },
    "security_patch": {
      "status": ""
    }
  }
}
```

### 환경 변수

| 변수 | 기본값 | 설명 |
| --- | --- | --- |
| `ABLESTACK_CLUSTER_JSON` | `/etc/ablestack/properties/cluster.json` | cluster.json 절대 경로 override |
| `ABLESTACK_CONFIG_PATH` | `/etc/ablestack` | properties, xml-template, shell 기준 루트 |
| `ABLESTACK_STATE_PATH` | `/etc/ablestack/vmconfig` | 생성된 CCVM/SCVM XML, secret 등 VM 설정 루트 |
| `ABLESTACK_API_LOG` | `/var/log/ablestack/api.log` | 모든 API 요청과 주요 action 로그 |
| `ABLESTACK_DETAIL_LOG` | `/var/log/ablestack/detail.log` | API 오류, panic, 실패 action의 원인/단계/누락 필드 등 상세 진단 로그 |
| `ABLESTACK_JOB_LOG` | `/var/log/ablestack/job.log` | background job 실패, 상태 변화, 자동 작업 결과 로그 |
| `ABLESTACK_LOG_ARCHIVE_DIR` | `/var/log/ablestack/archive` | 날짜가 지난 로그 gzip archive 경로 |
| `ABLESTACK_LOG_RETENTION_DAYS` | `90` | archive 로그 보관 기간(일) |
| `ABLESTACK_API_SCHEME` | `http` | 노드 간 API 호출 scheme |
| `ABLESTACK_API_PORT` | `18090` | 서버 listen 및 노드 간 API 호출 대상 port |
| `ABLESTACK_NODE_ROLE` | 없음 | 노드 역할 override. `/api/v1/glue` route는 `scvm`일 때만 등록 |
| `ABLESTACK_NODE_ROLE_FILE` | `/etc/ablestack/node-role` | 노드 역할 파일 경로 override |
| `ABLESTACK_SECURITY_PATCH_SCRIPT` | `/usr/local/sbin/security_patch.sh` | 보안 패치 스크립트 override |

### 서비스 확인

```bash
systemctl status ablestack-api.service
journalctl -u ablestack-api.service -f
tail -f /var/log/ablestack/api.log /var/log/ablestack/detail.log /var/log/ablestack/job.log
curl -sS http://127.0.0.1:18090/api/v1/cube/cluster/health
```

## 전체 구조

```text
Client or Web UI
  -> Gin Router: cmd/apiserver/main.go
    -> Handler: internal/handler/cube, internal/handler/glue, internal/handler/auth
      -> Model/DTO: internal/model/cube, internal/model/glue
      -> Service: internal/service/clusterconfig, internal/service/controller, internal/service/glueservice
      -> OS command: ceph, pcs, virsh, nmcli, lsblk, rbd, mysqldump, ssh-keyscan
```

### 주요 패키지 역할

| 경로 | 역할 |
| --- | --- |
| `cmd/apiserver` | Gin 서버 생성, middleware, route 등록, Swagger 노출 |
| `internal/handler/cube` | Cube 운영 API 대부분 구현 |
| `internal/handler/glue` | SCVM 전용 Glue API namespace. legacy glue-api 기능을 SSH 없이 이식 |
| `internal/model/cube` | Cube API 요청/응답 구조체 |
| `internal/model/glue` | Glue API 요청/응답 구조체 |
| `internal/service/clusterconfig` | `cluster.json`, `/etc/hosts` 적용 로직 |
| `internal/service/controller` | background job handler 등록/실행, 공통 error log 관리 |
| `internal/service/glueservice` | SCVM 로컬 `ceph`, `rbd`, `ceph orch` 명령 실행과 입력 검증 |
| `docs` | Swagger 산출물 및 운영 문서 |

## 공통 사용법

### Health check

```bash
curl -sS http://<ablecube-ip>:18090/api/v1/cube/cluster/health
```

응답 예:

```json
{
  "status": "ok"
}
```

### JSON POST 기본 형식

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/<path> \
  -H "Content-Type: application/json" \
  -d '{"action":"status"}'
```

## API 요약

| 종류 | Method | Endpoint | Action/Query | 설명 |
| --- | --- | --- | --- | --- |
| Auth | POST | `/auth/login` | `id,password` | 웹/API용 Bearer access token 발급 |
| Auth | GET | `/auth/me` | Bearer token | 현재 access token 검증 |
| Auth | POST | `/auth/internal-token/rotate` | Bearer token | 내부 호스트 간 공유 토큰 교체 |
| Health | GET | `/health` | 없음 | 라이선스 등록 전에도 호출 가능한 API server health |
| 디스크 | GET | `/cube/disk` | `action=list,gfs,rbd,detail`, `view=tree,flat,list` | 디스크, multipath, RBD, RAID controller 정보 조회 |
| Multipath | POST | `/cube/multipath/sync` | `action=sync,rescan` | SSH/SCP 없이 ablecube host 대상 SCSI rescan 및 multipath bindings/wwids 동기화 |
| 네트워크 | GET | `/cube/nics` | `action=list,detail` | ethernet, bridge, bond, IP, MAC, speed 조회 |
| 호스트 | GET | `/cube/hosts` | 없음 | `/etc/hosts`를 네트워크/역할별로 조회 |
| 클러스터 | GET | `/cube/cluster/health` | `option=host,scvm,ccvm` 콤마 조합 | API 생존 및 대상 노드 상태 점검 |
| 배포 상태 | GET | `/cube/deploy/status` | 없음 | UI용 배포 단계 enum과 기존 상태 스냅샷 조회 |
| 배포 실행 | POST | `/cube/deploy/run` | `mode,only,skip` | 라이선스/클러스터/SCVM/스토리지/CCVM 준비를 job으로 순차 실행 |
| 배포 실행 | GET | `/cube/deploy/jobs` | 없음 | 최근 올인원 배포 job 목록 조회 |
| 배포 실행 | GET | `/cube/deploy/jobs/{job_id}` | path `job_id` | 올인원 배포 job 상세 상태 조회 |
| SCVM | POST | `/cube/scvm/bootstrap` | `run_script` | 대표 SCVM의 `/root/bootstrap.sh` 실행 후 SCVM API health, 라이선스 등록/status 확인 |
| CCVM | POST | `/cube/ccvm/bootstrap` | `run_script` | 라이선스가 등록된 CCVM의 `/root/bootstrap.sh` 실행 후 API health와 라이선스 status 확인 |
| 클러스터 | GET | `/cube/cluster/config` | 없음 | 다운로드용 `clusterConfig`와 `security` 조회 |
| 클러스터 | POST | `/cube/cluster/apply` | `insert,remove,reset,check` | 클러스터 구성 오케스트레이션 |
| 클러스터 | POST | `/cube/cluster/apply-local` | 내부용 | 각 노드에서 실제 cluster config 적용 |
| System Profile | GET | `/cube/system/config` | 없음 | `systemProfile` 조회 |
| System Profile | POST | `/cube/system/config` | `status,update,allUpdate,reset` | `systemProfile` 조회/수정/초기화 |
| URL | GET | `/cube/url` | `option=cloudCenter,wallCenter,storageCenter` | Cloud/Wall/Storage 접속 URL 반환 |
| CloudInit | POST | `/cube/cloudinit/ccvm/generate` | body optional | `cluster.json` 기반 CCVM cloud-init ISO 생성 및 ablecubePn 대상 복사 |
| CloudInit | POST | `/cube/cloudinit/scvm/generate` | 없음 | 현재 노드와 `cluster.json` 기반 SCVM cloud-init ISO 생성 |
| CCVM | GET | `/cube/ccvm/status` | 없음 | CCVM 상태 조회 |
| CCVM | POST | `/cube/ccvm/xml` | body | CCVM libvirt XML 생성 및 대상 노드 배포 |
| CCVM | POST | `/cube/ccvm/lifecycle` | `initialize,setup,reset,copy,start,stop,restart,delete` | Cloud Center VM lifecycle |
| CCVM | POST | `/cube/ccvm/edit` | body | CCVM CPU/메모리 XML 수정 |
| CCVM | POST | `/cube/ccvm/secondary/resize` | body | CCVM secondary 용량 추가 |
| CCVM | POST | `/cube/ccvm/service/control` | `start,restart,stop,status` | CCVM의 서비스 제어 |
| CCVM Snapshot | POST | `/cube/ccvm/snap` | `list,backup,rollback` | HCI/HCI-FS CCVM RBD snapshot |
| CCVM Backup | POST | `/cube/ccvm/backup` | `backup,status,list,overview,schedule,unschedule,schedule-delete,unschedule-delete` | VM/Standalone CCVM 파일 백업 |
| CCVM Restore | POST | `/cube/ccvm/restore` | body | CCVM 파일 백업 복구 |
| SCVM | GET | `/cube/scvm/status` | 없음 | Storage Center VM 상태 조회 |
| SCVM | POST | `/cube/scvm/xml` | body | SCVM libvirt XML 및 hugepage 설정 생성 |
| SCVM | POST | `/cube/scvm/lifecycle` | `setup,reset,start,stop,delete,resource` | Storage Center VM lifecycle |
| PCS | POST | `/cube/pcs/control` | `setup,config,create,enable,disable,move,cleanup,status,remove,destroy,stop,sync,ccvm-status` | Cloud Center PCS 제어 |
| Glue | GET | `/cube/gluecluster/status` | 없음 | Glue cluster 상세 상태 조회 |
| Glue | POST | `/cube/gluecluster/update` | `set_noout,unset_noout` | 유지보수 모드 설정/해제 |
| Glue API | GET | `/glue` | 없음 | SCVM 전용 Glue API 등록/role 상태 |
| Glue API | GET | `/glue/status`, `/glue/hosts`, `/glue/version` | 없음 | SCVM 로컬 Glue/Ceph 상태 조회 |
| Glue API | 여러 Method | `/glue/pool`, `/glue/image`, `/glue/service` | endpoint별 body/path | SCVM 로컬 Ceph/RBD/orch 명령 실행 |
| Glue API | 여러 Method | `/glue/gluefs`, `/glue/nfs`, `/glue/rgw` | endpoint별 body/query/path | SCVM 로컬 GlueFS/NFS/RGW 조회/생성/수정/삭제 명령 실행 |
| Glue API | 여러 Method | `/glue/nvmeof` | endpoint별 body/query | SCVM 로컬 `ceph orch`, `rbd`, `podman run/exec` 기반 NVMe-oF 명령 실행 |
| Glue API | 여러 Method | `/glue/iscsi` | endpoint별 body/query | SCVM 로컬 `ceph orch`, Ceph dashboard API, `podman exec gwcli` 기반 iSCSI 명령 실행 |
| Glue API | 여러 Method | `/glue/smb` | endpoint별 body/query | SCVM 로컬 Samba 실행 스크립트 기반 SMB 명령 실행 |
| Glue API | 여러 Method | `/glue/mirror` | endpoint별 body/path/query | SCVM 로컬 `rbd mirror`와 bootstrap token import/export 기반 Mirror 명령 실행 |
| GFS | GET | `/cube/gfs/resource/status` | 없음 | GFS 관련 PCS 리소스 상태 |
| GFS | GET | `/cube/gfs/disk/status` | 없음 | GFS2 마운트 디스크 상태 |
| License | POST | `/cube/license` | `status,register` | 라이선스 조회/등록 |
| License | POST | `/cube/license/apply` | `register,status,roles` | 마스터 기준 ablecube/SCVM/CCVM role별 라이선스 fan-out |
| Version | GET | `/version` | 없음 | 현재 노드의 OS/Kernel/Cockpit/Mold/ABLESTACK 패키지 버전 조회, HCI 계열은 Glue 버전 포함 |
| Version | POST | `/cube/version/update` | `info,run`, `update_type=all,mold` | 마운트된 ISO 버전 조회 및 `/opt/ABLESTACK_UPDATE` 복사 후 업데이트 |
| Security | POST | `/cube/security/patch` | body flags | 보안 패치 실행 및 상태 업데이트 |
| SSH Key | POST | `/cube/ssh/key` | `generate,download,upload` | `/root/.ssh` 키 생성, 암호화 단일 파일 다운로드/업로드 |
| DB | POST | `/cube/db/dump` | `instantBackup,regularBackup,deleteOldBackup,checkBackup,deactiveBackup` | CCVM DB dump 및 스케줄 관리 |

## Glue API

`/api/v1/glue`는 SCVM Cockpit에서 사용할 SCVM 전용 namespace다. API 서버 시작 시 SCVM role로 판정된 경우에만 route를 등록하므로, 물리 host와 CCVM에서 호출하면 `404 Not Found`가 반환된다. Swagger 문서도 같은 기준을 사용해 host/CCVM에서는 `Glue-*` tag와 `/glue` path를 숨기고, SCVM에서는 Glue API와 인증, health, version, license 계열 API만 노출한다. 인증/라이선스 middleware는 기존 API와 동일하게 적용되므로, 라이선스 등록 전 public health처럼 열리는 API가 아니다.

SCVM role 판정 순서:

1. `ABLESTACK_NODE_ROLE=scvm`
2. `ABLESTACK_NODE_ROLE_FILE` 또는 기본 `/etc/ablestack/node-role` 파일 값이 `scvm`
3. `cluster.json`의 `hosts[].scvmMngt` 또는 `hosts[].scvm` 값이 현재 노드 IP와 일치

`clusterConfig.type`이 HCI인지 여부만으로는 SCVM을 판단하지 않는다. 물리 host도 HCI 타입이므로, Glue API 허용 조건에는 사용할 수 없다.

SCVM template/cloud-init에는 `ABLESTACK_NODE_ROLE=scvm` 또는 `/etc/ablestack/node-role` 파일을 서비스 시작 전에 넣는 것을 권장한다. `cluster.json` 기반 판정은 파일이 API 서버 시작 시점에 존재할 때만 route 등록에 사용할 수 있다.

현재 `/api/v1/glue/status`, `/api/v1/glue/hosts`, `/api/v1/glue/version`, `/api/v1/glue/pool`, `/api/v1/glue/image`, `/api/v1/glue/service`, `/api/v1/glue/gluefs`, `/api/v1/glue/nfs`, `/api/v1/glue/rgw`, `/api/v1/glue/nvmeof`, `/api/v1/glue/iscsi`, `/api/v1/glue/smb`, `/api/v1/glue/mirror`는 SCVM 로컬 명령 또는 SCVM 로컬 Ceph dashboard API로 동작한다. 명령은 SSH와 shell pipeline을 거치지 않고 `exec.CommandContext`로 실행하며, pool/image/service/filesystem/cluster/user/bucket/NVMe-oF NQN/UUID/IP/iSCSI IQN/SMB path 값은 허용 문자만 통과시킨다. SMB password와 RGW secret key는 명령 실패 응답에서 마스킹한다.

| Endpoint | 실행 명령 |
| --- | --- |
| `GET /glue/status` | `ceph -s -f json` |
| `GET /glue/hosts` | `ceph orch host ls -f json` |
| `GET /glue/version` | `ceph versions` |
| `GET /glue/pool?pool_type=rbd` | `ceph osd pool ls --format json` 후 Go 코드에서 필터링 |
| `DELETE /glue/pool/{pool_name}` | `ceph config get/set mon mon_allow_pool_delete`, `ceph osd pool rm ...` |
| `GET /glue/image` | RBD pool 목록 조회 후 `rbd ls -p <pool> --format json` |
| `GET /glue/image?pool_name=<pool>` | `rbd ls -l -p <pool> --format json` |
| `GET /glue/image?pool_name=<pool>&image_name=<image>` | `rbd info <pool>/<image> --format json` |
| `POST /glue/image` | `rbd create --size <MiB> <pool>/<image>` |
| `DELETE /glue/image` | `rbd rm <pool>/<image>` |
| `GET /glue/service` | `ceph orch ls -f json` |
| `POST /glue/service/{service_name}?control=restart` | `ceph orch <start|stop|restart|redeploy> <service_name>` |
| `DELETE /glue/service/{service_name}` | `ceph orch rm <service_name>` |
| `GET /glue/gluefs` | `ceph fs status -f json`, `ceph fs ls -f json` |
| `POST /glue/gluefs/{fs_name}` | `ceph fs volume create <fs_name> --placement <hosts>`, pool rename, pool size 설정 |
| `PUT /glue/gluefs` | `ceph fs rename`, pool rename, 필요 시 `ceph orch apply mds <new_name> --placement <hosts>` |
| `DELETE /glue/gluefs/{fs_name}` | subvolume group 확인, `ceph config get/set mon mon_allow_pool_delete`, `ceph fs volume rm ...` |
| `GET /glue/gluefs/info/{fs_name}` | `ceph fs get <fs_name> -f json` |
| `GET /glue/gluefs/subvolume/group?vol_name=<fs>` | `ceph fs subvolumegroup ls/info/getpath/snapshot ls` |
| `POST /glue/gluefs/subvolume/group` | `ceph fs subvolumegroup create <vol> <group> <size> <data_pool> --mode <mode>` |
| `PUT /glue/gluefs/subvolume/group` | `ceph fs subvolumegroup resize <vol> <group> <size> --no_shrink` |
| `DELETE /glue/gluefs/subvolume/group` | `ceph fs subvolumegroup rm <vol> <group> --force` |
| `GET /glue/nfs?cluster_id=<id>` | `ceph nfs cluster info [cluster_id]` |
| `POST /glue/nfs/{cluster_id}/{port}` | NFS service spec 생성 후 `ceph orch apply -i <spec>` |
| `PUT /glue/nfs/{cluster_id}/{port}` | `ceph orch apply -i <spec>`, `ceph orch redeploy nfs.<cluster_id>` |
| `DELETE /glue/nfs/{cluster_id}` | `ceph nfs cluster rm <cluster_id>` |
| `POST /glue/nfs/ingress` | ingress service spec 생성 후 `ceph orch apply -i <spec>` |
| `PUT /glue/nfs/ingress` | `ceph orch apply -i <spec>`, `ceph orch redeploy ingress.<service_id>` |
| `GET /glue/nfs/export?cluster_id=<id>` | `ceph nfs export ls <cluster_id> --detailed` |
| `POST /glue/nfs/export/{cluster_id}` | NFS export JSON spec 생성 후 `ceph nfs export apply <cluster_id> -i <spec>` |
| `PUT /glue/nfs/export/{cluster_id}` | export_id 포함 JSON spec 생성 후 `ceph nfs export apply <cluster_id> -i <spec>` |
| `DELETE /glue/nfs/export/{cluster_id}/{export_id}` | detailed export 조회 후 `ceph nfs export rm <cluster_id> <pseudo>` |
| `GET /glue/rgw` | `ceph orch ls --service_type rgw -f json`, `ceph orch ps --daemon_type rgw -f json` |
| `POST /glue/rgw` | realm/zonegroup/zone 생성, `ceph orch apply rgw ... --placement <hosts> --port <port>` |
| `PUT /glue/rgw` | `ceph orch apply rgw ... --placement <hosts> --port <port>` |
| `POST /glue/rgw/quota` | `radosgw-admin quota set --uid <user> ...`, `radosgw-admin quota enable/disable ...` |
| `GET /glue/rgw/user?username=<user>` | `radosgw-admin user list/info/stats` |
| `POST /glue/rgw/user` | `radosgw-admin user create`, 필요 시 S3 key 생성 |
| `PUT /glue/rgw/user` | `radosgw-admin user modify`, 필요 시 기존 key 제거 후 S3 key 생성 |
| `DELETE /glue/rgw/user` | `radosgw-admin user rm --uid <user> --purge-data` |
| `GET /glue/rgw/bucket?detail=true` | `radosgw-admin bucket list/stats` |
| `POST /glue/rgw/bucket` | Ceph dashboard `POST /api/rgw/bucket` |
| `PUT /glue/rgw/bucket` | Ceph dashboard `PUT /api/rgw/bucket/<bucket>` |
| `DELETE /glue/rgw/bucket` | `radosgw-admin bucket rm --bucket <bucket> --purge-objects --yes-i-really-mean-it` |
| `POST /glue/nvmeof` | `ceph osd pool create`, `rbd pool init`, `ceph osd pool set`, `ceph orch apply -i <spec>` |
| `POST /glue/nvmeof/image/download` | `podman pull <nvmeof-cli-image>` |
| `GET /glue/nvmeof/target` | `podman ps --format json`, `podman exec ... rpc.py nvmf_get_subsystems` |
| `POST /glue/nvmeof/target` | `podman run <nvmeof-cli> subsystem/listener/host/namespace`, 필요 시 `rbd create` |
| `GET /glue/nvmeof/subsystem` | `podman run <nvmeof-cli> --format json ... subsystem list` |
| `POST /glue/nvmeof/subsystem` | `podman run <nvmeof-cli> subsystem add`, `listener add`, `host add` |
| `DELETE /glue/nvmeof/subsystem` | `podman run <nvmeof-cli> subsystem del` |
| `GET /glue/nvmeof/namespace` | `podman run <nvmeof-cli> --format json ... namespace list` |
| `POST /glue/nvmeof/namespace` | `podman run <nvmeof-cli> namespace add`, 필요 시 `rbd create` |
| `DELETE /glue/nvmeof/namespace` | `podman run <nvmeof-cli> namespace del`, 필요 시 `rbd rm` |
| `POST /glue/iscsi` | iSCSI service spec 생성 후 `ceph orch apply -i <spec>` |
| `PUT /glue/iscsi` | `ceph orch apply -i <spec>`, `ceph orch redeploy iscsi.<service_id>` |
| `GET /glue/iscsi/discovery` | Ceph dashboard `GET /api/iscsi/discoveryauth` |
| `PUT /glue/iscsi/discovery` | Ceph dashboard `PUT /api/iscsi/discoveryauth` |
| `GET /glue/iscsi/target` | Ceph dashboard `GET /api/iscsi/target[/<iqn>]` |
| `POST /glue/iscsi/target` | Ceph dashboard `POST /api/iscsi/target` |
| `PUT /glue/iscsi/target` | Ceph dashboard `PUT /api/iscsi/target/<iqn>` |
| `DELETE /glue/iscsi/target` | Ceph dashboard `DELETE /api/iscsi/target/<iqn>` |
| `DELETE /glue/iscsi/target/purge` | `podman ps --format json`, `podman exec -i <iscsi-container> gwcli /iscsi-targets delete <iqn>` |
| `GET /glue/smb` | `<SMB_SCRIPT> select` |
| `POST /glue/smb` | `<SMB_SCRIPT> delete`, `<SMB_SCRIPT> create ...` |
| `DELETE /glue/smb` | `<SMB_SCRIPT> delete` |
| `POST /glue/smb/folder` | `<SMB_SCRIPT> share_folder_add ...` |
| `DELETE /glue/smb/folder` | `<SMB_SCRIPT> share_folder_delete ...` |
| `POST /glue/smb/user` | `<SMB_SCRIPT> user_create normal ...` |
| `PUT /glue/smb/user` | `<SMB_SCRIPT> user_update normal ...` |
| `DELETE /glue/smb/user` | `<SMB_SCRIPT> user_delete normal ...` |
| `GET /glue/mirror` | `rbd mirror pool status --format json --pretty-format` |
| `POST /glue/mirror` | `rbd mirror pool enable --site-name <local> -p <pool> image`, `ceph orch apply rbd-mirror`, `rbd mirror pool peer bootstrap create`, `ceph auth caps/get-key`, 선택적으로 `rbd mirror pool peer bootstrap import --token-path <file>` |
| `PUT /glue/mirror` | `rbd image-meta set rbd/MOLD-DR interval <interval>` |
| `DELETE /glue/mirror` | mirrored image disable, peer remove, `rbd mirror pool disable --pool <pool>`, `ceph orch rm rbd-mirror`, `rbd rm rbd/MOLD-DR` |
| `POST /glue/mirror/{mirrorPool}` | path pool 기준 `POST /glue/mirror`와 동일 |
| `DELETE /glue/mirror/{mirrorPool}` | path pool의 mirrored image disable, peer remove, `rbd mirror pool disable --pool <pool>` |
| `DELETE /glue/mirror/garbage?mirror_pool=<pool>` | local mirror peer/auth/service/metadata image cleanup |
| `GET /glue/mirror/image/{mirrorPool}` | `rbd mirror pool status <pool> --verbose --format json --pretty-format` |
| `GET /glue/mirror/image/{mirrorPool}/{imageName}` | `rbd mirror image status --pool <pool> --image <image> --format json` |

예시:

```bash
curl -sS http://<scvm-ip>:18090/api/v1/glue/status \
  -H "Authorization: Bearer <access_token>"

curl -sS "http://<scvm-ip>:18090/api/v1/glue/pool?pool_type=rbd" \
  -H "Authorization: Bearer <access_token>"

curl -X POST http://<scvm-ip>:18090/api/v1/glue/image \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{"pool_name":"rbd","image_name":"vm01","size":10}'
```

NVMe-oF CLI image는 기본적으로 `localhost:15000/glue/nvmeof-cli:Diplo`를 사용한다. 운영 환경에서 이미지나 REST endpoint가 다르면 `ABLESTACK_GLUE_NVME_OF_CLI_IMAGE`, `ABLESTACK_GLUE_NVME_OF_SERVER_ADDRESS`, `ABLESTACK_GLUE_NVME_OF_SERVER_PORT`로 override할 수 있다.

iSCSI target/discovery API는 Ceph dashboard REST API를 사용한다. `ABLESTACK_GLUE_DASHBOARD_URL`이 비어 있으면 `ceph mgr services -f json`에서 dashboard URL을 조회하고, 인증은 `ABLESTACK_GLUE_DASHBOARD_USER`, `ABLESTACK_GLUE_DASHBOARD_PASSWORD`를 사용한다. password가 비어 있으면 service 생성/수정은 가능하지만 target/discovery API는 실패한다.

SMB API는 SCVM 로컬에서 `ABLESTACK_GLUE_SMB_SCRIPT`가 가리키는 스크립트를 실행한다. 기본값은 RPM 설치 경로인 `/etc/ablestack/shell/Samba-Execute.sh`다. API 계층에서는 SSH host 반복을 제거했고, 요청은 JSON/form/query를 모두 받아 같은 검증 흐름을 사용한다. 실제 SCVM 검증 순서는 [SCVM SMB API 테스트 체크리스트](scvm-smb-test-checklist.md)를 기준으로 한다.

Mirror API는 remote cluster에 SSH/scp로 직접 접속하지 않는다. 각 SCVM에서 자기 local cluster의 `rbd mirror` 명령만 실행하고, peer 연결은 bootstrap token을 API 응답/요청으로 교환해 import한다.

양방향 Mirror 설정 순서:

1. A SCVM에서 `POST /glue/mirror`에 `local_cluster_name`, `mirror_pool`을 보내 A의 `local_token`을 받는다.
2. B SCVM에서 `POST /glue/mirror`에 `local_cluster_name`, `mirror_pool`, A의 `remote_token`을 보내 A token을 import하고 B의 `local_token`을 받는다.
3. A SCVM에서 `POST /glue/mirror`에 `mirror_pool`, B의 `remote_token`만 보내 B token을 import한다. 이 호출은 새 token을 만들지 않고 import만 수행한다.

## Auth API

웹 UI/API 클라이언트는 `/auth/login`에서 발급받은 access token을 `Authorization: Bearer <token>` 형태로 사용한다.
인증은 Linux 계정 인증만 사용하며, `/etc/ablestack/auth.json`의 `linux.allowed_users`, `linux.allowed_groups`로 허용 계정을 제한한다.
API access token 서명값은 활성 라이선스의 `license_key`에서 파생한다. 라이선스가 없거나 만료된 경우 라이선스 조회/등록과 Swagger를 제외한 API는 사용할 수 없다.
기본값은 `root` 사용자 또는 `wheel` 그룹 사용자를 허용한다.

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"id":"root","password":"<linux-password>"}'
```

```bash
curl http://<ablecube-ip>:18090/api/v1/auth/me \
  -H "Authorization: Bearer <access_token>"
```

### Cockpit 자동 인증

Cockpit 화면에서는 사용자가 이미 Linux 계정으로 로그인했으므로 비밀번호를 다시 입력하지 않고 로컬 helper를 실행해 Bearer 토큰을 발급한다. Cockpit에 로그인하는 것만으로는 helper가 자동 실행되지 않으며, ABLESTACK Cockpit 화면 초기화 코드에서 명시적으로 호출해야 한다. 이 시점에 인증 서명값이 비어 있으면 helper가 생성한다.

```bash
/usr/bin/ablestack-auth-token
```

기본 출력:

```json
{
  "code": 200,
  "token_type": "Bearer",
  "access_token": "<jwt>",
  "authorization": "Bearer <jwt>",
  "expires_in": 3600,
  "subject": "root"
}
```

Cockpit UI에서는 `cockpit.spawn()`으로 helper를 호출하고 `authorization` 값을 `cockpit.fetch()`의 `Authorization` 헤더에 붙인다.

```javascript
const raw = await cockpit.spawn(["/usr/bin/ablestack-auth-token"]);
const token = JSON.parse(raw).authorization;

const response = await cockpit.fetch("http://127.0.0.1:18090/api/v1/cube/hosts", {
  headers: {
    Authorization: token
  }
});
```

root가 아닌 Cockpit 사용자로 로그인했지만 helper를 root 권한으로 실행해야 하는 환경에서는 현재 Cockpit 사용자를 명시한다. helper는 대상 사용자가 `auth.json`의 허용 사용자/그룹에 포함되는지 확인한다.

```javascript
const raw = await cockpit.spawn(
  ["/usr/bin/ablestack-auth-token", "--user", cockpit.user.name],
  { superuser: "require" }
);
```

CLI에서 헤더 값만 필요하면:

```bash
/usr/bin/ablestack-auth-token --plain
```

라이선스 등록이 성공하면 `cluster.json`의 `security.internal_token`이 없을 때 자동으로 생성된다.
내부 fan-out 요청은 `X-Cube-Internal-Token` 헤더를 사용한다.

API access token 서명값은 활성 라이선스의 `license_key`에서 파생되므로, 각 호스트에 동일한 라이선스가 등록되면 별도 인증 서명값 동기화 없이 같은 token을 검증한다.

`cluster apply`는 insert 흐름에서 요청 body의 `security.internal_token`이 있으면 그 값을 그대로 현재 호스트와 `apply-local` 요청 body에 사용한다. 요청 body에 token이 없으면 현재 호스트의 `security.internal_token`을 보장하며, 기존 값이 없을 때 새 token을 생성한다. 원격 호스트의 token이 아직 비어 있는 최초 bootstrap 상태에서는 body token과 `X-Cube-Internal-Token` 헤더가 일치할 때만 `apply-local`을 허용하고, 이후 원격 호스트의 `cluster.json`에 같은 token을 저장한다.

내부 token 교체는 아래 API로 수행한다. 이 API는 현재 `cluster.json`의 `hosts[].ablecube` 대상에 새 token을 적용한 뒤 현재 호스트의 token도 교체한다.

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/auth/internal-token/rotate \
  -H "Authorization: Bearer <access_token>"
```

```http
POST /api/v1/auth/internal-token/apply
X-Cube-Internal-Token: <current-security.internal_token>
```

`internal-token/apply` API는 내부 API 서버 간 호출용이다. 사람이 Swagger나 curl에서 일반 테스트할 때 직접 호출하지 않는다.

## Cluster API

### `GET /cube/cluster/health`

기본 호출은 API 서버 생존 확인이다.

```bash
curl -sS http://<ablecube-ip>:18090/api/v1/cube/cluster/health
```

대상 노드 점검:

```bash
curl -sS "http://<ablecube-ip>:18090/api/v1/cube/cluster/health?option=host"
curl -sS "http://<ablecube-ip>:18090/api/v1/cube/cluster/health?option=scvm"
curl -sS "http://<ablecube-ip>:18090/api/v1/cube/cluster/health?option=ccvm"
curl -sS "http://<ablecube-ip>:18090/api/v1/cube/cluster/health?option=host,scvm"
```

특정 이름만 점검할 때는 `target_hostname`을 콤마로 여러 개 지정한다. 이름 규칙은 option별로 다르다.

| option | target_hostname 이름 |
| --- | --- |
| `host` | `hosts[].hostname` 값. 예: `ablecube12-1`, `NV1` |
| `scvm` | `scvm` + `hosts[].index`. 예: `scvm1`, `scvm2` |
| `ccvm` | 고정값 `ccvm` |

```bash
curl -sS "http://<ablecube-ip>:18090/api/v1/cube/cluster/health?option=host&target_hostname=ablecube31-1,ablecube31-2"
curl -sS "http://<ablecube-ip>:18090/api/v1/cube/cluster/health?option=scvm&target_hostname=scvm1,scvm2"
curl -sS "http://<ablecube-ip>:18090/api/v1/cube/cluster/health?option=host,scvm,ccvm&target_hostname=NV1,scvm1,ccvm"
```

`option` 없이 `target_hostname`만 지정하면 이름으로 role을 추론한다.

```bash
curl -sS "http://<ablecube-ip>:18090/api/v1/cube/cluster/health?target_hostname=NV1,scvm1,ccvm"
```

### `GET /cube/cluster/config`

`cluster.json` 다운로드에 필요한 `clusterConfig`와 `security`를 반환한다. `systemProfile`은 운영 상태라 다운로드 구성 파일에서는 제외한다. `security.internal_token`이 아직 없으면 이 조회 과정에서 생성해 응답에 포함한다.

```bash
curl -sS http://<ablecube-ip>:18090/api/v1/cube/cluster/config
```

### `GET /cube/deploy/status`

기존 개별 상태 API는 유지하고, UI가 배포 진행 화면을 단순하게 처리할 수 있도록 현재 배포 단계를 의미형 enum으로 반환한다. `ccfg_status`는 `clusterConfig`의 필수 값이 채워졌는지로 계산하고, `wall_monitoring_status`는 `systemProfile.bootstrap.wall` 값을 사용한다. `ablestack-vm`도 CloudCenter PCS/resource 상태를 사용하므로 `pcsCluster.hostnameN`이 구성 준비 완료 조건에 포함된다. `ablestack-vm`은 PCS 대상 1대부터 가능하고, `ablestack-hci`, `ablestack-hci-filesystem`은 기본 3대 이상이 필요하다.

```bash
curl -sS http://<ablecube-ip>:18090/api/v1/cube/deploy/status
```

대표 응답:

```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "os_type": "ablestack-hci",
    "stage": "cloud_vm_deploy",
    "stage_order": 7,
    "severity": "warning",
    "message_key": "cloud_vm_not_deployed",
    "available_actions": [
      "download_config_file",
      "open_storage_center",
      "deploy_cloud_vm"
    ],
    "raw": {
      "license_status": "true",
      "ccfg_status": "true",
      "scvm_status": "RUNNING",
      "scvm_bootstrap_status": "true",
      "sc_status": "HEALTH_OK",
      "cc_status": "UNKNOWN",
      "ccvm_status": "HEALTH_ERR",
      "ccvm_bootstrap_status": "false",
      "wall_monitoring_status": "false",
      "security_patch": "false"
    }
  }
}
```

`stage` 값:

| Stage | 의미 |
| --- | --- |
| `cluster_prepare` | 클러스터 구성 준비 필요 |
| `storage_vm_deploy` | 스토리지센터 VM 배포 필요 |
| `storage_vm_configure` | 스토리지센터 VM 구성 필요 |
| `storage_cluster_configure` | 스토리지 클러스터 구성 필요 |
| `hci_shared_file_configure` | HCI 공유 파일 구성 필요 |
| `gfs_storage_configure` | GFS 스토리지 구성 필요 |
| `local_storage_configure` | 로컬 스토리지 구성 필요 |
| `cloud_vm_deploy` | 클라우드센터 VM 배포 필요 |
| `cloud_vm_configure` | 클라우드센터 VM 구성 필요 |
| `cloud_cluster_configure` | 클라우드센터 클러스터 구성 필요 |
| `cloud_resource_configure` | 클라우드센터 리소스 구성 필요 |
| `monitoring_configure` | 모니터링센터 구성 필요 |
| `ready` | 배포 완료 상태 |
| `unsupported_cluster_type` | 지원하지 않는 cluster type |

`available_actions` 값:

| Action | UI 의미 |
| --- | --- |
| `download_config_file` | 구성 파일 다운로드 버튼 표시 |
| `prepare_cluster_config` | 클러스터 구성 준비 실행 |
| `deploy_storage_vm` | 스토리지센터 VM 배포 |
| `configure_storage_vm` | 스토리지센터 VM 구성 |
| `open_storage_center` | 스토리지센터 연결 |
| `configure_storage_cluster` | 스토리지 클러스터 구성 |
| `configure_hci_shared_file` | HCI 공유 파일 구성 |
| `configure_gfs_storage` | GFS 스토리지 구성 |
| `configure_local_storage` | 로컬 스토리지 구성 |
| `deploy_cloud_vm` | 클라우드센터 VM 배포 |
| `configure_cloud_vm` | 클라우드센터 VM 구성 |
| `configure_cloud_cluster` | 클라우드센터 PCS 클러스터 구성 |
| `configure_cloud_resource` | 클라우드센터 PCS resource 구성 |
| `configure_monitoring` | 모니터링센터 구성 |
| `open_cloud_center` | 클라우드센터 연결 |
| `open_monitoring_center` | 모니터링센터 연결 |
| `run_security_patch` | 보안 패치 실행 |

`raw`는 기존 화면에서 사용하던 `sessionStorage` 계열 값을 서버에서 계산한 스냅샷이며, `os_type`에 필요한 key만 반환한다. key가 없으면 해당 `os_type`에서 사용하지 않는 값이고, `UNKNOWN`은 해당 `os_type`에서 필요한 값이지만 조회하지 못했거나 아직 판단하지 못한 상태이다. `cc_status`는 `ablestack-hci`, `ablestack-hci-filesystem`, `ablestack-vm`에서 CloudCenter PCS/resource 상태로 사용하고, `ablestack-standalone`에서는 반환하지 않는다. `HEALTH_ERR1`은 CloudCenter PCS 클러스터 미구성, `HEALTH_ERR2`는 CloudCenter resource 미구성 또는 비정상 상태를 의미한다. `cc_status` 조회는 `pcsCluster` 기준으로 실행 가능한 PCS 노드를 선택하고, 필요하면 해당 노드의 `/cube/pcs/control`로 위임한다.

`stage`가 `ready`여도 운영 상태 경고가 있으면 `severity`는 `warning`이고 `warnings`에 상세 key가 들어간다. UI는 `stage=ready`만으로 무조건 성공 ribbon을 표시하지 말고 `severity`와 `warnings`를 함께 확인해야 한다.

UI는 `stage`, `message_key`, `available_actions`를 기준으로 화면 상태를 매핑하고, 상세 카드에는 기존 `/cube/scvm/status`, `/cube/ccvm/status`, `/cube/gluecluster/status` 같은 API를 계속 사용할 수 있다.

### `POST /cube/deploy/run`

기존 개별 API를 유지한 상태에서 설치/배포 단계를 한 번의 job으로 순차 실행한다. 이 API는 HTTP 요청을 오래 붙잡지 않고 `202 Accepted`와 `job_id`를 반환하며, 실제 작업은 서버 내부 goroutine에서 진행한다. 진행 상태는 `/cube/deploy/jobs/{job_id}`로 조회한다.

최초 라이선스가 전혀 없는 신규 장비에서는 운영 API가 차단되므로, 먼저 마스터 노드에 `/cube/license`로 라이선스를 등록한 뒤 Bearer token을 발급받고 `/cube/deploy/run` 또는 `/cube/license/apply`를 실행한다. `/cube/deploy/run`의 `license_apply` 단계는 마스터에 등록된 현재 라이선스 파일을 전체 ablecube host로 배포하거나, 요청 body의 `license_content`/`licenses` 값을 사용한다. 단독 Cloud VM 구성은 CCVM 시작 후 `/cube/license/apply`의 `roles:["ccvm"]`, `wait_for_ready:true`로 라이선스를 등록하고, 별도의 클라우드센터 구성 단계에서 `/cube/ccvm/bootstrap`을 실행한다.

기본 실행 순서:

| Step | 설명 | 주요 입력 |
| --- | --- | --- |
| `license_apply` | 마스터 라이선스 또는 요청 라이선스를 `hosts[].ablecube` 전체에 등록 | `license_content`, `licenses`, `license_filename` |
| `cluster_apply` | 기존 `/cube/cluster/apply`와 같은 검증/대상 계산으로 클러스터 구성 적용 | `cluster` |
| `scvm_prepare` | HCI/HCI-FS에서 host별 SCVM cloud-init, XML, lifecycle setup 실행 후 `/api/v1/health` 확인 | `scvm_by_host` |
| `scvm_bootstrap` | 대표 SCVM 1대의 `/root/bootstrap.sh` 실행, SCVM API health 확인, 라이선스 자동 등록, license status 확인 | `run_bootstrap_script`, `license_content`, `licenses`, `license_filename` |
| `storage_prepare` | VM/HCI-FS 등에서 GFS/PCS 스토리지 준비 실행 | `gfs` |
| `local_prepare` | Standalone에서 로컬 디스크 준비 실행 | `local` |
| `ccvm_prepare` | CCVM cloud-init, XML, lifecycle setup 실행 후 `/api/v1/health` 확인 | `ccvm_cloudinit`, `ccvm_xml`, `ccvm_lifecycle` |
| `ccvm_bootstrap` | 올인원에서는 CCVM 라이선스 등록 후 `/root/bootstrap.sh` 실행, CCVM API health와 license status 확인 | `run_bootstrap_script`, `license_content`, `licenses`, `license_filename` |
| `system_profile` | 성공한 step 기준으로 `systemProfile` 플래그를 전체 host에 반영 | `update_system_profile` |

`system_profile` 단계는 성공한 step만 반영한다. `license_apply` 성공 시 `license.status=true`와 복호화된 라이선스 `oem` 기반 `license.type`을 적용하고, `scvm_bootstrap` 성공 시 `bootstrap.scvm=true`, `storage_prepare` 성공 시 VM/HCI-FS의 `bootstrap.gfs_configure=true`, `local_prepare` 성공 시 Standalone의 `bootstrap.local_configure=true`, `ccvm_bootstrap` 성공 시 `bootstrap.ccvm=true`를 적용한다. 모니터링 구성 완료를 뜻하는 `bootstrap.wall`은 별도 모니터링 구성 API/절차에서 반영한다.

`/api/v1/health`와 `/health`는 라이선스 등록 전에도 호출 가능한 public health check이다. SCVM/CCVM template에는 `ablestack-api` RPM과 service enable 상태가 포함되어 있어야 하며, VM 안의 `cluster.json`은 `/etc/ablestack/properties/cluster.json` 경로에 있어야 한다.

cloud-init은 VM 내부에 `/root/bootstrap.sh`를 전달만 하고 실행하지 않는다. 실제 bootstrap 스크립트 실행은 host의 `/cube/scvm/bootstrap`, `/cube/ccvm/bootstrap` API가 qemu-guest-agent로 수행한다. SCVM bootstrap 스크립트는 Ceph bootstrap과 전체 SCVM 후속 설정을 포함하므로 기본적으로 대표 SCVM 1대에서만 실행하고, 이후 라이선스 등록/status 확인은 전체 SCVM을 대상으로 수행한다. 스크립트 실행 후 라이선스만 재시도할 때는 직접 bootstrap API 요청 body에 `{"run_script":false}`를 전달하고, `/cube/deploy/run`에서는 `{"run_bootstrap_script":false}`를 전달한다.

job 정보는 현재 프로세스 메모리에만 보관한다. API 서버가 재시작되면 `/cube/deploy/jobs` 이력은 초기화된다. 요청에 포함된 라이선스 원문은 job 응답에 저장하지 않고, target별 처리 결과만 남긴다.

전체 실행 예:

```bash
curl -X POST http://<master-ablecube-ip>:18090/api/v1/cube/deploy/run \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "mode": "all",
    "cluster": {
      "action": "insert",
      "option": "add",
      "type": "ablestack-hci",
      "ccvm": { "ip": "10.10.12.10" },
      "mngtNic": { "cidr": "16", "gw": "10.10.0.1", "dns": "8.8.8.8" },
      "external_timeserver": "time.google.com",
      "storage_network": "false",
      "pcs_cluster_list": ["10.10.12.1", "10.10.12.2", "10.10.12.3"],
      "hosts": [
        {
          "index": "1",
          "hostname": "ablecube12-1",
          "ablecube": "10.10.12.1",
          "scvmMngt": "10.10.12.11",
          "ablecubePn": "100.100.12.1",
          "scvm": "100.100.12.11",
          "scvmCn": "100.200.12.11"
        }
      ]
    },
    "scvm_by_host": {
      "ablecube12-1": {
        "cpu": 8,
        "memory": 32,
        "disk_type": "disk_passthrough",
        "disk_passthrough_list": ["/dev/disk/by-id/wwn-0x1111"],
        "management_network_bridge": "br-mngt",
        "storage_traffic_network_type": "bridge",
        "server_network_bridge": "br-storage",
        "replication_network_bridge": "br-repl"
      }
    },
    "ccvm_xml": {
      "cpu": 8,
      "memory": 32,
      "management_network_bridge": "br-mngt"
    }
  }'
```

부분 실행 예:

```bash
curl -X POST http://<master-ablecube-ip>:18090/api/v1/cube/deploy/run \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "mode": "partial",
    "only": ["license_apply", "cluster_apply"],
    "cluster": {
      "action": "insert",
      "option": "add",
      "type": "ablestack-vm",
      "ccvm": { "ip": "10.10.31.10" },
      "mngtNic": { "cidr": "16", "gw": "10.10.0.1", "dns": "8.8.8.8" },
      "storage_network": "false",
      "pcs_cluster_list": ["10.10.31.1"],
      "hosts": [
        { "index": "1", "hostname": "ablecube31-1", "ablecube": "10.10.31.1" }
      ]
    }
  }'
```

대표 응답:

```json
{
  "code": 202,
  "job_id": "018f9c39-7ca9-7f4e-9a04-f6373d8f7e2b",
  "status": "queued",
  "message": "deploy job started",
  "steps": [
    { "name": "license_apply", "status": "pending" },
    { "name": "cluster_apply", "status": "pending" }
  ]
}
```

타입별 권장 요청 예시는 [올인원 배포 실행 가이드](deploy-run-guide.md)를 참고한다.

### `GET /cube/deploy/jobs`

최근 올인원 배포 job 목록을 최신순으로 반환한다.

```bash
curl -sS http://<master-ablecube-ip>:18090/api/v1/cube/deploy/jobs \
  -H "Authorization: Bearer <access_token>"
```

### `GET /cube/deploy/jobs/{job_id}`

특정 job의 현재 상태, 현재 실행 step, step별 시작/종료 시각, 결과 payload를 반환한다.

```bash
curl -sS http://<master-ablecube-ip>:18090/api/v1/cube/deploy/jobs/<job_id> \
  -H "Authorization: Bearer <access_token>"
```

### `cluster.json` 구조

현재 `clusterConfig.ccvm`에는 CCVM IP만 저장한다. 관리망 CIDR, gateway, DNS는 `mngtNic`로 분리한다. root의 `security.internal_token`은 노드 간 내부 API 통신 token이며, 클러스터 구성 준비 단계에서 없으면 API가 생성하고 있으면 입력 값을 그대로 사용한다.

```json
{
  "clusterConfig": {
    "type": "ablestack-vm",
    "backup_path": "/mnt/glue-gfs/backup/ccvm",
    "ccvm": {
      "ip": "10.10.31.10"
    },
    "mngtNic": {
      "cidr": "16",
      "gw": "10.10.0.1",
      "dns": "8.8.8.8"
    },
    "pcsCluster": {
      "hostname1": "10.10.31.1",
      "hostname2": "10.10.31.2",
      "hostname3": "10.10.31.3",
      "hostname4": "10.10.31.4"
    },
    "hosts": [
      {
        "index": "1",
        "hostname": "ablecube31-1",
        "ablecube": "10.10.31.1"
      }
    ],
    "external_timeserver": "time.google.com",
    "storage_network": "false"
  },
  "security": {
    "internal_token": "<cluster-internal-token>"
  }
}
```

HCI/HCI-filesystem 계열은 SCVM 관련 IP가 필요하므로 `hosts`에 아래 필드가 추가된다.

| 필드 | 의미 |
| --- | --- |
| `index` | 노드 순번. SCVM hostname 생성 시 `scvm1`, `scvm2`처럼 사용 |
| `hostname` | ablecube hostname |
| `ablecube` | 관리망 ablecube IP |
| `scvmMngt` | SCVM 관리망 IP |
| `ablecubePn` | ablecube public/storage network IP. CCVM XML/cloud-init fan-out 대상 |
| `scvm` | SCVM public/storage network IP |
| `scvmCn` | SCVM cluster network IP |

`ablestack-vm`에서는 빈 SCVM 필드를 응답에 내보내지 않는다.

### `POST /cube/cluster/apply`

오케스트레이터 API다. 요청을 받은 노드가 대상 host를 계산하고 각 노드의 `/cube/cluster/apply-local`을 호출한다. `insert` 적용이 성공하면 각 노드에서 `/etc/chrony.conf` 생성과 `chronyd` 재시작까지 자동으로 함께 처리한다.

지원 action:

| Action | 설명 | 주요 필수값 |
| --- | --- | --- |
| `insert` | `cluster.json` 반영, `/etc/hosts` 재구성, 시간 서버 설정 적용 | `type`, `ccvm`, `hosts`, `pcs_cluster_list`(`standalone` 제외), `storage_network` |
| `remove` | 지정 hostname 제거 | `remove_hostname` |
| `reset` | `cluster.json`을 기본값으로 초기화 | 없음 |
| `check` | 대상 ablecube API health 확인 | `hosts`, `type`, `ccvm` 또는 기존 `cluster.json` |

`pcs_cluster_list`는 `clusterConfig.pcsCluster.hostname1...hostname16`으로 저장된다. `ablestack-vm`은 1~16개를 허용하고, `ablestack-hci`, `ablestack-hci-filesystem`은 3~16개를 허용한다. `ablestack-standalone`은 PCS 대상이 없으므로 필수가 아니다. HCI 계열에서 3대 초과 설치를 하더라도 전체 호스트를 자동으로 PCS에 넣지 말고, Ceph MON을 담당할 노드만 `pcs_cluster_list`에 넣는 것을 기본값으로 사용한다.

`insert` 예:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/cluster/apply \
  -H "Content-Type: application/json" \
  -d '{
    "action": "insert",
    "option": "add",
    "type": "ablestack-hci",
    "ccvm": {
      "ip": "10.10.12.10"
    },
    "mngtNic": {
      "cidr": "16",
      "gw": "10.10.0.1",
      "dns": "8.8.8.8"
    },
    "external_timeserver": "time.google.com",
    "storage_network": "false",
    "security": {
      "internal_token": "<cluster-internal-token>"
    },
    "pcs_cluster_list": ["10.10.12.1", "10.10.12.2", "10.10.12.3"],
    "hosts": [
      {
        "index": "1",
        "hostname": "ablecube12-1",
        "ablecube": "10.10.12.1",
        "scvmMngt": "10.10.12.11",
        "ablecubePn": "100.100.12.1",
        "scvm": "100.100.12.11",
        "scvmCn": "100.200.12.11"
      }
    ]
  }'
```

`remove` 예:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/cluster/apply \
  -H "Content-Type: application/json" \
  -d '{"action":"remove","remove_hostname":"ablecube12-3"}'
```

`reset` 예:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/cluster/apply \
  -H "Content-Type: application/json" \
  -d '{"action":"reset"}'
```

`check` 예:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/cluster/apply \
  -H "Content-Type: application/json" \
  -d '{"action":"check","option":"all"}'
```

### `POST /cube/cluster/apply-local`

내부 실행 API다. `/cube/cluster/apply`가 노드별로 호출하는 실제 작업 API이며 직접 호출은 권장하지 않는다.

## System Profile API

### `GET /cube/system/config`

`cluster.json`의 `systemProfile`을 반환한다.

```bash
curl -sS http://<ablecube-ip>:18090/api/v1/cube/system/config
```

### `POST /cube/system/config`

지원 action:

| Action | 설명 |
| --- | --- |
| `status` | 전체 또는 특정 `depth1/depth2` 조회 |
| `update` | 특정 `depth1/depth2` 값 수정 |
| `allUpdate` | 클러스터 타입에 맞는 bootstrap 완료 상태 일괄 반영 |
| `reset` | systemProfile 초기화 |

조회:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/system/config \
  -H "Content-Type: application/json" \
  -d '{"action":"status","depth1":"bootstrap","depth2":"scvm"}'
```

전체 fan-out 수정:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/system/config \
  -H "Content-Type: application/json" \
  -d '{"action":"update","depth1":"bootstrap","depth2":"wall","value":"true","option":"all"}'
```

## Host, Disk, NIC API

### `GET /cube/hosts`

`/etc/hosts`를 읽고 localhost, management/public/client network, role 기준으로 정리해 반환한다.

```bash
curl -sS http://<ablecube-ip>:18090/api/v1/cube/hosts
```

### `GET /cube/disk`

지원 query:

| Query | 설명 |
| --- | --- |
| `action=list` | 일반 디스크 목록. RAID controller 포함 |
| `action=gfs` | GFS용 multipath/single path 목록. RAID controller 제외 |
| `action=rbd` | RBD map 디스크 목록 |
| `action=detail` | GFS와 유사한 상세 분류 |
| `view=tree` | 기본 tree 응답 |
| `view=flat` | 평탄화된 장치 목록 |
| `view=list` | list view |

```bash
curl -sS "http://<ablecube-ip>:18090/api/v1/cube/disk?action=list"
curl -sS "http://<ablecube-ip>:18090/api/v1/cube/disk?action=gfs"
curl -sS "http://<ablecube-ip>:18090/api/v1/cube/disk?action=detail&view=flat"
```

### `POST /cube/multipath/sync`

`cluster.json`의 `hosts[].ablecube` 대상 API로 fan-out하여 multipath 관련 작업을 수행한다. 기존 `multipath_sync.sh`의 SSH/SCP 흐름을 사용하지 않고, 내부 API 호출과 HTTP payload로 `/etc/multipath/bindings`, `/etc/multipath/wwids`를 전달한다.

지원 action:

| Action | 설명 |
| --- | --- |
| `sync` | 각 host에서 SCSI rescan, `mpathconf --enable`, `multipathd` enable/restart, bindings/wwids 동기화 수행 |
| `rescan` | 각 host에서 `/sys/class/scsi_host/*/scan`만 실행 |

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/multipath/sync \
  -H "Content-Type: application/json" \
  -d '{"action":"sync"}'

curl -X POST http://<ablecube-ip>:18090/api/v1/cube/multipath/sync \
  -H "Content-Type: application/json" \
  -d '{"action":"rescan"}'
```

### `GET /cube/nics`

지원 query:

| Query | 설명 |
| --- | --- |
| `action=list` | ethernet, bridge, bond 등 기본 목록 |
| `action=detail` | MAC, IPv4, IPv6, members, speed, bond option 등 상세 목록 |

```bash
curl -sS "http://<ablecube-ip>:18090/api/v1/cube/nics?action=list"
curl -sS "http://<ablecube-ip>:18090/api/v1/cube/nics?action=detail"
```

## URL API

### `GET /cube/url`

Cloud Center, Wall Center, Storage Center 접속 URL을 반환한다.

```bash
curl -sS "http://<ablecube-ip>:18090/api/v1/cube/url?option=cloudCenter"
curl -sS "http://<ablecube-ip>:18090/api/v1/cube/url?option=wallCenter"
curl -sS "http://<ablecube-ip>:18090/api/v1/cube/url?option=storageCenter"
```

## CloudInit API

cloud-init API는 CCVM/SCVM 부팅에 필요한 ISO를 생성한다. 공통으로 `/etc/hosts`, `/root/.ssh/id_rsa`, `/root/.ssh/id_rsa.pub`를 읽어 ISO에 포함한다. 파일 경로, ISO 경로, 관리망 IP처럼 서버가 계산할 수 있는 값은 POST body로 받지 않고 `cluster.json`과 현재 호스트 정보를 기준으로 결정한다.

### `POST /cube/cloudinit/ccvm/generate`

`cluster.json` 기반으로 CCVM cloud-init ISO를 생성한다.

고정/자동 입력:

| 항목 | 값 |
| --- | --- |
| hostname | `ccvm` |
| ISO path | `/var/lib/libvirt/images/ccvm-cloudinit.iso` |
| mgmt NIC | `enp0s20` |
| mgmt IP | `clusterConfig.ccvm.ip` |
| mgmt prefix/gw/dns | `clusterConfig.mngtNic.cidr/gw/dns` |
| 복사 대상 | `clusterConfig.hosts[].ablecubePn` |

서비스 네트워크가 없으면 빈 body로 호출할 수 있다.

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/cloudinit/ccvm/generate \
  -H "Content-Type: application/json" \
  -d '{}'
```

서비스 네트워크가 있으면 `sn_*` 값을 전달한다.

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/cloudinit/ccvm/generate \
  -H "Content-Type: application/json" \
  -d '{
    "sn_nic": "enp0s21",
    "sn_ip": "172.16.31.10",
    "sn_prefix": 24,
    "sn_gw": "172.16.31.1",
    "sn_dns": "8.8.8.8"
  }'
```

### `POST /cube/cloudinit/scvm/generate`

현재 노드가 `cluster.json`의 어느 `hosts[]` 항목인지 hostname/IP로 식별하고 SCVM cloud-init ISO를 생성한다.

고정/자동 입력:

| 항목 | 값 |
| --- | --- |
| hostname | `scvm` + `hosts[].index` |
| ISO path | `/var/lib/libvirt/images/scvm-cloudinit.iso` |
| mgmt NIC | `enp0s20` |
| mgmt IP | 현재 노드 `hosts[].scvmMngt` |
| PN NIC/IP | `enp0s21`, 현재 노드 `hosts[].scvm` |
| CN NIC/IP | `enp0s22`, 현재 노드 `hosts[].scvmCn` |
| prefix/gw/dns | `clusterConfig.mngtNic` |

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/cloudinit/scvm/generate
```

## CCVM API

### `GET /cube/ccvm/status`

CCVM 상태를 조회한다. libvirt/qemu-guest-agent 정보를 함께 사용한다.

```bash
curl -sS http://<ablecube-ip>:18090/api/v1/cube/ccvm/status
```

### `POST /cube/ccvm/xml`

Cloud Center VM libvirt XML을 생성한다. XML 템플릿은 `/etc/ablestack/xml-template/ccvm-xml-template.xml`을 사용하고, 생성된 XML은 `/etc/ablestack/vmconfig/ccvm/ccvm.xml`에 저장된다.

cluster type별 disk 처리:

| Type | disk XML |
| --- | --- |
| `ablestack-hci` | RBD `rbd/ccvm`, Ceph secret UUID `11111111-1111-1111-1111-111111111111` |
| `ablestack-hci-filesystem` | HCI와 동일 |
| `ablestack-vm` | `gfs_mount_point/ccvm.qcow2` 파일 disk. `gfs_mount_point` 필수 |
| `ablestack-standalone` | `/mnt/glue/ccvm.qcow2` |
| 기타/기본 | `/var/lib/libvirt/images/ablestack-template.qcow2` |

배포 흐름:

| 단계 | 설명 |
| --- | --- |
| HCI secret | HCI/HCI-filesystem일 때만 `hosts[].ablecube` 대상에 secret 생성 API fan-out |
| XML 생성 | 요청 받은 노드에서 템플릿을 렌더링 |
| XML 설치 | HCI 계열과 스토리지 전용망을 사용하는 VM은 `hosts[].ablecubePn`, 스토리지 전용망을 사용하지 않는 VM은 `hosts[].ablecube` 대상의 `/etc/ablestack/vmconfig/ccvm/ccvm.xml`로 fan-out |
| 서비스 네트워크 | `service_network_bridge`가 있으면 두 번째 NIC XML 추가 |

`ablestack-vm` 예:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/ccvm/xml \
  -H "Content-Type: application/json" \
  -d '{
    "cpu": 4,
    "memory": 16,
    "gfs_mount_point": "/mnt/glue-gfs",
    "management_network_bridge": "br0",
    "service_network_bridge": "br1"
  }'
```

HCI 예:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/ccvm/xml \
  -H "Content-Type: application/json" \
  -d '{
    "cpu": 4,
    "memory": 16,
    "management_network_bridge": "br0"
  }'
```

### `POST /cube/ccvm/create`

CCVM XML과 제품 타입별 backing image를 준비하고 PCS `cloudcenter_res`를 비활성 상태로 생성한다. HCI 계열은 RBD를 사용하고, ABLESTACK-VM은 `/mnt/glue-gfs`의 공유 이미지와 XML을 사용한다. 생성 후 `/cube/ccvm/lifecycle`의 `start` action으로 CCVM을 시작한다.

요청 body는 `/cube/ccvm/xml`과 동일하다.

### `POST /cube/ccvm/lifecycle`

Cloud Center VM lifecycle 작업을 수행한다.

지원 action:

| Action | 설명 |
| --- | --- |
| `initialize` | 재배포 전에 기존 `cloudcenter_res`가 있으면 제거하고 `/mnt/glue-gfs/ccvm*`만 정리. 리소스나 PCS가 없으면 정상 통과 |
| `setup` | HCI 계열은 RBD CCVM 이미지를, ABLESTACK-VM은 `/mnt/glue-gfs`의 공유 CCVM 이미지/XML을 준비한 뒤 PCS 리소스를 생성하고 시작 |
| `reset` | cluster type에 따라 CCVM/PCS/GFS/local disk 초기화 |
| `copy` | CCVM 관련 파일 복사 |
| `start` | CCVM 시작 |
| `stop` | CCVM 종료. `destroy=true`면 강제 destroy 사용 |
| `restart` | CCVM 재시작 |
| `delete` | CCVM 삭제. `purge=true`면 이미지까지 삭제 |

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/ccvm/lifecycle \
  -H "Content-Type: application/json" \
  -d '{"action":"start"}'
```

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/ccvm/lifecycle \
  -H "Content-Type: application/json" \
  -d '{"action":"delete","purge":true}'
```

### `POST /cube/ccvm/edit`

CCVM이 정지 중일 때 CPU/메모리 값을 XML에 반영한다.

cluster type별 동작:

| Type | 동작 |
| --- | --- |
| `ablestack-vm` | `/mnt/glue-gfs/ccvm.xml` 공유 파일 수정 |
| `ablestack-standalone` | `/mnt/glue/ccvm.xml` 로컬 파일 수정 |
| `ablestack-hci` | 각 ablecube의 `/etc/ablestack/vmconfig/ccvm/ccvm.xml`로 fan-out |
| `ablestack-hci-filesystem` | HCI와 동일하게 각 host 로컬 XML 수정 |

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/ccvm/edit \
  -H "Content-Type: application/json" \
  -d '{"cpu":"16","memory":"32"}'
```

### `POST /cube/ccvm/secondary/resize`

CCVM secondary 용량을 1~500 GiB 범위에서 추가한다.

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/ccvm/secondary/resize \
  -H "Content-Type: application/json" \
  -d '{"add_size":100}'
```

### `POST /cube/ccvm/service/control`

CCVM의 서비스 제어 요청을 CCVM 노드로 전달한다.

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/ccvm/service/control \
  -H "Content-Type: application/json" \
  -d '{"action":"restart","service_name":"mold"}'
```

지원 action: `start`, `restart`, `stop`, `status`

### `POST /cube/ccvm/monitoring/config`

`action=configure`는 Host API가 `cluster.json`의 CCVM `/health`를 먼저 확인하고 CCVM API로 요청을 전달한 뒤, CCVM에 설치된 기존 Wall Python 도구를 다음 순서로 실행한다. Python 프로세스에는 `ABLESTACK_CLUSTER_JSON=/etc/ablestack/properties/cluster.json`을 전달한다. `--ccvm`, `--cube`, `--scvm` 값은 요청 body가 아니라 해당 `cluster.json`의 `ccvm.ip`, `hosts[].ablecube`, `hosts[].scvmMngt`에서만 생성한다.

1. `host_ping_test.py -hns <ccvm> <cube...> <scvm...>`
2. `config_netdive.py config --ccvm <ccvm> --cube <cube...>`
3. `start_services.py stop --service <wall services...>`
4. `config_wall.py config --ccvm <ccvm> --cube <cube...> [--scvm <scvm...>]`
5. `start_services.py start --service <wall services...>`
6. SMTP 선택 시 `config_smtp.py config --host <host:port> --user <user> --password <password>`

`config_wall.py config`가 기존 방식대로 Grafana/Prometheus 설정과 `config_loki.py` 실행을 담당한다. API는 각 스크립트 stdout의 JSON `code`를 검사하고, 마지막에 Prometheus, Node/Process/Blackbox exporter, Grafana, Netdive Analyzer, Loki, Promtail이 모두 active/enabled인지 최대 90초 확인한다. 모든 단계와 전체 호스트의 `bootstrap.wall=true` 반영이 성공해야 HTTP 200을 반환한다.

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/ccvm/monitoring/config \
  -H "Content-Type: application/json" \
  -d '{
    "action":"configure",
    "ccvm":["10.10.31.10"],
    "cube":["10.10.31.1","10.10.31.2"],
    "scvm":["10.10.31.11","10.10.31.12"],
    "smtp":{"enabled":false}
  }'
```

클라우드 클러스터 상태 카드의 수집 정보 재갱신은 `{"action":"update"}`를 사용한다. 이 경우 `cluster.json` 주소를 기본값으로 `config_wall.py update --ccvm <ccvm> --cube <cube...> [--scvm <scvm...>]`를 실행하고 실제 서비스 상태를 다시 검증한다.

## CCVM Snapshot API

### `POST /cube/ccvm/snap`

HCI/HCI-filesystem 환경에서 RBD 기반 CCVM snapshot을 관리한다.

| Action | 설명 |
| --- | --- |
| `list` | snapshot 목록 |
| `backup` | snapshot 생성. `snap_name` 생략 가능 |
| `rollback` | 지정 snapshot으로 rollback. CCVM 정지 상태 필요 |

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/ccvm/snap \
  -H "Content-Type: application/json" \
  -d '{"action":"list"}'
```

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/ccvm/snap \
  -H "Content-Type: application/json" \
  -d '{"action":"rollback","snap_name":"auto-2026-04-30"}'
```

자동 snapshot backup은 `systemProfile.bootstrap.ccvm=true` 조건에서 API 서버 내부 스케줄러가 수행한다. 실제 백업은 PCS current DC 노드에서만 실행되도록 구성되어 있다.

## CCVM File Backup/Restore API

### `POST /cube/ccvm/backup`

VM/Standalone 계열에서 virsh backup 기반 CCVM 파일 백업과 스케줄을 관리한다.

| Action | 설명 |
| --- | --- |
| `backup` | 즉시 백업 생성 |
| `status` | 백업 작업 상태 |
| `list` | 백업 파일 목록 |
| `overview` | 스케줄/삭제 정책/파일 목록 요약 |
| `schedule` | 정기 백업 스케줄 설정 |
| `unschedule` | 정기 백업 비활성화 |
| `schedule-delete` | 오래된 백업 삭제 스케줄 설정 |
| `unschedule-delete` | 삭제 스케줄 비활성화 |

즉시 백업:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/ccvm/backup \
  -H "Content-Type: application/json" \
  -d '{"action":"backup","target_dir":"/mnt/glue-gfs/backup/ccvm"}'
```

일간 스케줄:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/ccvm/backup \
  -H "Content-Type: application/json" \
  -d '{"action":"schedule","repeat":"daily","time":"01:00"}'
```

### `POST /cube/ccvm/restore`

백업 파일로 CCVM 디스크를 복구한다.

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/ccvm/restore \
  -H "Content-Type: application/json" \
  -d '{"target_file":"ccvm.qcow2-20260430_010000"}'
```

## SCVM API

### `GET /cube/scvm/status`

Storage Center VM 상태를 조회한다.

```bash
curl -sS http://<ablecube-ip>:18090/api/v1/cube/scvm/status
```

### `POST /cube/scvm/xml`

Storage Center VM libvirt XML과 hugepage 설정을 생성한다. XML 템플릿은 `/etc/ablestack/xml-template/scvm-xml-template.xml`을 사용하고, 생성 결과는 `/etc/ablestack/vmconfig/scvm/scvm.xml`이다.

생성 시 함께 처리하는 파일:

| 대상 | 설명 |
| --- | --- |
| `/etc/security/limits.conf` | `limits-template.conf`의 `{memory}` 값을 `memory * 1024 * 1024`로 치환 |
| `/etc/sysctl.conf` | `sysctl-template.conf`의 `{memory}` 값을 `memory * 1024`로 치환 |
| `/etc/ablestack/vmconfig/scvm/scvm.xml` | SCVM libvirt XML |

disk type:

| 값 | 필요 필드 | 설명 |
| --- | --- | --- |
| `lun_passthrough` | `lun_passthrough_list` | block device를 SCSI LUN으로 XML에 추가 |
| `disk_passthrough` | `disk_passthrough_list` | block device를 SCSI disk로 XML에 추가 |
| `raid_passthrough` | `raid_passthrough_list` | RAID PCI address를 hostdev로 XML에 추가 |

storage traffic network type:

| 값 | 필요 필드 | 설명 |
| --- | --- | --- |
| `bridge` | `server_network_bridge`, `replication_network_bridge` | bridge interface 2개 생성 |
| `nic_passthrough` | `server_nic_passthrough`, `replication_nic_passthrough` | PCI NIC 2개 passthrough |
| `nic_passthrough_bonding` | 각 bonding list 2개씩 | server/replication 각각 2개 NIC passthrough |

bridge 예:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/scvm/xml \
  -H "Content-Type: application/json" \
  -d '{
    "cpu": 4,
    "memory": 16,
    "disk_type": "disk_passthrough",
    "disk_passthrough_list": ["/dev/disk/by-id/wwn-0x1234"],
    "management_network_bridge": "br0",
    "storage_traffic_network_type": "bridge",
    "server_network_bridge": "br1",
    "replication_network_bridge": "br2"
  }'
```

NIC passthrough 예:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/scvm/xml \
  -H "Content-Type: application/json" \
  -d '{
    "cpu": 4,
    "memory": 16,
    "disk_type": "raid_passthrough",
    "raid_passthrough_list": ["00:1f.2"],
    "management_network_bridge": "br0",
    "storage_traffic_network_type": "nic_passthrough",
    "server_nic_passthrough": "0000:03:00.0",
    "replication_nic_passthrough": "0000:04:00.0"
  }'
```

### `POST /cube/scvm/lifecycle`

Storage Center VM lifecycle 작업을 수행한다. 오케스트레이터가 `cluster.json`의 대상 정보를 보고 로컬 또는 원격 노드 API를 호출한다.

지원 action:

| Action | 설명 |
| --- | --- |
| `setup` | SCVM setup |
| `reset` | SCVM reset |
| `start` | SCVM 시작 |
| `stop` | SCVM 정지 |
| `delete` | SCVM 삭제 |
| `resource` | CPU/메모리 변경. `cpu` 또는 `memory` 필요 |

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/scvm/lifecycle \
  -H "Content-Type: application/json" \
  -d '{"action":"start","target":"10.10.31.2"}'
```

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/scvm/lifecycle \
  -H "Content-Type: application/json" \
  -d '{"action":"resource","cpu":4,"memory":16}'
```

## PCS API

### `POST /cube/pcs/control`

Cloud Center PCS cluster/resource를 제어한다.

지원 action:

| Action | 설명 | 주요 필드 |
| --- | --- | --- |
| `setup` | cluster.json 기준 PCS setup | 내부 설정 |
| `setup-cron` | 기존 Python CCVM snapshot cron 제거 | 내부 호환/정리 작업. 자동 snapshot은 API 내부 Go scheduler가 수행 |
| `config` | PCS cluster 구성 | `cluster`, `hosts` |
| `create` | CCVM resource 생성 | `resource`, `xml` |
| `enable` | CCVM resource enable | `resource` |
| `disable` | CCVM resource disable | `resource` |
| `move` | CCVM resource 이동 | `target` |
| `cleanup` | PCS cleanup | `resource` |
| `status` | PCS 상태 조회 | `resource` |
| `remove` | PCS resource 제거 | `resource` |
| `destroy` | PCS cluster 삭제 | 없음 |
| `stop` | PCS cluster 정지 | 없음 |
| `sync` | totem token 시간 설정 | `time` |
| `ccvm-status` | libvirt에 CCVM domain 생성 여부 확인 | 없음 |

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/pcs/control \
  -H "Content-Type: application/json" \
  -d '{"action":"status"}'
```

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/pcs/control \
  -H "Content-Type: application/json" \
  -d '{"action":"move","target":"ablecube31-2"}'
```

## Glue and GFS API

### `GET /cube/gluecluster/status`

Glue cluster 상세 상태를 조회한다.

```bash
curl -sS http://<ablecube-ip>:18090/api/v1/cube/gluecluster/status
```

### `POST /cube/gluecluster/update`

유지보수 모드를 설정하거나 해제한다.

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/gluecluster/update \
  -H "Content-Type: application/json" \
  -d '{"action":"set_noout"}'
```

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/gluecluster/update \
  -H "Content-Type: application/json" \
  -d '{"action":"unset_noout"}'
```

### `GET /cube/gfs/resource/status`

`pcs status xml` 기반으로 펜스 장치, `glue-dlm`/`glue-lvmlockd` 잠금 장치와
`glue-gfs_res` LVM 활성화, `glue-gfs` GFS2 Filesystem clone의 노드별 상태를 조회한다.
GFS2 마운트 리소스는 `val.resources.gfs_mount_resources`에 반환하며,
기존 `glue_gfs_resources` 필드도 호환 목적으로 같은 값을 반환한다.

```bash
curl -sS http://<ablecube-ip>:18090/api/v1/cube/gfs/resource/status
```

### `GET /cube/gfs/disk/status`

GFS2로 마운트된 디스크 목록과 multipath/single mode 정보를 조회한다. `blockdevices[]`에는 기존 `size`와 함께 `df -hP` 기준 `used`, `avail`, `use_percent`가 포함된다.

```bash
curl -sS http://<ablecube-ip>:18090/api/v1/cube/gfs/disk/status
```

## DB Dump API

### `POST /cube/db/dump`

CCVM의 `cloud` DB dump와 백업/삭제 스케줄을 관리한다.

| Action | 설명 | 주요 필드 |
| --- | --- | --- |
| `instantBackup` | 즉시 dump 생성 | `path` |
| `regularBackup` | 정기 백업 스케줄 등록 | `path`, `repeat`, `timeone`, `timetwo` |
| `deleteOldBackup` | 오래된 dump 삭제 스케줄 등록 | `path`, `repeat`, `timeone`, `delete` |
| `checkBackup` | 백업/삭제 스케줄 조회 | `checkOption` |
| `deactiveBackup` | 백업/삭제 스케줄 비활성화 | `checkOption` |

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/db/dump \
  -H "Content-Type: application/json" \
  -d '{"action":"instantBackup","path":"/home/db_backup"}'
```

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/db/dump \
  -H "Content-Type: application/json" \
  -d '{"action":"regularBackup","path":"/home/db_backup","repeat":"daily","timeone":"02:00"}'
```

`repeat` 값:

| 값 | 의미 |
| --- | --- |
| `no` | 1회성 at job |
| `hourly` | 매시간 |
| `daily` | 매일 |
| `weekly` | 매주. `timetwo`는 0~6 weekday |
| `monthly` | 매월. `timetwo`는 `월간격-일`, 예: `1-15` |

`checkOption` 값:

| 값 | 의미 |
| --- | --- |
| `r` | 백업 스케줄 |
| `d` | 삭제 스케줄 |

## License API

### `POST /cube/license`

라이선스를 조회하거나 등록한다. 이 API는 Bearer token 없이 호출할 수 있다. 활성 라이선스가 이미 등록된 상태에서 라이선스를 교체하려면 기존 Bearer token이 필요하다.
등록된 라이선스가 없거나 만료되면 다른 운영 API는 `active license required` 오류로 차단된다.

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/license \
  -H "Content-Type: application/json" \
  -d '{"action":"status"}'
```

등록은 파일 업로드 방식을 사용할 수 있다. `license_file` 대신 `file` 필드명도 허용한다.
업로드한 파일명이 저장 파일명으로 사용된다. Swagger에서는 `action`에 `register`를 입력하고, `license_file` 파일 선택에서 로컬 라이선스 파일을 선택한다.

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/license \
  -F "action=register" \
  -F "license_file=@./license.lic"
```

기존 JSON 방식도 호환된다.

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/license \
  -H "Content-Type: application/json" \
  -d '{"action":"register","license_content":"BASE64_CONTENT"}'
```

등록에 성공하면 라이선스 파일을 저장한 뒤 `cluster.json`의 `systemProfile.license.status`를 `true`로, `systemProfile.license.type`을 복호화된 라이선스의 `oem` 값으로 갱신한다. `cluster.json`이 아직 생성되지 않은 초기 상태에서는 라이선스 등록 자체를 막지 않고 파일 저장만 수행한다.

### `POST /cube/license/apply`

마스터 노드에서 cluster 대상 노드로 라이선스를 fan-out 등록한다. 기존 `/cube/license`는 요청을 받은 로컬 노드만 처리하고, `/cube/license/apply`는 role 기준으로 대상 IP를 계산한다. `roles`가 비어 있으면 기존 호환성을 위해 `cluster.json`의 `hosts[].ablecube` 대상만 순회한다.

기본 bootstrap 순서:

1. 마스터 노드에 `/cube/license`로 라이선스 등록
2. `/auth/login`으로 Bearer token 발급
3. `/cube/license/apply`로 전체 물리 host에 라이선스 배포
4. `/cube/cluster/apply` 또는 `/cube/deploy/run` 실행
5. SCVM 생성 후 `scvm_bootstrap` 단계 또는 `roles:["scvm"]` 재실행으로 SCVM 라이선스 후처리
6. CCVM 생성 후 `ccvm_bootstrap` 단계 또는 `roles:["ccvm"]` 재실행으로 CCVM 라이선스 후처리

요청 body 없이 호출하면 마스터 노드에 현재 등록된 라이선스 파일을 읽어 전체 물리 host에 등록한다.

```bash
curl -X POST http://<master-ablecube-ip>:18090/api/v1/cube/license/apply \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{}'
```

대상 role을 지정할 수 있다. `roles`는 `ablecube`, `scvm`, `ccvm`, `all`을 지원한다. 명시하지 않으면 `ablecube`만 적용한다.

```bash
curl -X POST http://<master-ablecube-ip>:18090/api/v1/cube/license/apply \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "action": "register",
    "roles": ["scvm"]
  }'
```

요청 body로 동일 라이선스를 직접 전달할 수도 있다.

```bash
curl -X POST http://<master-ablecube-ip>:18090/api/v1/cube/license/apply \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "action": "register",
    "license_content": "BASE64_CONTENT",
    "filename": "license.lic"
  }'
```

호스트별 라이선스가 다른 경우 `licenses`에 `hostname`, role 이름, target IP, `index`, `scvmN` 중 하나를 key로 지정한다. CCVM은 `ccvm`, SCVM은 `scvm1`, `scvm2` 같은 key를 사용할 수 있다.

```bash
curl -X POST http://<master-ablecube-ip>:18090/api/v1/cube/license/apply \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "action": "register",
    "filename": "license.lic",
    "licenses": {
      "ablecube12-1": "BASE64_CONTENT_HOST_1",
      "ablecube12-2": "BASE64_CONTENT_HOST_2",
      "ablecube12-3": "BASE64_CONTENT_HOST_3"
    }
  }'
```

특정 target만 지정할 수도 있다.

```bash
curl -X POST http://<master-ablecube-ip>:18090/api/v1/cube/license/apply \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "action": "register",
    "targets": ["10.10.12.1", "10.10.12.2"],
    "license_content": "BASE64_CONTENT"
  }'
```

응답 예:

```json
{
  "code": 200,
  "message": "license apply success",
  "results": [
    {
      "role": "ablecube",
      "hostname": "ablecube12-1",
      "target": "10.10.12.1",
      "code": 200,
      "message": "ok",
      "val": "라이센스가 성공적으로 등록되었습니다."
    }
  ]
}
```

원격 호스트에 기존 활성 라이선스가 있어도 내부 fan-out 요청에는 `X-Cube-Internal-Token`을 포함하므로 같은 클러스터 내부 호출로 교체할 수 있다. 사용자가 직접 `/cube/license`로 특정 노드 라이선스를 교체하는 경우에는 기존 정책대로 Bearer token이 필요하다.

## Version API

### `GET /version`

현재 노드의 OS/Kernel/Cockpit/Mold/ABLESTACK 패키지 버전을 조회한다. `os_version`은 `/etc/os-release`의 `PRETTY_NAME`, `kernel_version`은 `uname -r`, `cockpit_version`은 `cockpit-bridge --version`의 `Version` 값, `mold_version`은 `/usr/share/cloudstack-common/scripts/installer/cloudstack-help-text`의 `ACS_VERSION`을 사용한다.
`ablestack_package_versions`는 `aspkg -qa` 결과 중 `ablestack`을 포함하는 패키지 목록이다.
`cluster.json`의 `clusterConfig.type`이 `ablestack-hci` 또는 `ablestack-hci-filesystem`이면 `glue version` 결과를 `glue_version`으로 포함한다.

```bash
curl http://<ablecube-ip>:18090/api/v1/version \
  -H "Authorization: Bearer <access_token>"
```

응답 예시는 다음과 같다.

```json
{
  "os_version": "ABLESTACK 2026",
  "kernel_version": "5.14.0-503.35.1.el9_5.x86_64",
  "cockpit_version": "344",
  "mold_version": "4.20.0",
  "glue_version": "glue version 1.0.0",
  "ablestack_package_versions": [
    "ablestack-qemu-exec-tools-0.8.0-1.el9.el9.noarch",
    "ablestack_v2k-0.8.0-1.el9.el9.noarch",
    "ablestack_vm_ftctl-0.8.0-1.noarch",
    "ablestack_vm_hangctl-0.8.0-1.noarch"
  ],
  "debug": false
}
```

## Version Update API

### `POST /cube/version/update`

마운트된 ABLESTACK ISO에서 버전 정보를 조회하거나 ISO 내용을 `/opt/ABLESTACK_UPDATE`로 복사한 뒤 선택한 업데이트 스크립트를 실행한다. `update_type`은 생략하면 `all`로 처리한다.

지원 업데이트 방식:

| update_type | 실행 스크립트 | 설명 |
| --- | --- | --- |
| `all` | `update-all.sh` | 커널, Mold 등 전체 업데이트 |
| `mold` | `update-mold.sh` | Mold 업데이트. host와 CCVM 대상 후처리는 스크립트가 담당 |

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/version/update \
  -H "Content-Type: application/json" \
  -d '{"action":"info","mount_path":"/mnt/ablestack-iso","update_type":"all"}'
```

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/version/update \
  -H "Content-Type: application/json" \
  -d '{"action":"run","mount_path":"/mnt/ablestack-iso","update_type":"mold"}'
```

`mount_path`는 절대 경로여야 하며, 내부에 `ks/ablestack-ks.cfg`와 선택한 업데이트 스크립트가 있어야 한다. `run` 실행 시 기존 `/opt/ABLESTACK_UPDATE` 디렉터리는 삭제 후 다시 생성되며, 이 경로가 심볼릭 링크 또는 마운트 지점이면 실행하지 않는다.

`info` 응답은 화면 표시용으로 현재/대상 OS와 Mold 버전을 함께 반환한다. 대상 OS 버전은 `ks/ablestack-ks.cfg`의 `ABLESTACK_VERSION`을 사용한다. 대상 Mold 버전은 `AppStream/Packages/mold`의 `cloudstack-common`/`cloudstack-management`/`cloudstack-agent`/`cloudstack-ui`/`cloudstack-usage` RPM 파일명에서 우선 추출하며, 예시는 `4.21.0.0-Mold.Diplo.202606121525`처럼 버전명과 날짜까지만 반환한다. RPM 파일명에서 찾지 못하면 `ks/ablestack-ks.cfg`의 `MOLD_VERSION`, `ACS_VERSION`, `CLOUDSTACK_VERSION` 또는 ISO 내부 `cloudstack-help-text`의 `ACS_VERSION`을 사용한다.

`run`은 `/cube/deploy/status`가 `stage=ready`인 경우에만 실행된다. 화면에서 버튼을 숨기더라도 직접 API 호출을 막기 위한 서버 측 조건이다.

## Security Patch API

### `POST /cube/security/patch`

`cluster.json` 대상에 `security_patch.sh`를 로컬/SSH로 실행하거나 `security_patch.status` 값을 업데이트한다.

기본 실행:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/security/patch \
  -H "Content-Type: application/json" \
  -d '{"targets":["all"],"ssh_user":"root","ssh_port":22}'
```

SSH 포트 변경:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/security/patch \
  -H "Content-Type: application/json" \
  -d '{"port_change":true,"new_port":10022,"targets":["all"],"ssh_user":"root","ssh_port":22}'
```

Ceph SSH 설정 변경:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/security/patch \
  -H "Content-Type: application/json" \
  -d '{"ceph_ssh_change":true,"new_port":10022}'
```

추가 호스트용 실행:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/security/patch \
  -H "Content-Type: application/json" \
  -d '{"add_host":true,"new_port":10022,"ssh_user":"root","ssh_port":22}'
```

JSON status 업데이트:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/security/patch \
  -H "Content-Type: application/json" \
  -d '{"update_json_file":true,"local":false}'
```

## SSH Key API

### `POST /cube/ssh/key`

각 호스트의 `/root/.ssh/id_rsa`, `/root/.ssh/id_rsa.pub`, `/root/.ssh/authorized_keys`를 관리한다. 기본적으로 기존 파일은 덮어쓴다. 다운로드 파일은 AES-GCM으로 암호화된 단일 파일이며, 파일명은 랜덤 `.dat` 형식으로 내려간다. 최초 설치 흐름에서 2, 3번 호스트에 아직 `internal_token`이 없어도 업로드할 수 있도록 암호화는 `internal_token`에 의존하지 않는다.

키 생성:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/ssh/key \
  -H "Content-Type: application/json" \
  -d '{"action":"generate"}'
```

Windows PC로 암호화 파일 다운로드:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/ssh/key \
  -H "Content-Type: application/json" \
  -d '{"action":"download"}' \
  -OJ
```

다운로드한 암호화 파일 업로드:

```bash
curl -X POST http://<ablecube-ip>:18090/api/v1/cube/ssh/key \
  -F action=upload \
  -F file=@<downloaded-file>.dat
```

업로드 시 파일을 복호화해 `id_rsa`, `id_rsa.pub`를 반영하고, `/root/.ssh/authorized_keys`는 `id_rsa.pub` 내용으로 함께 갱신된다.

운영자가 별도 공용 암호화 secret을 쓰고 싶으면 모든 호스트의 API 서비스 환경 변수에 같은 `ABLESTACK_SSH_KEY_BUNDLE_SECRET` 값을 설정한다. 설정하지 않으면 제품 기본값으로 암호화/복호화한다.

## Utility APIs

### Error Log

```bash
curl -sS http://<ablecube-ip>:18090/api/v1/err
curl -X DELETE http://<ablecube-ip>:18090/api/v1/err
```

## Background Jobs

`cmd/apiserver/main.go`에서 `controller`에 등록되는 주기 작업이다. 컨트롤러는 30초 주기로 등록된 handler를 실행한다. 반복 성공은 기록하지 않고 실패, 상태 변화, 자동 백업 실행 결과만 `/var/log/ablestack/job.log`에 남긴다.

| Job | 등록 함수 | 설명 |
| --- | --- | --- |
| Hosts cache | `CubeHandler.UpdateHosts` | `/etc/hosts` 캐시 갱신 |
| Cluster config cache | `CubeHandler.UpdateClusterConfig` | `cluster.json` 캐시 갱신 |
| SSH known_hosts scan | `CubeHandler.AutoSSHKnownHostsScan` | 하루 1회 조건으로 대상 host key scan |
| CCVM snapshot backup | `CubeHandler.AutoCCVMSnapshotBackup` | CCVM snapshot 자동 백업 |
| CCVM file backup schedule | `CubeHandler.AutoCCVMFileBackupSchedule` | CCVM 파일 백업 스케줄러 |
| NIC cache | `CubeHandler.UpdateNICs` | NIC 목록 갱신 |
| Disk cache | `CubeHandler.UpdateDisk` | Disk 목록 갱신 |

## 구현상 주의 사항

| 항목 | 현재 코드 기준 |
| --- | --- |
| SCVM lifecycle 경로 | `/cube/scvm/lifecycle` |
| System Profile 조회 | `/cube/system/config` |
| Cluster Config 조회 | `/cube/cluster/config` |
| CCVM secondary resize | `/cube/ccvm/secondary/resize` |
| CCVM/SCVM XML 생성물 | `/etc/ablestack/vmconfig/ccvm/ccvm.xml`, `/etc/ablestack/vmconfig/scvm/scvm.xml` |
| `ccvm` network config | `clusterConfig.ccvm.ip`만 사용. CIDR/GW/DNS는 `clusterConfig.mngtNic` 사용 |
| optional runtime command | `pcs`, `virsh`, `ceph`, `rbd`, `radosgw-admin`, `podman`, `smbpasswd`, `pdbedit`, `realm`, `genisoimage` 등은 RPM 설치를 막지 않고 해당 API 실행 시점에 필요 |

## 운영 전 보완 예정

운영 배포 전에는 `docs/post-development-review-notes.md`의 항목을 기준으로 API 인증, 내부 fan-out 토큰, CORS 제한, secret 제거, 백그라운드 작업 중복 실행 방지 등을 반영한다.
