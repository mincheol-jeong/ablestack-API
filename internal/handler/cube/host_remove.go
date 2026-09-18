package cube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"ablecloud.io/ablestack-api/internal/service/moldservice"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid"
)

type HostRemoveRequest = CubeModel.HostRemoveRequest

const hostRemoveJobLimit = 50

var hostRemoveJobs = newHostRemoveJobStore()

type hostRemoveJobStore struct {
	mu    sync.RWMutex
	jobs  map[string]CubeModel.HostRemoveJob
	order []string
}

func newHostRemoveJobStore() *hostRemoveJobStore {
	return &hostRemoveJobStore{jobs: map[string]CubeModel.HostRemoveJob{}}
}

// StartHostRemove godoc
//
//	@Summary		Start product-aware host removal job
//	@Description	HCI는 Mold Host, Ceph/SCVM을 제거하고 VM은 Mold Host, PCS/GFS 노드를 제거한 뒤 전체 노드 구성과 Wall을 갱신합니다.
//	@Tags			Cube-Cluster
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CubeModel.HostRemoveRequest	true	"host removal request"
//	@Success		202		{object}	CubeModel.HostRemoveStartResponse
//	@Failure		400		{object}	HTTP400BadRequest
//	@Router			/cube/hosts/remove [post]
func StartHostRemove(c *gin.Context) {
	var req HostRemoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, map[string]any{"code": http.StatusBadRequest, "message": "invalid request"})
		return
	}
	req.Hostname = strings.TrimSpace(req.Hostname)
	if req.Hostname == "" {
		c.JSON(http.StatusBadRequest, map[string]any{"code": http.StatusBadRequest, "message": "hostname required"})
		return
	}
	cfg, err := loadClusterConfigSection()
	if err != nil {
		c.JSON(http.StatusInternalServerError, map[string]any{"code": http.StatusInternalServerError, "message": "failed to read cluster.json"})
		return
	}
	host, ok := findClusterHostByName(cfg, req.Hostname)
	if !ok {
		c.JSON(http.StatusBadRequest, map[string]any{"code": http.StatusBadRequest, "message": "hostname not found"})
		return
	}
	if err := validateHostRemoval(cfg, host); err != nil {
		c.JSON(http.StatusBadRequest, map[string]any{"code": http.StatusBadRequest, "message": err.Error()})
		return
	}

	job := hostRemoveJobs.create(req, cfg.Type, hostRemoveStepNames(cfg.Type))
	go runHostRemoveJob(job.JobID, req, *cfg, host)
	c.JSON(http.StatusAccepted, CubeModel.HostRemoveStartResponse{
		Code: http.StatusAccepted, JobID: job.JobID, Status: job.Status,
		Message: "host removal job started", Steps: job.Steps,
	})
}

// GetHostRemoveJob godoc
//
//	@Summary	Get host removal job
//	@Tags		Cube-Cluster
//	@Produce	json
//	@Param		job_id	path		string	true	"job id"
//	@Success	200		{object}	CubeModel.HostRemoveJobResponse
//	@Router		/cube/hosts/remove/jobs/{job_id} [get]
func GetHostRemoveJob(c *gin.Context) {
	job, ok := hostRemoveJobs.get(strings.TrimSpace(c.Param("job_id")))
	if !ok {
		c.JSON(http.StatusNotFound, map[string]any{"code": http.StatusNotFound, "message": "job not found"})
		return
	}
	c.JSON(http.StatusOK, CubeModel.HostRemoveJobResponse{Code: http.StatusOK, Job: job, Message: "ok"})
}

// ListHostRemoveJobs godoc
//
//	@Summary	List host removal jobs
//	@Tags		Cube-Cluster
//	@Produce	json
//	@Success	200	{object}	CubeModel.HostRemoveJobListResponse
//	@Router		/cube/hosts/remove/jobs [get]
func ListHostRemoveJobs(c *gin.Context) {
	c.JSON(http.StatusOK, CubeModel.HostRemoveJobListResponse{Code: http.StatusOK, Jobs: hostRemoveJobs.list()})
}

func findClusterHostByName(cfg *CubeModel.ClusterConfigSection, hostname string) (CubeModel.ClusterHost, bool) {
	if cfg != nil {
		for _, host := range cfg.Hosts {
			if strings.EqualFold(strings.TrimSpace(host.Hostname), hostname) {
				return host, true
			}
		}
	}
	return CubeModel.ClusterHost{}, false
}

func validateHostRemoval(cfg *CubeModel.ClusterConfigSection, host CubeModel.ClusterHost) error {
	minimum := 1
	if isHCITarget(cfg.Type) {
		minimum = 3
	}
	if len(cfg.Hosts) <= minimum {
		return fmt.Errorf("%s requires at least %d remaining hosts", cfg.Type, minimum)
	}
	if isLocalTarget(host.Ablecube) {
		return fmt.Errorf("cannot remove the API controller host itself; run the removal from another remaining host")
	}
	return nil
}

func hostRemoveStepNames(clusterType string) []string {
	steps := []string{"precheck", "remove_mold_host"}
	if normalizeDeployOSType(clusterType) == "ablestack-vm" || normalizeDeployOSType(clusterType) == "ablestack-hci-filesystem" {
		steps = append(steps, "remove_pcs_node")
	}
	if isHCITarget(clusterType) {
		steps = append(steps, "remove_ceph_host", "delete_scvm")
	}
	return append(steps, "update_cluster_files", "update_monitoring", "final_verify")
}

func runHostRemoveJob(jobID string, req HostRemoveRequest, cfg CubeModel.ClusterConfigSection, host CubeModel.ClusterHost) {
	hostRemoveJobs.start(jobID)
	for _, step := range hostRemoveStepNames(cfg.Type) {
		hostRemoveJobs.stepRunning(jobID, step)
		output, err := executeHostRemoveStep(req, &cfg, host, step)
		if err != nil {
			hostRemoveJobs.stepFinished(jobID, step, output, err)
			hostRemoveJobs.finish(jobID, false, err.Error())
			return
		}
		hostRemoveJobs.stepFinished(jobID, step, output, nil)
	}
	hostRemoveJobs.finish(jobID, true, "host removal succeeded")
}

func executeHostRemoveStep(req HostRemoveRequest, cfg *CubeModel.ClusterConfigSection, host CubeModel.ClusterHost, step string) (any, error) {
	switch step {
	case "precheck":
		health, err := waitDeployRunAPIHealthWithPolicy(host.Ablecube, 5, 20*time.Second)
		return health, err
	case "remove_mold_host":
		auth := moldservice.AuthConfig{Username: req.Username, Password: req.Password, Domain: req.Domain}
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Hour+10*time.Minute)
		defer cancel()
		return moldservice.RemoveHost(ctx, auth, host.Hostname, host.Ablecube, req.Force)
	case "remove_ceph_host":
		master, ok := firstRemainingClusterHost(cfg.Hosts, host.Hostname)
		if !ok {
			return nil, fmt.Errorf("remaining Ceph management host not found")
		}
		target := bootstrapScriptTarget{
			Role: licenseApplyRoleSCVM, Action: "remove_host", Hostname: licenseApplySCVMHostname(&master),
			Target: master.Ablecube, Domain: scvmDomainName,
			Args: []string{"remove-host", licenseApplySCVMHostname(&host), fmt.Sprintf("%t", req.Force)},
		}
		result := executeBootstrapScriptTarget(target)
		if result.Code != http.StatusOK {
			return result, fmt.Errorf("Ceph host removal failed: %s", result.Message)
		}
		return result, nil
	case "delete_scvm":
		resp, err := callSCVMUpdateRemote(host.Ablecube, SCVMUpdateRequest{Action: "delete"})
		if err != nil {
			return resp, err
		}
		if resp.Code != http.StatusOK {
			return resp, fmt.Errorf("SCVM delete failed: %s", resp.Message)
		}
		return resp, nil
	case "remove_pcs_node":
		return removeHostPCSNode(cfg, host, req.Force)
	case "update_cluster_files":
		resp := runClusterConfigApplyForDeploy(ClusterConfigApplyRequest{
			Action: "remove", Type: cfg.Type, RemoveHostname: host.Hostname,
			ExcludeHostname: host.Hostname, Hosts: cfg.Hosts, CCVM: &cfg.CCVM, MngtNic: &cfg.MngtNic,
		})
		if resp.Code != http.StatusOK {
			return resp, fmt.Errorf("cluster file update failed: %s", resp.Message)
		}
		return resp, nil
	case "update_monitoring":
		updated, err := loadClusterConfigSection()
		if err != nil {
			return nil, err
		}
		monitorReq := buildCCVMMonitoringConfigRequest(updated)
		monitorReq.Action = "update"
		var resp CCVMMonitoringConfigResponse
		if isLocalTarget(updated.CCVM.IP) {
			resp = runCCVMMonitoringConfigLocal(monitorReq)
		} else {
			resp, err = callCCVMMonitoringConfigRemote(updated.CCVM.IP, monitorReq)
		}
		if err != nil {
			return resp, err
		}
		if resp.Code != http.StatusOK {
			return resp, fmt.Errorf("monitoring update failed: %s", resp.Message)
		}
		return resp, nil
	case "final_verify":
		updated, err := loadClusterConfigSection()
		if err != nil {
			return nil, err
		}
		if _, exists := findClusterHostByName(updated, host.Hostname); exists {
			return nil, fmt.Errorf("removed host still exists in cluster.json")
		}
		return map[string]any{"hostname": host.Hostname, "removed": true, "remaining_hosts": len(updated.Hosts)}, nil
	default:
		return nil, fmt.Errorf("unsupported host removal step: %s", step)
	}
}

func firstRemainingClusterHost(hosts []CubeModel.ClusterHost, removedHostname string) (CubeModel.ClusterHost, bool) {
	for _, candidate := range hosts {
		if !strings.EqualFold(strings.TrimSpace(candidate.Hostname), strings.TrimSpace(removedHostname)) {
			return candidate, true
		}
	}
	return CubeModel.ClusterHost{}, false
}

func removeHostPCSNode(cfg *CubeModel.ClusterConfigSection, host CubeModel.ClusterHost, force bool) (map[string]any, error) {
	node := strings.TrimSpace(host.Ablecube)
	if index := strings.TrimSpace(host.Index); index != "" {
		if n := CubeModel.NormalizePCSClusterList(cfg.PCSCluster.HostnameList()); len(n) > 0 {
			for i, candidate := range cfg.Hosts {
				if candidate.Hostname == host.Hostname && i < len(n) {
					node = n[i]
				}
			}
		}
	}
	commands := [][]string{
		{"stonith", "delete", "fence-" + host.Hostname},
		{"cluster", "node", "remove", node},
	}
	if force {
		commands[1] = append(commands[1], "--force")
	}
	results := make([]map[string]any, 0, len(commands)+1)
	for _, args := range commands {
		output, err := runGFSManageCommand(gfsManageCommandTimeout, "pcs", args...)
		results = append(results, map[string]any{"command": "pcs " + strings.Join(args, " "), "output": output})
		if err != nil && !strings.Contains(strings.ToLower(output), "does not exist") && !strings.Contains(strings.ToLower(output), "unable to find") {
			return map[string]any{"results": results}, err
		}
	}
	status, err := runGFSManageCommand(gfsManageCommandTimeout, "pcs", "status")
	results = append(results, map[string]any{"command": "pcs status", "output": status})
	return map[string]any{"node": node, "results": results}, err
}

func (s *hostRemoveJobStore) create(req HostRemoveRequest, clusterType string, steps []string) CubeModel.HostRemoveJob {
	id, _ := uuid.NewV4()
	job := CubeModel.HostRemoveJob{JobID: id.String(), Hostname: req.Hostname, ClusterType: clusterType, Status: CubeModel.HostRemoveStatusQueued, Message: "queued", CreatedAt: time.Now()}
	for _, name := range steps {
		job.Steps = append(job.Steps, CubeModel.HostRemoveStep{Name: name, Status: "pending"})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.JobID] = job
	s.order = append(s.order, job.JobID)
	if len(s.order) > hostRemoveJobLimit {
		delete(s.jobs, s.order[0])
		s.order = s.order[1:]
	}
	return cloneHostRemoveJob(job)
}

func (s *hostRemoveJobStore) get(id string) (CubeModel.HostRemoveJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	return cloneHostRemoveJob(job), ok
}
func (s *hostRemoveJobStore) list() []CubeModel.HostRemoveJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]CubeModel.HostRemoveJob, 0, len(s.order))
	for i := len(s.order) - 1; i >= 0; i-- {
		out = append(out, cloneHostRemoveJob(s.jobs[s.order[i]]))
	}
	return out
}
func (s *hostRemoveJobStore) update(id string, fn func(*CubeModel.HostRemoveJob)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[id]
	fn(&job)
	s.jobs[id] = job
}
func (s *hostRemoveJobStore) start(id string) {
	s.update(id, func(j *CubeModel.HostRemoveJob) {
		j.Status = CubeModel.HostRemoveStatusRunning
		j.StartedAt = time.Now()
		j.Message = "running"
	})
}
func (s *hostRemoveJobStore) stepRunning(id, name string) {
	s.update(id, func(j *CubeModel.HostRemoveJob) {
		j.CurrentStep = name
		for i := range j.Steps {
			if j.Steps[i].Name == name {
				j.Steps[i].Status = "running"
				j.Steps[i].StartedAt = time.Now()
			}
		}
	})
}
func (s *hostRemoveJobStore) stepFinished(id, name string, output any, err error) {
	s.update(id, func(j *CubeModel.HostRemoveJob) {
		for i := range j.Steps {
			if j.Steps[i].Name == name {
				now := time.Now()
				j.Steps[i].FinishedAt = now
				j.Steps[i].DurationMS = now.Sub(j.Steps[i].StartedAt).Milliseconds()
				j.Steps[i].Output = output
				if err != nil {
					j.Steps[i].Status = "failed"
					j.Steps[i].Message = err.Error()
				} else {
					j.Steps[i].Status = "succeeded"
					j.Steps[i].Message = "ok"
				}
			}
		}
	})
}
func (s *hostRemoveJobStore) finish(id string, success bool, message string) {
	s.update(id, func(j *CubeModel.HostRemoveJob) {
		j.CurrentStep = ""
		j.FinishedAt = time.Now()
		j.Message = message
		if success {
			j.Status = CubeModel.HostRemoveStatusSucceeded
		} else {
			j.Status = CubeModel.HostRemoveStatusFailed
		}
	})
}
func cloneHostRemoveJob(job CubeModel.HostRemoveJob) CubeModel.HostRemoveJob {
	raw, _ := json.Marshal(job)
	var out CubeModel.HostRemoveJob
	_ = json.Unmarshal(raw, &out)
	return out
}
