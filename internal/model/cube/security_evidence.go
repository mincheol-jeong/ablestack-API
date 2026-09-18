package cube

// SecurityEvidenceRequest는 보안 취약점 증적 자료 생성 옵션이다.
// @name SecurityEvidenceRequest
type SecurityEvidenceRequest struct {
	// 대상 그룹: all/ablecube/scvm/ccvm
	Targets []string `json:"targets,omitempty" example:"all"`
	// cluster.json 대신 명시적으로 수집할 hostname/IP 목록
	Hosts []string `json:"hosts,omitempty" example:"ablecube1,scvm1"`
	// 점검 항목: all, U-01,U-02 또는 U-01:U-13
	Items string `json:"items,omitempty" example:"all"`
	// SSH 사용자
	SSHUser string `json:"ssh_user,omitempty" example:"root"`
	// SSH 포트. 0이면 로컬 sshd 설정에서 자동 감지한다.
	SSHPort int `json:"ssh_port,omitempty" example:"22"`
	// 호스트별 명령 제한 시간(초)
	Timeout int `json:"timeout,omitempty" example:"120"`
	// 명령별 최대 출력 행 수
	MaxOutputLines int `json:"max_output_lines,omitempty" example:"400"`
}

// SecurityEvidenceMetadata는 생성된 최신 보안 증적 패키지 정보이다.
// @name SecurityEvidenceMetadata
type SecurityEvidenceMetadata struct {
	Path              string   `json:"path,omitempty"`
	Filename          string   `json:"filename" example:"ABLESTACK 보안 취약점 증적 자료.zip"`
	Size              int64    `json:"size" example:"1048576"`
	GeneratedAt       string   `json:"generatedAt" example:"2026-07-27T10:30:00+09:00"`
	Hosts             int      `json:"hosts" example:"3"`
	CollectedHosts    []string `json:"collectedHosts,omitempty"`
	RequestedHosts    int      `json:"requestedHosts,omitempty" example:"3"`
	RequestedTargets  []string `json:"requestedTargets,omitempty"`
	ClusterType       string   `json:"clusterType,omitempty" example:"ablestack-hci"`
	TargetGroups      []string `json:"targetGroups,omitempty"`
	Items             int      `json:"items" example:"67"`
	Slides            int      `json:"slides,omitempty" example:"68"`
	CollectorStatus   int      `json:"collectorStatus,omitempty"`
	CollectorWarning  string   `json:"collectorWarning,omitempty"`
	RunDirectory      string   `json:"runDirectory,omitempty"`
	DownloadAvailable bool     `json:"downloadAvailable"`
}

// SecurityEvidenceResponse는 보안 증적 생성/조회 응답이다.
// @name SecurityEvidenceResponse
type SecurityEvidenceResponse struct {
	Code    int                       `json:"code" example:"200"`
	Val     *SecurityEvidenceMetadata `json:"val,omitempty"`
	Message string                    `json:"message,omitempty" example:"ok"`
}
