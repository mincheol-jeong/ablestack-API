package mold

import (
	"net/http"

	MoldModel "ablecloud.io/ablestack-api/internal/model/mold"
	"ablecloud.io/ablestack-api/internal/service/moldservice"
	"github.com/gin-gonic/gin"
)

// BootstrapPlan godoc
//
//	@Summary		Mold bootstrap plan
//	@Description	Mold 인프라 자동화 입력값을 검증하고 실행 순서, create command, 검증용 list command, 누락 입력값을 반환합니다. hosts, primary/secondary storage, VLAN은 cluster.json의 type/hosts/ccvm.ip에서 자동 계산하며 실제 인프라는 생성하지 않습니다.
//	@Tags			Mold
//	@Accept			json
//	@Produce		json
//	@Param			body	body		MoldModel.BootstrapRequest	true	"Mold bootstrap request"
//	@Success		200	{object}	MoldModel.Response
//	@Failure		400	{object}	MoldModel.Response
//	@Failure		403	{object}	MoldModel.Response
//	@Router			/mold/bootstrap/plan [post]
func BootstrapPlan(context *gin.Context) {
	var req MoldModel.BootstrapRequest
	if err := context.ShouldBindJSON(&req); err != nil {
		context.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}
	req, err := moldservice.PrepareBootstrapRequest(req)
	if err != nil {
		serviceError(context, err)
		return
	}
	ok(context, moldservice.BuildBootstrapPlan(req))
}
