package cube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ablecloud.io/ablestack-api/internal/infra/utils"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"github.com/gin-gonic/gin"
)

type SecurityEvidenceRequest = CubeModel.SecurityEvidenceRequest
type SecurityEvidenceMetadata = CubeModel.SecurityEvidenceMetadata
type SecurityEvidenceResponse = CubeModel.SecurityEvidenceResponse

const (
	securityEvidenceDefaultHelper = "/usr/libexec/ablestack-api/python/security_evidence/security_evidence_package.py"
	securityEvidenceDefaultRoot   = "/var/lib/ablestack/security-evidence"
	securityEvidenceCommandTO     = 2 * time.Hour
)

var securityEvidenceItemsPattern = regexp.MustCompile(`^(all|[Uu]-[0-9]{1,3}([,:][Uu]-[0-9]{1,3})*)$`)

// GenerateSecurityEvidence godoc
//
//	@Summary		Generate Security Evidence
//	@Description	cluster.json 대상의 읽기 전용 보안 점검 결과를 수집해 TXT/XLSX/PPTX가 포함된 ZIP 패키지를 생성합니다.
//	@Tags			Cube-Security
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CubeModel.SecurityEvidenceRequest	false	"security evidence options"
//	@Success		200	{object}	CubeModel.SecurityEvidenceResponse
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/security/evidence [post]
func GenerateSecurityEvidence(context *gin.Context) {
	var req SecurityEvidenceRequest
	if context.Request != nil && context.Request.Body != nil && context.Request.ContentLength != 0 {
		if err := context.ShouldBindJSON(&req); err != nil {
			context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
				ErrCode: http.StatusBadRequest,
				Message: "invalid request",
			})
			return
		}
	}
	if err := normalizeSecurityEvidenceRequest(&req); err != nil {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	metadata, err := runSecurityEvidenceHelper(context.Request.Context(), "generate", req)
	if err != nil {
		context.JSON(http.StatusInternalServerError, SecurityEvidenceResponse{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		})
		return
	}
	context.JSON(http.StatusOK, SecurityEvidenceResponse{
		Code:    http.StatusOK,
		Val:     metadata,
		Message: "security evidence generated",
	})
}

// GetLatestSecurityEvidence godoc
//
//	@Summary		Latest Security Evidence
//	@Description	가장 최근에 생성된 보안 증적 ZIP 메타데이터를 반환합니다.
//	@Tags			Cube-Security
//	@Produce		json
//	@Success		200	{object}	CubeModel.SecurityEvidenceResponse
//	@Failure		404	{object}	HTTP404NotFound
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/security/evidence [get]
func GetLatestSecurityEvidence(context *gin.Context) {
	metadata, err := runSecurityEvidenceHelper(context.Request.Context(), "latest", SecurityEvidenceRequest{})
	if err != nil {
		status := http.StatusInternalServerError
		if isSecurityEvidenceNotFound(err) {
			status = http.StatusNotFound
		}
		context.JSON(status, SecurityEvidenceResponse{Code: status, Message: err.Error()})
		return
	}
	context.JSON(http.StatusOK, SecurityEvidenceResponse{
		Code:    http.StatusOK,
		Val:     metadata,
		Message: "ok",
	})
}

// DownloadLatestSecurityEvidence godoc
//
//	@Summary		Download Security Evidence
//	@Description	가장 최근에 생성된 보안 증적 ZIP 파일을 다운로드합니다.
//	@Tags			Cube-Security
//	@Produce		application/zip
//	@Success		200	{file}		file
//	@Failure		404	{object}	HTTP404NotFound
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/security/evidence/download [get]
func DownloadLatestSecurityEvidence(context *gin.Context) {
	metadata, err := runSecurityEvidenceHelper(context.Request.Context(), "latest", SecurityEvidenceRequest{})
	if err != nil {
		status := http.StatusInternalServerError
		if isSecurityEvidenceNotFound(err) {
			status = http.StatusNotFound
		}
		context.JSON(status, SecurityEvidenceResponse{Code: status, Message: err.Error()})
		return
	}
	path, err := validateSecurityEvidenceDownload(metadata)
	if err != nil {
		context.JSON(http.StatusNotFound, SecurityEvidenceResponse{
			Code:    http.StatusNotFound,
			Message: err.Error(),
		})
		return
	}
	filename := filepath.Base(strings.TrimSpace(metadata.Filename))
	if filename == "." || filename == "" {
		filename = "security-evidence.zip"
	}
	context.Header("Cache-Control", "no-store")
	context.FileAttachment(path, filename)
}

func normalizeSecurityEvidenceRequest(req *SecurityEvidenceRequest) error {
	if req == nil {
		return fmt.Errorf("request required")
	}
	if len(req.Targets) == 0 {
		req.Targets = []string{"all"}
	}
	targets := make([]string, 0, len(req.Targets))
	seen := map[string]struct{}{}
	for _, target := range req.Targets {
		target = strings.ToLower(strings.TrimSpace(target))
		switch target {
		case "all", "ablecube", "scvm", "ccvm":
		case "":
			continue
		default:
			return fmt.Errorf("invalid target: %s", target)
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		targets = append(targets, target)
	}
	if len(targets) == 0 {
		targets = []string{"all"}
	}
	req.Targets = targets

	req.Hosts = normalizeStringSlice(req.Hosts)
	for _, host := range req.Hosts {
		if len(host) > 255 || strings.HasPrefix(host, "-") || strings.ContainsAny(host, "\x00\r\n") {
			return fmt.Errorf("invalid host: %s", host)
		}
	}
	req.Items = strings.TrimSpace(req.Items)
	if req.Items == "" {
		req.Items = "all"
	}
	if !securityEvidenceItemsPattern.MatchString(req.Items) {
		return fmt.Errorf("invalid items")
	}
	req.SSHUser = strings.TrimSpace(req.SSHUser)
	if req.SSHUser == "" {
		req.SSHUser = "root"
	}
	if strings.HasPrefix(req.SSHUser, "-") || strings.ContainsAny(req.SSHUser, "\x00\r\n") {
		return fmt.Errorf("invalid ssh_user")
	}
	if req.SSHPort < 0 || req.SSHPort > 65535 {
		return fmt.Errorf("ssh_port must be between 0 and 65535")
	}
	if req.Timeout == 0 {
		req.Timeout = 120
	}
	if req.Timeout < 1 || req.Timeout > 3600 {
		return fmt.Errorf("timeout must be between 1 and 3600")
	}
	if req.MaxOutputLines == 0 {
		req.MaxOutputLines = 400
	}
	if req.MaxOutputLines < 1 || req.MaxOutputLines > 10000 {
		return fmt.Errorf("max_output_lines must be between 1 and 10000")
	}
	return nil
}

func runSecurityEvidenceHelper(parent context.Context, action string, req SecurityEvidenceRequest) (*SecurityEvidenceMetadata, error) {
	helper := resolveSecurityEvidenceHelper()
	if info, err := os.Stat(helper); err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("security evidence helper not found: %s", helper)
	}

	args := []string{helper, action, "--storage-root", resolveSecurityEvidenceRoot()}
	if action == "generate" {
		args = append(args,
			"--cluster-json", resolveClusterJSONPath(),
			"--items", req.Items,
			"--ssh-user", req.SSHUser,
			"--ssh-port", strconv.Itoa(req.SSHPort),
			"--timeout", strconv.Itoa(req.Timeout),
			"--max-output-lines", strconv.Itoa(req.MaxOutputLines),
		)
		if len(req.Hosts) > 0 {
			for _, host := range req.Hosts {
				args = append(args, "--host", host)
			}
		} else {
			args = append(args, "--targets")
			args = append(args, req.Targets...)
		}
	}

	commandContext, cancel := context.WithTimeout(parent, securityEvidenceCommandTO)
	defer cancel()
	cmd := exec.CommandContext(commandContext, "python3", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if commandContext.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("security evidence %s timed out", action)
	}

	code, metadata, message, parseErr := parseSecurityEvidenceHelperOutput(stdout.String())
	if parseErr != nil {
		if runErr != nil {
			return nil, fmt.Errorf("security evidence %s failed: %s", action, firstNonEmpty(strings.TrimSpace(stderr.String()), runErr.Error()))
		}
		return nil, parseErr
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("security evidence %s failed (%d): %s", action, code, firstNonEmpty(message, strings.TrimSpace(stderr.String())))
	}
	if runErr != nil {
		return nil, fmt.Errorf("security evidence %s failed: %s", action, firstNonEmpty(strings.TrimSpace(stderr.String()), runErr.Error()))
	}
	metadata.DownloadAvailable = true
	return metadata, nil
}

func parseSecurityEvidenceHelperOutput(output string) (int, *SecurityEvidenceMetadata, string, error) {
	lines := splitLines(output)
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var raw struct {
			Code int             `json:"code"`
			Val  json.RawMessage `json:"val"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		if raw.Code != http.StatusOK {
			var message string
			_ = json.Unmarshal(raw.Val, &message)
			return raw.Code, nil, message, nil
		}
		var metadata SecurityEvidenceMetadata
		if err := json.Unmarshal(raw.Val, &metadata); err != nil {
			return 0, nil, "", fmt.Errorf("invalid security evidence metadata: %w", err)
		}
		return raw.Code, &metadata, "", nil
	}
	return 0, nil, "", fmt.Errorf("invalid security evidence helper response")
}

func validateSecurityEvidenceDownload(metadata *SecurityEvidenceMetadata) (string, error) {
	if metadata == nil || strings.TrimSpace(metadata.Path) == "" {
		return "", fmt.Errorf("security evidence package path is empty")
	}
	root, err := filepath.Abs(resolveSecurityEvidenceRoot())
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Clean(metadata.Path))
	if err != nil {
		return "", err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("security evidence storage root not found")
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("security evidence package not found")
	}
	resolvedRelative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("security evidence package resolves outside storage root")
	}
	if !strings.EqualFold(filepath.Ext(resolvedPath), ".zip") {
		return "", fmt.Errorf("security evidence package is not a ZIP file")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("security evidence package not found")
	}
	return path, nil
}

func resolveSecurityEvidenceHelper() string {
	if path := strings.TrimSpace(os.Getenv("ABLESTACK_SECURITY_EVIDENCE_HELPER")); path != "" {
		return path
	}
	for _, path := range []string{
		securityEvidenceDefaultHelper,
		"/usr/share/cockpit/ablestack/python/security_evidence/security_evidence_package.py",
	} {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path
		}
	}
	return securityEvidenceDefaultHelper
}

func resolveSecurityEvidenceRoot() string {
	if path := strings.TrimSpace(os.Getenv("ABLESTACK_SECURITY_EVIDENCE_ROOT")); path != "" {
		return path
	}
	return securityEvidenceDefaultRoot
}

func isSecurityEvidenceNotFound(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "(404)") || strings.Contains(message, "not found") || strings.Contains(message, "없습니다")
}
