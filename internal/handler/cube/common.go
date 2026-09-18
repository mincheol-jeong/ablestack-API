package cube

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ablecloud.io/ablestack-api/internal/infra/logging"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"ablecloud.io/ablestack-api/internal/service/clusterconfig"
	"ablecloud.io/ablestack-api/internal/service/security"
)

// -----------------------------------------------------------------------------
// 공통 타입
//
// 사용 위치:
// - `cluster_config.go`
// - `scvm_status.go`
// - `system_config.go`
// - `ssh_scan.go`
//
// 여러 핸들러가 함께 쓰는 타입 alias를 한 곳에 모아둔 영역이다.
// -----------------------------------------------------------------------------

type ClusterHost = CubeModel.ClusterHost
type ClusterSystemProfile = CubeModel.ClusterSystemProfile

const (
	virshTimeout = 3 * time.Second

	defaultAbleStackConfigPath = "/etc/ablestack"
)

func resolveEnvPath(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

func resolveAbleStackConfigPath() string {
	if base := resolveEnvPath("ABLESTACK_CONFIG_PATH"); base != "" {
		return base
	}
	return defaultAbleStackConfigPath
}

func resolveAbleStackStatePath() string {
	if base := resolveEnvPath("ABLESTACK_STATE_PATH"); base != "" {
		return base
	}
	return filepath.Join(resolveAbleStackConfigPath(), "vmconfig")
}

func resolveAbleStackPropertiesPath() string {
	return filepath.Join(resolveAbleStackConfigPath(), "properties")
}

func resolveAbleStackXMLTemplatePath() string {
	return filepath.Join(resolveAbleStackConfigPath(), "xml-template")
}

func resolveAbleStackShellPath() string {
	return filepath.Join(resolveAbleStackConfigPath(), "shell")
}

func resolveAbleStackPropertyFile(name string) string {
	return filepath.Join(resolveAbleStackPropertiesPath(), name)
}

func resolveAbleStackVMConfigDir(name string) string {
	return filepath.Join(resolveAbleStackStatePath(), name)
}

func resolveAbleStackShellFile(candidates ...string) string {
	base := resolveAbleStackShellPath()
	for _, candidate := range candidates {
		path := filepath.Join(base, candidate)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	if len(candidates) == 0 {
		return base
	}
	return filepath.Join(base, candidates[0])
}

// -----------------------------------------------------------------------------
// 클러스터 타깃 / cluster.json 공통 helper
//
// 사용 위치:
// - `ccvm_service_control.go`
// - `ccvm_status.go`
// - `cluster_config.go`
// - `scvm_status.go`
// - `system_config.go`
// - `url.go`
//
// cluster.json 경로 해석, clusterConfig 디코딩, 원격 API URL 생성,
// 현재 노드 여부 판별 같은 공통 로직을 담당한다.
// -----------------------------------------------------------------------------

// isLocalTarget은 전달받은 IP가 현재 노드 자신인지 확인한다.
func isLocalTarget(target string) bool {
	if target == "" {
		return false
	}
	if target == "127.0.0.1" || strings.EqualFold(target, "localhost") {
		return true
	}
	ip := net.ParseIP(target)
	if ip == nil {
		return false
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		switch v := addr.(type) {
		case *net.IPNet:
			if v.IP.Equal(ip) {
				return true
			}
		case *net.IPAddr:
			if v.IP.Equal(ip) {
				return true
			}
		}
	}
	return false
}

// resolveClusterJSONPath는 환경 변수 우선순위에 따라 cluster.json 실제 경로를 결정한다.
func resolveClusterJSONPath() string {
	if env := resolveEnvPath("ABLESTACK_CLUSTER_JSON"); env != "" {
		return env
	}
	return resolveAbleStackPropertyFile("cluster.json")
}

// loadClusterConfigSection은 cluster.json의 `clusterConfig` 섹션만 읽어 구조체로 반환한다.
func loadClusterConfigSection() (*CubeModel.ClusterConfigSection, error) {
	current := &TypeClusterConfig{}
	if err := updateClusterConfig(current); err != nil {
		return nil, err
	}
	if current.Data == nil {
		return nil, fmt.Errorf("cluster.json is empty")
	}
	rawCfg, ok := current.Data["clusterConfig"]
	if !ok {
		return nil, fmt.Errorf("clusterConfig not found")
	}
	raw, err := json.Marshal(rawCfg)
	if err != nil {
		return nil, err
	}

	var cfg CubeModel.ClusterConfigSection
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// buildTargetURL은 대상 노드 API 호출용 기본 URL을 만든다.
func buildTargetURL(target string) string {
	scheme := os.Getenv("ABLESTACK_API_SCHEME")
	if scheme == "" {
		scheme = "http"
	}
	port := configuredAPIPort()
	return fmt.Sprintf("%s://%s:%s", scheme, target, port)
}

func configuredAPIPort() string {
	port := strings.TrimSpace(os.Getenv("ABLESTACK_API_PORT"))
	if port == "" {
		return "18090"
	}
	return port
}

// attachInternalToken은 호스트 간 내부 API 호출에 공유 내부 토큰을 추가한다.
func attachInternalToken(req *http.Request) {
	if req == nil {
		return
	}
	token, err := security.GetInternalToken()
	if err != nil || strings.TrimSpace(token) == "" {
		return
	}
	req.Header.Set(security.InternalTokenHeader, token)
}

// isHCITarget은 클러스터 타입이 HCI 계열인지 판별한다.
func isHCITarget(clusterType string) bool {
	switch strings.ToLower(clusterType) {
	case "ablestack-hci", "ablestack-hci-filesystem":
		return true
	default:
		return false
	}
}

// -----------------------------------------------------------------------------
// systemProfile 공통 helper
//
// 사용 위치:
// - `system_config.go`
// - `ssh_scan.go`
//
// 전체 cluster.json에서 systemProfile과 cluster type만 안전하게 꺼내기 위한 공통 함수들이다.
// -----------------------------------------------------------------------------

// loadClusterJSONRoot는 cluster.json 전체 문서를 map 형태로 읽는다.
func loadClusterJSONRoot() (map[string]any, error) {
	path := resolveClusterJSONPath()
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	root := map[string]any{}
	if err := json.Unmarshal(content, &root); err != nil {
		return nil, err
	}
	if root == nil {
		root = map[string]any{}
	}
	return root, nil
}

// extractSystemProfile은 전체 cluster.json에서 systemProfile만 추출해 구조체로 변환한다.
func extractSystemProfile(root map[string]any) (ClusterSystemProfile, error) {
	normalized := clusterconfig.NormalizeClusterJSON(root)
	raw, err := json.Marshal(normalized["systemProfile"])
	if err != nil {
		return ClusterSystemProfile{}, err
	}
	var profile ClusterSystemProfile
	if err := json.Unmarshal(raw, &profile); err != nil {
		return ClusterSystemProfile{}, err
	}
	return profile, nil
}

// extractClusterType은 전체 cluster.json에서 cluster type 문자열만 추출한다.
func extractClusterType(root map[string]any) string {
	rawCfg, ok := root["clusterConfig"]
	if !ok {
		return ""
	}
	raw, err := json.Marshal(rawCfg)
	if err != nil {
		return ""
	}
	var cfg CubeModel.ClusterConfigSection
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return ""
	}
	return cfg.Type
}

// -----------------------------------------------------------------------------
// SSH 스캔 공통 helper
//
// 사용 위치:
// - `cluster_config.go`
// - `ssh_scan.go`
// - `url.go`
//
// known_hosts 갱신에 필요한 대상 수집과 지연 스캔 스케줄링 로직을 공통으로 관리한다.
// -----------------------------------------------------------------------------

// scheduleSSHKnownHostsScan은 현재 cluster.json 기준으로 전체 대상에 대한 SSH 스캔을 예약한다.
func scheduleSSHKnownHostsScan() {
	hosts, err := collectSSHHostsFromClusterConfig(true, true, true)
	if err != nil || len(hosts) == 0 {
		return
	}
	scheduleSSHKnownHostsScanForHosts(hosts)
}

// scheduleSSHKnownHostsScanForHosts는 전달받은 host 목록에 대해 지연/재시도 기반 SSH 스캔을 수행한다.
func scheduleSSHKnownHostsScanForHosts(hosts []string) {
	targets := dedupeHosts(hosts)
	if len(targets) == 0 {
		return
	}
	port := getSSHScanPort()
	log.Printf("ssh-scan scheduled: targets=%d targets_hosts=%s port=%d", len(targets), formatHosts(targets), port)
	go func(pending []string) {
		delay := sshScanInitialDelay
		scannedSet := map[string]struct{}{}
		if removed, err := repairDefaultKnownHostsFile(); err != nil {
			log.Printf("ssh-scan initial known_hosts repair failed: err=%v", err)
		} else if removed > 0 {
			log.Printf("ssh-scan initial known_hosts repair completed: removed_invalid_lines=%d", removed)
		}
		remaining := pending
		for attempt := 1; attempt <= sshScanMaxAttempts; attempt++ {
			result, err := scanAndUpdateKnownHostsForHosts(remaining, port)
			for _, host := range result.OpenHosts {
				scannedSet[host] = struct{}{}
			}
			log.Printf("ssh-scan attempt: attempt=%d open=%d remaining=%d output_lines=%d open_hosts=%s remaining_hosts=%s port=%d",
				attempt,
				len(result.OpenHosts),
				len(result.Remaining),
				result.OutputLines,
				formatHosts(result.OpenHosts),
				formatHosts(result.Remaining),
				port,
			)
			if err == nil && len(result.OpenHosts) > 0 && len(result.Remaining) == 0 {
				log.Printf("ssh-scan completed: scanned=%d scanned_hosts=%s remaining=0 port=%d",
					len(scannedSet),
					formatHosts(mapKeys(scannedSet)),
					port,
				)
				return
			}
			if len(result.Remaining) == 0 {
				log.Printf("ssh-scan completed: scanned=%d scanned_hosts=%s remaining=0 port=%d",
					len(scannedSet),
					formatHosts(mapKeys(scannedSet)),
					port,
				)
				return
			}
			if attempt == sshScanMaxAttempts {
				logging.AppendJobLog("cube.SSHKnownHostsScan", "scan_incomplete", "error", "ssh scan stopped with remaining targets", map[string]any{
					"attempts":        attempt,
					"scanned":         len(scannedSet),
					"scanned_hosts":   formatHosts(mapKeys(scannedSet)),
					"remaining":       len(result.Remaining),
					"remaining_hosts": formatHosts(result.Remaining),
					"port":            port,
				})
				log.Printf("ssh-scan stopped: attempts=%d scanned=%d scanned_hosts=%s remaining=%d remaining_hosts=%s port=%d",
					attempt,
					len(scannedSet),
					formatHosts(mapKeys(scannedSet)),
					len(result.Remaining),
					formatHosts(result.Remaining),
					port,
				)
				return
			}
			if err != nil {
				log.Printf("ssh-scan attempt failed: attempt=%d err=%v", attempt, err)
			}
			remaining = result.Remaining
			time.Sleep(delay)
			if delay < sshScanMaxDelay {
				delay *= 2
				if delay > sshScanMaxDelay {
					delay = sshScanMaxDelay
				}
			}
		}
	}(targets)
}

// collectSSHHostsFromClusterConfig는 cluster.json에서 SSH 스캔 대상 host/IP 목록을 모은다.
func collectSSHHostsFromClusterConfig(includeAble bool, includeScvm bool, includeCcvm bool) ([]string, error) {
	cfg, err := loadClusterConfigSection()
	if err != nil {
		return nil, err
	}

	ipSet := map[string]struct{}{}
	hosts := make([]string, 0)

	if includeAble {
		for _, host := range cfg.Hosts {
			if strings.TrimSpace(host.Hostname) != "" {
				hosts = append(hosts, strings.TrimSpace(host.Hostname))
			}
			if strings.TrimSpace(host.Ablecube) != "" {
				ipSet[strings.TrimSpace(host.Ablecube)] = struct{}{}
			}
		}
	}
	if includeScvm {
		for _, host := range cfg.Hosts {
			if strings.TrimSpace(host.ScvmMngt) != "" {
				ipSet[strings.TrimSpace(host.ScvmMngt)] = struct{}{}
			}
		}
	}
	if includeCcvm {
		if strings.TrimSpace(cfg.CCVM.IP) != "" {
			ipSet[strings.TrimSpace(cfg.CCVM.IP)] = struct{}{}
		}
	}

	for ip := range ipSet {
		hosts = append(hosts, ip)
	}

	if aliases, err := collectHostsAliasesByIP(ipSet); err == nil {
		hosts = append(hosts, aliases...)
	}

	return dedupeHosts(hosts), nil
}

// dedupeHosts는 host 목록에서 중복과 빈 값을 제거한다.
func dedupeHosts(hosts []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(hosts))
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	return out
}

// -----------------------------------------------------------------------------
// 명령 실행 공통 helper
//
// 사용 위치:
// - `ccvm_status.go`
// - `scvm_status.go`
//
// 로컬 shell 명령과 virsh 실행의 timeout, 출력 수집, 줄 분리를 공통 처리한다.
// -----------------------------------------------------------------------------

// runVirshLines는 virsh 명령 실행 결과를 줄 단위로 반환한다.
func runVirshLines(args ...string) ([]string, bool, error) {
	return runCommandLinesWithEnv("virsh", virshTimeout, virshEnv(), args...)
}

// runCommandLines는 일반 시스템 명령 결과를 줄 단위로 반환한다.
func runCommandLines(command string, timeout time.Duration, args ...string) ([]string, bool, error) {
	return runCommandLinesWithEnv(command, timeout, nil, args...)
}

// runCommandOutputWithEnv는 환경 변수를 포함해 명령을 실행하고 원문 출력 문자열을 반환한다.
func runCommandOutputWithEnv(command string, timeout time.Duration, env []string, args ...string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	if len(env) > 0 {
		cmd.Env = env
	}
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(output), true, ctx.Err()
	}
	if err != nil {
		return string(output), false, err
	}
	return string(output), false, nil
}

// runCommandLinesWithEnv는 runCommandOutputWithEnv 결과를 줄 단위 배열로 바꿔준다.
func runCommandLinesWithEnv(command string, timeout time.Duration, env []string, args ...string) ([]string, bool, error) {
	output, timedOut, err := runCommandOutputWithEnv(command, timeout, env, args...)
	return splitLines(output), timedOut, err
}

// splitLines는 명령 출력 문자열을 비어 있지 않은 줄 목록으로 정리한다.
func splitLines(value string) []string {
	value = strings.TrimRight(value, "\n")
	if value == "" {
		return nil
	}
	raw := strings.Split(value, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// virshEnv는 virsh 출력 언어를 고정해 파싱 결과가 흔들리지 않게 만든다.
func virshEnv() []string {
	return append([]string{"LANG=en_US.utf-8", "LANGUAGE=en"}, os.Environ()...)
}
