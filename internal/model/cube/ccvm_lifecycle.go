package cube

// CCVMLifecycleRequest는 Cloud Center VM lifecycle 요청 본문이다.
// @name CCVMLifecycleRequest
type CCVMLifecycleRequest struct {
	// action: initialize/setup/reset/copy/start/stop/restart/delete
	Action string `json:"action" example:"reset"`
	// optional GFS disk path override for reset action
	Disk string `json:"disk,omitempty" example:"/dev/sdb"`
	// stop action에서 virsh shutdown 대신 destroy를 사용할지 여부
	Destroy bool `json:"destroy,omitempty" example:"false"`
	// delete action에서 CCVM 이미지까지 삭제할지 여부
	Purge bool `json:"purge,omitempty" example:"false"`
}

// CCVMLifecycleResponse는 Cloud Center VM lifecycle 결과이다.
// @name CCVMLifecycleResponse
type CCVMLifecycleResponse struct {
	Code    int                  `json:"code" example:"200"`
	Val     string               `json:"val" example:"cloud center reset success"`
	RetName string               `json:"retname,omitempty" example:"CCVM Lifecycle"`
	Message string               `json:"message,omitempty" example:"ok"`
	Action  string               `json:"action,omitempty" example:"reset"`
	OSType  string               `json:"os_type,omitempty" example:"ablestack-hci"`
	Results []ClusterApplyResult `json:"results,omitempty"`
}
