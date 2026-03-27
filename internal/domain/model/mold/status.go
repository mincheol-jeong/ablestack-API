package mold

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
	"github.com/gofrs/uuid"
	"github.com/melbahja/goph"
	"github.com/ycyun/Cube-API/internal/infra/utils"
	"log"
	"net/http"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"time"
)

type TypeServerStatus struct {
	Active  bool `json:"active"`
	Enabled bool `json:"enabled"`
}
type TypeMoldServiceStatus struct {
	MoldWebService *TypeServerStatus `json:"mold_web_service"`
	MoldDBService  *TypeServerStatus `json:"mold_db_service"`
} // @name TypeMoldWebStatus

type TypeMoldWebStatus struct {
	Code int    `json:"code"`
	Body string `json:"body"`
} // @name TypeMoldWebStatus

// TypeMoldStatus model info
// @Description Glue의 상태를 나타내는 구조체
type TypeMoldStatus struct {
	Fsid   uuid.UUID `json:"fsid" example:"9980ffe8-4bc1-11ee-9b1f-002481004170" format:"uuid"` //Glue클러스터를 구분하는 ID
	Health struct {
		Status string `json:"status" example:"HEALTH_WARN" format:"string"`
		Checks interface {
		} `json:"checks"`
		Mutes interface{} `json:"mutes"`
	} `json:"health"`
	ElectionEpoch int      `json:"election_epoch" example:"148" format:"uint32"`
	Quorum        []int    `json:"quorum"`
	QuorumNames   []string `json:"quorum_names"`
	QuorumAge     int      `json:"quorum_age"`
	Monmap        struct {
		Epoch             int    `json:"epoch"`
		MinMonReleaseName string `json:"min_mon_release_name"`
		NumMons           int    `json:"num_mons"`
	} `json:"monmap"`
	Osdmap struct {
		Epoch          int `json:"epoch"`
		NumOsds        int `json:"num_osds"`
		NumUpOsds      int `json:"num_up_osds"`
		OsdUpSince     int `json:"osd_up_since"`
		NumInOsds      int `json:"num_in_osds"`
		OsdInSince     int `json:"osd_in_since"`
		NumRemappedPgs int `json:"num_remapped_pgs"`
	} `json:"osdmap"`
	Pgmap struct {
		PgsByState []struct {
			StateName string `json:"state_name"`
			Count     int    `json:"count"`
		} `json:"pgs_by_state"`
		NumPgs        int   `json:"num_pgs"`
		NumPools      int   `json:"num_pools"`
		NumObjects    int   `json:"num_objects"`
		DataBytes     int64 `json:"data_bytes"`
		BytesUsed     int64 `json:"bytes_used"`
		BytesAvail    int64 `json:"bytes_avail"`
		BytesTotal    int64 `json:"bytes_total"`
		ReadBytesSec  int   `json:"read_bytes_sec"`
		WriteBytesSec int   `json:"write_bytes_sec"`
		ReadOpPerSec  int   `json:"read_op_per_sec"`
		WriteOpPerSec int   `json:"write_op_per_sec"`
	} `json:"pgmap"`
	Fsmap struct {
		Epoch     int           `json:"epoch"`
		ByRank    []interface{} `json:"by_rank"`
		UpStandby int           `json:"up:standby"`
	} `json:"fsmap"`
	Mgrmap struct {
		Available   bool     `json:"available"`
		NumStandbys int      `json:"num_standbys"`
		Modules     []string `json:"modules"`
		Services    struct {
			Dashboard  string `json:"dashboard"`
			Prometheus string `json:"prometheus"`
		} `json:"services"`
	} `json:"mgrmap"`
	Servicemap struct {
		Epoch    int         `json:"epoch"`
		Modified string      `json:"modified"`
		Services interface{} `json:"services"`
	} `json:"servicemap"`
	ProgressEvents struct {
	} `json:"progress_events"`
	RefreshTime time.Time `json:"refresh_time"`
} // @name TypeMoldStatus

var lockGlueStatus sync.Once
var lockWebStatus sync.Once
var lockServiceStatus sync.Once

var _moldStatus *TypeMoldStatus
var _moldWebStatus *TypeMoldWebStatus
var _moldServiceStatus *TypeMoldServiceStatus

func Status() *TypeMoldStatus {
	if _moldStatus == nil {
		lockGlueStatus.Do(
			func() {
				if gin.IsDebugging() {
					fmt.Println("Creating ", reflect.TypeOf(_moldStatus), " now.")
				}
				_moldStatus = &TypeMoldStatus{}
				_moldWebStatus = &TypeMoldWebStatus{}
				_moldServiceStatus = &TypeMoldServiceStatus{
					MoldWebService: &TypeServerStatus{},
					MoldDBService:  &TypeServerStatus{},
				}
			})
	} else {
		if gin.IsDebugging() {
			fmt.Println("get old ", reflect.TypeOf(_moldStatus), " instance.")
		}
	}
	return _moldStatus
}

func UpdateStatus() *TypeMoldStatus {
	Status()
	if gin.Mode() == gin.ReleaseMode {
		var stdout []byte

		cmd := exec.Command("ceph", "-s", "-f", "json")
		stdout, _ = cmd.CombinedOutput()

		if err := json.Unmarshal(stdout, &_moldStatus); err != nil {
			utils.FancyHandleError(err)

		}
	} else {

		stdout := []byte("{\n  \"fsid\":\"9980ffe8-4bc1-11ee-9b1f-002481004170\",\n  \"health\":{\n    \"status\":\"HEALTH_WARN\",\n    \"checks\":{\n      \"RECENT_MGR_MODULE_CRASH\":{\n        \"severity\":\"HEALTH_WARN\",\n        \"summary\":{\n          \"message\":\"4 mgr modules have recently crashed\",\n          \"count\":4\n        },\n        \"muted\":false\n      }\n    },\n    \"mutes\":[\n\n    ]\n  },\n  \"election_epoch\":148,\n  \"quorum\":[\n    0,\n    1,\n    2\n  ],\n  \"quorum_names\":[\n    \"scvm1\",\n    \"scvm3\",\n    \"scvm2\"\n  ],\n  \"quorum_age\":1320385,\n  \"monmap\":{\n    \"epoch\":9,\n    \"min_mon_release_name\":\"reef\",\n    \"num_mons\":3\n  },\n  \"osdmap\":{\n    \"epoch\":13906,\n    \"num_osds\":19,\n    \"num_up_osds\":19,\n    \"osd_up_since\":1694672928,\n    \"num_in_osds\":19,\n    \"osd_in_since\":1693900905,\n    \"num_remapped_pgs\":0\n  },\n  \"pgmap\":{\n    \"pgs_by_state\":[\n      {\n        \"state_name\":\"active+clean\",\n        \"count\":801\n      }\n    ],\n    \"num_pgs\":801,\n    \"num_pools\":8,\n    \"num_objects\":255687,\n    \"data_bytes\":1010750055765,\n    \"bytes_used\":1945430351872,\n    \"bytes_avail\":16298248544256,\n    \"bytes_total\":18243678896128,\n    \"read_bytes_sec\":5370,\n    \"write_bytes_sec\":3247913,\n    \"read_op_per_sec\":75,\n    \"write_op_per_sec\":99\n  },\n  \"fsmap\":{\n    \"epoch\":1,\n    \"by_rank\":[\n\n    ],\n    \"up:standby\":0\n  },\n  \"mgrmap\":{\n    \"available\":true,\n    \"num_standbys\":1,\n    \"modules\":[\n      \"cephadm\",\n      \"dashboard\",\n      \"iostat\",\n      \"nfs\",\n      \"prometheus\",\n      \"restful\"\n    ],\n    \"services\":{\n      \"dashboard\":\"https://10.10.1.13:8443/\",\n      \"prometheus\":\"http://100.100.1.13:9283/\"\n    }\n  },\n  \"servicemap\":{\n    \"epoch\":14150,\n    \"modified\":\"2023-10-11T08:41:52.944377+0000\",\n    \"services\":{\n      \"rbd-mirror\":{\n        \"daemons\":{\n          \"summary\":\"\",\n          \"20610527\":{\n            \"start_epoch\":13092,\n            \"start_stamp\":\"2023-10-05T02:04:38.425678+0000\",\n            \"gid\":20610527,\n            \"addr\":\"100.100.1.12:0/3953042359\",\n            \"metadata\":{\n              \"arch\":\"x86_64\",\n              \"ceph_release\":\"reef\",\n              \"ceph_version\":\"ceph version Glue-Diplo-4.0.0 (5dd24139a1eada541a3bc16b6941c5dde975e26d) reef (stable)\",\n              \"ceph_version_short\":\"Glue-Diplo-4.0.0\",\n              \"container_hostname\":\"scvm2\",\n              \"container_image\":\"localhost:5000/glue/daemon@sha256:87d1dba17511fc6dd0f89cead2aac496ac427226f475b2251f72f5a933268e59\",\n              \"cpu\":\"Intel(R) Xeon(R) Silver 4210 CPU @ 2.20GHz\",\n              \"distro\":\"rocky\",\n              \"distro_description\":\"Rocky Linux 9.2 (Blue Onyx)\",\n              \"distro_version\":\"9.2\",\n              \"hostname\":\"scvm2\",\n              \"id\":\"scvm2.zpdohp\",\n              \"instance_id\":\"20610527\",\n              \"kernel_description\":\"#1 SMP PREEMPT_DYNAMIC Wed Aug 16 10:08:14 KST 2023\",\n              \"kernel_version\":\"5.14.0-284.25.2.ablecloud.el9.x86_64\",\n              \"mem_swap_kb\":\"16777212\",\n              \"mem_total_kb\":\"32600252\",\n              \"os\":\"Linux\"\n            },\n            \"task_status\":{\n\n            }\n          },\n          \"21129580\":{\n            \"start_epoch\":13093,\n            \"start_stamp\":\"2023-10-05T02:04:50.518201+0000\",\n            \"gid\":21129580,\n            \"addr\":\"100.100.1.13:0/2840805437\",\n            \"metadata\":{\n              \"arch\":\"x86_64\",\n              \"ceph_release\":\"reef\",\n              \"ceph_version\":\"ceph version Glue-Diplo-4.0.0 (5dd24139a1eada541a3bc16b6941c5dde975e26d) reef (stable)\",\n              \"ceph_version_short\":\"Glue-Diplo-4.0.0\",\n              \"container_hostname\":\"scvm3\",\n              \"container_image\":\"localhost:5000/glue/daemon@sha256:87d1dba17511fc6dd0f89cead2aac496ac427226f475b2251f72f5a933268e59\",\n              \"cpu\":\"Intel(R) Xeon(R) Silver 4210 CPU @ 2.20GHz\",\n              \"distro\":\"rocky\",\n              \"distro_description\":\"Rocky Linux 9.2 (Blue Onyx)\",\n              \"distro_version\":\"9.2\",\n              \"hostname\":\"scvm3\",\n              \"id\":\"scvm3.yfdixv\",\n              \"instance_id\":\"21129580\",\n              \"kernel_description\":\"#1 SMP PREEMPT_DYNAMIC Wed Aug 16 10:08:14 KST 2023\",\n              \"kernel_version\":\"5.14.0-284.25.2.ablecloud.el9.x86_64\",\n              \"mem_swap_kb\":\"16777212\",\n              \"mem_total_kb\":\"32600256\",\n              \"os\":\"Linux\"\n            },\n            \"task_status\":{\n\n            }\n          }\n        }\n      },\n      \"rgw\":{\n        \"daemons\":{\n          \"summary\":\"\",\n          \"12167529\":{\n            \"start_epoch\":3806,\n            \"start_stamp\":\"2023-09-15T07:04:49.891558+0000\",\n            \"gid\":12167529,\n            \"addr\":\"100.100.1.11:0/2444620655\",\n            \"metadata\":{\n              \"arch\":\"x86_64\",\n              \"ceph_release\":\"reef\",\n              \"ceph_version\":\"ceph version Glue-Diplo-4.0.0 (5dd24139a1eada541a3bc16b6941c5dde975e26d) reef (stable)\",\n              \"ceph_version_short\":\"Glue-Diplo-4.0.0\",\n              \"container_hostname\":\"scvm1\",\n              \"container_image\":\"localhost:5000/glue/daemon@sha256:87d1dba17511fc6dd0f89cead2aac496ac427226f475b2251f72f5a933268e59\",\n              \"cpu\":\"Intel(R) Xeon(R) Silver 4210 CPU @ 2.20GHz\",\n              \"distro\":\"rocky\",\n              \"distro_description\":\"Rocky Linux 9.2 (Blue Onyx)\",\n              \"distro_version\":\"9.2\",\n              \"frontend_config#0\":\"beast port=80\",\n              \"frontend_type#0\":\"beast\",\n              \"hostname\":\"scvm1\",\n              \"id\":\"glue.scvm1.lzjtbp\",\n              \"kernel_description\":\"#1 SMP PREEMPT_DYNAMIC Wed Aug 16 10:08:14 KST 2023\",\n              \"kernel_version\":\"5.14.0-284.25.2.ablecloud.el9.x86_64\",\n              \"mem_swap_kb\":\"16777212\",\n              \"mem_total_kb\":\"32600252\",\n              \"num_handles\":\"1\",\n              \"os\":\"Linux\",\n              \"pid\":\"2\",\n              \"realm_id\":\"\",\n              \"realm_name\":\"\",\n              \"zone_id\":\"e4de42e3-5687-4aec-a230-0679a51b2e5e\",\n              \"zone_name\":\"default\",\n              \"zonegroup_id\":\"325fa56b-993b-46ab-8f98-857d4735a189\",\n              \"zonegroup_name\":\"default\"\n            },\n            \"task_status\":{\n\n            }\n          }\n        }\n      }\n    }\n  },\n  \"progress_events\":{\n\n  }\n}")

		if err := json.Unmarshal(stdout, &_moldStatus); err != nil {
			utils.FancyHandleError(err)
		}
	}
	_moldStatus.RefreshTime = time.Now()
	CheckMoldWeb()
	CheckMoldService()
	return _moldStatus
}

func CheckMoldWeb() *TypeMoldWebStatus {
	resp, err := http.Get("http://ccvm:8080/client")
	if err != nil {
		fmt.Println(err)
		//controller.AddError(err)
		_moldWebStatus.Code = 500
		return _moldWebStatus
	}
	if resp == nil {
		fmt.Println("response is nil")
		//controller.AddError(errors.New("response is nil"))
		_moldWebStatus.Code = 500
		return _moldWebStatus
	}
	status := resp.StatusCode
	fmt.Println(status)
	fmt.Print(err)
	_moldWebStatus.Code = status
	//body, _ := io.ReadAll(resp.Body)
	//_moldWebStatus.Body = string(body)
	return _moldWebStatus
}

func CheckMoldService() *TypeMoldServiceStatus {
	UpdateStatus()
	if gin.Mode() == gin.ReleaseMode {
		auth, err := goph.Key("/root/.ssh/id_rsa", "")
		if err != nil {
			log.Fatal(err)
		}
		client, err := goph.New("root", "ccvm", auth)
		if err != nil {
			log.Fatal(err)
		}
		defer client.Close()

		var stdout []byte

		cmd, err := client.Command("systemctl", "is-active", "cloudstack-management")
		if err != nil {
			return nil
		}
		stdout, _ = cmd.CombinedOutput()
		output := strings.TrimSpace(string(stdout))
		if output == "active" {
			_moldServiceStatus.MoldWebService.Active = true
		} else {
			_moldServiceStatus.MoldWebService.Active = false
		}

		cmd, err = client.Command("systemctl", "is-enabled", "cloudstack-management")
		if err != nil {
			return nil
		}
		stdout, _ = cmd.CombinedOutput()
		output = strings.TrimSpace(string(stdout))
		if output == "enabled" {
			_moldServiceStatus.MoldWebService.Enabled = true
		} else {
			_moldServiceStatus.MoldWebService.Enabled = false
		}

		cmd, err = client.Command("systemctl", "is-active", "mysqld")
		if err != nil {
			return nil
		}
		stdout, _ = cmd.CombinedOutput()
		output = strings.TrimSpace(string(stdout))
		if output == "active" {
			_moldServiceStatus.MoldDBService.Active = true
		} else {
			_moldServiceStatus.MoldDBService.Active = false
		}

		cmd, err = client.Command("systemctl", "is-enabled", "mysqld")
		if err != nil {
			return nil
		}
		stdout, _ = cmd.CombinedOutput()
		output = strings.TrimSpace(string(stdout))
		if output == "enabled" {
			_moldServiceStatus.MoldDBService.Enabled = true
		} else {
			_moldServiceStatus.MoldDBService.Enabled = false
		}
	} else {
		_moldServiceStatus.MoldDBService.Enabled = false
		_moldServiceStatus.MoldDBService.Active = false
		_moldServiceStatus.MoldWebService.Enabled = false
		_moldServiceStatus.MoldWebService.Active = false

	}
	return _moldServiceStatus
}
