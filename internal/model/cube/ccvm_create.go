package cube

// CCVMCreateRequest는 CCVM XML, backing image와 비활성 PCS 리소스 생성 요청 본문이다.
// @name CCVMCreateRequest
type CCVMCreateRequest = CCVMXMLCreateRequest

// CCVMCreateResponse는 CCVM 생성 결과이다.
// @name CCVMCreateResponse
type CCVMCreateResponse struct {
	Code    int                    `json:"code" example:"200"`
	Message string                 `json:"message,omitempty" example:"ccvm create success"`
	XML     CCVMXMLCreateResponse  `json:"xml"`
	Setup   CCVMPCSControlResponse `json:"setup"`
}
