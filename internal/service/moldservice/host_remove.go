package moldservice

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultHostRemoveAsyncTimeout = 2 * time.Hour

// RemoveHostResult는 CloudStack Host 삭제 및 listHosts 검증 결과이다.
type RemoveHostResult struct {
	HostID              string         `json:"host_id"`
	Host                map[string]any `json:"host,omitempty"`
	MaintenanceResponse map[string]any `json:"maintenance_response,omitempty"`
	MaintenanceVerify   map[string]any `json:"maintenance_verify_response,omitempty"`
	DeleteResponse      map[string]any `json:"delete_response,omitempty"`
	VerifyResponse      map[string]any `json:"verify_response,omitempty"`
}

// RemoveHost는 이름/IP로 Host를 찾고 deleteHost 완료 후 listHosts에서 사라졌는지 검증한다.
func RemoveHost(ctx context.Context, auth AuthConfig, hostname string, hostIP string, force bool) (RemoveHostResult, error) {
	client, err := NewClientFromCluster()
	if err != nil {
		return RemoveHostResult{}, err
	}
	session, err := client.Login(ctx, auth)
	if err != nil {
		return RemoveHostResult{}, err
	}
	list, err := client.CallCommand(ctx, http.MethodGet, "listHosts", session.SessionKey, url.Values{"listall": {"true"}})
	if err != nil {
		return RemoveHostResult{}, err
	}
	host := findCloudStackHost(list, hostname, hostIP)
	if host == nil {
		return RemoveHostResult{VerifyResponse: list}, nil
	}
	hostID := stringValue(host["id"])
	if hostID == "" {
		return RemoveHostResult{}, fmt.Errorf("listHosts result has no host id for %s", hostname)
	}
	result := RemoveHostResult{HostID: hostID, Host: host}
	resourceState := strings.TrimSpace(stringValue(host["resourcestate"]))
	if !strings.EqualFold(resourceState, "Maintenance") {
		maintenance, maintenanceErr := client.CallCommand(ctx, http.MethodPost, "prepareHostForMaintenance", session.SessionKey, url.Values{"id": {hostID}})
		result.MaintenanceResponse = maintenance
		if maintenanceErr != nil {
			return result, fmt.Errorf("prepareHostForMaintenance failed for %s: %w", hostname, maintenanceErr)
		}
		if jobID := nestedString(maintenance, "preparehostformaintenanceresponse", "jobid"); jobID != "" {
			if err := waitCloudStackJob(ctx, client, session.SessionKey, jobID); err != nil {
				return result, fmt.Errorf("prepareHostForMaintenance failed for %s: %w", hostname, err)
			}
		}
	}
	maintenanceVerify, err := client.CallCommand(ctx, http.MethodGet, "listHosts", session.SessionKey, url.Values{"id": {hostID}})
	result.MaintenanceVerify = maintenanceVerify
	if err != nil {
		return result, fmt.Errorf("failed to verify maintenance state for %s: %w", hostname, err)
	}
	maintenanceHost := findCloudStackHost(maintenanceVerify, hostname, hostIP)
	if maintenanceHost == nil {
		return result, fmt.Errorf("host %s disappeared before deleteHost", hostname)
	}
	if verifiedState := strings.TrimSpace(stringValue(maintenanceHost["resourcestate"])); !strings.EqualFold(verifiedState, "Maintenance") {
		return result, fmt.Errorf("host %s is not in Maintenance resource state after prepareHostForMaintenance: resourcestate=%s", hostname, firstNonEmptyString(verifiedState, "unknown"))
	}
	params := url.Values{
		"id":                       {hostID},
		"forced":                   {fmt.Sprintf("%t", force)},
		"forcedestroylocalstorage": {"false"},
	}
	deleted, err := client.CallCommand(ctx, http.MethodPost, "deleteHost", session.SessionKey, params)
	result.DeleteResponse = deleted
	if err != nil {
		return result, err
	}
	if !boolValue(nestedValue(deleted, "deletehostresponse", "success")) {
		displayText := nestedString(deleted, "deletehostresponse", "displaytext")
		return result, fmt.Errorf("deleteHost did not report success for %s: %s", hostname, firstNonEmptyString(displayText, "success=false"))
	}
	verify, err := client.CallCommand(ctx, http.MethodGet, "listHosts", session.SessionKey, url.Values{"id": {hostID}})
	result.VerifyResponse = verify
	if err != nil {
		return result, err
	}
	if findCloudStackHost(verify, hostname, hostIP) != nil {
		return result, fmt.Errorf("deleteHost completed but listHosts still returns %s", hostname)
	}
	return result, nil
}

func findCloudStackHost(response map[string]any, hostname string, hostIP string) map[string]any {
	wrapper, _ := response["listhostsresponse"].(map[string]any)
	items, _ := wrapper["host"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		name := strings.TrimSpace(stringValue(item["name"]))
		ip := strings.TrimSpace(stringValue(item["ipaddress"]))
		if (hostname != "" && strings.EqualFold(name, strings.TrimSpace(hostname))) || (hostIP != "" && ip == strings.TrimSpace(hostIP)) {
			return item
		}
	}
	return nil
}

func nestedString(root map[string]any, keys ...string) string {
	return stringValue(nestedValue(root, keys...))
}

func nestedValue(root map[string]any, keys ...string) any {
	var value any = root
	for _, key := range keys {
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = object[key]
	}
	return value
}

func waitCloudStackJob(ctx context.Context, client *Client, sessionKey string, jobID string) error {
	interval := durationFromEnv("ABLESTACK_MOLD_ASYNC_POLL_INTERVAL", defaultAsyncPollInterval)
	timeoutDuration := durationFromEnv("ABLESTACK_MOLD_HOST_REMOVE_TIMEOUT", defaultHostRemoveAsyncTimeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	timeout := time.NewTimer(timeoutDuration)
	defer timeout.Stop()
	for {
		response, err := client.CallCommand(ctx, http.MethodGet, "queryAsyncJobResult", sessionKey, url.Values{"jobid": {jobID}})
		if err != nil {
			return err
		}
		status := nestedString(response, "queryasyncjobresultresponse", "jobstatus")
		switch status {
		case "1":
			return nil
		case "2":
			message := nestedString(response, "queryasyncjobresultresponse", "jobresult", "errortext")
			return fmt.Errorf("CloudStack job %s failed: %s", jobID, firstNonEmptyString(message, "unknown error"))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return fmt.Errorf("CloudStack job %s timed out after %s", jobID, timeoutDuration)
		case <-ticker.C:
		}
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
