package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/drax2gma/stapply/internal/config"
	"github.com/drax2gma/stapply/internal/netutil"
	"github.com/drax2gma/stapply/internal/protocol"
	"github.com/nats-io/nats.go"
)

const Version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "ping":
		cmdPing(os.Args[2:])
	case "run":
		cmdRun(os.Args[2:])
	case "adhoc":
		cmdAdhoc(os.Args[2:])
	case "version":
		fmt.Printf("sapply-ctl version %s\n", Version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Usage: sapply-ctl <command> [options]

Commands:
  ping <agent_id>              Ping an agent
  run -c <config> -e <env>     Execute apps on environment
  adhoc -e <env> <action> ...  Execute single action on environment
  version                      Show version
  help                         Show this help

Global Options:
  -nats <server>               NATS server (default: localhost)`)
}

func cmdPing(args []string) {
	fs := flag.NewFlagSet("ping", flag.ExitOnError)
	natsURL := fs.String("nats", "nats://localhost:4222", "NATS server (FQDN or IP)")
	allowPublic := fs.Bool("allow-public", false, "Allow connection to public NATS servers")
	timeout := fs.Duration("timeout", 5*time.Second, "Request timeout")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: sapply-ctl ping <agent_id>")
		os.Exit(1)
	}

	agentID := fs.Arg(0)

	// Validate NATS URL
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

	// Create ping request
	req := protocol.NewPingRequest()
	data, err := json.Marshal(req)
	if err != nil {
		log.Fatalf("Failed to marshal request: %v", err)
	}

	// Send request
	subject := "sapply.ping." + agentID
	msg, err := nc.Request(subject, data, *timeout)
	if err != nil {
		if err == nats.ErrTimeout {
			fmt.Printf("❌ Agent %s: timeout (no response within %s)\n", agentID, *timeout)
			os.Exit(1)
		}
		log.Fatalf("Request failed: %v", err)
	}

	// Parse response
	var resp protocol.PingResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		log.Fatalf("Failed to parse response: %v", err)
	}

	fmt.Printf("✅ Agent %s: version=%s uptime=%ds\n",
		resp.AgentID, resp.Version, resp.UptimeSeconds)
}

func cmdAdhoc(args []string) {
	fs := flag.NewFlagSet("adhoc", flag.ExitOnError)
	configPath := fs.String("c", "", "Path to configuration file")
	envName := fs.String("e", "", "Environment name")
	natsURL := fs.String("nats", "nats://localhost:4222", "NATS server URL")
	allowPublic := fs.Bool("allow-public", false, "Allow connection to public NATS servers")
	timeout := fs.Duration("timeout", 30*time.Second, "Request timeout")
	fs.Parse(args)

	// Validate NATS URL
	*natsURL = netutil.NormalizeNATSURL(*natsURL)
	if err := netutil.ValidateNATSURL(*natsURL, *allowPublic); err != nil {
		log.Fatalf("NATS URL validation failed: %v", err)
	}

	if *configPath == "" || *envName == "" {
		fmt.Fprintln(os.Stderr, "Usage: sapply-ctl adhoc -c <config> -e <env> <action> <args...>")
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Error: action required")
		fmt.Fprintln(os.Stderr, "Usage: sapply-ctl adhoc -c <config> -e <env> <action> <args...>")
		os.Exit(1)
	}

	action := fs.Arg(0)
	actionArgs := strings.Join(fs.Args()[1:], " ")

	// Parse configuration
	cfg, err := config.Parse(*configPath)
	if err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

	// Get environment
	env, ok := cfg.Environments[*envName]
	if !ok {
		log.Fatalf("Environment not found: %s", *envName)
	}

	// Build args map based on action type
	stepArgs := make(map[string]string)
	switch action {
	case "cmd":
		stepArgs["command"] = actionArgs
	case "systemd":
		parts := strings.Fields(actionArgs)
		if len(parts) >= 1 {
			stepArgs["action"] = parts[0]
		}
		if len(parts) >= 2 {
			stepArgs["unit"] = parts[1]
		}
	default:
		stepArgs["args"] = actionArgs
	}

	// Connect to NATS
	nc, err := nats.Connect(*natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	fmt.Printf("🚀 Ad-hoc: %s %s\n", action, actionArgs)
	fmt.Printf("   Environment: %s\n", *envName)
	fmt.Printf("   Hosts: %v\n", env.Hosts)
	fmt.Println()

	// Execute on each host in parallel (using same pattern as cmdRun)
	concurrency := env.Concurrency
	if concurrency <= 0 {
		concurrency = len(env.Hosts)
	}

	type result struct {
		ok      int
		changed int
		failed  int
	}
	resultCh := make(chan result, len(env.Hosts))
	semaphore := make(chan struct{}, concurrency)

	for _, hostID := range env.Hosts {
		semaphore <- struct{}{}

		go func(hID string) {
			defer func() { <-semaphore }()

			var ok, changed, failed int

			host, exists := cfg.Hosts[hID]
			if !exists {
				fmt.Printf("⚠️  Host not found: %s\n", hID)
				resultCh <- result{failed: 1}
				return
			}

			agentID := host.AgentID
			if agentID == "" {
				agentID = hID
			}

			fmt.Printf("📦 Host: %s (agent_id=%s)\n", hID, agentID)

			req := protocol.NewRunRequest(action, stepArgs, int(*timeout/time.Millisecond))
			data, err := json.Marshal(req)
			if err != nil {
				fmt.Printf("   ❌ Marshal error: %v\n", err)
				resultCh <- result{failed: 1}
				return
			}

			subject := "sapply.run." + agentID
			msg, err := nc.Request(subject, data, *timeout)
			if err != nil {
				if err == nats.ErrTimeout {
					fmt.Printf("   ❌ Timeout\n")
				} else {
					fmt.Printf("   ❌ Error: %v\n", err)
				}
				resultCh <- result{failed: 1}
				return
			}

			var resp protocol.RunResponse
			if err := json.Unmarshal(msg.Data, &resp); err != nil {
				fmt.Printf("   ❌ Response parse error: %v\n", err)
				resultCh <- result{failed: 1}
				return
			}

			switch resp.Status {
			case protocol.StatusOK:
				if resp.Changed {
					fmt.Printf("   ✅ Changed (%dms)\n", resp.DurationMs)
					if resp.Stdout != "" {
						fmt.Printf("   %s\n", strings.TrimSpace(resp.Stdout))
					}
					changed++
				} else {
					fmt.Printf("   ✅ OK (%dms)\n", resp.DurationMs)
					if resp.Stdout != "" {
						fmt.Printf("   %s\n", strings.TrimSpace(resp.Stdout))
					}
					ok++
				}
			case protocol.StatusFailed:
				fmt.Printf("   ❌ Failed (exit=%d)\n", resp.ExitCode)
				if resp.Stderr != "" {
					fmt.Printf("   %s\n", strings.TrimSpace(resp.Stderr))
				}
				failed++
			case protocol.StatusError:
				fmt.Printf("   ❌ Error: %s\n", resp.Error)
				failed++
			}

			resultCh <- result{ok: ok, changed: changed, failed: failed}
		}(hostID)
	}

	// Wait for all hosts to complete
	var okCount, changedCount, failedCount int
	for i := 0; i < len(env.Hosts); i++ {
		r := <-resultCh
		okCount += r.ok
		changedCount += r.changed
		failedCount += r.failed
	}

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Summary: ok=%d changed=%d failed=%d\n", okCount, changedCount, failedCount)

	if failedCount > 0 {
		os.Exit(1)
	}
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("c", "", "Path to configuration file")
	envName := fs.String("e", "", "Environment name")
	natsURL := fs.String("nats", "nats://localhost:4222", "NATS server URL")
	allowPublic := fs.Bool("allow-public", false, "Allow connection to public NATS servers")
	timeout := fs.Duration("timeout", 30*time.Second, "Request timeout")
	fs.Parse(args)

	// Validate NATS URL
	*natsURL = netutil.NormalizeNATSURL(*natsURL)
	if err := netutil.ValidateNATSURL(*natsURL, *allowPublic); err != nil {
		log.Fatalf("NATS URL validation failed: %v", err)
	}

	if *configPath == "" || *envName == "" {
		fmt.Fprintln(os.Stderr, "Usage: sapply-ctl run -c <config> -e <env>")
		os.Exit(1)
	}

	// Parse configuration
	cfg, err := config.Parse(*configPath)
	if err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

	// Get environment
	env, ok := cfg.Environments[*envName]
	if !ok {
		log.Fatalf("Environment not found: %s", *envName)
	}

	// Connect to NATS
	nc, err := nats.Connect(*natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	fmt.Printf("🚀 Executing environment: %s\n", *envName)
	fmt.Printf("   Hosts: %v\n", env.Hosts)
	fmt.Printf("   Apps: %v\n", env.Apps)
	fmt.Println()

	// Determine concurrency limit
	concurrency := env.Concurrency
	if concurrency <= 0 {
		concurrency = len(env.Hosts) // No limit, run all in parallel
	}

	// Channel for collecting results
	type result struct {
		ok      int
		changed int
		failed  int
	}
	resultCh := make(chan result, len(env.Hosts))

	// Semaphore for concurrency control
	semaphore := make(chan struct{}, concurrency)

	// Execute hosts in parallel
	for _, hostID := range env.Hosts {
		// Acquire semaphore
		semaphore <- struct{}{}

		go func(hID string) {
			defer func() { <-semaphore }() // Release semaphore

			var ok, changed, failed int

			host, exists := cfg.Hosts[hID]
			if !exists {
				fmt.Printf("⚠️  Host not found: %s\n", hID)
				resultCh <- result{failed: 1}
				return
			}

			agentID := host.AgentID
			if agentID == "" {
				agentID = hID
			}

			fmt.Printf("📦 Host: %s (agent_id=%s)\n", hID, agentID)

			// Execute each app
			for _, appName := range env.Apps {
				app, appExists := cfg.Apps[appName]
				if !appExists {
					fmt.Printf("   ⚠️  App not found: %s\n", appName)
					failed++
					continue
				}

				fmt.Printf("   📋 App: %s\n", appName)

				steps := app.GetOrderedSteps()
				for i, step := range steps {
					fmt.Printf("      Step %d: %s\n", i+1, step.Action)

					// Use parsed args from step
					stepArgs := step.ArgsMap
					if stepArgs == nil {
						stepArgs = make(map[string]string)
					}

					req := protocol.NewRunRequest(step.Action, stepArgs, int(*timeout/time.Millisecond))
					data, err := json.Marshal(req)
					if err != nil {
						fmt.Printf("         ❌ Marshal error: %v\n", err)
						failed++
						continue
					}

					subject := "sapply.run." + agentID
					msg, err := nc.Request(subject, data, *timeout)
					if err != nil {
						if err == nats.ErrTimeout {
							fmt.Printf("         ❌ Timeout\n")
						} else {
							fmt.Printf("         ❌ Error: %v\n", err)
						}
						failed++
						continue
					}

					var resp protocol.RunResponse
					if err := json.Unmarshal(msg.Data, &resp); err != nil {
						fmt.Printf("         ❌ Response parse error: %v\n", err)
						failed++
						continue
					}

					switch resp.Status {
					case protocol.StatusOK:
						if resp.Changed {
							fmt.Printf("         ✅ Changed (%dms)\n", resp.DurationMs)
							changed++
						} else {
							fmt.Printf("         ✅ OK (%dms)\n", resp.DurationMs)
							ok++
						}
					case protocol.StatusFailed:
						fmt.Printf("         ❌ Failed (exit=%d): %s\n", resp.ExitCode, resp.Stderr)
						failed++
					case protocol.StatusError:
						fmt.Printf("         ❌ Error: %s\n", resp.Error)
						failed++
					}
				}
			}
			fmt.Println()

			resultCh <- result{ok: ok, changed: changed, failed: failed}
		}(hostID)
	}

	// Wait for all hosts to complete
	var okCount, changedCount, failedCount int
	for i := 0; i < len(env.Hosts); i++ {
		r := <-resultCh
		okCount += r.ok
		changedCount += r.changed
		failedCount += r.failed
	}

	// Print summary
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Summary: ok=%d changed=%d failed=%d\n", okCount, changedCount, failedCount)

	if failedCount > 0 {
		os.Exit(1)
	}
}
