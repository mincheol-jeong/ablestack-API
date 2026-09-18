# SCVM SMB API 테스트 체크리스트

이 문서는 SCVM에서 `/api/v1/glue/smb` API를 실제 Samba/Ceph 환경과 함께 검증할 때 사용한다. SMB 구성은 사용자가 shell을 직접 실행하는 흐름이 아니라 `Cockpit UI -> Glue SMB API -> ablestack-api backend -> /etc/ablestack/shell/Samba-Execute.sh` 순서로 처리한다.

## 전제 조건

- 테스트 대상은 `ABLESTACK_NODE_ROLE=scvm`이거나 `/etc/ablestack/node-role`이 `scvm`인 노드여야 한다.
- SCVM에서 `ablestack-api` service가 실행 중이어야 한다.
- SCVM API가 `/api/v1/glue` route를 등록한 상태여야 한다.
- Bearer token은 SCVM에 등록된 라이선스 기준으로 발급/검증되어야 한다.
- `ceph.conf`와 `ceph.client.admin.keyring`이 SCVM에 있어야 한다.
- Samba, CephFS mount, systemd, firewalld, realmd/AD 관련 패키지는 SCVM 이미지 또는 SCVM role 패키지에서 제공되어야 한다.

## RPM 의존성 기준

`ablestack-api` RPM은 host, SCVM, CCVM에 공통으로 설치된다. 따라서 SCVM에서만 필요한 Samba/Ceph/realmd/podman 계열 명령은 RPM hard dependency로 추가하지 않는다.

| 구분 | 항목 | 기준 |
| --- | --- | --- |
| API RPM hard dependency | `systemd`, `bash`, `python3` | API service와 보안 증적·Samba·shell helper 실행에 필요. Linux 인증은 Go에서 처리 |
| API RPM recommended | `firewalld` | 있으면 RPM `%post`에서 18090/tcp open 처리 |
| SCVM SMB runtime | `ceph`, `mount`, `findmnt` 또는 `mountpoint`, `smbpasswd`, `pdbedit`, `useradd`, `userdel`, `systemctl` | SMB normal mode 실행 시 SCVM에 필요 |
| SCVM SMB ADS runtime | `realm`, `winbind.service`, `update-crypto-policies`, NetworkManager | ADS mode 실행 시 SCVM에 필요 |
| SCVM helper | `/etc/ablestack/shell/Samba-Execute.sh`, `/etc/ablestack/shell/smb_conf` | API RPM의 `shell/*` 설치 경로 |

RPM 설치 확인:

```bash
rpm -q ablestack-api
test -x /etc/ablestack/shell/Samba-Execute.sh
test -x /etc/ablestack/shell/smb_conf
grep '^ABLESTACK_GLUE_SMB_SCRIPT=' /etc/ablestack/ablestack-api.env
```

## 기본 상태 확인

```bash
curl -sS http://<scvm-ip>:18090/api/v1/health

curl -sS http://<scvm-ip>:18090/api/v1/glue/smb \
  -H "Authorization: Bearer <access_token>"
```

확인 항목:

- HTTP status가 `200`인지 확인한다.
- 응답 JSON에 `status`, `state`, `hostname`, `security_type`, `path_list`가 포함되는지 확인한다.
- 최초 상태에서는 `security_type`이 `normal`이어야 한다.
- `path_list`가 비어 있거나 기존 share만 포함되어야 한다.

## Normal SMB 생성

```bash
curl -X POST http://<scvm-ip>:18090/api/v1/glue/smb \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "sec_type": "normal",
    "username": "smbuser01",
    "password": "Password1!",
    "cache_policy": "true",
    "folder_name": "share01",
    "path": "/gluefs/volumes/share01",
    "fs_name": "gluefs",
    "volume_path": "/volumes/share01"
  }'
```

확인 항목:

- API 응답 `status`가 `success`인지 확인한다.
- `/etc/samba/smb.conf`에 `[share01]` section과 `path = /gluefs/volumes/share01`가 있는지 확인한다.
- `/etc/fstab`에 같은 mount path가 중복 없이 한 줄만 등록되는지 확인한다.
- `findmnt /gluefs/volumes/share01` 또는 `mountpoint /gluefs/volumes/share01`가 성공하는지 확인한다.
- `systemctl is-active smb.service`가 `active`인지 확인한다.
- `pdbedit -L`에 `smbuser01`이 있는지 확인한다.

## 상태 재조회

```bash
curl -sS http://<scvm-ip>:18090/api/v1/glue/smb \
  -H "Authorization: Bearer <access_token>"
```

확인 항목:

- `security_type`이 `normal`인지 확인한다.
- `users`에 `smbuser01`이 포함되는지 확인한다.
- `path_list.share01.path`가 `/gluefs/volumes/share01`인지 확인한다.
- `path_list.share01.mount_yn`이 `true`인지 확인한다.

## Share folder 추가

```bash
curl -X POST http://<scvm-ip>:18090/api/v1/glue/smb/folder \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "cache_policy": "false",
    "folder_name": "share02",
    "path": "/gluefs/volumes/share02",
    "fs_name": "gluefs",
    "volume_path": "/volumes/share02"
  }'
```

확인 항목:

- `/etc/samba/smb.conf`에 `[share02]` section이 추가되는지 확인한다.
- `/etc/fstab`에 `/gluefs/volumes/share02`가 중복 없이 등록되는지 확인한다.
- `GET /api/v1/glue/smb`의 `path_list`에 `share02`가 추가되는지 확인한다.

## Share folder 삭제

```bash
curl -X DELETE http://<scvm-ip>:18090/api/v1/glue/smb/folder \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "folder_name": "share02",
    "path": "/gluefs/volumes/share02",
    "fs_name": "gluefs"
  }'
```

확인 항목:

- `/etc/samba/smb.conf`에서 `[share02]` section이 제거되는지 확인한다.
- `/etc/fstab`에서 `/gluefs/volumes/share02` 줄이 제거되는지 확인한다.
- `GET /api/v1/glue/smb`의 `path_list`에서 `share02`가 사라지는지 확인한다.

## SMB user 변경

생성:

```bash
curl -X POST http://<scvm-ip>:18090/api/v1/glue/smb/user \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{"username":"smbuser02","password":"Password1!"}'
```

수정:

```bash
curl -X PUT http://<scvm-ip>:18090/api/v1/glue/smb/user \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{"username":"smbuser02","password":"Password2!"}'
```

삭제:

```bash
curl -X DELETE http://<scvm-ip>:18090/api/v1/glue/smb/user \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{"username":"smbuser02"}'
```

확인 항목:

- 생성 후 `pdbedit -L`에 `smbuser02`가 있는지 확인한다.
- 수정 후 SMB client에서 새 password로 접근 가능한지 확인한다.
- 삭제 후 `pdbedit -L`에서 `smbuser02`가 사라지는지 확인한다.

## 전체 삭제

```bash
curl -X DELETE http://<scvm-ip>:18090/api/v1/glue/smb \
  -H "Authorization: Bearer <access_token>"
```

확인 항목:

- `systemctl is-active smb.service`가 inactive 또는 failed가 아닌 중지 상태인지 확인한다.
- `systemctl is-enabled smb.service`가 disabled인지 확인한다.
- `/etc/samba/smb.conf`가 기본 `[global]` normal 구성으로 정리되는지 확인한다.
- `/etc/fstab`에서 SMB API가 추가한 CephFS mount path들이 제거되는지 확인한다.
- `GET /api/v1/glue/smb`의 `security_type`이 `normal`인지 확인한다.
- `path_list`가 비어 있거나 의도한 잔여 share가 없는지 확인한다.

## ADS mode 확인

ADS는 AD DNS/realm 환경에 영향을 주므로 normal mode 검증이 끝난 뒤 별도 환경에서 확인한다.

```bash
curl -X POST http://<scvm-ip>:18090/api/v1/glue/smb \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "sec_type": "ads",
    "username": "administrator",
    "password": "Password1!",
    "cache_policy": "false",
    "folder_name": "adshare01",
    "path": "/gluefs/volumes/adshare01",
    "fs_name": "gluefs",
    "volume_path": "/volumes/adshare01",
    "realm": "EXAMPLE.LOCAL",
    "dns": "10.10.10.10"
  }'
```

확인 항목:

- `realm list`에 대상 realm이 표시되는지 확인한다.
- `systemctl is-active smb.service`와 `systemctl is-active winbind.service`가 `active`인지 확인한다.
- `/etc/samba/smb.conf`의 `[global]`에 `security = ads`, `realm = EXAMPLE.LOCAL`이 있는지 확인한다.
- `GET /api/v1/glue/smb`의 `security_type`이 `ads`이고 `realm`이 표시되는지 확인한다.
- 전체 삭제 후 `winbind.service`가 중지/disable되고 `security_type`이 `normal`로 돌아오는지 확인한다.

## 실패 응답 확인

의도적으로 누락 값을 보내 API가 실패를 명확히 반환하는지 확인한다.

```bash
curl -X POST http://<scvm-ip>:18090/api/v1/glue/smb \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{"sec_type":"normal","username":"smbuser01"}'
```

확인 항목:

- HTTP status가 실패로 내려오는지 확인한다.
- API 오류 내용에 실행 command와 stderr 원인이 포함되는지 확인한다.
- password 값이 응답이나 로그에 원문으로 노출되지 않는지 확인한다.

`Samba-Execute.sh`의 stderr는 `ERROR[64]`, `ERROR[65]`, `ERROR[66]`, `ERROR[70]` 형식으로 출력된다.

| 코드 | 의미 |
| --- | --- |
| `64` | 요청 인자 누락 또는 잘못된 action/option |
| `65` | helper 또는 필수 명령 누락 |
| `66` | 필수 설정 파일 누락 |
| `70` | 실제 runtime 명령 실패 |
