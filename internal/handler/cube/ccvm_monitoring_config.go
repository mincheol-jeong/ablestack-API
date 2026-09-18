package cube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"ablecloud.io/ablestack-api/internal/infra/utils"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"ablecloud.io/ablestack-api/internal/service/wallservice"
	"github.com/gin-gonic/gin"
)

type CCVMMonitoringConfigRequest = CubeModel.CCVMMonitoringConfigRequest
type CCVMMonitoringConfigResponse = CubeModel.CCVMMonitoringConfigResponse
type CCVMMonitoringConfigStep = CubeModel.CCVMMonitoringConfigStep
type CCVMMonitoringServiceStatus = CubeModel.CCVMMonitoringServiceStatus

const (
	ccvmMonitoringConfigLocalHeader = "X-Cube-CCVM-Monitoring-Local"
	ccvmMonitoringConfigTimeout     = 2 * time.Minute
	ccvmMonitoringPythonTimeout     = 10 * time.Minute
	ccvmMonitoringServiceWait       = 90 * time.Second
	ccvmMonitoringServiceInterval   = 2 * time.Second
)

// CCVMMonitoringConfig godoc
//
//	@Summary		Configure or update CCVM monitoring
//	@Description	configure는 CCVM의 기존 Wall Python 도구로 대상 확인, Netdive, Wall/Prometheus/Grafana/Loki, 서비스와 SMTP 구성을 수행하고 API가 실제 서비스 상태와 system profile 반영을 검증합니다. update는 config_wall.py update를 실행하고 서비스 상태를 검증합니다.
//	@Tags			Cube-CCVM
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CubeModel.CCVMMonitoringConfigRequest	false	"monitoring config request"
//	@Success		200		{object}	CubeModel.CCVMMonitoringConfigResponse
//	@Failure		400		{object}	HTTP400BadRequest
//	@Failure		500		{object}	HTTP500InternalServerError
//	@Router			/cube/ccvm/monitoring/config [post]
func CCVMMonitoringConfig(ginContext *gin.Context) {
	req := CCVMMonitoringConfigRequest{}
	if ginContext.Request.ContentLength != 0 {
		if err := ginContext.ShouldBindJSON(&req); err != nil {
			ginContext.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
				ErrCode: http.StatusBadRequest,
				Message: "invalid request",
			})
			return
		}
	}

	if isCCVMMonitoringConfigLocalRequest(ginContext) {
		if err := normalizeCCVMMonitoringConfigRequest(&req); err != nil {
			ginContext.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
				ErrCode: http.StatusBadRequest,
				Message: err.Error(),
			})
			return
		}
		resp := runCCVMMonitoringConfigLocal(req)
		ginContext.JSON(resp.Code, resp)
		return
	}

	cfg, err := loadClusterConfigSection()
	if err != nil {
		ginContext.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
			ErrCode: http.StatusInternalServerError,
			Message: "failed to read cluster.json",
		})
		return
	}

	builtReq := buildCCVMMonitoringConfigRequest(cfg)
	mergeCCVMMonitoringConfigRequest(&builtReq, req)
	req = builtReq
	if err := normalizeCCVMMonitoringConfigRequest(&req); err != nil {
		ginContext.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	target := strings.TrimSpace(cfg.CCVM.IP)
	if target == "" {
		ginContext.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: "ccvm ip required in cluster.json",
		})
		return
	}
	healthStep := runCCVMMonitoringGoStep("check_ccvm_api_health", func(ctx context.Context) (string, error) {
		if isLocalTarget(target) {
			return "ccvm api is local", nil
		}
		return checkCCVMMonitoringAPIHealth(ctx, target)
	})
	if healthStep.Code != http.StatusOK {
		resp := failedCCVMMonitoringResponse(req.Action, []CCVMMonitoringConfigStep{healthStep}, nil, healthStep)
		ginContext.JSON(resp.Code, resp)
		return
	}
	var resp CCVMMonitoringConfigResponse
	if isLocalTarget(target) {
		resp = runCCVMMonitoringConfigLocal(req)
	} else {
		resp, err = callCCVMMonitoringConfigRemote(target, req)
		if err != nil {
			if resp.Code != 0 {
				resp.Steps = append([]CCVMMonitoringConfigStep{healthStep}, resp.Steps...)
				ginContext.JSON(resp.Code, resp)
				return
			}
			ginContext.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
				ErrCode: http.StatusInternalServerError,
				Message: err.Error(),
			})
			return
		}
	}
	resp.Steps = append([]CCVMMonitoringConfigStep{healthStep}, resp.Steps...)
	if resp.Code == http.StatusOK && req.Action == "configure" {
		results, profileErr := resetCloudCenterApplySystemFlagsOnHosts(cfg, []resetCloudCenterSystemFlag{
			{Depth1: "bootstrap", Depth2: "wall", Value: "true"},
		})
		resp.SystemProfile = results
		step := CCVMMonitoringConfigStep{
			Name:    "update_system_profile",
			Status:  "succeeded",
			Code:    http.StatusOK,
			Message: "bootstrap.wall=true applied to all hosts",
		}
		if profileErr != nil {
			step.Status = "failed"
			step.Code = http.StatusInternalServerError
			step.Message = profileErr.Error()
			resp.Code = http.StatusInternalServerError
			resp.Message = "monitoring configuration succeeded but system profile update failed"
		}
		resp.Steps = append(resp.Steps, step)
	}

	ginContext.JSON(resp.Code, resp)
}

func checkCCVMMonitoringAPIHealth(ctx context.Context, target string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, buildTargetURL(target)+"/health", nil)
	if err != nil {
		return "", err
	}
	attachInternalToken(request)
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return "", fmt.Errorf("ccvm api health check failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("ccvm api health check returned %s", response.Status)
	}
	return "ccvm api health check succeeded", nil
}

func isCCVMMonitoringConfigLocalRequest(ginContext *gin.Context) bool {
	return strings.EqualFold(strings.TrimSpace(ginContext.GetHeader(ccvmMonitoringConfigLocalHeader)), "1")
}

func buildCCVMMonitoringConfigRequest(cfg *CubeModel.ClusterConfigSection) CCVMMonitoringConfigRequest {
	req := CCVMMonitoringConfigRequest{Action: "update"}
	if cfg == nil {
		return req
	}

	req.ClusterType = strings.TrimSpace(cfg.Type)
	req.CCVM = []string{strings.TrimSpace(cfg.CCVM.IP)}
	hosts := cfg.Hosts
	if strings.EqualFold(strings.TrimSpace(cfg.Type), "ablestack-standalone") && len(hosts) > 1 {
		hosts = hosts[:1]
	}
	for _, host := range hosts {
		req.Cube = append(req.Cube, strings.TrimSpace(host.Ablecube))
		if isHCITarget(cfg.Type) {
			req.SCVM = append(req.SCVM, strings.TrimSpace(host.ScvmMngt))
		}
	}
	return req
}

func mergeCCVMMonitoringConfigRequest(target *CCVMMonitoringConfigRequest, input CCVMMonitoringConfigRequest) {
	if target == nil {
		return
	}
	if strings.TrimSpace(input.Action) != "" {
		target.Action = input.Action
	}
	if input.SMTP != nil {
		target.SMTP = input.SMTP
	}
}

func normalizeCCVMMonitoringConfigRequest(req *CCVMMonitoringConfigRequest) error {
	if req == nil {
		return fmt.Errorf("request required")
	}
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	if req.Action == "" {
		req.Action = "update"
	}
	if req.Action != "configure" && req.Action != "update" {
		return fmt.Errorf("action must be configure or update")
	}
	req.CCVM = normalizeStringSlice(req.CCVM)
	req.Cube = normalizeStringSlice(req.Cube)
	req.SCVM = normalizeStringSlice(req.SCVM)
	if len(req.CCVM) == 0 {
		return fmt.Errorf("ccvm ip required")
	}
	if len(req.Cube) == 0 {
		return fmt.Errorf("cube hosts required")
	}
	for _, address := range append(append(append([]string{}, req.CCVM...), req.Cube...), req.SCVM...) {
		if parsed := net.ParseIP(address); parsed == nil || parsed.To4() == nil {
			return fmt.Errorf("invalid monitoring target ip: %s", address)
		}
	}
	if isHCITarget(req.ClusterType) && len(req.SCVM) == 0 {
		return fmt.Errorf("scvm hosts required for %s", strings.TrimSpace(req.ClusterType))
	}
	if req.SMTP != nil && req.SMTP.Enabled {
		req.SMTP.Host = strings.TrimSpace(req.SMTP.Host)
		req.SMTP.User = strings.TrimSpace(req.SMTP.User)
		if req.SMTP.Host == "" || req.SMTP.Port < 1 || req.SMTP.Port > 65535 || req.SMTP.User == "" || req.SMTP.Password == "" {
			return fmt.Errorf("smtp host, port, user, and password required")
		}
		if strings.ContainsAny(req.SMTP.Host+req.SMTP.User+req.SMTP.Password, "\r\n") {
			return fmt.Errorf("smtp values must not contain line breaks")
		}
	}
	return nil
}

func runCCVMMonitoringConfigForDeploy(input CCVMMonitoringConfigRequest, cfg *CubeModel.ClusterConfigSection) CCVMMonitoringConfigResponse {
	req := buildCCVMMonitoringConfigRequest(cfg)
	mergeCCVMMonitoringConfigRequest(&req, input)
	if err := normalizeCCVMMonitoringConfigRequest(&req); err != nil {
		return CCVMMonitoringConfigResponse{Code: http.StatusBadRequest, Action: req.Action, Message: err.Error()}
	}

	target := strings.TrimSpace(cfg.CCVM.IP)
	healthStep := runCCVMMonitoringGoStep("check_ccvm_api_health", func(ctx context.Context) (string, error) {
		if isLocalTarget(target) {
			return "ccvm api is local", nil
		}
		return checkCCVMMonitoringAPIHealth(ctx, target)
	})
	if healthStep.Code != http.StatusOK {
		return failedCCVMMonitoringResponse(req.Action, []CCVMMonitoringConfigStep{healthStep}, nil, healthStep)
	}

	var resp CCVMMonitoringConfigResponse
	var err error
	if isLocalTarget(target) {
		resp = runCCVMMonitoringConfigLocal(req)
	} else {
		resp, err = callCCVMMonitoringConfigRemote(target, req)
		if err != nil {
			if resp.Code == 0 {
				resp = CCVMMonitoringConfigResponse{Code: http.StatusInternalServerError, Action: req.Action, Message: err.Error()}
			}
		}
	}
	resp.Steps = append([]CCVMMonitoringConfigStep{healthStep}, resp.Steps...)
	return resp
}

func runCCVMMonitoringConfigLocal(req CCVMMonitoringConfigRequest) CCVMMonitoringConfigResponse {
	resp := CCVMMonitoringConfigResponse{Code: http.StatusOK, Action: req.Action}
	steps := make([]CCVMMonitoringConfigStep, 0, 8)
	manager := wallservice.New()
	manager.Services = append(manager.Services, "loki", "promtail")

	if req.Action == "configure" {
		step := runCCVMMonitoringPythonStep(manager, "check_targets", "host_ping_test.py", monitoringPingArgs(req)...)
		steps = append(steps, step)
		if step.Code != http.StatusOK {
			return failedCCVMMonitoringResponse(req.Action, steps, nil, step)
		}

		netdiveStep := runCCVMMonitoringPythonStep(manager, "configure_netdive", "config_netdive.py", monitoringConfigArgs("config", req, false)...)
		steps = append(steps, netdiveStep)
		if netdiveStep.Code != http.StatusOK {
			return failedCCVMMonitoringResponse(req.Action, steps, nil, netdiveStep)
		}

		stopStep := runCCVMMonitoringPythonStep(manager, "stop_services", "start_services.py", monitoringServiceArgs("stop")...)
		steps = append(steps, stopStep)
		if stopStep.Code != http.StatusOK {
			steps = append(steps, recoverCCVMMonitoringPythonServices(manager))
			return failedCCVMMonitoringResponse(req.Action, steps, nil, stopStep)
		}
	}

	wallStep := runCCVMMonitoringPythonStep(manager, "configure_wall", "config_wall.py", monitoringConfigArgs(req.Action, req, true)...)
	steps = append(steps, wallStep)
	if wallStep.Code != http.StatusOK {
		if req.Action == "configure" {
			steps = append(steps, recoverCCVMMonitoringPythonServices(manager))
		}
		return failedCCVMMonitoringResponse(req.Action, steps, nil, wallStep)
	}

	if req.Action == "configure" {
		startStep := runCCVMMonitoringPythonStep(manager, "start_services", "start_services.py", monitoringServiceArgs("start")...)
		steps = append(steps, startStep)
		if startStep.Code != http.StatusOK {
			return failedCCVMMonitoringResponse(req.Action, steps, nil, startStep)
		}
	}

	if req.Action == "configure" && req.SMTP != nil && req.SMTP.Enabled {
		smtpStep := runCCVMMonitoringPythonStep(manager, "configure_smtp", "config_smtp.py",
			"config", "--host", net.JoinHostPort(req.SMTP.Host, fmt.Sprint(req.SMTP.Port)),
			"--user", req.SMTP.User, "--password", req.SMTP.Password)
		steps = append(steps, smtpStep)
		if smtpStep.Code != http.StatusOK {
			return failedCCVMMonitoringResponse(req.Action, steps, nil, smtpStep)
		}
	}

	services, verifyStep := verifyCCVMMonitoringServices(manager)
	steps = append(steps, verifyStep)
	if verifyStep.Code != http.StatusOK {
		return failedCCVMMonitoringResponse(req.Action, steps, services, verifyStep)
	}

	resp.Steps = steps
	resp.Services = services
	resp.Message = "monitoring configuration success"
	if req.Action == "update" {
		resp.Message = "monitoring config update success"
	}
	resp.Val = "ok"
	return resp
}

func monitoringPingArgs(req CCVMMonitoringConfigRequest) []string {
	args := []string{"-hns"}
	args = append(args, req.CCVM...)
	args = append(args, req.Cube...)
	args = append(args, req.SCVM...)
	return args
}

func monitoringConfigArgs(action string, req CCVMMonitoringConfigRequest, includeSCVM bool) []string {
	args := []string{monitoringPythonAction(action), "--ccvm"}
	args = append(args, req.CCVM...)
	args = append(args, "--cube")
	args = append(args, req.Cube...)
	if includeSCVM && len(req.SCVM) > 0 {
		args = append(args, "--scvm")
		args = append(args, req.SCVM...)
	}
	return args
}

func monitoringPythonAction(action string) string {
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "configure" {
		return "config"
	}
	return action
}

func monitoringServiceArgs(action string) []string {
	args := []string{action, "--service", "blackbox-exporter", "node-exporter", "grafana-server", "process-exporter", "prometheus"}
	if action != "stop" {
		args = append(args, "netdive-analyzer")
	}
	return args
}

func runCCVMMonitoringPythonStep(manager *wallservice.Manager, name string, script string, args ...string) CCVMMonitoringConfigStep {
	appendAbleStackAPILog("ccvm-monitoring", "step=%s script=%s status=started", name, script)
	step := runCCVMMonitoringStep(name, ccvmMonitoringPythonTimeout, func(ctx context.Context) (string, error) {
		result, output, err := manager.RunPython(ctx, script, args...)
		if err != nil {
			return output, err
		}
		return firstNonEmpty(fmt.Sprint(result.Val), result.Message, output), nil
	})
	if step.Status == "failed" && strings.Contains(step.Message, "timed out") {
		step.Message = fmt.Sprintf("%s timed out after %s", script, ccvmMonitoringPythonTimeout)
		if script == "config_netdive.py" {
			step.Message += "; verify passwordless root SSH/SCP from CCVM to every cluster.json Cube IP"
		}
	}
	appendAbleStackAPILog("ccvm-monitoring", "step=%s script=%s status=%s code=%d message=%s", name, script, step.Status, step.Code, step.Message)
	return step
}

func runCCVMMonitoringGoStep(name string, run func(context.Context) (string, error)) CCVMMonitoringConfigStep {
	return runCCVMMonitoringStep(name, ccvmMonitoringConfigTimeout, run)
}

func runCCVMMonitoringStep(name string, timeout time.Duration, run func(context.Context) (string, error)) CCVMMonitoringConfigStep {
	step := CCVMMonitoringConfigStep{Name: name, Status: "failed", Code: http.StatusInternalServerError}
	commandContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	output, err := run(commandContext)
	step.Output = truncateMonitoringOutput(output)
	if commandContext.Err() == context.DeadlineExceeded {
		step.Message = fmt.Sprintf("%s timed out after %s", name, timeout)
		return step
	}
	if err != nil {
		step.Message = firstNonEmpty(err.Error(), step.Output)
		return step
	}
	step.Code = http.StatusOK
	step.Message = firstNonEmpty(output, "ok")
	step.Status = "succeeded"
	return step
}

func recoverCCVMMonitoringPythonServices(manager *wallservice.Manager) CCVMMonitoringConfigStep {
	return runCCVMMonitoringPythonStep(manager, "recover_services", "start_services.py", monitoringServiceArgs("start")...)
}

func verifyCCVMMonitoringServices(manager *wallservice.Manager) ([]CCVMMonitoringServiceStatus, CCVMMonitoringConfigStep) {
	deadline := time.Now().Add(ccvmMonitoringServiceWait)
	attempts := 0
	for {
		attempts++
		serviceCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		wallStatuses := manager.ServiceStatuses(serviceCtx)
		cancel()
		services := make([]CCVMMonitoringServiceStatus, 0, len(wallStatuses))
		failed := make([]string, 0)
		for _, service := range wallStatuses {
			services = append(services, CCVMMonitoringServiceStatus{Name: service.Name, Active: service.Active, Enabled: service.Enabled})
			if !service.Active || !service.Enabled {
				failed = append(failed, fmt.Sprintf("%s(active=%t,enabled=%t)", service.Name, service.Active, service.Enabled))
			}
		}
		if len(failed) == 0 {
			return services, CCVMMonitoringConfigStep{
				Name:    "verify_services",
				Status:  "succeeded",
				Code:    http.StatusOK,
				Message: fmt.Sprintf("all monitoring services are active and enabled after %d checks", attempts),
			}
		}
		if time.Now().After(deadline) {
			return services, CCVMMonitoringConfigStep{
				Name:    "verify_services",
				Status:  "failed",
				Code:    http.StatusInternalServerError,
				Message: "monitoring services not ready: " + strings.Join(failed, ", "),
			}
		}
		time.Sleep(ccvmMonitoringServiceInterval)
	}
}

func failedCCVMMonitoringResponse(action string, steps []CCVMMonitoringConfigStep, services []CCVMMonitoringServiceStatus, failed CCVMMonitoringConfigStep) CCVMMonitoringConfigResponse {
	return CCVMMonitoringConfigResponse{
		Code:     http.StatusInternalServerError,
		Action:   action,
		Message:  firstNonEmpty(failed.Message, "monitoring configuration failed"),
		Val:      firstNonEmpty(failed.Output, failed.Message),
		Steps:    steps,
		Services: services,
	}
}

func truncateMonitoringOutput(value string) string {
	const maxOutput = 4096
	value = strings.TrimSpace(value)
	if len(value) <= maxOutput {
		return value
	}
	return value[:maxOutput] + "..."
}

func callCCVMMonitoringConfigRemote(target string, req CCVMMonitoringConfigRequest) (CCVMMonitoringConfigResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return CCVMMonitoringConfigResponse{}, err
	}

	url := fmt.Sprintf("%s/api/v1/cube/ccvm/monitoring/config", buildTargetURL(target))
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return CCVMMonitoringConfigResponse{}, err
	}
	attachInternalToken(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(ccvmMonitoringConfigLocalHeader, "1")

	client := &http.Client{Timeout: 15*time.Minute + 10*time.Second}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return CCVMMonitoringConfigResponse{}, err
	}
	defer httpResp.Body.Close()

	var resp CCVMMonitoringConfigResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return CCVMMonitoringConfigResponse{}, err
	}
	if httpResp.StatusCode >= 300 {
		return resp, fmt.Errorf("%s", firstNonEmpty(resp.Message, resp.Val, httpResp.Status))
	}
	return resp, nil
}
