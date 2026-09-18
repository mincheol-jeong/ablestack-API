package cube

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	libvirtinfra "ablecloud.io/ablestack-api/internal/infra/libvirt"
	"ablecloud.io/ablestack-api/internal/infra/utils"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"ablecloud.io/ablestack-api/internal/service/clusterconfig"
	"github.com/gin-gonic/gin"
)

type DeployStatusResponse = CubeModel.DeployStatusResponse
type DeployStatusData = CubeModel.DeployStatusData
type DeployStatusRaw = CubeModel.DeployStatusRaw
type DeployStatusWarning = CubeModel.DeployStatusWarning
type DeployPollingTarget = CubeModel.DeployPollingTarget
type DeployPollingPolicy = CubeModel.DeployPollingPolicy

const deployStatusTTL = 5 * time.Second

var deployStatusCache = struct {
	mu      sync.Mutex
	expires time.Time
	data    DeployStatusData
}{}

// GetDeployStatus godoc
//
//	@Summary		Deployment Status
//	@Description	cluster.json, systemProfile, VM, storage, PCS 상태를 조합해 UI용 배포 단계를 반환합니다.
//	@Tags			Cube-Deploy
//	@Accept			x-www-form-urlencoded
//	@Produce		json
//	@Success		200	{object}	CubeModel.DeployStatusResponse
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/deploy/status [get]
func GetDeployStatus(context *gin.Context) {
	data, err := cachedDeployStatus()
	if err != nil {
		context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
			ErrCode: http.StatusInternalServerError,
			Message: err.Error(),
		})
		return
	}
	context.JSON(http.StatusOK, DeployStatusResponse{Code: http.StatusOK, Data: data, Message: "ok"})
}

func cachedDeployStatus() (DeployStatusData, error) {
	now := time.Now()
	deployStatusCache.mu.Lock()
	if now.Before(deployStatusCache.expires) {
		data := deployStatusCache.data
		deployStatusCache.mu.Unlock()
		return data, nil
	}
	deployStatusCache.mu.Unlock()

	data, err := buildDeployStatus()
	if err != nil {
		return DeployStatusData{}, err
	}

	deployStatusCache.mu.Lock()
	deployStatusCache.data = data
	deployStatusCache.expires = time.Now().Add(deployStatusTTL)
	deployStatusCache.mu.Unlock()
	return data, nil
}

func buildDeployStatus() (DeployStatusData, error) {
	cfg, profile, err := loadDeployClusterState()
	if err != nil {
		return DeployStatusData{}, err
	}

	data := DeployStatusData{
		OSType:    strings.TrimSpace(cfg.Type),
		Raw:       initialDeployStatusRaw(cfg, profile),
		CheckedAt: time.Now(),
	}

	if !deployFlagTrue(data.Raw.CCFGStatus) {
		return withDeployStage(data, CubeModel.DeployStageClusterPrepare, "cluster_config_required", CubeModel.DeployActionPrepareCluster), nil
	}

	switch normalizeDeployOSType(cfg.Type) {
	case "ablestack-hci":
		return evaluateHCIDeployStatus(data, cfg, false), nil
	case "ablestack-hci-filesystem":
		return evaluateHCIDeployStatus(data, cfg, true), nil
	case "ablestack-vm":
		return evaluateVMDeployStatus(data, cfg), nil
	case "ablestack-standalone":
		return evaluateStandaloneDeployStatus(data, cfg), nil
	default:
		return withDeployStage(data, CubeModel.DeployStageUnsupportedCluster, "unsupported_cluster_type"), nil
	}
}

func loadDeployClusterState() (*CubeModel.ClusterConfigSection, CubeModel.ClusterSystemProfile, error) {
	root, err := loadClusterJSONRoot()
	if err != nil {
		return nil, CubeModel.ClusterSystemProfile{}, err
	}
	normalized := clusterconfig.NormalizeClusterJSON(root)

	rawCfg, err := json.Marshal(normalized["clusterConfig"])
	if err != nil {
		return nil, CubeModel.ClusterSystemProfile{}, err
	}
	var cfg CubeModel.ClusterConfigSection
	if err := json.Unmarshal(rawCfg, &cfg); err != nil {
		return nil, CubeModel.ClusterSystemProfile{}, err
	}

	rawProfile, err := json.Marshal(normalized["systemProfile"])
	if err != nil {
		return nil, CubeModel.ClusterSystemProfile{}, err
	}
	var profile CubeModel.ClusterSystemProfile
	if err := json.Unmarshal(rawProfile, &profile); err != nil {
		return nil, CubeModel.ClusterSystemProfile{}, err
	}
	return &cfg, profile, nil
}

func initialDeployStatusRaw(cfg *CubeModel.ClusterConfigSection, profile CubeModel.ClusterSystemProfile) DeployStatusRaw {
	raw := DeployStatusRaw{
		LicenseStatus:        strings.TrimSpace(profile.License.Status),
		CCFGStatus:           deployBoolStatus(isDeployClusterConfigReady(cfg)),
		SCVMStatus:           CubeModel.DeployRuntimeUnknown,
		SCVMBootstrapStatus:  normalizeDeployBoolString(profile.Bootstrap.Scvm),
		StorageClusterStatus: CubeModel.DeployRuntimeUnknown,
		CloudClusterStatus:   CubeModel.DeployRuntimeUnknown,
		CCVMStatus:           CubeModel.DeployRuntimeUnknown,
		CCVMBootstrapStatus:  normalizeDeployBoolString(profile.Bootstrap.Ccvm),
		WallMonitoringStatus: normalizeDeployBoolString(profile.Bootstrap.Wall),
		GFSConfigure:         normalizeDeployBoolString(profile.Bootstrap.GFSConfigure),
		LocalConfigure:       normalizeDeployBoolString(profile.Bootstrap.LocalConfigure),
		SecurityPatchStatus:  normalizeDeployBoolString(profile.SecurityPatch.Status),
	}
	if raw.LicenseStatus == "" {
		raw.LicenseStatus = CubeModel.DeployStatusFalse
	}
	if !isDeployHCIType(cfg.Type) {
		raw.SCVMStatus = ""
		raw.SCVMBootstrapStatus = ""
		raw.StorageClusterStatus = ""
	}
	if !isDeployCloudClusterType(cfg.Type) {
		raw.CloudClusterStatus = ""
	}
	if normalizeDeployOSType(cfg.Type) != "ablestack-vm" && normalizeDeployOSType(cfg.Type) != "ablestack-hci-filesystem" {
		raw.GFSConfigure = ""
	}
	if normalizeDeployOSType(cfg.Type) != "ablestack-standalone" {
		raw.LocalConfigure = ""
	}
	return raw
}

func evaluateHCIDeployStatus(data DeployStatusData, cfg *CubeModel.ClusterConfigSection, requiresSharedFile bool) DeployStatusData {
	data.Raw.SCVMStatus = collectDeploySCVMStatus(cfg)

	if !deployFlagTrue(data.Raw.SCVMBootstrapStatus) {
		if !isDeployVMRunning(data.Raw.SCVMStatus) {
			return withDeployStage(data, CubeModel.DeployStageStorageVMDeploy, "storage_vm_not_deployed", CubeModel.DeployActionDownloadConfigFile, CubeModel.DeployActionPrepareCluster, CubeModel.DeployActionDeployStorageVM)
		}
		return withDeployStage(data, CubeModel.DeployStageStorageVMConfigure, "storage_vm_not_configured", CubeModel.DeployActionDownloadConfigFile, CubeModel.DeployActionConfigureStorageVM)
	}

	if requiresSharedFile && !deployFlagTrue(data.Raw.GFSConfigure) {
		return withDeployStage(data, CubeModel.DeployStageHCISharedFile, "hci_shared_file_not_configured", CubeModel.DeployActionOpenStorageCenter, CubeModel.DeployActionConfigureHCIFile)
	}

	if !deployFlagTrue(data.Raw.WallMonitoringStatus) {
		data.Raw.StorageClusterStatus = collectDeployStorageClusterStatus(cfg.Type)
		if !isDeployStorageClusterConfigured(data.Raw.StorageClusterStatus) {
			return withDeployStage(data, CubeModel.DeployStageStorageCluster, "storage_cluster_not_configured", CubeModel.DeployActionOpenStorageCenter, CubeModel.DeployActionConfigureStorage)
		}
	}

	data.Raw.CCVMStatus = collectDeployCCVMStatus(cfg)

	if !deployFlagTrue(data.Raw.CCVMBootstrapStatus) {
		if !isDeployVMRunning(data.Raw.CCVMStatus) {
			return withDeployStage(data, CubeModel.DeployStageCloudVMDeploy, "cloud_vm_not_deployed", CubeModel.DeployActionDownloadConfigFile, CubeModel.DeployActionOpenStorageCenter, CubeModel.DeployActionDeployCloudVM)
		}
		return withDeployStage(data, CubeModel.DeployStageCloudVMConfigure, "cloud_vm_not_configured", CubeModel.DeployActionConfigureCloudVM)
	}

	if !deployFlagTrue(data.Raw.WallMonitoringStatus) {
		data.Raw.CloudClusterStatus = collectDeployCloudClusterStatus(cfg)
		if data.Raw.CloudClusterStatus == CubeModel.DeployRuntimeHealthErrCluster || data.Raw.CloudClusterStatus == CubeModel.DeployRuntimeHealthErr {
			return withDeployStage(data, CubeModel.DeployStageCloudCluster, "cloud_cluster_not_configured", CubeModel.DeployActionConfigureCloud)
		}
		if data.Raw.CloudClusterStatus == CubeModel.DeployRuntimeHealthErrResource || data.Raw.CloudClusterStatus == CubeModel.DeployRuntimeUnknown {
			return withDeployStage(data, CubeModel.DeployStageCloudResource, "cloud_resource_not_configured", CubeModel.DeployActionConfigureResource)
		}
		return withDeployStage(data, CubeModel.DeployStageMonitoring, "monitoring_not_configured", CubeModel.DeployActionOpenStorageCenter, CubeModel.DeployActionOpenCloudCenter, CubeModel.DeployActionConfigureMonitor)
	}

	return readyDeployStatus(data, cfg)
}

func evaluateVMDeployStatus(data DeployStatusData, cfg *CubeModel.ClusterConfigSection) DeployStatusData {
	if !deployFlagTrue(data.Raw.GFSConfigure) {
		return withDeployStage(data, CubeModel.DeployStageGFSStorage, "gfs_storage_not_configured", CubeModel.DeployActionDownloadConfigFile, CubeModel.DeployActionConfigureGFS)
	}

	data.Raw.CCVMStatus = collectDeployCCVMStatus(cfg)

	if !deployFlagTrue(data.Raw.CCVMBootstrapStatus) {
		if !isDeployVMRunning(data.Raw.CCVMStatus) {
			return withDeployStage(data, CubeModel.DeployStageCloudVMDeploy, "cloud_vm_not_deployed", CubeModel.DeployActionDownloadConfigFile, CubeModel.DeployActionDeployCloudVM)
		}
		return withDeployStage(data, CubeModel.DeployStageCloudVMConfigure, "cloud_vm_not_configured", CubeModel.DeployActionConfigureCloudVM)
	}

	if !deployFlagTrue(data.Raw.WallMonitoringStatus) {
		data.Raw.CloudClusterStatus = collectDeployCloudClusterStatus(cfg)
		if data.Raw.CloudClusterStatus == CubeModel.DeployRuntimeHealthErrCluster || data.Raw.CloudClusterStatus == CubeModel.DeployRuntimeHealthErr {
			return withDeployStage(data, CubeModel.DeployStageCloudCluster, "cloud_cluster_not_configured", CubeModel.DeployActionConfigureCloud)
		}
		if data.Raw.CloudClusterStatus == CubeModel.DeployRuntimeHealthErrResource || data.Raw.CloudClusterStatus == CubeModel.DeployRuntimeUnknown {
			return withDeployStage(data, CubeModel.DeployStageCloudResource, "cloud_resource_not_configured", CubeModel.DeployActionConfigureResource)
		}
	}

	if !deployFlagTrue(data.Raw.WallMonitoringStatus) {
		return withDeployStage(data, CubeModel.DeployStageMonitoring, "monitoring_not_configured", CubeModel.DeployActionOpenCloudCenter, CubeModel.DeployActionConfigureMonitor)
	}

	return readyDeployStatus(data, cfg)
}

func evaluateStandaloneDeployStatus(data DeployStatusData, cfg *CubeModel.ClusterConfigSection) DeployStatusData {
	if !deployFlagTrue(data.Raw.LocalConfigure) {
		return withDeployStage(data, CubeModel.DeployStageLocalStorage, "local_storage_not_configured", CubeModel.DeployActionDownloadConfigFile, CubeModel.DeployActionConfigureLocal)
	}

	data.Raw.CCVMStatus = collectDeployCCVMStatus(cfg)

	if !deployFlagTrue(data.Raw.CCVMBootstrapStatus) {
		if !isDeployVMRunning(data.Raw.CCVMStatus) {
			return withDeployStage(data, CubeModel.DeployStageCloudVMDeploy, "cloud_vm_not_deployed", CubeModel.DeployActionDownloadConfigFile, CubeModel.DeployActionDeployCloudVM)
		}
		return withDeployStage(data, CubeModel.DeployStageCloudVMConfigure, "cloud_vm_not_configured", CubeModel.DeployActionConfigureCloudVM)
	}

	if !deployFlagTrue(data.Raw.WallMonitoringStatus) {
		return withDeployStage(data, CubeModel.DeployStageMonitoring, "monitoring_not_configured", CubeModel.DeployActionOpenCloudCenter, CubeModel.DeployActionConfigureMonitor)
	}

	return readyDeployStatus(data, cfg)
}

func readyDeployStatus(data DeployStatusData, cfg *CubeModel.ClusterConfigSection) DeployStatusData {
	if isDeployHCIType(cfg.Type) {
		if data.Raw.SCVMStatus == CubeModel.DeployRuntimeUnknown {
			data.Raw.SCVMStatus = collectDeploySCVMStatus(cfg)
		}
		if data.Raw.StorageClusterStatus == CubeModel.DeployRuntimeUnknown {
			data.Raw.StorageClusterStatus = collectDeployStorageClusterStatus(cfg.Type)
		}
	}
	if isDeployCloudClusterType(cfg.Type) {
		if data.Raw.CloudClusterStatus == CubeModel.DeployRuntimeUnknown {
			data.Raw.CloudClusterStatus = collectDeployCloudClusterStatus(cfg)
		}
	}
	if data.Raw.CCVMStatus == CubeModel.DeployRuntimeUnknown {
		data.Raw.CCVMStatus = collectDeployCCVMStatus(cfg)
	}

	data.Warnings = buildDeployWarnings(data)
	actions := readyDeployActions(cfg.Type, data.Raw)
	data = withDeployStage(data, CubeModel.DeployStageReady, "ready", actions...)
	if len(data.Warnings) > 0 {
		data.Severity = CubeModel.DeploySeverityWarning
	}
	return data
}

func withDeployStage(data DeployStatusData, stage string, messageKey string, actions ...string) DeployStatusData {
	data.Stage = stage
	data.StageOrder = deployStageOrder(stage)
	data.MessageKey = messageKey
	data.AvailableActions = dedupeDeployActions(actions)
	if stage == CubeModel.DeployStageReady {
		data.Severity = CubeModel.DeploySeveritySuccess
	} else {
		data.Severity = CubeModel.DeploySeverityWarning
	}
	data.Polling = buildDeployPollingPolicy(data)
	return data
}

func buildDeployPollingPolicy(data DeployStatusData) DeployPollingPolicy {
	disabled := func(reason string) DeployPollingTarget {
		return DeployPollingTarget{Enabled: false, Reason: reason}
	}
	enabled := func(reason string) DeployPollingTarget {
		return DeployPollingTarget{Enabled: true, Reason: reason}
	}

	policy := DeployPollingPolicy{
		StorageVM:      disabled("storage VM status is not applicable to this product"),
		StorageCluster: disabled("storage cluster status is not applicable to this product"),
		GFSResource:    disabled("GFS resource status is not applicable to this product"),
		GFSDisk:        disabled("GFS disk status is not applicable to this product"),
		CloudVM:        disabled("cloud center VM has not been confirmed running"),
		CloudCluster:   disabled("cloud center cluster prerequisite is not complete"),
		Mold:           disabled("cloud center bootstrap is not complete"),
	}

	if !deployFlagTrue(data.Raw.CCFGStatus) {
		reason := "cluster configuration is not complete"
		policy.StorageVM = disabled(reason)
		policy.StorageCluster = disabled(reason)
		policy.GFSResource = disabled(reason)
		policy.GFSDisk = disabled(reason)
		policy.CloudVM = disabled(reason)
		policy.CloudCluster = disabled(reason)
		policy.Mold = disabled(reason)
		return policy
	}

	osType := normalizeDeployOSType(data.OSType)
	if isDeployHCIType(osType) {
		if isDeployVMRunning(data.Raw.SCVMStatus) || deployFlagTrue(data.Raw.SCVMBootstrapStatus) {
			policy.StorageVM = enabled("storage VM has been confirmed running")
		} else {
			policy.StorageVM = disabled("storage VM has not been confirmed running")
		}
		if deployFlagTrue(data.Raw.SCVMBootstrapStatus) {
			policy.StorageCluster = enabled("storage VM bootstrap is complete")
		} else {
			policy.StorageCluster = disabled("storage VM bootstrap is not complete")
		}
	}

	if osType == "ablestack-vm" || osType == "ablestack-hci-filesystem" {
		if deployFlagTrue(data.Raw.GFSConfigure) {
			policy.GFSResource = enabled("GFS configuration is complete")
			policy.GFSDisk = enabled("GFS configuration is complete")
		} else {
			policy.GFSResource = disabled("GFS configuration is not complete")
			policy.GFSDisk = disabled("GFS configuration is not complete")
		}
	}

	cloudVMReady := isDeployVMRunning(data.Raw.CCVMStatus) || deployFlagTrue(data.Raw.CCVMBootstrapStatus)
	if cloudVMReady {
		policy.CloudVM = enabled("cloud center VM has been confirmed running")
	}

	switch {
	case osType == "ablestack-vm" && deployFlagTrue(data.Raw.GFSConfigure):
		policy.CloudCluster = enabled("GFS configuration is complete")
	case isDeployHCIType(osType) && cloudVMReady:
		policy.CloudCluster = enabled("cloud center VM has been confirmed running")
	}
	if deployFlagTrue(data.Raw.CCVMBootstrapStatus) {
		policy.Mold = enabled("cloud center bootstrap is complete")
	}

	return policy
}

func deployStageOrder(stage string) int {
	switch stage {
	case CubeModel.DeployStageClusterPrepare:
		return 1
	case CubeModel.DeployStageStorageVMDeploy:
		return 2
	case CubeModel.DeployStageStorageVMConfigure:
		return 3
	case CubeModel.DeployStageHCISharedFile:
		return 4
	case CubeModel.DeployStageStorageCluster:
		return 5
	case CubeModel.DeployStageGFSStorage, CubeModel.DeployStageLocalStorage:
		return 6
	case CubeModel.DeployStageCloudVMDeploy:
		return 7
	case CubeModel.DeployStageCloudVMConfigure:
		return 8
	case CubeModel.DeployStageCloudCluster:
		return 9
	case CubeModel.DeployStageCloudResource:
		return 10
	case CubeModel.DeployStageMonitoring:
		return 11
	case CubeModel.DeployStageReady:
		return 12
	default:
		return 0
	}
}

func isDeployClusterConfigReady(cfg *CubeModel.ClusterConfigSection) bool {
	if cfg == nil {
		return false
	}
	osType := normalizeDeployOSType(cfg.Type)
	if osType == "" {
		return false
	}
	if strings.TrimSpace(cfg.CCVM.IP) == "" {
		return false
	}
	if strings.TrimSpace(cfg.MngtNic.CIDR) == "" || strings.TrimSpace(cfg.MngtNic.GW) == "" || strings.TrimSpace(cfg.MngtNic.DNS) == "" {
		return false
	}
	if len(cfg.Hosts) == 0 {
		return false
	}
	if isDeployHCIType(osType) && len(cfg.Hosts) < 3 {
		return false
	}
	for _, host := range cfg.Hosts {
		if strings.TrimSpace(host.Hostname) == "" || strings.TrimSpace(host.Ablecube) == "" {
			return false
		}
		if isDeployHCIType(osType) {
			if strings.TrimSpace(host.AblecubePn) == "" || strings.TrimSpace(host.ScvmMngt) == "" ||
				strings.TrimSpace(host.Scvm) == "" || strings.TrimSpace(host.ScvmCn) == "" {
				return false
			}
		}
	}
	if isDeployCloudClusterType(osType) {
		minPCSHosts := 1
		if isDeployHCIType(osType) {
			minPCSHosts = 3
		}
		return len(cfg.PCSCluster.HostnameList()) >= minPCSHosts
	}
	return true
}

func collectDeploySCVMStatus(cfg *CubeModel.ClusterConfigSection) string {
	if cfg == nil || !isDeployHCIType(cfg.Type) {
		return CubeModel.DeployRuntimeNotApplicable
	}
	host, err := findSelfHost(cfg)
	if err != nil {
		return CubeModel.DeployRuntimeHealthErr
	}
	if !libvirtinfra.IsSocketAvailable() {
		if target := strings.TrimSpace(host.Ablecube); target != "" && !isLocalTarget(target) {
			if proxied, err := proxySCVMStatus(target); err == nil {
				return normalizeDeployVMStatus(proxied.ScvmStatus)
			}
		}
	}
	return normalizeDeployVMStatus(getCachedSCVMStatus(host).ScvmStatus)
}

func collectDeployStorageClusterStatus(osType string) string {
	if !isDeployHCIType(osType) {
		return CubeModel.DeployRuntimeNotApplicable
	}
	resp, err := CubeModel.GlueClusterStatusDetail()
	if err != nil {
		return CubeModel.DeployRuntimeHealthErr
	}
	return normalizeDeployHealthStatus(resp.ClusterStatus)
}

func collectDeployCCVMStatus(cfg *CubeModel.ClusterConfigSection) string {
	if cfg == nil || strings.TrimSpace(cfg.CCVM.IP) == "" {
		return CubeModel.DeployRuntimeHealthErr
	}
	ccvmIP := strings.TrimSpace(cfg.CCVM.IP)
	if isLocalTarget(ccvmIP) {
		localStatus, err := collectCCVMLocalStatus()
		if err != nil {
			return CubeModel.DeployRuntimeHealthErr
		}
		return normalizeDeployVMStatus(localStatus.State)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := callCCVMStatus(client, ccvmIP)
	if err != nil {
		return CubeModel.DeployRuntimeHealthErr
	}
	state := extractDeployCCVMState(resp.Data)
	if state == "" {
		return CubeModel.DeployRuntimeRunning
	}
	return normalizeDeployVMStatus(state)
}

func collectDeployCloudClusterStatus(cfg *CubeModel.ClusterConfigSection) string {
	if cfg == nil || !isDeployCloudClusterType(cfg.Type) {
		return CubeModel.DeployRuntimeNotApplicable
	}

	req := CCVMPCSControlRequest{Action: "status", Resource: pcsDefaultResourceID}
	target, ok := selectPCSExecutionTarget(cfg)
	if !ok || strings.TrimSpace(target.Target) == "" || isLocalTarget(target.Target) || target.Target == "local" {
		return deployCloudClusterStatusFromPCSResponse(runCCVMPCSLocal(req, firstNonEmpty(target.Target, "local")))
	}

	resp, err := callCCVMPCSRemote(target.Target, req)
	if err != nil {
		return CubeModel.DeployRuntimeUnknown
	}
	return deployCloudClusterStatusFromPCSResponse(resp)
}

func deployCloudClusterStatusFromPCSResponse(resp CCVMPCSControlResponse) string {
	if resp.Code != http.StatusOK {
		message := strings.ToLower(strings.TrimSpace(strings.Join([]string{resp.Message, stringifyDeployValue(resp.Val)}, " ")))
		switch {
		case strings.Contains(message, "cluster is not configured") ||
			strings.Contains(message, "cluster not configured") ||
			strings.Contains(message, "cluster not found"):
			return CubeModel.DeployRuntimeHealthErrCluster
		case strings.Contains(message, "resource not found"):
			return CubeModel.DeployRuntimeHealthErrResource
		default:
			return CubeModel.DeployRuntimeUnknown
		}
	}

	value, ok := extractDeployPCSStatusValue(resp.Val)
	if !ok {
		return CubeModel.DeployRuntimeUnknown
	}
	return deployCloudClusterStatusFromPCSValue(value)
}

func extractDeployPCSStatusValue(data any) (CCVMPCSStatusValue, bool) {
	switch value := data.(type) {
	case CCVMPCSStatusValue:
		return value, true
	case map[string]any:
		raw, err := json.Marshal(value)
		if err != nil {
			return CCVMPCSStatusValue{}, false
		}
		var status CCVMPCSStatusValue
		if err := json.Unmarshal(raw, &status); err != nil {
			return CCVMPCSStatusValue{}, false
		}
		return status, true
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return CCVMPCSStatusValue{}, false
		}
		var status CCVMPCSStatusValue
		if err := json.Unmarshal(raw, &status); err != nil {
			return CCVMPCSStatusValue{}, false
		}
		return status, true
	}
}

func deployCloudClusterStatusFromPCSValue(value CCVMPCSStatusValue) string {
	role := strings.TrimSpace(value.Role)
	active := strings.ToLower(strings.TrimSpace(value.Active))
	failed := strings.ToLower(strings.TrimSpace(value.Failed))
	started := strings.TrimSpace(value.Started)
	if strings.EqualFold(role, "Started") && started != "" && active != "false" && failed != "true" {
		return CubeModel.DeployRuntimeHealthOK
	}
	return CubeModel.DeployRuntimeHealthErrResource
}

func stringifyDeployValue(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(raw)
	}
}

func buildDeployWarnings(data DeployStatusData) []DeployStatusWarning {
	warnings := make([]DeployStatusWarning, 0)
	if deployRawRuntimeRelevant(data.Raw.SCVMStatus) && !isDeployVMRunning(data.Raw.SCVMStatus) {
		warnings = append(warnings, DeployStatusWarning{Key: "storage_vm_not_running", Message: "storage center VM is not RUNNING"})
	}
	if deployRawRuntimeRelevant(data.Raw.StorageClusterStatus) && data.Raw.StorageClusterStatus != CubeModel.DeployRuntimeHealthOK {
		warnings = append(warnings, DeployStatusWarning{Key: "storage_cluster_not_healthy", Message: "storage cluster is not HEALTH_OK"})
	}
	if deployRawRuntimeRelevant(data.Raw.CloudClusterStatus) && data.Raw.CloudClusterStatus != CubeModel.DeployRuntimeHealthOK {
		warnings = append(warnings, DeployStatusWarning{Key: "cloud_cluster_not_healthy", Message: "cloud center cluster is not HEALTH_OK"})
	}
	if deployRawRuntimeRelevant(data.Raw.CCVMStatus) && !isDeployVMRunning(data.Raw.CCVMStatus) {
		warnings = append(warnings, DeployStatusWarning{Key: "cloud_vm_not_running", Message: "cloud center VM is not RUNNING"})
	}
	if !deployFlagTrue(data.Raw.SecurityPatchStatus) {
		warnings = append(warnings, DeployStatusWarning{Key: "security_patch_required", Message: "security patch is not completed"})
	}
	return warnings
}

func readyDeployActions(osType string, raw DeployStatusRaw) []string {
	actions := []string{CubeModel.DeployActionOpenCloudCenter, CubeModel.DeployActionOpenMonitorCenter}
	if isDeployHCIType(osType) {
		actions = append([]string{CubeModel.DeployActionOpenStorageCenter}, actions...)
	}
	if !deployFlagTrue(raw.SecurityPatchStatus) {
		actions = append(actions, CubeModel.DeployActionRunSecurityPatch)
	}
	return actions
}

func extractDeployCCVMState(data any) string {
	switch value := data.(type) {
	case CCVMLocalStatus:
		return value.State
	case map[string]any:
		for _, key := range []string{"State", "state"} {
			if raw, ok := value[key]; ok {
				if text, ok := raw.(string); ok {
					return text
				}
			}
		}
	default:
		raw, err := json.Marshal(value)
		if err == nil {
			var status CCVMLocalStatus
			if err := json.Unmarshal(raw, &status); err == nil {
				return status.State
			}
		}
	}
	return ""
}

func normalizeDeployOSType(osType string) string {
	return strings.ToLower(strings.TrimSpace(osType))
}

func isDeployHCIType(osType string) bool {
	switch normalizeDeployOSType(osType) {
	case "ablestack-hci", "ablestack-hci-filesystem":
		return true
	default:
		return false
	}
}

func isDeployCloudClusterType(osType string) bool {
	switch normalizeDeployOSType(osType) {
	case "ablestack-hci", "ablestack-hci-filesystem", "ablestack-vm":
		return true
	default:
		return false
	}
}

func deployBoolStatus(ok bool) string {
	if ok {
		return CubeModel.DeployStatusTrue
	}
	return CubeModel.DeployStatusFalse
}

func normalizeDeployBoolString(value string) string {
	if deployFlagTrue(value) {
		return CubeModel.DeployStatusTrue
	}
	return CubeModel.DeployStatusFalse
}

func deployFlagTrue(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), CubeModel.DeployStatusTrue)
}

func normalizeDeployVMStatus(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return CubeModel.DeployRuntimeHealthErr
	}
	if strings.EqualFold(value, "running") {
		return CubeModel.DeployRuntimeRunning
	}
	return strings.ToUpper(value)
}

func normalizeDeployHealthStatus(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return CubeModel.DeployRuntimeHealthErr
	}
	return strings.ToUpper(value)
}

func isDeployVMRunning(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), CubeModel.DeployRuntimeRunning) ||
		strings.EqualFold(strings.TrimSpace(value), "running")
}

func deployRawRuntimeRelevant(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" &&
		value != CubeModel.DeployRuntimeUnknown &&
		value != CubeModel.DeployRuntimeNotApplicable
}

func isDeployStorageClusterConfigured(value string) bool {
	value = strings.ToUpper(strings.TrimSpace(value))
	return value != "" &&
		value != CubeModel.DeployRuntimeUnknown &&
		value != CubeModel.DeployRuntimeHealthErr
}

func dedupeDeployActions(actions []string) []string {
	out := make([]string, 0, len(actions))
	seen := map[string]struct{}{}
	for _, action := range actions {
		action = strings.TrimSpace(action)
		if action == "" {
			continue
		}
		if _, exists := seen[action]; exists {
			continue
		}
		seen[action] = struct{}{}
		out = append(out, action)
	}
	return out
}
