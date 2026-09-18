package cube

// SecurityPatchRequest는 security_patch.sh 실행 요청 본문이다.
// @name SecurityPatchRequest
type SecurityPatchRequest struct {
	// cluster.json path. 비어 있으면 기본 cluster.json을 사용한다.
	JSONPath string `json:"json,omitempty" example:"/etc/ablestack/properties/cluster.json"`
	// 변경할 SSH 포트. 지정하지 않으면 -P 없이 실행한다.
	NewPort *int `json:"new_port,omitempty" example:"10022"`
	// target kinds: ccvm/ablecube/scvm/all
	Targets []string `json:"targets,omitempty" example:"all"`
	// SSH user
	SSHUser string `json:"ssh_user,omitempty" example:"root"`
	// SSH connect port
	SSHPort int `json:"ssh_port,omitempty" example:"22"`
	// true면 실제 실행하지 않고 명령만 반환한다.
	DryRun bool `json:"dry_run,omitempty" example:"false"`
	// createReturn 호환 retname
	RetName string `json:"retname,omitempty" example:"Security Update"`
	// 추가 호스트용 로컬 실행 모드
	AddHost bool `json:"add_host,omitempty" example:"false"`
	// security_patch.sh에 --port-change를 전달한다.
	PortChange bool `json:"port_change,omitempty" example:"false"`
	// security_patch.status=true 업데이트만 수행한다.
	UpdateJSONFile bool `json:"update_json_file,omitempty" example:"false"`
	// 내부 fan-out 요청에서 현재 노드만 실행한다. update_json_file에도 동일하게 적용된다.
	Local bool `json:"local,omitempty" example:"false"`
	// security_patch.sh --ceph-ssh-change를 로컬에서 한 번만 실행한다.
	CephSSHChange bool `json:"ceph_ssh_change,omitempty" example:"false"`
}

// SecurityPatchTargetResult는 대상별 security patch 실행 결과이다.
// @name SecurityPatchTargetResult
type SecurityPatchTargetResult struct {
	IP             string `json:"ip"`
	TargetKind     string `json:"targetKind,omitempty"`
	ScriptPath     string `json:"scriptPath,omitempty"`
	ConnectPort    *int   `json:"connectPort,omitempty"`
	ChangeTo       *int   `json:"changeTo,omitempty"`
	OK             bool   `json:"ok"`
	RC             int    `json:"rc"`
	Stdout         string `json:"stdout,omitempty"`
	Stderr         string `json:"stderr,omitempty"`
	DryRunCmd      string `json:"dryRunCmd,omitempty"`
	RetriesPlanned int    `json:"retriesPlanned,omitempty"`
	RetryDelaySec  int    `json:"retryDelaySec,omitempty"`
	Attempts       int    `json:"attempts,omitempty"`
	SuccessAttempt *int   `json:"successAttempt,omitempty"`
	SuccessPattern string `json:"successPattern"`
	ClusterType    string `json:"clusterType"`
	IsLocal        bool   `json:"isLocal,omitempty"`
	Transport      string `json:"transport,omitempty" example:"api"`
	APIURL         string `json:"apiUrl,omitempty" example:"http://10.10.31.1:18090/api/v1/cube/security/patch"`
	HTTPStatus     int    `json:"httpStatus,omitempty" example:"200"`
}

// SecurityPatchSummary는 security patch 전체 실행 요약이다.
// @name SecurityPatchSummary
type SecurityPatchSummary struct {
	Message          string `json:"message,omitempty"`
	JSON             string `json:"json,omitempty"`
	Val              string `json:"val,omitempty"`
	RequestedNewPort *int   `json:"requestedNewPort,omitempty"`
	ConnectPort      *int   `json:"connectPort,omitempty"`
	SSHUser          string `json:"sshUser,omitempty"`
	Total            int    `json:"total,omitempty"`
	Success          int    `json:"success,omitempty"`
	Failed           int    `json:"failed,omitempty"`
	DryRun           bool   `json:"dryRun"`
	MaxRetries       int    `json:"maxRetries,omitempty"`
	RetryDelaySec    int    `json:"retryDelaySec,omitempty"`
	SuccessPattern   string `json:"successPattern,omitempty"`
	ClusterType      string `json:"clusterType,omitempty"`
	Alone            bool   `json:"alone,omitempty"`
	ScvmIncluded     bool   `json:"scvmIncluded,omitempty"`
	CephSSHChange    bool   `json:"cephSshChange,omitempty"`
}

// SecurityPatchValue는 security patch 응답 val 구조이다.
// @name SecurityPatchValue
type SecurityPatchValue struct {
	Summary SecurityPatchSummary        `json:"summary"`
	Targets []SecurityPatchTargetResult `json:"targets,omitempty"`
}

// SecurityPatchResponse는 security patch API 응답이다.
// @name SecurityPatchResponse
type SecurityPatchResponse struct {
	Code    int    `json:"code" example:"200"`
	Val     any    `json:"val,omitempty"`
	RetName string `json:"retname,omitempty"`
	Message string `json:"message,omitempty"`
}
