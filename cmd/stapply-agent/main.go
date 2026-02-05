package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/drax2gma/stapply/internal/actions"
	"github.com/drax2gma/stapply/internal/config"
	"github.com/drax2gma/stapply/internal/netutil"
	"github.com/drax2gma/stapply/internal/protocol"
	"github.com/drax2gma/stapply/internal/security"
	"github.com/drax2gma/stapply/internal/sysinfo"
	"github.com/nats-io/nats.go"
)

var Version = "0.1.0-dev"

var (
	startTime = time.Now()
	cpuUsage  float64
	cpuMutex  sync.Mutex
)

func main() {
	configPath := flag.String("config", "/etc/stapply/agent.ini", "Path to agent configuration file")
	allowPublic := flag.Bool("allow-public", false, "Allow connection to public NATS servers (insecure)")
	flag.Parse()

	// Load configuration
	cfg, err := config.ParseAgentConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if cfg.AgentID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			log.Fatalf("agent_id is missing and could not determine hostname: %v", err)
		}
		cfg.AgentID = hostname
	}

	// Handle STAPPLY_DEFAULT_NATS fallback
	if cfg.NatsServer == "" {
		if val := os.Getenv("STAPPLY_DEFAULT_NATS"); val != "" {
			// Validate: Must have dots (FQDN) or be a valid IP
			if !strings.Contains(val, ".") && !strings.Contains(val, ":") {
				log.Fatalf("Invalid STAPPLY_DEFAULT_NATS: %q. Must be an FQDN with dots or an IP address.", val)
			}
			cfg.NatsServer = val
		} else {
			cfg.NatsServer = "localhost"
		}
	}

	// Validate NATS URL for network security
	natsURL := netutil.NormalizeNATSURL(cfg.NatsServer)
	if err := netutil.ValidateNATSURL(natsURL, *allowPublic); err != nil {
		log.Fatalf("NATS URL validation failed: %v", err)
	}

	log.Printf("Starting stapply-agent version %s (agent_id=%s)", Version, cfg.AgentID)

	// Connect to NATS
	opts := []nats.Option{
		nats.Name("stapply-agent-" + cfg.AgentID),
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

	nc, err := nats.Connect(natsURL, opts...)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	log.Printf("Connected to NATS at %s", nc.ConnectedUrl())

	// Initialize action registry
	registry := actions.NewRegistry()

	// Get secret key from environment
	secretKey := os.Getenv("STAPPLY_SHARED_KEY")
	if secretKey != "" {
		log.Printf("Encryption enabled (key provided via STAPPLY_SHARED_KEY)")
	}

	// Subscribe to ping requests
	pingSubject := "stapply.ping." + cfg.AgentID
	_, err = nc.Subscribe(pingSubject, func(msg *nats.Msg) {
		handlePing(msg, cfg.AgentID, secretKey)
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to %s: %v", pingSubject, err)
	}
	log.Printf("Subscribed to %s", pingSubject)

	// Subscribe to run requests
	runSubject := "stapply.run." + cfg.AgentID
	_, err = nc.Subscribe(runSubject, func(msg *nats.Msg) {
		handleRun(msg, registry, secretKey)
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to %s: %v", runSubject, err)
	}
	log.Printf("Subscribed to %s", runSubject)

	// Subscribe to update requests
	updateSubject := "stapply.update." + cfg.AgentID
	_, err = nc.Subscribe(updateSubject, func(msg *nats.Msg) {
		handleUpdate(msg, cfg.AgentID, nc, secretKey)
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to %s: %v", updateSubject, err)
	}
	log.Printf("Subscribed to %s", updateSubject)

	// Subscribe to discovery requests
	discoverSubject := "stapply.discover." + cfg.AgentID
	_, err = nc.Subscribe(discoverSubject, func(msg *nats.Msg) {
		handleDiscover(msg, cfg.AgentID, secretKey)
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to %s: %v", discoverSubject, err)
	}
	// Start CPU monitoring
	go monitorCPU()

	log.Printf("Subscribed to %s", discoverSubject)

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

func handlePing(msg *nats.Msg, agentID, secretKey string) {
	data := msg.Data
	if secretKey != "" {
		var err error
		data, err = security.Decrypt(msg.Data, secretKey)
		if err != nil {
			log.Printf("Failed to decrypt ping request: %v", err)
			return
		}
	}

	var req protocol.PingRequest
	if err := json.Unmarshal(data, &req); err != nil {
		log.Printf("Invalid ping request: %v", err)
		return
	}

	// Check version mismatch
	if req.ControllerVersion != "" && req.ControllerVersion != Version {
		log.Printf("⚠️  Version mismatch: agent=%s, controller=%s", Version, req.ControllerVersion)
		if req.ControllerVersion > Version {
			log.Printf("⚠️  Agent is outdated. Run 'stapply-ctl update %s' to update.", agentID)
		}
	}

	cpuMutex.Lock()
	cpu := cpuUsage
	cpuMutex.Unlock()

	mem := getMemoryUsagePercentage()

	resp := protocol.NewPingResponse(
		req.RequestID,
		agentID,
		Version,
		int64(time.Since(startTime).Seconds()),
		cpu,
		mem,
	)

	respData, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Failed to marshal ping response: %v", err)
		return
	}

	if secretKey != "" {
		respData, err = security.Encrypt(respData, secretKey)
		if err != nil {
			log.Printf("Failed to encrypt ping response: %v", err)
			return
		}
	}

	if err := msg.Respond(respData); err != nil {
		log.Printf("Failed to send ping response: %v", err)
	}
}

func handleRun(msg *nats.Msg, registry *actions.Registry, secretKey string) {
	data := msg.Data
	if secretKey != "" {
		var err error
		data, err = security.Decrypt(msg.Data, secretKey)
		if err != nil {
			log.Printf("Failed to decrypt run request: %v", err)
			return
		}
	}

	var req protocol.RunRequest
	if err := json.Unmarshal(data, &req); err != nil {
		log.Printf("Invalid run request: %v", err)
		return
	}

	log.Printf("Executing action: %s (request_id=%s)", req.Action, req.RequestID)

	resp := registry.Execute(req.RequestID, req.Action, req.Args, req.DryRun)

	respData, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Failed to marshal run response: %v", err)
		return
	}

	if secretKey != "" {
		respData, err = security.Encrypt(respData, secretKey)
		if err != nil {
			log.Printf("Failed to encrypt run response: %v", err)
			return
		}
	}

	if err := msg.Respond(respData); err != nil {
		log.Printf("Failed to send run response: %v", err)
	}

	log.Printf("Action %s completed: status=%s changed=%v duration=%dms",
		req.Action, resp.Status, resp.Changed, resp.DurationMs)
}

func handleDiscover(msg *nats.Msg, agentID, secretKey string) {
	data := msg.Data
	if secretKey != "" {
		var err error
		data, err = security.Decrypt(msg.Data, secretKey)
		if err != nil {
			log.Printf("Failed to decrypt discover request: %v", err)
			return
		}
	}

	var req protocol.DiscoverRequest
	if err := json.Unmarshal(data, &req); err != nil {
		log.Printf("Invalid discover request: %v", err)
		return
	}

	log.Printf("Discovery request received (request_id=%s)", req.RequestID)

	resp, err := sysinfo.GatherFacts(agentID)
	if err != nil {
		log.Printf("Failed to gather system facts: %v", err)
		return
	}
	resp.RequestID = req.RequestID

	respData, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Failed to marshal discover response: %v", err)
		return
	}

	if secretKey != "" {
		respData, err = security.Encrypt(respData, secretKey)
		if err != nil {
			log.Printf("Failed to encrypt discover response: %v", err)
			return
		}
	}

	if err := msg.Respond(respData); err != nil {
		log.Printf("Failed to send discover response: %v", err)
	}
}

func monitorCPU() {
	prevIdle := uint64(0)
	prevTotal := uint64(0)

	for {
		idle, total := getCPUSample()
		diffIdle := float64(idle - prevIdle)
		diffTotal := float64(total - prevTotal)

		if diffTotal > 0 && prevTotal > 0 {
			usage := (diffTotal - diffIdle) / diffTotal * 100
			cpuMutex.Lock()
			cpuUsage = usage
			cpuMutex.Unlock()
		}

		prevIdle = idle
		prevTotal = total

		time.Sleep(3 * time.Second)
	}
}

func getCPUSample() (idle, total uint64) {
	contents, err := os.ReadFile("/proc/stat")
	if err != nil {
		return
	}
	lines := strings.Split(string(contents), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "cpu" {
			numFields := len(fields)
			for i := 1; i < numFields; i++ {
				val, _ := strconv.ParseUint(fields[i], 10, 64)
				total += val
				if i == 4 { // idle is the 5th field (index 4)
					idle = val
				}
			}
			return
		}
	}
	return
}

func getMemoryUsagePercentage() float64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	var total, free uint64

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		key := parts[0]
		val := parts[1]
		var v uint64
		fmt.Sscanf(val, "%d", &v)

		switch key {
		case "MemTotal:":
			total = v
		case "MemAvailable:":
			free = v
		}
	}

	if total == 0 {
		return 0
	}

	used := total - free
	return float64(used) / float64(total) * 100
}
