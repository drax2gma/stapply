package protocol

// Status represents the execution status.
type Status string

const (
	StatusOK      Status = "ok"
	StatusFailed  Status = "failed"
	StatusTimeout Status = "timeout"
	StatusError   Status = "error"
)

// PingResponse is the response to a ping request.
type PingResponse struct {
	RequestID     string `json:"request_id"`
	AgentID       string `json:"agent_id"`
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

// RunResponse is the response to a run request.
type RunResponse struct {
	RequestID  string `json:"request_id"`
	Status     Status `json:"status"`
	Changed    bool   `json:"changed"`
	ExitCode   int    `json:"exit_code,omitempty"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

// NewPingResponse creates a ping response.
func NewPingResponse(requestID, agentID, version string, uptimeSeconds int64) *PingResponse {
	return &PingResponse{
		RequestID:     requestID,
		AgentID:       agentID,
		Version:       version,
		UptimeSeconds: uptimeSeconds,
	}
}

// NewRunResponse creates a successful run response.
func NewRunResponse(requestID string, changed bool, exitCode int, stdout, stderr string, durationMs int64) *RunResponse {
	status := StatusOK
	if exitCode != 0 {
		status = StatusFailed
	}
	return &RunResponse{
		RequestID:  requestID,
		Status:     status,
		Changed:    changed,
		ExitCode:   exitCode,
		Stdout:     stdout,
		Stderr:     stderr,
		DurationMs: durationMs,
	}
}

// NewErrorResponse creates an error run response.
func NewErrorResponse(requestID string, err error, durationMs int64) *RunResponse {
	return &RunResponse{
		RequestID:  requestID,
		Status:     StatusError,
		Changed:    false,
		Error:      err.Error(),
		DurationMs: durationMs,
	}
}
