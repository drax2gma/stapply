package actions

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/drax2gma/stapply/internal/protocol"
)

// CmdAction executes shell commands.
type CmdAction struct{}

// Execute runs a shell command and returns the result.
func (a *CmdAction) Execute(requestID string, args map[string]string, dryRun bool) *protocol.RunResponse {
	start := time.Now()

	command, ok := args["command"]
	if !ok || command == "" {
		return protocol.NewErrorResponse(requestID,
			&ActionError{Action: "cmd", Err: ErrMissingArg("command")}, 0)
	}

	// For dry run, we just check if the command executable exists in PATH
	if dryRun {
		// Extract the executable name (first part of command)
		// This is a naive check; for complex shell commands it might be inaccurate
		// but serves as a basic preflight check.
		parts := strings.Fields(command)
		if len(parts) > 0 {
			exe := parts[0]
			// If it's a shell builtin or complex pipeline, LookPath might fail or be irrelevant.
			// attempting simple LookPath
			if _, err := exec.LookPath(exe); err != nil {
				// Don't fail the dry run hard, just report it
				return protocol.NewRunResponse(
					requestID,
					false,
					0,
					fmt.Sprintf("Dry run: Command '%s' not found in PATH", exe),
					"",
					time.Since(start).Milliseconds(),
				)
			}
		}

		return protocol.NewRunResponse(
			requestID,
			true, // assume change for dry run
			0,
			fmt.Sprintf("Dry run: Would execute command: %s", command),
			"",
			time.Since(start).Milliseconds(),
		)
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
