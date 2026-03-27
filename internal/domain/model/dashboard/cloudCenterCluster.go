package dashboard

import (
	"fmt"
	"github.com/gin-gonic/gin"
	Glue "github.com/ycyun/Cube-API/internal/domain/model/glue"
	"reflect"
	"sync"
	"time"
)

type TypeCloudCenterCluster struct {
	ClusterStatus TypeClusterStatus `json:"cluster-status"`

	Disks TypeDisks `json:"disks"`

	Gateways TypeGateways `json:"gateways"`

	Daemons *Glue.TypeGlueDaemons `json:"daemons"`

	StoragePools *Glue.TypeGlueStorageSize `json:"storage-pools"`

	RefreshTime time.Time `json:"refresh-time"`
} // @name TyepCloudCenterCluster

var lockCloudCenterCluster sync.Once

var _CloudCenterCluster *TypeCloudCenterCluster

func CloudCenterClusterStatus() *TypeCloudCenterCluster {
	if _CloudCenterCluster == nil {
		lockCloudCenterCluster.Do(
			func() {
				if gin.IsDebugging() {
					fmt.Println("Creating ", reflect.TypeOf(_CloudCenterCluster), " now.")
				}
				_CloudCenterCluster = &TypeCloudCenterCluster{}
			})
	} else {
		if gin.IsDebugging() {
			fmt.Println("get old ", reflect.TypeOf(_CloudCenterCluster), " instance.")
		}
	}
	return _CloudCenterCluster
}

func CloudCenterClusterUpdateStatus() *TypeCloudCenterCluster {
	glue := Glue.Status()
	gluehealth := Glue.Health()
	CloudCenterClusterStatus()
	_CloudCenterCluster.ClusterStatus = TypeClusterStatus{Status: gluehealth.Status, Message: gluehealth.GetMessages()}

	_CloudCenterCluster.Disks = TypeDisks{UP: glue.Osdmap.NumUpOsds, Total: glue.Osdmap.NumOsds, In: glue.Osdmap.NumInOsds}

	_CloudCenterCluster.Gateways = TypeGateways{UP: len(glue.Quorum), Total: glue.Monmap.NumMons, Quorum: glue.QuorumNames}

	_CloudCenterCluster.Daemons = Glue.DaemonList()

	_CloudCenterCluster.StoragePools = Glue.StorageSize()

	_CloudCenterCluster.RefreshTime = time.Now()
	return _CloudCenterCluster
}
