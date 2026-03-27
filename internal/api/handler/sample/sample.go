package sample

import (
	"github.com/gin-gonic/gin"
	Sample "github.com/ycyun/Cube-API/internal/domain/model/sample"
	"net/http"
)

// GetStatus godoc
//
//	@Summary		Show Status of <<Sample>>
//	@Description	<<Sample>>의 상태값을 보여줍니다.
//	@Tags			API, <<Sample>>
//	@Accept			x-www-form-urlencoded
//	@Produce		json
//	@Success		200	{object}	Sample.TypeSampleStatus
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		404	{object}	HTTP404NotFound
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/Sample [get]
func GetStatus(ctx *gin.Context) {
	ctx.IndentedJSON(http.StatusOK, Sample.Status())
}
func Monitor() {
	Sample.UpdateStatus()
}
