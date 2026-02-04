package protocol

// UpdateRequest is sent by the controller to update an agent.
type UpdateRequest struct {
	RequestID     string `json:"request_id"`
	TargetVersion string `json:"target_version"`
	BinaryURL     string `json:"binary_url"`
}

// UpdateResponse is sent by agent after attempting update.
type UpdateResponse struct {
	RequestID string `json:"request_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	Message   string `json:"message,omitempty"`
}

// NewUpdateRequest creates a new update request.
func NewUpdateRequest(targetVersion, binaryURL string) *UpdateRequest {
	return &UpdateRequest{
		RequestID:     generateID(),
		TargetVersion: targetVersion,
		BinaryURL:     binaryURL,
	}
}
