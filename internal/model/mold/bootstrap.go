package mold

import "time"

const (
	BootstrapJobStatusQueued    = "queued"
	BootstrapJobStatusRunning   = "running"
	BootstrapJobStatusSucceeded = "succeeded"
	BootstrapJobStatusFailed    = "failed"

	BootstrapStepStatusPending   = "pending"
	BootstrapStepStatusRunning   = "running"
	BootstrapStepStatusSucceeded = "succeeded"
	BootstrapStepStatusFailed    = "failed"
)

// BootstrapRequest describes Mold initial infrastructure bootstrap input.
// IDs created by earlier steps, such as zoneid/podid/clusterid, are resolved at runtime.
// @name MoldBootstrapRequest
type BootstrapRequest struct {
	Auth                  MoldAuthRequest               `json:"auth,omitempty"`
	Zone                  ZoneRequest                   `json:"zone"`
	PhysicalNetwork       PhysicalNetworkRequest        `json:"physical_network"`
	Pod                   PodRequest                    `json:"pod"`
	PublicIP              PublicIPRangeRequest          `json:"public_ip"`
	Cluster               ClusterRequest                `json:"cluster"`
	Hosts                 []HostRequest                 `json:"hosts,omitempty" swaggerignore:"true"`
	PrimaryStorage        PrimaryStorageRequest         `json:"primary_storage,omitempty" swaggerignore:"true"`
	SecondaryStorage      SecondaryStorageRequest       `json:"secondary_storage,omitempty" swaggerignore:"true"`
	SecondaryStagingStore *SecondaryStagingStoreRequest `json:"secondary_staging_store,omitempty"`
}

// ZoneRequest maps to createZone input.
// @name MoldZoneRequest
type ZoneRequest struct {
	Name                 string `json:"name" example:"Zone"`
	DNS1                 string `json:"dns1" example:"8.8.8.8"`
	DNS2                 string `json:"dns2,omitempty" example:"1.1.1.1"`
	InternalDNS1         string `json:"internaldns1" example:"10.10.0.1"`
	InternalDNS2         string `json:"internaldns2,omitempty" example:"10.10.0.2"`
	NetworkType          string `json:"networktype" example:"Advanced"`
	GuestCIDRAddress     string `json:"guestcidraddress,omitempty" example:"10.1.1.0/24"`
	Domain               string `json:"domain,omitempty" example:"local"`
	LocalStorageEnabled  string `json:"localstorageenabled,omitempty" example:"false"`
	SecurityGroupEnabled string `json:"securitygroupenabled,omitempty" example:"false"`
}

// PhysicalNetworkRequest maps to createPhysicalNetwork input.
// @name MoldPhysicalNetworkRequest
type PhysicalNetworkRequest struct {
	Name                 string `json:"name" example:"Physicalnetwork"`
	BroadcastDomainRange string `json:"broadcastdomainrange,omitempty" example:"Zone"`
	IsolationMethods     string `json:"isolationmethods,omitempty" example:"VLAN"`
	NetworkSpeed         string `json:"networkspeed,omitempty" example:"10G"`
	VLAN                 string `json:"vlan,omitempty" example:"1-1" swaggerignore:"true"`
	Tags                 string `json:"tags,omitempty" example:"guest"`
}

// PodRequest maps to createPod input.
// @name MoldPodRequest
type PodRequest struct {
	Name    string `json:"name" example:"Pod"`
	Gateway string `json:"gateway" example:"10.10.31.1"`
	Netmask string `json:"netmask" example:"255.255.255.0"`
	StartIP string `json:"startip" example:"10.10.31.100"`
	EndIP   string `json:"endip" example:"10.10.31.150"`
}

// PublicIPRangeRequest maps to createVlanIpRange input for an untagged public range.
// @name MoldPublicIPRangeRequest
type PublicIPRangeRequest struct {
	Gateway string `json:"gateway" example:"10.10.20.1"`
	Netmask string `json:"netmask" example:"255.255.255.0"`
	StartIP string `json:"startip" example:"10.10.20.100"`
	EndIP   string `json:"endip" example:"10.10.20.150"`
}

// ClusterRequest maps to addCluster input.
// @name MoldClusterRequest
type ClusterRequest struct {
	Name        string `json:"name" example:"Cluster"`
	ClusterType string `json:"clustertype" example:"CloudManaged"`
	Hypervisor  string `json:"hypervisor" example:"KVM"`
	Arch        string `json:"arch,omitempty" example:"x86_64"`
}

// HostRequest maps to addHost input.
// @name MoldHostRequest
type HostRequest struct {
	URL        string `json:"url" example:"http://10.10.31.1"`
	Username   string `json:"username" example:"root"`
	Password   string `json:"password" example:"password"`
	Hypervisor string `json:"hypervisor,omitempty" example:"KVM"`
	HostTags   string `json:"hosttags,omitempty" example:"compute"`
}

// PrimaryStorageRequest maps to createStoragePool input.
// @name MoldPrimaryStorageRequest
type PrimaryStorageRequest struct {
	Name            string   `json:"name,omitempty" example:"Primary Storage(RBD)"`
	URL             string   `json:"url,omitempty" example:"SharedMountPoint://localhost/mnt/glue-gfs"`
	Scope           string   `json:"scope,omitempty" example:"cluster"`
	Provider        string   `json:"provider,omitempty" example:"DefaultPrimary"`
	Protocol        string   `json:"protocol,omitempty" example:"SharedMountPoint"`
	Hypervisor      string   `json:"hypervisor,omitempty" example:"KVM"`
	Tags            string   `json:"tags,omitempty" example:"Primary Storage(Glue)"`
	RADOSMonitors   string   `json:"rados_monitors,omitempty" example:"scvm1,scvm2,scvm3"`
	RADOSPool       string   `json:"rados_pool,omitempty" example:"rbd"`
	RADOSUser       string   `json:"rados_user,omitempty" example:"admin"`
	KRBDPath        string   `json:"krbd_path,omitempty" example:"/dev/rbd"`
	DetailsProvider string   `json:"details_provider,omitempty" example:"ABLESTACK"`
	AutoRBDSecret   bool     `json:"-" swaggerignore:"true"`
	CephKeyHosts    []string `json:"-" swaggerignore:"true"`
}

// SecondaryStorageRequest maps to addSecondaryStorage input.
// @name MoldSecondaryStorageRequest
type SecondaryStorageRequest struct {
	Name     string `json:"name,omitempty" example:"Secondary Storage"`
	Provider string `json:"provider,omitempty" example:"NFS"`
	URL      string `json:"url" example:"nfs://10.10.31.20/export/secondary"`
}

// SecondaryStagingStoreRequest maps to createSecondaryStagingStore input when used.
// @name MoldSecondaryStagingStoreRequest
type SecondaryStagingStoreRequest struct {
	URL      string `json:"url" example:"nfs://10.10.31.20/export/staging"`
	Provider string `json:"provider,omitempty" example:"NFS"`
	Scope    string `json:"scope,omitempty" example:"zone"`
}

// BootstrapPlanValue describes the ordered Mold bootstrap plan and missing inputs.
// @name MoldBootstrapPlanValue
type BootstrapPlanValue struct {
	Ready         bool                `json:"ready"`
	Steps         []BootstrapPlanStep `json:"steps"`
	MissingInputs []string            `json:"missing_inputs,omitempty"`
	Notes         []string            `json:"notes,omitempty"`
}

// BootstrapPlanStep describes one bootstrap step.
// @name MoldBootstrapPlanStep
type BootstrapPlanStep struct {
	Name            string   `json:"name" example:"create_zone"`
	Command         string   `json:"command,omitempty" example:"createZone"`
	VerifyCommand   string   `json:"verify_command,omitempty" example:"listZones"`
	ResultKey       string   `json:"result_key,omitempty" example:"zone"`
	RequiredInputs  []string `json:"required_inputs,omitempty"`
	MissingInputs   []string `json:"missing_inputs,omitempty"`
	SuccessCriteria []string `json:"success_criteria,omitempty"`
	ErrorFields     []string `json:"error_fields,omitempty"`
	Notes           []string `json:"notes,omitempty"`
}

// BootstrapResultValue is the final Mold bootstrap execution result.
// @name MoldBootstrapResultValue
type BootstrapResultValue struct {
	Status    string                   `json:"status" example:"succeeded"`
	Endpoint  string                   `json:"endpoint,omitempty" example:"http://10.10.254.236:8080/client/api"`
	Steps     []BootstrapStepResult    `json:"steps"`
	Resources BootstrapResourceResults `json:"resources,omitempty"`
	Error     *BootstrapError          `json:"error,omitempty"`
}

// BootstrapStartResponse is returned when a Mold bootstrap job starts.
// @name MoldBootstrapStartResponse
type BootstrapStartResponse struct {
	Code    int                `json:"code" example:"202"`
	JobID   string             `json:"job_id" example:"018f9c39-7ca9-7f4e-9a04-f6373d8f7e2b"`
	Status  string             `json:"status" example:"queued"`
	Message string             `json:"message,omitempty" example:"mold bootstrap job started"`
	Steps   []BootstrapJobStep `json:"steps,omitempty"`
}

// BootstrapJob is an in-memory Mold bootstrap job.
// @name MoldBootstrapJob
type BootstrapJob struct {
	JobID       string                   `json:"job_id" example:"018f9c39-7ca9-7f4e-9a04-f6373d8f7e2b"`
	Status      string                   `json:"status" example:"running"`
	CurrentStep string                   `json:"current_step,omitempty" example:"create_zone"`
	Message     string                   `json:"message,omitempty" example:"ok"`
	CreatedAt   time.Time                `json:"created_at"`
	StartedAt   *time.Time               `json:"started_at,omitempty"`
	FinishedAt  *time.Time               `json:"finished_at,omitempty"`
	Steps       []BootstrapJobStep       `json:"steps"`
	Resources   BootstrapResourceResults `json:"resources,omitempty"`
	Error       *BootstrapError          `json:"error,omitempty"`
}

// BootstrapJobStep describes one Mold bootstrap job step state.
// @name MoldBootstrapJobStep
type BootstrapJobStep struct {
	Name          string               `json:"name" example:"create_zone"`
	Command       string               `json:"command,omitempty" example:"createZone"`
	VerifyCommand string               `json:"verify_command,omitempty" example:"listZones"`
	ResultKey     string               `json:"result_key,omitempty" example:"zone"`
	Status        string               `json:"status" example:"running"`
	Message       string               `json:"message,omitempty" example:"ok"`
	StartedAt     *time.Time           `json:"started_at,omitempty"`
	FinishedAt    *time.Time           `json:"finished_at,omitempty"`
	DurationMS    int64                `json:"duration_ms,omitempty" example:"1200"`
	Result        *BootstrapStepResult `json:"result,omitempty"`
	Error         *BootstrapError      `json:"error,omitempty"`
}

// BootstrapJobResponse is returned for one Mold bootstrap job.
// @name MoldBootstrapJobResponse
type BootstrapJobResponse struct {
	Code    int          `json:"code" example:"200"`
	Job     BootstrapJob `json:"job"`
	Message string       `json:"message,omitempty" example:"ok"`
}

// BootstrapJobListResponse is returned for Mold bootstrap job listing.
// @name MoldBootstrapJobListResponse
type BootstrapJobListResponse struct {
	Code    int            `json:"code" example:"200"`
	Jobs    []BootstrapJob `json:"jobs"`
	Message string         `json:"message,omitempty" example:"ok"`
}

// BootstrapStepResult describes one executed bootstrap step.
// @name MoldBootstrapStepResult
type BootstrapStepResult struct {
	Name           string           `json:"name" example:"create_zone"`
	Command        string           `json:"command,omitempty" example:"createZone"`
	VerifyCommand  string           `json:"verify_command,omitempty" example:"listZones"`
	ResultKey      string           `json:"result_key,omitempty" example:"zone"`
	Status         string           `json:"status" example:"succeeded"`
	Created        bool             `json:"created,omitempty"`
	Existing       bool             `json:"existing,omitempty"`
	Verified       bool             `json:"verified"`
	VerifiedBy     string           `json:"verified_by,omitempty" example:"listZones"`
	Resource       *ResourceResult  `json:"resource,omitempty"`
	Resources      []ResourceResult `json:"resources,omitempty"`
	RawResponse    any              `json:"raw_response,omitempty"`
	VerifyResponse any              `json:"verify_response,omitempty"`
	Error          *BootstrapError  `json:"error,omitempty"`
}

// BootstrapResourceResults groups verified resources by bootstrap domain.
// @name MoldBootstrapResourceResults
type BootstrapResourceResults struct {
	Zone                  *ResourceResult  `json:"zone,omitempty"`
	PhysicalNetwork       *ResourceResult  `json:"physical_network,omitempty"`
	TrafficTypes          []ResourceResult `json:"traffic_types,omitempty"`
	NetworkProviders      []ResourceResult `json:"network_providers,omitempty"`
	Pod                   *ResourceResult  `json:"pod,omitempty"`
	PublicIPRange         *ResourceResult  `json:"public_ip_range,omitempty"`
	Cluster               *ResourceResult  `json:"cluster,omitempty"`
	Hosts                 []ResourceResult `json:"hosts,omitempty"`
	PrimaryStorage        *ResourceResult  `json:"primary_storage,omitempty"`
	SecondaryStorage      *ResourceResult  `json:"secondary_storage,omitempty"`
	SecondaryStagingStore *ResourceResult  `json:"secondary_staging_store,omitempty"`
	SystemVMs             []ResourceResult `json:"system_vms,omitempty"`
}

// ResourceResult is one verified Mold resource.
// @name MoldResourceResult
type ResourceResult struct {
	ID         string `json:"id,omitempty" example:"2f6b0c0e-0000-0000-0000-000000000001"`
	Name       string `json:"name,omitempty" example:"zone-1"`
	URL        string `json:"url,omitempty" example:"nfs://10.10.31.20/export/secondary"`
	State      string `json:"state,omitempty" example:"Enabled"`
	Status     string `json:"status,omitempty" example:"Up"`
	VerifiedBy string `json:"verified_by,omitempty" example:"listZones"`
	Raw        any    `json:"raw,omitempty"`
}

// BootstrapError captures the command and Mold error details for a failed step.
// @name MoldBootstrapError
type BootstrapError struct {
	Step               string `json:"step,omitempty" example:"create_zone"`
	Method             string `json:"method,omitempty" example:"POST"`
	Endpoint           string `json:"endpoint,omitempty" example:"http://10.10.254.236:8080/client/api"`
	Command            string `json:"command,omitempty" example:"createZone"`
	VerifyCommand      string `json:"verify_command,omitempty" example:"listZones"`
	HTTPStatus         int    `json:"http_status,omitempty" example:"401"`
	HTTPBody           string `json:"http_body,omitempty"`
	SessionKeyAttached bool   `json:"sessionkey_attached,omitempty" example:"true"`
	Code               int    `json:"code,omitempty" example:"431"`
	ErrorCode          string `json:"error_code,omitempty" example:"431"`
	ErrorText          string `json:"error_text,omitempty" example:"unable to create zone"`
	Message            string `json:"message,omitempty" example:"mold api error response"`
	Raw                any    `json:"raw,omitempty"`
}
