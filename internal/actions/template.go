package actions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"text/template"
	"time"

	"github.com/drax2gma/stapply/internal/protocol"
)

// TemplateFileAction renders Go templates to files.
type TemplateFileAction struct{}

// Execute renders a template and writes to file with change detection.
func (a *TemplateFileAction) Execute(requestID string, args map[string]string, dryRun bool) *protocol.RunResponse {
	start := time.Now()

	// Validate required args
	path, ok := args["path"]
	if !ok || path == "" {
		return protocol.NewErrorResponse(requestID,
			&ActionError{Action: "template_file", Err: ErrMissingArg("path")}, 0)
	}

	templateText, ok := args["template"]
	if !ok || templateText == "" {
		return protocol.NewErrorResponse(requestID,
			&ActionError{Action: "template_file", Err: ErrMissingArg("template")}, 0)
	}

	// Parse template
	tmpl, err := template.New(path).Parse(templateText)
	if err != nil {
		return protocol.NewErrorResponse(requestID,
			fmt.Errorf("template parse error: %w", err), time.Since(start).Milliseconds())
	}

	// Parse vars (JSON map)
	vars := make(map[string]interface{})
	if varsJSON, ok := args["vars"]; ok && varsJSON != "" {
		if err := json.Unmarshal([]byte(varsJSON), &vars); err != nil {
			return protocol.NewErrorResponse(requestID,
				fmt.Errorf("vars parse error: %w", err), time.Since(start).Milliseconds())
		}
	}

	// Render template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return protocol.NewErrorResponse(requestID,
			fmt.Errorf("template execute error: %w", err), time.Since(start).Milliseconds())
	}

	renderedContent := buf.String()

	// Compute hash of rendered content
	newHash := computeHash([]byte(renderedContent))

	if dryRun {
		// Check change status
		changed := true
		if existingContent, err := os.ReadFile(path); err == nil {
			existingHash := computeHash(existingContent)
			if existingHash == newHash {
				changed = false
			}
		}

		statusMsg := "Dry run: Content match"
		if changed {
			statusMsg = "Dry run: Would render template to file"
		}

		return protocol.NewRunResponse(
			requestID,
			changed,
			0,
			statusMsg,
			"",
			time.Since(start).Milliseconds(),
		)
	}

	// Check if file exists and compare hash
	changed := true
	if existingContent, err := os.ReadFile(path); err == nil {
		existingHash := computeHash(existingContent)
		if existingHash == newHash {
			changed = false
		}
	}

	// Write file if changed
	if changed {
		if err := os.WriteFile(path, []byte(renderedContent), 0644); err != nil {
			return protocol.NewErrorResponse(requestID, err, time.Since(start).Milliseconds())
		}
	}

	// Apply mode if specified
	if mode, ok := args["mode"]; ok && mode != "" {
		if err := applyMode(path, mode); err != nil {
			return protocol.NewErrorResponse(requestID, err, time.Since(start).Milliseconds())
		}
	}

	return protocol.NewRunResponse(
		requestID,
		changed,
		0,
		"",
		"",
		time.Since(start).Milliseconds(),
	)
}
