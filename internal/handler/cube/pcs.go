package cube

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ablecloud.io/ablestack-api/internal/infra/utils"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"github.com/gin-gonic/gin"
)

type CCVMPCSControlRequest = CubeModel.CCVMPCSControlRequest
type CCVMPCSControlResponse = CubeModel.CCVMPCSControlResponse
type CCVMPCSNodeStatus = CubeModel.CCVMPCSNodeStatus
type CCVMPCSStatusValue = CubeModel.CCVMPCSStatusValue

const (
	ccvmPCSSetupAction            = "setup"
	ccvmPCSSetupCronAction        = "setup-cron"
	ccvmPCSSetupClusterName       = "cloudcenter_cluster"
	ccvmPCSSetupSuccessMessage    = "cloud center setup success"
	ccvmPCSSetupFailureMessage    = "cloud center setup fail"
	ccvmPCSSetupTemplatePath      = "/var/lib/libvirt/images/ablestack-template-back.qcow2"
	ccvmPCSSetupQemuRBDTarget     = "rbd:rbd/ccvm"
	ccvmPCSSetupRBDImageSpec      = "rbd/ccvm"
	ccvmPCSSetupImageSize         = "500G"
	ccvmPCSSetupCommandTimeout    = 30 * time.Minute
	ccvmPCSSetupRemoteTimeout     = 60 * time.Minute
	ccvmPCSSetupWaitTimeout       = 25 * time.Minute
	ccvmPCSSetupPollInterval      = 5 * time.Second
	ccvmPCSSetupCronPath          = "/var/spool/cron/root"
	ccvmPCSSetupCronMarker        = "create_ccvm_snap.py"
	ccvmPCSSetupFilesystemOSType  = "ablestack-hci-filesystem"
	ccvmPCSSetupClusterConfigType = "ablestack-hci"
	ccvmPCSSetupVMOSType          = "ablestack-vm"
	ccvmPCSSetupVMTemplatePath    = "/var/lib/libvirt/images/ablestack-template.qcow2"
	ccvmPCSSetupVMRuntimeDir      = "/mnt/glue-gfs"
	ccvmPCSSetupVMImagePath       = "/mnt/glue-gfs/ccvm.qcow2"
	ccvmPCSSetupVMXMLPath         = "/mnt/glue-gfs/ccvm.xml"
	ccvmPCSSetupVMResizeSize      = "+350G"
)

// CCVMPCSControl godoc
//
//	@Summary		PCS Control
//	@Description	CCVM Pacemaker/PCS setup/config/create/enable/disable/move/cleanup/status/remove/destroy/stop/sync/ccvm-status 작업을 수행합니다. setup은 clusterConfig.pcsCluster 노드의 기존 Python 스냅샷 cron을 정리한 뒤 cloudcenter_res를 구성합니다. 자동 스냅샷은 API 내부 Go 스케줄러가 수행합니다.
//	@Tags			Cube-PCS
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CubeModel.CCVMPCSControlRequest	true	"pcs control request"
//	@Success		200	{object}	CubeModel.CCVMPCSControlResponse
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/pcs/control [post]
func CCVMPCSControl(context *gin.Context) {
	var req CCVMPCSControlRequest
	if err := context.ShouldBindJSON(&req); err != nil {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: "invalid request",
		})
		return
	}

	if err := normalizeCCVMPCSControlRequest(&req); err != nil {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	localRequest := isPCSUpdateLocalRequest(context)
	if req.Action == ccvmPCSSetupCronAction && !localRequest {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: "unsupported action",
		})
		return
	}

	if localRequest {
		resp := runCCVMPCSLocal(req, "local")
		context.JSON(statusCodeFromCCVMPCSResponse(resp), resp)
		return
	}

	cfg, cfgErr := loadClusterConfigSection()
	if req.Action == ccvmPCSSetupAction {
		if cfgErr != nil {
			context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
				ErrCode: http.StatusInternalServerError,
				Message: "failed to read cluster.json",
			})
			return
		}
		resp := setupCCVMPCS(req, cfg)
		context.JSON(statusCodeFromCCVMPCSResponse(resp), resp)
		return
	}

	target, ok := selectPCSExecutionTarget(cfg)
	if !ok || strings.TrimSpace(target.Target) == "" || isLocalTarget(target.Target) || target.Target == "local" {
		resp := runCCVMPCSLocal(req, firstNonEmpty(target.Target, "local"))
		context.JSON(statusCodeFromCCVMPCSResponse(resp), resp)
		return
	}

	resp, err := callCCVMPCSRemote(target.Target, req)
	if err != nil {
		resp = ccvmPCSError(req, target.Target, http.StatusInternalServerError, err.Error(), err.Error())
	}
	context.JSON(statusCodeFromCCVMPCSResponse(resp), resp)
}

func normalizeCCVMPCSControlRequest(req *CCVMPCSControlRequest) error {
	if req == nil {
		return fmt.Errorf("request required")
	}

	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "config":
		req.Action = "config"
	case ccvmPCSSetupAction:
		req.Action = ccvmPCSSetupAction
	case ccvmPCSSetupCronAction:
		req.Action = ccvmPCSSetupCronAction
	case "create":
		req.Action = "create"
	case "enable":
		req.Action = "enable"
	case "disable":
		req.Action = "disable"
	case "move":
		req.Action = "move"
	case "cleanup":
		req.Action = "cleanup"
	case "status":
		req.Action = "status"
	case "remove":
		req.Action = "remove"
	case "destroy":
		req.Action = "destroy"
	case "stop":
		req.Action = "stop"
	case "sync":
		req.Action = "sync"
	case "ccvm-status":
		req.Action = "ccvm-status"
	default:
		return fmt.Errorf("unsupported action")
	}

	req.Cluster = strings.TrimSpace(req.Cluster)
	req.Resource = strings.TrimSpace(req.Resource)
	req.XML = strings.TrimSpace(req.XML)
	req.Target = strings.TrimSpace(req.Target)
	req.Time = strings.TrimSpace(req.Time)
	req.Hosts = normalizeStringSlice(req.Hosts)

	switch req.Action {
	case "config":
		if req.Cluster == "" {
			return fmt.Errorf("cluster required")
		}
		if len(req.Hosts) == 0 {
			return fmt.Errorf("hosts required")
		}
	case "create":
		defaultCCVMPCSResource(req)
		if req.XML == "" {
			return fmt.Errorf("xml required")
		}
	case "enable", "disable", "cleanup", "status", "remove":
		defaultCCVMPCSResource(req)
	case "move":
		defaultCCVMPCSResource(req)
		if req.Target == "" {
			return fmt.Errorf("target required")
		}
	case "sync":
		if req.Time == "" {
			return fmt.Errorf("time required")
		}
	}
	return nil
}

func normalizeStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func defaultCCVMPCSResource(req *CCVMPCSControlRequest) {
	if strings.TrimSpace(req.Resource) == "" {
		req.Resource = pcsDefaultResourceID
	}
}

func runCCVMPCSLocal(req CCVMPCSControlRequest, target string) CCVMPCSControlResponse {
	switch req.Action {
	case "config":
		return configCCVMPCSCluster(req, target)
	case ccvmPCSSetupAction:
		return setupCCVMPCSLocal(req, target)
	case ccvmPCSSetupCronAction:
		return setupCCVMPCSCronLocal(req, target)
	case "create":
		return createCCVMPCSResource(req, target)
	case "enable":
		return enableCCVMPCSResource(req, target)
	case "disable":
		return disableCCVMPCSResource(req, target)
	case "move":
		return moveCCVMPCSResource(req, target)
	case "cleanup":
		return cleanupCCVMPCSResource(req, target)
	case "status":
		return statusCCVMPCSResource(req, target)
	case "remove":
		return removeCCVMPCSResource(req, target)
	case "destroy":
		return destroyCCVMPCSCluster(req, target)
	case "stop":
		return stopCCVMPCSCluster(req, target)
	case "sync":
		return syncCCVMPCSCluster(req, target)
	case "ccvm-status":
		return ccvmPCSVMStatus(req, target)
	default:
		return ccvmPCSError(req, target, http.StatusBadRequest, "unsupported action", "unsupported action")
	}
}

// setupCCVMPCS는 외부 setup 요청의 오케스트레이션을 담당한다.
// 각 노드 API를 호출해 기존 Python snapshot cron을 정리한 뒤 PCS 구성을 수행한다.
func setupCCVMPCS(req CCVMPCSControlRequest, cfg *CubeModel.ClusterConfigSection) CCVMPCSControlResponse {
	targets := buildPCSExecutionTargets(cfg)
	if len(targets) == 0 {
		return ccvmPCSError(req, "local", http.StatusBadRequest, "pcsCluster required", "pcsCluster required")
	}

	localName, _ := os.Hostname()
	cronResults := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		resp := setupCCVMPCSCronOnTarget(target, localName)
		cronResults = append(cronResults, setupCCVMPCSTargetResult(target, resp))
		if resp.Code != http.StatusOK {
			return ccvmPCSError(req, firstNonEmpty(target.Target, "local"), http.StatusInternalServerError, map[string]any{
				"cron": cronResults,
			}, firstNonEmpty(resp.Message, "legacy ccvm snapshot cron cleanup failed"))
		}
	}

	setupTarget, ok := selectPCSExecutionTarget(cfg)
	if !ok || strings.TrimSpace(setupTarget.Target) == "" {
		setupTarget = pcsExecutionTarget{Target: "local"}
	}

	setupResp := setupCCVMPCSOnTarget(req, setupTarget, localName)
	val := map[string]any{
		"cron":  cronResults,
		"setup": setupResp.Val,
	}
	if setupResp.Code != http.StatusOK {
		return ccvmPCSError(req, firstNonEmpty(setupResp.Target, setupTarget.Target, "local"), setupResp.Code, val, setupResp.Message)
	}
	return ccvmPCSOK(req, firstNonEmpty(setupResp.Target, setupTarget.Target, "local"), val)
}

func setupCCVMPCSOnTarget(req CCVMPCSControlRequest, target pcsExecutionTarget, localName string) CCVMPCSControlResponse {
	if shouldRunCCVMPCSLocally(target, localName) {
		return runCCVMPCSLocal(req, firstNonEmpty(target.Target, "local"))
	}
	resp, err := callCCVMPCSRemoteWithTimeout(target.Target, req, ccvmPCSSetupRemoteTimeout)
	if err != nil {
		return ccvmPCSError(req, target.Target, http.StatusInternalServerError, err.Error(), err.Error())
	}
	if strings.EqualFold(resp.Target, "local") {
		resp.Target = target.Target
	}
	return resp
}

func setupCCVMPCSCronOnTarget(target pcsExecutionTarget, localName string) CCVMPCSControlResponse {
	req := CCVMPCSControlRequest{Action: ccvmPCSSetupCronAction}
	if shouldRunCCVMPCSLocally(target, localName) {
		return runCCVMPCSLocal(req, firstNonEmpty(target.Target, "local"))
	}
	resp, err := callCCVMPCSRemote(target.Target, req)
	if err != nil {
		return ccvmPCSError(req, target.Target, http.StatusInternalServerError, err.Error(), err.Error())
	}
	if strings.EqualFold(resp.Target, "local") {
		resp.Target = target.Target
	}
	return resp
}

func shouldRunCCVMPCSLocally(target pcsExecutionTarget, localName string) bool {
	targetValue := strings.TrimSpace(target.Target)
	return targetValue == "" || targetValue == "local" || isLocalTarget(targetValue) || isPCSLocalExecutionTarget(target, localName)
}

func setupCCVMPCSTargetResult(target pcsExecutionTarget, resp CCVMPCSControlResponse) map[string]any {
	return map[string]any{
		"hostname": target.Hostname,
		"pcs_host": target.PCSHost,
		"target":   firstNonEmpty(resp.Target, target.Target, "local"),
		"code":     resp.Code,
		"message":  resp.Message,
		"val":      resp.Val,
	}
}

func setupCCVMPCSLocal(req CCVMPCSControlRequest, target string) CCVMPCSControlResponse {
	cfg, err := loadClusterConfigSection()
	if err != nil {
		return ccvmPCSError(req, target, http.StatusInternalServerError, ccvmPCSSetupFailureMessage, err.Error())
	}
	if err := setupCCVMPCSLocalSteps(target, cfg, !req.CreateOnly); err != nil {
		return ccvmPCSError(req, target, http.StatusInternalServerError, ccvmPCSSetupFailureMessage, err.Error())
	}
	return ccvmPCSOK(req, target, ccvmPCSSetupSuccessMessage)
}

func setupCCVMPCSCronLocal(req CCVMPCSControlRequest, target string) CCVMPCSControlResponse {
	if err := setupCCVMSnapshotCronLocal(); err != nil {
		return ccvmPCSError(req, target, http.StatusInternalServerError, err.Error(), err.Error())
	}
	return ccvmPCSOK(req, target, "legacy ccvm snapshot cron cleanup success")
}

// setupCCVMPCSLocalSteps는 Python setupPcsCluster의 로컬 작업을 Go 명령 실행으로 대체한다.
// 실제 PCS 리소스 생성은 한 노드에서만 실행하고, legacy cron 정리는 setupCCVMPCS에서 별도로 fan-out 한다.
func setupCCVMPCSLocalSteps(target string, cfg *CubeModel.ClusterConfigSection, start bool) error {
	if cfg == nil {
		return fmt.Errorf("clusterConfig not found")
	}

	clusterType := strings.ToLower(strings.TrimSpace(cfg.Type))
	resourceXMLPath := filepath.Join(resolveAbleStackVMConfigDir("ccvm"), "ccvm.xml")

	if clusterType == ccvmPCSSetupVMOSType {
		if err := prepareCCVMPCSVMSetup(); err != nil {
			return err
		}
		resourceXMLPath = ccvmPCSSetupVMXMLPath
	}

	if clusterType == ccvmPCSSetupFilesystemOSType {
		if err := prepareCCVMPCSFilesystemSetup(target); err != nil {
			return err
		}
	}

	if clusterType == ccvmPCSSetupClusterConfigType || clusterType == ccvmPCSSetupFilesystemOSType {
		if _, err := runPCSCommand(
			ccvmPCSSetupCommandTimeout,
			"qemu-img",
			"convert", "-f", "qcow2", "-O", "rbd", ccvmPCSSetupTemplatePath, ccvmPCSSetupQemuRBDTarget,
		); err != nil {
			return err
		}
		if _, err := runPCSCommand(ccvmPCSSetupCommandTimeout, "rbd", "resize", "-s", ccvmPCSSetupImageSize, ccvmPCSSetupRBDImageSpec); err != nil {
			return err
		}
	}

	if clusterType == ccvmPCSSetupClusterConfigType {
		hosts := ccvmPCSSetupClusterHosts(cfg)
		if len(hosts) == 0 {
			return fmt.Errorf("pcsCluster required")
		}
		resp := configCCVMPCSCluster(CCVMPCSControlRequest{
			Action:  "config",
			Cluster: ccvmPCSSetupClusterName,
			Hosts:   hosts,
		}, target)
		if resp.Code != http.StatusOK {
			return fmt.Errorf("pcs cluster config failed: %s", firstNonEmpty(resp.Message, fmt.Sprint(resp.Val)))
		}
	}

	resp := createCCVMPCSResource(CCVMPCSControlRequest{
		Action:   "create",
		Resource: pcsDefaultResourceID,
		XML:      resourceXMLPath,
		Disabled: !start,
	}, target)
	if resp.Code != http.StatusOK {
		return fmt.Errorf("pcs resource create failed: %s", firstNonEmpty(resp.Message, fmt.Sprint(resp.Val)))
	}

	if clusterType == ccvmPCSSetupVMOSType || clusterType == ccvmPCSSetupFilesystemOSType {
		// GFS가 먼저 올라온 뒤 CCVM 리소스가 시작되도록 순서를 고정한다.
		if _, err := runPCSCommand(pcsCommandTimeout, "pcs", "constraint", "order", "start", "glue-gfs-clone", "then", pcsDefaultResourceID); err != nil {
			return err
		}
	}

	if !start {
		return nil
	}
	_, err := waitForCCVMPCSResourceStarted(pcsDefaultResourceID, ccvmPCSSetupWaitTimeout)
	return err
}

func prepareCCVMPCSVMSetup() error {
	if _, err := runPCSCommand(ccvmPCSSetupCommandTimeout, "mountpoint", "-q", ccvmPCSSetupVMRuntimeDir); err != nil {
		return fmt.Errorf("gfs mount point is not mounted: %s", ccvmPCSSetupVMRuntimeDir)
	}

	sourceXML := filepath.Join(resolveAbleStackVMConfigDir("ccvm"), "ccvm.xml")
	if err := requireRegularFile(sourceXML, "ccvm xml not found"); err != nil {
		return err
	}
	if err := copySCVMLifecycleFile(sourceXML, ccvmPCSSetupVMXMLPath, 0o644); err != nil {
		return fmt.Errorf("prepare shared ccvm xml: %w", err)
	}

	if _, err := os.Stat(ccvmPCSSetupVMImagePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := requireRegularFile(ccvmPCSSetupVMTemplatePath, "ccvm template image not found"); err != nil {
		return err
	}
	if err := copySCVMLifecycleFile(ccvmPCSSetupVMTemplatePath, ccvmPCSSetupVMImagePath, 0o666); err != nil {
		return fmt.Errorf("prepare shared ccvm image: %w", err)
	}
	if _, err := runPCSCommand(ccvmPCSSetupCommandTimeout, "qemu-img", "resize", ccvmPCSSetupVMImagePath, ccvmPCSSetupVMResizeSize); err != nil {
		return err
	}
	return nil
}

func prepareCCVMPCSFilesystemSetup(target string) error {
	exists, err := ccvmPCSSetupRBDImageExists()
	if err != nil || !exists {
		return err
	}
	if _, err := runPCSCommand(ccvmPCSSetupCommandTimeout, "rbd", "rm", "--no-progress", ccvmPCSSetupRBDImageSpec); err != nil {
		return err
	}

	resp := removeCCVMPCSResource(CCVMPCSControlRequest{
		Action:   "remove",
		Resource: pcsDefaultResourceID,
	}, target)
	if resp.Code != http.StatusOK && !ccvmPCSSetupResourceNotFound(resp) {
		return fmt.Errorf("pcs resource remove failed: %s", firstNonEmpty(resp.Message, fmt.Sprint(resp.Val)))
	}
	return nil
}

func ccvmPCSSetupRBDImageExists() (bool, error) {
	out, timedOut, err := runCommandOutputWithEnv("rbd", pcsCommandTimeout, pcsCommandEnv(), "info", ccvmPCSSetupRBDImageSpec)
	if timedOut {
		return false, fmt.Errorf("rbd info %s timed out after %s", ccvmPCSSetupRBDImageSpec, pcsCommandTimeout)
	}
	if err != nil {
		msg := strings.ToLower(strings.TrimSpace(out + " " + err.Error()))
		if strings.Contains(msg, "not found") ||
			strings.Contains(msg, "no such file") ||
			strings.Contains(msg, "does not exist") ||
			strings.Contains(msg, "doesn't exist") ||
			strings.Contains(msg, "error opening image") {
			return false, nil
		}
		return false, fmt.Errorf("rbd info %s failed: %s", ccvmPCSSetupRBDImageSpec, firstNonEmpty(out, err.Error()))
	}
	return true, nil
}

func ccvmPCSSetupResourceNotFound(resp CCVMPCSControlResponse) bool {
	msg := strings.ToLower(fmt.Sprint(resp.Val) + " " + resp.Message)
	return strings.Contains(msg, "resource not found") || strings.Contains(msg, "not found")
}

func ccvmPCSSetupClusterHosts(cfg *CubeModel.ClusterConfigSection) []string {
	if cfg == nil {
		return nil
	}
	return cfg.PCSCluster.HostnameList()
}

// setupCCVMSnapshotCronLocal은 Go scheduler와 중복되는 기존 Python snapshot cron을 제거한다.
func setupCCVMSnapshotCronLocal() error {
	return cleanupLegacyPythonCrontab(ccvmPCSSetupCronMarker)
}

func waitForCCVMPCSResourceStarted(resourceName string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	lastMessage := ""
	for {
		_, resource, err := loadCCVMPCSStatusAndResource(resourceName)
		if err == nil {
			startedNode := strings.TrimSpace(resource.Node.Name)
			if isCCVMPCSResourceStarted(resource) {
				return startedNode, nil
			}
			lastMessage = fmt.Sprintf(
				"role=%s nodes_running_on=%s node=%s active=%s failed=%s blocked=%s",
				resource.Role,
				resource.NodesRunningOn,
				startedNode,
				resource.Active,
				resource.Failed,
				resource.Blocked,
			)
		} else {
			lastMessage = err.Error()
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("wait pcs resource %s started timed out: %s", resourceName, lastMessage)
		}
		time.Sleep(ccvmPCSSetupPollInterval)
	}
}

func isCCVMPCSResourceStarted(resource ccvmSnapPCSResource) bool {
	return strings.EqualFold(strings.TrimSpace(resource.Role), "Started") &&
		strings.TrimSpace(resource.NodesRunningOn) == "1" &&
		strings.TrimSpace(resource.Node.Name) != ""
}

func configCCVMPCSCluster(req CCVMPCSControlRequest, target string) CCVMPCSControlResponse {
	if _, err := runPCSCommand(pcsCommandTimeout, "pcs", append([]string{"host", "auth", "-u", "hacluster", "-p", "password"}, req.Hosts...)...); err != nil {
		return ccvmPCSError(req, target, http.StatusInternalServerError, err.Error(), err.Error())
	}
	if _, err := runPCSCommand(pcsCommandTimeout, "pcs", append([]string{"cluster", "setup", req.Cluster}, req.Hosts...)...); err != nil {
		return ccvmPCSError(req, target, http.StatusInternalServerError, err.Error(), err.Error())
	}
	commands := [][]string{
		{"pcs", "cluster", "start", "--all"},
		{"systemctl", "enable", "--now", "corosync.service"},
		{"systemctl", "enable", "--now", "pacemaker.service"},
		{"pcs", "property", "set", "stonith-enabled=false"},
		{"pcs", "resource", "defaults", "update", "resource-stickiness=1"},
	}
	for _, command := range commands {
		if _, err := runPCSCommand(pcsCommandTimeout, command[0], command[1:]...); err != nil {
			return ccvmPCSError(req, target, http.StatusInternalServerError, err.Error(), err.Error())
		}
	}
	return ccvmPCSOK(req, target, map[string]any{"cluster name :": req.Cluster, "hosts": req.Hosts})
}

func createCCVMPCSResource(req CCVMPCSControlRequest, target string) CCVMPCSControlResponse {
	args := []string{
		"resource", "create", req.Resource, "VirtualDomain",
		"hypervisor=qemu:///system",
		"config=" + req.XML,
		"migration_transport=ssh",
		"meta", "allow-migrate=true", "priority=100",
		"op", "start", "timeout=120s",
		"op", "stop", "timeout=120s",
		"op", "monitor", "timeout=30", "interval=10",
	}
	if req.Disabled {
		args = append(args, "--disabled")
	}
	if _, err := runPCSCommand(pcsCommandTimeout, "pcs", args...); err != nil {
		return ccvmPCSError(req, target, http.StatusInternalServerError, err.Error(), err.Error())
	}
	return ccvmPCSOK(req, target, map[string]any{
		"resource name :":     req.Resource,
		"hypervisor":          "qemu:///system",
		"config":              req.XML,
		"migration_transport": "ssh",
	})
}

func enableCCVMPCSResource(req CCVMPCSControlRequest, target string) CCVMPCSControlResponse {
	if _, err := runPCSCommand(pcsCommandTimeout, "pcs", "resource", "cleanup", req.Resource); err != nil {
		return ccvmPCSError(req, target, http.StatusInternalServerError, err.Error(), err.Error())
	}
	if _, err := runPCSCommand(pcsCommandTimeout, "pcs", "resource", "enable", req.Resource); err != nil {
		return ccvmPCSError(req, target, http.StatusInternalServerError, err.Error(), err.Error())
	}
	startedNode, err := waitForCCVMPCSResourceStarted(req.Resource, ccvmPCSSetupWaitTimeout)
	if err != nil {
		return ccvmPCSError(req, target, http.StatusInternalServerError, err.Error(), err.Error())
	}
	return ccvmPCSOK(req, target, map[string]any{"status": "enable", "started": startedNode})
}

func disableCCVMPCSResource(req CCVMPCSControlRequest, target string) CCVMPCSControlResponse {
	if _, err := runPCSCommand(pcsCommandTimeout, "pcs", "resource", "disable", req.Resource); err != nil {
		return ccvmPCSError(req, target, http.StatusInternalServerError, err.Error(), err.Error())
	}
	return ccvmPCSOK(req, target, "disable")
}

func moveCCVMPCSResource(req CCVMPCSControlRequest, target string) CCVMPCSControlResponse {
	_, resource, err := loadCCVMPCSStatusAndResource(req.Resource)
	if err != nil {
		return ccvmPCSError(req, target, http.StatusBadRequest, err.Error(), err.Error())
	}

	currentHost := ""
	if resource.NodesRunningOn == "1" {
		currentHost = strings.TrimSpace(resource.Node.Name)
	}
	if currentHost == req.Target {
		val := "cannot be migrated to the same host."
		return ccvmPCSError(req, target, http.StatusInternalServerError, val, val)
	}
	if currentHost == "" {
		val := "Migration is not possible while stopped."
		return ccvmPCSError(req, target, http.StatusNotImplemented, val, val)
	}

	if _, err := runPCSCommand(pcsCommandTimeout, "pcs", "resource", "move", req.Resource, req.Target); err != nil {
		return ccvmPCSError(req, target, http.StatusInternalServerError, err.Error(), err.Error())
	}
	return ccvmPCSOK(req, target, map[string]any{"current host": currentHost, "target host": req.Target})
}

func cleanupCCVMPCSResource(req CCVMPCSControlRequest, target string) CCVMPCSControlResponse {
	if _, err := runPCSCommand(pcsCommandTimeout, "pcs", "resource", "cleanup", req.Resource); err != nil {
		return ccvmPCSError(req, target, http.StatusInternalServerError, err.Error(), err.Error())
	}
	if _, err := runPCSCommand(pcsCommandTimeout, "pcs", "stonith", "history", "cleanup"); err != nil {
		return ccvmPCSError(req, target, http.StatusInternalServerError, err.Error(), err.Error())
	}
	return ccvmPCSOK(req, target, "cleanup")
}

func statusCCVMPCSResource(req CCVMPCSControlRequest, target string) CCVMPCSControlResponse {
	status, resource, err := loadCCVMPCSStatusAndResource(req.Resource)
	if err != nil {
		return ccvmPCSError(req, target, http.StatusBadRequest, err.Error(), err.Error())
	}

	nodes := make([]CCVMPCSNodeStatus, 0, len(status.Nodes.Node))
	nodeList := make([]string, 0, len(status.Nodes.Node))
	for _, node := range status.Nodes.Node {
		nodeList = append(nodeList, node.Name)
		nodes = append(nodes, CCVMPCSNodeStatus{
			Host:             node.Name,
			Online:           node.Online,
			ResourcesRunning: node.ResourcesRunning,
			Standby:          node.Standby,
			StandbyOnfail:    node.StandbyOnfail,
			Maintenance:      node.Maintenance,
			Pending:          node.Pending,
			Unclean:          node.Unclean,
			Shutdown:         node.Shutdown,
			ExpectedUp:       node.ExpectedUp,
			IsDC:             node.IsDC,
			Type:             node.Type,
		})
	}

	started := ""
	if resource.NodesRunningOn == "1" {
		started = strings.TrimSpace(resource.Node.Name)
	}
	return ccvmPCSOK(req, target, CCVMPCSStatusValue{
		ClusteredHost: nodeList,
		Nodes:         nodes,
		Started:       started,
		Role:          resource.Role,
		Active:        resource.Active,
		Blocked:       resource.Blocked,
		Failed:        resource.Failed,
	})
}

func removeCCVMPCSResource(req CCVMPCSControlRequest, target string) CCVMPCSControlResponse {
	commands := [][]string{
		{"pcs", "resource", "cleanup", req.Resource},
		{"pcs", "resource", "disable", req.Resource},
		{"pcs", "resource", "remove", req.Resource},
		{"pcs", "resource", "refresh"},
	}
	for _, command := range commands {
		if _, err := runPCSCommand(pcsCommandTimeout, command[0], command[1:]...); err != nil {
			val := "resource not found."
			return ccvmPCSError(req, target, http.StatusBadRequest, val, val)
		}
	}
	return ccvmPCSOK(req, target, "remove")
}

func destroyCCVMPCSCluster(req CCVMPCSControlRequest, target string) CCVMPCSControlResponse {
	if _, err := runPCSCommand(pcsCommandTimeout, "pcs", "cluster", "destroy", "--all"); err != nil {
		val := "cluster not found."
		return ccvmPCSError(req, target, http.StatusBadRequest, val, val)
	}
	return ccvmPCSOK(req, target, "destroy")
}

func stopCCVMPCSCluster(req CCVMPCSControlRequest, target string) CCVMPCSControlResponse {
	if _, err := runPCSCommand(pcsCommandTimeout, "pcs", "cluster", "stop", "--all"); err != nil {
		val := "cluster not found."
		return ccvmPCSError(req, target, http.StatusBadRequest, val, val)
	}
	return ccvmPCSOK(req, target, "stop")
}

func syncCCVMPCSCluster(req CCVMPCSControlRequest, target string) CCVMPCSControlResponse {
	if _, err := runPCSCommand(pcsCommandTimeout, "pcs", "cluster", "config", "update", "totem", "token="+req.Time); err != nil {
		val := "Cloud VM Cluster Sync Mechanism Failed."
		return ccvmPCSError(req, target, http.StatusBadRequest, val, err.Error())
	}
	return ccvmPCSOK(req, target, "Cloud VM Cluster Sync Mechanism Success")
}

func ccvmPCSVMStatus(req CCVMPCSControlRequest, target string) CCVMPCSControlResponse {
	_, exists, err := readLocalCCVMState()
	if err != nil {
		message := "Failed to check CloudCenter VM status: " + err.Error()
		return ccvmPCSError(req, target, http.StatusInternalServerError, message, message)
	}
	if !exists {
		message := "The CloudCenter VM has not been created."
		return ccvmPCSError(req, target, http.StatusInternalServerError, message, message)
	}
	return ccvmPCSOK(req, target, "The CloudCenter virtual machine has been successfully created.")
}

func loadCCVMPCSStatusAndResource(resourceName string) (ccvmSnapPCSStatusXML, ccvmSnapPCSResource, error) {
	status, err := loadCCVMPcsStatusXML()
	if err != nil {
		return ccvmSnapPCSStatusXML{}, ccvmSnapPCSResource{}, fmt.Errorf("cluster is not configured.")
	}
	for _, resource := range collectCCVMSnapPCSResources(status) {
		if strings.TrimSpace(resource.ID) == resourceName {
			return status, resource, nil
		}
	}
	return status, ccvmSnapPCSResource{}, fmt.Errorf("resource not found.")
}
