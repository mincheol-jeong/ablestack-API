# 올인원 배포 실행 가이드

이 문서는 마스터 ablecube 호스트에서 라이선스 배포, 클러스터 구성 준비, SCVM 준비, 스토리지 준비, CCVM 준비를 한 번의 job으로 실행하는 방법을 설명한다. 기존 개별 API는 그대로 유지되며, 올인원 API는 기존 API 위에서 순차 실행하는 오케스트레이터 역할만 한다.

## API 개요

기준 URL:

```text
http://<master-ablecube-ip>:18090/api/v1
```

| Method | Endpoint | 용도 |
| --- | --- | --- |
| `POST` | `/cube/license` | 현재 요청을 받은 노드의 로컬 라이선스 등록/조회 |
| `POST` | `/cube/license/apply` | 마스터 기준 ablecube/SCVM/CCVM role별 라이선스 fan-out 등록 |
| `POST` | `/cube/scvm/bootstrap` | 대표 SCVM의 `/root/bootstrap.sh` 실행 후 SCVM API health, 라이선스 등록/status 확인 |
| `POST` | `/cube/ccvm/bootstrap` | 라이선스가 등록된 CCVM의 `/root/bootstrap.sh` 실행 후 API health와 라이선스 status 확인 |
| `POST` | `/cube/deploy/run` | 올인원 배포 job 시작 |
| `GET` | `/cube/deploy/jobs` | 최근 올인원 배포 job 목록 조회 |
| `GET` | `/cube/deploy/jobs/{job_id}` | 특정 올인원 배포 job 상세 조회 |

## 최초 실행 순서

신규 장비에는 활성 라이선스가 없으므로 `/cube/deploy/run` 같은 운영 API가 차단된다. 따라서 최초 설치에서는 아래 순서를 사용한다.

1. 마스터 노드에 로컬 라이선스를 등록한다.
2. `/auth/login`으로 Bearer token을 발급받는다.
3. `/cube/license/apply` 또는 `/cube/deploy/run`의 `license_apply` 단계로 전체 물리 host에 라이선스를 배포한다.
4. `/cube/deploy/run`으로 나머지 배포 단계를 실행한다. 이때 `scvm_prepare`/`ccvm_prepare` 뒤에 `scvm_bootstrap`/`ccvm_bootstrap`이 실행되어 VM 내부 `/root/bootstrap.sh` 실행, VM API health 확인, 라이선스 후처리를 자동 수행한다.
5. 필요하면 `/cube/scvm/bootstrap`, `/cube/ccvm/bootstrap` 또는 `only:["scvm_bootstrap"]`, `only:["ccvm_bootstrap"]`으로 VM bootstrap 단계만 재실행한다. 이미 `/root/bootstrap.sh`가 삭제된 뒤 라이선스 후처리만 재시도할 때는 `run_script:false` 또는 `run_bootstrap_script:false`를 함께 사용한다.
6. `/cube/deploy/jobs/{job_id}`로 진행 상태를 조회한다.
7. 실패한 step이 있으면 기존 개별 API로 수정/복구하거나 `only`로 특정 step만 재실행한다.

마스터 로컬 라이선스 등록:

```bash
curl -X POST http://<master-ablecube-ip>:18090/api/v1/cube/license \
  -F "action=register" \
  -F "license_file=@./license.lic"
```

토큰 발급:

```bash
curl -X POST http://<master-ablecube-ip>:18090/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"id":"root","password":"<linux-password>"}'
```

마스터의 현재 라이선스를 전체 물리 host에 배포:

```bash
curl -X POST http://<master-ablecube-ip>:18090/api/v1/cube/license/apply \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{}'
```

SCVM 생성 후 SCVM API 노드에 라이선스 후처리만 재실행:

```bash
curl -X POST http://<master-ablecube-ip>:18090/api/v1/cube/license/apply \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{"action":"register","roles":["scvm"]}'
```

CCVM 생성 후 CCVM API 노드에 라이선스 후처리만 재실행:

```bash
curl -X POST http://<master-ablecube-ip>:18090/api/v1/cube/license/apply \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{"action":"register","roles":["ccvm"]}'
```

SCVM bootstrap API 직접 실행:

```bash
curl -X POST http://<master-ablecube-ip>:18090/api/v1/cube/scvm/bootstrap \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{}'
```

CCVM bootstrap API 직접 실행:

```bash
curl -X POST http://<master-ablecube-ip>:18090/api/v1/cube/ccvm/bootstrap \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{}'
```

bootstrap 스크립트 실행 없이 라이선스 후처리만 재실행:

```bash
curl -X POST http://<master-ablecube-ip>:18090/api/v1/cube/scvm/bootstrap \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{"run_script":false}'
```

올인원 job에서 bootstrap step만 라이선스 후처리로 재실행:

```bash
curl -X POST http://<master-ablecube-ip>:18090/api/v1/cube/deploy/run \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{"only":["scvm_bootstrap"],"run_bootstrap_script":false}'
```

## Step 동작

`/cube/deploy/run`은 다음 step을 순서대로 처리한다.

| Step | 실행 조건 | 성공 시 systemProfile 반영 |
| --- | --- | --- |
| `license_apply` | 항상 선택 가능. `license_content`가 없으면 마스터 현재 라이선스 파일 사용 | `license.status=true`, `license.type=<license oem>` |
| `cluster_apply` | `cluster` 요청이 있으면 실행. `only`에 명시했는데 없으면 실패 | 없음 |
| `scvm_prepare` | HCI/HCI-FS에서 `scvm_by_host`가 있으면 SCVM cloud-init/XML/lifecycle setup 후 public health 확인 | 없음 |
| `scvm_bootstrap` | `scvm_prepare` 성공 후 자동 실행. 대표 SCVM 1대에서 `/root/bootstrap.sh` 실행 후 전체 SCVM 라이선스 후처리 | `bootstrap.scvm=true` |
| `rbd_prepare` | HCI-FS에서 `rbd.action=create` 요청으로 image를 생성하고 전체 ablecube의 `/etc/ceph/rbdmap` 반영 | 없음 |
| `storage_prepare` | Standalone 외 타입에서 `gfs`가 있으면 실행 | VM/HCI-FS: `bootstrap.gfs_configure=true` |
| `local_prepare` | Standalone에서 `local`이 있으면 실행 | `bootstrap.local_configure=true` |
| `ccvm_prepare` | `ccvm_cloudinit`, `ccvm_xml`, `ccvm_lifecycle` 중 하나 이상 있으면 CCVM cloud-init/XML/lifecycle setup 후 public health 확인 | 없음 |
| `ccvm_bootstrap` | `ccvm_prepare` 성공 후 CCVM 라이선스를 등록하고, CCVM이 실행 중인 host에서 `/root/bootstrap.sh`와 라이선스 status 확인 | `bootstrap.ccvm=true` |
| `monitoring_prepare` | `ccvm_bootstrap` 성공 후 CCVM Wall 최초 구성과 서비스 active/enabled 상태 검증 | `bootstrap.wall=true` |
| `system_profile` | 기본 실행. `update_system_profile=false`면 skip | 성공 step 기준으로 반영 |

`mode=all`에서 특정 step 입력이 없으면 그 step은 skip된다. `only`에 특정 step을 명시한 경우에는 필요한 입력이 없으면 실패한다. `only`에 `storage_prepare`가 있고 `rbd` payload도 있으면 기존 클라이언트 호환을 위해 `rbd_prepare`가 바로 앞에 자동 삽입된다.

`scvm_bootstrap`은 cloud-init이 VM 내부에 전달한 `/root/bootstrap.sh`를 host API가 qemu-guest-agent로 실행한 뒤, 대상 VM의 `/api/v1/health`가 응답하면 `/cube/license`로 라이선스를 등록하고 다시 status를 확인한다. 단독 Cloud VM 구성은 `/cube/license/apply`에 `roles:["ccvm"]`, `wait_for_ready:true`를 전달해 CCVM 시작 후 30초 간격으로 최대 6회 PCS `cloudcenter_res`가 Started인지 확인하고 라이선스만 등록한다. Standalone은 로컬 libvirt CCVM running 상태를 확인한다. 별도의 클라우드센터 구성 단계가 `/cube/ccvm/bootstrap`으로 상세 설정을 수행한 뒤 Mold bootstrap Job을 시작한다.

SCVM bootstrap 스크립트는 Ceph bootstrap과 전체 SCVM 후속 설정을 포함하므로 모든 SCVM에서 동시에 실행하지 않는다. 기본 동작은 `cluster.json`의 첫 번째 SCVM을 대표 실행 대상으로 사용하고, 라이선스 등록/status 확인은 전체 SCVM을 대상으로 수행한다.

이미 bootstrap 스크립트를 실행한 뒤 라이선스 후처리만 다시 실행해야 하면 직접 bootstrap API 호출에서 `run_script:false`를 전달하거나, `/cube/deploy/run`에서는 `run_bootstrap_script:false`를 전달한다.

`/cube/deploy/run`의 `scvm_bootstrap`/`ccvm_bootstrap` step은 별도 bootstrap API와 같은 실행 함수를 사용한다. 따라서 단독 운영 호출은 `/cube/scvm/bootstrap`, `/cube/ccvm/bootstrap`을 사용하고, 올인원에서는 동일한 동작이 job step 결과로 기록된다.

`bootstrap.wall`은 모니터링센터 구성 완료 플래그이므로 올인원 배포 job에서 자동으로 `true`로 변경하지 않는다. 모니터링 구성 API/절차가 완료된 뒤 별도로 반영한다.

job 이력은 프로세스 메모리에만 저장된다. API 서버가 재시작되면 `/cube/deploy/jobs` 목록은 초기화된다.

## 공통 요청 필드

```json
{
  "mode": "all",
  "only": ["license_apply", "cluster_apply"],
  "skip": ["local_prepare"],
  "license_content": "BASE64_CONTENT",
  "run_bootstrap_script": true,
  "licenses": {
    "ablecube12-1": "BASE64_CONTENT_HOST_1"
  },
  "license_filename": "license.lic",
  "cluster": {},
  "scvm_by_host": {},
  "gfs": {},
  "local": {},
  "ccvm_cloudinit": {},
  "ccvm_xml": {},
  "ccvm_lifecycle": {},
  "update_system_profile": true
}
```

`licenses`의 key는 `hosts[].hostname`, role 이름, target IP, `hosts[].index`, `scvmN` 중 하나를 사용할 수 있다.

## HCI 기준

HCI는 SCVM을 먼저 준비하고, SCVM bootstrap 이후 Storage Center/Glue 상태를 기반으로 CCVM을 준비한다. `cluster`에는 3대 이상의 host와 3대 이상의 `pcs_cluster_list`가 필요하다.

필수 흐름:

1. `license_apply`
2. `cluster_apply`
3. `scvm_prepare`
4. Storage Center 구성은 SCVM bootstrap/cloud-init 이후 Storage Center에서 처리
5. `ccvm_prepare`
6. `system_profile`

예시:

```json
{
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
      },
      {
        "index": "2",
        "hostname": "ablecube12-2",
        "ablecube": "10.10.12.2",
        "scvmMngt": "10.10.12.12",
        "ablecubePn": "100.100.12.2",
        "scvm": "100.100.12.12",
        "scvmCn": "100.200.12.12"
      },
      {
        "index": "3",
        "hostname": "ablecube12-3",
        "ablecube": "10.10.12.3",
        "scvmMngt": "10.10.12.13",
        "ablecubePn": "100.100.12.3",
        "scvm": "100.100.12.13",
        "scvmCn": "100.200.12.13"
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
    },
    "ablecube12-2": {
      "cpu": 8,
      "memory": 32,
      "disk_type": "disk_passthrough",
      "disk_passthrough_list": ["/dev/disk/by-id/wwn-0x2222"],
      "management_network_bridge": "br-mngt",
      "storage_traffic_network_type": "bridge",
      "server_network_bridge": "br-storage",
      "replication_network_bridge": "br-repl"
    },
    "ablecube12-3": {
      "cpu": 8,
      "memory": 32,
      "disk_type": "disk_passthrough",
      "disk_passthrough_list": ["/dev/disk/by-id/wwn-0x3333"],
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
}
```

SCVM passthrough disk/NIC 값은 host마다 다르므로 `/cube/disk`, `/cube/nics` 조회 결과를 기준으로 `scvm_by_host`를 구성한다.

## HCI Filesystem 기준

HCI Filesystem은 HCI 흐름에 공유 파일시스템 구성이 추가된다. `rbd_prepare`는 `/cube/rbd/manage`와 동일한 실행기로 RBD image를 생성하고 모든 ablecube에 rbdmap을 적용한다. 이후 `storage_prepare`가 `gfs` 요청으로 `/cube/gfs/manage` 흐름을 실행하고, 성공 시 `bootstrap.gfs_configure=true`를 반영한다.

`gfs.action=init-pcs-cluster`는 기존 PCS 구성을 초기화한 뒤 다음 순서로 GFS용 PCS cluster를 준비한다.

```text
modify_lvm_conf
set_cluster_password
auth_hosts
setup_cluster (setup --start)
cluster enable --all
cluster status 검증
```

기본값은 `cluster_name=cloudcenter_cluster`, `cluster_user=hacluster`, `cluster_password=password`이다. `configure-stonith`는 PCS cluster status 검증에 성공한 경우에만 실행된다.

GFS 스토리지 구성은 `init-pcs-cluster -> configure-stonith -> create-gfs -> set-alert` 순서로 호출한다. 따라서 `gfs.stonith`에 모든 호스트의 IPMI 주소와 계정이 필요하다. `create-gfs`는 `configure-stonith` 완료 후 최소 25초 동안 대기하면서 `pcs status`를 3초 간격으로 조회하고, `glue-locking-clone`이 전체 호스트에서 연속 2회 Started로 확인된 뒤 `vg_glue/lv_glue`, `glue-gfs`, `/mnt/glue-gfs` 고정값으로 GFS2 PCS 리소스를 생성한다. GFS2 journal 수는 `cluster.json`의 유효한 호스트 수에 예비 journal 1개를 더해 계산한다.

아래 JSON은 `rbd`, `gfs`, `ccvm_xml.gfs_mount_point` 입력 위치를 보여주는 축약 예시다. 실제 요청에서는 HCI 예시처럼 `hosts`와 `scvm_by_host`를 3대 이상 모두 입력하고, `pcs_cluster_list`도 3대 이상 지정해야 한다.

```json
{
  "mode": "all",
  "only": [
    "license_apply",
    "cluster_apply",
    "scvm_prepare",
    "scvm_bootstrap",
    "rbd_prepare",
    "storage_prepare",
    "ccvm_prepare",
    "ccvm_bootstrap",
    "system_profile"
  ],
  "cluster": {
    "action": "insert",
    "option": "add",
    "type": "ablestack-hci-filesystem",
    "ccvm": { "ip": "10.10.12.10" },
    "mngtNic": { "cidr": "16", "gw": "10.10.0.1", "dns": "8.8.8.8" },
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
  "rbd": {
    "action": "create",
    "pool_name": "rbd",
    "image_prefix": "gfs",
    "size": 5000
  },
  "gfs": {
    "action": "init-pcs-cluster",
    "cluster_name": "cloudcenter_cluster",
    "cluster_user": "hacluster",
    "cluster_password": "password",
    "mount_point": "/mnt/glue-gfs",
    "volume_groups": [
      { "vg_name": "vg_glue", "lv_name": "lv_glue" }
    ]
  },
  "ccvm_xml": {
    "cpu": 8,
    "memory": 32,
    "gfs_mount_point": "/mnt/glue-gfs",
    "management_network_bridge": "br-mngt"
  }
}
```

`rbd_prepare`의 `output.val`에는 생성된 `pool/image` 목록이, `output.results`에는 host별 rbdmap 적용 결과가 기록된다. 일부 host의 map 적용이 실패하면 해당 단계와 전체 Job이 실패하고 이후 `storage_prepare`는 실행되지 않는다.

## VM 기준

VM은 SCVM이 없고 GFS/PCS 스토리지 준비 후 CCVM을 구성한다. `pcs_cluster_list`는 1대 이상 필요하다.

```json
{
  "mode": "all",
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
  },
  "gfs": {
    "action": "init-pcs-cluster",
    "disks": ["/dev/disk/by-id/wwn-0xaaaa"],
    "volume_groups": [
      { "vg_name": "vg_glue", "lv_name": "lv_glue" }
    ]
  },
  "ccvm_xml": {
    "cpu": 8,
    "memory": 32,
    "gfs_mount_point": "/mnt/glue-gfs",
    "management_network_bridge": "br-mngt"
  }
}
```

VM에서 `storage_prepare`가 성공하면 `bootstrap.gfs_configure=true`가 반영된다.

## Standalone 기준

Standalone은 SCVM, PCS, GFS cluster가 없다. 로컬 디스크 준비 후 로컬 libvirt CCVM을 구성한다.

```json
{
  "mode": "all",
  "cluster": {
    "action": "insert",
    "option": "add",
    "type": "ablestack-standalone",
    "ccvm": { "ip": "10.10.41.10" },
    "mngtNic": { "cidr": "16", "gw": "10.10.0.1", "dns": "8.8.8.8" },
    "storage_network": "false",
    "hosts": [
      { "index": "1", "hostname": "ablecube41-1", "ablecube": "10.10.41.1" }
    ]
  },
  "local": {
    "action": "create-local-disk",
    "disks": ["/dev/sdb"]
  },
  "ccvm_xml": {
    "cpu": 8,
    "memory": 32,
    "management_network_bridge": "br-mngt"
  }
}
```

Standalone에서 `local_prepare`가 성공하면 `bootstrap.local_configure=true`가 반영된다.

## 부분 실행과 재시도

특정 step만 실행:

```json
{
  "mode": "partial",
  "only": ["ccvm_prepare"],
  "ccvm_xml": {
    "cpu": 8,
    "memory": 32,
    "management_network_bridge": "br-mngt"
  }
}
```

특정 step 제외:

```json
{
  "mode": "all",
  "skip": ["license_apply", "scvm_prepare"],
  "gfs": {
    "action": "init-pcs-cluster"
  },
  "ccvm_xml": {
    "cpu": 8,
    "memory": 32,
    "management_network_bridge": "br-mngt"
  }
}
```

systemProfile 자동 반영을 끄기:

```json
{
  "mode": "partial",
  "only": ["ccvm_prepare"],
  "update_system_profile": false,
  "ccvm_xml": {
    "cpu": 8,
    "memory": 32,
    "management_network_bridge": "br-mngt"
  }
}
```

## Job 조회

job 시작 응답:

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

상세 조회:

```bash
curl -sS http://<master-ablecube-ip>:18090/api/v1/cube/deploy/jobs/<job_id> \
  -H "Authorization: Bearer <access_token>"
```

job status:

| Status | 의미 |
| --- | --- |
| `queued` | job 생성 후 실행 대기 |
| `running` | step 실행 중 |
| `succeeded` | 선택된 모든 step 완료 |
| `failed` | 특정 step 실패 후 중단 |

step status:

| Status | 의미 |
| --- | --- |
| `pending` | 아직 실행 전 |
| `running` | 실행 중 |
| `succeeded` | 성공 |
| `failed` | 실패 |
| `skipped` | 타입 불일치 또는 입력 없음으로 건너뜀 |

## 운영 주의 사항

- 기존 개별 API는 계속 사용할 수 있다. 올인원 실패 후 특정 단계만 개별 API로 복구하고, `only`로 필요한 step만 재실행할 수 있다.
- `/cube/deploy/run`은 파괴적 reset을 기본으로 실행하지 않는다. reset/delete류 개별 API는 운영자가 명시적으로 호출해야 한다.
- SCVM XML의 passthrough disk/NIC 입력은 host마다 다르므로 `scvm_by_host`를 반드시 host별로 구성한다.
- `license_content`와 `licenses` 값은 job 조회 응답에 저장하지 않는다.
- `/cube/license/apply`와 `/cube/deploy/run`은 운영 API이므로 마스터 노드에 활성 라이선스가 있고 Bearer token을 발급받은 뒤 호출한다.
- `/cube/license/apply`의 `roles`가 비어 있으면 기존 호환성을 위해 `ablecube`만 대상으로 한다. VM이 아직 생성되지 않은 초기 단계에서는 `scvm`/`ccvm` role을 호출하지 않는다.
- SCVM/CCVM template에는 `ablestack-api` RPM과 service enable 상태가 포함되어 있어야 한다. CCVM/SCVM 안의 `cluster.json`은 `/etc/ablestack/properties/cluster.json` 경로에 있어야 한다.
