package cube

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"ablecloud.io/ablestack-api/internal/infra/utils"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"ablecloud.io/ablestack-api/internal/service/licenseservice"
	"ablecloud.io/ablestack-api/internal/service/security"
	"github.com/gin-gonic/gin"
)

type LicenseApplyRequest = CubeModel.LicenseApplyRequest
type LicenseApplyResponse = CubeModel.LicenseApplyResponse
type LicenseApplyTargetResult = CubeModel.LicenseApplyTargetResult

const (
	licenseApplyRemoteTimeout = 30 * time.Second
	ccvmLicenseReadyInterval  = 30 * time.Second
	ccvmLicenseReadyAttempts  = 6
)

const (
	licenseApplyRoleAblecube = "ablecube"
	licenseApplyRoleSCVM     = "scvm"
	licenseApplyRoleCCVM     = "ccvm"
	licenseApplyRoleCustom   = "custom"
)

type licenseApplyTarget struct {
	Role     string
	Hostname string
	Target   string
	Host     *CubeModel.ClusterHost
}

// ApplyLicenseToCluster godoc
//
//	@Summary		License Apply
//	@Description	마스터 노드의 라이선스 또는 요청으로 전달된 라이선스를 cluster.json 대상에 role 기반 fan-out 등록합니다. roles가 비어 있으면 기존 호환성을 위해 hosts[].ablecube만 대상으로 합니다.
//	@Tags			Cube-License
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CubeModel.LicenseApplyRequest	false	"license apply request"
//	@Success		200	{object}	CubeModel.LicenseApplyResponse
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/license/apply [post]
func ApplyLicenseToCluster(context *gin.Context) {
	req := LicenseApplyRequest{Action: "register"}
	if context.Request != nil && context.Request.Body != nil && context.Request.ContentLength != 0 {
		if err := context.ShouldBindJSON(&req); err != nil {
			context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
				ErrCode: http.StatusBadRequest,
				Message: "invalid request",
			})
			return
		}
	}

	cfg, err := loadClusterConfigSection()
	if err != nil && len(req.Targets) == 0 {
		context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
			ErrCode: http.StatusInternalServerError,
			Message: "failed to read cluster.json",
		})
		return
	}

	resp := runLicenseApply(req, cfg, context.GetHeader("Authorization"))
	context.JSON(statusCodeFromLicenseApplyResponse(resp), resp)
}

func runLicenseApply(req LicenseApplyRequest, cfg *CubeModel.ClusterConfigSection, authHeader string) LicenseApplyResponse {
	action := normalizeLicenseApplyAction(req.Action)
	if action == "" {
		return LicenseApplyResponse{Code: http.StatusBadRequest, Message: "unsupported action"}
	}
	req.Action = action
	req.Filename = firstNonEmpty(req.Filename, defaultLicenseFilename)

	targets, err := buildLicenseApplyTargets(req, cfg)
	if err != nil {
		return LicenseApplyResponse{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if len(targets) == 0 {
		return LicenseApplyResponse{Code: http.StatusBadRequest, Message: "no targets to apply"}
	}
	appendAbleStackAPILog("license_apply", "event=start action=%q target_count=%d wait_for_ready=%t", action, len(targets), req.WaitForReady)

	defaultContent := strings.TrimSpace(req.LicenseContent)
	if action == "register" && defaultContent == "" && len(req.Licenses) == 0 {
		defaultContent, err = currentLocalLicenseContent()
		if err != nil {
			appendAbleStackAPILog("license_apply", "event=local_license_read_failed error=%q", err.Error())
			return LicenseApplyResponse{Code: http.StatusBadRequest, Message: "license_content required: " + err.Error()}
		}
		appendAbleStackAPILog("license_apply", "event=local_license_loaded encoded_bytes=%d", len(defaultContent))
	}

	results := make([]LicenseApplyTargetResult, 0, len(targets))
	for _, target := range targets {
		attempts := 0
		if action == "register" && req.WaitForReady && target.Role == licenseApplyRoleCCVM {
			attempts, err = waitCCVMLicenseTargetReady(cfg, target.Target)
			if err != nil {
				results = append(results, LicenseApplyTargetResult{
					Role:     target.Role,
					Hostname: target.Hostname,
					Target:   target.Target,
					Code:     http.StatusInternalServerError,
					Message:  err.Error(),
					Attempts: attempts,
				})
				continue
			}
		}
		content := licenseContentForTarget(req, target, defaultContent)
		appendAbleStackAPILog("license_apply", "event=target_apply_start role=%q hostname=%q target=%q action=%q attempts=%d", target.Role, target.Hostname, target.Target, action, attempts)
		result := applyLicenseOnTarget(target, req, content, authHeader)
		result.Attempts = attempts
		appendAbleStackAPILog("license_apply", "event=target_apply_finished role=%q hostname=%q target=%q code=%d message=%q attempts=%d", target.Role, target.Hostname, target.Target, result.Code, result.Message, attempts)
		results = append(results, result)
	}

	if failed := firstFailedLicenseApplyResult(results); failed != nil {
		return LicenseApplyResponse{
			Code:    http.StatusInternalServerError,
			Message: firstNonEmpty(failed.Message, "license apply failed"),
			Results: results,
		}
	}
	return LicenseApplyResponse{Code: http.StatusOK, Message: "license apply success", Results: results}
}

func waitCCVMLicenseTargetReady(cfg *CubeModel.ClusterConfigSection, target string) (int, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return 0, fmt.Errorf("ccvm license readiness target is empty")
	}
	if cfg == nil {
		return 0, fmt.Errorf("ccvm license readiness cluster config is empty")
	}

	readinessSource := "pcs:cloudcenter_res"
	if resetCloudCenterIsStandalone(cfg) {
		readinessSource = "libvirt:ccvm"
	}
	appendAbleStackAPILog("ccvm_license", "event=readiness_start target=%q source=%q interval=%s max_attempts=%d", target, readinessSource, ccvmLicenseReadyInterval, ccvmLicenseReadyAttempts)
	var lastErr error
	for attempt := 1; attempt <= ccvmLicenseReadyAttempts; attempt++ {
		time.Sleep(ccvmLicenseReadyInterval)
		ready, startedNode, err := ccvmLicenseRuntimeReady(cfg)
		if err == nil && ready {
			appendAbleStackAPILog("ccvm_license", "event=readiness_success target=%q source=%q started_node=%q attempt=%d", target, readinessSource, startedNode, attempt)
			return attempt, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("ccvm runtime is not started")
		}
		appendAbleStackAPILog("ccvm_license", "event=readiness_retry target=%q source=%q started_node=%q attempt=%d error=%q", target, readinessSource, startedNode, attempt, lastErr.Error())
	}
	appendAbleStackAPILog("ccvm_license", "event=readiness_failed target=%q source=%q attempts=%d error=%q", target, readinessSource, ccvmLicenseReadyAttempts, lastErr.Error())
	return ccvmLicenseReadyAttempts, fmt.Errorf(
		"ccvm runtime readiness check failed after %d attempts at %s intervals: %w",
		ccvmLicenseReadyAttempts,
		ccvmLicenseReadyInterval,
		lastErr,
	)
}

func ccvmLicenseRuntimeReady(cfg *CubeModel.ClusterConfigSection) (bool, string, error) {
	if resetCloudCenterIsStandalone(cfg) {
		state, exists, err := readLocalCCVMState()
		if err != nil {
			return false, "", err
		}
		return exists && strings.EqualFold(strings.TrimSpace(state), "running"), "local", nil
	}

	status, err := ccvmSecondaryResizePCSStatus(cfg)
	if err != nil {
		return false, "", err
	}
	startedNode := strings.TrimSpace(status.Started)
	return ccvmLicensePCSReady(status), startedNode, nil
}

func ccvmLicensePCSReady(status ccvmSecondaryResizePCSStatusValue) bool {
	return strings.EqualFold(strings.TrimSpace(status.Role), "Started") && strings.TrimSpace(status.Started) != ""
}

func normalizeLicenseApplyAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "", "register", "update", "create":
		return "register"
	case "status":
		return "status"
	default:
		return ""
	}
}

func buildLicenseApplyTargets(req LicenseApplyRequest, cfg *CubeModel.ClusterConfigSection) ([]licenseApplyTarget, error) {
	if len(req.Targets) > 0 {
		targets := make([]licenseApplyTarget, 0, len(req.Targets))
		for _, target := range dedupeHosts(req.Targets) {
			targets = append(targets, licenseApplyTarget{Role: licenseApplyRoleCustom, Target: target})
		}
		return targets, nil
	}
	if cfg == nil {
		return nil, fmt.Errorf("clusterConfig required")
	}

	roles, err := normalizeLicenseApplyRoles(req.Roles)
	if err != nil {
		return nil, err
	}
	filter := licenseApplyHostnameFilter(req.TargetHostnames)

	targets := make([]licenseApplyTarget, 0, len(cfg.Hosts))
	seen := map[string]struct{}{}
	add := func(role string, hostname string, target string, host *CubeModel.ClusterHost) {
		role = strings.TrimSpace(role)
		hostname = strings.TrimSpace(hostname)
		target = strings.TrimSpace(target)
		if target == "" {
			return
		}
		if !licenseApplyTargetMatchesFilter(role, hostname, host, filter) {
			return
		}
		if _, ok := seen[target]; ok {
			return
		}
		seen[target] = struct{}{}
		targets = append(targets, licenseApplyTarget{
			Role:     role,
			Hostname: hostname,
			Target:   target,
			Host:     host,
		})
	}

	for _, role := range roles {
		switch role {
		case licenseApplyRoleAblecube:
			for i := range cfg.Hosts {
				host := &cfg.Hosts[i]
				add(role, host.Hostname, host.Ablecube, host)
			}
		case licenseApplyRoleSCVM:
			for i := range cfg.Hosts {
				host := &cfg.Hosts[i]
				add(role, licenseApplySCVMHostname(host), firstNonEmpty(host.ScvmMngt, host.Scvm), host)
			}
		case licenseApplyRoleCCVM:
			add(role, licenseApplyRoleCCVM, cfg.CCVM.IP, nil)
		}
	}
	if len(filter) > 0 && len(targets) == 0 {
		return nil, fmt.Errorf("target_hostname not found")
	}
	return targets, nil
}

func normalizeLicenseApplyRoles(values []string) ([]string, error) {
	parts := splitLicenseApplyValues(values)
	if len(parts) == 0 {
		return []string{licenseApplyRoleAblecube}, nil
	}

	roles := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	add := func(role string) {
		if _, ok := seen[role]; ok {
			return
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}

	for _, part := range parts {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "all":
			add(licenseApplyRoleAblecube)
			add(licenseApplyRoleSCVM)
			add(licenseApplyRoleCCVM)
		case "ablecube", "host", "hosts", "node", "nodes", "physical":
			add(licenseApplyRoleAblecube)
		case "scvm", "storage", "storage-vm", "storage_vm":
			add(licenseApplyRoleSCVM)
		case "ccvm", "cloud", "cloud-vm", "cloud_vm":
			add(licenseApplyRoleCCVM)
		default:
			return nil, fmt.Errorf("unsupported license target role: %s", part)
		}
	}
	return roles, nil
}

func splitLicenseApplyValues(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
		}) {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func licenseApplyHostnameFilter(values []string) map[string]struct{} {
	filter := map[string]struct{}{}
	for _, value := range splitLicenseApplyValues(values) {
		filter[strings.ToLower(value)] = struct{}{}
	}
	return filter
}

func licenseApplyTargetMatchesFilter(role string, hostname string, host *CubeModel.ClusterHost, filter map[string]struct{}) bool {
	if len(filter) == 0 {
		return true
	}
	for _, candidate := range licenseApplyTargetFilterNames(role, hostname, host) {
		if _, ok := filter[strings.ToLower(strings.TrimSpace(candidate))]; ok {
			return true
		}
	}
	return false
}

func licenseApplyTargetFilterNames(role string, hostname string, host *CubeModel.ClusterHost) []string {
	names := []string{hostname, role}
	if host != nil {
		names = append(names,
			host.Hostname,
			host.Index,
			role+strings.TrimSpace(host.Index),
		)
	}
	if role == licenseApplyRoleCCVM {
		names = append(names, licenseApplyRoleCCVM)
	}
	return names
}

func licenseApplySCVMHostname(host *CubeModel.ClusterHost) string {
	if host == nil {
		return licenseApplyRoleSCVM
	}
	if strings.TrimSpace(host.Index) != "" {
		return licenseApplyRoleSCVM + strings.TrimSpace(host.Index)
	}
	return firstNonEmpty(host.Hostname, licenseApplyRoleSCVM)
}

func currentLocalLicenseContent() (string, error) {
	path, err := licenseservice.CurrentLicenseFile()
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(raw)) == "" {
		return "", fmt.Errorf("current license file is empty")
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

func licenseContentForTarget(req LicenseApplyRequest, target licenseApplyTarget, defaultContent string) string {
	if len(req.Licenses) == 0 {
		return strings.TrimSpace(defaultContent)
	}
	keys := []string{
		target.Hostname,
		target.Target,
		target.Role,
	}
	if target.Role != "" && target.Hostname != "" {
		keys = append(keys, target.Role+":"+target.Hostname)
	}
	if target.Host != nil {
		keys = append(keys,
			target.Host.Hostname,
			target.Host.Ablecube,
			target.Host.ScvmMngt,
			target.Host.Scvm,
			target.Host.Index,
		)
		if target.Role == licenseApplyRoleAblecube {
			keys = append(keys, "ablecube"+strings.TrimSpace(target.Host.Index))
		}
		if target.Role == licenseApplyRoleSCVM {
			keys = append(keys, "scvm"+strings.TrimSpace(target.Host.Index))
		}
	}
	for _, key := range keys {
		if value := licenseMapValue(req.Licenses, key); value != "" {
			return value
		}
	}
	return strings.TrimSpace(defaultContent)
}

func licenseMapValue(values map[string]string, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if value := strings.TrimSpace(values[key]); value != "" {
		return value
	}
	lowerKey := strings.ToLower(key)
	for rawKey, value := range values {
		if strings.ToLower(strings.TrimSpace(rawKey)) == lowerKey {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func applyLicenseOnTarget(target licenseApplyTarget, req LicenseApplyRequest, content string, authHeader string) LicenseApplyTargetResult {
	result := LicenseApplyTargetResult{
		Role:     target.Role,
		Hostname: target.Hostname,
		Target:   target.Target,
	}
	if req.Action == "register" && strings.TrimSpace(content) == "" {
		result.Code = http.StatusBadRequest
		result.Message = "license_content required"
		return result
	}

	var resp LicenseResponse
	var err error
	if isLocalTarget(target.Target) {
		resp, err = applyLicenseLocal(req, content)
	} else {
		resp, err = callLicenseRemote(target.Target, req, content, authHeader)
	}
	if err != nil {
		result.Code = http.StatusInternalServerError
		result.Message = err.Error()
		return result
	}

	result.Code = resp.Code
	result.Val = resp.Val
	result.Message = licenseApplyMessage(resp)
	if result.Code == 0 {
		result.Code = http.StatusOK
	}
	return result
}

func applyLicenseLocal(req LicenseApplyRequest, content string) (LicenseResponse, error) {
	switch req.Action {
	case "status":
		return getLicenseStatus()
	case "register":
		status, err := licenseservice.Register(content, req.Filename)
		if err != nil {
			if isLicenseInputError(err) {
				return LicenseResponse{Code: http.StatusBadRequest, Val: fmt.Sprintf("라이센스 내용을 처리할 수 없습니다: %s", err.Error())}, nil
			}
			return LicenseResponse{Code: http.StatusInternalServerError, Val: fmt.Sprintf("라이센스 내용을 처리할 수 없습니다: %s", err.Error())}, nil
		}
		if _, _, err := security.EnsureInternalToken(); err != nil {
			return LicenseResponse{}, err
		}
		if err := syncLicenseSystemProfile(status); err != nil {
			return LicenseResponse{}, err
		}
		return LicenseResponse{Code: http.StatusOK, Val: "라이센스가 성공적으로 등록되었습니다."}, nil
	default:
		return LicenseResponse{Code: http.StatusBadRequest, Val: "unsupported action"}, nil
	}
}

func callLicenseRemote(target string, req LicenseApplyRequest, content string, authHeader string) (LicenseResponse, error) {
	body, err := json.Marshal(LicenseRequest{
		Action:         req.Action,
		LicenseContent: content,
	})
	if err != nil {
		return LicenseResponse{}, err
	}

	url := buildTargetURL(target) + "/api/v1/cube/license"
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return LicenseResponse{}, err
	}
	attachInternalToken(httpReq)
	if strings.TrimSpace(authHeader) != "" {
		httpReq.Header.Set("Authorization", authHeader)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	appendAbleStackAPILog("license_apply", "event=remote_request_start target=%q url=%q action=%q", target, url, req.Action)
	resp, err := (&http.Client{Timeout: licenseApplyRemoteTimeout}).Do(httpReq)
	if err != nil {
		appendAbleStackAPILog("license_apply", "event=remote_request_failed target=%q url=%q error=%q", target, url, err.Error())
		return LicenseResponse{}, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var out LicenseResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		if resp.StatusCode >= 300 {
			return LicenseResponse{}, fmt.Errorf("license apply failed: %s", firstNonEmpty(strings.TrimSpace(string(raw)), resp.Status))
		}
		return LicenseResponse{}, err
	}
	if out.Code == 0 {
		out.Code = resp.StatusCode
	}
	appendAbleStackAPILog("license_apply", "event=remote_response target=%q url=%q http_status=%d response_code=%d", target, url, resp.StatusCode, out.Code)
	return out, nil
}

func licenseApplyMessage(resp LicenseResponse) string {
	if resp.Code == http.StatusOK {
		return "ok"
	}
	switch value := resp.Val.(type) {
	case string:
		return value
	default:
		if value == nil {
			return ""
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(raw)
	}
}

func firstFailedLicenseApplyResult(results []LicenseApplyTargetResult) *LicenseApplyTargetResult {
	for i := range results {
		if results[i].Code != http.StatusOK {
			return &results[i]
		}
	}
	return nil
}

func statusCodeFromLicenseApplyResponse(resp LicenseApplyResponse) int {
	if resp.Code == http.StatusOK {
		return http.StatusOK
	}
	if resp.Code == http.StatusBadRequest {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
