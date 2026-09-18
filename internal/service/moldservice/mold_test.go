package moldservice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveMoldEndpointFromCluster(t *testing.T) {
	clusterPath := writeClusterJSON(t, `{
		"clusterConfig": {
			"ccvm": {"ip": "10.10.254.236"},
			"hosts": []
		}
	}`)
	t.Setenv("ABLESTACK_CLUSTER_JSON", clusterPath)
	t.Setenv("ABLESTACK_MOLD_API_SCHEME", "http")
	t.Setenv("ABLESTACK_MOLD_API_PORT", "8080")
	t.Setenv("ABLESTACK_MOLD_API_PATH", "/client/api")
	t.Setenv("ABLESTACK_MOLD_ENDPOINT", "")

	endpoint, ccvmIP, path, err := ResolveMoldEndpointFromCluster()
	if err != nil {
		t.Fatalf("ResolveMoldEndpointFromCluster error = %v", err)
	}
	if path != clusterPath {
		t.Fatalf("path = %q, want %q", path, clusterPath)
	}
	if ccvmIP != "10.10.254.236" {
		t.Fatalf("ccvmIP = %q", ccvmIP)
	}
	if endpoint != "http://10.10.254.236:8080/client/api" {
		t.Fatalf("endpoint = %q", endpoint)
	}
}

func TestClientLoginAndListCapabilities(t *testing.T) {
	var loginChecked bool
	var capabilitiesChecked bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm error = %v", err)
		}
		switch r.Form.Get("command") {
		case "login":
			loginChecked = true
			if r.Method != http.MethodPost {
				t.Fatalf("login method = %s", r.Method)
			}
			assertFormValue(t, r.Form, "username", "admin")
			assertFormValue(t, r.Form, "password", "password")
			assertFormValue(t, r.Form, "domain", "/")
			assertFormValue(t, r.Form, "response", "json")
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "mold-session-cookie", Path: "/"})
			writeJSON(t, w, map[string]any{
				"loginresponse": map[string]any{
					"sessionkey": "session-key",
				},
			})
		case "listCapabilities":
			capabilitiesChecked = true
			if r.Method != http.MethodGet {
				t.Fatalf("listCapabilities method = %s", r.Method)
			}
			assertFormValue(t, r.Form, "sessionkey", "session-key")
			assertFormValue(t, r.Form, "response", "json")
			cookie, err := r.Cookie("JSESSIONID")
			if err != nil {
				t.Fatalf("listCapabilities missing JSESSIONID cookie: %v", err)
			}
			if cookie.Value != "mold-session-cookie" {
				t.Fatalf("JSESSIONID = %q, want mold-session-cookie", cookie.Value)
			}
			writeJSON(t, w, map[string]any{
				"listcapabilitiesresponse": map[string]any{
					"capability": map[string]any{
						"securitygroupsenabled": true,
					},
				},
			})
		default:
			t.Fatalf("unexpected command %q", r.Form.Get("command"))
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, WithCCVMIP("10.10.254.236"))
	result, err := client.LoginAndListCapabilities(context.Background(), AuthConfig{})
	if err != nil {
		t.Fatalf("LoginAndListCapabilities error = %v", err)
	}
	if result.SessionKey != "session-key" {
		t.Fatalf("sessionkey = %q", result.SessionKey)
	}
	if result.Endpoint != server.URL {
		t.Fatalf("endpoint = %q", result.Endpoint)
	}
	if result.CCVMIP != "10.10.254.236" {
		t.Fatalf("ccvmIP = %q", result.CCVMIP)
	}
	if _, ok := result.Capabilities["listcapabilitiesresponse"]; !ok {
		t.Fatalf("capabilities missing listcapabilitiesresponse: %#v", result.Capabilities)
	}
	if !loginChecked || !capabilitiesChecked {
		t.Fatalf("loginChecked=%v capabilitiesChecked=%v", loginChecked, capabilitiesChecked)
	}
}

func TestClientCallCommandUsesStoredSessionKey(t *testing.T) {
	var checked bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm error = %v", err)
		}
		switch r.Form.Get("command") {
		case "login":
			writeJSON(t, w, map[string]any{
				"loginresponse": map[string]any{"sessionkey": "stored-session-key"},
			})
		case "listZones":
			checked = true
			assertFormValue(t, r.Form, "sessionkey", "stored-session-key")
			writeJSON(t, w, map[string]any{
				"listzonesresponse": map[string]any{"count": 0},
			})
		default:
			t.Fatalf("unexpected command %q", r.Form.Get("command"))
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	if _, err := client.Login(context.Background(), AuthConfig{}); err != nil {
		t.Fatalf("Login error = %v", err)
	}
	if _, err := client.CallCommand(context.Background(), http.MethodGet, "listZones", "", nil); err != nil {
		t.Fatalf("CallCommand error = %v", err)
	}
	if !checked {
		t.Fatalf("listZones was not called")
	}
}

func TestClientDetectsNestedMoldError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"listcapabilitiesresponse": map[string]any{
				"errorcode": 401,
				"errortext": "unable to verify user credentials and/or request signature",
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.ListCapabilities(context.Background(), "bad-session")
	if !errors.Is(err, ErrMoldErrorResponse) {
		t.Fatalf("ListCapabilities error = %v, want ErrMoldErrorResponse", err)
	}
}

func TestClientDetectsNestedMoldErrorFromHTTP401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		writeJSON(t, w, map[string]any{
			"listcapabilitiesresponse": map[string]any{
				"errorcode": 401,
				"errortext": "unable to verify user credentials and/or request signature",
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.ListCapabilities(context.Background(), "bad-session")
	if !errors.Is(err, ErrMoldAPIRequest) {
		t.Fatalf("ListCapabilities error = %v, want ErrMoldAPIRequest", err)
	}
	if !strings.Contains(err.Error(), "unable to verify user credentials") {
		t.Fatalf("ListCapabilities error = %v, want parsed errortext", err)
	}
	details := ErrorDetails(err)
	if details["http_status"] != http.StatusUnauthorized {
		t.Fatalf("http_status detail = %#v", details["http_status"])
	}
	if details["error_text"] != "unable to verify user credentials and/or request signature" {
		t.Fatalf("error_text detail = %#v", details["error_text"])
	}
	if details["sessionkey_attached"] != true {
		t.Fatalf("sessionkey_attached detail = %#v", details["sessionkey_attached"])
	}
	if _, ok := details["hint"].(string); !ok {
		t.Fatalf("missing hint detail: %#v", details)
	}
}

func writeClusterJSON(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cluster.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write cluster.json: %v", err)
	}
	return path
}

func assertFormValue(t *testing.T, form url.Values, key string, want string) {
	t.Helper()
	if got := form.Get(key); got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
