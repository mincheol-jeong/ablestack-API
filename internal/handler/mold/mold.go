package mold

import (
	"net/http"

	MoldModel "ablecloud.io/ablestack-api/internal/model/mold"
	"ablecloud.io/ablestack-api/internal/service/moldservice"
	"github.com/gin-gonic/gin"
)

// CreateSession godoc
//
//	@Summary		Mold session 생성
//	@Description	cluster.json의 clusterConfig.ccvm.ip로 Mold endpoint를 계산한 뒤 login command를 호출해 sessionkey를 반환합니다. 요청 body/form이 비어 있으면 admin/password, domain=/, response=json 기본값을 사용합니다.
//	@Tags			Mold
//	@Accept			json
//	@Accept			x-www-form-urlencoded
//	@Produce		json
//	@Param			body	body		MoldModel.MoldAuthRequest	false	"Mold login override"
//	@Success		200	{object}	MoldModel.Response
//	@Failure		400	{object}	MoldModel.Response
//	@Failure		403	{object}	MoldModel.Response
//	@Failure		502	{object}	MoldModel.Response
//	@Router			/mold/session [post]
func CreateSession(context *gin.Context) {
	auth, bound := bindAuthConfig(context)
	if !bound {
		return
	}
	client, err := moldservice.NewClientFromCluster()
	if err != nil {
		serviceError(context, err)
		return
	}
	session, err := client.Login(context.Request.Context(), auth)
	if err != nil {
		serviceError(context, err)
		return
	}
	okResponse := MoldModel.MoldSessionValue{
		Endpoint:      session.Endpoint,
		CCVMIP:        session.CCVMIP,
		SessionKey:    session.SessionKey,
		LoginResponse: session.LoginResponse,
	}
	ok(context, okResponse)
}

// GetCapabilities godoc
//
//	@Summary		Mold capabilities 조회
//	@Description	cluster.json의 clusterConfig.ccvm.ip로 Mold 기본 인증값(admin/password, domain=/)으로 login 후 listCapabilities command를 호출합니다. 인증값 override가 필요하면 POST /mold/capabilities를 사용합니다.
//	@Tags			Mold
//	@Produce		json
//	@Success		200	{object}	MoldModel.Response
//	@Failure		403	{object}	MoldModel.Response
//	@Failure		502	{object}	MoldModel.Response
//	@Router			/mold/capabilities [get]
func GetCapabilities(context *gin.Context) {
	listCapabilities(context, moldservice.AuthConfig{})
}

// PostCapabilities godoc
//
//	@Summary		Mold capabilities 조회
//	@Description	cluster.json의 clusterConfig.ccvm.ip로 Mold login 후 listCapabilities command를 호출합니다. 요청 body/form이 비어 있으면 admin/password, domain=/, response=json 기본값을 사용합니다.
//	@Tags			Mold
//	@Accept			json
//	@Accept			x-www-form-urlencoded
//	@Produce		json
//	@Param			body	body		MoldModel.MoldAuthRequest	false	"Mold login override"
//	@Success		200	{object}	MoldModel.Response
//	@Failure		400	{object}	MoldModel.Response
//	@Failure		403	{object}	MoldModel.Response
//	@Failure		502	{object}	MoldModel.Response
//	@Router			/mold/capabilities [post]
func PostCapabilities(context *gin.Context) {
	auth, bound := bindAuthConfig(context)
	if !bound {
		return
	}
	listCapabilities(context, auth)
}

func listCapabilities(context *gin.Context, auth moldservice.AuthConfig) {
	client, err := moldservice.NewClientFromCluster()
	if err != nil {
		serviceError(context, err)
		return
	}
	result, err := client.LoginAndListCapabilities(context.Request.Context(), auth)
	if err != nil {
		serviceError(context, err)
		return
	}
	ok(context, MoldModel.MoldCapabilitiesValue{
		Endpoint:     result.Endpoint,
		CCVMIP:       result.CCVMIP,
		SessionKey:   result.SessionKey,
		Capabilities: result.Capabilities,
	})
}

func bindAuthConfig(context *gin.Context) (moldservice.AuthConfig, bool) {
	var req MoldAuthRequest
	if context.Request.Method != http.MethodGet && context.Request.ContentLength != 0 {
		if err := context.ShouldBind(&req); err != nil {
			context.JSON(http.StatusBadRequest, Response{
				Code:    http.StatusBadRequest,
				Message: err.Error(),
			})
			return moldservice.AuthConfig{}, false
		}
	}
	return moldservice.AuthConfig{
		Username: req.Username,
		Password: req.Password,
		Domain:   req.Domain,
		Response: req.Response,
	}, true
}
