package cube

// SSHTrustRequest manages one identified public key in the local root account.
// @name SSHTrustRequest
type SSHTrustRequest struct {
	// action: ensure/status/remove
	Action string `json:"action" example:"ensure"`
	// OpenSSH public key. Required for ensure.
	PublicKey string `json:"public_key,omitempty" example:"ecdsa-sha2-nistp256 AAAA... cloud@ccvm"`
	// authorized_keys comment used as the stable registration identifier.
	Identifier string `json:"identifier,omitempty" example:"cloud@ccvm"`
}

// SSHTrustResponse describes the local authorized_keys change.
// @name SSHTrustResponse
type SSHTrustResponse struct {
	Code        int    `json:"code" example:"200"`
	Action      string `json:"action" example:"ensure"`
	Identifier  string `json:"identifier" example:"cloud@ccvm"`
	Message     string `json:"message" example:"ssh trust key ensured"`
	Present     bool   `json:"present"`
	Changed     bool   `json:"changed"`
	Fingerprint string `json:"fingerprint,omitempty" example:"SHA256:..."`
}
