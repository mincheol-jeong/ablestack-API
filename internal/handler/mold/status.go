package mold

import (
	MoldModel "ablecloud.io/ablestack-api/internal/model/mold"
	"github.com/gin-gonic/gin"
)

// Status godoc
//
//	@Summary		Mold API 상태
//	@Description	CCVM 전용 Mold API 등록 상태와 현재 노드의 CCVM 판정 결과를 반환합니다.
//	@Tags			Mold
//	@Produce		json
//	@Success		200	{object}	MoldModel.Response
//	@Failure		403	{object}	MoldModel.Response
//	@Router			/mold [get]
func Status(context *gin.Context) {
	ok(context, MoldModel.RootStatus{
		Name:      "mold",
		CCVMOnly:  true,
		Node:      DetectNodeRole(),
		Endpoints: endpointInfos(),
	})
}
