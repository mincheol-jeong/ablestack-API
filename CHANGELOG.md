# CHANGELOG

ABLESTACK API의 변경 이력은 이 파일에 기록한다. RPM 버전은 `VERSION` 파일을 기준으로 관리한다.

## Unreleased

### Changed

- `clusterConfig.gfs`에 `mkfs.gfs2` journal/resource group 크기 설정을 추가했습니다. `journal_size_mb`는 기본 512MB(8~1024MB), `resource_group_size_mb`는 기본 1024MB(32~2048MB)이며 두 값 모두 2의 거듭제곱만 허용합니다. GFS 생성 시 각각 `-J`, `-r` 인자로 적용하고 cluster.json에는 설정 범위 설명을 함께 보존합니다.
- 올인원 deploy Job을 제품별 실제 실행 흐름에 맞게 보강했습니다. VM/HCI Filesystem의 `storage_prepare`는 PCS 초기화만으로 완료 처리하지 않고 `init-pcs-cluster -> configure-stonith -> create-gfs -> set-alert`를 순차 실행하며, HCI Filesystem은 직전 `rbd_prepare`가 생성한 image를 `/dev/rbd/<pool>/<image>` 장치로 이어받습니다. GFS 전체 성공 후에만 `bootstrap.gfs_configure=true`를 반영합니다.
- 올인원 deploy에 `monitoring_prepare` 단계를 추가해 CCVM bootstrap 완료 후 기존 Wall 최초 구성 API와 서비스 검증을 실행하고, 성공 후에만 `bootstrap.wall=true`를 반영합니다. 최초 `license_apply`는 아직 저장되지 않은 올인원 요청의 호스트 목록을 우선 사용하며, 잘못된 mode/step은 시작 전에 거부하고 queued/running Job이 있는 동안 중복 배포 Job 시작을 `409`로 차단합니다.
- Apache CloudStack 4.22 공식 API 계약에 맞춰 `createPhysicalNetwork`의 속도 파라미터를 `networkspeed`로 교정했습니다. 호스트 제거는 `prepareHostForMaintenance` Job 완료 후 `listHosts.resourcestate=Maintenance`를 검증하고, 동기 `deleteHost`의 `success=true`를 확인하며 `forcedestroylocalstorage=false`를 명시해 로컬 스토리지의 강제 삭제를 방지합니다.
- 제품별 비동기 호스트 제거 Job을 추가했습니다. 공통으로 대상 Host API Health를 확인하고 Mold `prepareHostForMaintenance` 비동기 Job 완료와 `listHosts`의 실제 `Maintenance` 상태를 확인한 뒤에만 `deleteHost`를 호출합니다. HCI는 Ceph drain/OSD 및 SCVM 제거, VM은 PCS/GFS 노드와 펜싱 장치 제거, HCI Filesystem은 두 흐름을 순서대로 수행한 뒤 전체 노드의 `cluster.json`·`/etc/hosts`와 CCVM Wall 모니터링 대상을 갱신합니다. 각 단계의 진행·성공·실패와 원본 실행 결과는 Job 조회 API에서 확인할 수 있습니다.
- HCI 추가 호스트의 SCVM만 준비한 뒤 기존 첫 번째 SCVM의 Cephadm 공개키를 설치하고 `ceph orch host add`/검증으로 기존 Glue 클러스터에 조인하는 대상 지정 Job 흐름을 추가했습니다. 초기 Ceph bootstrap은 재실행하지 않습니다.
- 클러스터 추가 작업의 `hostType=add`를 `cluster.json`에 보존해 신규 호스트의 SCVM 배포 화면이 기존 클러스터 조인 흐름을 선택할 수 있도록 했습니다.
- 추가 호스트 프로파일을 기존 Ablecube·SCVM·CCVM 전체에 fan-out하고 신규 SCVM cloud-init에도 동일한 `cluster.json`과 `/etc/hosts`를 포함합니다. SCVM에서 파일을 재생성할 때 로컬 `scvm`, `scvm-mngt`, `cn-scvm` 별칭도 유지하도록 호스트명 판정을 보완했습니다.
- 추가 SCVM 시작 후 API Health를 20초 간격으로 최대 5회 확인하고, 성공하면 고정 대기 없이 `prepare_local_scvm`과 기존 Glue 클러스터 호스트 등록을 순서대로 실행합니다. 신규 Ceph 호스트에는 별도 `_admin` 라벨을 추가하지 않습니다.
- 신규 SCVM의 `node-exporter`, `process-exporter`, `glue-api.service`와 ipcorrector cron은 Health 확인 직후 `prepare_local_scvm`에서 활성화합니다.
- CCVM bootstrap의 SCVM Crushmap 설정, Ablecube TPM agent 파일 배포, Pacemaker/Corosync 활성화에서 SSH/SCP를 제거했다. Host API가 `cluster.json`의 동적 대상에 내부 토큰 API를 호출하고 각 SCVM/Ablecube API가 작업을 로컬 실행한 뒤 CCVM 내부 bootstrap을 진행하며, 대상별 결과를 bootstrap 응답과 deploy job에 기록한다.
- 상태 수집 컨트롤러를 취소 가능한 ticker 기반 스케줄러로 변경하고, 이전 실행이 끝나지 않은 handler의 중복 실행을 방지하며 종료 시 실행 중 작업을 기다리도록 개선했다. 오류 목록의 추가·조회·초기화도 동시성 안전하게 처리한다.
- RPM `%check`가 일부 package만 검사하던 방식을 `go vet ./...`와 `go test ./...` 전체 검사로 강화했다.
- SSH 포트 확인의 host/port 결합을 안전하게 처리하고, 운영 환경 정책에 맞춰 key scan을 IPv4 전용으로 제한했다.
- 올인원 deploy step의 원격 오류 문구를 동적 format 문자열로 해석하지 않도록 변경해 vet 실패와 `%` 포함 오류의 잘못된 출력을 방지했다.
- GFS 마운트 상태 조회의 장치 정보를 경로 모드별로 분리해 멀티패스 환경에서는 `/dev/mapper/mpath*`만, 싱글패스 환경에서는 `/dev/sd*` 물리 장치만 반환하도록 변경했습니다.
- 마운트된 GFS 디스크 상태 조회에서 동일 마운트에 여러 `disk_id`가 합쳐지더라도 확장·삭제 화면에는 정렬된 대표 UUID 하나만 반환하도록 변경했습니다.
- 전체 디스크 상세 조회(`GET /cube/disk?action=detail`)의 멀티패스 목록을 `multipath -ll` 결과 기준으로 반환하고, 멀티패스가 없는 환경은 `lsblk`의 실제 디스크를 반환하도록 변경했습니다. 두 방식 모두 `/`, `/boot`, `/boot/efi`, swap이 포함된 OS 디스크는 제외합니다.
- GFS/CLVM 디스크 구성 payload는 표시용 `/dev/mapper/*` 또는 `/dev/sdX` 대신 `/dev/disk/by-id/*` 안정 경로만 사용하도록 변경했다. multipath는 `dm-uuid-mpath-*`, single path는 `wwn-*`, `scsi-*`, `nvme-*`, `ata-*` 순으로 선택하고 안정 경로가 없는 장치는 후보에서 제외하며, 화면과 CLVM 응답에서는 WWN 컬럼을 제거하고 UUID만 사용한다.
- GFS/CLVM 디스크 목록에 정규화된 UUID와 경로 모드를 추가했다. multipath는 `DM_UUID`/by-id에서 `mpath-` 접두사를 제거한 WWID를, single path는 `lsblk WWN`을 사용하며 서비스 상태가 아닌 실제 블록 토폴로지로 구분한다.
- GFS 디스크 인벤토리에서 multipath 장치 하위에 파티션이 있으면 사용 중으로 표시하고, GFS 초기 구성·추가·확장 및 CLVM 추가 화면에서 해당 장치를 선택할 수 없도록 변경했다.
- CCVM의 매일 01:00 자동 스냅샷과 cloud DB dump를 API 프로세스 내부 Go 스케줄러로 통합했다. 기존 `create_ccvm_snap.py`, `backup_mysql.py` root cron은 API 시작 및 PCS setup 시 제거하며, 사용자가 DB Dump API로 만든 `RegularBackup`/`DeleteOldBackup` 일정은 유지한다.
- Linux 계정 비밀번호 검증에서 Python `crypt`/`spwd` subprocess를 제거했다. API가 `/etc/shadow`를 직접 읽고 EL9 기본 yescrypt와 SHA-512, SHA-256, bcrypt, MD5 crypt 해시를 Go에서 검증한다.
- 호출되지 않던 CCVM lifecycle Python script 실행·응답 파싱 helper를 제거했다.
- 보안 패치의 Host·SCVM·CCVM 원격 실행을 SSH에서 내부 토큰 기반 `/cube/security/patch` API fan-out으로 변경했다. 각 대상 API가 역할별 로컬 `security_patch.sh`를 실행하며 응답에 transport, API URL, HTTP 상태와 스크립트 결과를 포함한다. 후속 `security_patch.status` 전파는 기존 Python `ablestackJson.py` 호출을 제거하고 `/cube/system/config` API로 `cluster.json`의 `systemProfile.security_patch.status=true`를 반영한다.
- CCVM Wall Python 실행 시 `ABLESTACK_CLUSTER_JSON=/etc/ablestack/properties/cluster.json`을 전달해 SCVM·CCVM의 API 표준 cluster.json 경로를 사용하도록 통일했다.
- CCVM lifecycle에 재배포 전용 `initialize` action을 추가했다. `cloudcenter_res`가 존재할 때만 제거하고 `/mnt/glue-gfs/ccvm*` 파일만 정리하며, 리소스 또는 PCS가 아직 없으면 정상적으로 다음 구성 단계로 진행한다.
- API 서버 기본 포트를 `18090`으로 변경하고 listen 포트와 노드 간 호출 포트를 `ABLESTACK_API_PORT`로 통일했다. RPM 빌드 시 `API_PORT`/`api_port`로 포트를 지정할 수 있으며, 설치 후 `configure-api-port.sh`로 환경 갱신, 새 방화벽 포트 추가, 서비스 재시작 및 listen 확인, 기존 방화벽 포트 제거를 안전하게 처리한다. 실패 시 기존 포트 설정으로 복구한다.
- CCVM Wall 이미지가 제공하는 DB/Python 런타임 때문에 Host·SCVM·CCVM 공통 API RPM 설치가 차단되지 않도록 `sqlite`와 `mariadb`를 hard dependency에서 제외하고 역할별 런타임 요구사항으로 정리했다.
- `/cube/ccvm/monitoring/config`를 최초 구성(`configure`)과 수집 대상 재갱신(`update`)으로 분리했다. 최초 구성은 CCVM 기준 대상 연결 확인, Netdive 설정, 서비스 정지, Wall/Prometheus/Loki 설정, 서비스 enable/start, SMTP 설정, 전체 서비스 active/enabled 검증을 순서대로 수행하며 모든 단계 성공 후에만 전체 호스트의 `bootstrap.wall=true`를 반영한다.
- Wall 모니터링 구성을 CCVM 이미지에서 검증된 기존 `host_ping_test.py`, `config_netdive.py`, `start_services.py`, `config_wall.py`, `config_loki.py`, `config_smtp.py` 실행 흐름으로 복원했다. API는 스크립트별 JSON 결과와 Prometheus, exporter, Grafana, Netdive, Loki, Promtail의 실제 active/enabled 상태를 검증하고 모든 단계 성공 후에만 완료 상태를 반영한다.
- Python과 중복되던 Go Wall YAML/INI/DB 구성 및 원격 SSH/SCP 구현을 제거하고, API의 `wallservice`를 Python 실행·JSON 결과 파싱·로컬 서비스 상태 검증 역할로 축소했다.
- 모니터링 Python에 전달하는 `--cube`, `--ccvm`, `--scvm` 주소는 요청 body로 덮어쓰지 않고 `cluster.json`의 Host, CCVM, SCVM 관리 IP에서만 구성하도록 고정했다.
- CCVM Wall Python 단계 timeout을 기존 2분에서 10분으로 분리해 Netdive의 다중 Cube SCP/SSH 재시도를 허용하고, timeout 응답에 CCVM에서 Cube로의 passwordless SSH/SCP 점검 원인을 표시하도록 개선했다.
- GFS 리소스 상태 응답에 `glue-gfs_res` LVM 활성화와 `glue-gfs` Filesystem clone의 노드별 상태를 묶은 `gfs_mount_resources`를 추가하고 기존 `glue_gfs_resources` 응답은 호환용으로 유지했다.
- CCVM `setup`, `reset`, `start`, `restart` 성공 직후 `ccvm`, `ccvm-mngt`, CCVM 관리 IP의 SSH host key를 즉시 다시 수집하도록 연결했다. 스캔 전에 손상된 `known_hosts` 행을 원자적으로 복구해 기존 키 제거가 실패하지 않도록 했다.
- 모니터링 구성 응답에 단계별 실행 결과, 서비스별 active/enabled 상태와 system profile 반영 결과를 추가하고 명령별 표준 출력과 오류 원인을 실패 응답에 포함하도록 변경했다.
- 모니터링 대상 구성 API가 제품 타입에 따라 HCI 계열에서만 SCVM을 포함하고, ABLESTACK-Standalone에서는 첫 번째 Ablecube 한 대와 CCVM만 구성하도록 대상 생성을 정리했습니다.

### Added

- Mold bootstrap의 `add_host` 앞에 `sync_host_ssh_trust` job step을 추가했다. CCVM의 CloudStack management 공개키를 읽어 `cluster.json`의 모든 `hosts[].ablecube`에 `cloud@ccvm` 식별자로 멱등 등록하고, 같은 관리 개인키로 호스트별 SSH 접속을 검증한 결과를 job에 기록한다.
- 호스트의 다른 인증키를 유지하면서 `cloud@ccvm` 항목만 등록·조회·제거하는 `POST /cube/ssh/trust` API를 추가했다.
- 클라우드센터의 Wall 모니터링을 최초 구성하거나 수집 대상을 재갱신할 수 있도록 `POST /cube/ccvm/monitoring/config` API를 추가했다. Host API가 요청을 CCVM 내부 API로 전달하고 실행 결과를 단계별로 반환한다.

## [0.1.5] - 2026-07-02

### Added

- Health Check와 cloud-init 생성 후 CCVM 생성과 시작을 분리할 수 있도록 비활성 PCS 리소스를 생성하는 `/cube/ccvm/create` API를 추가했다.
- `/cube/security/evidence` 생성/최신 조회 API와 `/cube/security/evidence/download` ZIP 다운로드 API를 추가하고 U-01~U-67 수집 카탈로그, TXT/XLSX/PPTX 패키지 생성기를 API RPM에 포함했다.
- GFS Manage API에 `configure-stonith` action을 추가해 host 수에 따라 펜싱 장치를 동적으로 생성·갱신하고 대상별 결과를 반환하도록 했다.
- CCVM 전용 `/api/v1/mold` namespace를 추가하고, `cluster.json`의 `clusterConfig.ccvm.ip` 값을 기준으로 Mold `/client/api` endpoint를 동적으로 계산하도록 했다.
- Mold 초기 자동화의 선행 단계로 `/mold/session`에서 login sessionkey를 발급하고, `/mold/capabilities`에서 login 후 `listCapabilities`를 호출할 수 있도록 했다.
- Mold 인프라 자동화 입력값을 검증하고 `createZone -> createPhysicalNetwork -> createPod -> addCluster -> addHost -> createStoragePool -> addSecondaryStorage -> updateZone` 실행 순서와 각 list 검증 API를 반환하는 `/mold/bootstrap/plan` API를 추가했다.
- Mold bootstrap 실행 결과가 zone, physical network, pod, cluster, host, primary/secondary storage, system VM 단위로 성공/검증 상태와 Mold 에러 원문을 반환할 수 있도록 단계별 결과 모델을 추가했다.
- Mold bootstrap을 비동기 job으로 시작하는 `/mold/bootstrap`과 진행/최종 결과를 조회하는 `/mold/jobs`, `/mold/jobs/{job_id}` API 골격을 추가했다.
- Mold bootstrap job step 실행기를 추가해 `createZone`, `createPhysicalNetwork`, `createPod`, `addCluster`, `addHost`, `createStoragePool`, `addSecondaryStorage`, `createSecondaryStagingStore`, `updateZone`, `listSystemVms`를 순차 실행하고 각 단계마다 대응되는 `list*` API로 실제 반영 여부를 검증하도록 했다.
- Mold create/add/update 응답이 `jobid`를 반환하면 `queryAsyncJobResult`로 async job 완료를 polling하고, 실패 시 원래 step command와 Mold `errorcode/errortext`를 job 결과에 남기도록 했다.
- Mold bootstrap 기본 리소스 이름을 `Zone`, `Pod`, `Cluster`처럼 첫 글자만 대문자인 형식으로 정규화하도록 했다.
- Mold API가 `{command}response.errorcode/errortext` 형태로 반환하는 중첩 에러를 실패로 감지하도록 보완했다.
- Mold bootstrap job의 시작, 단계별 실행/성공/실패, 최종 완료 상태를 job ID와 command, 검증 command, 소요 시간, 오류 정보와 함께 `/var/log/ablestack/job.log`에 기록하도록 했다.
- Mold API readiness 확인에서 인증 전 `listCapabilities`가 CloudStack 형식의 HTTP 401 오류를 반환하면 endpoint가 응답 가능한 상태로 판단하고 `login_admin` 단계로 진행하도록 했다.
- Mold bootstrap이 `clusterConfig.type`에 따라 HCI는 `Primary Storage(RBD)`/ABLESTACK Glue Block, VM·Standalone·HCI Filesystem은 `Primary Storage(Glue)`/DefaultPrimary SharedMountPoint를 자동 구성하도록 했다.
- HCI Glue Block의 monitor를 `clusterConfig.hosts[].index` 기준 `scvm1,scvm2,...`로 만들고, 첫 번째 접근 가능한 host에서 `ceph auth get-key client.admin`으로 secret을 조회해 RBD URL과 `krbdPath=/dev/rbd`를 구성하도록 했다.
- Mold bootstrap의 host URL을 `clusterConfig.hosts[].ablecube`, secondary storage URL을 `clusterConfig.ccvm.ip`, physical network VLAN을 `1-1`로 자동 구성하도록 했다.
- CCVM cloud-init에 CloudStack management용 SSH 키를 `/var/cloudstack/management/.ssh/id_rsa(.pub)` 경로로 함께 배치해 passwordless host 연결에 사용할 수 있도록 했다.
- Mold Zone bootstrap에 Management, Guest, Public Traffic Type을 추가하고 각 `kvmnetworklabel`을 `bridge0`로 고정했으며, 기존 Traffic Type의 label이 다르면 `updateTrafficType`으로 교정하도록 했다.
- Traffic Type 구성 후 Physical Network를 `Enabled`로 전환하고 VirtualRouter/VpcVirtualRouter element와 Network Service Provider를 자동 활성화하도록 했다.
- Pod과 별도로 Public IP gateway/netmask/start/end를 입력받아 VLAN 없이 `createVlanIpRange(forvirtualnetwork=true)`로 생성하고 `listVlanIpRanges`로 검증하도록 했다.
- Mold 4.21 Zone Wizard 흐름에 맞춰 Secondary Storage 생성 command를 `addSecondaryStorage`에서 `addImageStore(provider=NFS)`로 변경하고 Physical Network 속도 파라미터를 `speed`로 교정했다.

### Changed

- CCVM bootstrap 실행 경로를 PCS Started 호스트의 QEMU Guest Agent `guest-exec` 방식에서 `clusterConfig.ccvm.ip`의 CCVM API 직접 호출 방식으로 변경했습니다. CCVM API가 `/root/bootstrap.sh`를 로컬 실행하므로 호스트별 QGA command 허용 설정에 의존하지 않습니다.
- 상태 카드 polling 시작 조건을 실제 구성 완료 시점에 맞췄다. HCI 계열은 SCVM 실행 후 스토리지 VM, SCVM bootstrap 후 스토리지 클러스터, CCVM 실행 후 클라우드 VM·클라우드 클러스터 조회를 활성화하며, ABLESTACK-VM은 GFS 구성 후 클라우드 클러스터, CCVM 실행 후 클라우드 VM 조회를 활성화한다.
- `/cube/deploy/status` 응답에 상태 카드별 `polling.enabled`와 비활성 사유를 추가하고, 클러스터 구성·SCVM bootstrap·GFS 구성·CCVM 실행·CCVM bootstrap 완료 여부에 따라 개별 상태 API 호출 가능 여부가 자동 전환되도록 변경했다.
- `ablestack-vm` 배포 상태 판정은 CCVM 실행과 bootstrap 완료를 확인한 뒤 CloudCenter PCS/resource 상태를 조회하도록 순서를 교정했다.
- CCVM 라이선스 등록 요청에 `wait_for_ready` 옵션을 추가해 가상머신 시작 후 30초 간격으로 최대 6회 PCS `cloudcenter_res` Started 상태를 확인한 뒤 라이선스를 전달하도록 변경했다. Standalone은 로컬 libvirt CCVM running 상태를 사용한다. 단독 Cloud VM 구성은 라이선스 등록까지만 수행하고, CCVM bootstrap 상세 설정과 `bootstrap.ccvm=true` 반영은 별도의 클라우드센터 구성 단계에서 수행한다.
- CCVM cloud-init이 8090/tcp 방화벽 규칙과 `ablestack-api.service` 활성화를 보장하도록 보완하고, 라이선스 등록 전 PCS/libvirt readiness 시도별 결과를 `api.log`에 기록하도록 변경했다.
- 라이선스 fan-out의 로컬 파일 읽기, 대상별 전송 시작·완료, 원격 응답과 CCVM `/cube/license` 수신·처리 결과를 민감한 라이선스 내용 없이 `api.log`에 기록하도록 보완했다.
- PCS CCVM start가 명령 실행 호스트의 로컬 `virsh domid`를 최대 25분 기다리던 문제를 수정하고, 클러스터 전체 `cloudcenter_res`가 어느 노드에서든 Started인지 확인한 즉시 반환해 후속 라이선스 등록 API가 실행되도록 변경했다.
- 개별 CCVM 배포 흐름을 제품 타입별로 분리해 HCI 계열은 RBD 이미지와 PN 대상 XML 배포를, ABLESTACK-VM은 GFS의 `ccvm.qcow2`와 관리망 Host 대상 XML 배포를 사용하도록 변경했다.
- ABLESTACK-VM CCVM 시작 시 `/mnt/glue-gfs` 마운트를 확인하고 공유 `ccvm.xml`과 `ccvm.qcow2`를 준비한 뒤 PCS 리소스를 생성하도록 변경했다.
- CCVM lifecycle `start`는 PCS 리소스 enable 이후 실제 CCVM domain이 실행 상태가 될 때까지 확인한 뒤 완료하도록 변경했다.
- 펜싱 장치의 `pcmk_reboot_action`과 PCS `stonith-action`을 `reboot`로 설정하도록 변경했다.
- 보안 패치 실행 시 ablecube는 API RPM에 포함된 최신 호스트 스크립트를, ccvm/scvm은 cloud-init으로 배포된 `/usr/local/sbin/security_patch.sh`를 사용하도록 대상별 실행 경로를 분리하고 응답에 `targetKind`, `scriptPath`를 포함했다.
- 보안 증적 PPTX에 예외처리 사유를 표시하고, 과도하게 긴 명령 결과는 최대 2페이지로 요약한 뒤 전체 결과를 TXT에서 확인하도록 안내하며 긴 제목 크기를 자동 조절하도록 변경했다.
- 보안 증적 U-01, U-27, U-42, U-66, U-67 점검 명령과 예외 판정을 Cockpit 화면의 최신 점검 기준에 맞췄다.
- HCI Filesystem 올인원 배포에 `rbd_prepare` Job 단계를 추가해 RBD image 생성과 전체 ablecube의 `/etc/ceph/rbdmap` 반영을 `storage_prepare`보다 먼저 실행하고 단계별 생성 image 및 host 적용 결과를 반환하도록 했다.
- 기존 클라이언트가 `only`에 `storage_prepare`만 지정하더라도 `rbd` payload가 있으면 `rbd_prepare`를 자동으로 선행 삽입하도록 했다.
- GFS `init-pcs-cluster`가 전체 ablecube의 LVM lock 설정, pcsd 활성화, hacluster 비밀번호 설정, `pcs host auth`, `pcs cluster setup --start`, `pcs cluster enable --all`, cluster status 검증을 순서대로 수행하도록 보완했다.
- GFS `init-pcs-cluster`의 고정 PCS 클러스터 이름을 `cloudcenter_cluster`로 통일했다.
- GFS 구성에 `create-gfs` action을 추가해 locking clone 준비 상태를 polling한 뒤 디스크 파티션, shared VG/LV, 호스트 수보다 journal이 1개 많은 GFS2 파일시스템, LVM/Filesystem clone 리소스와 제약조건을 생성하도록 했다.
- STONITH 구성 중 `glue-dlm`, `glue-lvmlockd`, `glue-locking-clone` 생성 오류를 무시하지 않고 호출자에게 반환하도록 변경했다.
- `create-gfs`가 locking 리소스 생성 직후 진행하지 않도록 최소 25초 안정화 시간과 전체 노드 연속 2회 Started 확인을 추가했다.
- GFS 마법사가 최종 완료 상태를 기록할 수 있도록 기존 `gfs-configure` System Config fan-out API를 React 완료 흐름에 연결했다.
- `configure-stonith`는 GFS용 PCS cluster가 준비되지 않았으면 실행하지 않고 `init-pcs-cluster` 선행 필요 오류를 반환하도록 변경했다.
- System Config API에 `gfs-configure` action을 추가해 `bootstrap.gfs_configure` 완료 상태를 내부 토큰 기반 API로 전체 ablecube에 전파하도록 했다.
- API가 배포하는 `security_patch.sh`에 Cockpit의 firewalld 상태 보장, U-34/U-36/U-46/U-48 조치, U-62 Banner 및 관련 파일 권한 변경을 동기화했다.
- SCVM bootstrap을 모든 host API의 QGA 기반 로컬 준비 후 index가 가장 낮은 master SCVM에서 Ceph bootstrap을 수행하는 단계형 흐름으로 변경하고, 단계별 응답에 `action`을 추가했다.
- `scvm_bootstrap.sh`의 직접 SSH/SCP를 제거하고 SCVM별 서비스 설정은 로컬 `prepare`, Ceph host 등록·설정/keyring 배포는 Cephadm orchestrator가 담당하도록 변경했다.
- master가 생성한 Cephadm 공개키를 QGA 실행 결과로 읽어 각 SCVM의 `authorized_keys`에 로컬 설치한 뒤 host 등록을 수행하며, Cephadm 개인키는 master 밖으로 전달하지 않도록 했다.
- `TypeHosts`가 내부 lock을 값으로 복사하지 않도록 host snapshot의 적용 인자와 생성 반환값을 포인터 방식으로 변경했다.
- `multipath_sync.sh`에서 직접 SSH/SCP로 각 호스트를 제어하던 흐름을 제거하고, `/cube/multipath/sync` API를 호출하는 로컬 wrapper 방식으로 변경했다.

### Fixed

- GFS 기존 LUN 확장 전에 모든 ablecube에서 SCSI 경로를 rescan하고 `multipathd resize map`을 실행하도록 연결했다. multipath map 크기 반영 후 파티션, PV, LV, GFS2 순서로 확장하며 SCSI·multipath·parted 단계 오류를 더 이상 무시하지 않는다.
- GFS 디스크 확장 시 숫자로 끝나는 multipath alias 또는 map 자체를 PV로 사용하는 구성에서 파티션 번호 `1`을 중복 추가해 `mpathb11` 같은 잘못된 경로로 `pvresize`하던 문제를 수정했다. `lsblk`에서 확인한 LV의 실제 직계 부모 PV 경로를 그대로 사용한다.
- GFS 디스크 삭제 시 VG/LV 제거 전에 실제 multipath 디스크와 PV 파티션 경로를 수집하고, `pvremove` 후 `parted rm 1`까지 완료하도록 수정했다. 화면도 삭제 요청에 물리 path 대신 multipath 경로를 우선 전달하며 PV 또는 파티션 삭제 실패를 성공으로 무시하지 않는다.
- GFS 생성 전 PCS 준비 상태는 `glue-locking-clone`, `glue-gfs-clone`, `glue-gfs_res-clone` 구간의 `Stopped`만 검사하도록 수정했다. fence 상태, 다른 리소스의 Failed Resource Actions 및 Started 노드 수는 판정에서 제외한다.
- 모니터링 구성 API의 `configure` action을 Wall Python이 허용하는 `config` positional action으로 변환하지 않아 `invalid choice: configure`로 실패하던 문제를 수정했다.
- `ssh-keyscan`의 stderr 오류 문구가 `/root/.ssh/known_hosts`에 섞여 파일 형식이 손상되던 문제를 수정했다. 갱신 전에 기존 파일의 잘못된 행을 제거하고, 유효한 OpenSSH host key만 저장하며, 동시 갱신과 `ssh-keygen -R` 실패를 명시적으로 처리한다.
- SSH 포트 전용 변경(`port_change=true`) 시 `PermitRootLogin` 정책까지 덮어쓰던 문제를 수정했다.
- Cockpit의 로컬 `ablestack.json`만 갱신하던 GFS 완료 상태가 호스트별로 달라질 수 있는 문제를 API 전체 fan-out 흐름으로 수정했다.
- 호스트 제거 시 남은 `clusterConfig.hosts[]`의 `index` 값을 1부터 다시 부여하도록 수정했다.
- HCI 계열 호스트 제거 시 `/etc/hosts`의 SCVM/PN/CN alias 정리 기준을 삭제 대상 host의 기존 `index` 값으로 맞췄다.

## [0.1.4] - 2026-06-19

### Added

- SCVM/CCVM bootstrap을 단독 실행할 수 있도록 `/cube/scvm/bootstrap`, `/cube/ccvm/bootstrap` API를 추가했다. 각 API는 host qemu-guest-agent로 VM 내부 `/root/bootstrap.sh`를 실행한 뒤 VM API health와 라이선스 후처리를 수행한다.
- 기존 `multipath_sync.sh` 흐름을 대체할 수 있도록 SSH/SCP 없이 host API fan-out으로 SCSI rescan과 multipath bindings/wwids 동기화를 수행하는 `/cube/multipath/sync` API를 추가했다.
- `/cube/version/update`에 `update_type=all,mold`를 추가해 전체 업데이트(`update-all.sh`)와 Mold 업데이트(`update-mold.sh`)를 선택 실행할 수 있도록 했다.

### Changed

- SCVM Swagger `doc.json`은 Glue 중심 화면이 되도록 Cube 운영/내부 통신 API path/tag를 숨기고, 인증, health, version, license 계열 API만 남기도록 변경했다.
- SCVM Swagger tag 순서를 Glue 계열이 먼저 보이도록 재정렬했다.
- Swagger 문서 필터링 후 사용하지 않는 model definition을 제거해 host/CCVM과 SCVM Swagger 화면에 역할과 무관한 schema가 남지 않도록 정리했다.
- `/cube/deploy/run`의 `scvm_bootstrap`, `ccvm_bootstrap` step이 별도 bootstrap API와 같은 실행 함수를 사용하도록 정리했다.
- SCVM bootstrap 스크립트는 Ceph bootstrap 중복 실행을 피하기 위해 대표 SCVM 1대에서만 실행하고, 라이선스 등록/status 확인은 전체 SCVM 대상으로 유지하도록 변경했다.
- bootstrap 스크립트 실행 후 라이선스 후처리만 재시도할 수 있도록 직접 bootstrap API에는 `run_script`, `/cube/deploy/run`에는 `run_bootstrap_script` 옵션을 추가했다.
- `/cube/version/update`는 ISO 마운트 경로를 직접 실행하지 않고 `/opt/ABLESTACK_UPDATE`로 복사한 뒤 작업 디렉터리에서 스크립트를 실행하도록 변경했다.
- `/cube/version/update`의 `info` 응답에 현재/대상 OS 버전과 Mold 버전을 포함하고, 대상 Mold 버전은 `AppStream/Packages/mold` RPM 파일명에서 버전명과 날짜까지만 우선 추출하도록 변경했다.
- `/cube/version/update`의 `run`은 `/cube/deploy/status`가 `stage=ready`인 경우에만 실행되도록 서버 측 조건을 추가했다.
- `/cube/gfs/disk/status`의 `blockdevices[]`에 `df -hP` 기준 `used`, `avail`, `use_percent`를 mountpoint별로 추가했다.
- `cluster apply` insert 흐름에서 요청 body의 `security.internal_token`이 있으면 그 값을 사용하고, 없으면 기존 token을 보장하거나 새로 생성해 apply-local payload로 전파하도록 정리했다.
- `/cube/cluster/config` 응답이 다운로드용 cluster.json에 필요한 `clusterConfig`와 `security`를 함께 반환하고 `systemProfile`은 제외하도록 변경했다.
- `clusterConfig.iscsi_storage` 필드명을 `storage_network`로 변경하고, 기존 `iscsi_storage` 입력은 호환용으로 `storage_network`로 정규화하도록 했다.

### Fixed

- `/cube/deploy/status`에서 SCVM/CCVM bootstrap flag가 이미 true인 경우에도 VM running 상태를 raw 응답에 계속 반환하도록 수정했다.
- `/cube/gfs/disk/status`가 `/mnt/glue-gfs`, `/mnt/glue-gfs-1`처럼 여러 GFS mountpoint가 있는 환경에서 각 mountpoint의 사용량을 개별 매칭하도록 보완했다.

## [0.1.3] - 2026-06-05

### Added

- SCVM XML 생성 API에 `disk_passthrough` 디스크 타입과 `disk_passthrough_list` 입력을 추가했다.
- 라이선스 등록 API에서 `multipart/form-data` 파일 업로드를 지원해 서버가 라이선스 파일을 읽고 인코딩 처리하도록 추가했다.
- 마스터 노드에서 라이선스를 전체 ablecube 호스트로 배포할 수 있도록 `/cube/license/apply` API를 추가했다.
- 라이선스 배포, 클러스터 구성 적용, SCVM 준비, 스토리지 준비, 로컬 스토리지 준비, CCVM 준비, systemProfile 반영을 순차 실행하는 올인원 배포 job API `/cube/deploy/run`, `/cube/deploy/jobs`, `/cube/deploy/jobs/{job_id}`를 추가했다.
- 올인원 배포 API 사용 방법과 HCI/HCI Filesystem/VM/Standalone 타입별 실행 흐름을 `docs/deploy-run-guide.md`에 추가했다.
- 모든 API 요청과 주요 action 로그를 `/var/log/ablestack/api.log`에, 오류 상세 로그를 `/var/log/ablestack/detail.log`에 남기도록 추가했다.
- background job 실패, 상태 변화, 자동 백업 실행 결과를 `/var/log/ablestack/job.log`에 남기도록 추가했다.
- 날짜가 지난 로그를 `/var/log/ablestack/archive`에 `api.log-YYYY-MM-DD.gz`, `detail.log-YYYY-MM-DD.gz`, `job.log-YYYY-MM-DD.gz` 형식으로 일 단위 압축 보관하고 기본 90일 보관 후 정리하도록 했다.
- `/api/v1/glue` namespace와 `internal/handler/glue`, `internal/model/glue` 골격을 추가하고, Glue API route를 SCVM role에서만 등록하도록 제한했다.
- `internal/service/glueservice`를 추가하고 `/api/v1/glue/status`, `/hosts`, `/version`, `/pool`, `/image`, `/service`를 SCVM 로컬 `ceph`, `rbd`, `ceph orch` 명령 기반 실제 실행 API로 연결했다.
- `/api/v1/glue/gluefs`, `/nfs`, `/rgw`의 조회 계열 endpoint를 SCVM 로컬 `ceph`, `ceph nfs`, `ceph orch`, `radosgw-admin` 명령 기반 실제 실행 API로 연결했다.
- `/api/v1/glue/gluefs`, `/nfs`, `/rgw`의 생성/수정/삭제 endpoint를 SCVM 로컬 `ceph`, `ceph nfs`, `ceph orch`, `radosgw-admin`, Ceph dashboard API 기반 실제 실행 API로 연결하고, NFS ingress spec 적용/재배포 흐름을 추가했다.
- `/api/v1/glue/nvmeof` endpoint를 SCVM 로컬 `ceph orch`, `rbd`, `podman run/exec` 기반 실제 실행 API로 연결했다.
- `/api/v1/glue/iscsi` endpoint를 SCVM 로컬 `ceph orch`, Ceph dashboard API, `podman exec gwcli` 기반 실제 실행 API로 연결했다.
- `/api/v1/glue/smb` endpoint를 SCVM 로컬 Samba 실행 스크립트 기반 실제 실행 API로 연결하고, SMB password가 실패 응답에 노출되지 않도록 마스킹했다.
- `/api/v1/glue/mirror` endpoint를 SCVM 로컬 `rbd mirror`, `ceph orch`, bootstrap token import/export 기반 실제 실행 API로 연결했다.
- 기존 glue-api의 `Samba-Execute.sh`와 `smb_conf` helper를 `shell/` 리소스로 포함해 RPM 설치 시 `/etc/ablestack/shell/` 아래에 배치되도록 추가했다.
- SCVM SMB API 테스트 체크리스트를 `docs/scvm-smb-test-checklist.md`에 추가했다.

### Changed

- 라이선스 등록 성공 시 `cluster.json`의 `systemProfile.license.status=true`와 함께 복호화된 라이선스 `oem` 값을 `systemProfile.license.type`에 자동 반영하도록 변경했다.
- NVMe-oF 흐름에서 legacy SSH 기반 실행을 제거하고 SCVM 로컬 실행만 사용하도록 변경했다.
- iSCSI purge 흐름에서 legacy SSH 기반 실행을 제거하고 SCVM 로컬 실행만 사용하도록 변경했다.
- SMB 흐름에서 legacy SSH host 반복을 제거하고 SCVM 로컬 실행만 사용하도록 변경했다.
- SMB 기본 실행 경로를 기존 glue-api 경로에서 `/etc/ablestack/shell/Samba-Execute.sh`로 변경했다.
- `Samba-Execute.sh`를 action별 함수 구조와 flag parser로 정리하고, 필수 값/의존 명령/설정 파일 오류를 명확한 exit code와 stderr 메시지로 반환하도록 개선했다.
- Samba/Ceph/realmd 같은 SCVM 전용 runtime command는 API RPM hard dependency가 아니라 role 이미지/패키지에서 보장하는 기준으로 정리했다.
- Mirror 흐름에서 legacy SSH/scp 기반 remote cluster 직접 제어를 제거하고 bootstrap token 교환 방식으로 변경했다.
- Glue API 명령 실행에서 legacy SSH와 shell pipeline 의존을 제거하고, pool/image/service 입력값을 검증해 legacy `grep|cut` 방식과 command injection 위험을 줄였다.
- Swagger tag를 `Cube-License`, `Cube-Nic`, `Glue-RGW`, `Glue-GlueFS`처럼 기능 단위로 세분화하고, host/CCVM에서는 Swagger `doc.json`에서도 SCVM 전용 Glue path가 보이지 않도록 변경했다.
- `/cube/license/apply`에 `roles` 입력을 추가해 기존 `hosts[].ablecube` 기본 배포는 유지하면서, SCVM 생성 후 `roles:["scvm"]`, CCVM 생성 후 `roles:["ccvm"]`, 전체 API 대상에는 `roles:["all"]`로 라이선스를 fan-out 등록할 수 있도록 했다.
- 라이선스 배포 응답에 `role`을 포함해 `ablecube`, `scvm`, `ccvm`, 명시 target 대상이 구분되도록 했다.
- `/api/v1/health`와 `/health`를 라이선스/토큰 없이 호출 가능한 public health check로 추가하고, 내부 health probe가 기존 `/cube/cluster/health` 대신 이 경로를 사용하도록 변경했다.
- 올인원 배포 job에 `scvm_bootstrap`, `ccvm_bootstrap` 단계를 추가해 VM 생성 후 API health 확인, 라이선스 자동 등록, 라이선스 status 확인을 분리했다.
- SCVM/CCVM bootstrap 성공 시점에 `systemProfile.bootstrap.scvm`, `systemProfile.bootstrap.ccvm`을 true로 반영하도록 변경했다.
- CCVM cloud-init에 포함하는 `cluster.json` 경로를 API 기본 경로인 `/etc/ablestack/properties/cluster.json`로 변경했다.
- legacy `/neighbor`, `/glue`, `/mold`, `/pcs`, `/dashboard` API 라우트와 관련 handler/model 패키지를 제거하고, Cube API 중심 구조로 정리했다.
- neighbor 전용 `configs/config.json`, `CUBE_CONFIG_PATH`, `controller.SaveConfig/LoadConfig` 흐름을 제거했다.
- 라이선스 조회/등록 API는 Bearer token 없이 호출할 수 있도록 하고, 활성 라이선스가 있는 상태에서 라이선스를 교체할 때만 기존 Bearer token을 요구하도록 했다.
- API access token 서명값을 활성 라이선스의 `license_key`에서 파생하도록 변경했다.
- `ABLESTACK_AUTH_TOKEN_SECRET` 환경 변수 override를 제거해 API access token 서명값이 항상 활성 라이선스 기준으로 계산되도록 했다.
- 라이선스가 없거나 만료된 경우 라이선스 조회/등록과 Swagger를 제외한 API 요청을 차단하도록 했다.
- API 서버 내부 background job 실행 주기를 10초에서 30초로 조정했다.
- 라이선스 파일 업로드 등록에서 `original_filename` 입력을 제거하고 업로드 파일명을 그대로 저장 파일명으로 사용하도록 변경했다.
- 활성 라이선스 교체 시 Bearer token 외에 유효한 `X-Cube-Internal-Token` 내부 호출도 허용해 `/cube/license/apply` fan-out에서 원격 호스트 라이선스를 교체할 수 있도록 했다.
- 라이선스 기반 인증 구조에 맞춰 `/auth/sync`, `/auth/apply`와 `auth.json`의 `access_token_secret` 설정을 제거하고, RPM 업데이트 시 기존 `auth.json`의 legacy key도 정리하며 내부 토큰 관리는 `/auth/internal-token/*` API로 분리했다.
- `detail.log`에 라이선스 등록 실패 단계, 누락 필드, 파일명, 오류 분류 등 원인 분석용 진단 정보를 남기도록 개선했다.
- Go 모듈 기준 버전과 RPM 빌드 요구 버전을 Go 1.26.2로 상향했다.
- Go 모듈 의존성을 최신 안정 버전 기준으로 갱신했다.
- `/version` API가 OS `PRETTY_NAME`, Kernel `uname -r`, Cockpit `Version`, Mold `ACS_VERSION`, ABLESTACK 패키지 목록, HCI 계열의 `glue version` 값을 함께 반환하도록 확장하고 라우트를 활성화했다.
- `/version` API 응답에서 하드코딩된 CUBE 버전 기본값을 제거했다.

### Fixed

- `golang.org/x/net`을 `v0.55.0`으로 올려 HTTP/2 취약 의존성 경고를 해소하고, SSH 경로 보안 수정을 위해 `golang.org/x/crypto`를 `v0.52.0`으로 함께 업데이트했다.
- 라이선스 등록 API의 multipart 업로드 처리 추가 중 `err` 재선언으로 RPM 빌드가 실패하던 문제를 수정했다.

## [0.1.2] - 2026-05-27

### Added

- Cockpit 로그인 세션에서 비밀번호 재입력 없이 Bearer 토큰을 발급할 수 있도록 `ablestack-auth-token` CLI를 추가했다.
- 클러스터 전체 API 서버의 인증 서명값을 맞출 수 있도록 `/auth/sync`, `/auth/apply` API를 추가했다.
- `cluster apply` insert 시 `security.internal_token`을 생성하고 apply-local payload로 전파하도록 했다.
- `/cube/ssh/key` API를 추가해 각 호스트에서 `/root/.ssh/id_rsa`, `/root/.ssh/id_rsa.pub`, `/root/.ssh/authorized_keys`를 생성하고 관리할 수 있도록 했다.
- SSH 키를 Windows PC로 옮겨 다른 호스트에 적용할 수 있도록 암호화된 단일 파일 다운로드/업로드 흐름을 추가했다.

### Changed

- 인증 서명값은 토큰 발급 경로에서만 생성하고, API 서버 시작/토큰 검증/동기화 경로에서는 자동 생성하지 않도록 정리했다.
- `/auth/sync`는 `host`, `scvm`, `ccvm`, `all` 옵션으로 동기화 대상을 선택하고 대상별 성공/실패 결과를 반환하도록 변경했다.
- `/auth/sync`의 `all` 대상은 HCI 계열에서 `host/scvm/ccvm`, VM/standalone 계열에서 `host/ccvm`만 포함하도록 os_type 기준을 반영했다.
- 정렬된 `cluster.json` 구조에서도 auth sync 대상이 정상 수집되도록 `clusterConfig` 읽기 로직을 보완했다.
- 대상 호스트의 `security.internal_token`이 비어 있거나 다른 값이면 `/auth/sync`를 실행한 호스트의 internal token과 인증 서명값으로 덮어쓰도록 보완했다.
- cloud-init ISO 생성은 CCVM/SCVM 전용 API만 노출하고, 파일 경로와 NIC/IP를 모두 받던 저수준 `/cube/cloudinit/generate` POST API는 제거했다.
- 단일 동작만 수행하는 HBA 조회와 Glue config 동기화 POST API는 Swagger에서 요청 body를 제거하고 기존 body 입력은 호환만 유지하도록 정리했다.
- `/cube/cluster/health`의 `option`은 `host,scvm`처럼 콤마 조합을 허용하고, `target_hostname`은 host/scvm/ccvm 역할별 표시 이름을 콤마로 지정할 수 있도록 정리했다. `option` 없이 `target_hostname`만 지정하면 이름으로 role을 추론한다.
- Swagger 요청 body에서 내부 fan-out용 `security`, deprecated alias 필드를 숨겨 화면에서 불필요한 값을 보내지 않도록 정리했다.
- 클러스터 구성 `insert` 적용 성공 시 각 호스트에서 `/etc/chrony.conf` 생성과 `chronyd` 재시작을 자동 실행하도록 했다.
- 시간 서버 구성은 `clusterConfig.external_timeserver`와 `hosts[].index` 1, 2의 `ablecube` 값을 기준으로 자동 계산하도록 정리했다.
- 시간 서버 수동 재적용 endpoint는 내부/수동 용도로 유지하되 Swagger와 공개 API 설명 문서에서는 숨겼다.
- SSH 키 다운로드 파일은 키 내용을 직접 노출하지 않도록 AES-GCM으로 암호화하고 랜덤 `.dat` 파일명으로 내려주도록 했다.

## [0.1.1] - 2026-05-26

### Added

- UI 배포 흐름을 단순화할 수 있도록 `/cube/deploy/status` API를 추가했다.
- 배포 상태 응답에 `stage`, `message_key`, `severity`, `available_actions`, `warnings`, `raw`를 포함하도록 했다.
- `os_type`별로 필요한 raw 상태 값만 반환하도록 정리했다.
- CloudCenter PCS/resource 상태를 배포 상태 판단에 포함했다.
- PCS 클러스터 대상을 `pcs_cluster_list` 입력 기준으로 1~16개까지 관리할 수 있도록 확장했다.

### Changed

- 기존 화면의 sessionStorage 기반 배포 판단 로직을 백엔드에서 의미형 상태 enum으로 계산할 수 있도록 구조화했다.
- `ablestack-vm`은 PCS 대상 1대부터 허용하고, `ablestack-hci`와 `ablestack-hci-filesystem`은 PCS 대상 3대 이상을 요구하도록 검증 기준을 분리했다.
- HCI 계열에서 3대 초과 설치 시 전체 호스트가 아니라 Ceph MON 대상 노드만 PCS 목록으로 사용하도록 문서화했다.
- PCS status, setup, snapshot, Glue config 복사 경로가 `hostname1~3` 고정값 대신 동적 PCS 목록을 사용하도록 변경했다.
- Swagger와 API 설명 문서를 신규 배포 상태 API와 동적 PCS 목록 기준에 맞춰 갱신했다.

### Fixed

- CloudCenter 상태 조회가 로컬 PCS만 확인하던 흐름을 보완해 실행 가능한 PCS 노드를 선택하도록 정리했다.
- Glue config 복사 시 첫 번째 PCS 노드에만 의존하지 않고 사용 가능한 PCS 노드를 순차적으로 확인하도록 보완했다.
- Mold 상태 조회 중 fatal 종료나 재귀 호출로 API 프로세스 안정성에 영향을 줄 수 있는 부분을 정리했다.

## [0.1.0] - 2026-05-26

### Added

- RPM 빌드 기준 버전을 `VERSION` 파일로 분리했다.
- RPM 산출물에 `CHANGELOG.md`와 `VERSION`을 문서 파일로 포함한다.
- Linux 계정 기반 로그인 API와 Bearer 토큰 인증 흐름을 추가했다.
- Swagger UI에서 Bearer 토큰을 사용할 수 있도록 보안 정의와 문서 패치 스크립트를 추가했다.
- 내부 호스트 간 호출용 토큰을 `cluster.json`에서 생성, 검증, 교체할 수 있는 흐름을 추가했다.

### Changed

- RPM 설치 경로를 `/etc/ablestack` 기준으로 정리했다.
- 생성되는 VM 설정 파일 경로를 `/etc/ablestack/vmconfig`로 통일했다.
- RPM 업데이트 시 기존 설정 값을 유지하고 누락된 JSON key만 병합하도록 정리했다.
- RPM 설치 시 `firewalld`가 있으면 API 포트 `8090/tcp`를 자동으로 열도록 정리했다.
- Go module import 경로를 GitHub 저장소명 대신 `ablecloud.io/ablestack-api` 기준으로 변경했다.
- `internal/handler/cube` 파일명을 기능 기준으로 정리했다.

### Fixed

- RPM 빌드 시 이전 파일명에 남아 있던 import 경로 때문에 실패하던 문제를 정리했다.
- `configs/config.json` 누락으로 서비스 시작 중 panic이 발생하던 경로 문제를 정리했다.
- `ccvm_secondary_resize.go`의 libvirt helper 참조 오류를 정리했다.
