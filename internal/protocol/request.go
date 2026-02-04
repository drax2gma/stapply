package protocol

import "github.com/google/uuid"

// RequestType identifies the type of request.
type RequestType string

const (
	RequestTypePing RequestType = "ping"
	RequestTypeRun  RequestType = "run"
)

// PingRequest is a health check request.
type PingRequest struct {
	RequestID string      `json:"request_id"`
	Type      RequestType `json:"type"`
}

// RunRequest is an action execution request.
type RunRequest struct {
	RequestID string            `json:"request_id"`
	Type      RequestType       `json:"type"`
	TimeoutMs int               `json:"timeout_ms"`
	Action    string            `json:"action"`
	Args      map[string]string `json:"args"`
}

// NewPingRequest creates a new ping request with a generated ID.
func NewPingRequest() *PingRequest {
	return &PingRequest{
		RequestID: generateID(),
		Type:      RequestTypePing,
	}
}

// NewRunRequest creates a new run request with a generated ID.
func NewRunRequest(action string, args map[string]string, timeoutMs int) *RunRequest {
	return &RunRequest{
		RequestID: generateID(),
		Type:      RequestTypeRun,
		TimeoutMs: timeoutMs,
		Action:    action,
		Args:      args,
	}
}

// generateID generates a unique request ID.
func generateID() string {
	return uuid.New().String()
}
