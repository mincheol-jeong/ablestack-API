package mold

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"ablecloud.io/ablestack-api/internal/infra/logging"
	MoldModel "ablecloud.io/ablestack-api/internal/model/mold"
	"ablecloud.io/ablestack-api/internal/service/moldservice"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid"
)

const (
	bootstrapJobLimit       = 50
	bootstrapStartedMessage = "mold bootstrap job started"
	bootstrapSuccessMessage = "mold bootstrap job succeeded"
	bootstrapFailedMessage  = "mold bootstrap job failed"
)

var bootstrapJobs = newBootstrapJobStore()

type bootstrapJobStore struct {
	mu    sync.RWMutex
	jobs  map[string]MoldModel.BootstrapJob
	order []string
}

func newBootstrapJobStore() *bootstrapJobStore {
	return &bootstrapJobStore{
		jobs: map[string]MoldModel.BootstrapJob{},
	}
}

// StartBootstrap godoc
//
//	@Summary		Mold bootstrap job 시작
//	@Description	Mold 인프라 자동화를 비동기 job으로 시작합니다. hosts, primary/secondary storage, VLAN은 cluster.json에서 자동 계산하고 각 단계 결과는 /mold/jobs/{job_id}에서 조회합니다.
//	@Tags			Mold
//	@Accept			json
//	@Produce		json
//	@Param			body	body		MoldModel.BootstrapRequest	true	"Mold bootstrap request"
//	@Success		202	{object}	MoldModel.BootstrapStartResponse
//	@Failure		400	{object}	MoldModel.Response
//	@Failure		403	{object}	MoldModel.Response
//	@Router			/mold/bootstrap [post]
func StartBootstrap(context *gin.Context) {
	var req MoldModel.BootstrapRequest
	if err := context.ShouldBindJSON(&req); err != nil {
		context.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}
	var err error
	req, err = moldservice.PrepareBootstrapRequest(req)
	if err != nil {
		serviceError(context, err)
		return
	}

	plan := moldservice.BuildBootstrapPlan(req)
	if !plan.Ready {
		context.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "missing required bootstrap inputs",
			Val:     plan,
		})
		return
	}

	job := bootstrapJobs.create(plan)
	go runBootstrapJob(job.JobID, req)

	context.JSON(http.StatusAccepted, MoldModel.BootstrapStartResponse{
		Code:    http.StatusAccepted,
		JobID:   job.JobID,
		Status:  job.Status,
		Message: bootstrapStartedMessage,
		Steps:   job.Steps,
	})
}

// GetBootstrapJob godoc
//
//	@Summary		Mold bootstrap job 조회
//	@Description	Mold bootstrap job의 현재 step, 완료 step, 실패 에러, 최종 리소스 결과를 반환합니다.
//	@Tags			Mold
//	@Produce		json
//	@Param			job_id	path		string	true	"job id"
//	@Success		200		{object}	MoldModel.BootstrapJobResponse
//	@Failure		404		{object}	MoldModel.Response
//	@Router			/mold/jobs/{job_id} [get]
func GetBootstrapJob(context *gin.Context) {
	jobID := strings.TrimSpace(context.Param("job_id"))
	job, ok := bootstrapJobs.get(jobID)
	if !ok {
		context.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "job not found"})
		return
	}
	context.JSON(http.StatusOK, MoldModel.BootstrapJobResponse{Code: http.StatusOK, Job: job, Message: "ok"})
}

// ListBootstrapJobs godoc
//
//	@Summary		Mold bootstrap job 목록
//	@Description	최근 Mold bootstrap job 목록을 반환합니다.
//	@Tags			Mold
//	@Produce		json
//	@Success		200	{object}	MoldModel.BootstrapJobListResponse
//	@Router			/mold/jobs [get]
func ListBootstrapJobs(context *gin.Context) {
	context.JSON(http.StatusOK, MoldModel.BootstrapJobListResponse{
		Code:    http.StatusOK,
		Jobs:    bootstrapJobs.list(),
		Message: "ok",
	})
}

func (s *bootstrapJobStore) create(plan MoldModel.BootstrapPlanValue) MoldModel.BootstrapJob {
	now := time.Now()
	jobID := newBootstrapJobID()
	steps := make([]MoldModel.BootstrapJobStep, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		steps = append(steps, MoldModel.BootstrapJobStep{
			Name:          step.Name,
			Command:       step.Command,
			VerifyCommand: step.VerifyCommand,
			ResultKey:     step.ResultKey,
			Status:        MoldModel.BootstrapStepStatusPending,
		})
	}
	job := MoldModel.BootstrapJob{
		JobID:     jobID,
		Status:    MoldModel.BootstrapJobStatusQueued,
		Message:   "queued",
		CreatedAt: now,
		Steps:     steps,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[jobID] = job
	s.order = append(s.order, jobID)
	s.pruneLocked()
	return cloneBootstrapJob(job)
}

func (s *bootstrapJobStore) get(jobID string) (MoldModel.BootstrapJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return MoldModel.BootstrapJob{}, false
	}
	return cloneBootstrapJob(job), true
}

func (s *bootstrapJobStore) list() []MoldModel.BootstrapJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]MoldModel.BootstrapJob, 0, len(s.order))
	for i := len(s.order) - 1; i >= 0; i-- {
		if job, ok := s.jobs[s.order[i]]; ok {
			jobs = append(jobs, cloneBootstrapJob(job))
		}
	}
	return jobs
}

func (s *bootstrapJobStore) update(jobID string, update func(*MoldModel.BootstrapJob)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return
	}
	update(&job)
	s.jobs[jobID] = job
}

func (s *bootstrapJobStore) pruneLocked() {
	for len(s.order) > bootstrapJobLimit {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.jobs, oldest)
	}
}

func cloneBootstrapJob(job MoldModel.BootstrapJob) MoldModel.BootstrapJob {
	if len(job.Steps) > 0 {
		steps := make([]MoldModel.BootstrapJobStep, len(job.Steps))
		copy(steps, job.Steps)
		job.Steps = steps
	}
	if len(job.Resources.Hosts) > 0 {
		hosts := make([]MoldModel.ResourceResult, len(job.Resources.Hosts))
		copy(hosts, job.Resources.Hosts)
		job.Resources.Hosts = hosts
	}
	if len(job.Resources.TrafficTypes) > 0 {
		trafficTypes := make([]MoldModel.ResourceResult, len(job.Resources.TrafficTypes))
		copy(trafficTypes, job.Resources.TrafficTypes)
		job.Resources.TrafficTypes = trafficTypes
	}
	if len(job.Resources.NetworkProviders) > 0 {
		providers := make([]MoldModel.ResourceResult, len(job.Resources.NetworkProviders))
		copy(providers, job.Resources.NetworkProviders)
		job.Resources.NetworkProviders = providers
	}
	if len(job.Resources.SystemVMs) > 0 {
		systemVMs := make([]MoldModel.ResourceResult, len(job.Resources.SystemVMs))
		copy(systemVMs, job.Resources.SystemVMs)
		job.Resources.SystemVMs = systemVMs
	}
	return job
}

func newBootstrapJobID() string {
	id, err := uuid.NewV4()
	if err == nil {
		return id.String()
	}
	return fmt.Sprintf("mold-bootstrap-%d", time.Now().UnixNano())
}

func runBootstrapJob(jobID string, req MoldModel.BootstrapRequest) {
	jobStartedAt := time.Now()
	bootstrapJobs.update(jobID, func(job *MoldModel.BootstrapJob) {
		now := jobStartedAt
		job.Status = MoldModel.BootstrapJobStatusRunning
		job.Message = bootstrapStartedMessage
		job.StartedAt = &now
	})
	logging.AppendJobLog("mold.bootstrap", "started", "info", bootstrapStartedMessage, map[string]any{
		"job_id": jobID,
	})

	job, ok := bootstrapJobs.get(jobID)
	if !ok {
		return
	}
	runtime := moldservice.NewBootstrapRuntime(req)
	for _, step := range job.Steps {
		stepStartedAt := time.Now()
		bootstrapJobs.update(jobID, func(job *MoldModel.BootstrapJob) {
			job.CurrentStep = step.Name
			markBootstrapStepRunning(job, step.Name)
		})
		logging.AppendJobLog("mold.bootstrap", "step_started", "info", "mold bootstrap step started", bootstrapStepLogFields(jobID, step, nil, 0))

		result := runtime.RunStep(context.Background(), step)
		duration := time.Since(stepStartedAt)
		if result.Error != nil {
			bootstrapJobs.update(jobID, func(job *MoldModel.BootstrapJob) {
				markBootstrapStepFinished(job, step.Name, result)
				job.Status = MoldModel.BootstrapJobStatusFailed
				job.Message = bootstrapFailedMessage + ": " + result.Error.Message
				job.CurrentStep = ""
				job.Error = result.Error
				now := time.Now()
				job.FinishedAt = &now
			})
			logging.AppendJobLog("mold.bootstrap", "step_failed", "error", result.Error.Message, bootstrapStepLogFields(jobID, step, result.Error, duration))
			logging.AppendJobLog("mold.bootstrap", "failed", "error", bootstrapFailedMessage, map[string]any{
				"job_id":      jobID,
				"failed_step": step.Name,
				"duration_ms": time.Since(jobStartedAt).Milliseconds(),
			})
			return
		}

		bootstrapJobs.update(jobID, func(job *MoldModel.BootstrapJob) {
			markBootstrapStepFinished(job, step.Name, result)
			mergeBootstrapResource(job, result)
		})
		logging.AppendJobLog("mold.bootstrap", "step_succeeded", "success", resultMessage(result), bootstrapStepLogFields(jobID, step, nil, duration))
	}

	bootstrapJobs.update(jobID, func(job *MoldModel.BootstrapJob) {
		job.Status = MoldModel.BootstrapJobStatusSucceeded
		job.Message = bootstrapSuccessMessage
		job.CurrentStep = ""
		now := time.Now()
		job.FinishedAt = &now
	})
	logging.AppendJobLog("mold.bootstrap", "succeeded", "success", bootstrapSuccessMessage, map[string]any{
		"job_id":      jobID,
		"duration_ms": time.Since(jobStartedAt).Milliseconds(),
	})
}

func bootstrapStepLogFields(jobID string, step MoldModel.BootstrapJobStep, stepErr *MoldModel.BootstrapError, duration time.Duration) map[string]any {
	fields := map[string]any{
		"job_id":         jobID,
		"step":           step.Name,
		"command":        step.Command,
		"verify_command": step.VerifyCommand,
	}
	if duration > 0 {
		fields["duration_ms"] = duration.Milliseconds()
	}
	if stepErr != nil {
		fields["endpoint"] = stepErr.Endpoint
		fields["http_status"] = stepErr.HTTPStatus
		fields["error_code"] = stepErr.ErrorCode
		fields["error_text"] = stepErr.ErrorText
		fields["sessionkey_attached"] = stepErr.SessionKeyAttached
	}
	return fields
}

func markBootstrapStepRunning(job *MoldModel.BootstrapJob, stepName string) {
	now := time.Now()
	for i := range job.Steps {
		if job.Steps[i].Name != stepName {
			continue
		}
		job.Steps[i].Status = MoldModel.BootstrapStepStatusRunning
		job.Steps[i].Message = "running"
		job.Steps[i].StartedAt = &now
		return
	}
}

func markBootstrapStepFinished(job *MoldModel.BootstrapJob, stepName string, result MoldModel.BootstrapStepResult) {
	now := time.Now()
	for i := range job.Steps {
		if job.Steps[i].Name != stepName {
			continue
		}
		if job.Steps[i].StartedAt == nil {
			job.Steps[i].StartedAt = &now
		}
		job.Steps[i].Status = result.Status
		job.Steps[i].Message = resultMessage(result)
		job.Steps[i].FinishedAt = &now
		job.Steps[i].DurationMS = now.Sub(*job.Steps[i].StartedAt).Milliseconds()
		job.Steps[i].Result = &result
		job.Steps[i].Error = result.Error
		return
	}
}

func resultMessage(result MoldModel.BootstrapStepResult) string {
	if result.Error != nil {
		return result.Error.Message
	}
	if result.Verified {
		return "verified"
	}
	return "ok"
}

func mergeBootstrapResource(job *MoldModel.BootstrapJob, result MoldModel.BootstrapStepResult) {
	if len(result.Resources) > 0 {
		switch result.ResultKey {
		case "traffic_types":
			job.Resources.TrafficTypes = append(job.Resources.TrafficTypes, result.Resources...)
		case "network_providers":
			job.Resources.NetworkProviders = append(job.Resources.NetworkProviders, result.Resources...)
		case "hosts":
			job.Resources.Hosts = append(job.Resources.Hosts, result.Resources...)
		case "system_vms":
			job.Resources.SystemVMs = append(job.Resources.SystemVMs, result.Resources...)
		}
	}
	if result.Resource == nil {
		return
	}
	switch result.ResultKey {
	case "zone":
		job.Resources.Zone = result.Resource
	case "physical_network":
		job.Resources.PhysicalNetwork = result.Resource
	case "public_ip_range":
		job.Resources.PublicIPRange = result.Resource
	case "pod":
		job.Resources.Pod = result.Resource
	case "cluster":
		job.Resources.Cluster = result.Resource
	case "hosts":
		job.Resources.Hosts = append(job.Resources.Hosts, *result.Resource)
	case "primary_storage":
		job.Resources.PrimaryStorage = result.Resource
	case "secondary_storage":
		job.Resources.SecondaryStorage = result.Resource
	case "secondary_staging_store":
		job.Resources.SecondaryStagingStore = result.Resource
	case "system_vms":
		job.Resources.SystemVMs = append(job.Resources.SystemVMs, *result.Resource)
	}
}
