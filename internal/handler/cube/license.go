package cube

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"ablecloud.io/ablestack-api/internal/infra/logging"
	"ablecloud.io/ablestack-api/internal/infra/utils"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"ablecloud.io/ablestack-api/internal/service/authservice"
	"ablecloud.io/ablestack-api/internal/service/licenseservice"
	"ablecloud.io/ablestack-api/internal/service/security"
)

type LicenseRequest = CubeModel.LicenseRequest
type LicenseResponse = CubeModel.LicenseResponse
type LicenseStatus = CubeModel.LicenseStatus

const (
	maxLicenseUploadBytes    = 10 * 1024 * 1024
	maxLicenseMultipartBytes = maxLicenseUploadBytes + 1024*1024
	defaultLicenseFilename   = "license.lic"
)

type boundLicenseRequest struct {
	Action         string
	LicenseContent string
	Filename       string
}

// LicenseControl godoc
//
//	@Summary		License Control
//	@Description	라이센스 등록 및 상태 확인을 수행합니다. 등록은 JSON license_content 또는 multipart license_file/file 업로드를 지원합니다.
//	@Tags			Cube-License
//	@Accept			mpfd
//	@Produce		json
//	@Param			action	formData	string	true	"status/register"
//	@Param			license_file	formData	file	false	"license file"
//	@Success		200	{object}	CubeModel.LicenseResponse
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/license [post]
func LicenseControl(context *gin.Context) {
	req, err := bindLicenseRequest(context)
	if err != nil {
		addLicenseDetail(context, "control", "bind_request", err.Error(), map[string]any{
			"content_type": context.ContentType(),
		})
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	action := normalizeLicenseAction(req.Action)
	if action == "" {
		reason := "unsupported action"
		if strings.TrimSpace(req.Action) == "" {
			reason = "missing required field: action"
		}
		addLicenseDetail(context, "control", "normalize_action", reason, map[string]any{
			"raw_action":       req.Action,
			"accepted_actions": []string{"status", "register", "update", "create"},
		})
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: "unsupported action",
		})
		return
	}
	appendAbleStackAPILog("license", "event=request_received action=%q client=%q content_type=%q has_content=%t", action, context.ClientIP(), context.ContentType(), strings.TrimSpace(req.LicenseContent) != "")

	var resp LicenseResponse

	switch action {
	case "status":
		resp, err = getLicenseStatus()
	case "register":
		if licenseservice.HasActiveLicense() {
			if _, authErr := authservice.ClaimsFromRequest(context.Request); authErr != nil {
				if !security.ValidateInternalToken(context.GetHeader(security.InternalTokenHeader)) {
					addLicenseDetail(context, "register", "authorize_replace", "authorization required to replace active license", map[string]any{
						"has_active_license": true,
						"auth_error":         authErr,
					})
					context.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "authorization required to replace active license"})
					return
				}
			}
		}
		resp, err = registerLicense(context, req.LicenseContent, req.Filename)
	}
	if err != nil {
		appendAbleStackAPILog("license", "event=request_failed action=%q client=%q error=%q", action, context.ClientIP(), err.Error())
		context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
			ErrCode: http.StatusInternalServerError,
			Message: err.Error(),
		})
		return
	}

	if resp.Code != 200 {
		appendAbleStackAPILog("license", "event=request_rejected action=%q client=%q code=%d", action, context.ClientIP(), resp.Code)
		status := resp.Code
		if status < 100 || status > 599 {
			status = http.StatusInternalServerError
		}
		context.JSON(status, resp)
		return
	}
	appendAbleStackAPILog("license", "event=request_success action=%q client=%q code=%d", action, context.ClientIP(), resp.Code)
	context.JSON(http.StatusOK, resp)
}

func bindLicenseRequest(context *gin.Context) (boundLicenseRequest, error) {
	if context.ContentType() != "multipart/form-data" {
		var req LicenseRequest
		if err := context.ShouldBindJSON(&req); err != nil {
			return boundLicenseRequest{}, fmt.Errorf("invalid request")
		}
		return boundLicenseRequest{
			Action:         req.Action,
			LicenseContent: req.LicenseContent,
			Filename:       defaultLicenseFilename,
		}, nil
	}

	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, maxLicenseMultipartBytes)

	action := strings.TrimSpace(context.PostForm("action"))
	fileHeader, err := licenseFormFile(context)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return boundLicenseRequest{Action: action}, nil
		}
		return boundLicenseRequest{}, err
	}

	encodedContent, err := encodeLicenseUpload(fileHeader)
	if err != nil {
		return boundLicenseRequest{}, err
	}
	if action == "" {
		action = "register"
	}

	return boundLicenseRequest{
		Action:         action,
		LicenseContent: encodedContent,
		Filename:       fileHeader.Filename,
	}, nil
}

func licenseFormFile(context *gin.Context) (*multipart.FileHeader, error) {
	for _, field := range []string{"license_file", "file"} {
		fileHeader, err := context.FormFile(field)
		if err == nil {
			return fileHeader, nil
		}
		if !errors.Is(err, http.ErrMissingFile) {
			return nil, err
		}
	}
	return nil, http.ErrMissingFile
}

func encodeLicenseUpload(fileHeader *multipart.FileHeader) (string, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, maxLicenseUploadBytes+1))
	if err != nil {
		return "", err
	}
	if len(content) > maxLicenseUploadBytes {
		return "", fmt.Errorf("license file exceeds %d bytes", maxLicenseUploadBytes)
	}
	if strings.TrimSpace(string(content)) == "" {
		return "", fmt.Errorf("license file is empty")
	}

	return base64.StdEncoding.EncodeToString(content), nil
}

// normalizeLicenseAction은 지원하는 라이선스 액션만 내부 표준 값으로 변환한다.
func normalizeLicenseAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "status":
		return "status"
	case "register", "update", "create":
		return "register"
	default:
		return ""
	}
}

// getLicenseStatus는 현재 노드에 등록된 라이선스 파일을 읽어 상태를 계산한다.
func getLicenseStatus() (LicenseResponse, error) {
	status, err := licenseservice.CurrentStatus()
	if errors.Is(err, licenseservice.ErrNoLicense) {
		return LicenseResponse{Code: 404, Val: "등록된 라이센스가 없습니다."}, nil
	}
	if err != nil {
		return LicenseResponse{Code: 500, Val: fmt.Sprintf("라이센스 정보를 읽을 수 없습니다: %s", err.Error())}, nil
	}

	resp := LicenseStatus{
		Status:   status.Status,
		Expired:  status.Expired,
		Issued:   status.Issued,
		OEM:      status.OEM,
		FilePath: status.FilePath,
	}
	return LicenseResponse{Code: 200, Val: resp}, nil
}

// registerLicense는 전달받은 라이선스 내용을 복호화 검증 후 호스트 전용 경로에 저장한다.
func registerLicense(context *gin.Context, content string, filename string) (LicenseResponse, error) {
	if strings.TrimSpace(content) == "" {
		addLicenseDetail(context, "register", "validate_request", "missing license content", map[string]any{
			"missing_field":              licenseContentFieldName(context),
			"accepted_multipart_fields":  []string{"license_file", "file"},
			"accepted_json_fields":       []string{"license_content"},
			"content_type":               context.ContentType(),
			"filename":                   filename,
			"license_content_provided":   false,
			"expected_multipart_example": "license_file=@./license.lic",
		})
		return LicenseResponse{Code: 400, Val: "유효하지 않은 라이센스 내용입니다."}, nil
	}

	if strings.TrimSpace(filename) == "" {
		addLicenseDetail(context, "register", "validate_request", "missing license filename", map[string]any{
			"missing_field": "license_file filename",
			"content_type":  context.ContentType(),
		})
		return LicenseResponse{Code: 400, Val: "license filename required"}, nil
	}

	status, err := licenseservice.Register(content, filename)
	if err != nil {
		addLicenseDetail(context, "register", "process_license", licenseErrorReason(err), map[string]any{
			"filename":    filename,
			"error":       err,
			"error_class": licenseErrorClass(err),
		})
		if isLicenseInputError(err) {
			return LicenseResponse{Code: 400, Val: fmt.Sprintf("라이센스 내용을 처리할 수 없습니다: %s", err.Error())}, nil
		}
		return LicenseResponse{Code: 500, Val: fmt.Sprintf("라이센스 내용을 처리할 수 없습니다: %s", err.Error())}, nil
	}
	if _, _, err := security.EnsureInternalToken(); err != nil {
		addLicenseDetail(context, "register", "ensure_internal_token", "failed to ensure internal token after license registration", map[string]any{
			"filename": filename,
			"error":    err,
		})
		return LicenseResponse{}, err
	}
	if err := syncLicenseSystemProfile(status); err != nil {
		addLicenseDetail(context, "register", "sync_system_profile", "failed to update cluster.json license profile", map[string]any{
			"filename": filename,
			"oem":      status.OEM,
			"error":    err,
		})
		return LicenseResponse{}, err
	}

	return LicenseResponse{Code: 200, Val: "라이센스가 성공적으로 등록되었습니다."}, nil
}

func syncLicenseSystemProfile(status licenseservice.Status) error {
	root, err := loadClusterJSONRoot()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	profile, err := ensureSystemProfileMap(root)
	if err != nil {
		return err
	}
	license := ensureMap(profile, "license")
	license["status"] = "true"
	license["type"] = strings.TrimSpace(status.OEM)
	return saveClusterJSONRoot(root)
}

func currentLicenseTypeValue() string {
	status, err := licenseservice.CurrentStatus()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(status.OEM)
}

func addLicenseDetail(context *gin.Context, operation string, stage string, reason string, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["action"] = strings.TrimSpace(context.PostForm("action"))
	if fields["action"] == "" && context.ContentType() != "multipart/form-data" {
		fields["action"] = "json_body"
	}
	logging.AddDetail(context, "license", operation, stage, reason, fields)
}

func licenseContentFieldName(context *gin.Context) string {
	if context != nil && context.ContentType() == "multipart/form-data" {
		return "license_file"
	}
	return "license_content"
}

func licenseErrorClass(err error) string {
	switch {
	case errors.Is(err, licenseservice.ErrExpired):
		return "expired"
	case errors.Is(err, licenseservice.ErrInactive):
		return "inactive"
	case errors.Is(err, licenseservice.ErrInvalid):
		return "invalid"
	case errors.Is(err, licenseservice.ErrLicenseKey):
		return "license_key_missing"
	case errors.Is(err, licenseservice.ErrNotYetValid):
		return "not_yet_valid"
	default:
		return "internal"
	}
}

func licenseErrorReason(err error) string {
	switch licenseErrorClass(err) {
	case "expired":
		return "license expired"
	case "inactive":
		return "license status is not active"
	case "invalid":
		return "license content is invalid or cannot be decoded/decrypted"
	case "license_key_missing":
		return "license_key is missing in decrypted license"
	case "not_yet_valid":
		return "license issued date is in the future"
	default:
		return "license registration failed"
	}
}

func isLicenseInputError(err error) bool {
	return errors.Is(err, licenseservice.ErrExpired) ||
		errors.Is(err, licenseservice.ErrInactive) ||
		errors.Is(err, licenseservice.ErrInvalid) ||
		errors.Is(err, licenseservice.ErrLicenseKey) ||
		errors.Is(err, licenseservice.ErrNotYetValid)
}
