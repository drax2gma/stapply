package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"syscall"

	"github.com/drax2gma/stapply/internal/protocol"
	"github.com/drax2gma/stapply/internal/security"
	"github.com/nats-io/nats.go"
)

func handleUpdate(msg *nats.Msg, agentID string, nc *nats.Conn, secretKey string) {
	data := msg.Data
	if secretKey != "" {
		var err error
		data, err = security.Decrypt(msg.Data, secretKey)
		if err != nil {
			log.Printf("Failed to decrypt update request: %v", err)
			return
		}
	}

	var req protocol.UpdateRequest
	if err := json.Unmarshal(data, &req); err != nil {
		log.Printf("Invalid update request: %v", err)
		return
	}

	log.Printf("🔄 Update requested: %s -> %s", Version, req.TargetVersion)

	// Check if already at target version
	if Version == req.TargetVersion {
		resp := &protocol.UpdateResponse{
			RequestID: req.RequestID,
			Success:   true,
			Message:   "Agent already at target version",
		}
		sendUpdateResponse(msg, resp, secretKey)
		return
	}

	// Determine target path based on current executable
	executable, err := os.Executable()
	if err != nil {
		log.Printf("⚠️  Failed to get executable path, defaulting to /usr/local/bin/stapply-agent: %v", err)
		executable = "/usr/local/bin/stapply-agent"
	}

	// Resolve symlinks if any
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		log.Printf("⚠️  Failed to resolve symlinks: %v", err)
	}

	// Download new binary to a temporary file on the SAME filesystem
	tmpPath := executable + ".new"
	if err := downloadBinary(req.BinaryURL, tmpPath); err != nil {
		log.Printf("❌ Failed to download binary: %v", err)
		resp := &protocol.UpdateResponse{
			RequestID: req.RequestID,
			Success:   false,
			Error:     fmt.Sprintf("download failed: %v", err),
		}
		sendUpdateResponse(msg, resp, secretKey)
		return
	}

	// Make it executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		log.Printf("❌ Failed to chmod binary: %v", err)
		os.Remove(tmpPath) // Cleanup
		resp := &protocol.UpdateResponse{
			RequestID: req.RequestID,
			Success:   false,
			Error:     fmt.Sprintf("chmod failed: %v", err),
		}
		sendUpdateResponse(msg, resp, secretKey)
		return
	}

	// Replace the binary (atomic rename on same FS)
	if err := os.Rename(tmpPath, executable); err != nil {
		log.Printf("❌ Failed to replace binary: %v", err)
		os.Remove(tmpPath) // Cleanup
		resp := &protocol.UpdateResponse{
			RequestID: req.RequestID,
			Success:   false,
			Error:     fmt.Sprintf("replace failed: %v", err),
		}
		sendUpdateResponse(msg, resp, secretKey)
		return
	}

	// Send success response before exiting
	resp := &protocol.UpdateResponse{
		RequestID: req.RequestID,
		Success:   true,
		Message:   fmt.Sprintf("Updated to %s, restarting...", req.TargetVersion),
	}
	sendUpdateResponse(msg, resp, secretKey)

	log.Printf("✅ Binary replaced")

	// Drain NATS connection
	nc.Drain()

	// Check if running under systemd
	if isRunningUnderSystemd() {
		log.Printf("Running under systemd, exiting for restart...")
		os.Exit(0)
	} else {
		log.Printf("Not running under systemd, restarting in-place...")
		// Get current executable path and args
		executable, err := os.Executable()
		if err != nil {
			log.Printf("Failed to get executable path: %v", err)
			os.Exit(1)
		}

		// Restart using execve (replace current process)
		err = syscall.Exec(executable, os.Args, os.Environ())
		if err != nil {
			log.Printf("Failed to restart: %v", err)
			os.Exit(1)
		}
	}
}

func sendUpdateResponse(msg *nats.Msg, resp *protocol.UpdateResponse, secretKey string) {
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Failed to marshal update response: %v", err)
		return
	}

	if secretKey != "" {
		data, err = security.Encrypt(data, secretKey)
		if err != nil {
			log.Printf("Failed to encrypt update response: %v", err)
			return
		}
	}

	if err := msg.Respond(data); err != nil {
		log.Printf("Failed to send update response: %v", err)
	}
}

func downloadBinary(url, destPath string) error {
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	_, err = io.Copy(out, resp.Body)
	return err
}

// isRunningUnderSystemd checks if the agent is running under systemd
func isRunningUnderSystemd() bool {
	// Check for INVOCATION_ID environment variable (set by systemd)
	if os.Getenv("INVOCATION_ID") != "" {
		return true
	}

	// Check if parent process is systemd (PID 1 or name contains "systemd")
	ppid := os.Getppid()
	if ppid == 1 {
		return true
	}

	return false
}
