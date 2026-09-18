package wallservice

import (
	"context"
	"strings"
)

func (m *Manager) ServiceStatuses(ctx context.Context) []ServiceStatus {
	statuses := make([]ServiceStatus, 0, len(m.Services))
	for _, service := range m.Services {
		active, _ := m.run(ctx, "systemctl", "is-active", service)
		enabled, _ := m.run(ctx, "systemctl", "is-enabled", service)
		statuses = append(statuses, ServiceStatus{
			Name:    service,
			Active:  strings.TrimSpace(active) == "active",
			Enabled: enabledServiceState(enabled),
		})
	}
	return statuses
}

func enabledServiceState(value string) bool {
	switch strings.TrimSpace(value) {
	case "enabled", "enabled-runtime", "static", "indirect", "alias", "linked", "linked-runtime", "generated", "transient":
		return true
	default:
		return false
	}
}
