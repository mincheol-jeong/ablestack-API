package cube

import (
	"net/http"

	"ablecloud.io/ablestack-api/internal/infra/utils"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"github.com/gin-gonic/gin"
)

// CreateCCVM godoc
//
//	@Summary		Create CCVM
//	@Description	제품 타입에 맞는 CCVM XML과 backing image를 준비하고 PCS 리소스를 비활성 상태로 생성합니다. 시작은 /cube/ccvm/lifecycle action=start를 사용합니다.
//	@Tags			Cube-CCVM
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CubeModel.CCVMCreateRequest	true	"ccvm create request"
//	@Success		200		{object}	CubeModel.CCVMCreateResponse
//	@Failure		400		{object}	HTTP400BadRequest
//	@Failure		500		{object}	HTTP500InternalServerError
//	@Router			/cube/ccvm/create [post]
func CreateCCVM(context *gin.Context) {
	req := CCVMXMLCreateRequest{}
	if err := context.ShouldBindJSON(&req); err != nil {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: "invalid request",
		})
		return
	}

	cfg, err := loadClusterConfigSection()
	if err != nil {
		context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
			ErrCode: http.StatusInternalServerError,
			Message: "failed to read cluster.json",
		})
		return
	}
	if err := normalizeCCVMXMLCreateRequest(&req, cfg); err != nil {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	xmlResp := runCreateCCVMXML(cfg, req)
	if xmlResp.Code != http.StatusOK {
		context.JSON(statusCodeFromCCVMXMLCreateResponse(xmlResp), CubeModel.CCVMCreateResponse{
			Code:    xmlResp.Code,
			Message: firstNonEmpty(xmlResp.Message, "ccvm xml create failed"),
			XML:     xmlResp,
		})
		return
	}

	setupResp := setupCCVMPCS(CCVMPCSControlRequest{Action: ccvmPCSSetupAction, CreateOnly: true}, cfg)
	if setupResp.Code != http.StatusOK {
		context.JSON(statusCodeFromCCVMPCSResponse(setupResp), CubeModel.CCVMCreateResponse{
			Code:    setupResp.Code,
			Message: firstNonEmpty(setupResp.Message, "ccvm resource create failed"),
			XML:     xmlResp,
			Setup:   setupResp,
		})
		return
	}

	context.JSON(http.StatusOK, CubeModel.CCVMCreateResponse{
		Code:    http.StatusOK,
		Message: "ccvm create success",
		XML:     xmlResp,
		Setup:   setupResp,
	})
}
