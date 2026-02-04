package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/drax2gma/stapply/internal/protocol"
	"github.com/nats-io/nats.go"
)

func handleUpdate(msg *nats.Msg, agentID string, nc *nats.Conn) {
	var req protocol.UpdateRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
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
		sendUpdateResponse(msg, resp)
		return
	}

	// Download new binary
	tmpPath := "/tmp/sapply-agent-new"
	if err := downloadBinary(req.BinaryURL, tmpPath); err != nil {
		log.Printf("❌ Failed to download binary: %v", err)
		resp := &protocol.UpdateResponse{
			RequestID: req.RequestID,
			Success:   false,
			Error:     fmt.Sprintf("download failed: %v", err),
		}
		sendUpdateResponse(msg, resp)
		return
	}

	// Make it executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		log.Printf("❌ Failed to chmod binary: %v", err)
		resp := &protocol.UpdateResponse{
			RequestID: req.RequestID,
			Success:   false,
			Error:     fmt.Sprintf("chmod failed: %v", err),
		}
		sendUpdateResponse(msg, resp)
		return
	}

	// Replace the binary
	binaryPath := "/usr/local/bin/sapply-agent"
	if err := os.Rename(tmpPath, binaryPath); err != nil {
		log.Printf("❌ Failed to replace binary: %v", err)
		resp := &protocol.UpdateResponse{
			RequestID: req.RequestID,
			Success:   false,
			Error:     fmt.Sprintf("replace failed: %v", err),
		}
		sendUpdateResponse(msg, resp)
		return
	}

	// Send success response before exiting
	resp := &protocol.UpdateResponse{
		RequestID: req.RequestID,
		Success:   true,
		Message:   fmt.Sprintf("Updated to %s, restarting...", req.TargetVersion),
	}
	sendUpdateResponse(msg, resp)

	log.Printf("✅ Binary replaced, exiting for systemd restart...")

	// Drain and close NATS connection
	nc.Drain()

	// Exit - systemd will restart us
	os.Exit(0)
}

func sendUpdateResponse(msg *nats.Msg, resp *protocol.UpdateResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Failed to marshal update response: %v", err)
		return
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
