package actions

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/drax2gma/stapply/internal/protocol"
)

// DeployArtifactAction handles chunked binary transfer.
// It is designed to be stateless regarding request handling, but stateful on disk.
type DeployArtifactAction struct {
	// fileLocks prevents concurrent writes to the same file from multiple goroutines (if any)
	fileLocks sync.Map // map[string]*sync.Mutex
}

func (a *DeployArtifactAction) Execute(requestID string, args map[string]string, dryRun bool) *protocol.RunResponse {
	// Parse arguments
	destPath := args["dest"]
	if destPath == "" {
		return protocol.NewErrorResponse(requestID, fmt.Errorf("missing 'dest' argument"), 0)
	}

	chunkDataB64 := args["chunk_data"]
	if chunkDataB64 == "" {
		return protocol.NewErrorResponse(requestID, fmt.Errorf("missing 'chunk_data' argument"), 0)
	}

	chunkIndexStr := args["chunk_index"]
	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		return protocol.NewErrorResponse(requestID, fmt.Errorf("invalid 'chunk_index': %v", err), 0)
	}

	totalChunksStr := args["total_chunks"]
	totalChunks, err := strconv.Atoi(totalChunksStr)
	if err != nil {
		return protocol.NewErrorResponse(requestID, fmt.Errorf("invalid 'total_chunks': %v", err), 0)
	}

	checksum := args["checksum"] // SHA256 of the *entire* file
	modeStr := args["mode"]
	mode := os.FileMode(0644)
	if modeStr != "" {
		if m, err := strconv.ParseUint(modeStr, 8, 32); err == nil {
			mode = os.FileMode(m)
		}
	}

	if dryRun {
		return protocol.NewRunResponse(requestID, false, 0,
			fmt.Sprintf("Would write chunk %d/%d to %s", chunkIndex+1, totalChunks, destPath), "", 0)
	}

	// Lock based on destination path to avoid race conditions if requests come in parallel (though unexpected for same file)
	lockVal, _ := a.fileLocks.LoadOrStore(destPath, &sync.Mutex{})
	mutex := lockVal.(*sync.Mutex)
	mutex.Lock()
	defer mutex.Unlock()

	// Decode data
	data, err := base64.StdEncoding.DecodeString(chunkDataB64)
	if err != nil {
		return protocol.NewErrorResponse(requestID, fmt.Errorf("base64 decode failed: %v", err), 0)
	}

	// Prepare file flags
	flags := os.O_CREATE | os.O_WRONLY
	if chunkIndex == 0 {
		// First chunk: Truncate file
		flags |= os.O_TRUNC
		// Create directory if not exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return protocol.NewErrorResponse(requestID, fmt.Errorf("failed to create directory: %v", err), 0)
		}
	} else {
		// Subsequent chunks: Append
		flags |= os.O_APPEND
	}

	f, err := os.OpenFile(destPath, flags, mode)
	if err != nil {
		return protocol.NewErrorResponse(requestID, fmt.Errorf("failed to open file: %v", err), 0)
	}
	defer f.Close()

	// Write chunk
	if _, err := f.Write(data); err != nil {
		return protocol.NewErrorResponse(requestID, fmt.Errorf("failed to write chunk: %v", err), 0)
	}

	// Per-chunk success message
	msg := fmt.Sprintf("Received chunk %d/%d (%d bytes)", chunkIndex+1, totalChunks, len(data))

	// Final verification
	if chunkIndex == totalChunks-1 {
		// Close file to flush writes before reading back
		f.Close()

		if checksum != "" {
			hashStr, err := calculateSHA256(destPath)
			if err != nil {
				return protocol.NewErrorResponse(requestID, fmt.Errorf("failed to calculate checksum: %v", err), 0)
			}
			if hashStr != checksum {
				return protocol.NewErrorResponse(requestID, fmt.Errorf("checksum mismatch: expected %s, got %s", checksum, hashStr), 0)
			}
			msg += " - Checksum Verified ✅"
		}

		// Clean up lock (optional, keeps map form growing indefinitely)
		a.fileLocks.Delete(destPath)
	}

	return protocol.NewRunResponse(requestID, true, 0, msg, "", 0)
}

func calculateSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
