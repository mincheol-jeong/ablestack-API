package dashboard

import (
	"fmt"
	"github.com/gin-gonic/gin"
	Cube "github.com/ycyun/Cube-API/internal/domain/model/cube"
	Mold "github.com/ycyun/Cube-API/internal/domain/model/mold"
	"reflect"
	"sync"
	"time"
)

type TypeCloudVM struct {
	WebStatus     *Mold.TypeMoldWebStatus     `json:"web-status"`
	ServiceStatus *Mold.TypeMoldServiceStatus `json:"service-status"`
	VMStatus      *Cube.TypeVMStatus          `json:"vm-status"`
	RefreshTime   time.Time                   `json:"refresh-time"`
} // @name TyepCloudVMCluster

var lockCloudVMCluster sync.Once

var _CloudVMCluster *TypeCloudVM

func CloudVMStatus() *TypeCloudVM {
	if _CloudVMCluster == nil {
		lockCloudVMCluster.Do(
			func() {
				if gin.IsDebugging() {
					fmt.Println("Creating ", reflect.TypeOf(_CloudVMCluster), " now.")
				}
				_CloudVMCluster = &TypeCloudVM{}
			})
	} else {
		if gin.IsDebugging() {
			fmt.Println("get old ", reflect.TypeOf(_CloudVMCluster), " instance.")
		}
	}
	return _CloudVMCluster
}

func CloudVMUpdateStatus() *TypeCloudVM {
	CloudVMStatus()

	//c := controller.Init()
	//infos := c.UpdateCCVMInfo()
	//for i, info := range infos.Neighbors {
	//	if info.Info["running"] == true {
	//		tmp, _ := json.Marshal(info.Info)
	//		json.Unmarshal(tmp, &_CloudVMCluster.VMStatus)
	//		fmt.Println(i)
	//	}
	//}
	//_CloudVMCluster.VMStatus = Cube.GetVMStatus("ccvm")
	_CloudVMCluster.ServiceStatus = Mold.CheckMoldService()
	_CloudVMCluster.WebStatus = Mold.CheckMoldWeb()

	_CloudVMCluster.RefreshTime = time.Now()
	return _CloudVMCluster
}
