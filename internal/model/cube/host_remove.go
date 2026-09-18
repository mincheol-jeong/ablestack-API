package cube

import "time"

const (
	HostRemoveStatusQueued    = "queued"
	HostRemoveStatusRunning   = "running"
	HostRemoveStatusSucceeded = "succeeded"
	HostRemoveStatusFailed    = "failed"
)

// HostRemoveRequest는 기존 클러스터에서 호스트 하나를 제거하는 요청이다.
// @name HostRemoveRequest
type HostRemoveRequest struct {
	Hostname string `json:"hostname" example:"ablecube31-4"`
	// CloudStack 관리자 계정. 비어 있으면 admin/password, domain=/ 기본값을 사용한다.
	Username string `json:"username,omitempty" example:"admin"`
	Password string `json:"password,omitempty" example:"password"`
	Domain   string `json:"domain,omitempty" example:"/"`
	Force    bool   `json:"force,omitempty" example:"false"`
}

// HostRemoveStep은 호스트 제거 Job의 개별 단계 결과이다.
// @name HostRemoveStep
type HostRemoveStep struct {
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Message    string    `json:"message,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	DurationMS int64     `json:"duration_ms,omitempty"`
	Output     any       `json:"output,omitempty"`
}

// HostRemoveJob은 제품별 호스트 제거 진행 상태이다.
// @name HostRemoveJob
type HostRemoveJob struct {
	JobID       string           `json:"job_id"`
	Hostname    string           `json:"hostname"`
	ClusterType string           `json:"cluster_type"`
	Status      string           `json:"status"`
	CurrentStep string           `json:"current_step,omitempty"`
	Message     string           `json:"message,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	StartedAt   time.Time        `json:"started_at,omitempty"`
	FinishedAt  time.Time        `json:"finished_at,omitempty"`
	Steps       []HostRemoveStep `json:"steps"`
}

// HostRemoveStartResponse는 호스트 제거 Job 시작 응답이다.
// @name HostRemoveStartResponse
type HostRemoveStartResponse struct {
	Code    int              `json:"code"`
	JobID   string           `json:"job_id"`
	Status  string           `json:"status"`
	Message string           `json:"message,omitempty"`
	Steps   []HostRemoveStep `json:"steps"`
}

// HostRemoveJobResponse는 호스트 제거 Job 조회 응답이다.
// @name HostRemoveJobResponse
type HostRemoveJobResponse struct {
	Code    int           `json:"code"`
	Job     HostRemoveJob `json:"job"`
	Message string        `json:"message,omitempty"`
}

// HostRemoveJobListResponse는 최근 호스트 제거 Job 목록이다.
// @name HostRemoveJobListResponse
type HostRemoveJobListResponse struct {
	Code int             `json:"code"`
	Jobs []HostRemoveJob `json:"jobs"`
}
