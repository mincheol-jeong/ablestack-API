package cube

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"ablecloud.io/ablestack-api/internal/infra/utils"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid"
)

type DeployRunRequest = CubeModel.DeployRunRequest
type DeployRunStartResponse = CubeModel.DeployRunStartResponse
type DeployRunJobResponse = CubeModel.DeployRunJobResponse
type DeployRunJobListResponse = CubeModel.DeployRunJobListResponse
type DeployRunStepResult = CubeModel.DeployRunStepResult

const (
	deployRunJobLimit           = 50
	deployRunRemoteHTTPTO       = 10 * time.Minute
	deployRunCCVMCloudInitTO    = 10 * time.Minute
	deployRunBootstrapReadyTO   = 5 * time.Minute
	deployRunHealthCheckTO      = 5 * time.Second
	deployRunHealthRetryDelay   = 5 * time.Second
	deployRunSCVMHealthAttempts = 5
	deployRunSCVMHealthInterval = 20 * time.Second
	deployRunSuccessMessage     = "deploy job succeeded"
	deployRunFailedMessage      = "deploy job failed"
	deployRunStartedMessage     = "deploy job started"
	deployRunDefaultMode        = "all"
	deployRunPartialMode        = "partial"
)

var deployRunJobs = newDeployRunJobStore()

type deployRunJobStore struct {
	mu    sync.RWMutex
	jobs  map[string]CubeModel.DeployRunJob
	order []string
}

type deployRunStepOutcome struct {
	Status  string
	Message string
	Output  any
}

func newDeployRunJobStore() *deployRunJobStore {
	return &deployRunJobStore{
		jobs: map[string]CubeModel.DeployRunJob{},
	}
}

// StartDeployRun godoc
//
//	@Summary		All-in-one Deploy Run
//	@Description	기존 개별 API를 보존한 상태에서 라이선스/클러스터/SCVM/RBD/GFS/CCVM/Wall 모니터링 준비 단계를 job으로 순차 실행합니다. queued/running job은 동시에 하나만 허용합니다.
//	@Tags			Cube-Deploy
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CubeModel.DeployRunRequest	true	"deploy run request"
//	@Success		202	{object}	CubeModel.DeployRunStartResponse
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		409	{object}	HTTP409Conflict
//	@Router			/cube/deploy/run [post]
func StartDeployRun(context *gin.Context) {
	var req DeployRunRequest
	if err := context.ShouldBindJSON(&req); err != nil {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: "invalid request",
		})
		return
	}

	if err := validateDeployRunSelection(req); err != nil {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	steps := selectedDeployRunSteps(req)
	if len(steps) == 0 {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: "no steps to run",
		})
		return
	}

	job, ok := deployRunJobs.createIfIdle(req, steps)
	if !ok {
		context.JSON(http.StatusConflict, utils.HTTP409Conflict{
			ErrCode: http.StatusConflict,
			Message: "another deploy job is already queued or running",
		})
		return
	}
	authHeader := context.GetHeader("Authorization")
	go runDeployRunJob(job.JobID, req, steps, authHeader)

	context.JSON(http.StatusAccepted, DeployRunStartResponse{
		Code:    http.StatusAccepted,
		JobID:   job.JobID,
		Status:  job.Status,
		Message: deployRunStartedMessage,
		Steps:   job.Steps,
	})
}

// GetDeployRunJob godoc
//
//	@Summary		All-in-one Deploy Job
//	@Description	올인원 배포 job 상태와 step별 결과를 반환합니다.
//	@Tags			Cube-Deploy
//	@Produce		json
//	@Param			job_id	path		string	true	"job id"
//	@Success		200		{object}	CubeModel.DeployRunJobResponse
//	@Failure		404		{object}	HTTP404NotFound
//	@Router			/cube/deploy/jobs/{job_id} [get]
func GetDeployRunJob(context *gin.Context) {
	jobID := strings.TrimSpace(context.Param("job_id"))
	job, ok := deployRunJobs.get(jobID)
	if !ok {
		context.JSON(http.StatusNotFound, gin.H{"code": http.StatusNotFound, "message": "job not found"})
		return
	}
	context.JSON(http.StatusOK, DeployRunJobResponse{Code: http.StatusOK, Job: job, Message: "ok"})
}

// ListDeployRunJobs godoc
//
//	@Summary		All-in-one Deploy Jobs
//	@Description	최근 올인원 배포 job 목록을 반환합니다.
//	@Tags			Cube-Deploy
//	@Produce		json
//	@Success		200	{object}	CubeModel.DeployRunJobListResponse
//	@Router			/cube/deploy/jobs [get]
func ListDeployRunJobs(context *gin.Context) {
	context.JSON(http.StatusOK, DeployRunJobListResponse{
		Code:    http.StatusOK,
		Jobs:    deployRunJobs.list(),
		Message: "ok",
	})
}

func (s *deployRunJobStore) createIfIdle(req DeployRunRequest, steps []string) (CubeModel.DeployRunJob, bool) {
	now := time.Now()
	jobID := newDeployRunJobID()
	stepResults := make([]CubeModel.DeployRunStepResult, 0, len(steps))
	for _, step := range steps {
		stepResults = append(stepResults, CubeModel.DeployRunStepResult{
			Name:   step,
			Status: CubeModel.DeployRunStepStatusPending,
		})
	}
	job := CubeModel.DeployRunJob{
		JobID:     jobID,
		Status:    CubeModel.DeployRunStatusQueued,
		Mode:      normalizeDeployRunMode(req.Mode),
		CreatedAt: now,
		Steps:     stepResults,
	}
	if req.Cluster != nil {
		job.OSType = normalizeDeployOSType(req.Cluster.Type)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.jobs {
		if existing.Status == CubeModel.DeployRunStatusQueued || existing.Status == CubeModel.DeployRunStatusRunning {
			return CubeModel.DeployRunJob{}, false
		}
	}
	s.jobs[jobID] = job
	s.order = append(s.order, jobID)
	s.pruneLocked()
	return cloneDeployRunJob(job), true
}

func (s *deployRunJobStore) get(jobID string) (CubeModel.DeployRunJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return CubeModel.DeployRunJob{}, false
	}
	return cloneDeployRunJob(job), true
}

func (s *deployRunJobStore) list() []CubeModel.DeployRunJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]CubeModel.DeployRunJob, 0, len(s.order))
	for i := len(s.order) - 1; i >= 0; i-- {
		if job, ok := s.jobs[s.order[i]]; ok {
			jobs = append(jobs, cloneDeployRunJob(job))
		}
	}
	return jobs
}

func (s *deployRunJobStore) update(jobID string, update func(*CubeModel.DeployRunJob)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return
	}
	update(&job)
	s.jobs[jobID] = job
}

func (s *deployRunJobStore) pruneLocked() {
	for len(s.order) > deployRunJobLimit {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.jobs, oldest)
	}
}

func cloneDeployRunJob(job CubeModel.DeployRunJob) CubeModel.DeployRunJob {
	if len(job.Steps) > 0 {
		steps := make([]CubeModel.DeployRunStepResult, len(job.Steps))
		copy(steps, job.Steps)
		job.Steps = steps
	}
	return job
}

func newDeployRunJobID() string {
	id, err := uuid.NewV4()
	if err == nil {
		return id.String()
	}
	return fmt.Sprintf("deploy-%d", time.Now().UnixNano())
}

func normalizeDeployRunMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", deployRunDefaultMode:
		return deployRunDefaultMode
	case deployRunPartialMode:
		return deployRunPartialMode
	default:
		return deployRunDefaultMode
	}
}

func selectedDeployRunSteps(req DeployRunRequest) []string {
	allSteps := []string{
		CubeModel.DeployRunStepLicenseApply,
		CubeModel.DeployRunStepClusterApply,
		CubeModel.DeployRunStepSCVMPrepare,
		CubeModel.DeployRunStepSCVMBootstrap,
		CubeModel.DeployRunStepRBDPrepare,
		CubeModel.DeployRunStepStoragePrepare,
		CubeModel.DeployRunStepLocalPrepare,
		CubeModel.DeployRunStepCCVMPrepare,
		CubeModel.DeployRunStepCCVMBootstrap,
		CubeModel.DeployRunStepMonitoringPrepare,
		CubeModel.DeployRunStepSystemProfile,
	}

	var selected []string
	if len(req.Only) > 0 {
		for _, step := range req.Only {
			normalized := normalizeDeployRunStepName(step)
			if isKnownDeployRunStep(normalized) {
				selected = append(selected, normalized)
			}
		}
		selected = insertDeployRunRBDPrepare(selected, req)
	} else {
		selected = append(selected, allSteps...)
	}

	skip := map[string]struct{}{}
	for _, step := range req.Skip {
		normalized := normalizeDeployRunStepName(step)
		if normalized != "" {
			skip[normalized] = struct{}{}
		}
	}

	out := make([]string, 0, len(selected))
	seen := map[string]struct{}{}
	for _, step := range selected {
		if _, ok := skip[step]; ok {
			continue
		}
		if _, ok := seen[step]; ok {
			continue
		}
		seen[step] = struct{}{}
		out = append(out, step)
	}
	return out
}

func validateDeployRunSelection(req DeployRunRequest) error {
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode != "" && mode != deployRunDefaultMode && mode != deployRunPartialMode {
		return fmt.Errorf("mode must be all or partial")
	}
	for field, values := range map[string][]string{"only": req.Only, "skip": req.Skip} {
		for _, value := range values {
			if normalizeDeployRunStepName(value) == "" {
				return fmt.Errorf("unsupported %s step: %s", field, strings.TrimSpace(value))
			}
		}
	}
	return nil
}

func insertDeployRunRBDPrepare(steps []string, req DeployRunRequest) []string {
	if req.RBD == nil {
		return steps
	}
	hasRBD := false
	storageIndex := -1
	for index, step := range steps {
		switch step {
		case CubeModel.DeployRunStepRBDPrepare:
			hasRBD = true
		case CubeModel.DeployRunStepStoragePrepare:
			if storageIndex < 0 {
				storageIndex = index
			}
		}
	}
	if hasRBD || storageIndex < 0 {
		return steps
	}
	out := make([]string, 0, len(steps)+1)
	out = append(out, steps[:storageIndex]...)
	out = append(out, CubeModel.DeployRunStepRBDPrepare)
	out = append(out, steps[storageIndex:]...)
	return out
}

func normalizeDeployRunStepName(step string) string {
	normalized := strings.ToLower(strings.TrimSpace(step))
	normalized = strings.NewReplacer("-", "_", " ", "_").Replace(normalized)
	switch normalized {
	case "license", "license_register", "license_apply":
		return CubeModel.DeployRunStepLicenseApply
	case "cluster", "cluster_config", "cluster_apply":
		return CubeModel.DeployRunStepClusterApply
	case "scvm", "storage_vm", "storage_vm_prepare", "scvm_prepare":
		return CubeModel.DeployRunStepSCVMPrepare
	case "scvm_bootstrap", "storage_vm_bootstrap":
		return CubeModel.DeployRunStepSCVMBootstrap
	case "rbd", "rbd_create", "rbd_map", "rbd_prepare":
		return CubeModel.DeployRunStepRBDPrepare
	case "storage", "gfs", "gfs_prepare", "storage_prepare":
		return CubeModel.DeployRunStepStoragePrepare
	case "local", "local_storage", "local_prepare":
		return CubeModel.DeployRunStepLocalPrepare
	case "ccvm", "cloud_vm", "cloud_vm_prepare", "ccvm_prepare":
		return CubeModel.DeployRunStepCCVMPrepare
	case "ccvm_bootstrap", "cloud_vm_bootstrap":
		return CubeModel.DeployRunStepCCVMBootstrap
	case "monitoring", "wall", "wall_config", "monitoring_prepare":
		return CubeModel.DeployRunStepMonitoringPrepare
	case "profile", "system", "system_config", "system_profile":
		return CubeModel.DeployRunStepSystemProfile
	default:
		return ""
	}
}

func isKnownDeployRunStep(step string) bool {
	switch step {
	case CubeModel.DeployRunStepLicenseApply,
		CubeModel.DeployRunStepClusterApply,
		CubeModel.DeployRunStepSCVMPrepare,
		CubeModel.DeployRunStepSCVMBootstrap,
		CubeModel.DeployRunStepRBDPrepare,
		CubeModel.DeployRunStepStoragePrepare,
		CubeModel.DeployRunStepLocalPrepare,
		CubeModel.DeployRunStepCCVMPrepare,
		CubeModel.DeployRunStepCCVMBootstrap,
		CubeModel.DeployRunStepMonitoringPrepare,
		CubeModel.DeployRunStepSystemProfile:
		return true
	default:
		return false
	}
}

func deployRunStepExplicit(req DeployRunRequest, step string) bool {
	for _, value := range req.Only {
		if normalizeDeployRunStepName(value) == step {
			return true
		}
	}
	return false
}

func runDeployRunJob(jobID string, req DeployRunRequest, steps []string, authHeader string) {
	started := time.Now()
	deployRunJobs.update(jobID, func(job *CubeModel.DeployRunJob) {
		job.Status = CubeModel.DeployRunStatusRunning
		job.StartedAt = started
		job.Message = deployRunStartedMessage
	})

	executed := map[string]bool{}
	runtime := &deployRunRuntime{}
	for _, step := range steps {
		deployRunJobs.update(jobID, func(job *CubeModel.DeployRunJob) {
			job.CurrentStep = step
			markDeployRunStepRunning(job, step)
		})

		outcome, err := runDeployRunStep(req, step, authHeader, executed, runtime)
		if err != nil {
			outcome = deployRunStepOutcome{
				Status:  CubeModel.DeployRunStepStatusFailed,
				Message: err.Error(),
				Output:  outcome.Output,
			}
			deployRunJobs.update(jobID, func(job *CubeModel.DeployRunJob) {
				markDeployRunStepFinished(job, step, outcome)
				job.Status = CubeModel.DeployRunStatusFailed
				job.CurrentStep = ""
				job.Message = deployRunFailedMessage + ": " + err.Error()
				job.FinishedAt = time.Now()
			})
			return
		}

		deployRunJobs.update(jobID, func(job *CubeModel.DeployRunJob) {
			markDeployRunStepFinished(job, step, outcome)
		})
		if outcome.Status == CubeModel.DeployRunStepStatusSucceeded {
			executed[step] = true
		}
	}

	deployRunJobs.update(jobID, func(job *CubeModel.DeployRunJob) {
		job.Status = CubeModel.DeployRunStatusSucceeded
		job.CurrentStep = ""
		job.Message = deployRunSuccessMessage
		job.FinishedAt = time.Now()
	})
}

func markDeployRunStepRunning(job *CubeModel.DeployRunJob, step string) {
	now := time.Now()
	for i := range job.Steps {
		if job.Steps[i].Name == step {
			job.Steps[i].Status = CubeModel.DeployRunStepStatusRunning
			job.Steps[i].StartedAt = now
			job.Steps[i].Message = "running"
			return
		}
	}
}

func markDeployRunStepFinished(job *CubeModel.DeployRunJob, step string, outcome deployRunStepOutcome) {
	now := time.Now()
	for i := range job.Steps {
		if job.Steps[i].Name != step {
			continue
		}
		if job.Steps[i].StartedAt.IsZero() {
			job.Steps[i].StartedAt = now
		}
		job.Steps[i].Status = outcome.Status
		job.Steps[i].Message = outcome.Message
		job.Steps[i].FinishedAt = now
		job.Steps[i].DurationMS = now.Sub(job.Steps[i].StartedAt).Milliseconds()
		job.Steps[i].Output = outcome.Output
		return
	}
}

type deployRunRuntime struct {
	rbdImages []string
}

func runDeployRunStep(req DeployRunRequest, step string, authHeader string, executed map[string]bool, runtime *deployRunRuntime) (deployRunStepOutcome, error) {
	switch step {
	case CubeModel.DeployRunStepLicenseApply:
		return runDeployRunLicenseApply(req, authHeader)
	case CubeModel.DeployRunStepClusterApply:
		return runDeployRunClusterApply(req)
	case CubeModel.DeployRunStepSCVMPrepare:
		return runDeployRunSCVMPrepare(req)
	case CubeModel.DeployRunStepSCVMBootstrap:
		return runDeployRunSCVMBootstrap(req, authHeader, executed)
	case CubeModel.DeployRunStepRBDPrepare:
		return runDeployRunRBDPrepare(req, runtime)
	case CubeModel.DeployRunStepStoragePrepare:
		return runDeployRunStoragePrepare(req, runtime)
	case CubeModel.DeployRunStepLocalPrepare:
		return runDeployRunLocalPrepare(req)
	case CubeModel.DeployRunStepCCVMPrepare:
		return runDeployRunCCVMPrepare(req)
	case CubeModel.DeployRunStepCCVMBootstrap:
		return runDeployRunCCVMBootstrap(req, authHeader, executed)
	case CubeModel.DeployRunStepMonitoringPrepare:
		return runDeployRunMonitoringPrepare(req, executed)
	case CubeModel.DeployRunStepSystemProfile:
		return runDeployRunSystemProfile(req, executed)
	default:
		return deployRunStepOutcome{}, fmt.Errorf("unsupported deploy step")
	}
}

func deployRunSucceeded(message string, output any) deployRunStepOutcome {
	return deployRunStepOutcome{Status: CubeModel.DeployRunStepStatusSucceeded, Message: firstNonEmpty(message, "ok"), Output: output}
}

func deployRunSkipped(message string, output any) deployRunStepOutcome {
	return deployRunStepOutcome{Status: CubeModel.DeployRunStepStatusSkipped, Message: firstNonEmpty(message, "skipped"), Output: output}
}

func deployRunMissingInput(req DeployRunRequest, step string, message string) (deployRunStepOutcome, error) {
	if deployRunStepExplicit(req, step) {
		return deployRunStepOutcome{}, fmt.Errorf("%s", message)
	}
	return deployRunSkipped(message, nil), nil
}

func runDeployRunLicenseApply(req DeployRunRequest, authHeader string) (deployRunStepOutcome, error) {
	cfg, err := loadClusterConfigSection()
	if err != nil && len(req.Licenses) == 0 && strings.TrimSpace(req.LicenseContent) == "" {
		return deployRunStepOutcome{}, fmt.Errorf("failed to read cluster.json")
	}
	cfg = deployRunLicenseConfig(req, cfg)
	licenseReq := LicenseApplyRequest{
		Action:         "register",
		LicenseContent: req.LicenseContent,
		Licenses:       req.Licenses,
		Filename:       req.LicenseFilename,
	}
	resp := runLicenseApply(licenseReq, cfg, authHeader)
	if resp.Code != http.StatusOK {
		return deployRunStepOutcome{Output: resp}, errors.New(firstNonEmpty(resp.Message, "license apply failed"))
	}
	return deployRunSucceeded(resp.Message, resp), nil
}

func deployRunLicenseConfig(req DeployRunRequest, current *CubeModel.ClusterConfigSection) *CubeModel.ClusterConfigSection {
	if req.Cluster == nil || len(req.Cluster.Hosts) == 0 {
		return current
	}
	var cfg CubeModel.ClusterConfigSection
	if current != nil {
		cfg = *current
	}
	cfg.Hosts = append([]CubeModel.ClusterHost(nil), req.Cluster.Hosts...)
	if value := strings.TrimSpace(req.Cluster.Type); value != "" {
		cfg.Type = value
	}
	if req.Cluster.CCVM != nil && strings.TrimSpace(req.Cluster.CCVM.IP) != "" {
		cfg.CCVM = *req.Cluster.CCVM
	}
	return &cfg
}

func runDeployRunClusterApply(req DeployRunRequest) (deployRunStepOutcome, error) {
	if req.Cluster == nil {
		return deployRunMissingInput(req, CubeModel.DeployRunStepClusterApply, "cluster request required")
	}
	resp := runClusterConfigApplyForDeploy(*req.Cluster)
	if resp.Code != http.StatusOK {
		return deployRunStepOutcome{Output: resp}, errors.New(firstNonEmpty(resp.Message, "cluster apply failed"))
	}
	return deployRunSucceeded(resp.Message, resp), nil
}

func runDeployRunSCVMPrepare(req DeployRunRequest) (deployRunStepOutcome, error) {
	cfg, err := loadClusterConfigSection()
	if err != nil {
		return deployRunStepOutcome{}, fmt.Errorf("failed to read cluster.json")
	}
	if !isHCITarget(cfg.Type) {
		return deployRunSkipped("scvm_prepare is not required for "+strings.TrimSpace(cfg.Type), nil), nil
	}
	if len(req.SCVMByHost) == 0 {
		return deployRunMissingInput(req, CubeModel.DeployRunStepSCVMPrepare, "scvm_by_host required")
	}

	filter := licenseApplyHostnameFilter(req.TargetHostnames)
	results := make([]map[string]any, 0, len(cfg.Hosts))
	for _, host := range cfg.Hosts {
		if len(filter) > 0 && !licenseApplyTargetMatchesFilter(licenseApplyRoleSCVM, licenseApplySCVMHostname(&host), &host, filter) {
			continue
		}
		target := strings.TrimSpace(host.Ablecube)
		if target == "" {
			continue
		}
		xmlReq, ok := scvmXMLRequestForHost(req.SCVMByHost, host)
		if !ok {
			return deployRunStepOutcome{Output: results}, fmt.Errorf("scvm config not found for host %s", firstNonEmpty(host.Hostname, target))
		}

		result, err := prepareSCVMOnHost(target, host, xmlReq)
		results = append(results, result)
		if err != nil {
			return deployRunStepOutcome{Output: results}, err
		}
	}
	if len(results) == 0 {
		return deployRunStepOutcome{}, fmt.Errorf("SCVM prepare target not found")
	}
	return deployRunSucceeded("scvm prepare success", results), nil
}

func scvmXMLRequestForHost(values map[string]SCVMXMLCreateRequest, host CubeModel.ClusterHost) (SCVMXMLCreateRequest, bool) {
	keys := []string{
		host.Hostname,
		host.Ablecube,
		host.Index,
		"scvm" + strings.TrimSpace(host.Index),
	}
	for _, key := range keys {
		if value, ok := scvmXMLMapValue(values, key); ok {
			return value, true
		}
	}
	return SCVMXMLCreateRequest{}, false
}

func scvmXMLMapValue(values map[string]SCVMXMLCreateRequest, key string) (SCVMXMLCreateRequest, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return SCVMXMLCreateRequest{}, false
	}
	if value, ok := values[key]; ok {
		return value, true
	}
	lowerKey := strings.ToLower(key)
	for rawKey, value := range values {
		if strings.ToLower(strings.TrimSpace(rawKey)) == lowerKey {
			return value, true
		}
	}
	return SCVMXMLCreateRequest{}, false
}

func prepareSCVMOnHost(target string, host CubeModel.ClusterHost, xmlReq SCVMXMLCreateRequest) (map[string]any, error) {
	result := map[string]any{
		"hostname": host.Hostname,
		"target":   target,
	}

	var cloudResp GenCloudInitResponse
	if _, err := deployRunPostJSON(target, "/api/v1/cube/cloudinit/scvm/generate", map[string]any{}, deployRunRemoteHTTPTO, &cloudResp); err != nil {
		result["cloudinit"] = err.Error()
		return result, fmt.Errorf("%s scvm cloudinit failed: %s", firstNonEmpty(host.Hostname, target), err.Error())
	}
	result["cloudinit"] = cloudResp
	if cloudResp.Code != http.StatusOK {
		return result, fmt.Errorf("%s scvm cloudinit failed: %s", firstNonEmpty(host.Hostname, target), firstNonEmpty(cloudResp.Message, fmt.Sprint(cloudResp.Val)))
	}

	var xmlResp SCVMXMLCreateResponse
	if _, err := deployRunPostJSON(target, "/api/v1/cube/scvm/xml", xmlReq, deployRunRemoteHTTPTO, &xmlResp); err != nil {
		result["xml"] = err.Error()
		return result, fmt.Errorf("%s scvm xml failed: %s", firstNonEmpty(host.Hostname, target), err.Error())
	}
	result["xml"] = xmlResp
	if xmlResp.Code != http.StatusOK {
		return result, fmt.Errorf("%s scvm xml failed: %s", firstNonEmpty(host.Hostname, target), firstNonEmpty(xmlResp.Message, fmt.Sprint(xmlResp.Val)))
	}

	lifeResp, err := callSCVMUpdateRemote(target, SCVMUpdateRequest{Action: "setup"})
	if err != nil {
		result["lifecycle"] = err.Error()
		return result, fmt.Errorf("%s scvm setup failed: %s", firstNonEmpty(host.Hostname, target), err.Error())
	}
	result["lifecycle"] = lifeResp
	if lifeResp.Code != http.StatusOK {
		return result, fmt.Errorf("%s scvm setup failed: %s", firstNonEmpty(host.Hostname, target), firstNonEmpty(lifeResp.Message, fmt.Sprint(lifeResp.Val)))
	}
	scvmTarget := firstNonEmpty(host.ScvmMngt, host.Scvm)
	health, err := waitDeployRunAPIHealthWithPolicy(
		scvmTarget,
		deployRunSCVMHealthAttempts,
		deployRunSCVMHealthInterval,
	)
	result["api_health"] = health
	if err != nil {
		return result, fmt.Errorf("%s scvm api health failed: %s", firstNonEmpty(host.Hostname, scvmTarget), err.Error())
	}
	return result, nil
}

func runDeployRunSCVMBootstrap(req DeployRunRequest, authHeader string, executed map[string]bool) (deployRunStepOutcome, error) {
	cfg, err := loadClusterConfigSection()
	if err != nil {
		return deployRunStepOutcome{}, fmt.Errorf("failed to read cluster.json")
	}
	if !isHCITarget(cfg.Type) {
		return deployRunSkipped("scvm_bootstrap is not required for "+strings.TrimSpace(cfg.Type), nil), nil
	}
	if !executed[CubeModel.DeployRunStepSCVMPrepare] && !deployRunStepExplicit(req, CubeModel.DeployRunStepSCVMBootstrap) {
		return deployRunSkipped("scvm_bootstrap waits for scvm_prepare or explicit selection", nil), nil
	}
	return bootstrapResponseToDeployOutcome(runBootstrapRole(bootstrapRequestFromDeployRun(req), cfg, licenseApplyRoleSCVM, authHeader))
}

func runDeployRunRBDPrepare(req DeployRunRequest, runtime *deployRunRuntime) (deployRunStepOutcome, error) {
	cfg, err := loadClusterConfigSection()
	if err != nil {
		return deployRunStepOutcome{}, fmt.Errorf("failed to read cluster.json")
	}
	if normalizeDeployOSType(cfg.Type) != "ablestack-hci-filesystem" {
		return deployRunSkipped("rbd_prepare is required for ablestack-hci-filesystem only", nil), nil
	}
	if req.RBD == nil {
		return deployRunMissingInput(req, CubeModel.DeployRunStepRBDPrepare, "rbd request required")
	}

	rbdReq := *req.RBD
	if err := normalizeRBDManageRequest(&rbdReq, false); err != nil {
		return deployRunStepOutcome{}, err
	}
	if rbdReq.Action != "create" {
		return deployRunStepOutcome{}, fmt.Errorf("rbd_prepare supports create action only")
	}
	resp := runRBDManage(rbdReq, cfg)
	if resp.Code != http.StatusOK {
		return deployRunStepOutcome{Output: resp}, errors.New(firstNonEmpty(resp.Message, fmt.Sprint(resp.Val), "rbd prepare failed"))
	}
	if runtime != nil {
		runtime.rbdImages = normalizeStringSlice(rbdManageResponseImages(resp))
	}
	return deployRunSucceeded(firstNonEmpty(resp.Message, "rbd prepare success"), resp), nil
}

func rbdManageResponseImages(resp RBDManageResponse) []string {
	if values, ok := resp.Val.([]string); ok {
		return values
	}
	for _, result := range resp.Results {
		if len(result.Images) > 0 {
			return result.Images
		}
	}
	return nil
}

func runDeployRunStoragePrepare(req DeployRunRequest, runtime *deployRunRuntime) (deployRunStepOutcome, error) {
	cfg, err := loadClusterConfigSection()
	if err != nil {
		return deployRunStepOutcome{}, fmt.Errorf("failed to read cluster.json")
	}
	osType := normalizeDeployOSType(cfg.Type)
	if osType != "ablestack-vm" && osType != "ablestack-hci-filesystem" {
		return deployRunSkipped("storage_prepare is not required for "+strings.TrimSpace(cfg.Type), nil), nil
	}
	if req.GFS == nil {
		return deployRunMissingInput(req, CubeModel.DeployRunStepStoragePrepare, "gfs request required")
	}

	rbdImages := []string(nil)
	if runtime != nil {
		rbdImages = runtime.rbdImages
	}
	requests, err := buildDeployRunGFSRequests(*req.GFS, osType, rbdImages)
	if err != nil {
		return deployRunStepOutcome{}, err
	}

	output := map[string]any{}
	for _, gfsReq := range requests {
		if err := normalizeGFSManageRequest(&gfsReq); err != nil {
			return deployRunStepOutcome{Output: output}, fmt.Errorf("%s validation failed: %w", gfsReq.Action, err)
		}
		resp := runGFSManage(gfsReq, cfg)
		output[gfsReq.Action] = resp
		if resp.Code != http.StatusOK {
			return deployRunStepOutcome{Output: output}, fmt.Errorf("%s failed: %s", gfsReq.Action, firstNonEmpty(resp.Message, fmt.Sprint(resp.Val), "storage prepare failed"))
		}
	}
	return deployRunSucceeded("GFS storage prepare success", output), nil
}

func buildDeployRunGFSRequests(base GFSManageRequest, osType string, rbdImages []string) ([]GFSManageRequest, error) {
	base.Action = "init-pcs-cluster"
	if base.ClusterName == "" {
		base.ClusterName = gfsManageDefaultCluster
	}
	if base.ClusterUser == "" {
		base.ClusterUser = gfsManageDefaultPCSUser
	}
	if base.ClusterPassword == "" {
		base.ClusterPassword = gfsManageDefaultPCSPass
	}
	if base.VGName == "" {
		base.VGName = "vg_glue"
	}
	if base.LVName == "" {
		base.LVName = "lv_glue"
	}
	if base.GFSName == "" {
		base.GFSName = "glue-gfs"
	}
	if base.MountPoint == "" {
		base.MountPoint = "/mnt/glue-gfs"
	}
	base.VolumeGroups = normalizeGFSManageVolumeGroups(base.VolumeGroups, base.VGName, base.LVName)

	switch normalizeDeployOSType(osType) {
	case "ablestack-hci-filesystem":
		if len(rbdImages) == 0 {
			return nil, fmt.Errorf("rbd_prepare did not return mapped images")
		}
		base.Disks = make([]string, 0, len(rbdImages))
		for _, image := range rbdImages {
			image = strings.Trim(strings.TrimSpace(image), "/")
			if image != "" {
				base.Disks = append(base.Disks, "/dev/rbd/"+image)
			}
		}
	case "ablestack-vm":
		base.Disks = normalizeStringSlice(append(base.Disks, splitCommaValues(base.Disk)...))
	default:
		return nil, fmt.Errorf("GFS storage is not supported for %s", osType)
	}
	if len(base.Disks) == 0 {
		return nil, fmt.Errorf("gfs disks required")
	}
	if len(base.Stonith) == 0 {
		return nil, fmt.Errorf("gfs stonith devices required")
	}

	initReq := base
	stonithReq := GFSManageRequest{Action: "configure-stonith", Stonith: base.Stonith}
	createReq := base
	createReq.Action = "create-gfs"
	alertReq := GFSManageRequest{Action: "set-alert"}
	return []GFSManageRequest{initReq, stonithReq, createReq, alertReq}, nil
}

func runDeployRunLocalPrepare(req DeployRunRequest) (deployRunStepOutcome, error) {
	cfg, err := loadClusterConfigSection()
	if err != nil {
		return deployRunStepOutcome{}, fmt.Errorf("failed to read cluster.json")
	}
	if normalizeDeployOSType(cfg.Type) != "ablestack-standalone" {
		return deployRunSkipped("local_prepare is required for ablestack-standalone only", nil), nil
	}
	if req.Local == nil {
		return deployRunMissingInput(req, CubeModel.DeployRunStepLocalPrepare, "local request required")
	}

	localReq := *req.Local
	if err := normalizeLocalManageRequest(&localReq); err != nil {
		return deployRunStepOutcome{}, err
	}
	resp := runLocalManage(localReq)
	if resp.Code != http.StatusOK {
		return deployRunStepOutcome{Output: resp}, errors.New(firstNonEmpty(resp.Message, fmt.Sprint(resp.Val), "local prepare failed"))
	}
	return deployRunSucceeded(firstNonEmpty(resp.Message, "local prepare success"), resp), nil
}

func runDeployRunCCVMPrepare(req DeployRunRequest) (deployRunStepOutcome, error) {
	cfg, err := loadClusterConfigSection()
	if err != nil {
		return deployRunStepOutcome{}, fmt.Errorf("failed to read cluster.json")
	}
	hasPayload := req.CCVMCloudInit != nil || req.CCVMXML != nil || req.CCVMLifecycle != nil
	if !hasPayload {
		return deployRunMissingInput(req, CubeModel.DeployRunStepCCVMPrepare, "ccvm_cloudinit, ccvm_xml or ccvm_lifecycle required")
	}

	output := map[string]any{}
	cloudReq := CCVMCloudInitCreateRequest{}
	if req.CCVMCloudInit != nil {
		cloudReq = *req.CCVMCloudInit
	}
	cloudResp := runCCVMCloudInitForDeploy(cfg, cloudReq)
	output["cloudinit"] = cloudResp
	if cloudResp.Code != http.StatusOK {
		return deployRunStepOutcome{Output: output}, errors.New(firstNonEmpty(cloudResp.Message, "ccvm cloudinit failed"))
	}

	if req.CCVMXML != nil {
		xmlReq := *req.CCVMXML
		if err := normalizeCCVMXMLCreateRequest(&xmlReq, cfg); err != nil {
			return deployRunStepOutcome{Output: output}, err
		}
		xmlResp := runCreateCCVMXML(cfg, xmlReq)
		output["xml"] = xmlResp
		if xmlResp.Code != http.StatusOK {
			return deployRunStepOutcome{Output: output}, errors.New(firstNonEmpty(xmlResp.Message, fmt.Sprint(xmlResp.Val), "ccvm xml failed"))
		}
	}

	if req.CCVMLifecycle != nil || req.CCVMXML != nil {
		lifeReq := CCVMLifecycleRequest{Action: "setup"}
		if req.CCVMLifecycle != nil {
			lifeReq = *req.CCVMLifecycle
		}
		if err := normalizeCCVMLifecycleRequest(&lifeReq); err != nil {
			return deployRunStepOutcome{Output: output}, err
		}
		lifeResp := runCCVMLifecycle(lifeReq, cfg)
		output["lifecycle"] = lifeResp
		if lifeResp.Code != http.StatusOK {
			return deployRunStepOutcome{Output: output}, errors.New(firstNonEmpty(lifeResp.Message, lifeResp.Val, "ccvm lifecycle failed"))
		}
	}
	health, err := waitDeployRunAPIHealth(cfg.CCVM.IP)
	output["api_health"] = health
	if err != nil {
		return deployRunStepOutcome{Output: output}, fmt.Errorf("ccvm api health failed: %s", err.Error())
	}

	return deployRunSucceeded("ccvm prepare success", output), nil
}

func runDeployRunCCVMBootstrap(req DeployRunRequest, authHeader string, executed map[string]bool) (deployRunStepOutcome, error) {
	cfg, err := loadClusterConfigSection()
	if err != nil {
		return deployRunStepOutcome{}, fmt.Errorf("failed to read cluster.json")
	}
	if !executed[CubeModel.DeployRunStepCCVMPrepare] && !deployRunStepExplicit(req, CubeModel.DeployRunStepCCVMBootstrap) {
		return deployRunSkipped("ccvm_bootstrap waits for ccvm_prepare or explicit selection", nil), nil
	}
	licenseResp := runLicenseApply(LicenseApplyRequest{
		Action:         "register",
		LicenseContent: req.LicenseContent,
		Licenses:       req.Licenses,
		Filename:       req.LicenseFilename,
		Roles:          []string{licenseApplyRoleCCVM},
		WaitForReady:   true,
	}, cfg, authHeader)
	if licenseResp.Code != http.StatusOK {
		return deployRunStepOutcome{Output: map[string]any{"license_apply": licenseResp}}, errors.New(firstNonEmpty(licenseResp.Message, "ccvm license apply failed"))
	}
	return bootstrapResponseToDeployOutcome(runBootstrapRole(bootstrapRequestFromDeployRun(req), cfg, licenseApplyRoleCCVM, authHeader))
}

func runDeployRunMonitoringPrepare(req DeployRunRequest, executed map[string]bool) (deployRunStepOutcome, error) {
	if !executed[CubeModel.DeployRunStepCCVMBootstrap] && !deployRunStepExplicit(req, CubeModel.DeployRunStepMonitoringPrepare) {
		return deployRunSkipped("monitoring_prepare waits for ccvm_bootstrap or explicit selection", nil), nil
	}
	cfg, err := loadClusterConfigSection()
	if err != nil {
		return deployRunStepOutcome{}, fmt.Errorf("failed to read cluster.json")
	}
	monitoringReq := CCVMMonitoringConfigRequest{Action: "configure"}
	if req.Monitoring != nil {
		monitoringReq = *req.Monitoring
		if strings.TrimSpace(monitoringReq.Action) == "" {
			monitoringReq.Action = "configure"
		}
	}
	resp := runCCVMMonitoringConfigForDeploy(monitoringReq, cfg)
	if resp.Code != http.StatusOK {
		return deployRunStepOutcome{Output: resp}, errors.New(firstNonEmpty(resp.Message, "monitoring prepare failed"))
	}
	return deployRunSucceeded(firstNonEmpty(resp.Message, "monitoring prepare success"), resp), nil
}

func runCCVMCloudInitForDeploy(cfg *CubeModel.ClusterConfigSection, req CCVMCloudInitCreateRequest) GenCloudInitResponse {
	if err := normalizeCCVMCloudInitCreateRequest(&req); err != nil {
		return GenCloudInitResponse{Code: http.StatusBadRequest, Message: err.Error()}
	}
	genReq, err := buildCreateCCVMCloudInitRequest(cfg, req)
	if err != nil {
		return GenCloudInitResponse{Code: http.StatusBadRequest, Message: err.Error()}
	}
	targets, targetField := ccvmCloudInitCopyTargets(cfg)
	if len(targets) == 0 {
		return GenCloudInitResponse{Code: http.StatusBadRequest, Message: targetField + " required"}
	}
	resp := runGenCloudInit(genReq, cfg)
	if resp.Code != http.StatusOK {
		return resp
	}
	if err := copyCCVMCloudInitISOToTargets(targets); err != nil {
		resp.Code = http.StatusInternalServerError
		resp.Message = err.Error()
		return resp
	}
	resp.Message = ccvmCloudInitSuccessMessage
	return resp
}

func waitDeployRunAPIHealth(target string) (map[string]any, error) {
	target = strings.TrimSpace(target)
	result := map[string]any{
		"target": target,
	}
	if target == "" {
		err := fmt.Errorf("empty target")
		result["message"] = err.Error()
		return result, err
	}

	client := &http.Client{Timeout: deployRunHealthCheckTO}
	deadline := time.Now().Add(deployRunBootstrapReadyTO)
	var lastErr error
	attempts := 0
	for {
		attempts++
		if err := callHealthTarget(client, target); err == nil {
			result["code"] = http.StatusOK
			result["message"] = "ok"
			result["attempts"] = attempts
			return result, nil
		} else {
			lastErr = err
		}
		if time.Now().Add(deployRunHealthRetryDelay).After(deadline) {
			break
		}
		time.Sleep(deployRunHealthRetryDelay)
	}

	result["code"] = http.StatusInternalServerError
	result["message"] = lastErr.Error()
	result["attempts"] = attempts
	return result, fmt.Errorf("%s api health check failed after %s: %w", target, deployRunBootstrapReadyTO, lastErr)
}

func waitDeployRunAPIHealthWithPolicy(target string, maxAttempts int, interval time.Duration) (map[string]any, error) {
	target = strings.TrimSpace(target)
	result := map[string]any{"target": target}
	if target == "" {
		err := fmt.Errorf("empty target")
		result["message"] = err.Error()
		return result, err
	}
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if interval < 0 {
		interval = 0
	}

	client := &http.Client{Timeout: deployRunHealthCheckTO}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := callHealthTarget(client, target); err == nil {
			result["code"] = http.StatusOK
			result["message"] = "ok"
			result["attempts"] = attempt
			result["interval_seconds"] = int(interval.Seconds())
			return result, nil
		} else {
			lastErr = err
		}
		if attempt < maxAttempts {
			time.Sleep(interval)
		}
	}

	result["code"] = http.StatusInternalServerError
	result["message"] = lastErr.Error()
	result["attempts"] = maxAttempts
	result["interval_seconds"] = int(interval.Seconds())
	return result, fmt.Errorf("%s api health check failed after %d attempts at %s intervals: %w", target, maxAttempts, interval, lastErr)
}

func runDeployRunSystemProfile(req DeployRunRequest, executed map[string]bool) (deployRunStepOutcome, error) {
	if req.UpdateSystemProfile != nil && !*req.UpdateSystemProfile {
		return deployRunSkipped("system profile update disabled", nil), nil
	}
	cfg, err := loadClusterConfigSection()
	if err != nil {
		return deployRunStepOutcome{}, fmt.Errorf("failed to read cluster.json")
	}

	flags := make([]resetCloudCenterSystemFlag, 0)
	if executed[CubeModel.DeployRunStepLicenseApply] {
		flags = append(flags, resetCloudCenterSystemFlag{Depth1: "license", Depth2: "status", Value: "true"})
		if licenseType := currentLicenseTypeValue(); licenseType != "" {
			flags = append(flags, resetCloudCenterSystemFlag{Depth1: "license", Depth2: "type", Value: licenseType})
		}
	}
	if executed[CubeModel.DeployRunStepSCVMBootstrap] && isHCITarget(cfg.Type) {
		flags = append(flags, resetCloudCenterSystemFlag{Depth1: "bootstrap", Depth2: "scvm", Value: "true"})
	}
	if executed[CubeModel.DeployRunStepStoragePrepare] {
		switch normalizeDeployOSType(cfg.Type) {
		case "ablestack-vm", "ablestack-hci-filesystem":
			flags = append(flags, resetCloudCenterSystemFlag{Depth1: "bootstrap", Depth2: "gfs_configure", Value: "true"})
		}
	}
	if executed[CubeModel.DeployRunStepLocalPrepare] && normalizeDeployOSType(cfg.Type) == "ablestack-standalone" {
		flags = append(flags, resetCloudCenterSystemFlag{Depth1: "bootstrap", Depth2: "local_configure", Value: "true"})
	}
	if executed[CubeModel.DeployRunStepCCVMBootstrap] {
		flags = append(flags, resetCloudCenterSystemFlag{Depth1: "bootstrap", Depth2: "ccvm", Value: "true"})
	}
	if executed[CubeModel.DeployRunStepMonitoringPrepare] {
		flags = append(flags, resetCloudCenterSystemFlag{Depth1: "bootstrap", Depth2: "wall", Value: "true"})
	}
	flags = dedupeDeployRunSystemFlags(flags)
	if len(flags) == 0 {
		return deployRunSkipped("no completed step flags to update", nil), nil
	}

	results, err := resetCloudCenterApplySystemFlagsOnHosts(cfg, flags)
	output := map[string]any{
		"flags":   flags,
		"results": results,
	}
	if err != nil {
		return deployRunStepOutcome{Output: output}, err
	}
	return deployRunSucceeded("system profile update success", output), nil
}

func dedupeDeployRunSystemFlags(flags []resetCloudCenterSystemFlag) []resetCloudCenterSystemFlag {
	out := make([]resetCloudCenterSystemFlag, 0, len(flags))
	seen := map[string]struct{}{}
	for _, flag := range flags {
		key := strings.Join([]string{flag.Depth1, flag.Depth2, flag.Value}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, flag)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.Join([]string{out[i].Depth1, out[i].Depth2}, ".") < strings.Join([]string{out[j].Depth1, out[j].Depth2}, ".")
	})
	return out
}

func deployRunPostJSON(target string, path string, payload any, timeout time.Duration, out any) (int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	httpReq, err := http.NewRequest(http.MethodPost, buildTargetURL(target)+path, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	attachInternalToken(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			if resp.StatusCode >= 300 {
				return resp.StatusCode, fmt.Errorf("%s", firstNonEmpty(strings.TrimSpace(string(raw)), resp.Status))
			}
			return resp.StatusCode, err
		}
	}
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("%s", firstNonEmpty(strings.TrimSpace(string(raw)), resp.Status))
	}
	return resp.StatusCode, nil
}
