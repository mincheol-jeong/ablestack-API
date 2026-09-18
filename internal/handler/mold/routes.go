package mold

import (
	"strings"

	"github.com/gin-gonic/gin"
)

var moldRoutes = []routeSpec{
	{Method: "GET", Path: "", Description: "Mold API registration and CCVM role status", Status: "active"},
	{Method: "GET", Path: "/status", Description: "Mold API registration and CCVM role status", Status: "active"},
	{Method: "POST", Path: "/session", Description: "create Mold API session using clusterConfig.ccvm.ip", Status: "active"},
	{Method: "GET", Path: "/capabilities", Description: "login and call Mold listCapabilities using clusterConfig.ccvm.ip", Status: "active"},
	{Method: "POST", Path: "/capabilities", Description: "login and call Mold listCapabilities using clusterConfig.ccvm.ip", Status: "active"},
	{Method: "POST", Path: "/bootstrap/plan", Description: "validate Mold bootstrap input and return ordered ensure/list verification plan", Status: "active"},
	{Method: "POST", Path: "/bootstrap", Description: "start Mold bootstrap as an async job", Status: "active"},
	{Method: "GET", Path: "/jobs", Description: "list recent Mold bootstrap jobs", Status: "active"},
	{Method: "GET", Path: "/jobs/:job_id", Description: "show Mold bootstrap job progress and final result", Status: "active"},
}

func RegisterRoutes(group *gin.RouterGroup) {
	group.Use(RequireCCVM())
	group.GET("", Status)
	group.GET("/status", Status)
	group.POST("/session", CreateSession)
	group.GET("/capabilities", GetCapabilities)
	group.POST("/capabilities", PostCapabilities)
	group.POST("/bootstrap/plan", BootstrapPlan)
	group.POST("/bootstrap", StartBootstrap)
	group.GET("/jobs", ListBootstrapJobs)
	group.GET("/jobs/:job_id", GetBootstrapJob)
}

func RegisterRoutesIfCCVM(group *gin.RouterGroup) bool {
	if !IsCCVMNode() {
		return false
	}
	RegisterRoutes(group)
	return true
}

func endpointInfos() []EndpointInfo {
	out := make([]EndpointInfo, 0, len(moldRoutes))
	for _, route := range moldRoutes {
		out = append(out, EndpointInfo{
			Method:      route.Method,
			Path:        moldBasePath + route.Path,
			Description: route.Description,
			Status:      firstNonEmpty(route.Status, "active"),
		})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
