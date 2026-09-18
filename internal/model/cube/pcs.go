package cube

// CCVMPCSControlRequest는 CCVM Pacemaker/PCS 제어 요청 본문이다.
// @name CCVMPCSControlRequest
type CCVMPCSControlRequest struct {
	// action: setup/config/create/enable/disable/move/cleanup/status/remove/destroy/stop/sync/ccvm-status
	Action string `json:"action" example:"status"`
	// cluster name for config action
	Cluster string `json:"cluster,omitempty" example:"cloudcenter_cluster"`
	// hostnames for config action
	Hosts []string `json:"hosts,omitempty" example:"ablecube31-1,ablecube31-2,ablecube31-3"`
	// PCS resource name. 비어 있으면 cloudcenter_res를 사용한다.
	Resource string `json:"resource,omitempty" example:"cloudcenter_res"`
	// CCVM libvirt XML path for create action
	XML string `json:"xml,omitempty" example:"/etc/libvirt/qemu/ccvm.xml"`
	// target hostname/IP for move action
	Target string `json:"target,omitempty" example:"ablecube31-2"`
	// cluster token time for sync action
	Time string `json:"time,omitempty" example:"3000"`
	// setup 내부 호출에서 PCS 리소스를 생성만 하고 시작하지 않음
	CreateOnly bool `json:"create_only,omitempty" swaggerignore:"true"`
	// create 내부 호출에서 PCS 리소스를 비활성 상태로 생성
	Disabled bool `json:"disabled,omitempty" swaggerignore:"true"`
}

// CCVMPCSNodeStatus는 pcs status xml의 노드 상태이다.
// @name CCVMPCSNodeStatus
type CCVMPCSNodeStatus struct {
	Host             string `json:"host"`
	Online           string `json:"online"`
	ResourcesRunning string `json:"resources_running"`
	Standby          string `json:"standby"`
	StandbyOnfail    string `json:"standby_onfail"`
	Maintenance      string `json:"maintenance"`
	Pending          string `json:"pending"`
	Unclean          string `json:"unclean"`
	Shutdown         string `json:"shutdown"`
	ExpectedUp       string `json:"expected_up"`
	IsDC             string `json:"is_dc"`
	Type             string `json:"type"`
}

// CCVMPCSStatusValue는 status action의 val 구조이다.
// @name CCVMPCSStatusValue
type CCVMPCSStatusValue struct {
	ClusteredHost []string            `json:"clustered_host"`
	Nodes         []CCVMPCSNodeStatus `json:"nodes"`
	Started       string              `json:"started,omitempty"`
	Role          string              `json:"role"`
	Active        string              `json:"active"`
	Blocked       string              `json:"blocked"`
	Failed        string              `json:"failed"`
}

// CCVMPCSControlResponse는 CCVM Pacemaker/PCS 제어 결과이다.
// @name CCVMPCSControlResponse
type CCVMPCSControlResponse struct {
	Code    int    `json:"code" example:"200"`
	Val     any    `json:"val,omitempty"`
	RetName string `json:"retname,omitempty"`
	Message string `json:"message,omitempty"`
	Action  string `json:"action,omitempty" example:"status"`
	Target  string `json:"target,omitempty" example:"10.10.31.1"`
}
