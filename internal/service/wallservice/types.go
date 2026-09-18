package wallservice

const (
	DefaultRoot            = "/usr/share/ablestack/ablestack-wall"
	DefaultClusterJSONPath = "/etc/ablestack/properties/cluster.json"
)

var DefaultServices = []string{
	"blackbox-exporter",
	"node-exporter",
	"grafana-server",
	"process-exporter",
	"prometheus",
	"netdive-analyzer",
}

type ServiceStatus struct {
	Name    string
	Active  bool
	Enabled bool
}

type Manager struct {
	Runner   CommandRunner
	Services []string
}

func New() *Manager {
	return &Manager{
		Runner:   OSCommandRunner{},
		Services: append([]string(nil), DefaultServices...),
	}
}
