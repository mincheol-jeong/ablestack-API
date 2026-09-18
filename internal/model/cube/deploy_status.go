package cube

import "time"

const (
	DeployStageClusterPrepare      = "cluster_prepare"
	DeployStageStorageVMDeploy     = "storage_vm_deploy"
	DeployStageStorageVMConfigure  = "storage_vm_configure"
	DeployStageStorageCluster      = "storage_cluster_configure"
	DeployStageHCISharedFile       = "hci_shared_file_configure"
	DeployStageGFSStorage          = "gfs_storage_configure"
	DeployStageLocalStorage        = "local_storage_configure"
	DeployStageCloudVMDeploy       = "cloud_vm_deploy"
	DeployStageCloudVMConfigure    = "cloud_vm_configure"
	DeployStageCloudCluster        = "cloud_cluster_configure"
	DeployStageCloudResource       = "cloud_resource_configure"
	DeployStageMonitoring          = "monitoring_configure"
	DeployStageReady               = "ready"
	DeployStageUnsupportedCluster  = "unsupported_cluster_type"
	DeploySeverityWarning          = "warning"
	DeploySeveritySuccess          = "success"
	DeployStatusTrue               = "true"
	DeployStatusFalse              = "false"
	DeployRuntimeUnknown           = "UNKNOWN"
	DeployRuntimeNotApplicable     = "NOT_APPLICABLE"
	DeployRuntimeHealthOK          = "HEALTH_OK"
	DeployRuntimeHealthWarn        = "HEALTH_WARN"
	DeployRuntimeHealthErr         = "HEALTH_ERR"
	DeployRuntimeHealthErrCluster  = "HEALTH_ERR1"
	DeployRuntimeHealthErrResource = "HEALTH_ERR2"
	DeployRuntimeRunning           = "RUNNING"
	DeployActionDownloadConfigFile = "download_config_file"
	DeployActionPrepareCluster     = "prepare_cluster_config"
	DeployActionDeployStorageVM    = "deploy_storage_vm"
	DeployActionConfigureStorageVM = "configure_storage_vm"
	DeployActionOpenStorageCenter  = "open_storage_center"
	DeployActionConfigureStorage   = "configure_storage_cluster"
	DeployActionConfigureHCIFile   = "configure_hci_shared_file"
	DeployActionConfigureGFS       = "configure_gfs_storage"
	DeployActionConfigureLocal     = "configure_local_storage"
	DeployActionDeployCloudVM      = "deploy_cloud_vm"
	DeployActionConfigureCloudVM   = "configure_cloud_vm"
	DeployActionConfigureCloud     = "configure_cloud_cluster"
	DeployActionConfigureResource  = "configure_cloud_resource"
	DeployActionConfigureMonitor   = "configure_monitoring"
	DeployActionOpenCloudCenter    = "open_cloud_center"
	DeployActionOpenMonitorCenter  = "open_monitoring_center"
	DeployActionRunSecurityPatch   = "run_security_patch"
)

// DeployStatusRaw contains the old UI status values in a server-side snapshot.
// @name DeployStatusRaw
type DeployStatusRaw struct {
	LicenseStatus        string `json:"license_status,omitempty" example:"true"`
	CCFGStatus           string `json:"ccfg_status,omitempty" example:"true"`
	SCVMStatus           string `json:"scvm_status,omitempty" example:"RUNNING"`
	SCVMBootstrapStatus  string `json:"scvm_bootstrap_status,omitempty" example:"true"`
	StorageClusterStatus string `json:"sc_status,omitempty" example:"HEALTH_OK"`
	CloudClusterStatus   string `json:"cc_status,omitempty" example:"HEALTH_OK"`
	CCVMStatus           string `json:"ccvm_status,omitempty" example:"RUNNING"`
	CCVMBootstrapStatus  string `json:"ccvm_bootstrap_status,omitempty" example:"true"`
	WallMonitoringStatus string `json:"wall_monitoring_status,omitempty" example:"true"`
	GFSConfigure         string `json:"gfs_configure,omitempty" example:"false"`
	LocalConfigure       string `json:"local_configure,omitempty" example:"false"`
	SecurityPatchStatus  string `json:"security_patch,omitempty" example:"false"`
}

// DeployStatusWarning describes a non-blocking operational issue.
// @name DeployStatusWarning
type DeployStatusWarning struct {
	Key     string `json:"key" example:"storage_cluster_not_healthy"`
	Message string `json:"message,omitempty" example:"storage cluster is not HEALTH_OK"`
}

// DeployPollingTarget describes whether a dashboard status API should be polled.
// @name DeployPollingTarget
type DeployPollingTarget struct {
	Enabled bool   `json:"enabled" example:"true"`
	Reason  string `json:"reason" example:"cloud center VM is running"`
}

// DeployPollingPolicy controls status-card polling from the deployment snapshot.
// @name DeployPollingPolicy
type DeployPollingPolicy struct {
	StorageVM      DeployPollingTarget `json:"storage_vm"`
	StorageCluster DeployPollingTarget `json:"storage_cluster"`
	GFSResource    DeployPollingTarget `json:"gfs_resource"`
	GFSDisk        DeployPollingTarget `json:"gfs_disk"`
	CloudVM        DeployPollingTarget `json:"cloud_vm"`
	CloudCluster   DeployPollingTarget `json:"cloud_cluster"`
	Mold           DeployPollingTarget `json:"mold"`
}

// DeployStatusData is the summarized deployment state for the UI.
// @name DeployStatusData
type DeployStatusData struct {
	OSType           string                `json:"os_type" example:"ablestack-hci"`
	Stage            string                `json:"stage" example:"cloud_vm_deploy"`
	StageOrder       int                   `json:"stage_order" example:"7"`
	Severity         string                `json:"severity" example:"warning"`
	MessageKey       string                `json:"message_key" example:"cloud_vm_not_deployed"`
	AvailableActions []string              `json:"available_actions,omitempty" example:"deploy_cloud_vm,open_storage_center"`
	Warnings         []DeployStatusWarning `json:"warnings,omitempty"`
	Raw              DeployStatusRaw       `json:"raw"`
	Polling          DeployPollingPolicy   `json:"polling"`
	CheckedAt        time.Time             `json:"checked_at"`
}

// DeployStatusResponse describes the deployment status summary response.
// @name DeployStatusResponse
type DeployStatusResponse struct {
	Code    int              `json:"code" example:"200"`
	Data    DeployStatusData `json:"data"`
	Message string           `json:"message,omitempty" example:"ok"`
}
