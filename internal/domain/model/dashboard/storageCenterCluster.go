package dashboard

import (
	"fmt"
	"github.com/gin-gonic/gin"
	Glue "github.com/ycyun/Cube-API/internal/domain/model/glue"
	"reflect"
	"sync"
	"time"
)

type TypeClusterStatus struct {
	Status  string   `json:"status"`
	Message []string `json:"message"`
}

type TypeDisks struct {
	UP    int `json:"up"`
	Total int `json:"total"`
	In    int `json:"in"`
}

type TypeGateways struct {
	UP     int      `json:"up"`
	Total  int      `json:"total"`
	Quorum []string `json:"quorum"`
}

type TypeStorageCenterCluster struct {
	ClusterStatus TypeClusterStatus `json:"cluster-status"`

	Disks TypeDisks `json:"disks"`

	Gateways TypeGateways `json:"gateways"`

	Daemons *Glue.TypeGlueDaemons `json:"daemons"`

	StoragePools *Glue.TypeGlueStorageSize `json:"storage-pools"`

	RefreshTime time.Time `json:"refresh-time"`
} // @name TypeStorageCenterCluster

var lockStorageCenterCluster sync.Once

var _StorageCenterCluster *TypeStorageCenterCluster

func StorageCenterClusterStatus() *TypeStorageCenterCluster {
	if _StorageCenterCluster == nil {
		lockStorageCenterCluster.Do(
			func() {
				if gin.IsDebugging() {
					fmt.Println("Creating ", reflect.TypeOf(_StorageCenterCluster), " now.")
				}
				_StorageCenterCluster = &TypeStorageCenterCluster{}
			})
	} else {
		if gin.IsDebugging() {
			fmt.Println("get old ", reflect.TypeOf(_StorageCenterCluster), " instance.")
		}
	}
	return _StorageCenterCluster
}

func StorageCenterClusterUpdateStatus() *TypeStorageCenterCluster {
	glue := Glue.Status()
	gluehealth := Glue.Health()
	StorageCenterClusterStatus()
	_StorageCenterCluster.ClusterStatus = TypeClusterStatus{Status: gluehealth.Status, Message: gluehealth.GetMessages()}

	_StorageCenterCluster.Disks = TypeDisks{UP: glue.Osdmap.NumUpOsds, Total: glue.Osdmap.NumOsds, In: glue.Osdmap.NumInOsds}

	_StorageCenterCluster.Gateways = TypeGateways{UP: len(glue.Quorum), Total: glue.Monmap.NumMons, Quorum: glue.QuorumNames}

	_StorageCenterCluster.Daemons = Glue.DaemonList()

	_StorageCenterCluster.StoragePools = Glue.StorageSize()

	_StorageCenterCluster.RefreshTime = time.Now()
	return _StorageCenterCluster
}

//
//type gluePools struct {
//	Pools []string `json:"pools"`
//} // @name GluePools

type Auth struct {
	Token       string `json:"token"`
	Username    string `json:"username"`
	Permissions struct {
		Cephfs            []string `json:"cephfs"`
		ConfigOpt         []string `json:"config-opt"`
		DashboardSettings []string `json:"dashboard-settings"`
		Grafana           []string `json:"grafana"`
		Hosts             []string `json:"hosts"`
		Iscsi             []string `json:"iscsi"`
		Log               []string `json:"log"`
		Manager           []string `json:"manager"`
		Monitor           []string `json:"monitor"`
		NfsGanesha        []string `json:"nfs-ganesha"`
		Osd               []string `json:"osd"`
		Pool              []string `json:"pool"`
		Prometheus        []string `json:"prometheus"`
		RbdImage          []string `json:"rbd-image"`
		RbdMirroring      []string `json:"rbd-mirroring"`
		Rgw               []string `json:"rgw"`
		User              []string `json:"user"`
	} `json:"permissions"`
	PwdExpirationDate interface{} `json:"pwdExpirationDate"`
	Sso               bool        `json:"sso"`
	PwdUpdateRequired bool        `json:"pwdUpdateRequired"`
} // @name Auth
