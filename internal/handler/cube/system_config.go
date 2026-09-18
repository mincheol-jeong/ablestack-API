package cube

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"ablecloud.io/ablestack-api/internal/infra/utils"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"ablecloud.io/ablestack-api/internal/service/clusterconfig"
	"github.com/gin-gonic/gin"
)

type SystemConfigRequest = CubeModel.SystemConfigRequest
type SystemConfigResponse = CubeModel.SystemConfigResponse

// SystemConfigTargetResult는 원격 노드별 system config 적용 결과를 담는 응답 항목이다.
type SystemConfigTargetResult struct {
	Target  string `json:"target"`
	Code    int    `json:"code"`
	Val     any    `json:"val,omitempty"`
	Message string `json:"message,omitempty"`
}

// GetSystemConfig godoc
//
//	@Summary		System Config
//	@Description	systemProfile을 반환합니다.
//	@Tags			Cube-System
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	CubeModel.ClusterSystemProfile
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/system/config [get]
func GetSystemConfig(context *gin.Context) {
	root, err := loadClusterJSONRoot()
	if err != nil {
		context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
			ErrCode: http.StatusInternalServerError,
			Message: "failed to read cluster.json",
		})
		return
	}

	profile, err := extractSystemProfile(root)
	if err != nil {
		context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
			ErrCode: http.StatusInternalServerError,
			Message: "failed to read system profile",
		})
		return
	}

	context.JSON(http.StatusOK, profile)
}

// UpdateSystemConfig godoc
//
//	@Summary		System Config Update
//	@Description	systemProfile을 상태 조회/수정합니다.
//	@Tags			Cube-System
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CubeModel.SystemConfigRequest	true	"system config request"
//	@Success		200	{object}	CubeModel.SystemConfigResponse
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/system/config [post]
func UpdateSystemConfig(context *gin.Context) {
	var req SystemConfigRequest
	if err := context.ShouldBindJSON(&req); err != nil {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: "invalid request",
		})
		return
	}
	normalizeSystemConfigConvenienceRequest(&req)

	action := normalizeSystemAction(req.Action)
	if action == "" {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: "unsupported action",
		})
		return
	}

	root, err := loadClusterJSONRoot()
	if err != nil {
		context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
			ErrCode: http.StatusInternalServerError,
			Message: "failed to read cluster.json",
		})
		return
	}

	profile, err := ensureSystemProfileMap(root)
	if err != nil {
		context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
			ErrCode: http.StatusInternalServerError,
			Message: "failed to read system profile",
		})
		return
	}

	switch action {
	case "status":
		if normalizeSystemOption(req.Option) == "all" {
			targets, err := buildSystemTargetsFromRoot(root)
			if err != nil {
				context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
					ErrCode: http.StatusInternalServerError,
					Message: err.Error(),
				})
				return
			}
			if len(targets) == 0 {
				context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
					ErrCode: http.StatusBadRequest,
					Message: "no targets to apply",
				})
				return
			}

			remoteReq := req
			remoteReq.Option = ""
			results := applySystemStatusToTargets(targets, remoteReq)
			hasFail := false
			for _, res := range results {
				if res.Code != 200 {
					hasFail = true
					break
				}
			}
			val := map[string]any{
				"message": "status",
				"results": results,
			}
			resp := SystemConfigResponse{Code: 200, Val: val}
			if hasFail {
				resp.Code = 500
				resp.Val = map[string]any{
					"message": "status failed",
					"results": results,
				}
				context.JSON(http.StatusInternalServerError, resp)
				return
			}
			context.JSON(http.StatusOK, resp)
			return
		}
		val, err := buildSystemStatus(profile, req)
		if err != nil {
			context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
				ErrCode: http.StatusBadRequest,
				Message: err.Error(),
			})
			return
		}
		context.JSON(http.StatusOK, SystemConfigResponse{Code: 200, Val: val})
		return
	case "update":
		if err := updateSystemProfileValue(profile, req); err != nil {
			context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
				ErrCode: http.StatusBadRequest,
				Message: err.Error(),
			})
			return
		}
	case "allupdate":
		if err := applySystemAllUpdate(root, profile); err != nil {
			context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
				ErrCode: http.StatusInternalServerError,
				Message: err.Error(),
			})
			return
		}
	case "reset":
		applySystemReset(profile)
	}

	if err := saveClusterJSONRoot(root); err != nil {
		context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
			ErrCode: http.StatusInternalServerError,
			Message: "failed to save cluster.json",
		})
		return
	}

	if normalizeSystemOption(req.Option) == "all" {
		targets, err := buildSystemTargetsFromRoot(root)
		if err != nil {
			context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
				ErrCode: http.StatusInternalServerError,
				Message: err.Error(),
			})
			return
		}
		if len(targets) == 0 {
			context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
				ErrCode: http.StatusBadRequest,
				Message: "no targets to apply",
			})
			return
		}

		remoteReq := req
		remoteReq.Option = ""
		results := applySystemConfigToTargets(targets, remoteReq)
		hasFail := false
		for _, res := range results {
			if res.Code != 200 {
				hasFail = true
				break
			}
		}
		val := map[string]any{
			"message": "apply success",
			"results": results,
		}
		resp := SystemConfigResponse{Code: 200, Val: val}
		if hasFail {
			resp.Code = 500
			resp.Val = map[string]any{
				"message": "apply failed",
				"results": results,
			}
			context.JSON(http.StatusInternalServerError, resp)
			return
		}
		context.JSON(http.StatusOK, resp)
		return
	}

	context.JSON(http.StatusOK, SystemConfigResponse{Code: 200, Val: "ok"})
}

// normalizeSystemConfigConvenienceRequest는 UI의 완료 상태 액션을 표준 update fan-out 요청으로 변환한다.
func normalizeSystemConfigConvenienceRequest(req *SystemConfigRequest) {
	if req == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "gfs-configure", "gfs_configure":
		req.Action = "update"
		req.Option = "all"
		req.Depth1 = "bootstrap"
		req.Depth2 = "gfs_configure"
		if strings.TrimSpace(req.Value) == "" {
			req.Value = "true"
		}
	}
}

// normalizeSystemAction은 외부 입력 action을 내부 처리용 표준 값으로 정규화한다.
func normalizeSystemAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "status":
		return "status"
	case "update":
		return "update"
	case "allupdate":
		return "allupdate"
	case "reset":
		return "reset"
	default:
		return ""
	}
}

// normalizeSystemOption은 system config 요청 옵션을 내부 표준 값으로 정규화한다.
func normalizeSystemOption(option string) string {
	switch strings.ToLower(strings.TrimSpace(option)) {
	case "all":
		return "all"
	default:
		return ""
	}
}

// buildSystemStatus는 depth1/depth2 조건에 맞는 systemProfile 일부 또는 전체를 반환한다.
func buildSystemStatus(profile map[string]any, req SystemConfigRequest) (any, error) {
	if strings.TrimSpace(req.Depth1) == "" {
		return profile, nil
	}
	section, ok := profile[req.Depth1]
	if !ok {
		return nil, fmt.Errorf("invalid depth1")
	}
	if strings.TrimSpace(req.Depth2) == "" {
		return map[string]any{req.Depth1: section}, nil
	}
	sectionMap, ok := section.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid depth1")
	}
	val, ok := sectionMap[req.Depth2]
	if !ok {
		return nil, fmt.Errorf("invalid depth2")
	}
	return map[string]any{req.Depth2: val}, nil
}

// updateSystemProfileValue는 지정한 depth1/depth2 경로에 값을 반영한다.
func updateSystemProfileValue(profile map[string]any, req SystemConfigRequest) error {
	if strings.TrimSpace(req.Depth1) == "" {
		return fmt.Errorf("depth1 required")
	}
	if strings.TrimSpace(req.Depth2) == "" {
		return fmt.Errorf("depth2 required")
	}
	if strings.TrimSpace(req.Value) == "" {
		return fmt.Errorf("value required")
	}
	section := ensureMap(profile, req.Depth1)
	section[req.Depth2] = req.Value
	return nil
}

// applySystemAllUpdate는 클러스터 타입에 맞는 bootstrap 완료 상태를 일괄 반영한다.
func applySystemAllUpdate(root map[string]any, profile map[string]any) error {
	clusterType := extractClusterType(root)
	bootstrap := ensureMap(profile, "bootstrap")
	switch strings.ToLower(strings.TrimSpace(clusterType)) {
	case "ablestack-hci", "ablestack-hci-filesystem":
		bootstrap["scvm"] = "true"
	case "ablestack-vm":
		bootstrap["gfs_configure"] = "true"
	}
	bootstrap["ccvm"] = "true"
	bootstrap["wall"] = "true"
	return nil
}

// applySystemReset은 bootstrap 관련 상태를 모두 false로 초기화한다.
func applySystemReset(profile map[string]any) {
	bootstrap := ensureMap(profile, "bootstrap")
	bootstrap["scvm"] = "false"
	bootstrap["ccvm"] = "false"
	bootstrap["wall"] = "false"
	bootstrap["gfs_configure"] = "false"
	bootstrap["local_configure"] = "false"
}

// buildSystemTargetsFromRoot는 cluster.json 기준으로 system config 전파 대상 host 목록을 만든다.
func buildSystemTargetsFromRoot(root map[string]any) ([]string, error) {
	normalized := clusterconfig.NormalizeClusterJSON(root)
	rawCfg, ok := normalized["clusterConfig"]
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
	targetMap := map[string]struct{}{}
	for _, host := range cfg.Hosts {
		if strings.TrimSpace(host.Ablecube) == "" {
			continue
		}
		targetMap[host.Ablecube] = struct{}{}
	}
	targets := make([]string, 0, len(targetMap))
	for target := range targetMap {
		targets = append(targets, target)
	}
	return targets, nil
}

// applySystemConfigToTargets는 update/reset 요청을 각 대상 host에 순차 전파한다.
func applySystemConfigToTargets(targets []string, req SystemConfigRequest) []CubeModel.ClusterApplyResult {
	client := &http.Client{Timeout: 10 * time.Second}
	results := make([]CubeModel.ClusterApplyResult, 0, len(targets))
	for _, target := range targets {
		res := CubeModel.ClusterApplyResult{Target: target}
		if err := callSystemConfigRemote(client, target, req); err != nil {
			res.Code = 500
			res.Message = err.Error()
		} else {
			res.Code = 200
			res.Message = "ok"
		}
		results = append(results, res)
	}
	return results
}

// applySystemStatusToTargets는 status 요청을 각 대상 host에 보내고 결과를 모은다.
func applySystemStatusToTargets(targets []string, req SystemConfigRequest) []SystemConfigTargetResult {
	client := &http.Client{Timeout: 10 * time.Second}
	results := make([]SystemConfigTargetResult, 0, len(targets))
	for _, target := range targets {
		results = append(results, callSystemConfigRemoteStatus(client, target, req))
	}
	return results
}

// callSystemConfigRemote는 원격 host의 system config API를 호출한다.
func callSystemConfigRemote(client *http.Client, target string, req SystemConfigRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	baseURL := buildTargetURL(target)
	url := fmt.Sprintf("%s/api/v1/cube/system/config", baseURL)
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	attachInternalToken(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("system-config failed: %s", resp.Status)
	}
	return nil
}

// callSystemConfigRemoteStatus는 원격 status 응답을 HTTP 상태와 함께 구조화해서 반환한다.
func callSystemConfigRemoteStatus(client *http.Client, target string, req SystemConfigRequest) SystemConfigTargetResult {
	res := SystemConfigTargetResult{Target: target}
	body, err := json.Marshal(req)
	if err != nil {
		res.Code = 500
		res.Message = err.Error()
		return res
	}

	baseURL := buildTargetURL(target)
	url := fmt.Sprintf("%s/api/v1/cube/system/config", baseURL)
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		res.Code = 500
		res.Message = err.Error()
		return res
	}
	attachInternalToken(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		res.Code = 500
		res.Message = err.Error()
		return res
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	decoded := SystemConfigResponse{}
	if err := json.Unmarshal(raw, &decoded); err == nil {
		res.Code = decoded.Code
		res.Val = decoded.Val
		if resp.StatusCode >= 300 && res.Code == 0 {
			res.Code = resp.StatusCode
		}
		return res
	}

	res.Code = resp.StatusCode
	if len(raw) > 0 {
		res.Message = string(raw)
	} else if resp.StatusCode >= 300 {
		res.Message = resp.Status
	}
	return res
}

// saveClusterJSONRoot는 전체 cluster.json 문서를 정규화 후 파일로 저장한다.
func saveClusterJSONRoot(root map[string]any) error {
	path := resolveClusterJSONPath()
	root = clusterconfig.NormalizeClusterJSON(root)
	raw, err := json.MarshalIndent(root, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0644)
}

// ensureSystemProfileMap은 root에 systemProfile 맵이 항상 존재하도록 보장한다.
func ensureSystemProfileMap(root map[string]any) (map[string]any, error) {
	normalized := clusterconfig.NormalizeClusterJSON(root)
	raw, err := json.Marshal(normalized["systemProfile"])
	if err != nil {
		return nil, err
	}
	profile := map[string]any{}
	if err := json.Unmarshal(raw, &profile); err != nil {
		return nil, err
	}
	if profile == nil {
		profile = map[string]any{}
	}
	root["systemProfile"] = profile
	return profile, nil
}

// ensureMap은 부모 맵에 지정한 키의 하위 맵이 없으면 새로 만들어 반환한다.
func ensureMap(parent map[string]any, key string) map[string]any {
	if parent == nil {
		return map[string]any{}
	}
	if val, ok := parent[key].(map[string]any); ok {
		return val
	}
	newMap := map[string]any{}
	parent[key] = newMap
	return newMap
}
