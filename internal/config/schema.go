package config

import (
	"bufio"
	"os"
	"strings"
)

// Config holds all parsed configuration sections.
type Config struct {
	Environments map[string]*Environment
	Hosts        map[string]*Host
	Apps         map[string]*App
}

// Environment defines a deployment environment.
type Environment struct {
	Name        string
	Hosts       []string // List of host IDs
	Apps        []string // List of app names
	Concurrency int      // Max parallel agents (0 = unlimited)
}

// Host defines a target machine.
type Host struct {
	ID      string   // Host identifier (matches section name)
	AgentID string   // NATS subject agent_id
	Tags    []string // Optional metadata tags
}

// App defines an application with ordered steps.
type App struct {
	Name  string
	Steps map[int]Step // Step number -> Step definition
}

// Step defines a single action to execute.
type Step struct {
	Action string // Action type: cmd, write_file, template_file, systemd
	Args   string // Action arguments (action-specific format)
}

// GetOrderedSteps returns steps sorted by step number.
func (a *App) GetOrderedSteps() []Step {
	if len(a.Steps) == 0 {
		return nil
	}

	// Find max step number
	maxStep := 0
	for n := range a.Steps {
		if n > maxStep {
			maxStep = n
		}
	}

	// Collect steps in order
	result := make([]Step, 0, len(a.Steps))
	for i := 1; i <= maxStep; i++ {
		if step, ok := a.Steps[i]; ok {
			result = append(result, step)
		}
	}
	return result
}

// AgentConfig holds agent-specific configuration.
type AgentConfig struct {
	AgentID   string
	NatsURL   string
	NatsCreds string
}

// ParseAgentConfig parses an agent configuration file.
func ParseAgentConfig(path string) (*AgentConfig, error) {
	cfg, err := parseSimpleINI(path)
	if err != nil {
		return nil, err
	}

	agent := cfg["agent"]
	if agent == nil {
		agent = make(map[string]string)
	}

	return &AgentConfig{
		AgentID:   agent["agent_id"],
		NatsURL:   withDefault(agent["nats_url"], "nats://localhost:4222"),
		NatsCreds: agent["nats_creds"],
	}, nil
}

// parseSimpleINI parses a simple INI file without typed sections.
func parseSimpleINI(path string) (map[string]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := make(map[string]map[string]string)
	var currentSection string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = line[1 : len(line)-1]
			result[currentSection] = make(map[string]string)
			continue
		}

		if idx := strings.Index(line, "="); idx != -1 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			if result[currentSection] == nil {
				result[currentSection] = make(map[string]string)
			}
			result[currentSection][key] = value
		}
	}

	return result, scanner.Err()
}

func withDefault(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}
