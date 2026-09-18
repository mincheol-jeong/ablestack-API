package cube

import "time"

const (
	DeployRunStatusQueued    = "queued"
	DeployRunStatusRunning   = "running"
	DeployRunStatusSucceeded = "succeeded"
	DeployRunStatusFailed    = "failed"

	DeployRunStepStatusPending   = "pending"
	DeployRunStepStatusRunning   = "running"
	DeployRunStepStatusSucceeded = "succeeded"
	DeployRunStepStatusFailed    = "failed"
	DeployRunStepStatusSkipped   = "skipped"

	DeployRunStepLicenseApply      = "license_apply"
	DeployRunStepClusterApply      = "cluster_apply"
	DeployRunStepSCVMPrepare       = "scvm_prepare"
	DeployRunStepSCVMBootstrap     = "scvm_bootstrap"
	DeployRunStepRBDPrepare        = "rbd_prepare"
	DeployRunStepStoragePrepare    = "storage_prepare"
	DeployRunStepLocalPrepare      = "local_prepare"
	DeployRunStepCCVMPrepare       = "ccvm_prepare"
	DeployRunStepCCVMBootstrap     = "ccvm_bootstrap"
	DeployRunStepMonitoringPrepare = "monitoring_prepare"
	DeployRunStepSystemProfile     = "system_profile"
)

// LicenseApplyRequest describes cluster-wide license fan-out.
// @name LicenseApplyRequest
type LicenseApplyRequest struct {
	// action: register/status
	Action string `json:"action,omitempty" example:"register"`
	// license file content (base64). If empty for register, current local license is reused.
	LicenseContent string `json:"license_content,omitempty" example:"BASE64_CONTENT"`
	// host-specific license content keyed by hostname, role name, target IP, index, or scvmN name.
	Licenses map[string]string `json:"licenses,omitempty"`
	// license filename used when saving on targets
	Filename string `json:"filename,omitempty" example:"license.lic"`
	// target roles: ablecube/scvm/ccvm/all. Defaults to ablecube for backward compatibility.
	Roles []string `json:"roles,omitempty" example:"ablecube,scvm,ccvm"`
	// explicit target IPs. If set, roles are ignored.
	Targets []string `json:"targets,omitempty" example:"10.10.31.1,10.10.31.2"`
	// explicit target hostnames from cluster.json hosts[].hostname. ccvm can be selected with "ccvm".
	TargetHostnames []string `json:"target_hostnames,omitempty" example:"ablecube31-1,ablecube31-2"`
	// wait for the CCVM PCS resource or local libvirt domain before registering its license.
	WaitForReady bool `json:"wait_for_ready,omitempty" example:"true"`
}

// LicenseApplyTargetResult is a per-host license fan-out result.
// @name LicenseApplyTargetResult
type LicenseApplyTargetResult struct {
	Role     string `json:"role,omitempty" example:"ablecube"`
	Hostname string `json:"hostname,omitempty" example:"ablecube31-1"`
	Target   string `json:"target" example:"10.10.31.1"`
	Code     int    `json:"code" example:"200"`
	Message  string `json:"message,omitempty" example:"ok"`
	Attempts int    `json:"attempts,omitempty" example:"2"`
	Val      any    `json:"val,omitempty"`
}

// LicenseApplyResponse describes cluster-wide license fan-out result.
// @name LicenseApplyResponse
type LicenseApplyResponse struct {
	Code    int                        `json:"code" example:"200"`
	Message string                     `json:"message,omitempty" example:"license apply success"`
	Results []LicenseApplyTargetResult `json:"results,omitempty"`
}

// DeployRunRequest starts an all-in-one deployment orchestration job.
// @name DeployRunRequest
type DeployRunRequest struct {
	// mode: all/partial. all is the default.
	Mode string `json:"mode,omitempty" example:"all"`
	// run only these steps.
	Only []string `json:"only,omitempty" example:"license_apply,cluster_apply,scvm_bootstrap,rbd_prepare,storage_prepare,ccvm_bootstrap"`
	// skip these steps.
	Skip []string `json:"skip,omitempty" example:"local_prepare"`
	// allow reset/destructive future steps. Current implementation does not run destructive reset by default.
	ForceReset bool `json:"force_reset,omitempty" example:"false"`

	// license file content (base64). If empty, current local license is reused by license_apply.
	LicenseContent string `json:"license_content,omitempty" example:"BASE64_CONTENT"`
	// host-specific license content keyed by hostname, ablecube IP, index, or scvmN name.
	Licenses map[string]string `json:"licenses,omitempty"`
	// license filename used when saving on targets.
	LicenseFilename string `json:"license_filename,omitempty" example:"license.lic"`
	// scvm_bootstrap/ccvm_bootstrap 단계에서 VM 내부 /root/bootstrap.sh를 실행할지 여부. 기본값은 true.
	RunBootstrapScript *bool `json:"run_bootstrap_script,omitempty" example:"true"`
	// SCVM 준비/bootstrap 대상을 cluster.json hosts[].hostname으로 제한한다.
	TargetHostnames []string `json:"target_hostnames,omitempty" example:"ablecube31-4"`
	// 신규 SCVM을 기존 Ceph 클러스터에 조인한다. 초기 Ceph bootstrap은 실행하지 않는다.
	JoinExistingSCVMCluster bool `json:"join_existing_scvm_cluster,omitempty" example:"true"`

	// cluster apply request. If omitted, cluster_apply is skipped unless explicitly selected.
	Cluster *ClusterApplyRequest `json:"cluster,omitempty"`
	// host-specific SCVM XML requests keyed by hostname, ablecube IP, index, or scvmN name.
	SCVMByHost map[string]SCVMXMLCreateRequest `json:"scvm_by_host,omitempty"`
	// GFS/PCS storage request for VM/HCI filesystem flows.
	GFS *GFSManageRequest `json:"gfs,omitempty"`
	// RBD image creation and host rbdmap request for HCI filesystem flows.
	RBD *RBDManageRequest `json:"rbd,omitempty"`
	// local storage request for standalone flows.
	Local *LocalManageRequest `json:"local,omitempty"`
	// optional CCVM cloud-init service-network override.
	CCVMCloudInit *CCVMCloudInitCreateRequest `json:"ccvm_cloudinit,omitempty"`
	// CCVM XML request.
	CCVMXML *CCVMXMLCreateRequest `json:"ccvm_xml,omitempty"`
	// optional CCVM lifecycle request. Defaults to setup when ccvm_xml is provided.
	CCVMLifecycle *CCVMLifecycleRequest `json:"ccvm_lifecycle,omitempty"`
	// CCVM Wall monitoring configuration. action defaults to configure in deploy/run.
	Monitoring *CCVMMonitoringConfigRequest `json:"monitoring,omitempty"`
	// update systemProfile flags for successfully executed steps. Defaults to true.
	UpdateSystemProfile *bool `json:"update_system_profile,omitempty" example:"true"`
}

// DeployRunStepResult describes one deployment job step result.
// @name DeployRunStepResult
type DeployRunStepResult struct {
	Name       string    `json:"name" example:"cluster_apply"`
	Status     string    `json:"status" example:"succeeded"`
	Message    string    `json:"message,omitempty" example:"ok"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	DurationMS int64     `json:"duration_ms,omitempty" example:"1200"`
	Output     any       `json:"output,omitempty"`
}

// DeployRunJob describes an in-memory deployment orchestration job.
// @name DeployRunJob
type DeployRunJob struct {
	JobID       string                `json:"job_id" example:"018f9c39-7ca9-7f4e-9a04-f6373d8f7e2b"`
	Status      string                `json:"status" example:"running"`
	Mode        string                `json:"mode,omitempty" example:"all"`
	OSType      string                `json:"os_type,omitempty" example:"ablestack-hci"`
	CurrentStep string                `json:"current_step,omitempty" example:"scvm_prepare"`
	Message     string                `json:"message,omitempty" example:"ok"`
	CreatedAt   time.Time             `json:"created_at"`
	StartedAt   time.Time             `json:"started_at,omitempty"`
	FinishedAt  time.Time             `json:"finished_at,omitempty"`
	Steps       []DeployRunStepResult `json:"steps"`
}

// DeployRunStartResponse is returned when a deployment job starts.
// @name DeployRunStartResponse
type DeployRunStartResponse struct {
	Code    int                   `json:"code" example:"202"`
	JobID   string                `json:"job_id" example:"018f9c39-7ca9-7f4e-9a04-f6373d8f7e2b"`
	Status  string                `json:"status" example:"queued"`
	Message string                `json:"message,omitempty" example:"deploy job started"`
	Steps   []DeployRunStepResult `json:"steps,omitempty"`
}

// DeployRunJobResponse is returned for one deployment job.
// @name DeployRunJobResponse
type DeployRunJobResponse struct {
	Code    int          `json:"code" example:"200"`
	Job     DeployRunJob `json:"job"`
	Message string       `json:"message,omitempty" example:"ok"`
}

// DeployRunJobListResponse is returned for deployment job listing.
// @name DeployRunJobListResponse
type DeployRunJobListResponse struct {
	Code    int            `json:"code" example:"200"`
	Jobs    []DeployRunJob `json:"jobs"`
	Message string         `json:"message,omitempty" example:"ok"`
}
