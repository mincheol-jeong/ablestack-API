package cube

import (
	"bytes"
	stdcontext "context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	libvirtinfra "ablecloud.io/ablestack-api/internal/infra/libvirt"
	"ablecloud.io/ablestack-api/internal/infra/utils"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"github.com/gin-gonic/gin"
)

type BootstrapRequest = CubeModel.BootstrapRequest
type BootstrapResponse = CubeModel.BootstrapResponse
type BootstrapScriptResult = CubeModel.BootstrapScriptResult
type BootstrapHealthResult = CubeModel.BootstrapHealthResult

const (
	bootstrapLocalHeader         = "X-Cube-Bootstrap-Local"
	bootstrapDirectHeader        = "X-Cube-Bootstrap-Direct"
	bootstrapNodePrepareHeader   = "X-Cube-Bootstrap-Node-Prepare"
	bootstrapScriptPath          = "/root/bootstrap.sh"
	bootstrapGuestLogPath        = "/var/log/ablestack-bootstrap.log"
	bootstrapTPMPropertiesPath   = "/etc/cloudstack/agent/tpm.properties"
	bootstrapSetCrushmapPath     = "/usr/local/sbin/setCrushmap.sh"
	bootstrapRemoteRequestTO     = 125 * time.Minute
	bootstrapScriptExecTO        = 120 * time.Minute
	bootstrapNodePrepareTO       = 5 * time.Minute
	bootstrapGuestCommandTO      = 10 * time.Second
	bootstrapGuestPollInterval   = 2 * time.Second
	bootstrapGuestReadyInterval  = 5 * time.Second
	bootstrapCephPublicKeyMarker = "CEPHADM_PUBLIC_KEY_BASE64="
)

type bootstrapScriptTarget struct {
	Role     string
	Action   string
	Hostname string
	Target   string
	Domain   string
	Args     []string
}

type bootstrapGuestExecResponse struct {
	Return struct {
		PID int `json:"pid"`
	} `json:"return"`
}

type bootstrapGuestExecStatusResponse struct {
	Return struct {
		Exited   bool   `json:"exited"`
		ExitCode int    `json:"exitcode"`
		OutData  string `json:"out-data"`
		ErrData  string `json:"err-data"`
	} `json:"return"`
}

// SCVMBootstrap godoc
//
//	@Summary		SCVM Bootstrap
//	@Description	각 host API가 로컬 SCVM을 qemu-guest-agent로 준비한 뒤 첫 번째 SCVM에서 Ceph bootstrap/host 등록/검증을 수행하고, SCVM API health와 라이선스를 확인합니다. deploy_run의 scvm_bootstrap step도 같은 실행 흐름을 사용합니다.
//	@Tags			Cube-SCVM
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CubeModel.BootstrapRequest	false	"scvm bootstrap request"
//	@Success		200	{object}	CubeModel.BootstrapResponse
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/scvm/bootstrap [post]
func SCVMBootstrap(context *gin.Context) {
	runBootstrapHandler(context, licenseApplyRoleSCVM)
}

// CCVMBootstrap godoc
//
//	@Summary		CCVM Bootstrap
//	@Description	내부 API로 Ablecube TPM/PCS 서비스와 HCI SCVM Crushmap을 로컬 준비하고, 라이선스가 등록된 CCVM API에서 /root/bootstrap.sh를 실행한 뒤 CCVM API health와 라이선스 status를 확인합니다. deploy_run의 ccvm_bootstrap step도 같은 실행 흐름을 사용합니다.
//	@Tags			Cube-CCVM
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CubeModel.BootstrapRequest	false	"ccvm bootstrap request"
//	@Success		200	{object}	CubeModel.BootstrapResponse
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/ccvm/bootstrap [post]
func CCVMBootstrap(context *gin.Context) {
	runBootstrapHandler(context, licenseApplyRoleCCVM)
}

func runBootstrapHandler(context *gin.Context, role string) {
	var req BootstrapRequest
	if context.Request != nil && context.Request.Body != nil && context.Request.ContentLength != 0 {
		if err := context.ShouldBindJSON(&req); err != nil {
			context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
				ErrCode: http.StatusBadRequest,
				Message: "invalid request",
			})
			return
		}
	}

	if isBootstrapLocalRequest(context) {
		resp := runBootstrapScriptLocalRequest(req, role)
		context.JSON(statusCodeFromBootstrapResponse(resp), resp)
		return
	}
	if role == licenseApplyRoleCCVM && isBootstrapNodePrepareRequest(context) {
		resp := runCCVMBootstrapNodePrepareRequest(req)
		context.JSON(statusCodeFromBootstrapResponse(resp), resp)
		return
	}
	if role == licenseApplyRoleCCVM && isBootstrapDirectRequest(context) {
		resp := runCCVMBootstrapScriptDirectRequest(req)
		context.JSON(statusCodeFromBootstrapResponse(resp), resp)
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
	if role == licenseApplyRoleSCVM && !isHCITarget(cfg.Type) {
		context.JSON(http.StatusOK, BootstrapResponse{
			Code:    http.StatusOK,
			Role:    role,
			Message: "scvm_bootstrap is not required for " + strings.TrimSpace(cfg.Type),
		})
		return
	}

	resp := runBootstrapRole(req, cfg, role, context.GetHeader("Authorization"))
	if resp.Code == http.StatusOK && role == licenseApplyRoleCCVM {
		results, err := resetCloudCenterApplySystemFlagsOnHosts(cfg, []resetCloudCenterSystemFlag{
			{Depth1: "bootstrap", Depth2: "ccvm", Value: "true"},
		})
		resp.SystemProfile = results
		if err != nil {
			resp.Code = http.StatusInternalServerError
			resp.Message = "ccvm bootstrap system profile update failed: " + err.Error()
		}
	}
	context.JSON(statusCodeFromBootstrapResponse(resp), resp)
}

func bootstrapRequestFromDeployRun(req DeployRunRequest) BootstrapRequest {
	return BootstrapRequest{
		LicenseContent:      req.LicenseContent,
		Licenses:            req.Licenses,
		LicenseFilename:     req.LicenseFilename,
		RunScript:           req.RunBootstrapScript,
		TargetHostnames:     req.TargetHostnames,
		JoinExistingCluster: req.JoinExistingSCVMCluster,
	}
}

func runBootstrapRole(req BootstrapRequest, cfg *CubeModel.ClusterConfigSection, role string, authHeader string) BootstrapResponse {
	role = normalizeBootstrapRole(role)
	if role == "" {
		return BootstrapResponse{Code: http.StatusBadRequest, Message: "unsupported bootstrap role"}
	}

	targets, err := buildLicenseApplyTargets(LicenseApplyRequest{
		Roles:           []string{role},
		TargetHostnames: req.TargetHostnames,
	}, cfg)
	if err != nil {
		return BootstrapResponse{Code: http.StatusBadRequest, Role: role, Message: err.Error()}
	}
	if len(targets) == 0 {
		return BootstrapResponse{Code: http.StatusBadRequest, Role: role, Message: role + " target not found"}
	}

	scriptResults, err := runBootstrapScripts(req, cfg, role)
	if err != nil {
		return BootstrapResponse{
			Code:    http.StatusInternalServerError,
			Role:    role,
			Script:  scriptResults,
			Message: err.Error(),
		}
	}

	readiness := waitBootstrapTargetsReady(targets, role)
	if failed := firstFailedBootstrapHealth(readiness); failed != nil {
		return BootstrapResponse{
			Code:    http.StatusInternalServerError,
			Role:    role,
			Script:  scriptResults,
			Health:  readiness,
			Message: failed.Message,
		}
	}

	var applyResp *LicenseApplyResponse
	if role != licenseApplyRoleCCVM {
		result := runBootstrapLicenseApply(req, role, cfg, authHeader)
		applyResp = &result
		if result.Code != http.StatusOK {
			return BootstrapResponse{
				Code:         statusCodeFromLicenseApplyResponse(result),
				Role:         role,
				Script:       scriptResults,
				Health:       readiness,
				LicenseApply: applyResp,
				Message:      firstNonEmpty(result.Message, role+"_bootstrap license apply failed"),
			}
		}
	}

	statusResp := runLicenseApply(LicenseApplyRequest{
		Action:          "status",
		Roles:           []string{role},
		TargetHostnames: req.TargetHostnames,
	}, cfg, authHeader)
	if statusResp.Code != http.StatusOK {
		return BootstrapResponse{
			Code:          statusCodeFromLicenseApplyResponse(statusResp),
			Role:          role,
			Script:        scriptResults,
			Health:        readiness,
			LicenseApply:  applyResp,
			LicenseStatus: &statusResp,
			Message:       firstNonEmpty(statusResp.Message, role+"_bootstrap license status failed"),
		}
	}

	return BootstrapResponse{
		Code:          http.StatusOK,
		Role:          role,
		Script:        scriptResults,
		Health:        readiness,
		LicenseApply:  applyResp,
		LicenseStatus: &statusResp,
		Message:       role + "_bootstrap success",
	}
}

func waitBootstrapTargetsReady(targets []licenseApplyTarget, role string) []BootstrapHealthResult {
	results := make([]BootstrapHealthResult, 0, len(targets))
	for _, target := range targets {
		health, _ := waitBootstrapAPIHealth(target.Target, role)
		results = append(results, bootstrapHealthResult(target, health))
	}
	return results
}

func firstFailedBootstrapHealth(results []BootstrapHealthResult) *BootstrapHealthResult {
	for i := range results {
		if results[i].Code != http.StatusOK {
			return &results[i]
		}
	}
	return nil
}

func runBootstrapLicenseApply(req BootstrapRequest, role string, cfg *CubeModel.ClusterConfigSection, authHeader string) LicenseApplyResponse {
	return runLicenseApply(LicenseApplyRequest{
		Action:          "register",
		LicenseContent:  req.LicenseContent,
		Licenses:        req.Licenses,
		Filename:        req.LicenseFilename,
		Roles:           []string{role},
		TargetHostnames: req.TargetHostnames,
	}, cfg, authHeader)
}

func waitBootstrapAPIHealth(target string, role string) (map[string]any, error) {
	return waitDeployRunAPIHealth(target)
}

func normalizeBootstrapRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "scvm", "storage", "storage-vm", "storage_vm":
		return licenseApplyRoleSCVM
	case "ccvm", "cloud", "cloud-vm", "cloud_vm":
		return licenseApplyRoleCCVM
	default:
		return ""
	}
}

func isBootstrapLocalRequest(context *gin.Context) bool {
	return strings.EqualFold(strings.TrimSpace(context.GetHeader(bootstrapLocalHeader)), "1")
}

func isBootstrapDirectRequest(context *gin.Context) bool {
	return strings.EqualFold(strings.TrimSpace(context.GetHeader(bootstrapDirectHeader)), "1")
}

func isBootstrapNodePrepareRequest(context *gin.Context) bool {
	return strings.EqualFold(strings.TrimSpace(context.GetHeader(bootstrapNodePrepareHeader)), "1")
}

func runCCVMBootstrapNodePrepareRequest(req BootstrapRequest) BootstrapResponse {
	target := bootstrapScriptTarget{
		Role:     licenseApplyRoleCCVM,
		Action:   strings.TrimSpace(req.ScriptAction),
		Hostname: firstNonEmpty(req.ScriptHostname, "local"),
		Target:   "local",
		Args:     normalizeStringSlice(req.ScriptArgs),
	}
	result := runCCVMBootstrapNodePrepareLocal(target)
	resp := BootstrapResponse{
		Code:   result.Code,
		Role:   licenseApplyRoleCCVM,
		Script: []BootstrapScriptResult{result},
	}
	if result.Code == http.StatusOK {
		resp.Message = "ccvm bootstrap node prepare success"
	} else {
		resp.Message = firstNonEmpty(result.Message, "ccvm bootstrap node prepare failed")
	}
	return resp
}

func runCCVMBootstrapScriptDirectRequest(req BootstrapRequest) BootstrapResponse {
	target := bootstrapScriptTarget{
		Role:     licenseApplyRoleCCVM,
		Action:   "bootstrap",
		Hostname: firstNonEmpty(req.ScriptHostname, licenseApplyRoleCCVM),
		Target:   "local",
		Args:     normalizeStringSlice(req.ScriptArgs),
	}
	result := runBootstrapScriptDirect(target)
	resp := BootstrapResponse{
		Code:   result.Code,
		Role:   licenseApplyRoleCCVM,
		Script: []BootstrapScriptResult{result},
	}
	if result.Code == http.StatusOK {
		resp.Message = "bootstrap script success"
	} else {
		resp.Message = firstNonEmpty(result.Message, "bootstrap script failed")
	}
	return resp
}

func runBootstrapScriptLocalRequest(req BootstrapRequest, role string) BootstrapResponse {
	role = normalizeBootstrapRole(role)
	if role == "" {
		return BootstrapResponse{Code: http.StatusBadRequest, Message: "unsupported bootstrap role"}
	}
	target := bootstrapScriptTarget{
		Role:     role,
		Action:   strings.TrimSpace(req.ScriptAction),
		Hostname: firstNonEmpty(req.ScriptHostname, role),
		Target:   firstNonEmpty(req.ScriptTarget, "local"),
		Domain:   firstNonEmpty(req.ScriptDomain, defaultBootstrapScriptDomain(role)),
		Args:     normalizeStringSlice(req.ScriptArgs),
	}
	result := runBootstrapScriptLocal(target)
	resp := BootstrapResponse{
		Code:   result.Code,
		Role:   role,
		Script: []BootstrapScriptResult{result},
	}
	if result.Code == http.StatusOK {
		resp.Message = "bootstrap script success"
	} else {
		resp.Message = firstNonEmpty(result.Message, "bootstrap script failed")
	}
	return resp
}

func runBootstrapScripts(req BootstrapRequest, cfg *CubeModel.ClusterConfigSection, role string) ([]BootstrapScriptResult, error) {
	if !shouldRunBootstrapScript(req) {
		return nil, nil
	}
	switch role {
	case licenseApplyRoleSCVM:
		return runSCVMClusterBootstrap(req, cfg)
	case licenseApplyRoleCCVM:
		return runCCVMClusterBootstrap(req, cfg)
	default:
		return nil, fmt.Errorf("unsupported bootstrap role: %s", role)
	}
}

func runCCVMClusterBootstrap(req BootstrapRequest, cfg *CubeModel.ClusterConfigSection) ([]BootstrapScriptResult, error) {
	prepareTargets, err := buildCCVMBootstrapNodePrepareTargets(cfg)
	if err != nil {
		return nil, err
	}
	results := make([]BootstrapScriptResult, 0, len(prepareTargets)+1)
	for _, target := range prepareTargets {
		result := callBootstrapScriptAPI(target, bootstrapNodePrepareHeader)
		results = append(results, result)
		if result.Code != http.StatusOK {
			return results, bootstrapTargetError(licenseApplyRoleCCVM, target, result)
		}
	}

	target, err := buildCCVMBootstrapScriptTarget(req, cfg)
	if err != nil {
		return results, err
	}
	result := callCCVMBootstrapScriptDirect(target)
	results = append(results, result)
	if result.Code != http.StatusOK {
		return results, bootstrapTargetError(licenseApplyRoleCCVM, target, result)
	}
	return results, nil
}

func buildCCVMBootstrapNodePrepareTargets(cfg *CubeModel.ClusterConfigSection) ([]bootstrapScriptTarget, error) {
	if cfg == nil {
		return nil, fmt.Errorf("clusterConfig required")
	}
	hosts := append([]CubeModel.ClusterHost(nil), cfg.Hosts...)
	sort.SliceStable(hosts, func(i, j int) bool {
		left, leftErr := strconv.Atoi(strings.TrimSpace(hosts[i].Index))
		right, rightErr := strconv.Atoi(strings.TrimSpace(hosts[j].Index))
		if leftErr == nil && rightErr == nil && left != right {
			return left < right
		}
		return strings.TrimSpace(hosts[i].Index) < strings.TrimSpace(hosts[j].Index)
	})

	targets := make([]bootstrapScriptTarget, 0, len(hosts)+1)
	if strings.EqualFold(strings.TrimSpace(cfg.Type), "ablestack-hci") {
		for i := range hosts {
			host := &hosts[i]
			target := firstNonEmpty(host.ScvmMngt, host.Scvm)
			if target == "" {
				continue
			}
			targets = append(targets, bootstrapScriptTarget{
				Role:     licenseApplyRoleCCVM,
				Action:   "configure_crushmap",
				Hostname: licenseApplySCVMHostname(host),
				Target:   target,
			})
			break
		}
		if len(targets) == 0 {
			return nil, fmt.Errorf("ablestack-hci requires hosts[].scvmMngt for CCVM bootstrap")
		}
	}

	enableClusterServices := !strings.EqualFold(strings.TrimSpace(cfg.Type), "ablestack-standalone")
	for i := range hosts {
		host := &hosts[i]
		target := strings.TrimSpace(host.Ablecube)
		if target == "" {
			return nil, fmt.Errorf("%s hosts[].ablecube required", firstNonEmpty(host.Hostname, host.Index))
		}
		args := []string(nil)
		if enableClusterServices {
			args = []string{"enable-cluster-services"}
		}
		targets = append(targets, bootstrapScriptTarget{
			Role:     licenseApplyRoleCCVM,
			Action:   "prepare_host",
			Hostname: firstNonEmpty(host.Hostname, "ablecube"+strings.TrimSpace(host.Index)),
			Target:   target,
			Args:     args,
		})
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("clusterConfig.hosts required")
	}
	return targets, nil
}

func runCCVMBootstrapNodePrepareLocal(target bootstrapScriptTarget) BootstrapScriptResult {
	ctx, cancel := stdcontext.WithTimeout(stdcontext.Background(), bootstrapNodePrepareTO)
	defer cancel()

	var output string
	var err error
	switch target.Action {
	case "configure_crushmap":
		output, err = runBootstrapNodeCommand(ctx, bootstrapSetCrushmapPath)
	case "prepare_host":
		err = writeBootstrapTPMProperties()
		if err == nil && stringSliceContains(target.Args, "enable-cluster-services") {
			output, err = runBootstrapNodeCommand(ctx, "/usr/bin/systemctl", "enable", "--now", "pacemaker", "corosync")
			if err == nil {
				output = firstNonEmpty(output, "pacemaker and corosync enabled")
			}
		}
	default:
		err = fmt.Errorf("unsupported CCVM bootstrap node action: %s", firstNonEmpty(target.Action, "empty"))
	}
	if stdcontext.Cause(ctx) != nil {
		err = fmt.Errorf("CCVM bootstrap node prepare timed out after %s", bootstrapNodePrepareTO)
	}
	if err != nil {
		result := bootstrapScriptError(target, err.Error())
		result.Output = output
		return result
	}
	if target.Action == "prepare_host" {
		output = "tpm.properties configured; " + firstNonEmpty(output, "cluster services not required")
	}
	return BootstrapScriptResult{
		Role:     target.Role,
		Action:   target.Action,
		Hostname: target.Hostname,
		Target:   "local",
		Code:     http.StatusOK,
		Message:  "ok",
		Output:   output,
	}
}

func runBootstrapNodeCommand(ctx stdcontext.Context, name string, args ...string) (string, error) {
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	message := strings.TrimSpace(string(output))
	if err != nil {
		return message, fmt.Errorf("%s failed: %s", filepath.Base(name), firstNonEmpty(message, err.Error()))
	}
	return message, nil
}

func writeBootstrapTPMProperties() error {
	directory := filepath.Dir(bootstrapTPMPropertiesPath)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create CloudStack agent directory: %w", err)
	}
	temp, err := os.CreateTemp(directory, ".tpm.properties-*")
	if err != nil {
		return fmt.Errorf("create temporary tpm.properties: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return fmt.Errorf("chmod temporary tpm.properties: %w", err)
	}
	if _, err := temp.WriteString("host.tpm.enable=true\n"); err != nil {
		temp.Close()
		return fmt.Errorf("write temporary tpm.properties: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync temporary tpm.properties: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary tpm.properties: %w", err)
	}
	if err := os.Rename(tempName, bootstrapTPMPropertiesPath); err != nil {
		return fmt.Errorf("replace tpm.properties: %w", err)
	}
	return nil
}

func stringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), expected) {
			return true
		}
	}
	return false
}

func runSCVMClusterBootstrap(req BootstrapRequest, cfg *CubeModel.ClusterConfigSection) ([]BootstrapScriptResult, error) {
	if req.JoinExistingCluster {
		return runSCVMClusterJoin(req, cfg)
	}
	targets, err := buildSCVMBootstrapScriptTargets(req, cfg)
	if err != nil {
		return nil, err
	}
	prepareTargets := targets[:len(targets)-1]
	master := targets[len(targets)-1]
	results := make([]BootstrapScriptResult, 0, len(prepareTargets)*2+3)
	run := func(target bootstrapScriptTarget) error {
		result := executeBootstrapScriptTarget(target)
		results = append(results, result)
		if result.Code != http.StatusOK {
			return bootstrapTargetError(licenseApplyRoleSCVM, target, result)
		}
		return nil
	}

	for _, target := range prepareTargets {
		if err := run(target); err != nil {
			return results, err
		}
	}
	if err := run(master); err != nil {
		return results, err
	}

	exportKey := master
	exportKey.Action = "export_key"
	exportKey.Args = []string{"export-key"}
	if err := run(exportKey); err != nil {
		return results, err
	}
	publicKey, err := parseCephadmPublicKey(results[len(results)-1].Output)
	if err != nil {
		results[len(results)-1].Code = http.StatusInternalServerError
		results[len(results)-1].Message = err.Error()
		return results, err
	}
	results[len(results)-1].Output = "Cephadm public key exported"
	encodedKey := base64.StdEncoding.EncodeToString([]byte(publicKey))
	for _, target := range prepareTargets {
		installKey := target
		installKey.Action = "install_key"
		installKey.Args = []string{"install-key", encodedKey}
		if err := run(installKey); err != nil {
			return results, err
		}
	}

	finalize := master
	finalize.Action = "finalize"
	finalize.Args = []string{"finalize"}
	if err := run(finalize); err != nil {
		return results, err
	}
	return results, nil
}

// runSCVMClusterJoin prepares only the requested SCVMs and lets the existing
// first SCVM update Ceph orchestration. It never runs cephadm bootstrap.
func runSCVMClusterJoin(req BootstrapRequest, cfg *CubeModel.ClusterConfigSection) ([]BootstrapScriptResult, error) {
	if cfg == nil || len(cfg.Hosts) < 2 {
		return nil, fmt.Errorf("existing Ceph cluster and new SCVM host are required")
	}
	filter := licenseApplyHostnameFilter(req.TargetHostnames)
	if len(filter) == 0 {
		return nil, fmt.Errorf("target_hostnames required for existing Ceph cluster join")
	}

	allTargets, err := buildSCVMBootstrapScriptTargets(BootstrapRequest{}, cfg)
	if err != nil {
		return nil, err
	}
	master := allTargets[0]
	master.Action = "export_key"
	master.Args = []string{"export-key"}

	joinTargets, err := buildSCVMBootstrapScriptTargets(req, cfg)
	if err != nil {
		return nil, err
	}
	joinTargets = joinTargets[:len(joinTargets)-1]
	if len(joinTargets) == 0 {
		return nil, fmt.Errorf("new SCVM target not found")
	}
	for _, target := range joinTargets {
		if strings.EqualFold(target.Hostname, master.Hostname) {
			return nil, fmt.Errorf("existing Ceph master cannot be selected as a new SCVM")
		}
	}

	results := make([]BootstrapScriptResult, 0, len(joinTargets)*2+2)
	run := func(target bootstrapScriptTarget) error {
		result := executeBootstrapScriptTarget(target)
		results = append(results, result)
		if result.Code != http.StatusOK {
			return bootstrapTargetError(licenseApplyRoleSCVM, target, result)
		}
		return nil
	}
	for _, target := range joinTargets {
		if err := run(target); err != nil {
			return results, err
		}
	}
	if err := run(master); err != nil {
		return results, err
	}
	publicKey, err := parseCephadmPublicKey(results[len(results)-1].Output)
	if err != nil {
		return results, err
	}
	results[len(results)-1].Output = "Cephadm public key exported"
	encodedKey := base64.StdEncoding.EncodeToString([]byte(publicKey))
	for _, target := range joinTargets {
		installKey := target
		installKey.Action = "install_key"
		installKey.Args = []string{"install-key", encodedKey}
		if err := run(installKey); err != nil {
			return results, err
		}
	}
	finalize := master
	finalize.Action = "finalize"
	finalize.Args = []string{"finalize"}
	if err := run(finalize); err != nil {
		return results, err
	}
	return results, nil
}

func executeBootstrapScriptTarget(target bootstrapScriptTarget) BootstrapScriptResult {
	if isBootstrapScriptLocalTarget(target) {
		return runBootstrapScriptLocal(target)
	}
	return callBootstrapScriptRemote(target)
}

func bootstrapTargetError(role string, target bootstrapScriptTarget, result BootstrapScriptResult) error {
	return fmt.Errorf("%s %s failed on %s: %s", role, firstNonEmpty(target.Action, "bootstrap"), firstNonEmpty(target.Hostname, target.Target), firstNonEmpty(result.Message, "unknown error"))
}

func parseCephadmPublicKey(output string) (string, error) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, bootstrapCephPublicKeyMarker) {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(line, bootstrapCephPublicKeyMarker)))
		if err != nil {
			return "", fmt.Errorf("invalid Cephadm public key output: %w", err)
		}
		key := strings.TrimSpace(string(raw))
		if !strings.HasPrefix(key, "ssh-") {
			return "", fmt.Errorf("invalid Cephadm public key format")
		}
		return key, nil
	}
	return "", fmt.Errorf("Cephadm public key not found in master bootstrap output")
}

func shouldRunBootstrapScript(req BootstrapRequest) bool {
	if req.RunScript == nil {
		return true
	}
	return *req.RunScript
}

func buildSCVMBootstrapScriptTargets(req BootstrapRequest, cfg *CubeModel.ClusterConfigSection) ([]bootstrapScriptTarget, error) {
	if cfg == nil {
		return nil, fmt.Errorf("clusterConfig required")
	}
	filter := licenseApplyHostnameFilter(req.TargetHostnames)
	hosts := append([]CubeModel.ClusterHost(nil), cfg.Hosts...)
	sort.SliceStable(hosts, func(i, j int) bool {
		left, leftErr := strconv.Atoi(strings.TrimSpace(hosts[i].Index))
		right, rightErr := strconv.Atoi(strings.TrimSpace(hosts[j].Index))
		if leftErr == nil && rightErr == nil && left != right {
			return left < right
		}
		return strings.TrimSpace(hosts[i].Index) < strings.TrimSpace(hosts[j].Index)
	})

	targets := make([]bootstrapScriptTarget, 0, len(hosts)+1)
	for i := range hosts {
		host := &hosts[i]
		hostname := licenseApplySCVMHostname(host)
		if !licenseApplyTargetMatchesFilter(licenseApplyRoleSCVM, hostname, host, filter) {
			continue
		}
		target := strings.TrimSpace(host.Ablecube)
		if target == "" {
			return nil, fmt.Errorf("%s hosts[].ablecube required", firstNonEmpty(host.Hostname, hostname))
		}
		targets = append(targets, bootstrapScriptTarget{
			Role:     licenseApplyRoleSCVM,
			Action:   "prepare",
			Hostname: hostname,
			Target:   target,
			Domain:   scvmDomainName,
			Args:     []string{"prepare"},
		})
	}
	if len(targets) == 0 {
		if len(filter) > 0 {
			return nil, fmt.Errorf("target_hostname not found")
		}
		return nil, fmt.Errorf("scvm host not found")
	}
	master := targets[0]
	master.Action = "bootstrap"
	master.Args = []string{"bootstrap"}
	targets = append(targets, master)
	return targets, nil
}

func buildCCVMBootstrapScriptTarget(req BootstrapRequest, cfg *CubeModel.ClusterConfigSection) (bootstrapScriptTarget, error) {
	if cfg == nil {
		return bootstrapScriptTarget{}, fmt.Errorf("clusterConfig required")
	}
	filter := licenseApplyHostnameFilter(req.TargetHostnames)
	if len(filter) > 0 && !licenseApplyTargetMatchesFilter(licenseApplyRoleCCVM, licenseApplyRoleCCVM, nil, filter) {
		return bootstrapScriptTarget{}, fmt.Errorf("target_hostname not found")
	}
	target := strings.TrimSpace(cfg.CCVM.IP)
	if target == "" {
		return bootstrapScriptTarget{}, fmt.Errorf("clusterConfig.ccvm.ip required")
	}
	return bootstrapScriptTarget{
		Role:     licenseApplyRoleCCVM,
		Action:   "bootstrap",
		Hostname: licenseApplyRoleCCVM,
		Target:   target,
		Args:     ccvmBootstrapScriptArgs(),
	}, nil
}

func ccvmBootstrapScriptArgs() []string {
	if licenseType := currentLicenseTypeValue(); licenseType != "" {
		return []string{licenseType}
	}
	return nil
}

func defaultBootstrapScriptDomain(role string) string {
	switch role {
	case licenseApplyRoleSCVM:
		return scvmDomainName
	case licenseApplyRoleCCVM:
		return ccvmSnapName
	default:
		return ""
	}
}

func isBootstrapScriptLocalTarget(target bootstrapScriptTarget) bool {
	raw := strings.TrimSpace(target.Target)
	return raw == "" || strings.EqualFold(raw, "local") || isLocalTarget(raw)
}

func callBootstrapScriptRemote(target bootstrapScriptTarget) BootstrapScriptResult {
	return callBootstrapScriptAPI(target, bootstrapLocalHeader)
}

func callCCVMBootstrapScriptDirect(target bootstrapScriptTarget) BootstrapScriptResult {
	return callBootstrapScriptAPI(target, bootstrapDirectHeader)
}

func callBootstrapScriptAPI(target bootstrapScriptTarget, modeHeader string) BootstrapScriptResult {
	req := BootstrapRequest{
		ScriptDomain:   target.Domain,
		ScriptArgs:     target.Args,
		ScriptHostname: target.Hostname,
		ScriptTarget:   target.Target,
		ScriptAction:   target.Action,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return bootstrapScriptError(target, err.Error())
	}

	url := fmt.Sprintf("%s/api/v1/cube/%s/bootstrap", buildTargetURL(target.Target), target.Role)
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return bootstrapScriptError(target, err.Error())
	}
	attachInternalToken(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(modeHeader, "1")

	client := &http.Client{Timeout: bootstrapRemoteRequestTO}
	resp, err := client.Do(httpReq)
	if err != nil {
		return bootstrapScriptError(target, err.Error())
	}
	defer resp.Body.Close()

	var out BootstrapResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return bootstrapScriptError(target, err.Error())
	}
	if len(out.Script) == 0 {
		return bootstrapScriptError(target, firstNonEmpty(out.Message, "empty bootstrap script response"))
	}
	result := out.Script[0]
	if strings.TrimSpace(result.Role) == "" {
		result.Role = target.Role
	}
	if strings.TrimSpace(result.Action) == "" {
		result.Action = target.Action
	}
	if strings.TrimSpace(result.Hostname) == "" {
		result.Hostname = target.Hostname
	}
	if strings.TrimSpace(result.Target) == "" || strings.EqualFold(result.Target, "local") {
		result.Target = target.Target
	}
	if strings.TrimSpace(result.Domain) == "" {
		result.Domain = target.Domain
	}
	if result.Code == 0 {
		result.Code = out.Code
	}
	if result.Code == 0 {
		result.Code = resp.StatusCode
	}
	if result.Code != http.StatusOK && strings.TrimSpace(result.Message) == "" {
		result.Message = firstNonEmpty(out.Message, resp.Status)
	}
	return result
}

func runBootstrapScriptDirect(target bootstrapScriptTarget) BootstrapScriptResult {
	ctx, cancel := stdcontext.WithTimeout(stdcontext.Background(), bootstrapScriptExecTO)
	defer cancel()

	commandArgs := []string{
		"-lc",
		fmt.Sprintf("set -o pipefail; %s \"$@\" > %s 2>&1; rc=$?; tail -n 80 %s 2>/dev/null || true; exit $rc", bootstrapScriptPath, bootstrapGuestLogPath, bootstrapGuestLogPath),
		"bootstrap",
	}
	commandArgs = append(commandArgs, normalizeStringSlice(target.Args)...)
	output, err := exec.CommandContext(ctx, "/bin/bash", commandArgs...).CombinedOutput()
	if stdcontext.Cause(ctx) != nil {
		result := bootstrapScriptError(target, fmt.Sprintf("ccvm bootstrap script timed out after %s", bootstrapScriptExecTO))
		result.Output = strings.TrimSpace(string(output))
		return result
	}
	if err != nil {
		result := bootstrapScriptError(target, fmt.Sprintf("ccvm bootstrap script failed: %s", firstNonEmpty(strings.TrimSpace(string(output)), err.Error())))
		result.Output = strings.TrimSpace(string(output))
		return result
	}
	return BootstrapScriptResult{
		Role:     target.Role,
		Action:   firstNonEmpty(target.Action, "bootstrap"),
		Hostname: firstNonEmpty(target.Hostname, licenseApplyRoleCCVM),
		Target:   firstNonEmpty(target.Target, "local"),
		Code:     http.StatusOK,
		Message:  "ok",
		Output:   strings.TrimSpace(string(output)),
	}
}

func runBootstrapScriptLocal(target bootstrapScriptTarget) BootstrapScriptResult {
	target.Domain = firstNonEmpty(target.Domain, defaultBootstrapScriptDomain(target.Role))
	target.Args = normalizeStringSlice(target.Args)
	if strings.TrimSpace(target.Domain) == "" {
		return bootstrapScriptError(target, "bootstrap script domain required")
	}
	if err := waitBootstrapGuestAgent(target.Domain, deployRunBootstrapReadyTO); err != nil {
		return bootstrapScriptError(target, err.Error())
	}
	output, err := runBootstrapGuestScript(target.Domain, target.Args, bootstrapScriptExecTO)
	if err != nil {
		result := bootstrapScriptError(target, err.Error())
		result.Output = output
		return result
	}
	return BootstrapScriptResult{
		Role:     target.Role,
		Action:   target.Action,
		Hostname: target.Hostname,
		Target:   firstNonEmpty(target.Target, "local"),
		Domain:   target.Domain,
		Code:     http.StatusOK,
		Message:  "ok",
		Output:   output,
	}
}

func waitBootstrapGuestAgent(domain string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		_, timedOut, err := libvirtinfra.RunGuestAgentCommand(domain, libvirtinfra.GuestAgentCommandRequest{Execute: "guest-ping"}, bootstrapGuestCommandTO)
		if !timedOut && err == nil {
			return nil
		}
		if timedOut {
			lastErr = fmt.Errorf("%s guest-ping timed out", domain)
		} else {
			lastErr = err
		}
		if time.Now().Add(bootstrapGuestReadyInterval).After(deadline) {
			break
		}
		time.Sleep(bootstrapGuestReadyInterval)
	}
	return fmt.Errorf("%s guest agent not ready after %s: %w", domain, timeout, lastErr)
}

func runBootstrapGuestScript(domain string, args []string, timeout time.Duration) (string, error) {
	commandArgs := []string{
		"-lc",
		fmt.Sprintf("set -o pipefail; %s \"$@\" > %s 2>&1; rc=$?; tail -n 80 %s 2>/dev/null || true; exit $rc", bootstrapScriptPath, bootstrapGuestLogPath, bootstrapGuestLogPath),
		"bootstrap",
	}
	commandArgs = append(commandArgs, args...)
	req := libvirtinfra.GuestAgentCommandRequest{
		Execute: "guest-exec",
		Arguments: map[string]any{
			"path":           "/bin/bash",
			"arg":            commandArgs,
			"capture-output": true,
		},
	}
	resp, timedOut, err := libvirtinfra.RunGuestAgentCommand(domain, req, bootstrapGuestCommandTO)
	if timedOut {
		return "", fmt.Errorf("%s guest bootstrap exec timed out", domain)
	}
	if err != nil {
		return "", err
	}
	var execResp bootstrapGuestExecResponse
	if err := json.Unmarshal([]byte(resp), &execResp); err != nil {
		return "", err
	}
	if execResp.Return.PID <= 0 {
		return "", fmt.Errorf("%s guest bootstrap exec returned empty pid", domain)
	}

	deadline := time.Now().Add(timeout)
	for {
		status, err := bootstrapGuestExecStatus(domain, execResp.Return.PID)
		if err != nil {
			return "", err
		}
		if status.Return.Exited {
			output := firstNonEmpty(
				bootstrapDecodeGuestOutput(status.Return.ErrData),
				bootstrapDecodeGuestOutput(status.Return.OutData),
			)
			if status.Return.ExitCode == 0 {
				return output, nil
			}
			return output, fmt.Errorf("%s bootstrap script exited with code %d: %s", domain, status.Return.ExitCode, firstNonEmpty(output, "check "+bootstrapGuestLogPath))
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("%s bootstrap script timed out after %s", domain, timeout)
		}
		time.Sleep(bootstrapGuestPollInterval)
	}
}

func bootstrapGuestExecStatus(domain string, pid int) (bootstrapGuestExecStatusResponse, error) {
	req := libvirtinfra.GuestAgentCommandRequest{
		Execute: "guest-exec-status",
		Arguments: map[string]any{
			"pid": pid,
		},
	}
	resp, timedOut, err := libvirtinfra.RunGuestAgentCommand(domain, req, bootstrapGuestCommandTO)
	if timedOut {
		return bootstrapGuestExecStatusResponse{}, fmt.Errorf("%s guest bootstrap status timed out", domain)
	}
	if err != nil {
		return bootstrapGuestExecStatusResponse{}, err
	}
	var out bootstrapGuestExecStatusResponse
	if err := json.Unmarshal([]byte(resp), &out); err != nil {
		return bootstrapGuestExecStatusResponse{}, err
	}
	return out, nil
}

func bootstrapDecodeGuestOutput(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return value
	}
	return strings.TrimSpace(string(decoded))
}

func bootstrapScriptError(target bootstrapScriptTarget, message string) BootstrapScriptResult {
	return BootstrapScriptResult{
		Role:     target.Role,
		Action:   target.Action,
		Hostname: target.Hostname,
		Target:   firstNonEmpty(target.Target, "local"),
		Domain:   target.Domain,
		Code:     http.StatusInternalServerError,
		Message:  strings.TrimSpace(message),
	}
}

func bootstrapHealthResult(target licenseApplyTarget, health map[string]any) BootstrapHealthResult {
	return BootstrapHealthResult{
		Role:     target.Role,
		Hostname: target.Hostname,
		Target:   firstNonEmpty(mapStringValue(health, "target"), target.Target),
		Code:     mapIntValue(health, "code"),
		Message:  mapStringValue(health, "message"),
		Attempts: mapIntValue(health, "attempts"),
	}
}

func mapStringValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	switch value := values[key].(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	}
}

func mapIntValue(values map[string]any, key string) int {
	if values == nil {
		return 0
	}
	switch value := values[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func bootstrapResponseToDeployOutcome(resp BootstrapResponse) (deployRunStepOutcome, error) {
	if resp.Code == http.StatusOK {
		return deployRunSucceeded(firstNonEmpty(resp.Message, resp.Role+"_bootstrap success"), resp), nil
	}
	return deployRunStepOutcome{Output: resp}, fmt.Errorf("%s", firstNonEmpty(resp.Message, resp.Role+"_bootstrap failed"))
}

func statusCodeFromBootstrapResponse(resp BootstrapResponse) int {
	if resp.Code == http.StatusOK {
		return http.StatusOK
	}
	if resp.Code == http.StatusBadRequest {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
