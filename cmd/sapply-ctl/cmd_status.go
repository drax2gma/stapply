package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/drax2gma/stapply/internal/config"
)

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	configPath := fs.String("c", "", "Path to configuration file")
	fs.Parse(args)

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: sapply-ctl status -c <config>")
		os.Exit(1)
	}

	// Parse configuration
	cfg, err := config.Parse(*configPath)
	if err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

	fmt.Println("📋 Configuration Summary")
	fmt.Println()

	// List environments
	fmt.Printf("🌍 Environments (%d):\n", len(cfg.Environments))
	for envName, env := range cfg.Environments {
		fmt.Printf("  • %s\n", envName)
		fmt.Printf("    Hosts: %v\n", env.Hosts)
		fmt.Printf("    Apps: %v\n", env.Apps)
		if env.Concurrency > 0 {
			fmt.Printf("    Concurrency: %d\n", env.Concurrency)
		}
	}
	fmt.Println()

	// List hosts
	fmt.Printf("🖥️  Hosts (%d):\n", len(cfg.Hosts))
	for hostID, host := range cfg.Hosts {
		agentID := host.AgentID
		if agentID == "" {
			agentID = hostID
		}
		fmt.Printf("  • %s (agent_id=%s)\n", hostID, agentID)
		if len(host.Tags) > 0 {
			fmt.Printf("    Tags: %v\n", host.Tags)
		}
	}
	fmt.Println()

	// List apps
	fmt.Printf("📦 Apps (%d):\n", len(cfg.Apps))
	for appName, app := range cfg.Apps {
		fmt.Printf("  • %s (%d steps)\n", appName, len(app.Steps))
	}
}
