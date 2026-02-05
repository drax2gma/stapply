package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func cmdInstallerCustom(args []string) {
	reader := bufio.NewReader(os.Stdin)

	// Header
	fmt.Println("🚀 Interactive Installer Generator")
	fmt.Println("--------------------------------")

	// Prompt for Agent ID
	fmt.Print("Enter Agent ID (leave empty for hostname): ")
	agentID, _ := reader.ReadString('\n')
	agentID = strings.TrimSpace(agentID)

	// Prompt for NATS Server
	fmt.Print("Enter NATS Server FQDN (default: localhost): ")
	natsServer, _ := reader.ReadString('\n')
	natsServer = strings.TrimSpace(natsServer)

	// Build flags
	var agentIDFlag string
	if agentID != "" {
		agentIDFlag = fmt.Sprintf(" --agent-id %s", agentID)
	}

	var natsFlag string
	if natsServer != "" && natsServer != "localhost" {
		// Clean up input just in case user pasted a URL
		server := natsServer
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

	fmt.Println("\n✅ Run this command on your target machine:")
	fmt.Println()
	fmt.Printf("curl -fsSL https://raw.githubusercontent.com/drax2gma/stapply/main/install.sh | sudo bash -s%s%s\n", separator, argsStr)
	fmt.Println()
}
