package cube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ablecloud.io/ablestack-api/internal/infra/utils"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"github.com/gin-gonic/gin"
)

type GFSManageRequest = CubeModel.GFSManageRequest
type GFSManageResponse = CubeModel.GFSManageResponse
type GFSManageTargetResult = CubeModel.GFSManageTargetResult
type GFSManageVolumeGroup = CubeModel.GFSManageVolumeGroup
type GFSManageStonithDevice = CubeModel.GFSManageStonithDevice

const (
	gfsManageLocalHeader      = "X-Cube-GFS-Manage-Local"
	gfsManageCommandTimeout   = 5 * time.Minute
	gfsManageRemoteTimeout    = 15 * time.Minute
	gfsManageShortTimeout     = 30 * time.Second
	gfsManageLVMConfPath      = "/etc/lvm/lvm.conf"
	gfsManageAlertLogPath     = "/var/log/pcmk_alert_file.log"
	gfsManageAlertDetailPath  = "/var/log/pcmk_alert_detail.log"
	gfsManageHCIFilesystem    = "ablestack-hci-filesystem"
	gfsManageDefaultCluster   = "cloudcenter_cluster"
	gfsManageDefaultPCSUser   = "hacluster"
	gfsManageDefaultPCSPass   = "password"
	gfsManageResourceCleanup  = "resource-cleanup"
	gfsManagePrepareAlertFile = "prepare-alert-file"
	gfsManageSetPCSPassword   = "set-cluster-password"
	gfsManagePrepareGFSNode   = "prepare-gfs-node"
	gfsManageClusterReadyWait = 90 * time.Second
	gfsManageLockingSettle    = 25 * time.Second
	gfsManageReadyPoll        = 3 * time.Second
	gfsManageReadyChecks      = 2
)

type gfsManageTarget struct {
	Hostname string
	Target   string
}

type gfsManageVGReport struct {
	Report []struct {
		VG []struct {
			VGName string `json:"vg_name"`
		} `json:"vg"`
	} `json:"report"`
}

type gfsManageLSBLKPayload struct {
	Blockdevices []gfsManageBlockDevice `json:"blockdevices"`
}

type gfsManageBlockDevice struct {
	Name       string                 `json:"name"`
	Path       string                 `json:"path"`
	Type       string                 `json:"type"`
	Mountpoint string                 `json:"mountpoint"`
	Children   []gfsManageBlockDevice `json:"children,omitempty"`
}

type gfsManageDevicePaths struct {
	Disks      []string
	Partitions []string
	LVPaths    []string
	MapNames   []string
	BlockNames []string
}

type gfsManageFormatSettings struct {
	JournalSizeMB       int
	ResourceGroupSizeMB int
}

// GFSManage godoc
//
//	@Summary		GFS Manage
//	@Description	GFS/PCS 로컬 작업을 수행하거나 cluster.json hosts[].ablecube 대상 API로 fan-out 합니다. SSH는 사용하지 않습니다.
//	@Tags			Cube-GFS
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CubeModel.GFSManageRequest	true	"gfs manage request"
//	@Success		200	{object}	CubeModel.GFSManageResponse
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/gfs/manage [post]
func GFSManage(context *gin.Context) {
	var req GFSManageRequest
	if err := context.ShouldBindJSON(&req); err != nil {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: "invalid request",
		})
		return
	}

	if err := normalizeGFSManageRequest(&req); err != nil {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	if isGFSManageLocalRequest(context) {
		resp := runGFSManageLocal(req, "local", nil)
		context.JSON(statusCodeFromGFSManageResponse(resp), resp)
		return
	}

	cfg, err := loadClusterConfigSection()
	if err != nil {
		context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
			ErrCode: http.StatusInternalServerError,
			Message: "failed to read cluster.json",
		})
		return
	}

	resp := runGFSManage(req, cfg)
	context.JSON(statusCodeFromGFSManageResponse(resp), resp)
}

func normalizeGFSManageRequest(req *GFSManageRequest) error {
	if req == nil {
		return fmt.Errorf("request required")
	}

	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "init-pcs-cluster", "init":
		req.Action = "init-pcs-cluster"
	case "create-gfs":
		req.Action = "create-gfs"
	case "modify-lvm-conf":
		req.Action = "modify-lvm-conf"
		if req.UseLVMLockd == nil {
			value := true
			req.UseLVMLockd = &value
		}
	case gfsManageSetPCSPassword:
		req.Action = gfsManageSetPCSPassword
	case "partprobe":
		req.Action = "partprobe"
	case "lvmdevices-add":
		req.Action = "lvmdevices-add"
	case gfsManageResourceCleanup:
		req.Action = gfsManageResourceCleanup
	case gfsManagePrepareGFSNode:
		req.Action = gfsManagePrepareGFSNode
	case "check-host":
		req.Action = "check-host"
	case "configure-stonith":
		req.Action = "configure-stonith"
	case "check-stonith":
		req.Action = "check-stonith"
		req.Control = strings.ToLower(strings.TrimSpace(req.Control))
		if req.Control == "" {
			req.Control = "check"
		}
	case "check-ipmi":
		req.Action = "check-ipmi"
	case "set-alert":
		req.Action = "set-alert"
	case gfsManagePrepareAlertFile:
		req.Action = gfsManagePrepareAlertFile
	case "list-gfs":
		req.Action = "list-gfs"
	case "delete-gfs":
		req.Action = "delete-gfs"
	case "rescan":
		req.Action = "rescan"
	case "extend":
		req.Action = "extend"
	case "scan":
		req.Action = "scan"
	case "add-extend":
		req.Action = "add-extend"
	default:
		return fmt.Errorf("unsupported action")
	}

	req.Disk = strings.TrimSpace(req.Disk)
	req.Disks = normalizeStringSlice(append(req.Disks, splitCommaValues(req.Disk)...))
	req.VGName = strings.TrimSpace(req.VGName)
	req.LVName = strings.TrimSpace(req.LVName)
	req.GFSName = strings.TrimSpace(req.GFSName)
	req.MountPoint = strings.TrimSpace(req.MountPoint)
	req.ClusterName = strings.TrimSpace(req.ClusterName)
	req.ClusterUser = strings.TrimSpace(req.ClusterUser)
	req.ClusterPassword = strings.TrimSpace(req.ClusterPassword)
	if req.Action == "init-pcs-cluster" || req.Action == "create-gfs" || req.Action == gfsManageSetPCSPassword {
		if req.ClusterName == "" {
			req.ClusterName = gfsManageDefaultCluster
		}
		if req.ClusterUser == "" {
			req.ClusterUser = gfsManageDefaultPCSUser
		}
		if req.ClusterPassword == "" {
			req.ClusterPassword = gfsManageDefaultPCSPass
		}
		if strings.ContainsAny(req.ClusterUser, ":\x00\r\n") {
			return fmt.Errorf("invalid cluster_user")
		}
		if strings.ContainsAny(req.ClusterPassword, "\x00\r\n") {
			return fmt.Errorf("invalid cluster_password")
		}
		if strings.ContainsAny(req.ClusterName, "\x00\r\n") {
			return fmt.Errorf("invalid cluster_name")
		}
	}
	req.NonStopCheck = strings.ToLower(strings.TrimSpace(req.NonStopCheck))
	req.VolumeGroups = normalizeGFSManageVolumeGroups(req.VolumeGroups, req.VGName, req.LVName)
	req.Stonith = normalizeGFSManageStonithDevices(req.Stonith)

	if req.Action == "init-pcs-cluster" && len(req.Disks) > 0 && len(req.VolumeGroups) == 0 {
		return fmt.Errorf("volume_groups or vg_name/lv_name required when disks are provided")
	}
	if req.Action == "lvmdevices-add" && len(req.Disks) == 0 {
		return fmt.Errorf("disks required")
	}
	if req.Action == "check-stonith" {
		switch req.Control {
		case "check", "enable", "disable", "security-disable", "security-enable":
		default:
			return fmt.Errorf("unsupported stonith control")
		}
	}
	if req.Action == "check-ipmi" && len(req.Stonith) == 0 {
		return fmt.Errorf("stonith required")
	}
	if req.Action == "configure-stonith" && len(req.Stonith) == 0 {
		return fmt.Errorf("stonith required")
	}
	if req.Action == "create-gfs" {
		if req.VGName == "" {
			req.VGName = "vg_glue"
		}
		if req.LVName == "" {
			req.LVName = "lv_glue"
		}
		if req.GFSName == "" {
			req.GFSName = "glue-gfs"
		}
		if req.MountPoint == "" {
			req.MountPoint = "/mnt/glue-gfs"
		}
		if len(req.Disks) == 0 {
			return fmt.Errorf("disks required")
		}
	}
	switch req.Action {
	case "delete-gfs":
		if req.GFSName == "" || req.VGName == "" || req.LVName == "" || len(req.Disks) == 0 {
			return fmt.Errorf("disks, gfs_name, vg_name and lv_name required")
		}
	case "rescan", "extend":
		if req.VGName == "" || req.LVName == "" || req.MountPoint == "" {
			return fmt.Errorf("vg_name, lv_name and mount_point required")
		}
	case "add-extend":
		if req.VGName == "" || req.LVName == "" || req.MountPoint == "" || req.GFSName == "" || len(req.Disks) == 0 {
			return fmt.Errorf("disks, gfs_name, vg_name, lv_name and mount_point required")
		}
	}
	return nil
}

func normalizeGFSManageVolumeGroups(values []GFSManageVolumeGroup, vgName string, lvName string) []GFSManageVolumeGroup {
	if vgName != "" || lvName != "" {
		values = append(values, GFSManageVolumeGroup{VGName: vgName, LVName: lvName})
	}
	seen := map[string]struct{}{}
	out := make([]GFSManageVolumeGroup, 0, len(values))
	for _, value := range values {
		value.VGName = strings.TrimSpace(value.VGName)
		value.LVName = strings.TrimSpace(value.LVName)
		if value.VGName == "" || value.LVName == "" {
			continue
		}
		key := value.VGName + "|" + value.LVName
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeGFSManageStonithDevices(values []GFSManageStonithDevice) []GFSManageStonithDevice {
	out := make([]GFSManageStonithDevice, 0, len(values))
	for _, value := range values {
		value.IPAddr = strings.TrimSpace(value.IPAddr)
		value.IPPort = strings.TrimSpace(value.IPPort)
		value.Login = strings.TrimSpace(value.Login)
		value.Passwd = strings.TrimSpace(value.Passwd)
		value.Host = strings.TrimSpace(value.Host)
		value.Hostname = strings.TrimSpace(value.Hostname)
		if value.IPPort == "" {
			value.IPPort = "623"
		}
		if value.IPAddr == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func splitCommaValues(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	raw := strings.Split(value, ",")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func isGFSManageLocalRequest(context *gin.Context) bool {
	return strings.EqualFold(strings.TrimSpace(context.GetHeader(gfsManageLocalHeader)), "1")
}

func runGFSManage(req GFSManageRequest, cfg *CubeModel.ClusterConfigSection) GFSManageResponse {
	switch req.Action {
	case "init-pcs-cluster":
		target := selectGFSManageExecutionTarget(cfg)
		if target.Target == "" || target.Target == "local" || isLocalTarget(target.Target) || isGFSManageLocalHostname(target.Hostname) {
			return runGFSManageLocal(req, firstNonEmpty(target.Target, "local"), cfg)
		}
		resp, err := callGFSManageRemote(&http.Client{Timeout: gfsManageRemoteTimeout}, target, req)
		if err != nil {
			return gfsManageError(req, target.Target, err.Error(), nil)
		}
		if strings.EqualFold(resp.Target, "local") {
			resp.Target = target.Target
		}
		return resp
	case "set-alert":
		return runGFSManageSetAlert(req, cfg)
	case "create-gfs", "delete-gfs", "extend", "add-extend":
		target := selectGFSManageExecutionTarget(cfg)
		if target.Target == "" || target.Target == "local" || isLocalTarget(target.Target) || isGFSManageLocalHostname(target.Hostname) {
			return runGFSManageLocal(req, firstNonEmpty(target.Target, "local"), cfg)
		}
		resp, err := callGFSManageRemote(&http.Client{Timeout: gfsManageRemoteTimeout}, target, req)
		if err != nil {
			return gfsManageError(req, target.Target, err.Error(), nil)
		}
		if strings.EqualFold(resp.Target, "local") {
			resp.Target = target.Target
		}
		return resp
	case "modify-lvm-conf", gfsManageSetPCSPassword, "partprobe", "lvmdevices-add", gfsManageResourceCleanup, gfsManagePrepareAlertFile, gfsManagePrepareGFSNode:
		return runGFSManageFanout(req, cfg)
	case "scan", "rescan":
		return runGFSManageFanout(req, cfg)
	case "check-host":
		return gfsManageOK(req, "local", gfsManageSortedHosts(cfg), nil)
	case "configure-stonith", "check-stonith", "check-ipmi", "list-gfs":
		return runGFSManageLocal(req, "local", cfg)
	default:
		return gfsManageError(req, "local", "unsupported action", nil)
	}
}

func runGFSManageFanout(req GFSManageRequest, cfg *CubeModel.ClusterConfigSection) GFSManageResponse {
	targets := buildGFSManageTargets(cfg)
	if len(targets) == 0 {
		return gfsManageError(req, "local", "hosts[].ablecube required", nil)
	}

	client := &http.Client{Timeout: gfsManageRemoteTimeout}
	results := make([]GFSManageTargetResult, 0, len(targets))
	for _, target := range targets {
		result := runGFSManageOnTarget(client, target, req, cfg)
		results = append(results, result)
	}
	if err := firstGFSManageResultError(results); err != nil {
		return gfsManageError(req, "fanout", err.Error(), results)
	}
	return gfsManageOK(req, "fanout", "ok", results)
}

func runGFSManageSetAlert(req GFSManageRequest, cfg *CubeModel.ClusterConfigSection) GFSManageResponse {
	prepareReq := GFSManageRequest{Action: gfsManagePrepareAlertFile}
	prepareResp := runGFSManageFanout(prepareReq, cfg)
	if prepareResp.Code != http.StatusOK {
		return gfsManageError(req, "fanout", firstNonEmpty(prepareResp.Message, "alert file prepare failed"), prepareResp.Results)
	}

	target := selectGFSManageExecutionTarget(cfg)
	var setupResp GFSManageResponse
	if target.Target == "" || target.Target == "local" || isLocalTarget(target.Target) || isGFSManageLocalHostname(target.Hostname) {
		setupResp = runGFSManageLocal(req, firstNonEmpty(target.Target, "local"), cfg)
	} else {
		var err error
		setupResp, err = callGFSManageRemote(&http.Client{Timeout: gfsManageRemoteTimeout}, target, req)
		if err != nil {
			return gfsManageError(req, target.Target, err.Error(), prepareResp.Results)
		}
	}
	if setupResp.Code != http.StatusOK {
		return gfsManageError(req, firstNonEmpty(setupResp.Target, target.Target), firstNonEmpty(setupResp.Message, "pcs alert setup failed"), prepareResp.Results)
	}
	return gfsManageOK(req, firstNonEmpty(setupResp.Target, target.Target, "local"), map[string]any{
		"prepare": prepareResp.Results,
		"setup":   setupResp.Val,
	}, prepareResp.Results)
}

func runGFSManageOnTarget(client *http.Client, target gfsManageTarget, req GFSManageRequest, cfg *CubeModel.ClusterConfigSection) GFSManageTargetResult {
	if target.Target == "" || isLocalTarget(target.Target) || isGFSManageLocalHostname(target.Hostname) {
		resp := runGFSManageLocal(req, firstNonEmpty(target.Target, "local"), cfg)
		return gfsManageTargetResult(target, resp)
	}
	resp, err := callGFSManageRemote(client, target, req)
	if err != nil {
		return GFSManageTargetResult{
			Hostname: target.Hostname,
			Target:   target.Target,
			Code:     http.StatusInternalServerError,
			Message:  err.Error(),
		}
	}
	return gfsManageTargetResult(target, resp)
}

func runGFSManageLocal(req GFSManageRequest, target string, cfg *CubeModel.ClusterConfigSection) GFSManageResponse {
	switch req.Action {
	case "init-pcs-cluster":
		if cfg == nil {
			var err error
			cfg, err = loadClusterConfigSection()
			if err != nil {
				return gfsManageError(req, target, err.Error(), nil)
			}
		}
		return runGFSManageInitPCSCluster(req, cfg, target)
	case "modify-lvm-conf":
		useLVMLockd := true
		if req.UseLVMLockd != nil {
			useLVMLockd = *req.UseLVMLockd
		}
		if err := modifyGFSManageLVMConf(useLVMLockd); err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		return gfsManageOK(req, target, "Modify Lvm Conf Success", nil)
	case gfsManageSetPCSPassword:
		if err := setGFSManageClusterPassword(req.ClusterUser, req.ClusterPassword); err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		return gfsManageOK(req, target, "Set Cluster Password Success", nil)
	case "partprobe":
		if err := runGFSManagePartprobe(req.Disks); err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		return gfsManageOK(req, target, "Partprobe Success", nil)
	case "lvmdevices-add":
		if cfg == nil {
			loaded, err := loadClusterConfigSection()
			if err == nil {
				cfg = loaded
			}
		}
		osType := ""
		if cfg != nil {
			osType = cfg.Type
		}
		if err := runGFSManageLVMDevicesAdd(req.Disks, osType); err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		return gfsManageOK(req, target, "Lvmdevices Add Success", nil)
	case gfsManagePrepareGFSNode:
		if cfg == nil {
			loaded, err := loadClusterConfigSection()
			if err == nil {
				cfg = loaded
			}
		}
		osType := ""
		if cfg != nil {
			osType = cfg.Type
		}
		if err := prepareGFSManageNode(req.Disks, req.MountPoint, osType); err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		return gfsManageOK(req, target, "Prepare GFS Node Success", nil)
	case gfsManageResourceCleanup:
		_, _ = runGFSManageCommandIgnore("pcs", "resource", "cleanup")
		return gfsManageOK(req, target, "Resource Cleanup Success", nil)
	case "check-host":
		if cfg == nil {
			var err error
			cfg, err = loadClusterConfigSection()
			if err != nil {
				return gfsManageError(req, target, err.Error(), nil)
			}
		}
		return gfsManageOK(req, target, gfsManageSortedHosts(cfg), nil)
	case "configure-stonith":
		if cfg == nil {
			var err error
			cfg, err = loadClusterConfigSection()
			if err != nil {
				return gfsManageError(req, target, err.Error(), nil)
			}
		}
		val, err := runGFSManageConfigureStonith(req.Stonith, cfg)
		if err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		return gfsManageOK(req, target, val, nil)
	case "check-stonith":
		val, err := runGFSManageStonithControl(req.Control)
		if err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		return gfsManageOK(req, target, val, nil)
	case "check-ipmi":
		val, err := runGFSManageIPMICheck(req.Stonith)
		if err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		return gfsManageOK(req, target, val, nil)
	case "set-alert":
		if err := prepareGFSManageAlertFile(); err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		if err := createGFSManageAlert(); err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		return gfsManageOK(req, target, "Pcs Alert Success", nil)
	case gfsManagePrepareAlertFile:
		if err := prepareGFSManageAlertFile(); err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		return gfsManageOK(req, target, "Alert File Prepare Success", nil)
	case "list-gfs":
		vgs, err := listGFSManageVolumeGroups()
		if err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		return gfsManageOK(req, target, vgs, nil)
	case "create-gfs":
		if cfg == nil {
			loaded, err := loadClusterConfigSection()
			if err != nil {
				return gfsManageError(req, target, err.Error(), nil)
			}
			cfg = loaded
		}
		val, err := createGFSManageDisk(req, cfg)
		if err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		return gfsManageOK(req, target, val, nil)
	case "delete-gfs":
		if cfg == nil {
			loaded, err := loadClusterConfigSection()
			if err == nil {
				cfg = loaded
			}
		}
		if err := deleteGFSManageDisk(req, cfg); err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		return gfsManageOK(req, target, "Success to gfs delete", nil)
	case "scan":
		if err := scanGFSManageSCSIHosts(); err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		return gfsManageOK(req, target, "Success to scan GFS Disk", nil)
	case "rescan":
		if err := rescanGFSManageDisk(req.VGName, req.LVName); err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		return gfsManageOK(req, target, "Success to scan GFS Disk", nil)
	case "extend":
		if cfg == nil {
			loaded, err := loadClusterConfigSection()
			if err == nil {
				cfg = loaded
			}
		}
		if err := extendGFSManageDisk(req, cfg); err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		return gfsManageOK(req, target, "Success to extend GFS Disk", nil)
	case "add-extend":
		if cfg == nil {
			loaded, err := loadClusterConfigSection()
			if err == nil {
				cfg = loaded
			}
		}
		if err := addExtendGFSManageDisk(req, cfg); err != nil {
			return gfsManageError(req, target, err.Error(), nil)
		}
		return gfsManageOK(req, target, "Success to Extend Add GFS Disk", nil)
	default:
		return gfsManageError(req, target, "unsupported action", nil)
	}
}

func runGFSManageInitPCSCluster(req GFSManageRequest, cfg *CubeModel.ClusterConfigSection, target string) GFSManageResponse {
	targets := buildGFSManageTargets(cfg)
	if len(targets) == 0 {
		return gfsManageError(req, target, "hosts[].ablecube required", nil)
	}

	if len(targets) == 1 {
		_, _ = runGFSManageCommandIgnore("pcs", "cluster", "stop")
		_, _ = runGFSManageCommandIgnore("pcs", "cluster", "destroy")
	} else {
		_, _ = runGFSManageCommandIgnore("pcs", "cluster", "stop", "--all")
		_, _ = runGFSManageCommandIgnore("pcs", "cluster", "destroy", "--all")
	}

	steps := make([]GFSManageTargetResult, 0)
	if len(req.Disks) > 0 && len(req.VolumeGroups) > 0 {
		disabled := false
		lvmReq := GFSManageRequest{Action: "modify-lvm-conf", UseLVMLockd: &disabled}
		lvmResults := runGFSManageFanout(lvmReq, cfg).Results
		tagGFSManageResults(lvmResults, "reset_lvm_conf")
		steps = append(steps, lvmResults...)
		if err := firstGFSManageResultError(lvmResults); err != nil {
			return gfsManageError(req, target, err.Error(), steps)
		}

		for _, vg := range req.VolumeGroups {
			if err := cleanupGFSManageVolumeGroup(vg); err != nil {
				return gfsManageError(req, target, err.Error(), steps)
			}
		}

		for _, disk := range req.Disks {
			cleanupGFSManageDisk(disk, cfg.Type)
		}

		probeReq := GFSManageRequest{Action: "partprobe", Disks: req.Disks}
		probeResults := runGFSManageFanout(probeReq, cfg).Results
		tagGFSManageResults(probeResults, "partprobe")
		steps = append(steps, probeResults...)
		if err := firstGFSManageResultError(probeResults); err != nil {
			return gfsManageError(req, target, err.Error(), steps)
		}

		cleanupReq := GFSManageRequest{Action: gfsManageResourceCleanup}
		cleanupResults := runGFSManageFanout(cleanupReq, cfg).Results
		tagGFSManageResults(cleanupResults, "resource_cleanup")
		steps = append(steps, cleanupResults...)
		if err := firstGFSManageResultError(cleanupResults); err != nil {
			return gfsManageError(req, target, err.Error(), steps)
		}
	}

	enabled := true
	lvmReq := GFSManageRequest{Action: "modify-lvm-conf", UseLVMLockd: &enabled}
	lvmResults := runGFSManageFanout(lvmReq, cfg).Results
	tagGFSManageResults(lvmResults, "modify_lvm_conf")
	steps = append(steps, lvmResults...)
	if err := firstGFSManageResultError(lvmResults); err != nil {
		return gfsManageError(req, target, "modify_lvm_conf failed: "+err.Error(), steps)
	}

	passwordReq := GFSManageRequest{
		Action:          gfsManageSetPCSPassword,
		ClusterUser:     req.ClusterUser,
		ClusterPassword: req.ClusterPassword,
	}
	passwordResults := runGFSManageFanout(passwordReq, cfg).Results
	tagGFSManageResults(passwordResults, "set_cluster_password")
	steps = append(steps, passwordResults...)
	if err := firstGFSManageResultError(passwordResults); err != nil {
		return gfsManageError(req, target, "set_cluster_password failed: "+err.Error(), steps)
	}

	hosts := gfsManageTargetAddresses(targets)
	authOutput, err := authGFSManageHosts(hosts, req.ClusterUser, req.ClusterPassword)
	authResult := gfsManageCoordinatorStepResult("auth_hosts", target, authOutput, err)
	steps = append(steps, authResult)
	if err != nil {
		return gfsManageError(req, target, err.Error(), steps)
	}

	setupOutput, err := setupGFSManageCluster(req.ClusterName, hosts)
	setupResult := gfsManageCoordinatorStepResult("setup_cluster", target, setupOutput, err)
	steps = append(steps, setupResult)
	if err != nil {
		return gfsManageError(req, target, err.Error(), steps)
	}

	return gfsManageOK(req, target, map[string]any{
		"message":      "Init PCS Cluster Success",
		"cluster_name": req.ClusterName,
		"hosts":        hosts,
	}, steps)
}

func tagGFSManageResults(results []GFSManageTargetResult, step string) {
	for index := range results {
		results[index].Step = step
	}
}

func gfsManageTargetAddresses(targets []gfsManageTarget) []string {
	hosts := make([]string, 0, len(targets))
	for _, target := range targets {
		if value := strings.TrimSpace(target.Target); value != "" {
			hosts = append(hosts, value)
		}
	}
	return hosts
}

func gfsManageCoordinatorStepResult(step string, target string, output any, err error) GFSManageTargetResult {
	result := GFSManageTargetResult{
		Step:    step,
		Target:  firstNonEmpty(target, "local"),
		Code:    http.StatusOK,
		Message: "ok",
		Val:     output,
	}
	if err != nil {
		result.Code = http.StatusInternalServerError
		result.Message = err.Error()
	}
	return result
}

func setGFSManageClusterPassword(username string, password string) error {
	if _, err := runGFSManageCommand(gfsManageShortTimeout, "systemctl", "enable", "--now", "pcsd.service"); err != nil {
		return fmt.Errorf("enable pcsd failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), gfsManageShortTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "chpasswd")
	cmd.Env = gfsManageCommandEnv()
	cmd.Stdin = strings.NewReader(username + ":" + password + "\n")
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("set cluster password timed out after %s", gfsManageShortTimeout)
	}
	if err != nil {
		return fmt.Errorf("set cluster password failed: %s", firstNonEmpty(strings.TrimSpace(string(output)), err.Error()))
	}
	return nil
}

func authGFSManageHosts(hosts []string, username string, password string) (map[string]any, error) {
	authenticated := make([]string, 0, len(hosts))
	for _, host := range hosts {
		args := buildGFSManageHostAuthArgs(host, username, password)
		out, timedOut, err := runCommandOutputWithEnv("pcs", gfsManageCommandTimeout, gfsManageCommandEnv(), args...)
		if timedOut {
			return map[string]any{"authenticated": authenticated}, fmt.Errorf("pcs host auth %s timed out", host)
		}
		if err != nil {
			return map[string]any{"authenticated": authenticated}, fmt.Errorf("pcs host auth %s failed: %s", host, firstNonEmpty(strings.TrimSpace(out), err.Error()))
		}
		authenticated = append(authenticated, host)
	}
	return map[string]any{"authenticated": authenticated}, nil
}

func buildGFSManageHostAuthArgs(host string, username string, password string) []string {
	return []string{"host", "auth", host, "-u", username, "-p", password}
}

func setupGFSManageCluster(clusterName string, hosts []string) (map[string]any, error) {
	args := buildGFSManageClusterSetupArgs(clusterName, hosts)
	if _, err := runGFSManageCommand(gfsManageCommandTimeout, "pcs", args...); err != nil {
		return nil, fmt.Errorf("pcs cluster setup failed: %w", err)
	}
	if _, err := runGFSManageCommand(gfsManageCommandTimeout, "pcs", "cluster", "enable", "--all"); err != nil {
		return nil, fmt.Errorf("pcs cluster enable failed: %w", err)
	}
	status, err := runGFSManageCommand(gfsManageCommandTimeout, "pcs", "cluster", "status")
	if err != nil {
		return nil, fmt.Errorf("pcs cluster status verification failed: %w", err)
	}
	return map[string]any{
		"cluster_name": clusterName,
		"hosts":        hosts,
		"enabled":      true,
		"status":       strings.TrimSpace(status),
	}, nil
}

func buildGFSManageClusterSetupArgs(clusterName string, hosts []string) []string {
	args := []string{"cluster", "setup", clusterName, "--start"}
	args = append(args, hosts...)
	return append(args, "quorum", "wait_for_all=1", "last_man_standing=1")
}

func buildGFSManageTargets(cfg *CubeModel.ClusterConfigSection) []gfsManageTarget {
	if cfg == nil {
		return nil
	}
	seen := map[string]struct{}{}
	targets := make([]gfsManageTarget, 0, len(cfg.Hosts))
	for _, host := range cfg.Hosts {
		target := strings.TrimSpace(host.Ablecube)
		if target == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		targets = append(targets, gfsManageTarget{
			Hostname: strings.TrimSpace(host.Hostname),
			Target:   target,
		})
	}
	return targets
}

func selectGFSManageExecutionTarget(cfg *CubeModel.ClusterConfigSection) gfsManageTarget {
	targets := buildGFSManageTargets(cfg)
	if len(targets) == 0 {
		return gfsManageTarget{Target: "local"}
	}
	for _, target := range targets {
		if isLocalTarget(target.Target) || isGFSManageLocalHostname(target.Hostname) {
			return target
		}
	}
	client := &http.Client{Timeout: 2 * time.Second}
	for _, target := range targets {
		if err := callHealthTarget(client, target.Target); err == nil {
			return target
		}
	}
	return targets[0]
}

func isGFSManageLocalHostname(hostname string) bool {
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if hostname == "" {
		return false
	}
	localName, err := os.Hostname()
	if err != nil {
		return false
	}
	localName = strings.ToLower(strings.TrimSpace(localName))
	shortName := strings.SplitN(localName, ".", 2)[0]
	return hostname == localName || hostname == shortName
}

func callGFSManageRemote(client *http.Client, target gfsManageTarget, req GFSManageRequest) (GFSManageResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return GFSManageResponse{}, err
	}
	url := fmt.Sprintf("%s/api/v1/cube/gfs/manage", buildTargetURL(target.Target))
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return GFSManageResponse{}, err
	}
	attachInternalToken(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(gfsManageLocalHeader, "1")

	resp, err := client.Do(httpReq)
	if err != nil {
		return GFSManageResponse{}, err
	}
	defer resp.Body.Close()

	var out GFSManageResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		if resp.StatusCode >= 300 {
			return GFSManageResponse{}, fmt.Errorf("gfs manage failed: %s", resp.Status)
		}
		return GFSManageResponse{}, err
	}
	if out.Code == 0 {
		out.Code = resp.StatusCode
	}
	if strings.TrimSpace(out.Target) == "" || strings.EqualFold(strings.TrimSpace(out.Target), "local") {
		out.Target = target.Target
	}
	if strings.TrimSpace(out.Action) == "" {
		out.Action = req.Action
	}
	return out, nil
}

func modifyGFSManageLVMConf(useLVMLockd bool) error {
	data, err := os.ReadFile(gfsManageLVMConfPath)
	if err != nil {
		return err
	}
	content := string(data)
	if useLVMLockd {
		content = replaceGFSManageLVMConf(content, "# use_lvmlockd = 0", "use_lvmlockd = 1")
		content = replaceGFSManageLVMConf(content, "use_lvmlockd = 0", "use_lvmlockd = 1")
		content = replaceGFSManageLVMConf(content, "# use_devicesfile = 0", "use_devicesfile = 1")
		content = replaceGFSManageLVMConf(content, "use_devicesfile = 0", "use_devicesfile = 1")
	} else {
		content = replaceGFSManageLVMConf(content, "use_lvmlockd = 1", "# use_lvmlockd = 0")
		content = replaceGFSManageLVMConf(content, "use_devicesfile = 1", "use_devicesfile = 0")
	}
	if string(data) != content {
		if err := os.WriteFile(gfsManageLVMConfPath, []byte(content), 0644); err != nil {
			return err
		}
	}
	if useLVMLockd {
		_, err = runGFSManageCommand(gfsManageShortTimeout, "mpathconf", "--enable")
		return err
	}
	return nil
}

func replaceGFSManageLVMConf(content string, oldValue string, newValue string) string {
	return strings.ReplaceAll(content, oldValue, newValue)
}

func cleanupGFSManageVolumeGroup(vg GFSManageVolumeGroup) error {
	lvPath, exists, err := findGFSManageLVPath(vg.VGName, vg.LVName)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	_, _ = runGFSManageCommandIgnore("vgchange", "--lock-type", "none", "--lock-opt", "force", vg.VGName, "-y")
	_, _ = runGFSManageCommandIgnore("vgchange", "-aey", vg.VGName)
	_, _ = runGFSManageCommandIgnore("lvremove", "--lockopt", "skiplv", lvPath, "-y")
	if _, err := runGFSManageCommand(gfsManageCommandTimeout, "vgremove", vg.VGName, "-y"); err != nil {
		return err
	}
	return nil
}

func findGFSManageLVPath(vgName string, lvName string) (string, bool, error) {
	if vgName == "" || lvName == "" {
		return "", false, nil
	}
	devPath := "/dev/" + vgName + "/" + lvName
	if _, err := os.Stat(devPath); err == nil {
		return devPath, true, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", false, err
	}

	mapperPath := "/dev/mapper/" + strings.ReplaceAll(vgName, "-", "--") + "-" + strings.ReplaceAll(lvName, "-", "--")
	if _, err := os.Stat(mapperPath); err == nil {
		return mapperPath, true, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", false, err
	}

	out, timedOut, err := runCommandOutputWithEnv("lvdisplay", gfsManageShortTimeout, gfsManageCommandEnv(), "-c")
	if timedOut {
		return "", false, fmt.Errorf("lvdisplay timed out after %s", gfsManageShortTimeout)
	}
	if err != nil && strings.TrimSpace(out) == "" {
		return "", false, nil
	}
	needle := "/" + vgName + "/" + lvName
	for _, line := range splitLines(out) {
		fields := strings.Split(line, ":")
		if len(fields) == 0 {
			continue
		}
		path := strings.TrimSpace(fields[0])
		if strings.Contains(path, needle) {
			return path, true, nil
		}
	}
	return "", false, nil
}

func cleanupGFSManageDisk(disk string, osType string) {
	disk = strings.TrimSpace(disk)
	if disk == "" {
		return
	}
	for _, partition := range gfsManagePartitionCandidates(disk, osType) {
		_, _ = runGFSManageCommandIgnore("pvremove", "-ff", "--yes", partition)
	}
	_, _ = runGFSManageCommandIgnore("parted", "-s", disk, "rm", "1")
}

func gfsManagePartitionCandidates(disk string, osType string) []string {
	disk = strings.TrimSpace(disk)
	if disk == "" {
		return nil
	}
	candidates := []string{}
	if strings.EqualFold(strings.TrimSpace(osType), gfsManageHCIFilesystem) {
		candidates = append(candidates, disk+"-part1", disk+"p1", disk+"1")
	} else if strings.Contains(disk, "dm-uuid-mpath-") {
		candidates = append(candidates, strings.Replace(disk, "dm-uuid-mpath-", "dm-uuid-part1-mpath-", 1))
	} else if strings.Contains(strings.ToLower(disk), "mpath") {
		candidates = append(candidates, disk+"1", disk+"p1")
	} else if lastGFSManageCharIsDigit(disk) {
		candidates = append(candidates, disk+"p1", disk+"-part1")
	} else {
		candidates = append(candidates, disk+"1")
	}
	return normalizeStringSlice(candidates)
}

func lastGFSManageCharIsDigit(value string) bool {
	if value == "" {
		return false
	}
	last := value[len(value)-1]
	return last >= '0' && last <= '9'
}

func runGFSManagePartprobe(disks []string) error {
	targets := normalizeStringSlice(disks)
	if len(targets) == 0 {
		targets = listGFSManageLocalDisks()
	}
	for _, disk := range targets {
		_, _ = runGFSManageCommandIgnore("partprobe", disk)
	}
	return nil
}

func runGFSManageLVMDevicesAdd(disks []string, osType string) error {
	for _, disk := range disks {
		_, _ = runGFSManageCommandIgnore("partprobe", disk)
		for _, partition := range gfsManagePartitionCandidates(disk, osType) {
			_, _ = runGFSManageCommandIgnore("lvmdevices", "--adddev", partition)
		}
	}
	return nil
}

func prepareGFSManageNode(disks []string, mountPoint string, osType string) error {
	for _, disk := range disks {
		if _, err := runGFSManageCommand(gfsManageShortTimeout, "partprobe", disk); err != nil {
			return fmt.Errorf("partprobe failed for %s: %w", disk, err)
		}
		partition, err := waitForGFSManagePartition(disk, osType)
		if err != nil {
			return err
		}
		if _, err := runGFSManageCommand(gfsManageShortTimeout, "lvmdevices", "--adddev", partition); err != nil {
			return fmt.Errorf("lvmdevices add failed for %s: %w", partition, err)
		}
	}
	if mountPoint == "" {
		return nil
	}
	return os.MkdirAll(mountPoint, 0755)
}

func createGFSManageDisk(req GFSManageRequest, cfg *CubeModel.ClusterConfigSection) (map[string]any, error) {
	if err := waitForGFSManageLockingReady(); err != nil {
		return nil, fmt.Errorf("wait_pcs_ready failed: %w", err)
	}

	osType := ""
	if cfg != nil {
		osType = cfg.Type
	}
	partitions := make([]string, 0, len(req.Disks))
	for _, disk := range req.Disks {
		if _, err := runGFSManageCommand(
			gfsManageCommandTimeout,
			"parted", "-s", disk, "mklabel", "gpt", "mkpart", req.GFSName, "0%", "100%", "set", "1", "lvm", "on",
		); err != nil {
			return nil, fmt.Errorf("partition_create failed for %s: %w", disk, err)
		}
		_, _ = runGFSManageCommandIgnore("partprobe", disk)
		partition, err := waitForGFSManagePartition(disk, osType)
		if err != nil {
			return nil, fmt.Errorf("partition_wait failed for %s: %w", disk, err)
		}
		partitions = append(partitions, partition)
	}

	prepareReq := GFSManageRequest{Action: gfsManagePrepareGFSNode, Disks: req.Disks, MountPoint: req.MountPoint}
	prepareResults := runGFSManageFanout(prepareReq, cfg).Results
	if err := firstGFSManageResultError(prepareResults); err != nil {
		return nil, fmt.Errorf("prepare_gfs_nodes failed: %w", err)
	}

	for _, partition := range partitions {
		if _, err := runGFSManageCommand(gfsManageCommandTimeout, "pvcreate", "-ff", "--yes", partition); err != nil {
			return nil, fmt.Errorf("pvcreate failed for %s: %w", partition, err)
		}
	}
	vgArgs := append([]string{"--yes", "--shared", req.VGName}, partitions...)
	if _, err := runGFSManageCommand(gfsManageCommandTimeout, "vgcreate", vgArgs...); err != nil {
		return nil, fmt.Errorf("vgcreate failed: %w", err)
	}
	if _, err := runGFSManageCommand(
		gfsManageCommandTimeout,
		"lvcreate", "--yes", "--activate", "sy", "-l", "+100%FREE", "-n", req.LVName, req.VGName,
	); err != nil {
		return nil, fmt.Errorf("lvcreate failed: %w", err)
	}

	lvPath := "/dev/" + req.VGName + "/" + req.LVName
	journalCount := gfsManageJournalCount(cfg)
	mkfsArgs, formatSettings, err := buildGFSManageMkfsArgs(req, cfg, lvPath)
	if err != nil {
		return nil, err
	}
	if _, err := runGFSManageCommand(
		gfsManageCommandTimeout,
		"mkfs.gfs2", mkfsArgs...,
	); err != nil {
		return nil, fmt.Errorf("mkfs_gfs2 failed: %w", err)
	}

	for index, args := range buildGFSManageResourceCommands(req, lvPath) {
		if _, err := runGFSManageCommand(gfsManageCommandTimeout, "pcs", args...); err != nil {
			return nil, fmt.Errorf("pcs_resource_create[%d] failed: %w", index+1, err)
		}
	}
	_, _ = runGFSManageCommandIgnore("pcs", "resource", "cleanup")

	return map[string]any{
		"message":                "Create GFS Success",
		"cluster_name":           req.ClusterName,
		"gfs_name":               req.GFSName,
		"mount_point":            req.MountPoint,
		"vg_name":                req.VGName,
		"lv_name":                req.LVName,
		"lv_path":                lvPath,
		"journals":               journalCount,
		"journal_size_mb":        formatSettings.JournalSizeMB,
		"resource_group_size_mb": formatSettings.ResourceGroupSizeMB,
		"partitions":             partitions,
		"prepare_nodes":          prepareResults,
	}, nil
}

func buildGFSManageMkfsArgs(req GFSManageRequest, cfg *CubeModel.ClusterConfigSection, lvPath string) ([]string, gfsManageFormatSettings, error) {
	settings, err := resolveGFSManageFormatSettings(cfg)
	if err != nil {
		return nil, gfsManageFormatSettings{}, err
	}
	args := []string{
		fmt.Sprintf("-j%d", gfsManageJournalCount(cfg)),
		"-J", fmt.Sprint(settings.JournalSizeMB),
		"-r", fmt.Sprint(settings.ResourceGroupSizeMB),
		"-p", "lock_dlm",
		"-t", req.ClusterName + ":" + req.GFSName,
		lvPath, "-O", "-K", "-q",
	}
	return args, settings, nil
}

func resolveGFSManageFormatSettings(cfg *CubeModel.ClusterConfigSection) (gfsManageFormatSettings, error) {
	settings := gfsManageFormatSettings{
		JournalSizeMB:       CubeModel.GFSDefaultJournalSizeMB,
		ResourceGroupSizeMB: CubeModel.GFSDefaultResourceGroupSizeMB,
	}
	if cfg != nil {
		if cfg.GFS.JournalSizeMB != 0 {
			settings.JournalSizeMB = cfg.GFS.JournalSizeMB
		}
		if cfg.GFS.ResourceGroupSizeMB != 0 {
			settings.ResourceGroupSizeMB = cfg.GFS.ResourceGroupSizeMB
		}
	}
	if err := validateGFSManagePowerOfTwoSize(
		"clusterConfig.gfs.journal_size_mb",
		settings.JournalSizeMB,
		CubeModel.GFSMinJournalSizeMB,
		CubeModel.GFSMaxJournalSizeMB,
	); err != nil {
		return gfsManageFormatSettings{}, err
	}
	if err := validateGFSManagePowerOfTwoSize(
		"clusterConfig.gfs.resource_group_size_mb",
		settings.ResourceGroupSizeMB,
		CubeModel.GFSMinResourceGroupSizeMB,
		CubeModel.GFSMaxResourceGroupSizeMB,
	); err != nil {
		return gfsManageFormatSettings{}, err
	}
	return settings, nil
}

func validateGFSManagePowerOfTwoSize(name string, value int, minimum int, maximum int) error {
	if value < minimum || value > maximum || value&(value-1) != 0 {
		return fmt.Errorf("%s must be a power of two between %dMB and %dMB", name, minimum, maximum)
	}
	return nil
}

func gfsManageJournalCount(cfg *CubeModel.ClusterConfigSection) int {
	return len(buildGFSManageTargets(cfg)) + 1
}

func waitForGFSManageLockingReady() error {
	startedAt := time.Now()
	deadline := startedAt.Add(gfsManageClusterReadyWait)
	lastStatus := ""
	consecutiveReady := 0
	for {
		status, err := runGFSManageCommand(gfsManageShortTimeout, "pcs", "status")
		lastStatus = strings.TrimSpace(status)
		if err == nil && gfsManageCloneStatusReady(lastStatus) {
			consecutiveReady++
			if time.Since(startedAt) >= gfsManageLockingSettle && consecutiveReady >= gfsManageReadyChecks {
				return nil
			}
		} else {
			consecutiveReady = 0
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("GFS clone resources not ready within %s: %s", gfsManageClusterReadyWait, lastStatus)
		}
		time.Sleep(gfsManageReadyPoll)
	}
}

func gfsManageCloneStatusReady(status string) bool {
	statusLower := strings.ToLower(status)
	if !strings.Contains(statusLower, "glue-locking-clone") {
		return false
	}
	for _, resource := range []string{"glue-locking-clone", "glue-gfs-clone", "glue-gfs_res-clone"} {
		section, found := gfsManageCloneStatusSection(statusLower, resource)
		if found && strings.Contains(section, "stopped") {
			return false
		}
	}
	return true
}

func gfsManageCloneStatusSection(statusLower string, resource string) (string, bool) {
	start := strings.Index(statusLower, resource)
	if start < 0 {
		return "", false
	}
	section := statusLower[start:]
	for _, boundary := range []string{"\n- clone set:", "\nclone set:", "\nfailed resource actions:", "\ndaemon status:"} {
		if next := strings.Index(section[1:], boundary); next >= 0 {
			section = section[:next+1]
		}
	}
	return section, true
}

func buildGFSManageResourceCommands(req GFSManageRequest, lvPath string) [][]string {
	lvmResource := req.GFSName + "_res"
	return [][]string{
		{"resource", "create", lvmResource, "--group", req.GFSName + "-group", "ocf:heartbeat:LVM-activate", "lvname=" + req.LVName, "vgname=" + req.VGName, "activation_mode=shared", "vg_access_mode=lvmlockd"},
		{"resource", "clone", lvmResource, "interleave=true"},
		{"constraint", "order", "start", "glue-locking-clone", "then", lvmResource + "-clone"},
		{"constraint", "colocation", "add", lvmResource + "-clone", "with", "glue-locking-clone"},
		{"resource", "create", req.GFSName, "--group", req.GFSName + "-group", "ocf:heartbeat:Filesystem", "device=" + lvPath, "directory=" + req.MountPoint, "fstype=gfs2", "options=noatime", "op", "monitor", "timeout=120s", "interval=10s", "op", "start", "timeout=80s", "op", "stop", "timeout=80s", "on-fail=fence"},
		{"resource", "clone", req.GFSName, "interleave=true"},
		{"constraint", "order", "start", lvmResource + "-clone", "then", req.GFSName + "-clone"},
		{"constraint", "colocation", "add", lvmResource + "-clone", "with", req.GFSName + "-clone"},
	}
}

func listGFSManageLocalDisks() []string {
	out, timedOut, err := runCommandOutputWithEnv("lsblk", gfsManageShortTimeout, gfsManageCommandEnv(), "-r", "-n", "-o", "NAME,TYPE", "-d")
	if timedOut || err != nil {
		return nil
	}
	disks := make([]string, 0)
	for _, line := range splitLines(out) {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[1] == "rom" {
			continue
		}
		disks = append(disks, "/dev/"+fields[0])
	}
	return disks
}

func runGFSManageStonithControl(control string) (any, error) {
	out, err := runGFSManageCommand(gfsManageShortTimeout, "pcs", "stonith", "status")
	if err != nil && control == "check" {
		return nil, err
	}
	if control == "check" {
		return strings.TrimSpace(out), nil
	}

	resourceIDs := parseGFSManageStonithResourceIDs(out)
	for _, resourceID := range resourceIDs {
		switch control {
		case "enable", "security-enable":
			if _, err := runGFSManageCommand(gfsManageShortTimeout, "pcs", "stonith", "enable", resourceID); err != nil {
				return nil, err
			}
		case "disable", "security-disable":
			if _, err := runGFSManageCommand(gfsManageShortTimeout, "pcs", "stonith", "disable", resourceID); err != nil {
				return nil, err
			}
		}
	}
	switch control {
	case "security-disable":
		if _, err := runGFSManageCommand(gfsManageShortTimeout, "pcs", "property", "set", "maintenance-mode=true"); err != nil {
			return nil, err
		}
	case "security-enable":
		if _, err := runGFSManageCommand(gfsManageShortTimeout, "pcs", "property", "set", "maintenance-mode=false"); err != nil {
			return nil, err
		}
	}
	return "Stonith Pcs Cluster Success", nil
}

func runGFSManageConfigureStonith(devices []GFSManageStonithDevice, cfg *CubeModel.ClusterConfigSection) (any, error) {
	clusterStatus, err := runGFSManageCommand(gfsManageShortTimeout, "pcs", "cluster", "status")
	if err != nil {
		return nil, fmt.Errorf("PCS cluster is not ready; run init-pcs-cluster before configure-stonith: %w", err)
	}

	devices, err = resolveGFSManageStonithDevices(devices, cfg)
	if err != nil {
		return nil, err
	}

	status, _ := runGFSManageCommand(gfsManageShortTimeout, "pcs", "stonith", "status")
	existing := map[string]struct{}{}
	for _, resourceID := range parseGFSManageStonithResourceIDs(status) {
		existing[resourceID] = struct{}{}
	}

	results := make([]map[string]any, 0, len(devices))
	for _, device := range devices {
		resourceID := "fence-" + device.Hostname
		action := "create"
		_, exists := existing[resourceID]
		if exists {
			action = "update"
		}
		args := buildGFSManageStonithCommand(device, exists)
		if _, err := runGFSManageCommand(gfsManageCommandTimeout, "pcs", args...); err != nil {
			return results, err
		}
		_, _ = runGFSManageCommandIgnore("pcs", "constraint", "location", resourceID, "avoids", device.Host)
		results = append(results, map[string]any{
			"resource": resourceID,
			"host":     device.Host,
			"ipaddr":   device.IPAddr,
			"action":   action,
		})
	}

	for _, args := range [][]string{
		{"property", "set", "stonith-enabled=true"},
		{"property", "set", "stonith-action=reboot"},
	} {
		if _, err := runGFSManageCommand(gfsManageShortTimeout, "pcs", args...); err != nil {
			return results, err
		}
	}
	lockingCommands := [][]string{
		{"resource", "create", "glue-dlm", "--group", "glue-locking", "ocf:pacemaker:controld", "op", "monitor", "interval=45s", "on-fail=fence"},
		{"resource", "create", "glue-lvmlockd", "--group", "glue-locking", "ocf:heartbeat:lvmlockd", "op", "monitor", "interval=45s", "on-fail=fence"},
		{"resource", "clone", "glue-locking", "interleave=true"},
	}
	for _, args := range lockingCommands {
		if _, err := runGFSManageCommand(gfsManageCommandTimeout, "pcs", args...); err != nil {
			return results, fmt.Errorf("configure locking resource failed: %w", err)
		}
	}

	return map[string]any{
		"message":       "Configure Stonith Devices Success",
		"stonithAction": "reboot",
		"clusterStatus": strings.TrimSpace(clusterStatus),
		"devices":       results,
	}, nil
}

func buildGFSManageStonithCommand(device GFSManageStonithDevice, update bool) []string {
	action := "create"
	args := []string{"stonith", action, "fence-" + device.Hostname, "fence_ipmilan"}
	if update {
		action = "update"
		args = []string{"stonith", action, "fence-" + device.Hostname}
	}
	return append(args,
		"delay=10",
		"ip="+device.IPAddr,
		"ipport="+device.IPPort,
		"lanplus=1",
		"method=onoff",
		"username="+device.Login,
		"password="+device.Passwd,
		"pcmk_host_list="+device.Host,
		"pcmk_off_action=off",
		"pcmk_reboot_action=reboot",
		"debug_file=/var/log/stonith.log",
	)
}

func resolveGFSManageStonithDevices(devices []GFSManageStonithDevice, cfg *CubeModel.ClusterConfigSection) ([]GFSManageStonithDevice, error) {
	if cfg == nil {
		return nil, fmt.Errorf("clusterConfig required")
	}
	out := append([]GFSManageStonithDevice(nil), devices...)
	for index := range out {
		if index < len(cfg.Hosts) {
			host := cfg.Hosts[index]
			if out[index].Host == "" {
				out[index].Host = firstNonEmpty(host.Ablecube, host.AblecubePn)
			}
			if out[index].Hostname == "" {
				out[index].Hostname = strings.TrimSpace(host.Hostname)
			}
		}
		if out[index].Host == "" {
			return nil, fmt.Errorf("stonith[%d].host required", index)
		}
		if out[index].Hostname == "" {
			return nil, fmt.Errorf("stonith[%d].hostname required", index)
		}
		if strings.ContainsAny(out[index].Hostname, " \t\r\n/") {
			return nil, fmt.Errorf("stonith[%d].hostname is invalid", index)
		}
		if out[index].Login == "" || out[index].Passwd == "" {
			return nil, fmt.Errorf("stonith[%d] login and passwd required", index)
		}
	}
	return out, nil
}

func parseGFSManageStonithResourceIDs(output string) []string {
	seen := map[string]struct{}{}
	ids := make([]string, 0)
	for _, line := range splitLines(output) {
		for _, field := range strings.Fields(line) {
			field = strings.Trim(field, "*: ")
			if field == "" {
				continue
			}
			if strings.HasPrefix(field, "fence-") {
				if _, ok := seen[field]; ok {
					continue
				}
				seen[field] = struct{}{}
				ids = append(ids, field)
			}
		}
	}
	return ids
}

func runGFSManageIPMICheck(devices []GFSManageStonithDevice) (any, error) {
	type ipmiResult struct {
		IP      string `json:"ip"`
		Status  string `json:"status,omitempty"`
		Message string `json:"message,omitempty"`
	}
	results := make([]ipmiResult, 0, len(devices))
	var firstErr error
	for _, device := range devices {
		out, err := runGFSManageCommand(
			gfsManageShortTimeout,
			"ipmitool",
			"-I", "lanplus",
			"-H", device.IPAddr,
			"-U", device.Login,
			"-P", device.Passwd,
			"power", "status",
		)
		item := ipmiResult{IP: device.IPAddr}
		if err != nil {
			item.Message = err.Error()
			if firstErr == nil {
				firstErr = err
			}
		} else {
			item.Status = strings.TrimSpace(out)
		}
		results = append(results, item)
	}
	return results, firstErr
}

func prepareGFSManageAlertFile() error {
	for _, path := range []string{gfsManageAlertLogPath, gfsManageAlertDetailPath} {
		if err := prepareGFSManageAlertLogFile(path); err != nil {
			return err
		}
	}
	return nil
}

func prepareGFSManageAlertLogFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if _, err := runGFSManageCommand(gfsManageShortTimeout, "chown", "hacluster:haclient", path); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

func createGFSManageAlert() error {
	_, _ = runGFSManageCommandIgnore("pcs", "alert", "remove", "alert_file")
	if _, err := runGFSManageCommand(gfsManageShortTimeout, "pcs", "alert", "create", "id=alert_file", "description=Log events to a file.", "path="+gfsManageAlertScriptPath()); err != nil {
		return err
	}
	_, err := runGFSManageCommand(gfsManageShortTimeout, "pcs", "alert", "recipient", "add", "alert_file", "id=alert_logfile", "value="+gfsManageAlertLogPath)
	return err
}

func gfsManageAlertScriptPath() string {
	return resolveAbleStackShellFile("alert_file.sh", filepath.Join("host", "alert_file.sh"))
}

func gfsManageSortedHosts(cfg *CubeModel.ClusterConfigSection) []CubeModel.ClusterHost {
	if cfg == nil {
		return nil
	}
	hosts := append([]CubeModel.ClusterHost(nil), cfg.Hosts...)
	sort.Slice(hosts, func(i int, j int) bool {
		return strings.TrimSpace(hosts[i].Hostname) < strings.TrimSpace(hosts[j].Hostname)
	})
	return hosts
}

func listGFSManageVolumeGroups() ([]map[string]string, error) {
	out, err := runGFSManageCommand(gfsManageShortTimeout, "vgs", "-o", "vg_name", "--reportformat", "json")
	if err != nil {
		return nil, err
	}
	var report gfsManageVGReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		return nil, err
	}
	vgs := make([]map[string]string, 0)
	for _, section := range report.Report {
		for _, vg := range section.VG {
			if strings.Contains(vg.VGName, "vg_glue") {
				vgs = append(vgs, map[string]string{"vg_name": vg.VGName})
			}
		}
	}
	return vgs, nil
}

func deleteGFSManageDisk(req GFSManageRequest, cfg *CubeModel.ClusterConfigSection) error {
	paths, err := collectGFSManageDevicePaths(req.VGName, req.LVName, "")
	if err != nil {
		return fmt.Errorf("resolve GFS delete paths failed: %w", err)
	}

	_, _ = runGFSManageCommand(gfsManageCommandTimeout, "pcs", "resource", "disable", req.GFSName)
	_, _ = runGFSManageCommand(gfsManageCommandTimeout, "pcs", "resource", "disable", req.GFSName+"_res")
	time.Sleep(8 * time.Second)
	_, _ = runGFSManageCommand(gfsManageCommandTimeout, "pcs", "resource", "delete", req.GFSName, "--force")
	time.Sleep(8 * time.Second)
	_, _ = runGFSManageCommand(gfsManageCommandTimeout, "pcs", "resource", "delete", req.GFSName+"_res", "--force")
	_, _ = runGFSManageCommand(gfsManageCommandTimeout, "pcs", "resource", "cleanup")

	if err := cleanupGFSManageVolumeGroup(GFSManageVolumeGroup{VGName: req.VGName, LVName: req.LVName}); err != nil {
		return err
	}
	if err := removeGFSManagePVsAndPartitions(paths.Partitions, paths.Disks); err != nil {
		return err
	}
	return fanoutGFSManagePartprobe(paths.Disks, cfg)
}

func removeGFSManagePVsAndPartitions(partitions []string, disks []string) error {
	partitions = normalizeStringSlice(partitions)
	disks = normalizeStringSlice(disks)
	if len(partitions) == 0 || len(disks) == 0 {
		return fmt.Errorf("GFS PV and disk paths are required")
	}
	for _, partition := range partitions {
		if _, err := runGFSManageCommand(gfsManageCommandTimeout, "pvremove", "-ff", "--yes", partition); err != nil {
			return fmt.Errorf("pvremove failed for %s: %w", partition, err)
		}
		_, _ = runGFSManageCommandIgnore("lvmdevices", "--deldev", partition)
	}
	for _, disk := range disks {
		if _, err := runGFSManageCommand(gfsManageCommandTimeout, "parted", "-s", disk, "rm", "1"); err != nil {
			return fmt.Errorf("partition delete failed for %s: %w", disk, err)
		}
		_, _ = runGFSManageCommandIgnore("partprobe", disk)
	}
	return nil
}

func scanGFSManageSCSIHosts() error {
	hosts, err := filepath.Glob("/sys/class/scsi_host/*/scan")
	if err != nil {
		return err
	}
	for _, host := range hosts {
		if err := os.WriteFile(host, []byte("- - -"), 0200); err != nil {
			return err
		}
	}
	return nil
}

func rescanGFSManageDisk(vgName string, lvName string) error {
	paths, err := collectGFSManageDevicePaths(vgName, lvName, "")
	if err != nil {
		return err
	}
	for _, blockName := range paths.BlockNames {
		rescanPath := filepath.Join("/sys/block", blockName, "device", "rescan")
		if err := os.WriteFile(rescanPath, []byte("1"), 0200); err != nil {
			return fmt.Errorf("SCSI rescan failed for %s: %w", blockName, err)
		}
	}
	for _, mapName := range paths.MapNames {
		if _, err := runGFSManageCommand(gfsManageCommandTimeout, "multipathd", "resize", "map", mapName); err != nil {
			return fmt.Errorf("multipath resize failed for %s: %w", mapName, err)
		}
	}
	if _, err := runGFSManageCommand(gfsManageShortTimeout, "udevadm", "settle"); err != nil {
		return fmt.Errorf("udev settle after multipath resize failed: %w", err)
	}
	return nil
}

func extendGFSManageDisk(req GFSManageRequest, cfg *CubeModel.ClusterConfigSection) error {
	if req.NonStopCheck == "true" {
		if _, err := runGFSManageCommand(gfsManageShortTimeout, "pcs", "property", "set", "maintenance-mode=true"); err != nil {
			return err
		}
		defer func() {
			_, _ = runGFSManageCommand(gfsManageShortTimeout, "pcs", "property", "set", "maintenance-mode=false")
		}()
	}

	rescanReq := GFSManageRequest{
		Action:     "rescan",
		VGName:     req.VGName,
		LVName:     req.LVName,
		MountPoint: req.MountPoint,
	}
	rescanResp := runGFSManageFanout(rescanReq, cfg)
	if rescanResp.Code != http.StatusOK {
		return fmt.Errorf("rescan multipath devices failed: %s", firstNonEmpty(rescanResp.Message, fmt.Sprint(rescanResp.Val)))
	}

	paths, err := collectGFSManageDevicePaths(req.VGName, req.LVName, "")
	if err != nil {
		return err
	}
	for _, disk := range paths.Disks {
		if _, err := runGFSManageCommand(gfsManageCommandTimeout, "parted", "-s", disk, "resizepart", "1", "100%", "-f"); err != nil {
			return fmt.Errorf("partition resize failed for %s: %w", disk, err)
		}
	}
	if err := fanoutGFSManagePartprobe(paths.Disks, cfg); err != nil {
		return err
	}
	for _, partition := range paths.Partitions {
		if _, err := runGFSManageCommand(gfsManageCommandTimeout, "pvresize", partition); err != nil {
			return err
		}
	}
	if _, err := runGFSManageCommand(gfsManageCommandTimeout, "lvextend", "-l", "+100%FREE", req.VGName+"/"+req.LVName); err != nil {
		return err
	}
	if _, err := runGFSManageCommand(gfsManageCommandTimeout, "gfs2_grow", req.MountPoint); err != nil {
		return err
	}
	return fanoutGFSManagePartprobe(paths.Disks, cfg)
}

func addExtendGFSManageDisk(req GFSManageRequest, cfg *CubeModel.ClusterConfigSection) error {
	if req.NonStopCheck == "true" {
		if _, err := runGFSManageCommand(gfsManageShortTimeout, "pcs", "property", "set", "maintenance-mode=true"); err != nil {
			return err
		}
		defer func() {
			_, _ = runGFSManageCommand(gfsManageShortTimeout, "pcs", "property", "set", "maintenance-mode=false")
		}()
	}

	partitions := make([]string, 0, len(req.Disks))
	osType := ""
	if cfg != nil {
		osType = cfg.Type
	}
	for _, disk := range req.Disks {
		if _, err := runGFSManageCommand(gfsManageCommandTimeout, "parted", "-s", disk, "mklabel", "gpt", "mkpart", req.GFSName, "0%", "100%", "set", "1", "lvm", "on"); err != nil {
			return err
		}
		_, _ = runGFSManageCommand(gfsManageShortTimeout, "partprobe", disk)
		partition, err := waitForGFSManagePartition(disk, osType)
		if err != nil {
			return err
		}
		if _, err := runGFSManageCommand(gfsManageCommandTimeout, "pvcreate", partition); err != nil {
			return err
		}
		partitions = append(partitions, partition)
	}

	if err := fanoutGFSManageLVMDevices(req.Disks, cfg); err != nil {
		return err
	}
	if _, err := runGFSManageCommand(gfsManageCommandTimeout, "vgextend", append([]string{req.VGName}, partitions...)...); err != nil {
		return err
	}
	if _, err := runGFSManageCommand(gfsManageCommandTimeout, "lvextend", "-l", "+100%FREE", "/dev/"+req.VGName+"/"+req.LVName); err != nil {
		return err
	}
	_, err := runGFSManageCommand(gfsManageCommandTimeout, "gfs2_grow", req.MountPoint)
	return err
}

func waitForGFSManagePartition(disk string, osType string) (string, error) {
	deadline := time.Now().Add(gfsManageShortTimeout)
	candidates := gfsManagePartitionCandidates(disk, osType)
	for {
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("partition not found for %s", disk)
		}
		time.Sleep(time.Second)
	}
}

func collectGFSManageDevicePaths(vgName string, lvName string, osType string) (gfsManageDevicePaths, error) {
	out, err := runGFSManageCommand(gfsManageShortTimeout, "lsblk", "-J", "-o", "name,path,type,mountpoint")
	if err != nil {
		return gfsManageDevicePaths{}, err
	}
	var payload gfsManageLSBLKPayload
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return gfsManageDevicePaths{}, err
	}

	targetName := vgName + "-" + lvName
	result := gfsManageDevicePaths{}
	var walk func(node gfsManageBlockDevice, ancestors []gfsManageBlockDevice)
	walk = func(node gfsManageBlockDevice, ancestors []gfsManageBlockDevice) {
		if node.Name == targetName {
			result.LVPaths = append(result.LVPaths, firstNonEmpty(node.Path, "/dev/mapper/"+node.Name))
			result = appendGFSManageAncestorPaths(result, ancestors, osType)
		}
		next := append(append([]gfsManageBlockDevice{}, ancestors...), node)
		for _, child := range node.Children {
			walk(child, next)
		}
	}
	for _, dev := range payload.Blockdevices {
		walk(dev, nil)
	}
	result.Disks = normalizeStringSlice(result.Disks)
	result.Partitions = normalizeStringSlice(result.Partitions)
	result.LVPaths = normalizeStringSlice(result.LVPaths)
	result.MapNames = normalizeStringSlice(result.MapNames)
	result.BlockNames = normalizeStringSlice(result.BlockNames)
	if len(result.Disks) == 0 && len(result.Partitions) == 0 {
		return result, fmt.Errorf("gfs disk not found")
	}
	return result, nil
}

func appendGFSManageAncestorPaths(result gfsManageDevicePaths, ancestors []gfsManageBlockDevice, _ string) gfsManageDevicePaths {
	if len(ancestors) == 0 {
		return result
	}
	root := ancestors[0]
	parent := ancestors[len(ancestors)-1]
	if root.Name != "" {
		result.BlockNames = append(result.BlockNames, root.Name)
	}
	if mpath, ok := nearestGFSManageMultipathAncestor(ancestors); ok {
		diskPath := firstNonEmpty(mpath.Path, root.Path)
		result.Disks = append(result.Disks, diskPath)
		result.MapNames = append(result.MapNames, mpath.Name)
		result.Partitions = append(result.Partitions, firstNonEmpty(parent.Path, "/dev/"+parent.Name, diskPath))
		return result
	}
	result.Disks = append(result.Disks, firstNonEmpty(root.Path, "/dev/"+root.Name))
	result.Partitions = append(result.Partitions, firstNonEmpty(parent.Path, "/dev/"+parent.Name))
	return result
}

func nearestGFSManageMultipathAncestor(ancestors []gfsManageBlockDevice) (gfsManageBlockDevice, bool) {
	for i := len(ancestors) - 1; i >= 0; i-- {
		node := ancestors[i]
		if strings.EqualFold(node.Type, "mpath") ||
			(!strings.EqualFold(node.Type, "part") && strings.Contains(strings.ToLower(node.Name), "mpath")) {
			return node, true
		}
	}
	return gfsManageBlockDevice{}, false
}

func fanoutGFSManagePartprobe(disks []string, cfg *CubeModel.ClusterConfigSection) error {
	if len(disks) == 0 || cfg == nil {
		return nil
	}
	req := GFSManageRequest{Action: "partprobe", Disks: disks}
	if err := normalizeGFSManageRequest(&req); err != nil {
		return err
	}
	resp := runGFSManageFanout(req, cfg)
	if resp.Code != http.StatusOK {
		return fmt.Errorf("partprobe failed: %s", firstNonEmpty(resp.Message, fmt.Sprint(resp.Val)))
	}
	return nil
}

func fanoutGFSManageLVMDevices(disks []string, cfg *CubeModel.ClusterConfigSection) error {
	if len(disks) == 0 || cfg == nil {
		return nil
	}
	req := GFSManageRequest{Action: "lvmdevices-add", Disks: disks}
	if err := normalizeGFSManageRequest(&req); err != nil {
		return err
	}
	resp := runGFSManageFanout(req, cfg)
	if resp.Code != http.StatusOK {
		return fmt.Errorf("lvmdevices-add failed: %s", firstNonEmpty(resp.Message, fmt.Sprint(resp.Val)))
	}
	return nil
}

func firstGFSManageResultError(results []GFSManageTargetResult) error {
	for _, result := range results {
		if result.Code != http.StatusOK {
			return fmt.Errorf("%s: %s", result.Target, result.Message)
		}
	}
	return nil
}

func gfsManageTargetResult(target gfsManageTarget, resp GFSManageResponse) GFSManageTargetResult {
	return GFSManageTargetResult{
		Hostname: target.Hostname,
		Target:   firstNonEmpty(resp.Target, target.Target, "local"),
		Code:     resp.Code,
		Message:  firstNonEmpty(resp.Message, "ok"),
		Val:      resp.Val,
	}
}

func gfsManageOK(req GFSManageRequest, target string, val any, results []GFSManageTargetResult) GFSManageResponse {
	return GFSManageResponse{
		Code:    http.StatusOK,
		Val:     val,
		Message: "ok",
		Action:  req.Action,
		Target:  target,
		Results: results,
	}
}

func gfsManageError(req GFSManageRequest, target string, message string, results []GFSManageTargetResult) GFSManageResponse {
	return GFSManageResponse{
		Code:    http.StatusInternalServerError,
		Val:     message,
		Message: message,
		Action:  req.Action,
		Target:  target,
		Results: results,
	}
}

func statusCodeFromGFSManageResponse(resp GFSManageResponse) int {
	if resp.Code == http.StatusOK {
		return http.StatusOK
	}
	if resp.Code == http.StatusBadRequest {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func runGFSManageCommand(timeout time.Duration, command string, args ...string) (string, error) {
	out, timedOut, err := runCommandOutputWithEnv(command, timeout, gfsManageCommandEnv(), args...)
	if timedOut {
		return out, fmt.Errorf("%s %s timed out after %s", command, strings.Join(args, " "), timeout)
	}
	if err != nil {
		return out, fmt.Errorf("%s %s failed: %s", command, strings.Join(args, " "), firstNonEmpty(strings.TrimSpace(out), err.Error()))
	}
	return out, nil
}

func runGFSManageCommandIgnore(command string, args ...string) (string, error) {
	return runGFSManageCommand(gfsManageCommandTimeout, command, args...)
}

func gfsManageCommandEnv() []string {
	return append([]string{"LANG=en_US.utf-8", "LANGUAGE=en"}, os.Environ()...)
}
