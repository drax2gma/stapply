package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/drax2gma/stapply/internal/config"
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
  version                      Show version
  help                         Show this help

Global Options:
  -nats <url>                  NATS server URL (default: nats://localhost:4222)`)
}

func cmdPing(args []string) {
	fs := flag.NewFlagSet("ping", flag.ExitOnError)
	natsURL := fs.String("nats", "nats://localhost:4222", "NATS server URL")
	timeout := fs.Duration("timeout", 5*time.Second, "Request timeout")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: sapply-ctl ping <agent_id>")
		os.Exit(1)
	}

	agentID := fs.Arg(0)

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

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("c", "", "Path to configuration file")
	envName := fs.String("e", "", "Environment name")
	natsURL := fs.String("nats", "nats://localhost:4222", "NATS server URL")
	timeout := fs.Duration("timeout", 30*time.Second, "Request timeout")
	fs.Parse(args)

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

	// Track results
	var okCount, changedCount, failedCount int

	// Execute for each host
	for _, hostID := range env.Hosts {
		host, ok := cfg.Hosts[hostID]
		if !ok {
			fmt.Printf("⚠️  Host not found: %s\n", hostID)
			failedCount++
			continue
		}

		agentID := host.AgentID
		if agentID == "" {
			agentID = hostID
		}

		fmt.Printf("📦 Host: %s (agent_id=%s)\n", hostID, agentID)

		// Execute each app
		for _, appName := range env.Apps {
			app, ok := cfg.Apps[appName]
			if !ok {
				fmt.Printf("   ⚠️  App not found: %s\n", appName)
				failedCount++
				continue
			}

			fmt.Printf("   📋 App: %s\n", appName)

			steps := app.GetOrderedSteps()
			for i, step := range steps {
				fmt.Printf("      Step %d: %s\n", i+1, step.Action)

				// Build args from step
				stepArgs := make(map[string]string)
				stepArgs["command"] = step.Args // For cmd action

				req := protocol.NewRunRequest(step.Action, stepArgs, int(*timeout/time.Millisecond))
				data, err := json.Marshal(req)
				if err != nil {
					fmt.Printf("         ❌ Marshal error: %v\n", err)
					failedCount++
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
					failedCount++
					continue
				}

				var resp protocol.RunResponse
				if err := json.Unmarshal(msg.Data, &resp); err != nil {
					fmt.Printf("         ❌ Response parse error: %v\n", err)
					failedCount++
					continue
				}

				switch resp.Status {
				case protocol.StatusOK:
					if resp.Changed {
						fmt.Printf("         ✅ Changed (%dms)\n", resp.DurationMs)
						changedCount++
					} else {
						fmt.Printf("         ✅ OK (%dms)\n", resp.DurationMs)
						okCount++
					}
				case protocol.StatusFailed:
					fmt.Printf("         ❌ Failed (exit=%d): %s\n", resp.ExitCode, resp.Stderr)
					failedCount++
				case protocol.StatusError:
					fmt.Printf("         ❌ Error: %s\n", resp.Error)
					failedCount++
				}
			}
		}
		fmt.Println()
	}

	// Print summary
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Summary: ok=%d changed=%d failed=%d\n", okCount, changedCount, failedCount)

	if failedCount > 0 {
		os.Exit(1)
	}
}
