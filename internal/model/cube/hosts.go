package cube

import (
	"sync"
	"time"
)

type TypeHost struct {
	IP        string   `json:"ip"`
	HostNames []string `json:"hostnames"`
}

type HostRoleGroup struct {
	CCVM     *TypeHost  `json:"ccvm,omitempty"`
	Ablecube []TypeHost `json:"ablecube,omitempty"`
	SCVM     []TypeHost `json:"scvm,omitempty"`
	Self     *TypeHost  `json:"self,omitempty"`
}

type TypeHosts struct {
	Localhost         []TypeHost     `json:"localhost,omitempty"`
	ManagementNetwork *HostRoleGroup `json:"management-network,omitempty"`
	PublicNetwork     *HostRoleGroup `json:"public-network,omitempty"`
	ClientNetwork     *HostRoleGroup `json:"client-network,omitempty"`
	Others            []TypeHost     `json:"others,omitempty"`
	RefreshTime       time.Time      `json:"refresh_time"`
	mu                *sync.RWMutex
}

var lockHosts sync.Once
var _Hosts *TypeHosts

func Hosts() *TypeHosts {
	lockHosts.Do(func() {
		_Hosts = &TypeHosts{mu: &sync.RWMutex{}}
	})
	return _Hosts
}

func (h *TypeHosts) Lock() {
	if h == nil || h.mu == nil {
		return
	}
	h.mu.Lock()
}

func (h *TypeHosts) Unlock() {
	if h == nil || h.mu == nil {
		return
	}
	h.mu.Unlock()
}

func (h *TypeHosts) RLock() {
	if h == nil || h.mu == nil {
		return
	}
	h.mu.RLock()
}

func (h *TypeHosts) RUnlock() {
	if h == nil || h.mu == nil {
		return
	}
	h.mu.RUnlock()
}

func (h *TypeHosts) ApplyFrom(src *TypeHosts) {
	if h == nil || src == nil {
		return
	}
	if h.mu != nil {
		h.mu.Lock()
		defer h.mu.Unlock()
	}
	h.Localhost = src.Localhost
	h.ManagementNetwork = src.ManagementNetwork
	h.PublicNetwork = src.PublicNetwork
	h.ClientNetwork = src.ClientNetwork
	h.Others = src.Others
	h.RefreshTime = src.RefreshTime
}
