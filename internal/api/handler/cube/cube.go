package cube

import (
	"net/http"

	"github.com/gin-gonic/gin"
	Cube "github.com/ycyun/Cube-API/internal/domain/model/cube"
	utils "github.com/ycyun/Cube-API/internal/infra/utils"
)

var _ = utils.TypeVersion{}

// Version godoc
//
//	@Summary		Show Versions of CUBE
//	@Description	CUBE 의 버전을 보여줍니다.
//	@Tags			API, CUBE
//	@Accept			x-www-form-urlencoded
//	@Produce		json
//	@Success		200	{object}	utils.TypeVersion
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		404	{object}	HTTP404NotFound
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/version [get]
func Version(context *gin.Context) {
	dat := Cube.Cube().GetVersion()
	// Print the output
	dat.Debug = gin.IsDebugging()
	context.IndentedJSON(http.StatusOK, dat)
} // @name Version
