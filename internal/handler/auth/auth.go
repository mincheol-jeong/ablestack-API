package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"ablecloud.io/ablestack-api/internal/service/authservice"
	"ablecloud.io/ablestack-api/internal/service/licenseservice"
	"ablecloud.io/ablestack-api/internal/service/security"
	"github.com/gin-gonic/gin"
)

type LoginRequest struct {
	ID       string `json:"id" example:"root"`
	Password string `json:"password" example:"password"`
}

type LoginResponse struct {
	Code          int    `json:"code" example:"200"`
	TokenType     string `json:"token_type" example:"Bearer"`
	AccessToken   string `json:"access_token"`
	Authorization string `json:"authorization" example:"Bearer eyJ..."`
	ExpiresIn     int64  `json:"expires_in" example:"3600"`
}

type MeResponse struct {
	Code int                    `json:"code" example:"200"`
	Val  authservice.TokenClaim `json:"val"`
}

type InternalTokenRotateResponse struct {
	Code    int                   `json:"code" example:"200"`
	Val     map[string]any        `json:"val"`
	Results []InternalTokenResult `json:"results,omitempty"`
	Message string                `json:"message,omitempty"`
}

type InternalTokenApplyRequest struct {
	InternalToken string `json:"internal_token"`
}

type InternalTokenResult struct {
	Target  string `json:"target"`
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

// Login godoc
//
//	@Summary		Login
//	@Description	웹 UI/API용 access token을 발급합니다.
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		LoginRequest	true	"login request"
//	@Success		200	{object}	LoginResponse
//	@Failure		401	{object}	map[string]any
//	@Router			/auth/login [post]
func Login(context *gin.Context) {
	var req LoginRequest
	if err := context.ShouldBindJSON(&req); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": "invalid request"})
		return
	}
	if !authservice.VerifyLinuxCredentials(req.ID, req.Password) {
		context.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "invalid credentials"})
		return
	}

	token, err := authservice.IssueAccessToken(strings.TrimSpace(req.ID))
	if err != nil {
		status := http.StatusInternalServerError
		if isLicenseAccessError(err) {
			status = http.StatusForbidden
		}
		context.JSON(status, gin.H{"code": status, "message": err.Error()})
		return
	}
	context.JSON(http.StatusOK, LoginResponse{
		Code:          http.StatusOK,
		TokenType:     token.TokenType,
		AccessToken:   token.AccessToken,
		Authorization: token.Authorization,
		ExpiresIn:     token.ExpiresIn,
	})
}

// Me godoc
//
//	@Summary		Current User
//	@Description	Bearer token을 검증하고 현재 token claim을 반환합니다.
//	@Tags			Auth
//	@Produce		json
//	@Success		200	{object}	MeResponse
//	@Failure		401	{object}	map[string]any
//	@Router			/auth/me [get]
func Me(context *gin.Context) {
	claim, err := authservice.ClaimsFromRequest(context.Request)
	if err != nil {
		context.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": err.Error()})
		return
	}
	context.JSON(http.StatusOK, MeResponse{Code: http.StatusOK, Val: claim})
}

// RotateInternalToken godoc
//
//	@Summary		Rotate Internal Token
//	@Description	클러스터 내부 API 호출용 X-Cube-Internal-Token 값을 교체하고 대상 AbleCube 노드에 적용합니다.
//	@Tags			Auth
//	@Produce		json
//	@Success		200	{object}	InternalTokenRotateResponse
//	@Failure		401	{object}	map[string]any
//	@Failure		409	{object}	map[string]any
//	@Failure		500	{object}	map[string]any
//	@Router			/auth/internal-token/rotate [post]
func RotateInternalToken(context *gin.Context) {
	if _, err := authservice.ClaimsFromRequest(context.Request); err != nil {
		context.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": err.Error()})
		return
	}
	if strings.TrimSpace(os.Getenv("CUBE_INTERNAL_TOKEN")) != "" || strings.TrimSpace(os.Getenv("ABLESTACK_INTERNAL_TOKEN")) != "" {
		context.JSON(http.StatusConflict, gin.H{"code": http.StatusConflict, "message": "internal token is managed by environment"})
		return
	}

	oldToken, _, err := security.EnsureInternalToken()
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": err.Error()})
		return
	}
	newToken, err := security.GenerateToken()
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": err.Error()})
		return
	}

	targets, err := security.ClusterAblecubeTargets()
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": err.Error()})
		return
	}
	results := applyInternalTokenToTargets(targets, oldToken, newToken)
	for _, result := range results {
		if result.Code >= 300 {
			context.JSON(http.StatusInternalServerError, InternalTokenRotateResponse{
				Code:    http.StatusInternalServerError,
				Message: "internal token rotation failed",
				Results: results,
			})
			return
		}
	}

	if err := security.SetInternalToken(newToken); err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": err.Error()})
		return
	}
	context.JSON(http.StatusOK, InternalTokenRotateResponse{
		Code: http.StatusOK,
		Val: map[string]any{
			"message":    "internal token rotated",
			"token":      security.MaskToken(newToken),
			"target_cnt": len(results),
		},
		Results: results,
	})
}

// ApplyInternalToken godoc
//
//	@Summary		Apply Internal Token
//	@Description	내부 API 서버 간 호출로 전달받은 X-Cube-Internal-Token 값을 현재 호스트에 적용합니다.
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			X-Cube-Internal-Token	header	string					true	"current internal token"
//	@Param			body					body	InternalTokenApplyRequest	true	"internal token apply request"
//	@Success		200						{object}	map[string]any
//	@Failure		400						{object}	map[string]any
//	@Failure		401						{object}	map[string]any
//	@Failure		500						{object}	map[string]any
//	@Router			/auth/internal-token/apply [post]
func ApplyInternalToken(context *gin.Context) {
	if !security.ValidateInternalToken(context.GetHeader(security.InternalTokenHeader)) {
		context.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "invalid internal token"})
		return
	}
	var req InternalTokenApplyRequest
	if err := context.ShouldBindJSON(&req); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": "invalid request"})
		return
	}
	if strings.TrimSpace(req.InternalToken) == "" {
		context.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": "internal_token required"})
		return
	}
	if err := security.SetInternalToken(req.InternalToken); err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": err.Error()})
		return
	}
	context.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "val": "internal token applied"})
}

func Middleware() gin.HandlerFunc {
	return func(context *gin.Context) {
		if context.Request.Method == http.MethodOptions || isPublicPath(context.Request.URL.Path) {
			context.Next()
			return
		}

		if err := requireActiveLicense(context.Request.URL.Path); err != nil {
			context.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "message": err.Error()})
			return
		}

		if hasCubeInternalHeader(context.Request.Header) {
			if !security.ValidateInternalToken(context.GetHeader(security.InternalTokenHeader)) {
				if allowClusterApplyLocalBootstrap(context) {
					context.Next()
					return
				}
				context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "invalid internal token"})
				return
			}
			context.Next()
			return
		}

		if !authservice.AuthRequired() {
			context.Next()
			return
		}
		if _, err := authservice.ClaimsFromRequest(context.Request); err != nil {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": err.Error()})
			return
		}
		context.Next()
	}
}

func allowClusterApplyLocalBootstrap(context *gin.Context) bool {
	if context.Request.Method != http.MethodPost || context.Request.URL.Path != "/api/v1/cube/cluster/apply-local" {
		return false
	}
	currentToken, err := security.GetInternalToken()
	if err != nil || strings.TrimSpace(currentToken) != "" {
		return false
	}
	headerToken := strings.TrimSpace(context.GetHeader(security.InternalTokenHeader))
	if len(headerToken) < 16 {
		return false
	}
	body, err := io.ReadAll(context.Request.Body)
	if err != nil {
		return false
	}
	context.Request.Body = io.NopCloser(bytes.NewReader(body))

	var payload struct {
		Action   string `json:"action"`
		Security struct {
			InternalToken string `json:"internal_token"`
		} `json:"security"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(payload.Action), "insert") {
		return false
	}
	return strings.TrimSpace(payload.Security.InternalToken) == headerToken
}

func isPublicPath(path string) bool {
	return path == "/api/v1/auth/login" ||
		path == "/api/v1/health" ||
		path == "/health" ||
		path == "/api/v1/cube/license" ||
		strings.HasPrefix(path, "/api/v1/swagger") ||
		strings.HasPrefix(path, "/swagger")
}

func requireActiveLicense(path string) error {
	if path == "/api/v1/cube/license" ||
		path == "/api/v1/auth/login" ||
		path == "/api/v1/health" ||
		path == "/health" ||
		strings.HasPrefix(path, "/api/v1/swagger") ||
		strings.HasPrefix(path, "/swagger") {
		return nil
	}
	if _, err := licenseservice.CurrentAuthSecret(); err != nil {
		return fmt.Errorf("active license required: %w", err)
	}
	return nil
}

func isLicenseAccessError(err error) bool {
	return errors.Is(err, licenseservice.ErrNoLicense) ||
		errors.Is(err, licenseservice.ErrExpired) ||
		errors.Is(err, licenseservice.ErrInactive) ||
		errors.Is(err, licenseservice.ErrInvalid) ||
		errors.Is(err, licenseservice.ErrLicenseKey) ||
		errors.Is(err, licenseservice.ErrNotYetValid)
}

func hasCubeInternalHeader(header http.Header) bool {
	for key, values := range header {
		if strings.EqualFold(key, security.InternalTokenHeader) {
			return true
		}
		if strings.HasPrefix(strings.ToLower(key), "x-cube-") && len(values) > 0 {
			for _, value := range values {
				if strings.TrimSpace(value) != "" {
					return true
				}
			}
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func applyInternalTokenToTargets(targets []string, oldToken string, newToken string) []InternalTokenResult {
	results := make([]InternalTokenResult, 0, len(targets))
	for _, target := range targets {
		if security.IsLocalTarget(target) {
			continue
		}
		result := applyInternalTokenToTarget(target, oldToken, newToken)
		results = append(results, result)
	}
	return results
}

func applyInternalTokenToTarget(target string, oldToken string, newToken string) InternalTokenResult {
	body, _ := json.Marshal(InternalTokenApplyRequest{InternalToken: newToken})
	url := fmt.Sprintf("%s/api/v1/auth/internal-token/apply", buildTargetURL(target))
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return InternalTokenResult{Target: target, Code: http.StatusInternalServerError, Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(security.InternalTokenHeader, oldToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return InternalTokenResult{Target: target, Code: http.StatusInternalServerError, Message: err.Error()}
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return InternalTokenResult{Target: target, Code: resp.StatusCode, Message: strings.TrimSpace(string(raw))}
	}
	return InternalTokenResult{Target: target, Code: resp.StatusCode}
}

func buildTargetURL(target string) string {
	scheme := firstNonEmpty(os.Getenv("ABLESTACK_API_SCHEME"), "http")
	port := firstNonEmpty(os.Getenv("ABLESTACK_API_PORT"), "18090")
	return fmt.Sprintf("%s://%s:%s", scheme, strings.TrimSpace(target), port)
}
