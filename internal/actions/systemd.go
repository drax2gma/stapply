package actions

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/drax2gma/stapply/internal/protocol"
)

// SystemdAction controls systemd units.
type SystemdAction struct{}

// Execute performs systemd operations with change detection.
func (a *SystemdAction) Execute(requestID string, args map[string]string) *protocol.RunResponse {
	start := time.Now()

	// Validate args
	action, ok := args["action"]
	if !ok || action == "" {
		return protocol.NewErrorResponse(requestID,
			&ActionError{Action: "systemd", Err: ErrMissingArg("action")}, 0)
	}

	// daemon-reload doesn't need a unit
	if action != "daemon-reload" {
		unit, ok := args["unit"]
		if !ok || unit == "" {
			return protocol.NewErrorResponse(requestID,
				&ActionError{Action: "systemd", Err: ErrMissingArg("unit")}, 0)
		}
		args["unit"] = unit // Ensure it's set for later use
	}

	// Validate action type
	validActions := map[string]bool{
		"enable":        true,
		"disable":       true,
		"start":         true,
		"stop":          true,
		"restart":       true,
		"daemon-reload": true,
	}
	if !validActions[action] {
		return protocol.NewErrorResponse(requestID,
			fmt.Errorf("invalid systemd action: %s", action), time.Since(start).Milliseconds())
	}

	// Detect change based on action type
	changed := true
	switch action {
	case "enable", "disable":
		changed = a.checkEnabledStateChange(args["unit"], action)
	case "start", "stop":
		changed = a.checkActiveStateChange(args["unit"], action)
	case "restart":
		// Restart always causes change if service is running
		changed = a.isServiceActive(args["unit"])
	case "daemon-reload":
		// daemon-reload always reports changed (can't detect)
		changed = true
	}

	// Execute systemd command
	var cmd *exec.Cmd
	if action == "daemon-reload" {
		cmd = exec.Command("systemctl", "daemon-reload")
	} else {
		cmd = exec.Command("systemctl", action, args["unit"])
	}

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
		changed,
		exitCode,
		stdout.String(),
		stderr.String(),
		time.Since(start).Milliseconds(),
	)
}

// checkEnabledStateChange checks if enable/disable would change state.
func (a *SystemdAction) checkEnabledStateChange(unit, action string) bool {
	isEnabled := a.isServiceEnabled(unit)

	switch action {
	case "enable":
		return !isEnabled // Changed if currently disabled
	case "disable":
		return isEnabled // Changed if currently enabled
	}
	return true
}

// checkActiveStateChange checks if start/stop would change state.
func (a *SystemdAction) checkActiveStateChange(unit, action string) bool {
	isActive := a.isServiceActive(unit)

	switch action {
	case "start":
		return !isActive // Changed if currently inactive
	case "stop":
		return isActive // Changed if currently active
	}
	return true
}

// isServiceEnabled checks if a service is enabled.
func (a *SystemdAction) isServiceEnabled(unit string) bool {
	cmd := exec.Command("systemctl", "is-enabled", unit)
	output, _ := cmd.Output()
	return strings.TrimSpace(string(output)) == "enabled"
}

// isServiceActive checks if a service is active.
func (a *SystemdAction) isServiceActive(unit string) bool {
	cmd := exec.Command("systemctl", "is-active", unit)
	output, _ := cmd.Output()
	return strings.TrimSpace(string(output)) == "active"
}
