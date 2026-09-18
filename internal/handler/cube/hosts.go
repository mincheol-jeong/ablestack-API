package cube

import (
	"net/http"
	"os"
	"strings"
	"time"

	"ablecloud.io/ablestack-api/internal/infra/logging"
	"ablecloud.io/ablestack-api/internal/infra/utils"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"github.com/gin-gonic/gin"
)

type TypeHost = CubeModel.TypeHost
type HostRoleGroup = CubeModel.HostRoleGroup
type TypeHosts = CubeModel.TypeHosts

// GetHosts godoc
//
//	@Summary		Show List of Hosts
//	@Description	Cube-Hosts의 Hosts 파일의 목록을 보여줍니다.
//	@Tags			Cube-Hosts
//	@Accept			x-www-form-urlencoded
//	@Produce		json
//	@Success		200	{object}	CubeModel.TypeHosts
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		404	{object}	HTTP404NotFound
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/hosts [get]
func GetHosts(context *gin.Context) {
	current := &TypeHosts{}
	if err := updateHosts(current); err != nil {
		context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
			ErrCode: http.StatusInternalServerError,
			Message: "failed to read hosts",
		})
		return
	}
	context.IndentedJSON(http.StatusOK, current)
}

// UpdateHosts는 전역 hosts 모델을 현재 /etc/hosts 기준으로 새로 고친다.
func UpdateHosts() {
	err := updateHosts(CubeModel.Hosts())
	logging.RecordJobResult("cube.UpdateHosts", err, nil)
}

// updateHosts는 hosts 파일을 읽어 응답 구조체 형태로 재구성한다.
func updateHosts(host *TypeHosts) error {
	if host == nil {
		return nil
	}

	content, err := readHostsFile()
	if err != nil {
		return err
	}

	entries := parseHostsEntries(content)
	result := buildHostsResponse(entries)
	result.RefreshTime = time.Now()
	host.ApplyFrom(result)
	return nil
}

// readHostsFile은 운영 환경에서는 /etc/hosts를, 디버그 환경에서는 샘플 데이터를 읽는다.
func readHostsFile() ([]byte, error) {
	if gin.Mode() == gin.ReleaseMode {
		return os.ReadFile("/etc/hosts")
	}

	// Debug 샘플
	return []byte("#comment\n##commentttt\n127.0.0.1\tlocalhost localhost.localdomain localhost4 localhost4.localdomain4\n::1\tlocalhost localhost.localdomain localhost6 localhost.localdomain6\n10.10.33.10\tccvm-mngt ccvm\n10.10.33.1\tablecube1 ablecube\n10.10.33.11\tscvm1-mngt scvm-mngt\n100.100.33.1\tablecube1-pn ablecube-pn\n100.100.33.11\tscvm1 scvm\n100.200.33.11\tscvm1-cn scvm-cn\n10.10.33.2\tablecube2\n10.10.33.12\tscvm2-mngt\n100.100.33.2\tablecube2-pn\n100.100.33.12\tscvm2\n100.200.33.12\tscvm2-cn\n10.10.33.3\tablecube3\n10.10.33.13\tscvm3-mngt\n100.100.33.3\tablecube3-pn\n100.100.33.13\tscvm3\n100.200.33.13\tscvm3-cn\n10.10.33.4\tablecube4\n10.10.33.14\tscvm4-mngt\n100.100.33.4\tablecube4-pn\n100.100.33.14\tscvm4\n100.200.33.14\tscvm4-cn\n10.10.33.5\tablecube5\n10.10.33.15\tscvm5-mngt\n100.100.33.5\tablecube5-pn\n100.100.33.15\tscvm5\n100.200.33.15\tscvm5-cn\n###comment"), nil
}

// parseHostsEntries는 hosts 파일 텍스트를 IP/hostname 엔트리 목록으로 변환한다.
func parseHostsEntries(content []byte) []TypeHost {
	lines := strings.Split(string(content), "\n")
	entries := make([]TypeHost, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		items := strings.Fields(line)
		if len(items) < 2 {
			continue
		}
		entries = append(entries, TypeHost{IP: items[0], HostNames: items[1:]})
	}
	return entries
}

// buildHostsResponse는 엔트리를 네트워크/역할 기준으로 분류한 최종 응답 구조를 만든다.
func buildHostsResponse(entries []TypeHost) *TypeHosts {
	result := &TypeHosts{}

	for _, entry := range entries {
		if isLocalhostEntry(entry) {
			result.Localhost = append(result.Localhost, entry)
			continue
		}

		network := detectHostNetwork(entry)
		role := detectHostRole(entry)
		self := isSelfEntry(entry)
		cleanEntry := sanitizeEntry(entry)

		switch network {
		case "public-network":
			addToRoleGroup(&result.PublicNetwork, role, cleanEntry, self, &result.Others)
		case "client-network":
			addToRoleGroup(&result.ClientNetwork, role, cleanEntry, self, &result.Others)
		default:
			addToRoleGroup(&result.ManagementNetwork, role, cleanEntry, self, &result.Others)
		}
	}

	return result
}

// isLocalhostEntry는 localhost 계열 엔트리인지 판별한다.
func isLocalhostEntry(entry TypeHost) bool {
	if entry.IP == "127.0.0.1" || entry.IP == "::1" {
		return true
	}
	for _, name := range entry.HostNames {
		if strings.Contains(strings.ToLower(name), "localhost") {
			return true
		}
	}
	return false
}

// detectHostNetwork는 IP 대역과 hostname 규칙으로 관리망/공인망/클라이언트망을 판별한다.
func detectHostNetwork(entry TypeHost) string {
	names := strings.ToLower(strings.Join(entry.HostNames, " "))
	ip := entry.IP

	if strings.HasPrefix(ip, "100.100.") || strings.Contains(names, "pn-") || strings.Contains(names, "-pn") {
		return "public-network"
	}
	if strings.HasPrefix(ip, "100.200.") || strings.Contains(names, "cn-") || strings.Contains(names, "-cn") {
		return "client-network"
	}
	return "management-network"
}

// detectHostRole는 hostname 패턴을 기준으로 ablecube/scvm/ccvm 역할을 추론한다.
func detectHostRole(entry TypeHost) string {
	names := strings.ToLower(strings.Join(entry.HostNames, " "))
	switch {
	case strings.Contains(names, "ccvm"):
		return "ccvm"
	case strings.Contains(names, "ablecube"):
		return "ablecube"
	case strings.Contains(names, "scvm"):
		return "scvm"
	default:
		return "other"
	}
}

// isSelfEntry는 엔트리의 hostname 중 현재 노드를 가리키는 별칭이 있는지 확인한다.
func isSelfEntry(entry TypeHost) bool {
	for _, name := range entry.HostNames {
		if isSelfHostname(name) {
			return true
		}
	}
	return false
}

// isSelfHostname은 자기 자신을 의미하는 고정 hostname 별칭인지 판별한다.
func isSelfHostname(name string) bool {
	switch strings.ToLower(name) {
	case "ablecube", "scvm", "pn-ablecube", "ablecube-pn", "cn-scvm", "scvm-cn":
		return true
	default:
		return false
	}
}

// sanitizeEntry는 self alias를 제외한 hostname 목록으로 엔트리를 정리한다.
func sanitizeEntry(entry TypeHost) TypeHost {
	cleaned := entry
	cleaned.HostNames = sanitizeHostnames(entry.HostNames)
	return cleaned
}

// sanitizeHostnames는 self alias hostname을 제거해 표시용 배열로 만든다.
func sanitizeHostnames(names []string) []string {
	if len(names) == 0 {
		return names
	}
	clean := make([]string, 0, len(names))
	for _, name := range names {
		if isSelfHostname(name) {
			continue
		}
		clean = append(clean, name)
	}
	if len(clean) == 0 {
		return names
	}
	return clean
}

// addToRoleGroup는 분류된 엔트리를 네트워크 그룹 구조체에 맞게 배치한다.
func addToRoleGroup(group **HostRoleGroup, role string, entry TypeHost, self bool, others *[]TypeHost) {
	if role == "other" {
		*others = append(*others, entry)
		return
	}
	if *group == nil {
		*group = &HostRoleGroup{}
	}
	if self {
		(*group).Self = &entry
		if role == "ccvm" {
			return
		}
	}
	switch role {
	case "ccvm":
		if (*group).CCVM == nil {
			(*group).CCVM = &entry
		} else {
			*others = append(*others, entry)
		}
	case "ablecube":
		(*group).Ablecube = append((*group).Ablecube, entry)
	case "scvm":
		(*group).SCVM = append((*group).SCVM, entry)
	default:
		*others = append(*others, entry)
	}
}
