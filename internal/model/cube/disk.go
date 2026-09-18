package cube

import (
	"sync"
	"time"
)

// DiskDevice는 lsblk JSON 구조와 맞추기 위한 모델입니다.
type DiskDevice struct {
	Name        string       `json:"name"`
	Kname       string       `json:"kname,omitempty"`
	Pkname      *string      `json:"pkname,omitempty"` // 내부 트리 재구성에만 쓰고, 최종 응답에서는 제거합니다.
	Path        *string      `json:"path,omitempty"`
	Mountpoint  *string      `json:"mountpoint,omitempty"`
	DmUUID      *string      `json:"dm_uuid,omitempty"`
	ID          *string      `json:"id,omitempty"`
	RbdPath     *string      `json:"rbd_path,omitempty"`
	Rota        *bool        `json:"rota,omitempty"`
	Model       *string      `json:"model,omitempty"`
	Size        *string      `json:"size,omitempty"`
	State       *string      `json:"state,omitempty"`
	Group       *string      `json:"group,omitempty"`
	Type        *string      `json:"type,omitempty"`
	Tran        *string      `json:"tran,omitempty"`
	Subsystems  *string      `json:"subsystems,omitempty"`
	Vendor      *string      `json:"vendor,omitempty"`
	Wwn         *string      `json:"wwn,omitempty"`
	UUID        string       `json:"uuid,omitempty"`
	PathMode    string       `json:"path_mode,omitempty"`
	InUse       bool         `json:"in_use,omitempty"`
	InUseReason string       `json:"in_use_reason,omitempty"`
	SinglePath  []DiskDevice `json:"single_path,omitempty"`
	Children    []DiskDevice `json:"children,omitempty"`
}

// TypeBlockDevice는 내부 캐시/처리용 구조입니다.
type TypeBlockDevice struct {
	Blockdevices    []DiskDevice        `json:"blockdevices"`
	RaidControllers []map[string]string `json:"raidcontrollers,omitempty"`
	RefreshTime     time.Time           `json:"-"`
	mu              *sync.RWMutex
} // @name TypeBlockDevice

// DiskResponse는 “API 응답 전용 DTO”입니다.
// (TypeBlockDevice를 그대로 반환하면 내부 필드가 섞이거나 스키마가 꼬일 수 있어서 분리합니다.)
type DiskResponse struct {
	Blockdevices    []DiskDevice        `json:"blockdevices"`
	RaidControllers []map[string]string `json:"raidcontrollers,omitempty"`
	RefreshTime     string              `json:"refresh_time,omitempty"`
} // @name DiskResponse

// DiskDetailResponse는 action=detail 응답 DTO입니다.
type DiskDetailResponse struct {
	Type        string                      `json:"type"`
	Devices     map[string]DiskDetailDevice `json:"devices"`
	RefreshTime string                      `json:"refresh_time,omitempty"`
} // @name DiskDetailResponse

// DiskDetailDevice는 multipath/single 상세 정보를 담습니다.
type DiskDetailDevice struct {
	MultipathID   []string `json:"multipath_id,omitempty"`
	MultipathName []string `json:"multipath_name,omitempty"`
	SingleID      []string `json:"single_id,omitempty"`
	SingleName    []string `json:"single_name,omitempty"`
	Scsi          []string `json:"scsi,omitempty"`
	Wwn           []string `json:"wwn,omitempty"`
}

// FlatViewResponse는 view=flat 일 때 응답입니다.
type FlatViewResponse struct {
	Devices         []FlatDiskItem      `json:"devices"`
	RaidControllers []map[string]string `json:"raidcontrollers,omitempty"`
	RefreshTime     string              `json:"refresh_time,omitempty"`
} // @name FlatViewResponse

// FlatDiskItem은 트리 구조를 평탄화한 단일 행입니다.
type FlatDiskItem struct {
	Name    string  `json:"name"`
	Kname   string  `json:"kname,omitempty"`
	Path    *string `json:"path,omitempty"`
	ID      *string `json:"id,omitempty"`
	RbdPath *string `json:"rbd_path,omitempty"`
	Type    *string `json:"type,omitempty"`
	Parent  string  `json:"parent,omitempty"`
	Depth   int     `json:"depth"`
}

var lockBlockDevice sync.Once
var _BlockDevice *TypeBlockDevice

func Disk() *TypeBlockDevice {
	lockBlockDevice.Do(func() {
		_BlockDevice = &TypeBlockDevice{mu: &sync.RWMutex{}}
	})
	return _BlockDevice
}

func (d *TypeBlockDevice) Lock() {
	if d == nil || d.mu == nil {
		return
	}
	d.mu.Lock()
}

func (d *TypeBlockDevice) Unlock() {
	if d == nil || d.mu == nil {
		return
	}
	d.mu.Unlock()
}

func (d *TypeBlockDevice) RLock() {
	if d == nil || d.mu == nil {
		return
	}
	d.mu.RLock()
}

func (d *TypeBlockDevice) RUnlock() {
	if d == nil || d.mu == nil {
		return
	}
	d.mu.RUnlock()
}
