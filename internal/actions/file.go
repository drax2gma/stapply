package actions

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/drax2gma/stapply/internal/protocol"
)

// WriteFileAction writes content to a file with change detection.
type WriteFileAction struct{}

// Execute writes a file and detects changes via hash comparison.
func (a *WriteFileAction) Execute(requestID string, args map[string]string, dryRun bool) *protocol.RunResponse {
	start := time.Now()

	// Validate required args
	path, ok := args["path"]
	if !ok || path == "" {
		return protocol.NewErrorResponse(requestID,
			&ActionError{Action: "write_file", Err: ErrMissingArg("path")}, 0)
	}

	content, ok := args["content"]
	if !ok {
		return protocol.NewErrorResponse(requestID,
			&ActionError{Action: "write_file", Err: ErrMissingArg("content")}, 0)
	}

	if dryRun {
		// Check if directory exists
		dir := filepath.Dir(path)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return protocol.NewErrorResponse(requestID,
				fmt.Errorf("dry run: directory %s does not exist", dir), time.Since(start).Milliseconds())
		}

		// Check if file exists to determine change status
		changed := true
		if existingContent, err := os.ReadFile(path); err == nil {
			newHash := computeHash([]byte(content))
			existingHash := computeHash(existingContent)
			if existingHash == newHash {
				changed = false
			}
		}

		statusMsg := "Dry run: Content match"
		if changed {
			statusMsg = "Dry run: Would update file content"
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

	// Compute hash of new content
	newHash := computeHash([]byte(content))

	// Check if directory exists for dry run
	if dryRun {
		// Check if directory exists
		dir := filepath.Dir(path)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return protocol.NewErrorResponse(requestID,
				fmt.Errorf("dry run: directory %s does not exist", dir), time.Since(start).Milliseconds())
		}

		// Check if file exists to determine change status
		changed := true
		if existingContent, err := os.ReadFile(path); err == nil {
			existingHash := computeHash(existingContent)
			if existingHash == newHash {
				changed = false
			}
		}

		statusMsg := "Dry run: Content match"
		if changed {
			statusMsg = "Dry run: Would update file content"
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

	// Write file if changed or doesn't exist
	if changed {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return protocol.NewErrorResponse(requestID, err, time.Since(start).Milliseconds())
		}
	}

	// Apply mode if specified
	if mode, ok := args["mode"]; ok && mode != "" {
		if err := applyMode(path, mode); err != nil {
			return protocol.NewErrorResponse(requestID, err, time.Since(start).Milliseconds())
		}
	}

	// Apply owner if specified
	if owner, ok := args["owner"]; ok && owner != "" {
		if err := applyOwner(path, owner); err != nil {
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

// computeHash computes SHA256 hash of data.
func computeHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// applyMode applies octal file mode.
func applyMode(path, modeStr string) error {
	mode, err := strconv.ParseUint(modeStr, 8, 32)
	if err != nil {
		return fmt.Errorf("invalid mode %q: %w", modeStr, err)
	}
	return os.Chmod(path, os.FileMode(mode))
}

// applyOwner applies user:group ownership.
func applyOwner(path, owner string) error {
	// Validate owner format (user:group)
	hasColon := false
	for _, ch := range owner {
		if ch == ':' {
			hasColon = true
			break
		}
	}
	if !hasColon {
		return fmt.Errorf("invalid owner format %q (expected user:group)", owner)
	}

	// Use chown command (requires appropriate permissions)
	cmd := exec.Command("chown", owner, path)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("chown failed: %w", err)
	}

	return nil
}

// getFileOwner retrieves current file ownership.
func getFileOwner(path string) (uid, gid int, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("cannot get file stat")
	}
	return int(stat.Uid), int(stat.Gid), nil
}
