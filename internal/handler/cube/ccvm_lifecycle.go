package cube

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ablecloud.io/ablestack-api/internal/infra/utils"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"github.com/gin-gonic/gin"
)

type CCVMLifecycleRequest = CubeModel.CCVMLifecycleRequest
type CCVMLifecycleResponse = CubeModel.CCVMLifecycleResponse

const (
	ccvmLifecycleRetName           = "CCVM Lifecycle"
	resetCloudCenterCommandTimeout = 5 * time.Minute
	resetCloudCenterShortTimeout   = 30 * time.Second
	resetCloudCenterCloudInitISO   = "/var/lib/libvirt/images/ccvm-cloudinit.iso"
	resetCloudCenterSuccessHCI     = "cloud center reset success"
	resetCloudCenterFailHCI        = "cloud center reset fail"
	resetCloudCenterSuccessGFS     = "cloud center and gfs disk reset success"
	resetCloudCenterFailGFS        = "cloud center and gfs disk reset fail"
	resetCloudCenterSuccessLocal   = "cloud center and local disk reset success"
	resetCloudCenterFailLocal      = "cloud center and local disk reset fail"
	localCCVMTemplateImage         = "/var/lib/libvirt/images/ablestack-template-back.qcow2"
	localCCVMRuntimeDir            = "/mnt/glue"
	localCCVMImagePath             = "/mnt/glue/ccvm.qcow2"
	localCCVMXMLPath               = "/mnt/glue/ccvm.xml"
	localCCVMResizeSize            = "+350G"
	ccvmInitializeGFSDirectory     = "/mnt/glue-gfs"
)

var (
	resetCloudCenterPSuffixPartition     = regexp.MustCompile(`p[0-9]+$`)
	resetCloudCenterAlphaSuffixPartition = regexp.MustCompile(`[a-z][0-9]+$`)
	resetCloudCenterNumberSuffix         = regexp.MustCompile(`[0-9]+$`)
)

// CCVMLifecycle godoc
//
//	@Summary		CCVM Lifecycle
//	@Description	Cloud Center VM lifecycle 작업을 수행합니다. 사용 가능한 action: initialize, setup, reset, copy, start, stop, restart, delete. initialize는 기존 cloudcenter_res와 /mnt/glue-gfs/ccvm*만 정리하며, reset은 clusterConfig.type에 따라 PCS/GFS/local disk 설정까지 초기화합니다.
//	@Tags			Cube-CCVM
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CubeModel.CCVMLifecycleRequest	true	"ccvm lifecycle request"
//	@Success		200	{object}	CubeModel.CCVMLifecycleResponse
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/ccvm/lifecycle [post]
func CCVMLifecycle(context *gin.Context) {
	req, err := bindCCVMLifecycleRequest(context)
	if err != nil {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: err.Error(),
		})
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

	resp := runCCVMLifecycle(req, cfg)
	context.JSON(statusCodeFromCCVMLifecycleResponse(resp), resp)
}

func bindCCVMLifecycleRequest(context *gin.Context) (CCVMLifecycleRequest, error) {
	req := CCVMLifecycleRequest{}
	if context.Request == nil || context.Request.Body == nil {
		return req, fmt.Errorf("request required")
	}
	raw, err := io.ReadAll(context.Request.Body)
	if err != nil {
		return req, err
	}
	if strings.TrimSpace(string(raw)) == "" {
		return req, fmt.Errorf("request required")
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return req, fmt.Errorf("invalid request")
	}
	if err := normalizeCCVMLifecycleRequest(&req); err != nil {
		return req, err
	}
	return req, nil
}

func normalizeCCVMLifecycleRequest(req *CCVMLifecycleRequest) error {
	if req == nil {
		return fmt.Errorf("request required")
	}
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "initialize":
		req.Action = "initialize"
	case "setup":
		req.Action = "setup"
	case "reset":
		req.Action = "reset"
	case "copy":
		req.Action = "copy"
	case "start":
		req.Action = "start"
	case "stop":
		req.Action = "stop"
	case "restart":
		req.Action = "restart"
	case "delete":
		req.Action = "delete"
	default:
		return fmt.Errorf("unsupported action")
	}
	req.Disk = strings.TrimSpace(req.Disk)
	return nil
}

func runCCVMLifecycle(req CCVMLifecycleRequest, cfg *CubeModel.ClusterConfigSection) CCVMLifecycleResponse {
	if cfg == nil {
		return resetCloudCenterError("", "cloud center lifecycle fail", "clusterConfig not found", nil)
	}

	osType := strings.ToLower(strings.TrimSpace(cfg.Type))
	var resp CCVMLifecycleResponse
	switch req.Action {
	case "initialize":
		resp = runCCVMInitializeLifecycle(req, cfg)
	case "setup":
		resp = runCCVMSetupLifecycle(req, cfg)
	case "reset":
		resp = runResetCloudCenter(req, cfg)
	case "copy":
		resp = runCCVMCopyLifecycle(req, cfg)
	case "start":
		resp = runCCVMStartLifecycle(req, cfg)
	case "stop":
		resp = runCCVMStopLifecycle(req, cfg)
	case "restart":
		resp = runCCVMRestartLifecycle(req, cfg)
	case "delete":
		resp = runCCVMDeleteLifecycle(req, cfg)
	default:
		resp = resetCloudCenterError(osType, "cloud center lifecycle fail", "unsupported action", nil)
	}
	resp.Action = req.Action
	if resp.OSType == "" {
		resp.OSType = osType
	}
	if resp.Code == http.StatusOK && shouldRefreshCCVMKnownHosts(req.Action) {
		scheduleSSHKnownHostsScanForHosts(ccvmKnownHostsTargets(cfg))
	}
	return resp
}

func runCCVMInitializeLifecycle(req CCVMLifecycleRequest, cfg *CubeModel.ClusterConfigSection) CCVMLifecycleResponse {
	osType := strings.ToLower(strings.TrimSpace(cfg.Type))
	statusResp := runCCVMLifecyclePCSAction(cfg, CCVMPCSControlRequest{
		Action:   "status",
		Resource: pcsDefaultResourceID,
	})

	results := make([]CubeModel.ClusterApplyResult, 0, 1)
	resourceRemoved := false
	switch statusResp.Code {
	case http.StatusOK:
		removeResp := runCCVMLifecyclePCSAction(cfg, CCVMPCSControlRequest{
			Action:   "remove",
			Resource: pcsDefaultResourceID,
		})
		if removeResp.Code != http.StatusOK {
			return resetCloudCenterError(osType, "cloud center initialize fail", firstNonEmpty(removeResp.Message, fmt.Sprint(removeResp.Val)), nil)
		}
		results = append(results, CubeModel.ClusterApplyResult{
			Target:  firstNonEmpty(removeResp.Target, "local"),
			Code:    http.StatusOK,
			Message: pcsDefaultResourceID + " removed",
		})
		resourceRemoved = true
	case http.StatusBadRequest:
		// PCS 또는 cloudcenter_res가 아직 구성되지 않은 재시도는 정상 초기화로 처리한다.
		results = append(results, CubeModel.ClusterApplyResult{
			Target:  firstNonEmpty(statusResp.Target, "local"),
			Code:    http.StatusOK,
			Message: pcsDefaultResourceID + " not found; skipped",
		})
	default:
		return resetCloudCenterError(osType, "cloud center initialize fail", firstNonEmpty(statusResp.Message, fmt.Sprint(statusResp.Val)), nil)
	}

	removed, err := removeCCVMInitializeFiles(ccvmInitializeGFSDirectory)
	if err != nil {
		return resetCloudCenterError(osType, "cloud center initialize fail", err.Error(), results)
	}
	return resetCloudCenterOK(osType, fmt.Sprintf("cloud center initialize success: resource_removed=%t removed_files=%d", resourceRemoved, removed), results)
}

func removeCCVMInitializeFiles(directory string) (int, error) {
	matches, err := filepath.Glob(filepath.Join(directory, "ccvm*"))
	if err != nil {
		return 0, fmt.Errorf("find ccvm files: %w", err)
	}
	removed := 0
	for _, path := range matches {
		if err := os.RemoveAll(path); err != nil {
			return removed, fmt.Errorf("remove %s: %w", path, err)
		}
		removed++
	}
	return removed, nil
}

func shouldRefreshCCVMKnownHosts(action string) bool {
	switch action {
	case "setup", "reset", "start", "restart":
		return true
	default:
		return false
	}
}

func ccvmKnownHostsTargets(cfg *CubeModel.ClusterConfigSection) []string {
	hosts := []string{"ccvm", "ccvm-mngt"}
	if cfg != nil {
		hosts = append(hosts, strings.TrimSpace(cfg.CCVM.IP))
	}
	return dedupeHosts(hosts)
}

func runResetCloudCenter(req CCVMLifecycleRequest, cfg *CubeModel.ClusterConfigSection) CCVMLifecycleResponse {
	if cfg == nil {
		return resetCloudCenterError("", resetCloudCenterFailHCI, "clusterConfig not found", nil)
	}

	osType := strings.ToLower(strings.TrimSpace(cfg.Type))
	switch osType {
	case "ablestack-hci":
		if err := resetCloudCenterHCI(); err != nil {
			return resetCloudCenterError(osType, resetCloudCenterFailHCI, err.Error(), nil)
		}
		return resetCloudCenterOK(osType, resetCloudCenterSuccessHCI, nil)
	case "ablestack-vm":
		if err := resetCloudCenterVM(req, cfg); err != nil {
			return resetCloudCenterError(osType, resetCloudCenterFailGFS, err.Error(), nil)
		}
		results, err := resetCloudCenterApplyGFSSystemFlags(cfg)
		if err != nil {
			return resetCloudCenterError(osType, resetCloudCenterFailGFS, err.Error(), results)
		}
		return resetCloudCenterOK(osType, resetCloudCenterSuccessGFS, results)
	case "ablestack-standalone":
		if err := resetCloudCenterStandalone(); err != nil {
			return resetCloudCenterError(osType, resetCloudCenterFailLocal, err.Error(), nil)
		}
		results, err := resetCloudCenterApplyLocalSystemFlags([]resetCloudCenterSystemFlag{
			{Depth1: "bootstrap", Depth2: "ccvm", Value: "false"},
		})
		if err != nil {
			return resetCloudCenterError(osType, resetCloudCenterFailLocal, err.Error(), results)
		}
		return resetCloudCenterOK(osType, resetCloudCenterSuccessLocal, results)
	case "ablestack-hci-filesystem":
		if err := resetCloudCenterHCIFilesystem(req, cfg); err != nil {
			return resetCloudCenterError(osType, resetCloudCenterFailGFS, err.Error(), nil)
		}
		results, err := resetCloudCenterApplyGFSSystemFlags(cfg)
		if err != nil {
			return resetCloudCenterError(osType, resetCloudCenterFailGFS, err.Error(), results)
		}
		return resetCloudCenterOK(osType, resetCloudCenterSuccessGFS, results)
	default:
		return resetCloudCenterError(osType, "cloud center reset fail", "unsupported cluster type", nil)
	}
}

func runCCVMSetupLifecycle(req CCVMLifecycleRequest, cfg *CubeModel.ClusterConfigSection) CCVMLifecycleResponse {
	osType := strings.ToLower(strings.TrimSpace(cfg.Type))
	if resetCloudCenterIsStandalone(cfg) {
		if err := setupCCVMLocal(); err != nil {
			return resetCloudCenterError(osType, "cloud center setup fail", err.Error(), nil)
		}
		return resetCloudCenterOK(osType, "cloud center setup success", nil)
	}

	resp := setupCCVMPCS(CCVMPCSControlRequest{Action: "setup"}, cfg)
	if resp.Code != http.StatusOK {
		return resetCloudCenterError(osType, "cloud center setup fail", firstNonEmpty(resp.Message, fmt.Sprint(resp.Val)), nil)
	}
	return resetCloudCenterOK(osType, "cloud center setup success", []CubeModel.ClusterApplyResult{{
		Target:  firstNonEmpty(resp.Target, "local"),
		Code:    http.StatusOK,
		Message: "ok",
	}})
}

func runCCVMCopyLifecycle(req CCVMLifecycleRequest, cfg *CubeModel.ClusterConfigSection) CCVMLifecycleResponse {
	osType := strings.ToLower(strings.TrimSpace(cfg.Type))
	if !resetCloudCenterIsStandalone(cfg) {
		return resetCloudCenterError(osType, "cloud center copy fail", "copy action supports ablestack-standalone only", nil)
	}
	if err := copyCCVMLocalFiles(); err != nil {
		return resetCloudCenterError(osType, "cloud center copy fail", err.Error(), nil)
	}
	return resetCloudCenterOK(osType, "cloud center copy success", nil)
}

func runCCVMStartLifecycle(req CCVMLifecycleRequest, cfg *CubeModel.ClusterConfigSection) CCVMLifecycleResponse {
	osType := strings.ToLower(strings.TrimSpace(cfg.Type))
	if resetCloudCenterIsStandalone(cfg) {
		if err := startCCVMLocal(); err != nil {
			return resetCloudCenterError(osType, "cloud center start fail", err.Error(), nil)
		}
		return resetCloudCenterOK(osType, "cloud center start success", nil)
	}
	resp := runCCVMLifecyclePCSAction(cfg, CCVMPCSControlRequest{
		Action:   "enable",
		Resource: pcsDefaultResourceID,
	})
	if resp.Code != http.StatusOK {
		return resetCloudCenterError(osType, "cloud center start fail", firstNonEmpty(resp.Message, fmt.Sprint(resp.Val)), nil)
	}
	return resetCloudCenterOK(osType, "cloud center start success", []CubeModel.ClusterApplyResult{{
		Target:  firstNonEmpty(resp.Target, "local"),
		Code:    http.StatusOK,
		Message: "ok",
	}})
}

func runCCVMStopLifecycle(req CCVMLifecycleRequest, cfg *CubeModel.ClusterConfigSection) CCVMLifecycleResponse {
	osType := strings.ToLower(strings.TrimSpace(cfg.Type))
	if resetCloudCenterIsStandalone(cfg) {
		if err := stopCCVMLocal(req.Destroy); err != nil {
			return resetCloudCenterError(osType, "cloud center stop fail", err.Error(), nil)
		}
		return resetCloudCenterOK(osType, "cloud center stop success", nil)
	}
	resp := runCCVMLifecyclePCSAction(cfg, CCVMPCSControlRequest{
		Action:   "disable",
		Resource: pcsDefaultResourceID,
	})
	if resp.Code != http.StatusOK {
		return resetCloudCenterError(osType, "cloud center stop fail", firstNonEmpty(resp.Message, fmt.Sprint(resp.Val)), nil)
	}
	return resetCloudCenterOK(osType, "cloud center stop success", []CubeModel.ClusterApplyResult{{
		Target:  firstNonEmpty(resp.Target, "local"),
		Code:    http.StatusOK,
		Message: "ok",
	}})
}

func runCCVMRestartLifecycle(req CCVMLifecycleRequest, cfg *CubeModel.ClusterConfigSection) CCVMLifecycleResponse {
	stopResp := runCCVMStopLifecycle(req, cfg)
	if stopResp.Code != http.StatusOK {
		stopResp.Action = req.Action
		return stopResp
	}
	startResp := runCCVMStartLifecycle(req, cfg)
	if startResp.Code != http.StatusOK {
		startResp.Action = req.Action
		return startResp
	}
	startResp.Val = "cloud center restart success"
	startResp.Message = "ok"
	startResp.Action = req.Action
	return startResp
}

func runCCVMDeleteLifecycle(req CCVMLifecycleRequest, cfg *CubeModel.ClusterConfigSection) CCVMLifecycleResponse {
	osType := strings.ToLower(strings.TrimSpace(cfg.Type))
	if resetCloudCenterIsStandalone(cfg) {
		if err := deleteCCVMLocal(req.Purge); err != nil {
			return resetCloudCenterError(osType, "cloud center delete fail", err.Error(), nil)
		}
		if err := resetCloudCenterPrepareWorkspace(true); err != nil {
			return resetCloudCenterError(osType, "cloud center delete fail", err.Error(), nil)
		}
		return resetCloudCenterOK(osType, "cloud center delete success", nil)
	}

	resp := runCCVMLifecyclePCSAction(cfg, CCVMPCSControlRequest{
		Action:   "remove",
		Resource: pcsDefaultResourceID,
	})
	if resp.Code != http.StatusOK && resp.Code != http.StatusBadRequest {
		return resetCloudCenterError(osType, "cloud center delete fail", firstNonEmpty(resp.Message, fmt.Sprint(resp.Val)), nil)
	}
	if err := resetCloudCenterRemoveRBDCCVM(); err != nil {
		return resetCloudCenterError(osType, "cloud center delete fail", err.Error(), nil)
	}
	resetCloudCenterResetVirshDomain()
	if err := resetCloudCenterPrepareWorkspace(true); err != nil {
		return resetCloudCenterError(osType, "cloud center delete fail", err.Error(), nil)
	}
	return resetCloudCenterOK(osType, "cloud center delete success", []CubeModel.ClusterApplyResult{{
		Target:  firstNonEmpty(resp.Target, "local"),
		Code:    http.StatusOK,
		Message: "ok",
	}})
}

func runCCVMLifecyclePCSAction(cfg *CubeModel.ClusterConfigSection, req CCVMPCSControlRequest) CCVMPCSControlResponse {
	target, ok := selectPCSExecutionTarget(cfg)
	if !ok || strings.TrimSpace(target.Target) == "" || isLocalTarget(target.Target) || target.Target == "local" {
		return runCCVMPCSLocal(req, firstNonEmpty(target.Target, "local"))
	}
	resp, err := callCCVMPCSRemote(target.Target, req)
	if err != nil {
		return ccvmPCSError(req, target.Target, http.StatusInternalServerError, err.Error(), err.Error())
	}
	return resp
}

func resetCloudCenterIsStandalone(cfg *CubeModel.ClusterConfigSection) bool {
	return cfg != nil && strings.EqualFold(strings.TrimSpace(cfg.Type), "ablestack-standalone")
}

func setupCCVMLocal() error {
	if err := copyCCVMLocalFiles(); err != nil {
		return err
	}
	return createCCVMLocal()
}

func copyCCVMLocalFiles() error {
	targetDir := resolveAbleStackVMConfigDir("ccvm")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}

	files := []struct {
		src  string
		dst  string
		mode os.FileMode
	}{
		{src: "/etc/hosts", dst: filepath.Join(targetDir, "hosts"), mode: 0o644},
		{src: "/root/.ssh/id_rsa", dst: filepath.Join(targetDir, "id_rsa"), mode: 0o600},
		{src: "/root/.ssh/id_rsa.pub", dst: filepath.Join(targetDir, "id_rsa.pub"), mode: 0o644},
	}
	for _, file := range files {
		if err := copySCVMLifecycleFile(file.src, file.dst, file.mode); err != nil {
			return fmt.Errorf("copy %s: %w", filepath.Base(file.src), err)
		}
	}
	return nil
}

func createCCVMLocal() error {
	state, exists, err := readLocalCCVMState()
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("ccvm domain already exists: state=%s", strings.TrimSpace(state))
	}

	sourceXML := filepath.Join(resolveAbleStackVMConfigDir("ccvm"), "ccvm.xml")
	if err := requireRegularFile(sourceXML, "ccvm xml not found"); err != nil {
		return err
	}
	if err := requireRegularFile(localCCVMTemplateImage, "ccvm template image not found"); err != nil {
		return err
	}
	if err := os.MkdirAll(localCCVMRuntimeDir, 0o755); err != nil {
		return err
	}
	if err := copySCVMLifecycleFile(localCCVMTemplateImage, localCCVMImagePath, 0o666); err != nil {
		return fmt.Errorf("prepare ccvm image: %w", err)
	}
	if err := copySCVMLifecycleFile(sourceXML, localCCVMXMLPath, 0o644); err != nil {
		return fmt.Errorf("prepare ccvm xml: %w", err)
	}
	if _, err := runCCVMLifecycleCommand(resetCloudCenterCommandTimeout, "qemu-img", "resize", localCCVMImagePath, localCCVMResizeSize); err != nil {
		return err
	}
	if _, err := runCCVMLifecycleCommand(resetCloudCenterCommandTimeout, "virsh", "define", "--file", localCCVMXMLPath); err != nil {
		return err
	}
	if _, err := runCCVMLifecycleCommand(resetCloudCenterCommandTimeout, "virsh", "autostart", ccvmSnapName); err != nil {
		return err
	}
	if _, err := runCCVMLifecycleCommand(resetCloudCenterCommandTimeout, "virsh", "start", ccvmSnapName); err != nil {
		return err
	}
	return waitForCCVMState(resetCloudCenterCommandTimeout, "running")
}

func startCCVMLocal() error {
	state, exists, err := readLocalCCVMState()
	if err != nil {
		return err
	}
	if exists && strings.EqualFold(state, "running") {
		return nil
	}
	if _, err := runCCVMLifecycleCommand(resetCloudCenterCommandTimeout, "virsh", "start", ccvmSnapName); err != nil {
		return err
	}
	return waitForCCVMState(resetCloudCenterCommandTimeout, "running")
}

func stopCCVMLocal(destroy bool) error {
	state, exists, err := readLocalCCVMState()
	if err != nil {
		return err
	}
	if exists && strings.EqualFold(state, "shut off") {
		return nil
	}
	if !exists {
		return fmt.Errorf("ccvm domain not found")
	}

	action := "shutdown"
	if destroy {
		action = "destroy"
	}
	if _, err := runCCVMLifecycleCommand(resetCloudCenterCommandTimeout, "virsh", action, ccvmSnapName); err != nil {
		return err
	}
	return waitForCCVMState(resetCloudCenterCommandTimeout, "shut off")
}

func deleteCCVMLocal(purge bool) error {
	_, _ = runCCVMLifecycleCommand(resetCloudCenterShortTimeout, "virsh", "destroy", ccvmSnapName)
	if _, err := runCCVMLifecycleCommand(resetCloudCenterShortTimeout, "virsh", "undefine", ccvmSnapName, "--nvram"); err != nil {
		if !isCCVMDomainNotFoundError(err) {
			return err
		}
	}
	if err := waitForCCVMDomainGone(resetCloudCenterCommandTimeout); err != nil {
		return err
	}
	if purge {
		for _, imagePath := range []string{localCCVMImagePath, "/var/lib/libvirt/images/ccvm.qcow2"} {
			if err := os.Remove(imagePath); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func waitForCCVMState(timeout time.Duration, desired string) error {
	deadline := time.Now().Add(timeout)
	for {
		state, exists, err := readLocalCCVMState()
		if err == nil && exists && strings.EqualFold(state, desired) {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("wait ccvm %s timed out: %w", desired, err)
			}
			if !exists {
				return fmt.Errorf("wait ccvm %s timed out: domain not found", desired)
			}
			return fmt.Errorf("wait ccvm %s timed out: current state=%s", desired, state)
		}
		time.Sleep(time.Second)
	}
}

func waitForCCVMDomainGone(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		_, exists, err := readLocalCCVMState()
		if !exists || isCCVMDomainNotFoundError(err) {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("wait ccvm delete timed out: %w", err)
			}
			return fmt.Errorf("wait ccvm delete timed out")
		}
		time.Sleep(time.Second)
	}
}

func isCCVMDomainNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return isCCVMDomainNotFoundOutput(err.Error())
}

func isCCVMDomainNotFoundOutput(output string) bool {
	output = strings.ToLower(strings.TrimSpace(output))
	return strings.Contains(output, "domain not found") ||
		strings.Contains(output, "failed to get domain") ||
		strings.Contains(output, "no domain")
}

func runCCVMLifecycleCommand(timeout time.Duration, command string, args ...string) (string, error) {
	env := resetCloudCenterCommandEnv()
	if command == "virsh" {
		env = virshEnv()
	}
	out, timedOut, err := runCommandOutputWithEnv(command, timeout, env, args...)
	if timedOut {
		return out, fmt.Errorf("%s %s timed out after %s", command, strings.Join(args, " "), timeout)
	}
	if err != nil {
		return out, fmt.Errorf("%s %s failed: %s", command, strings.Join(args, " "), firstNonEmpty(out, err.Error()))
	}
	return out, nil
}

func resetCloudCenterHCI() error {
	if err := resetCloudCenterRemovePCSResource(); err != nil {
		return err
	}
	if err := resetCloudCenterDestroyPCSCluster(); err != nil {
		return err
	}
	if err := resetCloudCenterRemoveRBDCCVM(); err != nil {
		return err
	}
	resetCloudCenterResetVirshDomain()
	return resetCloudCenterPrepareWorkspace(true)
}

func resetCloudCenterVM(req CCVMLifecycleRequest, cfg *CubeModel.ClusterConfigSection) error {
	if err := resetCloudCenterInitGFSCluster(req, cfg); err != nil {
		return err
	}
	resetCloudCenterResetVirshDomain()
	return resetCloudCenterPrepareWorkspace(true)
}

func resetCloudCenterStandalone() error {
	resetCloudCenterResetVirshDomain()
	if err := resetCloudCenterPrepareWorkspace(true); err != nil {
		return err
	}
	resp := runLocalManage(LocalManageRequest{Action: "reset"})
	if resp.Code != http.StatusOK {
		return fmt.Errorf("%s", firstNonEmpty(resp.Message, fmt.Sprint(resp.Val)))
	}
	return nil
}

func resetCloudCenterHCIFilesystem(req CCVMLifecycleRequest, cfg *CubeModel.ClusterConfigSection) error {
	if err := resetCloudCenterInitGFSCluster(req, cfg); err != nil {
		return err
	}
	if err := resetCloudCenterRemoveRBDCCVM(); err != nil {
		return err
	}
	return resetCloudCenterPrepareWorkspace(false)
}

func resetCloudCenterRemovePCSResource() error {
	resp := removeCCVMPCSResource(CCVMPCSControlRequest{
		Action:   "remove",
		Resource: pcsDefaultResourceID,
	}, "local")
	if resp.Code != http.StatusOK && resp.Code != http.StatusBadRequest {
		return fmt.Errorf("remove %s failed: %s", pcsDefaultResourceID, firstNonEmpty(resp.Message, fmt.Sprint(resp.Val)))
	}
	return nil
}

func resetCloudCenterDestroyPCSCluster() error {
	resp := destroyCCVMPCSCluster(CCVMPCSControlRequest{Action: "destroy"}, "local")
	if resp.Code != http.StatusOK && resp.Code != http.StatusBadRequest {
		return fmt.Errorf("pcs cluster destroy failed: %s", firstNonEmpty(resp.Message, fmt.Sprint(resp.Val)))
	}
	return nil
}

func resetCloudCenterRemoveRBDCCVM() error {
	exists, err := ccvmPCSSetupRBDImageExists()
	if err != nil || !exists {
		return err
	}
	_, err = runPCSCommand(ccvmPCSSetupCommandTimeout, "rbd", "rm", "--no-progress", ccvmPCSSetupRBDImageSpec)
	return err
}

func resetCloudCenterResetVirshDomain() {
	_, _, _ = runCommandOutputWithEnv("virsh", resetCloudCenterShortTimeout, virshEnv(), "destroy", ccvmSnapName)
	_, _, _ = runCommandOutputWithEnv("virsh", resetCloudCenterShortTimeout, virshEnv(), "undefine", ccvmSnapName, "--keep-nvram")
}

func resetCloudCenterPrepareWorkspace(clear bool) error {
	ccvmConfigDir := resolveAbleStackVMConfigDir("ccvm")
	if err := os.MkdirAll(ccvmConfigDir, 0755); err != nil {
		return err
	}
	if err := os.Remove(resetCloudCenterCloudInitISO); err != nil && !os.IsNotExist(err) {
		return err
	}
	if clear {
		return removeDirContents(ccvmConfigDir)
	}
	return nil
}

func resetCloudCenterInitGFSCluster(req CCVMLifecycleRequest, cfg *CubeModel.ClusterConfigSection) error {
	if len(resetCloudCenterHostTargets(cfg)) == 0 {
		return fmt.Errorf("hosts[].ablecube required")
	}

	gfsReq := GFSManageRequest{Action: "init-pcs-cluster"}
	vgNames := resetCloudCenterGlueVGNames()
	if len(vgNames) > 0 {
		diskArg := strings.TrimSpace(req.Disk)
		if diskArg == "" {
			diskArg = strings.Join(resetCloudCenterGlueBaseDisks(), ",")
		}
		if diskArg == "" {
			return fmt.Errorf("gfs disk not found")
		}
		lvNames := resetCloudCenterLVNames(vgNames)
		gfsReq.Disks = splitCommaValues(diskArg)
		gfsReq.VolumeGroups = make([]GFSManageVolumeGroup, 0, len(vgNames))
		for i, vgName := range vgNames {
			lvName := vgName
			if i < len(lvNames) {
				lvName = lvNames[i]
			}
			gfsReq.VolumeGroups = append(gfsReq.VolumeGroups, GFSManageVolumeGroup{
				VGName: vgName,
				LVName: lvName,
			})
		}
	}

	if err := normalizeGFSManageRequest(&gfsReq); err != nil {
		return err
	}
	resp := runGFSManage(gfsReq, cfg)
	if resp.Code != http.StatusOK {
		return fmt.Errorf("%s", firstNonEmpty(resp.Message, fmt.Sprint(resp.Val)))
	}
	return nil
}

func resetCloudCenterGlueVGNames() []string {
	out, _, _ := runCommandOutputWithEnv("pvs", resetCloudCenterCommandTimeout, resetCloudCenterCommandEnv(), "--noheadings", "-o", "vg_name")
	seen := map[string]struct{}{}
	vgs := make([]string, 0)
	for _, value := range strings.Fields(out) {
		value = strings.TrimSpace(value)
		if !strings.Contains(value, "vg_glue") {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		vgs = append(vgs, value)
	}
	return vgs
}

func resetCloudCenterGlueBaseDisks() []string {
	out, _, _ := runCommandOutputWithEnv("pvs", resetCloudCenterCommandTimeout, resetCloudCenterCommandEnv(), "--noheadings", "-o", "pv_name,vg_name")
	seen := map[string]struct{}{}
	disks := make([]string, 0)
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.Contains(fields[1], "vg_glue") {
			continue
		}
		disk := resetCloudCenterBaseDisk(fields[0])
		if disk == "" {
			continue
		}
		if _, ok := seen[disk]; ok {
			continue
		}
		seen[disk] = struct{}{}
		disks = append(disks, disk)
	}
	return disks
}

func resetCloudCenterBaseDisk(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if resetCloudCenterPSuffixPartition.MatchString(path) {
		return resetCloudCenterPSuffixPartition.ReplaceAllString(path, "")
	}
	if resetCloudCenterAlphaSuffixPartition.MatchString(path) {
		return resetCloudCenterNumberSuffix.ReplaceAllString(path, "")
	}
	return path
}

func resetCloudCenterLVNames(vgNames []string) []string {
	out := make([]string, 0, len(vgNames))
	for _, vgName := range vgNames {
		if strings.HasPrefix(vgName, "vg") {
			out = append(out, strings.Replace(vgName, "vg", "lv", 1))
			continue
		}
		out = append(out, vgName)
	}
	return out
}

type resetCloudCenterSystemFlag struct {
	Depth1 string
	Depth2 string
	Value  string
}

func resetCloudCenterGFSSystemFlags() []resetCloudCenterSystemFlag {
	return []resetCloudCenterSystemFlag{
		{Depth1: "bootstrap", Depth2: "ccvm", Value: "false"},
		{Depth1: "monitoring", Depth2: "wall", Value: "false"},
		{Depth1: "bootstrap", Depth2: "gfs_configure", Value: "false"},
	}
}

func resetCloudCenterApplyGFSSystemFlags(cfg *CubeModel.ClusterConfigSection) ([]CubeModel.ClusterApplyResult, error) {
	return resetCloudCenterApplySystemFlagsOnHosts(cfg, resetCloudCenterGFSSystemFlags())
}

func resetCloudCenterApplySystemFlagsOnHosts(cfg *CubeModel.ClusterConfigSection, flags []resetCloudCenterSystemFlag) ([]CubeModel.ClusterApplyResult, error) {
	targets := resetCloudCenterHostTargets(cfg)
	if len(targets) == 0 {
		return resetCloudCenterApplyLocalSystemFlags(flags)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	results := make([]CubeModel.ClusterApplyResult, 0, len(targets))
	for _, target := range targets {
		res := CubeModel.ClusterApplyResult{Target: target}
		var err error
		if isLocalTarget(target) || strings.EqualFold(target, "local") {
			_, err = resetCloudCenterApplyLocalSystemFlags(flags)
		} else {
			err = resetCloudCenterApplyRemoteSystemFlags(client, target, flags)
		}
		if err != nil {
			res.Code = http.StatusInternalServerError
			res.Message = err.Error()
		} else {
			res.Code = http.StatusOK
			res.Message = "ok"
		}
		results = append(results, res)
	}
	return results, resetCloudCenterFirstResultError(results)
}

func resetCloudCenterApplyLocalSystemFlags(flags []resetCloudCenterSystemFlag) ([]CubeModel.ClusterApplyResult, error) {
	root, err := loadClusterJSONRoot()
	if err != nil {
		return []CubeModel.ClusterApplyResult{{Target: "local", Code: http.StatusInternalServerError, Message: err.Error()}}, err
	}
	profile, err := ensureSystemProfileMap(root)
	if err != nil {
		return []CubeModel.ClusterApplyResult{{Target: "local", Code: http.StatusInternalServerError, Message: err.Error()}}, err
	}
	for _, flag := range flags {
		if err := updateSystemProfileValue(profile, SystemConfigRequest{
			Action: "update",
			Depth1: flag.Depth1,
			Depth2: flag.Depth2,
			Value:  flag.Value,
		}); err != nil {
			return []CubeModel.ClusterApplyResult{{Target: "local", Code: http.StatusInternalServerError, Message: err.Error()}}, err
		}
	}
	if err := saveClusterJSONRoot(root); err != nil {
		return []CubeModel.ClusterApplyResult{{Target: "local", Code: http.StatusInternalServerError, Message: err.Error()}}, err
	}
	return []CubeModel.ClusterApplyResult{{Target: "local", Code: http.StatusOK, Message: "ok"}}, nil
}

func resetCloudCenterApplyRemoteSystemFlags(client *http.Client, target string, flags []resetCloudCenterSystemFlag) error {
	for _, flag := range flags {
		if err := callSystemConfigRemote(client, target, SystemConfigRequest{
			Action: "update",
			Depth1: flag.Depth1,
			Depth2: flag.Depth2,
			Value:  flag.Value,
		}); err != nil {
			return err
		}
	}
	return nil
}

func resetCloudCenterHostTargets(cfg *CubeModel.ClusterConfigSection) []string {
	if cfg == nil {
		return nil
	}
	seen := map[string]struct{}{}
	targets := make([]string, 0, len(cfg.Hosts))
	for _, host := range cfg.Hosts {
		target := strings.TrimSpace(host.Ablecube)
		if target == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		targets = append(targets, target)
	}
	return targets
}

func resetCloudCenterFirstResultError(results []CubeModel.ClusterApplyResult) error {
	for _, result := range results {
		if result.Code != http.StatusOK {
			return fmt.Errorf("system config update failed: %s: %s", result.Target, result.Message)
		}
	}
	return nil
}

func resetCloudCenterCommandEnv() []string {
	return append([]string{"LANG=en_US.utf-8", "LANGUAGE=en"}, os.Environ()...)
}

func resetCloudCenterOK(osType string, val string, results []CubeModel.ClusterApplyResult) CCVMLifecycleResponse {
	return CCVMLifecycleResponse{
		Code:    http.StatusOK,
		Val:     val,
		RetName: ccvmLifecycleRetName,
		Message: "ok",
		OSType:  osType,
		Results: results,
	}
}

func resetCloudCenterError(osType string, val string, message string, results []CubeModel.ClusterApplyResult) CCVMLifecycleResponse {
	return CCVMLifecycleResponse{
		Code:    http.StatusInternalServerError,
		Val:     val,
		RetName: ccvmLifecycleRetName,
		Message: message,
		OSType:  osType,
		Results: results,
	}
}

func statusCodeFromCCVMLifecycleResponse(resp CCVMLifecycleResponse) int {
	if resp.Code == http.StatusOK {
		return http.StatusOK
	}
	if resp.Code == http.StatusBadRequest {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
