package main

import (
	"github.com/satori/go.uuid"
	"time"
)

type LabelsType = struct {
	AgentDiscoveredArch string `json:"agent_discovered_arch"`
	AgentDiscoveredOs   string `json:"agent_discovered_os"`
}
type AgentReleaseEntry = struct {
	Id      string
	ArType  string `json:"type"`
	Version string
	Labels  LabelsType
	Url     string
	Exe     string
}
type AgentReleaseType = struct {
	Content []AgentReleaseEntry
}

type TemplateFields = struct {
	ResourceId    string
	PrivateZoneID string
	CertDir       string
	ApiKey        string
	RegularId     string
}
type config = struct {
	env             string
	currentUUID     uuid.UUID
	id              string
	privateZoneId   string
	resourceId      string
	tenantId        string
	regularId       string
	adminId         string
	publicApiUrl    string
	adminApiUrl     string
	agentReleaseUrl string
	certDir         string
	regularToken    string
	adminToken      string
	dir             string
	kafkaBrokers    []string
	topic           string
	port            string
	certFile        string
	keyFile         string
	caFile          string
	regularApiKey   string
	adminApiKey     string
	adminPassword   string
	publicZoneId    string
}

type IdentityResp struct {
	Access struct {
		ServiceCatalog []struct {
			Endpoints []struct {
				TenantID  string `json:"tenantId"`
				PublicURL string `json:"publicURL"`
				Region    string `json:"region"`
			} `json:"endpoints"`
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"serviceCatalog"`
		User struct {
			RAXAUTHSessionInactivityTimeout string `json:"RAX-AUTH:sessionInactivityTimeout"`
			RAXAUTHDefaultRegion            string `json:"RAX-AUTH:defaultRegion"`
			Roles                           []struct {
				Name        string `json:"name"`
				TenantID    string `json:"tenantId,omitempty"`
				Description string `json:"description"`
				ID          string `json:"id"`
			} `json:"roles"`
			RAXAUTHPhonePin      string `json:"RAX-AUTH:phonePin"`
			Name                 string `json:"name"`
			ID                   string `json:"id"`
			RAXAUTHDomainID      string `json:"RAX-AUTH:domainId"`
			RAXAUTHPhonePinState string `json:"RAX-AUTH:phonePinState"`
		} `json:"user"`
		Token struct {
			Expires                time.Time `json:"expires"`
			RAXAUTHIssued          time.Time `json:"RAX-AUTH:issued"`
			RAXAUTHAuthenticatedBy []string  `json:"RAX-AUTH:authenticatedBy"`
			ID                     string    `json:"id"`
			Tenant                 struct {
				Name string `json:"name"`
				ID   string `json:"id"`
			} `json:"tenant"`
		} `json:"token"`
	} `json:"access"`
}

type AgentReleaseCreateResp struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Version string `json:"version"`
	Labels  struct {
		AgentDiscoveredOs   string `json:"agent_discovered_os"`
		AgentDiscoveredArch string `json:"agent_discovered_arch"`
	} `json:"labels"`
	URL              string    `json:"url"`
	Exe              string    `json:"exe"`
	CreatedTimestamp time.Time `json:"createdTimestamp"`
	UpdatedTimestamp time.Time `json:"updatedTimestamp"`
}
type GetAgentInstallsResp struct {
	Content []struct {
		ID           string `json:"id"`
		AgentRelease struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Version string `json:"version"`
			Labels  struct {
				AgentDiscoveredArch string `json:"agent_discovered_arch"`
				AgentDiscoveredOs   string `json:"agent_discovered_os"`
			} `json:"labels"`
			URL              string    `json:"url"`
			Exe              string    `json:"exe"`
			CreatedTimestamp time.Time `json:"createdTimestamp"`
			UpdatedTimestamp time.Time `json:"updatedTimestamp"`
		} `json:"agentRelease"`
		LabelSelector struct {
			AgentDiscoveredOs string `json:"agent_discovered_os"`
		} `json:"labelSelector"`
		CreatedTimestamp time.Time `json:"createdTimestamp"`
		UpdatedTimestamp time.Time `json:"updatedTimestamp"`
	} `json:"content"`
	Number        int  `json:"number"`
	TotalPages    int  `json:"totalPages"`
	TotalElements int  `json:"totalElements"`
	Last          bool `json:"last"`
	First         bool `json:"first"`
}
type GetResourcesResp struct {
	Content []struct {
		TenantID   string `json:"tenantId"`
		ResourceID string `json:"resourceId"`
		Labels     struct {
			AgentDiscoveredArch     string `json:"agent_discovered_arch"`
			AgentDiscoveredHostname string `json:"agent_discovered_hostname"`
			AgentEnvironment        string `json:"agent_environment"`
			AgentDiscoveredOs       string `json:"agent_discovered_os"`
		} `json:"labels"`
		Metadata                  interface{} `json:"metadata"`
		PresenceMonitoringEnabled bool        `json:"presenceMonitoringEnabled"`
		AssociatedWithEnvoy       bool        `json:"associatedWithEnvoy"`
		CreatedTimestamp          time.Time   `json:"createdTimestamp"`
		UpdatedTimestamp          time.Time   `json:"updatedTimestamp"`
	} `json:"content"`
	Number        int  `json:"number"`
	TotalPages    int  `json:"totalPages"`
	TotalElements int  `json:"totalElements"`
	Last          bool `json:"last"`
	First         bool `json:"first"`
}
type GetZonesResp struct {
	Content []struct {
		Name              string        `json:"name"`
		PollerTimeout     int           `json:"pollerTimeout"`
		Provider          interface{}   `json:"provider"`
		ProviderRegion    interface{}   `json:"providerRegion"`
		SourceIPAddresses []interface{} `json:"sourceIpAddresses"`
		CreatedTimestamp  time.Time     `json:"createdTimestamp"`
		UpdatedTimestamp  time.Time     `json:"updatedTimestamp"`
		Public            bool          `json:"public"`
	} `json:"content"`
	Number        int  `json:"number"`
	TotalPages    int  `json:"totalPages"`
	TotalElements int  `json:"totalElements"`
	Last          bool `json:"last"`
	First         bool `json:"first"`
}
type GetMonitorsResp struct {
	Content []struct {
		ID            string      `json:"id"`
		Name          interface{} `json:"name"`
		LabelSelector struct {
			AgentDiscoveredOs string `json:"agent_discovered_os"`
		} `json:"labelSelector"`
		LabelSelectorMethod string      `json:"labelSelectorMethod"`
		ResourceID          interface{} `json:"resourceId"`
		Interval            string      `json:"interval"`
		CreatedTimestamp    time.Time   `json:"createdTimestamp"`
		UpdatedTimestamp    time.Time   `json:"updatedTimestamp"`
	} `json:"content"`
	Number        int  `json:"number"`
	TotalPages    int  `json:"totalPages"`
	TotalElements int  `json:"totalElements"`
	Last          bool `json:"last"`
	First         bool `json:"first"`
}

type GetTasksResp struct {
	Content []struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Measurement    string `json:"measurement"`
		TaskParameters struct {
			Info     interface{} `json:"info"`
			Warning  interface{} `json:"warning"`
			Critical struct {
				Expression struct {
					Field      string `json:"field"`
					Threshold  int    `json:"threshold"`
					Comparator string `json:"comparator"`
				} `json:"expression"`
				ConsecutiveCount int `json:"consecutiveCount"`
			} `json:"critical"`
			EvalExpressions   interface{} `json:"evalExpressions"`
			WindowLength      interface{} `json:"windowLength"`
			WindowFields      interface{} `json:"windowFields"`
			FlappingDetection bool        `json:"flappingDetection"`
			LabelSelector     struct {
				AgentEnvironment string `json:"agent_environment"`
			} `json:"labelSelector"`
		} `json:"taskParameters"`
		CreatedTimestamp time.Time `json:"createdTimestamp"`
		UpdatedTimestamp time.Time `json:"updatedTimestamp"`
	} `json:"content"`
	Number        int  `json:"number"`
	TotalPages    int  `json:"totalPages"`
	TotalElements int  `json:"totalElements"`
	Last          bool `json:"last"`
	First         bool `json:"first"`
}

type GetPoliciesResp struct {
	Content []struct {
		ID               string    `json:"id"`
		Scope            string    `json:"scope"`
		Subscope         string    `json:"subscope"`
		CreatedTimestamp time.Time `json:"createdTimestamp"`
		UpdatedTimestamp time.Time `json:"updatedTimestamp"`
		Name             string    `json:"name"`
		MonitorID        string    `json:"monitorId"`
	} `json:"content"`
	Number        int  `json:"number"`
	TotalPages    int  `json:"totalPages"`
	TotalElements int  `json:"totalElements"`
	Last          bool `json:"last"`
	First         bool `json:"first"`
}

type CreatePolicyMonitorResp struct {
	ID            string      `json:"id"`
	Name          interface{} `json:"name"`
	LabelSelector struct {
		AgentDiscoveredOs string `json:"agent_discovered_os"`
	} `json:"labelSelector"`
	LabelSelectorMethod string      `json:"labelSelectorMethod"`
	ResourceID          interface{} `json:"resourceId"`
	Interval            string      `json:"interval"`
	Details             struct {
		Type            string   `json:"type"`
		MonitoringZones []string `json:"monitoringZones"`
		Plugin          struct {
			Type               string `json:"type"`
			Address            string `json:"address"`
			ResponseTimeout    string `json:"responseTimeout"`
			Method             string `json:"method"`
			FollowRedirects    bool   `json:"followRedirects"`
			InsecureSkipVerify bool   `json:"insecureSkipVerify"`
		} `json:"plugin"`
	} `json:"details"`
	CreatedTimestamp time.Time `json:"createdTimestamp"`
	UpdatedTimestamp time.Time `json:"updatedTimestamp"`
}