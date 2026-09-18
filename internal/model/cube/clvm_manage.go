package cube

// CLVMManageRequest는 CLVM 디스크 관리 요청 본문이다.
// @name CLVMManageRequest
type CLVMManageRequest struct {
	// action: create-clvm/list-clvm/delete-clvm
	Action string `json:"action" example:"list-clvm"`
	// comma separated disk list for legacy clients
	Disk string `json:"disk,omitempty" example:"/dev/disk/by-id/dm-uuid-mpath-3600..."`
	// disk list
	Disks []string `json:"disks,omitempty" example:"/dev/disk/by-id/dm-uuid-mpath-3600..."`
	// comma separated VG names for delete-clvm
	VGName string `json:"vg_name,omitempty" example:"vg_clvm01"`
	// VG name list for delete-clvm
	VGNames []string `json:"vg_names,omitempty" example:"vg_clvm01,vg_clvm02"`
	// comma separated PV names for delete-clvm
	PVName string `json:"pv_name,omitempty" example:"/dev/sdb1"`
	// PV name list for delete-clvm
	PVNames []string `json:"pv_names,omitempty" example:"/dev/sdb1,/dev/sdc1"`
}

// CLVMManageDisk는 CLVM PV 목록 항목이다.
// @name CLVMManageDisk
type CLVMManageDisk struct {
	VGName   string `json:"vg_name" example:"vg_clvm01"`
	PVName   string `json:"pv_name" example:"/dev/sdb1"`
	PVSize   string `json:"pv_size" example:"100.00GB"`
	UUID     string `json:"uuid" example:"3600140510763a8bd9247d4f6ad8460de"`
	PathMode string `json:"path_mode" example:"multipath"`
	DiskID   string `json:"disk_id" example:"/dev/disk/by-id/wwn-0x600...-part1"`
}

// CLVMManageResponse는 CLVM 디스크 관리 결과이다.
// @name CLVMManageResponse
type CLVMManageResponse struct {
	Code    int    `json:"code" example:"200"`
	Val     any    `json:"val,omitempty"`
	Message string `json:"message,omitempty" example:"ok"`
	Action  string `json:"action,omitempty" example:"list-clvm"`
}
