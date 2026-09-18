package mold

import (
	"errors"
	"net/http"

	MoldModel "ablecloud.io/ablestack-api/internal/model/mold"
	"ablecloud.io/ablestack-api/internal/service/moldservice"
	"github.com/gin-gonic/gin"
)

const (
	moldBasePath = "/api/v1/mold"
)

type Response = MoldModel.Response
type NodeRoleStatus = MoldModel.NodeRoleStatus
type EndpointInfo = MoldModel.EndpointInfo
type RootStatus = MoldModel.RootStatus
type MoldAuthRequest = MoldModel.MoldAuthRequest
type MoldSessionValue = MoldModel.MoldSessionValue
type MoldCapabilitiesValue = MoldModel.MoldCapabilitiesValue

type routeSpec struct {
	Method      string
	Path        string
	Description string
	Status      string
}

func ok(context *gin.Context, val any) {
	context.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "ok", Val: val})
}

func serviceError(context *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, moldservice.ErrMissingCCVMIP):
		status = http.StatusBadRequest
	case errors.Is(err, moldservice.ErrMoldAPIRequest),
		errors.Is(err, moldservice.ErrMoldErrorResponse),
		errors.Is(err, moldservice.ErrInvalidMoldResponse),
		errors.Is(err, moldservice.ErrMissingMoldSession):
		status = http.StatusBadGateway
	}
	response := Response{
		Code:    status,
		Message: err.Error(),
	}
	if details := moldservice.ErrorDetails(err); len(details) > 0 {
		response.Val = details
	}
	context.JSON(status, response)
}
