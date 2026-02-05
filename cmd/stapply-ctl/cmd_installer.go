package main

import (
	"flag"
	"fmt"
	"strings"
)

func cmdInstaller(args []string) {
	fs := flag.NewFlagSet("installer", flag.ExitOnError)
	natsURL := fs.String("nats", "", "NATS server (FQDN)")
	fs.Parse(args)

	// Check if agent ID was provided
	var agentIDFlag string
	if fs.NArg() > 0 {
		agentIDFlag = fmt.Sprintf(" --agent-id %s", fs.Arg(0))
	}

	// Only include --nats-server if provided
	var natsFlag string
	if *natsURL != "" {
		// Clean up NATS URL to be FQDN only (strip scheme and port)
		server := *natsURL
		server = strings.TrimPrefix(server, "nats://")
		server = strings.TrimPrefix(server, "tls://")
		if idx := strings.Index(server, ":"); idx != -1 {
			server = server[:idx]
		}
		natsFlag = fmt.Sprintf(" --nats-server %s", server)
	}

	// Construct arguments string
	argsStr := natsFlag + agentIDFlag
	separator := ""
	if argsStr != "" {
		separator = " --"
	}

	// Just output the one-liner
	fmt.Printf("curl -fsSL https://raw.githubusercontent.com/drax2gma/stapply/main/install.sh | sudo bash -s%s%s\n", separator, argsStr)
}
