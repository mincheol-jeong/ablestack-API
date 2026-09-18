package mold

// Response is the common Mold API response body.
// @name MoldResponse
type Response struct {
	Code    int    `json:"code" example:"200"`
	Message string `json:"message,omitempty" example:"ok"`
	Val     any    `json:"val,omitempty"`
}

// NodeRoleStatus describes current node role detection for Mold route guards.
// @name MoldNodeRoleStatus
type NodeRoleStatus struct {
	Role   string `json:"role" example:"ccvm"`
	Source string `json:"source" example:"env"`
	CCVM   bool   `json:"ccvm" example:"true"`
	Reason string `json:"reason,omitempty" example:"ABLESTACK_NODE_ROLE=ccvm"`
}

// EndpointInfo describes a registered Mold endpoint.
// @name MoldEndpointInfo
type EndpointInfo struct {
	Method      string `json:"method" example:"POST"`
	Path        string `json:"path" example:"/api/v1/mold/session"`
	Description string `json:"description,omitempty" example:"create Mold API session"`
	Status      string `json:"status" example:"active"`
}

// RootStatus is returned by the Mold API root endpoint.
// @name MoldRootStatus
type RootStatus struct {
	Name      string         `json:"name" example:"mold"`
	CCVMOnly  bool           `json:"ccvm_only" example:"true"`
	Node      NodeRoleStatus `json:"node"`
	Endpoints []EndpointInfo `json:"endpoints"`
}

// MoldAuthRequest contains optional Mold login overrides.
// Empty fields use product defaults: admin/password, domain=/, response=json.
// @name MoldAuthRequest
type MoldAuthRequest struct {
	Username string `json:"username,omitempty" form:"username" example:"admin"`
	Password string `json:"password,omitempty" form:"password" example:"password"`
	Domain   string `json:"domain,omitempty" form:"domain" example:"/"`
	Response string `json:"response,omitempty" form:"response" example:"json"`
}

// MoldSessionValue is returned after Mold login.
// @name MoldSessionValue
type MoldSessionValue struct {
	Endpoint      string         `json:"endpoint" example:"http://10.10.254.236:8080/client/api"`
	CCVMIP        string         `json:"ccvm_ip" example:"10.10.254.236"`
	SessionKey    string         `json:"sessionkey" example:"session-key"`
	LoginResponse map[string]any `json:"login_response,omitempty"`
}

// MoldCapabilitiesValue contains a fresh session key and listCapabilities response.
// @name MoldCapabilitiesValue
type MoldCapabilitiesValue struct {
	Endpoint     string         `json:"endpoint" example:"http://10.10.254.236:8080/client/api"`
	CCVMIP       string         `json:"ccvm_ip" example:"10.10.254.236"`
	SessionKey   string         `json:"sessionkey" example:"session-key"`
	Capabilities map[string]any `json:"capabilities"`
}
