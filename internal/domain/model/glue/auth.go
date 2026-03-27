package glue

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
	"github.com/ycyun/Cube-API/internal/infra/utils"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"time"
)

type TypeAuthDump struct {
	AuthDump []TypeAuth `json:"auth_dump"`
}
type TypeAuths struct {
	Auth        map[string]*TypeAuth `json:"auth"`
	RefreshTime time.Time            `json:"refreshTime"`
} // @name TypeAuth
type TypeAuth struct {
	Entity string `json:"entity"`
	Key    string `json:"key"`
	Caps   struct {
		Mds string `json:"mds"`
		Mgr string `json:"mgr"`
		Mon string `json:"mon"`
		Osd string `json:"osd"`
	} `json:"caps"`
	RefreshTime time.Time `json:"refreshTime"`
} // @name TypeAuth

var TypeGlueAuth sync.Once

var _glueAuth *TypeAuths

func Auth() *TypeAuths {
	if _glueAuth == nil {
		TypeGlueAuth.Do(
			func() {
				if gin.IsDebugging() {
					fmt.Println("Creating ", reflect.TypeOf(_glueAuth), " now.")
				}
				_glueAuth = &TypeAuths{}
				_glueAuth.Auth = make(map[string]*TypeAuth)
			})
	} else {
		if gin.IsDebugging() {
			fmt.Println("get old ", reflect.TypeOf(_glueAuth), " instance.")
		}
	}
	return _glueAuth
}

func UpdateAuth(user string) bool {
	ret := false
	var tmpAuth []TypeAuth
	if gin.Mode() == gin.ReleaseMode {
		var stdout []byte

		cmd := exec.Command("ceph", "auth", "get", user, "-f", "json")
		stdout, _ = cmd.CombinedOutput()

		if err := json.Unmarshal(stdout, &tmpAuth); err != nil && gin.IsDebugging() {
			utils.FancyHandleError(err)

		}
	} else {

		stdout := []byte("[\n  {\n    \"entity\": \"mds.GlueFS.scvm1.lkkini\",\n    \"key\": \"AQBN6RRmov0eAxAA3O+kldeUthhb8L/nqfvCXg==\",\n    \"caps\": {\n      \"mds\": \"allow\",\n      \"mon\": \"profile mds\",\n      \"osd\": \"allow rw tag cephfs *=*\"\n    }\n  },\n  {\n    \"entity\": \"mds.GlueFS.scvm2.cpgggc\",\n    \"key\": \"AQBP6RRmrPUhExAAYrSwRKAytbFJGtdsNPoeHQ==\",\n    \"caps\": {\n      \"mds\": \"allow\",\n      \"mon\": \"profile mds\",\n      \"osd\": \"allow rw tag cephfs *=*\"\n    }\n  },\n  {\n    \"entity\": \"osd.0\",\n    \"key\": \"AQBOtBRmI/JUEBAA9KU1QNxVGhwL8BFVWFEp/g==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.1\",\n    \"key\": \"AQBOtBRmqaacEhAA4kFQ6Rp4Un6wttXHNQsKAw==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.10\",\n    \"key\": \"AQBZtBRmf8lQABAA1rxjaJ3OBL56k6GFsO3+LA==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.11\",\n    \"key\": \"AQBZtBRm2tD7ABAAYjoU9XMj39JGc31qxAR+bw==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.12\",\n    \"key\": \"AQBctBRmz5h/HhAAEWv888aSv1buSpZ2tnB3JQ==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.13\",\n    \"key\": \"AQBctBRm7YuAIxAACNQV9nbgYsNN7BqZhs/l2g==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.14\",\n    \"key\": \"AQBctBRmDFlWJBAACc0eXELg77Qhqiu3cFvW6w==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.15\",\n    \"key\": \"AQBgtBRma5msAxAAMfiRFHnm1+41fg1IvWIKfg==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.16\",\n    \"key\": \"AQBgtBRmlC9KChAA3FWSFVxGRyWJsW2xedsxkQ==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.17\",\n    \"key\": \"AQBgtBRmv29YCxAAZMe160uo47A5lR28Vs7DEA==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.18\",\n    \"key\": \"AQBjtBRm/rKUKBAAmpbOPjuFLnDXHgVdMU1brw==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.2\",\n    \"key\": \"AQBOtBRmOE/MGBAArqB3i9JlHkvWe4Adu92xrQ==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.3\",\n    \"key\": \"AQBRtBRmkfRiNhAAAVmUtnBf2e2ZfTuStDPIEg==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.4\",\n    \"key\": \"AQBRtBRmbcfZNxAAFwQ94ER6Sy7v/rIVbc6DPA==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.5\",\n    \"key\": \"AQBStBRm6R4NAhAAJSHHif/5lLXzqhf2XIVrSA==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.6\",\n    \"key\": \"AQBVtBRmPwD5GRAAO1b0KgTrDQl6X9SmlvgcEQ==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.7\",\n    \"key\": \"AQBVtBRmzSHqGxAADByr3v+lCVhRlFp1k6v3NA==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.8\",\n    \"key\": \"AQBVtBRm8yc7IBAASt7NRSWETShewMlJdbeLEA==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"osd.9\",\n    \"key\": \"AQBYtBRmBDDwNxAA5O0OR0Jo3Wt9UAEiIBQQCA==\",\n    \"caps\": {\n      \"mgr\": \"allow profile osd\",\n      \"mon\": \"allow profile osd\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"client.admin\",\n    \"key\": \"AQAVshRmjX8YDhAAvxCD9NHFLiBTjeu0VAu4AQ==\",\n    \"caps\": {\n      \"mds\": \"allow *\",\n      \"mgr\": \"allow *\",\n      \"mon\": \"allow *\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"client.bootstrap-mds\",\n    \"key\": \"AQAXshRmZMOqGhAAhpFLaj7rKu9SnejMbfOQdw==\",\n    \"caps\": {\n      \"mon\": \"allow profile bootstrap-mds\"\n    }\n  },\n  {\n    \"entity\": \"client.bootstrap-mgr\",\n    \"key\": \"AQAXshRm9MyqGhAAxIAhWOpByXgL5Os/Ux+F7w==\",\n    \"caps\": {\n      \"mon\": \"allow profile bootstrap-mgr\"\n    }\n  },\n  {\n    \"entity\": \"client.bootstrap-osd\",\n    \"key\": \"AQAXshRmAdWqGhAAUmPoPnxCu3jM4sA5EV8e0w==\",\n    \"caps\": {\n      \"mon\": \"allow profile bootstrap-osd\"\n    }\n  },\n  {\n    \"entity\": \"client.bootstrap-rbd\",\n    \"key\": \"AQAXshRmTdyqGhAAFQA3hHAri7mPSd7Qu/2q7Q==\",\n    \"caps\": {\n      \"mon\": \"allow profile bootstrap-rbd\"\n    }\n  },\n  {\n    \"entity\": \"client.bootstrap-rbd-mirror\",\n    \"key\": \"AQAXshRmWuSqGhAAG136AOcgTVMZW6ReZnSozA==\",\n    \"caps\": {\n      \"mon\": \"allow profile bootstrap-rbd-mirror\"\n    }\n  },\n  {\n    \"entity\": \"client.bootstrap-rgw\",\n    \"key\": \"AQAXshRmM+6qGhAAt+rKUXQIQt4cxyIteWIr6w==\",\n    \"caps\": {\n      \"mon\": \"allow profile bootstrap-rgw\"\n    }\n  },\n  {\n    \"entity\": \"client.ceph-exporter.scvm1\",\n    \"key\": \"AQBFshRm7gcBIhAAyDJEci1P1qhfYW5Xr/GRXQ==\",\n    \"caps\": {\n      \"mgr\": \"allow r\",\n      \"mon\": \"allow r\",\n      \"osd\": \"allow r\"\n    }\n  },\n  {\n    \"entity\": \"client.ceph-exporter.scvm2\",\n    \"key\": \"AQBjshRmO5OIIBAAQiTFSgSJwXhbjM7VMMTlpg==\",\n    \"caps\": {\n      \"mgr\": \"allow r\",\n      \"mon\": \"allow r\",\n      \"osd\": \"allow r\"\n    }\n  },\n  {\n    \"entity\": \"client.ceph-exporter.scvm3\",\n    \"key\": \"AQB2shRmnVpGAxAA1QQT7VGGEV2QEStqt7aa+g==\",\n    \"caps\": {\n      \"mgr\": \"allow r\",\n      \"mon\": \"allow r\",\n      \"osd\": \"allow r\"\n    }\n  },\n  {\n    \"entity\": \"client.crash.scvm1\",\n    \"key\": \"AQBIshRmBNClJxAABigg4zvJKrrOyG5W0wKDGA==\",\n    \"caps\": {\n      \"mgr\": \"profile crash\",\n      \"mon\": \"profile crash\"\n    }\n  },\n  {\n    \"entity\": \"client.crash.scvm2\",\n    \"key\": \"AQB4shRmWYIrKhAASJLHodXAfoytj+aQ1K0Ddw==\",\n    \"caps\": {\n      \"mgr\": \"profile crash\",\n      \"mon\": \"profile crash\"\n    }\n  },\n  {\n    \"entity\": \"client.crash.scvm3\",\n    \"key\": \"AQB6shRm3cC8JxAAorlyBT/YMJkER4tv/1Cjew==\",\n    \"caps\": {\n      \"mgr\": \"profile crash\",\n      \"mon\": \"profile crash\"\n    }\n  },\n  {\n    \"entity\": \"client.iscsi.iscsi.scvm2.kejfwe\",\n    \"key\": \"AQCjx1ZmMZceLxAAp3ow9svBCiyLZrV2H4ydog==\",\n    \"caps\": {\n      \"mgr\": \"allow command \\\"service status\\\"\",\n      \"mon\": \"profile rbd, allow command \\\"osd blocklist\\\", allow command \\\"config-key get\\\" with \\\"key\\\" prefix \\\"iscsi/\\\"\",\n      \"osd\": \"allow rwx\"\n    }\n  },\n  {\n    \"entity\": \"client.iscsi.iscsi.scvm2.rzvekh\",\n    \"key\": \"AQBCx1Zm+4E0OhAAPGoHyOzoCNuqgN7Dwsi94w==\",\n    \"caps\": {\n      \"mgr\": \"allow command \\\"service status\\\"\",\n      \"mon\": \"profile rbd, allow command \\\"osd blocklist\\\", allow command \\\"config-key get\\\" with \\\"key\\\" prefix \\\"iscsi/\\\"\",\n      \"osd\": \"allow rwx\"\n    }\n  },\n  {\n    \"entity\": \"client.iscsi.iscsi.scvm2.zweaws\",\n    \"key\": \"AQCAx1Zmv4huMhAA+oomPB91jDw6XfKVsmaQXg==\",\n    \"caps\": {\n      \"mgr\": \"allow command \\\"service status\\\"\",\n      \"mon\": \"profile rbd, allow command \\\"osd blocklist\\\", allow command \\\"config-key get\\\" with \\\"key\\\" prefix \\\"iscsi/\\\"\",\n      \"osd\": \"allow rwx\"\n    }\n  },\n  {\n    \"entity\": \"client.rgw.rgw.scvm1.zkldzc\",\n    \"key\": \"AQAobxdmBvaFFRAAld8u6K57v1pF2/B0KoL9Wg==\",\n    \"caps\": {\n      \"mgr\": \"allow rw\",\n      \"mon\": \"allow *\",\n      \"osd\": \"allow rwx tag rgw *=*\"\n    }\n  },\n  {\n    \"entity\": \"client.rgw.rgw.scvm2.yvrccq\",\n    \"key\": \"AQAlbxdmxFepDRAABLvaFpTDoHd6kvzdGYS4qw==\",\n    \"caps\": {\n      \"mgr\": \"allow rw\",\n      \"mon\": \"allow *\",\n      \"osd\": \"allow rwx tag rgw *=*\"\n    }\n  },\n  {\n    \"entity\": \"client.rgw.rgw.scvm3.kvgthh\",\n    \"key\": \"AQAibxdmpYZ4BRAAe7CrGQVnhOrfKEpOfk3AvQ==\",\n    \"caps\": {\n      \"mgr\": \"allow rw\",\n      \"mon\": \"allow *\",\n      \"osd\": \"allow rwx tag rgw *=*\"\n    }\n  },\n  {\n    \"entity\": \"mgr.scvm1.vdxotg\",\n    \"key\": \"AQAVshRmNt3iFxAAbFDhFpltpqp4EP60m5BAAA==\",\n    \"caps\": {\n      \"mds\": \"allow *\",\n      \"mon\": \"profile mgr\",\n      \"osd\": \"allow *\"\n    }\n  },\n  {\n    \"entity\": \"mgr.scvm3.ccomul\",\n    \"key\": \"AQCCshRmER+nGBAAk/QRz/AY9ESw5/RmrWUbew==\",\n    \"caps\": {\n      \"mds\": \"allow *\",\n      \"mon\": \"profile mgr\",\n      \"osd\": \"allow *\"\n    }\n  }\n]\n")

		if err := json.Unmarshal(stdout, &tmpAuth); err != nil {
			utils.FancyHandleError(err)

		}
	}

	for _, v := range tmpAuth {
		if strings.Contains(v.Entity, user) {
			_glueAuth.Auth[user] = &v
			ret = true
			_glueAuth.Auth[user].RefreshTime = time.Now()
			if strings.Compare(v.Entity, user) == 0 {
				break
			}
		}
	}
	//if _, ok := _glueAuth.Auth[user]; ok {
	//	_glueAuth.Auth[user] = &tmpAuth
	//} else {
	//	_glueAuth.Auth.add
	//}
	_glueAuth.RefreshTime = time.Now()
	return ret
}
func UpdateAuths() {
	Auth()
	var authdump TypeAuthDump
	if gin.Mode() == gin.ReleaseMode {
		var stdout []byte

		cmd := exec.Command("ceph", "auth", "ls", "-f", "json")
		stdout, _ = cmd.CombinedOutput()

		if err := json.Unmarshal(stdout, &authdump); err != nil && gin.IsDebugging() {
			utils.FancyHandleError(err)

		}
	} else {

		stdout := []byte("{\n  \"auth_dump\" : [ {\n    \"entity\" : \"osd.0\",\n    \"key\" : \"AQBAA35nJVO0JRAA+u+6vnTNF4/rjTDRFgrKwA==\",\n    \"caps\" : {\n      \"mgr\" : \"allow profile osd\",\n      \"mon\" : \"allow profile osd\",\n      \"osd\" : \"allow *\"\n    }\n  }, {\n    \"entity\" : \"osd.1\",\n    \"key\" : \"AQBEA35nr2IEMBAAucOjYUKhS0UulQdIVzZo0g==\",\n    \"caps\" : {\n      \"mgr\" : \"allow profile osd\",\n      \"mon\" : \"allow profile osd\",\n      \"osd\" : \"allow *\"\n    }\n  }, {\n    \"entity\" : \"osd.2\",\n    \"key\" : \"AQBIA35n7YtMJhAAjq6IxRKhQCp1fM+O8qxPrw==\",\n    \"caps\" : {\n      \"mgr\" : \"allow profile osd\",\n      \"mon\" : \"allow profile osd\",\n      \"osd\" : \"allow *\"\n    }\n  }, {\n    \"entity\" : \"client.admin\",\n    \"key\" : \"AQCfAH5n8+zJEhAAWvMzk9GIKdjR3kRuS6pK7w==\",\n    \"caps\" : {\n      \"mds\" : \"allow *\",\n      \"mgr\" : \"allow *\",\n      \"mon\" : \"allow *\",\n      \"osd\" : \"allow *\"\n    }\n  }, {\n    \"entity\" : \"client.bootstrap-mds\",\n    \"key\" : \"AQClAH5nywsNHxAA7F1uUhZeJc2rSE8lClBnMQ==\",\n    \"caps\" : {\n      \"mon\" : \"allow profile bootstrap-mds\"\n    }\n  }, {\n    \"entity\" : \"client.bootstrap-mgr\",\n    \"key\" : \"AQClAH5nKhENHxAAA7ZCBgvp8A19XQQY/QAsXw==\",\n    \"caps\" : {\n      \"mon\" : \"allow profile bootstrap-mgr\"\n    }\n  }, {\n    \"entity\" : \"client.bootstrap-osd\",\n    \"key\" : \"AQClAH5nDBYNHxAA/FRfoa1sUKssyNSBOeGjAw==\",\n    \"caps\" : {\n      \"mon\" : \"allow profile bootstrap-osd\"\n    }\n  }, {\n    \"entity\" : \"client.bootstrap-rbd\",\n    \"key\" : \"AQClAH5nQRsNHxAApaz693JRpCbgo34aBSkG0g==\",\n    \"caps\" : {\n      \"mon\" : \"allow profile bootstrap-rbd\"\n    }\n  }, {\n    \"entity\" : \"client.bootstrap-rbd-mirror\",\n    \"key\" : \"AQClAH5n+R8NHxAAZUo19/W56001Bs9sXENkIg==\",\n    \"caps\" : {\n      \"mon\" : \"allow profile bootstrap-rbd-mirror\"\n    }\n  }, {\n    \"entity\" : \"client.bootstrap-rgw\",\n    \"key\" : \"AQClAH5nWCUNHxAAKH14DzGB0D69GOXZWCKJpA==\",\n    \"caps\" : {\n      \"mon\" : \"allow profile bootstrap-rgw\"\n    }\n  }, {\n    \"entity\" : \"client.ceph-exporter.Rocky\",\n    \"key\" : \"AQDHAH5n3ItQDRAAwE7OitIrHTViqKDhbo1mLw==\",\n    \"caps\" : {\n      \"mgr\" : \"allow r\",\n      \"mon\" : \"profile ceph-exporter\",\n      \"osd\" : \"allow r\"\n    }\n  }, {\n    \"entity\" : \"client.crash.Rocky\",\n    \"key\" : \"AQDIAH5nadQMDRAAzIMkRrHWWdRyquCIjhYeeA==\",\n    \"caps\" : {\n      \"mgr\" : \"profile crash\",\n      \"mon\" : \"profile crash\"\n    }\n  }, {\n    \"entity\" : \"mgr.Rocky.fhhwgx\",\n    \"key\" : \"AQCgAH5nuHApABAAYCMLvF3GrhNTyR2fYNKXhQ==\",\n    \"caps\" : {\n      \"mds\" : \"allow *\",\n      \"mon\" : \"profile mgr\",\n      \"osd\" : \"allow *\"\n    }\n  } ]\n}")

		if err := json.Unmarshal(stdout, &authdump); err != nil {
			utils.FancyHandleError(err)

		}
	}
	_tmpauth := map[string]*TypeAuth{}
	for _, v := range authdump.AuthDump {
		_tmpauth[v.Entity] = &v
		_tmpauth[v.Entity].RefreshTime = time.Now()
	}
	_glueAuth.Auth = _tmpauth
	_glueAuth.RefreshTime = time.Now()
}
func GetAuth(user User) *TypeAuth {
	ret := false
	Auth()
	if user.Username == "" {
		user.Username = "client.admin"
	}
	fmt.Println(_glueAuth)
	if _, ok := _glueAuth.Auth[user.Username]; ok {
		return _glueAuth.Auth[user.Username]
	} else {
		ret = UpdateAuth(user.Username)
	}
	if ret {
		return _glueAuth.Auth[user.Username]
	} else {
		return nil
	}
}

func GetAuths() *TypeAuths {
	Auth()
	return _glueAuth
}
