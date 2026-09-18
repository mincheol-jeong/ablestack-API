package moldservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultMoldUsername = "admin"
	defaultMoldPassword = "password"
	defaultMoldDomain   = "/"
	defaultMoldResponse = "json"
	defaultHTTPTimeout  = 30 * time.Second
	maxMoldResponseBody = 10 << 20
)

var (
	ErrMissingCCVMIP       = errors.New("clusterConfig.ccvm.ip required")
	ErrMoldAPIRequest      = errors.New("mold api request failed")
	ErrMoldErrorResponse   = errors.New("mold api error response")
	ErrInvalidMoldResponse = errors.New("invalid mold api response")
	ErrMissingMoldSession  = errors.New("mold sessionkey not found")
)

type AuthConfig struct {
	Username string
	Password string
	Domain   string
	Response string
}

type Client struct {
	endpoint   string
	ccvmIP     string
	httpClient *http.Client
	sessionKey string
}

type Session struct {
	Endpoint      string
	CCVMIP        string
	SessionKey    string
	LoginResponse map[string]any
}

type CapabilitiesResult struct {
	Endpoint     string
	CCVMIP       string
	SessionKey   string
	Capabilities map[string]any
}

type APIError struct {
	Kind          error
	Method        string
	Endpoint      string
	Command       string
	Code          int
	ErrorCode     string
	ErrorText     string
	Raw           any
	HTTPStatus    int
	HTTPBody      string
	VerifyCommand string
	SessionKeySet bool
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	message := firstNonEmpty(e.ErrorText, e.HTTPBody, "unknown mold error")
	if e.Kind != nil {
		return fmt.Sprintf("%v: %s", e.Kind, message)
	}
	return message
}

func (e *APIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Kind
}

type Option func(*Client)

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.httpClient = client
		}
	}
}

func WithCCVMIP(ip string) Option {
	return func(c *Client) {
		c.ccvmIP = strings.TrimSpace(ip)
	}
}

func DefaultAuthConfig() AuthConfig {
	return AuthConfig{
		Username: defaultMoldUsername,
		Password: defaultMoldPassword,
		Domain:   defaultMoldDomain,
		Response: defaultMoldResponse,
	}
}

func (a AuthConfig) WithDefaults() AuthConfig {
	defaults := DefaultAuthConfig()
	if strings.TrimSpace(a.Username) == "" {
		a.Username = defaults.Username
	}
	if strings.TrimSpace(a.Password) == "" {
		a.Password = defaults.Password
	}
	if strings.TrimSpace(a.Domain) == "" {
		a.Domain = defaults.Domain
	}
	if strings.TrimSpace(a.Response) == "" {
		a.Response = defaults.Response
	}
	return a
}

func NewClient(endpoint string, options ...Option) *Client {
	jar, _ := cookiejar.New(nil)
	client := &Client{
		endpoint: strings.TrimSpace(endpoint),
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
			Jar:     jar,
		},
	}
	for _, option := range options {
		if option != nil {
			option(client)
		}
	}
	return client
}

func NewClientFromCluster() (*Client, error) {
	endpoint, ccvmIP, _, err := ResolveMoldEndpointFromCluster()
	if err != nil {
		return nil, err
	}
	return NewClient(endpoint, WithCCVMIP(ccvmIP)), nil
}

func (c *Client) Endpoint() string {
	if c == nil {
		return ""
	}
	return c.endpoint
}

func (c *Client) CCVMIP() string {
	if c == nil {
		return ""
	}
	return c.ccvmIP
}

func (c *Client) SessionKey() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.sessionKey)
}

func (c *Client) Login(ctx context.Context, auth AuthConfig) (Session, error) {
	if c == nil || strings.TrimSpace(c.endpoint) == "" {
		return Session{}, fmt.Errorf("%w: endpoint required", ErrInvalidMoldResponse)
	}
	auth = auth.WithDefaults()
	values := url.Values{}
	values.Set("command", "login")
	values.Set("username", strings.TrimSpace(auth.Username))
	values.Set("password", auth.Password)
	values.Set("domain", strings.TrimSpace(auth.Domain))
	values.Set("response", strings.TrimSpace(auth.Response))

	body, err := c.call(ctx, http.MethodPost, values)
	if err != nil {
		return Session{}, err
	}
	loginResponse, ok := body["loginresponse"].(map[string]any)
	if !ok {
		return Session{}, fmt.Errorf("%w: loginresponse missing", ErrInvalidMoldResponse)
	}
	sessionKey := stringValue(loginResponse["sessionkey"])
	if sessionKey == "" {
		return Session{}, ErrMissingMoldSession
	}
	c.sessionKey = sessionKey
	return Session{
		Endpoint:      c.endpoint,
		CCVMIP:        c.ccvmIP,
		SessionKey:    sessionKey,
		LoginResponse: body,
	}, nil
}

func (c *Client) ListCapabilities(ctx context.Context, sessionKey string) (map[string]any, error) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		sessionKey = c.SessionKey()
	}
	if sessionKey == "" {
		return nil, ErrMissingMoldSession
	}
	values := url.Values{}
	values.Set("command", "listCapabilities")
	values.Set("response", "json")
	values.Set("sessionkey", sessionKey)
	return c.call(ctx, http.MethodGet, values)
}

func (c *Client) CallCommand(ctx context.Context, method string, command string, sessionKey string, params url.Values) (map[string]any, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, fmt.Errorf("%w: command required", ErrInvalidMoldResponse)
	}
	if params == nil {
		params = url.Values{}
	} else {
		params = cloneValues(params)
	}
	params.Set("command", command)
	if params.Get("response") == "" {
		params.Set("response", "json")
	}
	if strings.TrimSpace(sessionKey) != "" {
		params.Set("sessionkey", strings.TrimSpace(sessionKey))
	} else if storedSessionKey := c.SessionKey(); storedSessionKey != "" {
		params.Set("sessionkey", storedSessionKey)
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	return c.call(ctx, method, params)
}

func (c *Client) LoginAndListCapabilities(ctx context.Context, auth AuthConfig) (CapabilitiesResult, error) {
	session, err := c.Login(ctx, auth)
	if err != nil {
		return CapabilitiesResult{}, err
	}
	capabilities, err := c.ListCapabilities(ctx, "")
	if err != nil {
		return CapabilitiesResult{}, err
	}
	return CapabilitiesResult{
		Endpoint:     session.Endpoint,
		CCVMIP:       session.CCVMIP,
		SessionKey:   session.SessionKey,
		Capabilities: capabilities,
	}, nil
}

func (c *Client) call(ctx context.Context, method string, values url.Values) (map[string]any, error) {
	endpoint := strings.TrimSpace(c.endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("%w: endpoint required", ErrInvalidMoldResponse)
	}
	command := strings.TrimSpace(values.Get("command"))

	var body io.Reader
	requestURL := endpoint
	if method == http.MethodPost {
		body = strings.NewReader(values.Encode())
	} else {
		separator := "?"
		if strings.Contains(requestURL, "?") {
			separator = "&"
		}
		requestURL += separator + values.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, err
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Accept", "application/json")

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMoldAPIRequest, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxMoldResponseBody))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out := map[string]any{}
		if err := json.Unmarshal(raw, &out); err == nil {
			if apiErr := extractMoldAPIError(command, out); apiErr != nil {
				apiErr.Kind = ErrMoldAPIRequest
				apiErr.HTTPStatus = resp.StatusCode
				apiErr.HTTPBody = truncateBody(raw)
				enrichAPIError(apiErr, method, endpoint, values)
				return nil, apiErr
			}
		}
		return nil, &APIError{
			Kind:          ErrMoldAPIRequest,
			Method:        method,
			Endpoint:      endpoint,
			Command:       command,
			HTTPStatus:    resp.StatusCode,
			HTTPBody:      truncateBody(raw),
			ErrorText:     truncateBody(raw),
			SessionKeySet: strings.TrimSpace(values.Get("sessionkey")) != "",
		}
	}

	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidMoldResponse, err)
	}
	if apiErr := extractMoldAPIError(command, out); apiErr != nil {
		enrichAPIError(apiErr, method, endpoint, values)
		return nil, apiErr
	}
	return out, nil
}

func moldEndpointOverride() string {
	return strings.TrimSpace(os.Getenv("ABLESTACK_MOLD_ENDPOINT"))
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case float64:
		return strings.TrimSpace(fmt.Sprintf("%.0f", typed))
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func APIErrorFrom(err error) (*APIError, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr != nil {
		return apiErr, true
	}
	return nil, false
}

func ErrorDetails(err error) map[string]any {
	if err == nil {
		return nil
	}
	details := map[string]any{
		"reason": err.Error(),
	}
	if apiErr, ok := APIErrorFrom(err); ok {
		setDetail(details, "type", errorName(apiErr.Kind))
		setDetail(details, "method", apiErr.Method)
		setDetail(details, "endpoint", apiErr.Endpoint)
		setDetail(details, "command", apiErr.Command)
		setDetail(details, "verify_command", apiErr.VerifyCommand)
		setDetail(details, "http_status", apiErr.HTTPStatus)
		setDetail(details, "error_code", apiErr.ErrorCode)
		setDetail(details, "error_text", apiErr.ErrorText)
		setDetail(details, "http_body", apiErr.HTTPBody)
		details["sessionkey_attached"] = apiErr.SessionKeySet
		if apiErr.Raw != nil {
			details["raw"] = apiErr.Raw
		}
		if hint := apiErrorHint(apiErr); hint != "" {
			details["hint"] = hint
		}
		return details
	}
	setDetail(details, "type", errorName(errors.Unwrap(err)))
	return details
}

func cloneValues(values url.Values) url.Values {
	out := make(url.Values, len(values))
	for key, list := range values {
		copied := make([]string, len(list))
		copy(copied, list)
		out[key] = copied
	}
	return out
}

func extractMoldAPIError(command string, body map[string]any) *APIError {
	if errResponse, ok := body["errorresponse"].(map[string]any); ok {
		return apiErrorFromMap(command, errResponse, body)
	}
	if nested := findNestedMoldError(body); nested != nil {
		return apiErrorFromMap(command, nested, body)
	}
	return nil
}

func findNestedMoldError(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		errorText := stringValue(typed["errortext"])
		errorCode := stringValue(typed["errorcode"])
		if errorText != "" || errorCode != "" {
			return typed
		}
		for _, child := range typed {
			if nested := findNestedMoldError(child); nested != nil {
				return nested
			}
		}
	case []any:
		for _, child := range typed {
			if nested := findNestedMoldError(child); nested != nil {
				return nested
			}
		}
	}
	return nil
}

func apiErrorFromMap(command string, errResponse map[string]any, raw any) *APIError {
	errorCode := stringValue(errResponse["errorcode"])
	errorText := firstNonEmpty(
		stringValue(errResponse["errortext"]),
		errorCode,
		stringValue(errResponse["uuidList"]),
		"unknown mold error",
	)
	return &APIError{
		Kind:      ErrMoldErrorResponse,
		Command:   command,
		Code:      intValue(errResponse["errorcode"]),
		ErrorCode: errorCode,
		ErrorText: errorText,
		Raw:       raw,
	}
}

func enrichAPIError(apiErr *APIError, method string, endpoint string, values url.Values) {
	if apiErr == nil {
		return
	}
	apiErr.Method = strings.ToUpper(strings.TrimSpace(method))
	apiErr.Endpoint = strings.TrimSpace(endpoint)
	apiErr.SessionKeySet = strings.TrimSpace(values.Get("sessionkey")) != ""
}

func setDetail(details map[string]any, key string, value any) {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) != "" {
			details[key] = typed
		}
	case int:
		if typed != 0 {
			details[key] = typed
		}
	case bool:
		details[key] = typed
	case nil:
	default:
		details[key] = typed
	}
}

func errorName(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func apiErrorHint(err *APIError) string {
	if err == nil {
		return ""
	}
	switch {
	case err.HTTPStatus == http.StatusUnauthorized || err.ErrorCode == "401":
		if err.SessionKeySet {
			return "Mold rejected the generated sessionkey/cookie. Check whether the admin account is still in first-login state, the password is correct, the Mold session cookie is accepted, and the CloudStack API allows sessionkey authentication."
		}
		return "Mold rejected the request without an attached sessionkey. The client should login first or use an authenticated command path."
	case errors.Is(err.Kind, ErrMoldAPIRequest):
		return "The Mold HTTP API returned a non-success response. Inspect http_status, error_code, error_text, and raw for the CloudStack-side cause."
	case errors.Is(err.Kind, ErrMoldErrorResponse):
		return "The Mold API returned a command-level error. Inspect command, error_code, error_text, and raw."
	default:
		return ""
	}
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		out, _ := typed.Int64()
		return int(out)
	case string:
		var out int
		_, _ = fmt.Sscanf(strings.TrimSpace(typed), "%d", &out)
		return out
	default:
		return 0
	}
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

func truncateBody(raw []byte) string {
	const max = 1024
	value := strings.TrimSpace(string(raw))
	if len(value) <= max {
		return value
	}
	return value[:max] + "..."
}
