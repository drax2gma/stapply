package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/drax2gma/stapply/internal/actions"
	"github.com/drax2gma/stapply/internal/config"
	"github.com/drax2gma/stapply/internal/protocol"
	"github.com/nats-io/nats.go"
)

const Version = "0.1.0"

var startTime = time.Now()

func main() {
	configPath := flag.String("config", "/etc/sapply/agent.ini", "Path to agent configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.ParseAgentConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if cfg.AgentID == "" {
		log.Fatal("agent_id is required in configuration")
	}

	log.Printf("Starting sapply-agent version %s (agent_id=%s)", Version, cfg.AgentID)

	// Connect to NATS
	opts := []nats.Option{
		nats.Name("sapply-agent-" + cfg.AgentID),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1), // Unlimited reconnects
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			if err != nil {
				log.Printf("Disconnected from NATS: %v", err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("Reconnected to NATS at %s", nc.ConnectedUrl())
		}),
	}

	if cfg.NatsCreds != "" {
		opts = append(opts, nats.UserCredentials(cfg.NatsCreds))
	}

	nc, err := nats.Connect(cfg.NatsURL, opts...)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	log.Printf("Connected to NATS at %s", nc.ConnectedUrl())

	// Initialize action registry
	registry := actions.NewRegistry()

	// Subscribe to ping requests
	pingSubject := "sapply.ping." + cfg.AgentID
	_, err = nc.Subscribe(pingSubject, func(msg *nats.Msg) {
		handlePing(msg, cfg.AgentID)
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to %s: %v", pingSubject, err)
	}
	log.Printf("Subscribed to %s", pingSubject)

	// Subscribe to run requests
	runSubject := "sapply.run." + cfg.AgentID
	_, err = nc.Subscribe(runSubject, func(msg *nats.Msg) {
		handleRun(msg, registry)
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to %s: %v", runSubject, err)
	}
	log.Printf("Subscribed to %s", runSubject)

	// Wait for shutdown signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("Received signal %v, shutting down...", sig)
	case <-ctx.Done():
	}

	// Drain connections before exit
	if err := nc.Drain(); err != nil {
		log.Printf("Error draining NATS connection: %v", err)
	}

	log.Println("Agent stopped")
}

func handlePing(msg *nats.Msg, agentID string) {
	var req protocol.PingRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("Invalid ping request: %v", err)
		return
	}

	resp := protocol.NewPingResponse(
		req.RequestID,
		agentID,
		Version,
		int64(time.Since(startTime).Seconds()),
	)

	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Failed to marshal ping response: %v", err)
		return
	}

	if err := msg.Respond(data); err != nil {
		log.Printf("Failed to send ping response: %v", err)
	}
}

func handleRun(msg *nats.Msg, registry *actions.Registry) {
	var req protocol.RunRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("Invalid run request: %v", err)
		return
	}

	log.Printf("Executing action: %s (request_id=%s)", req.Action, req.RequestID)

	resp := registry.Execute(req.RequestID, req.Action, req.Args)

	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Failed to marshal run response: %v", err)
		return
	}

	if err := msg.Respond(data); err != nil {
		log.Printf("Failed to send run response: %v", err)
	}

	log.Printf("Action %s completed: status=%s changed=%v duration=%dms",
		req.Action, resp.Status, resp.Changed, resp.DurationMs)
}
