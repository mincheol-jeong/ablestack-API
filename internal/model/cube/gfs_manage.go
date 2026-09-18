package cube

// GFSManageRequest는 GFS/PCS 로컬 작업과 ablecube fan-out 작업 요청 본문이다.
// @name GFSManageRequest
type GFSManageRequest struct {
	// action: init-pcs-cluster/create-gfs/modify-lvm-conf/set-cluster-password/partprobe/lvmdevices-add/resource-cleanup/check-host/configure-stonith/check-stonith/check-ipmi/set-alert/list-gfs/delete-gfs/rescan/extend/scan/add-extend
	Action string `json:"action" example:"init-pcs-cluster"`
	// comma separated disk list for legacy clients
	Disk string `json:"disk,omitempty" example:"/dev/sdb,/dev/sdc"`
	// disk list
	Disks []string `json:"disks,omitempty" example:"/dev/sdb,/dev/sdc"`
	// volume group name for single VG requests
	VGName string `json:"vg_name,omitempty" example:"vg_glue"`
	// logical volume name for single VG requests
	LVName string `json:"lv_name,omitempty" example:"lv_glue"`
	// GFS PCS filesystem resource name
	GFSName string `json:"gfs_name,omitempty" example:"glue-gfs"`
	// GFS mount point
	MountPoint string `json:"mount_point,omitempty" example:"/mnt/glue-gfs"`
	// multiple VG/LV pairs
	VolumeGroups []GFSManageVolumeGroup `json:"volume_groups,omitempty"`
	// PCS cluster name. init-pcs-cluster defaults to cloudcenter_cluster.
	ClusterName string `json:"cluster_name,omitempty" example:"cloudcenter_cluster"`
	// PCS authentication user. init-pcs-cluster defaults to hacluster.
	ClusterUser string `json:"cluster_user,omitempty" example:"hacluster"`
	// PCS authentication password. init-pcs-cluster defaults to password.
	ClusterPassword string `json:"cluster_password,omitempty" example:"password"`
	// set PCS maintenance-mode around extend operations
	NonStopCheck string `json:"non_stop_check,omitempty" example:"false"`
	// enable or disable lvmlockd and devicesfile in lvm.conf. nil defaults to true for modify-lvm-conf.
	UseLVMLockd *bool `json:"use_lvmlockd,omitempty" example:"true"`
	// check-stonith control: check/enable/disable/security-disable/security-enable
	Control string `json:"control,omitempty" example:"check"`
	// STONITH/IPMI devices
	Stonith []GFSManageStonithDevice `json:"stonith,omitempty"`
}

// GFSManageVolumeGroup는 GFS cleanup 대상 VG/LV 쌍이다.
// @name GFSManageVolumeGroup
type GFSManageVolumeGroup struct {
	VGName string `json:"vg_name" example:"vg_glue"`
	LVName string `json:"lv_name" example:"lv_glue"`
}

// GFSManageStonithDevice는 STONITH/IPMI 장비 접속 정보이다.
// @name GFSManageStonithDevice
type GFSManageStonithDevice struct {
	IPAddr   string `json:"ipaddr" example:"192.168.0.10"`
	IPPort   string `json:"ipport,omitempty" example:"623"`
	Login    string `json:"login" example:"admin"`
	Passwd   string `json:"passwd" example:"password"`
	Host     string `json:"host,omitempty" example:"10.10.31.1"`
	Hostname string `json:"hostname,omitempty" example:"ablecube31-1"`
}

// GFSManageTargetResult는 ablecube 대상별 실행 결과이다.
// @name GFSManageTargetResult
type GFSManageTargetResult struct {
	Step     string `json:"step,omitempty" example:"modify_lvm_conf"`
	Hostname string `json:"hostname,omitempty" example:"ablecube31-1"`
	Target   string `json:"target" example:"10.10.31.1"`
	Code     int    `json:"code" example:"200"`
	Message  string `json:"message" example:"ok"`
	Val      any    `json:"val,omitempty"`
}

// GFSManageResponse는 GFS 관리 작업 결과이다.
// @name GFSManageResponse
type GFSManageResponse struct {
	Code    int                     `json:"code" example:"200"`
	Val     any                     `json:"val,omitempty"`
	Message string                  `json:"message,omitempty" example:"ok"`
	Action  string                  `json:"action,omitempty" example:"init-pcs-cluster"`
	Target  string                  `json:"target,omitempty" example:"10.10.31.1"`
	Results []GFSManageTargetResult `json:"results,omitempty"`
}
