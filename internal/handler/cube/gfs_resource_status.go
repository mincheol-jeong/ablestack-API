package cube

import (
	"fmt"
	"net/http"
	"time"

	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"github.com/gin-gonic/gin"
)

type GFSResourceStatusResponse = CubeModel.GFSResourceStatusResponse

const gfsResourceStatusCommandTimeout = 10 * time.Second

// GetGFSResourceStatus godoc
//
//	@Summary		GFS Resource Status
//	@Description	pcs status xml 기반 펜싱, 잠금 및 glue-gfs/glue-gfs_res GFS2 마운트 리소스 상태를 조회합니다.
//	@Tags			Cube-GFS
//	@Accept			x-www-form-urlencoded
//	@Produce		json
//	@Success		200	{object}	CubeModel.GFSResourceStatusResponse
//	@Failure		500	{object}	CubeModel.GFSResourceStatusResponse
//	@Router			/cube/gfs/resource/status [get]
func GetGFSResourceStatus(context *gin.Context) {
	status, err := loadGFSResourceStatus()
	if err != nil {
		context.JSON(http.StatusInternalServerError, GFSResourceStatusResponse{
			Code: http.StatusInternalServerError,
			Val:  fmt.Sprintf("PCS Not Configured: %s", err.Error()),
		})
		return
	}

	context.IndentedJSON(http.StatusOK, GFSResourceStatusResponse{
		Code: http.StatusOK,
		Val:  status,
	})
}

func loadGFSResourceStatus() (CubeModel.GFSResourceStatusValue, error) {
	out, err := runPCSCommand(gfsResourceStatusCommandTimeout, "pcs", "status", "xml")
	if err != nil {
		return CubeModel.GFSResourceStatusValue{}, err
	}
	return CubeModel.ParseGFSResourceStatusXML([]byte(out))
}
