package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"math"

	"github.com/drax2gma/stapply/internal/config"
	"github.com/drax2gma/stapply/internal/netutil"
	"github.com/drax2gma/stapply/internal/protocol"
	"github.com/drax2gma/stapply/internal/security"
	"github.com/nats-io/nats.go"
)

var Version = "0.1.0-dev"

func getDefaultNATSURL() string {
	val := os.Getenv("STAPPLY_DEFAULT_NATS")
	if val == "" {
		return ""
	}

	// Validate: Must have dots (FQDN) or be a valid IP
	// Simple check for dots first
	if !strings.Contains(val, ".") && !strings.Contains(val, ":") {
		// Single word like "localhost" is strictly forbidden by requirements "MUST be an FQDN with dots"
		// But wait, IPv6 has colons.
		log.Fatalf("Invalid STAPPLY_DEFAULT_NATS: %q. Must be an FQDN with dots or an IP address.", val)
	}

	// Further validation could use netutil but this basic check covers the specific user request "dots in it".
	return val
}

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
	case "update":
		cmdUpdate(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "discover":
		cmdDiscover(os.Args[2:])
	case "installer":
		cmdInstaller(os.Args[2:])
	case "installer-custom":
		cmdInstallerCustom(os.Args[2:])
	case "preflight":
		cmdPreflight(os.Args[2:])
	case "version":
		fmt.Printf("stapply-ctl version %s\n", Version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	const (
		Reset  = "\033[0m"
		Bold   = "\033[1m"
		Dim    = "\033[2m"
		Cyan   = "\033[36m"
		Yellow = "\033[33m"
	)

	fmt.Printf(`%sStapply Controller CLI%s %s%s

%sUsage:%s
  stapply-ctl <command> [flags]

%sCore Commands:%s
  %srun%s       -c <cfg> -e <env>      Execute full deployment plan
  %spreflight%s -c <cfg> -e <env>      Dry-run with system health checks
  %sadhoc%s     -e <target> <action>   Execute single ad-hoc action
  %sping%s      <agent_id>             Check agent availability and version
  %sstatus%s    -c <cfg>               Validate and visualize configuration

%sManagement Commands:%s
  %sdiscover%s  <agent_id>             Gather system facts from remote node
  %supdate%s    <agent_id>             Update agent to controller version
  %sinstaller%s                        Generate one-line installation command
  %sinstaller-custom%s                 Interactive installer generator

%sOther:%s
  %shelp%s                             Show this help
  %sversion%s                          Show version

%sUse "stapply-ctl <command> -h" for detailed help on specific commands.%s
`,
		Bold, Reset, Dim, Version,
		Bold, Reset,
		Bold, Reset,
		Cyan, Reset,
		Cyan, Reset,
		Cyan, Reset,
		Cyan, Reset,
		Bold, Reset,
		Cyan, Reset,
		Cyan, Reset,
		Cyan, Reset,
		Cyan, Reset,
		Cyan, Reset,
		Bold, Reset,
		Cyan, Reset,
		Cyan, Reset,
		Dim, Reset)
}

func cmdPing(args []string) {
	fs := flag.NewFlagSet("ping", flag.ExitOnError)
	defaultNats := getDefaultNATSURL()
	natsURL := fs.String("nats", defaultNats, "NATS server (FQDN or IP)")
	allowPublic := fs.Bool("allow-public", false, "Allow connection to public NATS servers")
	timeout := fs.Duration("timeout", 5*time.Second, "Request timeout")
	secretKey := fs.String("sec", "", "Shared secret key for encryption")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: stapply-ctl ping <agent_id>")
		os.Exit(1)
	}

	agentID := fs.Arg(0)

	// Default NATS URL to agent_id if not specified
	if *natsURL == "" {
		*natsURL = agentID
	}

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
	req := protocol.NewPingRequest(Version)
	data, err := json.Marshal(req)
	if err != nil {
		log.Fatalf("Failed to marshal request: %v", err)
	}

	// Send request
	subject := "stapply.ping." + agentID
	start := time.Now()

	if *secretKey != "" {
		var err error
		data, err = security.Encrypt(data, *secretKey)
		if err != nil {
			log.Fatalf("Failed to encrypt request: %v", err)
		}
	}

	msg, err := nc.Request(subject, data, *timeout)
	rtt := time.Since(start)

	if err != nil {
		if err == nats.ErrTimeout {
			fmt.Printf("❌ Agent %s: timeout (no response within %s)\n", agentID, *timeout)
			os.Exit(1)
		}
		log.Fatalf("Request failed: %v", err)
	}

	// Parse response
	if *secretKey != "" {
		var err error
		msg.Data, err = security.Decrypt(msg.Data, *secretKey)
		if err != nil {
			log.Fatalf("Failed to decrypt response: %v", err)
		}
	}

	var resp protocol.PingResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		log.Fatalf("Failed to parse response: %v", err)
	}

	uptimeDur := time.Duration(resp.UptimeSeconds) * time.Second
	fmt.Printf("✅ Agent %s: version=%s uptime=%s cpu=%.1f%% mem=%.1f%% rtt=%v\n",
		resp.AgentID, resp.Version, uptimeDur, resp.CPUUsage, resp.MemoryUsage, rtt.Round(time.Millisecond))
}

func cmdDiscover(args []string) {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	defaultNats := getDefaultNATSURL()
	natsURL := fs.String("nats", defaultNats, "NATS server (FQDN or IP)")
	allowPublic := fs.Bool("allow-public", false, "Allow connection to public NATS servers")
	timeout := fs.Duration("timeout", 5*time.Second, "Request timeout")
	secretKey := fs.String("sec", "", "Shared secret key for encryption")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: stapply-ctl discover <agent_id>")
		os.Exit(1)
	}

	agentID := fs.Arg(0)

	// Default NATS URL to agent_id if not specified
	if *natsURL == "" {
		*natsURL = agentID
	}

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

	// Create discover request
	req := protocol.NewDiscoverRequest()
	data, err := json.Marshal(req)
	if err != nil {
		log.Fatalf("Failed to marshal request: %v", err)
	}

	// Determine effective secret key
	effectiveKey := *secretKey
	if effectiveKey == "" {
		effectiveKey = os.Getenv("STAPPLY_SHARED_KEY")
	}

	// Send request
	if effectiveKey != "" {
		var err error
		data, err = security.Encrypt(data, effectiveKey)
		if err != nil {
			log.Fatalf("Failed to encrypt request: %v", err)
		}
	}

	subject := "stapply.discover." + agentID
	msg, err := nc.Request(subject, data, *timeout)
	if err != nil {
		if err == nats.ErrTimeout {
			fmt.Printf("❌ Agent %s: timeout (no response within %s)\n", agentID, *timeout)
			os.Exit(1)
		}
		log.Fatalf("Request failed: %v", err)
	}

	// Parse response
	if effectiveKey != "" {
		var err error
		msg.Data, err = security.Decrypt(msg.Data, effectiveKey)
		if err != nil {
			log.Fatalf("Failed to decrypt response: %v", err)
		}
	}

	var resp protocol.DiscoverResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		log.Fatalf("Failed to parse response: %v", err)
	}

	// Print facts
	fmt.Printf("🔍 Discovery Results for %s\n", resp.AgentID)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Hostname:    %s\n", resp.Hostname)
	fmt.Printf("OS/Arch:     %s/%s\n", resp.OS, resp.Arch)
	fmt.Printf("CPU Count:   %d\n", resp.CPUCount)
	fmt.Printf("Memory:      %d MB (Free: %d MB)\n", resp.MemoryTotal/1024/1024, resp.MemoryFree/1024/1024)
	fmt.Printf("Root Disk:   %d%% Used\n", resp.DiskUsageRoot)
	fmt.Printf("IP Addrs:    %s\n", strings.Join(resp.IPAddresses, ", "))
	fmt.Println()
}

func cmdAdhoc(args []string) {
	fs := flag.NewFlagSet("adhoc", flag.ExitOnError)
	configPath := fs.String("c", "", "Path to configuration file")
	envName := fs.String("e", "", "Environment name")

	defaultNats := getDefaultNATSURL()
	if defaultNats == "" {
		defaultNats = "nats://localhost:4222"
	}
	natsURL := fs.String("nats", defaultNats, "NATS server URL")

	allowPublic := fs.Bool("allow-public", false, "Allow connection to public NATS servers")
	timeout := fs.Duration("timeout", 30*time.Second, "Request timeout")
	secretKey := fs.String("sec", "", "Shared secret key for encryption")
	fs.Parse(args)

	// Validate NATS URL
	*natsURL = netutil.NormalizeNATSURL(*natsURL)
	if err := netutil.ValidateNATSURL(*natsURL, *allowPublic); err != nil {
		log.Fatalf("NATS URL validation failed: %v", err)
	}

	if *envName == "" {
		fmt.Fprintln(os.Stderr, "Usage: stapply-ctl adhoc [-c <config>] -e <env|agent_id> <action> <args...>")
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Error: action required")
		fmt.Fprintln(os.Stderr, "Usage: stapply-ctl adhoc [-c <config>] -e <env|agent_id> <action> <args...>")
		os.Exit(1)
	}

	action := fs.Arg(0)
	actionArgs := strings.Join(fs.Args()[1:], " ")

	// Two modes: with config (multi-host environment) or without config (single agent)
	var hosts []string
	var cfg *config.Config

	if *configPath != "" {
		// Config mode: load environment from config file
		if !strings.HasSuffix(*configPath, ".stay.ini") {
			fmt.Fprintf(os.Stderr, "Error: config file must have .stay.ini extension: %s\n", *configPath)
			os.Exit(1)
		}

		var err error
		cfg, err = config.Parse(*configPath)
		if err != nil {
			log.Fatalf("Failed to parse config: %v", err)
		}

		env, ok := cfg.Environments[*envName]
		if !ok {
			log.Fatalf("Environment not found: %s", *envName)
		}
		hosts = env.Hosts

		// Determine effective secret key
		if *secretKey == "" {
			*secretKey = os.Getenv("STAPPLY_SHARED_KEY")
		}
	} else {
		// Direct mode: treat envName as agent_id
		hosts = []string{*envName}

		// Default NATS to agent_id if not specified
		if *natsURL == "nats://localhost:4222" {
			*natsURL = netutil.NormalizeNATSURL(*envName)
		}
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
	if *configPath != "" {
		fmt.Printf("   Environment: %s\n", *envName)
	} else {
		fmt.Printf("   Agent: %s\n", *envName)
	}
	fmt.Printf("   Hosts: %v\n", hosts)
	fmt.Println()

	// Execute on each host in parallel
	concurrency := len(hosts)
	if *configPath != "" {
		if env, ok := cfg.Environments[*envName]; ok && env.Concurrency > 0 {
			concurrency = env.Concurrency
		}
	}

	type result struct {
		ok      int
		changed int
		failed  int
	}
	resultCh := make(chan result, len(hosts))
	semaphore := make(chan struct{}, concurrency)

	for _, hostID := range hosts {
		semaphore <- struct{}{}

		go func(hID string) {
			defer func() { <-semaphore }()

			var ok, changed, failed int

			// Get agent_id
			var agentID string
			if cfg != nil {
				host, exists := cfg.Hosts[hID]
				if !exists {
					fmt.Printf("⚠️  Host not found: %s\n", hID)
					resultCh <- result{failed: 1}
					return
				}
				agentID = host.AgentID
				if agentID == "" {
					agentID = hID
				}
			} else {
				// Direct mode: hID is agent_id
				agentID = hID
			}

			fmt.Printf("📦 Host: %s (agent_id=%s)\n", hID, agentID)

			req := protocol.NewRunRequest(action, stepArgs, int(*timeout/time.Millisecond), false)
			data, err := json.Marshal(req)
			if err != nil {
				fmt.Printf("   ❌ Marshal error: %v\n", err)
				resultCh <- result{failed: 1}
				return
			}

			if *secretKey != "" {
				var err error
				data, err = security.Encrypt(data, *secretKey)
				if err != nil {
					fmt.Printf("   ❌ Encrypt error: %v\n", err)
					resultCh <- result{failed: 1}
					return
				}
			}

			subject := "stapply.run." + agentID
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
			if *secretKey != "" {
				var err error
				msg.Data, err = security.Decrypt(msg.Data, *secretKey)
				if err != nil {
					fmt.Printf("   ❌ Decrypt error: %v\n", err)
					resultCh <- result{failed: 1}
					return
				}
			}

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
	for i := 0; i < len(hosts); i++ {
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

	defaultNats := getDefaultNATSURL()
	if defaultNats == "" {
		defaultNats = "nats://localhost:4222"
	}
	natsURL := fs.String("nats", defaultNats, "NATS server URL")

	allowPublic := fs.Bool("allow-public", false, "Allow connection to public NATS servers")
	timeout := fs.Duration("timeout", 30*time.Second, "Request timeout")
	secretKey := fs.String("sec", "", "Shared secret key for encryption")
	fs.Parse(args)

	// Validate NATS URL
	*natsURL = netutil.NormalizeNATSURL(*natsURL)
	if err := netutil.ValidateNATSURL(*natsURL, *allowPublic); err != nil {
		log.Fatalf("NATS URL validation failed: %v", err)
	}

	if *configPath == "" || *envName == "" {
		fmt.Fprintln(os.Stderr, "Usage: stapply-ctl run -c <config> -e <env>")
		os.Exit(1)
	}

	if !strings.HasSuffix(*configPath, ".stay.ini") {
		fmt.Fprintf(os.Stderr, "Error: config file must have .stay.ini extension: %s\n", *configPath)
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

		// Determine effective secret key for this run
		// Priority: 1. Flag, 2. Env Var
		effectiveKey := *secretKey
		if effectiveKey == "" {
			effectiveKey = os.Getenv("STAPPLY_SHARED_KEY")
		}

		go func(hID, key string) {
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

					if step.Action == "cmd" {
						cmdStr := stepArgs["command"]
						if strings.HasPrefix(cmdStr, "STAPPLY_ACTION: deploy_artifact") {
							// Parse args from string: src=... dest=...
							kvText := strings.TrimPrefix(cmdStr, "STAPPLY_ACTION: deploy_artifact ")
							artifactArgs := parseKVString(kvText)

							src := artifactArgs["src"]
							dest := artifactArgs["dest"]

							if src == "" || dest == "" {
								fmt.Printf("         ❌ Invalid deploy_artifact args: %s\n", kvText)
								failed++
								continue
							}

							fmt.Printf("         📦 Deploying artifact: %s -> %s\n", src, dest)

							if err := runDeployArtifact(nc, agentID, src, dest, *timeout, key); err != nil {
								fmt.Printf("         ❌ Artifact deployment failed: %v\n", err)
								failed++
							} else {
								fmt.Printf("         ✅ Artifact deployed successfully\n")
							}
							continue
						}
					}

					req := protocol.NewRunRequest(step.Action, stepArgs, int(*timeout/time.Millisecond), false)
					data, err := json.Marshal(req)
					if err != nil {
						fmt.Printf("         ❌ Marshal error: %v\n", err)
						failed++
						continue
					}

					if key != "" {
						var err error
						data, err = security.Encrypt(data, key)
						if err != nil {
							fmt.Printf("         ❌ Encrypt error: %v\n", err)
							failed++
							continue
						}
					}

					subject := "stapply.run." + agentID
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
					if key != "" {
						var err error
						msg.Data, err = security.Decrypt(msg.Data, key)
						if err != nil {
							fmt.Printf("         ❌ Decrypt error: %v\n", err)
							failed++
							continue
						}
					}

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
		}(hostID, effectiveKey)
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

func cmdPreflight(args []string) {
	fs := flag.NewFlagSet("preflight", flag.ExitOnError)
	configPath := fs.String("c", "", "Path to configuration file")
	envName := fs.String("e", "", "Environment name")

	defaultNats := getDefaultNATSURL()
	if defaultNats == "" {
		defaultNats = "nats://localhost:4222"
	}
	natsURL := fs.String("nats", defaultNats, "NATS server URL")

	allowPublic := fs.Bool("allow-public", false, "Allow connection to public NATS servers")
	timeout := fs.Duration("timeout", 30*time.Second, "Request timeout")
	secretKey := fs.String("sec", "", "Shared secret key for encryption")
	// Determine effective secret key
	effectiveKey := *secretKey
	if effectiveKey == "" {
		effectiveKey = os.Getenv("STAPPLY_SHARED_KEY")
	}

	// Validate NATS URL
	*natsURL = netutil.NormalizeNATSURL(*natsURL)
	if err := netutil.ValidateNATSURL(*natsURL, *allowPublic); err != nil {
		log.Fatalf("NATS URL validation failed: %v", err)
	}

	if *configPath == "" || *envName == "" {
		fmt.Fprintln(os.Stderr, "Usage: stapply-ctl preflight -c <config> -e <env>")
		os.Exit(1)
	}

	if !strings.HasSuffix(*configPath, ".stay.ini") {
		fmt.Fprintf(os.Stderr, "Error: config file must have .stay.ini extension: %s\n", *configPath)
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

	fmt.Printf("🛡️  Preflight Check: %s\n", *envName)
	fmt.Println()

	// 1. System Health Checks (Discovery)
	fmt.Println("1. System Health Checks")
	fmt.Println("───────────────────────")

	type hostHealth struct {
		id string
		ok bool
	}
	healthCh := make(chan hostHealth, len(env.Hosts))

	for _, hostID := range env.Hosts {
		go func(hID string) {
			host, exists := cfg.Hosts[hID]
			if !exists {
				fmt.Printf("   ❌ Host not found in config: %s\n", hID)
				healthCh <- hostHealth{hID, false}
				return
			}
			agentID := host.AgentID
			if agentID == "" {
				agentID = hID
			}

			// Send Discover Request
			req := protocol.NewDiscoverRequest()
			data, err := json.Marshal(req)
			if err != nil {
				fmt.Printf("   ❌ [%s] Marshal error: %v\n", hID, err)
				healthCh <- hostHealth{hID, false}
				return
			}

			if effectiveKey != "" {
				var err error
				data, err = security.Encrypt(data, *secretKey)
				if err != nil {
					fmt.Printf("   ❌ [%s] Encrypt error: %v\n", hID, err)
					healthCh <- hostHealth{hID, false}
					return
				}
			}

			subject := "stapply.discover." + agentID
			msg, err := nc.Request(subject, data, *timeout)
			if err != nil {
				fmt.Printf("   ❌ [%s] Discovery failed: %v\n", hID, err)
				healthCh <- hostHealth{hID, false}
				return
			}

			var resp protocol.DiscoverResponse
			if effectiveKey != "" {
				var err error
				msg.Data, err = security.Decrypt(msg.Data, effectiveKey)
				if err != nil {
					fmt.Printf("   ❌ [%s] Decrypt error: %v\n", hID, err)
					healthCh <- hostHealth{hID, false}
					return
				}
			}

			if err := json.Unmarshal(msg.Data, &resp); err != nil {
				fmt.Printf("   ❌ [%s] Response parse error: %v\n", hID, err)
				healthCh <- hostHealth{hID, false}
				return
			}

			// Check Health Metrics
			ok := true
			freeMemMB := resp.MemoryFree / 1024 / 1024
			if freeMemMB < 256 {
				fmt.Printf("   ⚠️  [%s] Low Memory: %d MB free (warning < 256MB)\n", hID, freeMemMB)
				ok = false
			}

			if resp.DiskUsageRoot > 90 {
				fmt.Printf("   ⚠️  [%s] High Disk Usage: %d%% used (warning > 90%%)\n", hID, resp.DiskUsageRoot)
				ok = false
			}

			if ok {
				fmt.Printf("   ✅ [%s] System Healthy (OS: %s, Mem: %dMB Free, Disk: %d%% Used)\n",
					hID, resp.OS, freeMemMB, resp.DiskUsageRoot)
			} else {
				fmt.Printf("   ⚠️  [%s] System checks completed with warnings\n", hID)
			}
			healthCh <- hostHealth{hID, true} // We consider it "passable" to continue to dry-run unless completely failed
		}(hostID)
	}

	for i := 0; i < len(env.Hosts); i++ {
		<-healthCh
	}
	fmt.Println()

	// 2. Dry Run Execution
	fmt.Println("2. Dry Run Execution")
	fmt.Println("────────────────────")

	// Reuse logic from cmdRun but with DryRun=true
	// Determine concurrency limit
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
				resultCh <- result{failed: 1}
				return
			}
			agentID := host.AgentID
			if agentID == "" {
				agentID = hID
			}

			fmt.Printf("📦 Host: %s\n", hID)

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
					// Use parsed args from step
					stepArgs := step.ArgsMap
					if stepArgs == nil {
						stepArgs = make(map[string]string)
					}

					// DRY RUN REQUEST
					req := protocol.NewRunRequest(step.Action, stepArgs, int(*timeout/time.Millisecond), true)
					data, err := json.Marshal(req)
					if err != nil {
						fmt.Printf("      ❌ Marshal error: %v\n", err)
						failed++
						continue
					}

					if effectiveKey != "" {
						var err error
						data, err = security.Encrypt(data, effectiveKey)
						if err != nil {
							fmt.Printf("      ❌ Encrypt error: %v\n", err)
							failed++
							continue
						}
					}

					subject := "stapply.run." + agentID
					msg, err := nc.Request(subject, data, *timeout)
					if err != nil {
						fmt.Printf("      ❌ Step %d (%s): Request failed: %v\n", i+1, step.Action, err)
						failed++
						continue
					}

					var resp protocol.RunResponse
					if effectiveKey != "" {
						var err error
						msg.Data, err = security.Decrypt(msg.Data, effectiveKey)
						if err != nil {
							fmt.Printf("      ❌ Decrypt error: %v\n", err)
							failed++
							continue
						}
					}

					if err := json.Unmarshal(msg.Data, &resp); err != nil {
						fmt.Printf("      ❌ Step %d: Response error: %v\n", i+1, err)
						failed++
						continue
					}

					switch resp.Status {
					case protocol.StatusOK:
						if resp.Changed {
							fmt.Printf("      ✅ Step %d: %s (Changed)\n", i+1, resp.Stdout)
							changed++
						} else {
							fmt.Printf("      ✅ Step %d: %s (OK)\n", i+1, resp.Stdout)
							ok++
						}
					case protocol.StatusFailed:
						fmt.Printf("      ❌ Step %d: Failed: %s\n", i+1, resp.Stderr)
						failed++
					case protocol.StatusError:
						fmt.Printf("      ❌ Step %d: Error: %s\n", i+1, resp.Error)
						failed++
					}
				}
			}
			resultCh <- result{ok: ok, changed: changed, failed: failed}
		}(hostID)
	}

	var okCount, changedCount, failedCount int
	for i := 0; i < len(env.Hosts); i++ {
		r := <-resultCh
		okCount += r.ok
		changedCount += r.changed
		failedCount += r.failed
	}

	fmt.Println()
	fmt.Printf("Config Check: ok=%d changed=%d failed=%d\n", okCount, changedCount, failedCount)
	if failedCount > 0 {
		fmt.Println("❌ Preflight check FAILED")
		os.Exit(1)
	} else {
		fmt.Println("✅ Preflight check PASSED")
	}
}

// parseKVString parses "key=value key2=val2" into a map
func parseKVString(s string) map[string]string {
	m := make(map[string]string)
	parts := strings.Fields(s)
	for _, part := range parts {
		if idx := strings.Index(part, "="); idx != -1 {
			k := part[:idx]
			v := part[idx+1:]
			m[k] = v
		}
	}
	return m
}

func runDeployArtifact(nc *nats.Conn, agentID, src, dest string, timeout time.Duration, secretKey string) error {
	// 1. Open local file
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %v", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat src: %v", err)
	}
	totalSize := stat.Size()

	// 2. Calculate Checksum
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("calc checksum: %v", err)
	}
	checksum := hex.EncodeToString(h.Sum(nil))

	// Reset file pointer
	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("seek: %v", err)
	}

	// 3. Chunking Loop
	const chunkSize = 10 * 1024 * 1024 // 10MB
	totalChunks := int(math.Ceil(float64(totalSize) / float64(chunkSize)))

	buf := make([]byte, chunkSize)
	for i := 0; i < totalChunks; i++ {
		n, err := f.Read(buf)
		if err != nil && err != io.EOF {
			return fmt.Errorf("read chunk %d: %v", i, err)
		}
		if n == 0 {
			break
		}

		chunkData := buf[:n]
		encoded := base64.StdEncoding.EncodeToString(chunkData)

		args := map[string]string{
			"dest":         dest,
			"chunk_index":  fmt.Sprintf("%d", i),
			"total_chunks": fmt.Sprintf("%d", totalChunks),
			"total_size":   fmt.Sprintf("%d", totalSize),
			"checksum":     checksum,
			"chunk_data":   encoded,
			"mode":         "0755", // Default executable
		}

		req := protocol.NewRunRequest("deploy_artifact", args, int(timeout/time.Millisecond), false)
		data, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("marshal chunk %d: %v", i, err)
		}

		if secretKey != "" {
			data, err = security.Encrypt(data, secretKey)
			if err != nil {
				return fmt.Errorf("encrypt chunk %d: %v", i, err)
			}
		}

		subject := "stapply.run." + agentID
		msg, err := nc.Request(subject, data, timeout)
		if err != nil {
			return fmt.Errorf("send chunk %d: %v", i, err)
		}

		var resp protocol.RunResponse
		if secretKey != "" {
			msg.Data, err = security.Decrypt(msg.Data, secretKey)
			if err != nil {
				return fmt.Errorf("decrypt chunk %d response: %v", i, err)
			}
		}

		if err := json.Unmarshal(msg.Data, &resp); err != nil {
			return fmt.Errorf("parse chunk %d response: %v", i, err)
		}

		if resp.Status != protocol.StatusOK {
			return fmt.Errorf("chunk %d failed: %s (stderr: %s)", i, resp.Error, resp.Stderr)
		}

		fmt.Printf("            Sent chunk %d/%d (%d bytes)\r", i+1, totalChunks, n)
	}
	fmt.Println() // Newline after progress
	return nil
}
