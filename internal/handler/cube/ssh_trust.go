package cube

import (
	"net/http"
	"strings"

	"ablecloud.io/ablestack-api/internal/infra/utils"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"ablecloud.io/ablestack-api/internal/service/sshtrust"
	"github.com/gin-gonic/gin"
)

// SSHTrust godoc
//
//	@Summary		CloudStack Management SSH Trust
//	@Description	/root/.ssh/authorized_keys에서 cloud@ccvm 식별 공개키만 멱등적으로 등록, 조회 또는 제거합니다.
//	@Tags			Cube-SSH
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CubeModel.SSHTrustRequest	true	"ssh trust request"
//	@Success		200		{object}	CubeModel.SSHTrustResponse
//	@Failure		400		{object}	HTTP400BadRequest
//	@Failure		500		{object}	HTTP500InternalServerError
//	@Router			/cube/ssh/trust [post]
func SSHTrust(context *gin.Context) {
	var req CubeModel.SSHTrustRequest
	if err := context.ShouldBindJSON(&req); err != nil {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{ErrCode: http.StatusBadRequest, Message: "invalid request"})
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == "" {
		action = "ensure"
	}
	identifier := strings.TrimSpace(req.Identifier)
	if identifier == "" {
		identifier = sshtrust.DefaultIdentifier
	}
	if identifier != sshtrust.DefaultIdentifier {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{ErrCode: http.StatusBadRequest, Message: "identifier must be cloud@ccvm"})
		return
	}

	var result sshtrust.Result
	var err error
	switch action {
	case "ensure":
		if strings.TrimSpace(req.PublicKey) == "" {
			context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{ErrCode: http.StatusBadRequest, Message: "public_key required"})
			return
		}
		result, err = sshtrust.EnsureAuthorizedKey(sshtrust.DefaultAuthorizedKeysPath, req.PublicKey, identifier)
	case "status":
		result, err = sshtrust.Status(sshtrust.DefaultAuthorizedKeysPath, identifier)
	case "remove":
		result, err = sshtrust.RemoveAuthorizedKey(sshtrust.DefaultAuthorizedKeysPath, identifier)
	default:
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{ErrCode: http.StatusBadRequest, Message: "action must be ensure, status or remove"})
		return
	}
	if err != nil {
		context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{ErrCode: http.StatusInternalServerError, Message: err.Error()})
		return
	}
	message := "ssh trust key " + action + " complete"
	context.JSON(http.StatusOK, CubeModel.SSHTrustResponse{
		Code: http.StatusOK, Action: action, Identifier: identifier, Message: message,
		Present: result.Present, Changed: result.Changed, Fingerprint: result.Fingerprint,
	})
}
