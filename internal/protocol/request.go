package protocol

import "github.com/google/uuid"

// RequestType identifies the type of request.
type RequestType string

const (
	RequestTypePing     RequestType = "ping"
	RequestTypeRun      RequestType = "run"
	RequestTypeDiscover RequestType = "discover"
)

// PingRequest is a health check request.
type PingRequest struct {
	RequestID         string      `json:"request_id"`
	Type              RequestType `json:"type"`
	ControllerVersion string      `json:"controller_version"`
}

// DiscoverRequest is a request to gather system facts.
type DiscoverRequest struct {
	RequestID string      `json:"request_id"`
	Type      RequestType `json:"type"`
}

// NewDiscoverRequest creates a new discovery request.
func NewDiscoverRequest() *DiscoverRequest {
	return &DiscoverRequest{
		RequestID: generateID(),
		Type:      RequestTypeDiscover,
	}
}

// RunRequest is an action execution request.
type RunRequest struct {
	RequestID string            `json:"request_id"`
	Type      RequestType       `json:"type"`
	TimeoutMs int               `json:"timeout_ms"`
	Action    string            `json:"action"`
	Args      map[string]string `json:"args"`
	DryRun    bool              `json:"dry_run,omitempty"`
}

// NewPingRequest creates a new ping request with a generated ID.
func NewPingRequest(controllerVersion string) *PingRequest {
	return &PingRequest{
		RequestID:         generateID(),
		Type:              RequestTypePing,
		ControllerVersion: controllerVersion,
	}
}

// NewRunRequest creates a new run request with a generated ID.
func NewRunRequest(action string, args map[string]string, timeoutMs int, dryRun bool) *RunRequest {
	return &RunRequest{
		RequestID: generateID(),
		Type:      RequestTypeRun,
		TimeoutMs: timeoutMs,
		Action:    action,
		Args:      args,
		DryRun:    dryRun,
	}
}

// generateID generates a unique request ID.
func generateID() string {
	return uuid.New().String()
}
