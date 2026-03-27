package cube

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"
)

type TypeHost struct {
	IP        string   `json:"ip"`
	HostNames []string `json:"hostnames"`
}

type TypeHosts struct {
	Host        []TypeHost `json:"host"`
	RefreshTime time.Time  `json:"refresh_time"`
}

var lockHosts sync.Once
var _Hosts *TypeHosts

func Hosts() *TypeHosts {
	if _Hosts == nil {
		lockHosts.Do(
			func() {
				fmt.Println("Creating ", reflect.TypeOf(_Hosts), " now.")
				_Hosts = &TypeHosts{}
			})
	} else {
		fmt.Println("get old ", reflect.TypeOf(_Hosts), " instance.")
	}

	return _Hosts
}

// Get godoc
//
//	@Summary		Show List of Hosts
//	@Description	Cube의 Hosts 파일의 목록을 보여줍니다.
//	@Tags			API, CUBE
//	@Accept			x-www-form-urlencoded
//	@Produce		json
//	@Success		200	{object}	TypeHosts
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		404	{object}	HTTP404NotFound
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/hosts [get]
func (host *TypeHosts) Get(context *gin.Context) {

	context.IndentedJSON(http.StatusOK, host)
}

func (host *TypeHosts) Update() {
	var stdout []byte
	var err error
	var hosts []TypeHost
	if gin.Mode() == gin.ReleaseMode {
		stdout, err = os.ReadFile("/etc/hosts")
		if err != nil {
			fmt.Println(err)
		}
	} else {
		stdout = []byte("#comment\n##commentttt\n127.0.0.1\tlocalhost localhost.localdomain localhost4 localhost4.localdomain4\n::1\tlocalhost localhost.localdomain localhost6 localhost6.localdomain6\n10.10.33.10\tccvm-mngt ccvm\n10.10.33.1\tablecube1 ablecube\n10.10.33.11\tscvm1-mngt scvm-mngt\n100.100.33.1\tablecube1-pn ablecube-pn\n100.100.33.11\tscvm1 scvm\n100.200.33.11\tscvm1-cn scvm-cn\n10.10.33.2\tablecube2\n10.10.33.12\tscvm2-mngt\n100.100.33.2\tablecube2-pn\n100.100.33.12\tscvm2\n100.200.33.12\tscvm2-cn\n10.10.33.3\tablecube3\n10.10.33.13\tscvm3-mngt\n100.100.33.3\tablecube3-pn\n100.100.33.13\tscvm3\n100.200.33.13\tscvm3-cn\n10.10.33.4\tablecube4\n10.10.33.14\tscvm4-mngt\n100.100.33.4\tablecube4-pn\n100.100.33.14\tscvm4\n100.200.33.14\tscvm4-cn\n10.10.33.5\tablecube5\n10.10.33.15\tscvm5-mngt\n100.100.33.5\tablecube5-pn\n100.100.33.15\tscvm5\n100.200.33.15\tscvm5-cn\n###comment")
	}

	lines := strings.Split(string(stdout), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") == false && strings.TrimSpace(line) != "" {
			items := strings.Fields(line)
			host := TypeHost{items[0], items[1:]}
			hosts = append(hosts, host)
		}

	}
	host.Host = hosts
	host.RefreshTime = time.Now()
}

func UpdateHosts() {
	var stdout []byte
	var err error
	var hosts []TypeHost
	if gin.Mode() == gin.ReleaseMode {
		stdout, err = os.ReadFile("/etc/hosts")
		if err != nil {
			fmt.Println(err)
		}
	} else {
		stdout = []byte("#comment\n##commentttt\n127.0.0.1\tlocalhost localhost.localdomain localhost4 localhost4.localdomain4\n::1\tlocalhost localhost.localdomain localhost6 localhost6.localdomain6\n10.10.33.10\tccvm-mngt ccvm\n10.10.33.1\tablecube1 ablecube\n10.10.33.11\tscvm1-mngt scvm-mngt\n100.100.33.1\tablecube1-pn ablecube-pn\n100.100.33.11\tscvm1 scvm\n100.200.33.11\tscvm1-cn scvm-cn\n10.10.33.2\tablecube2\n10.10.33.12\tscvm2-mngt\n100.100.33.2\tablecube2-pn\n100.100.33.12\tscvm2\n100.200.33.12\tscvm2-cn\n10.10.33.3\tablecube3\n10.10.33.13\tscvm3-mngt\n100.100.33.3\tablecube3-pn\n100.100.33.13\tscvm3\n100.200.33.13\tscvm3-cn\n10.10.33.4\tablecube4\n10.10.33.14\tscvm4-mngt\n100.100.33.4\tablecube4-pn\n100.100.33.14\tscvm4\n100.200.33.14\tscvm4-cn\n10.10.33.5\tablecube5\n10.10.33.15\tscvm5-mngt\n100.100.33.5\tablecube5-pn\n100.100.33.15\tscvm5\n100.200.33.15\tscvm5-cn\n###comment")
	}

	lines := strings.Split(string(stdout), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") == false && strings.TrimSpace(line) != "" {
			items := strings.Fields(line)
			host := TypeHost{items[0], items[1:]}
			hosts = append(hosts, host)
		}

	}
	_Hosts.Host = hosts
	_Hosts.RefreshTime = time.Now()
}
