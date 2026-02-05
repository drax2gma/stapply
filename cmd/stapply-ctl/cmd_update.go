package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/drax2gma/stapply/internal/netutil"
	"github.com/drax2gma/stapply/internal/protocol"
	"github.com/nats-io/nats.go"
)

func cmdUpdate(args []string) {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	defaultNats := getDefaultNATSURL()
	natsURL := fs.String("nats", defaultNats, "NATS server (FQDN or IP)")
	allowPublic := fs.Bool("allow-public", false, "Allow connection to public NATS servers")
	timeout := fs.Duration("timeout", 30*time.Second, "Request timeout")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: stapply-ctl update <agent_id>")
		os.Exit(1)
	}

	agentID := fs.Arg(0)

	// Default NATS URL to agent_id if not specified
	if *natsURL == "" {
		*natsURL = agentID
	}

	// Validate and normalize NATS URL
	*natsURL = netutil.NormalizeNATSURL(*natsURL)
	if err := netutil.ValidateNATSURL(*natsURL, *allowPublic); err != nil {
		log.Fatalf("NATS URL validation failed: %v", err)
	}

	// Connect to NATS
	nc, err := nats.Connect(*natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	fmt.Printf("🔄 Updating agent %s to version %s\n", agentID, Version)

	// Build binary URL (repo-based distribution)
	binaryURL := "https://raw.githubusercontent.com/drax2gma/stapply/main/bin/stapply-agent"

	// Create update request
	req := protocol.NewUpdateRequest(Version, binaryURL)
	data, err := json.Marshal(req)
	if err != nil {
		log.Fatalf("Failed to marshal request: %v", err)
	}

	// Send update request
	subject := "stapply.update." + agentID
	msg, err := nc.Request(subject, data, *timeout)
	if err != nil {
		if err == nats.ErrTimeout {
			fmt.Printf("❌ Agent %s: timeout (no response within %s)\n", agentID, *timeout)
			os.Exit(1)
		}
		log.Fatalf("Request failed: %v", err)
	}

	// Parse response
	var resp protocol.UpdateResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		log.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Success {
		fmt.Printf("✅ %s\n", resp.Message)
	} else {
		fmt.Printf("❌ Update failed: %s\n", resp.Error)
		os.Exit(1)
	}
}
