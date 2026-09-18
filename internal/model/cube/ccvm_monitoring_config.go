package cube

// CCVMMonitoringSMTPConfig는 Wall/Grafana와 Mold 알림 SMTP 설정이다.
// @name CCVMMonitoringSMTPConfig
type CCVMMonitoringSMTPConfig struct {
	Enabled  bool   `json:"enabled" example:"true"`
	Host     string `json:"host,omitempty" example:"10.10.31.20"`
	Port     int    `json:"port,omitempty" example:"25"`
	User     string `json:"user,omitempty" example:"admin@example.com"`
	Password string `json:"password,omitempty" example:"password"`
}

// CCVMMonitoringConfigRequest는 CCVM의 모니터링 최초 구성 또는 수집 대상 갱신 요청이다.
// @name CCVMMonitoringConfigRequest
type CCVMMonitoringConfigRequest struct {
	Action      string                    `json:"action,omitempty" example:"configure" enums:"configure,update"`
	CCVM        []string                  `json:"ccvm,omitempty" example:"10.10.31.10"`
	Cube        []string                  `json:"cube,omitempty" example:"10.10.31.1,10.10.31.2"`
	SCVM        []string                  `json:"scvm,omitempty" example:"10.10.31.11,10.10.31.12"`
	SMTP        *CCVMMonitoringSMTPConfig `json:"smtp,omitempty"`
	ClusterType string                    `json:"cluster_type,omitempty" swaggerignore:"true"`
}

// CCVMMonitoringConfigStep는 모니터링 구성의 개별 실행 결과이다.
// @name CCVMMonitoringConfigStep
type CCVMMonitoringConfigStep struct {
	Name    string `json:"name" example:"configure_wall"`
	Status  string `json:"status" example:"succeeded"`
	Code    int    `json:"code" example:"200"`
	Message string `json:"message,omitempty" example:"success wall configuration"`
	Output  string `json:"output,omitempty"`
}

// CCVMMonitoringServiceStatus는 구성 후 Wall 서비스 상태이다.
// @name CCVMMonitoringServiceStatus
type CCVMMonitoringServiceStatus struct {
	Name    string `json:"name" example:"prometheus"`
	Active  bool   `json:"active" example:"true"`
	Enabled bool   `json:"enabled" example:"true"`
}

// CCVMMonitoringConfigResponse는 모니터링 수집 대상 갱신 결과이다.
// @name CCVMMonitoringConfigResponse
type CCVMMonitoringConfigResponse struct {
	Code          int                           `json:"code" example:"200"`
	Action        string                        `json:"action,omitempty" example:"configure"`
	Message       string                        `json:"message" example:"monitoring configuration success"`
	Val           string                        `json:"val,omitempty" example:"ok"`
	Steps         []CCVMMonitoringConfigStep    `json:"steps,omitempty"`
	Services      []CCVMMonitoringServiceStatus `json:"services,omitempty"`
	SystemProfile []ClusterApplyResult          `json:"system_profile,omitempty"`
}
