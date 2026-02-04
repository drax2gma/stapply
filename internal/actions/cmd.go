package actions

import (
	"bytes"
	"os"
	"os/exec"
	"time"

	"github.com/drax2gma/stapply/internal/protocol"
)

// CmdAction executes shell commands.
type CmdAction struct{}

// Execute runs a shell command and returns the result.
func (a *CmdAction) Execute(requestID string, args map[string]string) *protocol.RunResponse {
	start := time.Now()

	command, ok := args["command"]
	if !ok || command == "" {
		return protocol.NewErrorResponse(requestID,
			&ActionError{Action: "cmd", Err: ErrMissingArg("command")}, 0)
	}

	// Check "creates" idempotency guard
	if creates := args["creates"]; creates != "" {
		if _, err := os.Stat(creates); err == nil {
			// File exists, skip execution
			return &protocol.RunResponse{
				RequestID:  requestID,
				Status:     protocol.StatusOK,
				Changed:    false,
				Stdout:     "",
				Stderr:     "",
				DurationMs: time.Since(start).Milliseconds(),
			}
		}
	}

	// Execute command via shell
	cmd := exec.Command("sh", "-c", command)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return protocol.NewErrorResponse(requestID, err, time.Since(start).Milliseconds())
		}
	}

	return protocol.NewRunResponse(
		requestID,
		true, // cmd actions always report changed=true unless creates guard triggered
		exitCode,
		stdout.String(),
		stderr.String(),
		time.Since(start).Milliseconds(),
	)
}
