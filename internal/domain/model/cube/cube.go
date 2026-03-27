package cube

import (
	"fmt"
	"github.com/ycyun/Cube-API/internal/infra/utils"
	"reflect"
	"sync"
	"time"
)

type TypeCUBE struct {
	Disks       *TypeBlockDevice  `json:"disk"`
	NICs        *TypeNICStatus    `json:"nic"`
	Hosts       *TypeHosts        `json:"hosts"`
	Version     utils.TypeVersion `json:"version"`
	RefreshTime time.Time         `json:"refreshTime"`
}

var lockCUBE sync.Once
var cube *TypeCUBE

func Cube() *TypeCUBE {
	if cube == nil {
		lockCUBE.Do(
			func() {
				fmt.Println("Creating ", reflect.TypeOf(cube), " now.")
				cube = &TypeCUBE{
					Version: utils.TypeVersion{Version: "v5.0.0"},
					Disks:   Disk(),
					NICs:    NIC(),
					Hosts:   Hosts(),
				}
			})
	} else {
		fmt.Println("get old ", reflect.TypeOf(cube), " instance.")
	}

	return cube
}

func (c *TypeCUBE) GetVersion() utils.TypeVersion {
	return c.Version
} // @name version

func (c *TypeCUBE) Update() utils.TypeVersion {
	return c.Version
} // @name version
