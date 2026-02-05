package config

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var (
	sectionRe  = regexp.MustCompile(`^\[(\w+):([^\]]+)\]$`)
	keyValueRe = regexp.MustCompile(`^([^=]+)=(.*)$`)
)

// Parse reads an INI file and returns a Config.
func Parse(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	cfg := &Config{
		Environments: make(map[string]*Environment),
		Hosts:        make(map[string]*Host),
		Apps:         make(map[string]*App),
	}

	var currentSection string
	var currentName string

	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for section header
		if strings.HasPrefix(line, "[") {
			matches := sectionRe.FindStringSubmatch(line)
			if matches == nil {
				return nil, fmt.Errorf("line %d: invalid section header: %s", lineNum, line)
			}
			currentSection = matches[1]
			currentName = matches[2]

			switch currentSection {
			case "env":
				cfg.Environments[currentName] = &Environment{Name: currentName}
			case "host":
				cfg.Hosts[currentName] = &Host{ID: currentName}
			case "app":
				cfg.Apps[currentName] = &App{Name: currentName, Steps: make(map[int]Step)}
			default:
				return nil, fmt.Errorf("line %d: unknown section type: %s", lineNum, currentSection)
			}
			continue
		}

		// Parse key=value
		matches := keyValueRe.FindStringSubmatch(line)
		if matches == nil {
			return nil, fmt.Errorf("line %d: invalid key=value: %s", lineNum, line)
		}

		key := strings.TrimSpace(matches[1])
		value := strings.TrimSpace(matches[2])

		if err := cfg.setKeyValue(currentSection, currentName, key, value, lineNum); err != nil {
			return nil, err
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan config: %w", err)
	}

	return cfg, nil
}

// setKeyValue sets a key-value pair in the appropriate section.
func (c *Config) setKeyValue(section, name, key, value string, lineNum int) error {
	switch section {
	case "env":
		env := c.Environments[name]
		switch key {
		case "hosts":
			env.Hosts = parseList(value)
		case "apps":
			env.Apps = parseList(value)
		case "concurrency":
			// Accept boolean (true=5, false=1) or number
			switch strings.ToLower(value) {
			case "true", "yes", "on":
				env.Concurrency = 5 // Max parallel when enabled
			case "false", "no", "off":
				env.Concurrency = 1 // Sequential
			default:
				n, err := strconv.Atoi(value)
				if err != nil {
					return fmt.Errorf("line %d: invalid concurrency value: %s (use true/false or number)", lineNum, value)
				}
				if n > 5 {
					n = 5 // Cap at 5
				}
				if n < 1 {
					n = 1
				}
				env.Concurrency = n
			}
		default:
			return fmt.Errorf("line %d: unknown env key: %s", lineNum, key)
		}

	case "host":
		host := c.Hosts[name]
		switch key {
		case "agent_id":
			host.AgentID = value
		case "tags":
			host.Tags = parseList(value)
		default:
			return fmt.Errorf("line %d: unknown host key: %s", lineNum, key)
		}

	case "app":
		app := c.Apps[name]
		// Parse step keys like "step1", "step2", etc.
		if strings.HasPrefix(key, "step") {
			numStr := strings.TrimPrefix(key, "step")
			num, err := strconv.Atoi(numStr)
			if err != nil {
				return fmt.Errorf("line %d: invalid step number: %s", lineNum, key)
			}
			step, err := parseStep(value)
			if err != nil {
				return fmt.Errorf("line %d: %w", lineNum, err)
			}
			app.Steps[num] = step
		} else {
			return fmt.Errorf("line %d: unknown app key: %s", lineNum, key)
		}

	case "security":
		// Security section is deprecated in config, use STAPPLY_SHARED_KEY env var
		// We ignore it to avoid breaking old configs immediately, or we could just remove it.
		// Since we removed the struct, we must not assign to c.Security.
		if key == "secret_key" {
			// Ignore
		} else {
			return fmt.Errorf("line %d: unknown security key: %s (security section is deprecated)", lineNum, key)
		}

	case "":
		return fmt.Errorf("line %d: key outside of section", lineNum)

	default:
		return fmt.Errorf("line %d: unknown section type: %s", lineNum, section)
	}

	return nil
}

// parseList splits a comma-separated value into a slice.
func parseList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// shellTokenize splits a string into tokens, respecting quoted strings.
// Supports both single and double quotes.
func shellTokenize(input string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, ch := range input {
		switch {
		case (ch == '"' || ch == '\'') && !inQuote:
			// Start quote
			inQuote = true
			quoteChar = ch
		case ch == quoteChar && inQuote:
			// End quote
			inQuote = false
			quoteChar = 0
		case ch == ' ' && !inQuote:
			// Token separator (only outside quotes)
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}

	// Don't forget the last token
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// parseStep parses a step value like "cmd:apt-get install -y nginx" or "write_file:/path/to/file mode=0644".
func parseStep(value string) (Step, error) {
	idx := strings.Index(value, ":")
	if idx == -1 {
		return Step{}, fmt.Errorf("invalid step format (missing ':'): %s", value)
	}

	action := value[:idx]
	args := value[idx+1:]

	step := Step{
		Action:  action,
		Args:    args,
		ArgsMap: make(map[string]string),
	}

	// Parse args based on action type
	switch action {
	case "cmd":
		// For cmd, args is just the command
		step.ArgsMap["command"] = args

	case "write_file", "template_file":
		// For file actions, first token is path, rest are key=value pairs
		// Use shellTokenize to respect quoted values with spaces
		parts := shellTokenize(args)
		if len(parts) == 0 {
			return Step{}, fmt.Errorf("missing path for %s action", action)
		}
		step.ArgsMap["path"] = parts[0]

		// Parse remaining key=value pairs
		for _, part := range parts[1:] {
			if eqIdx := strings.Index(part, "="); eqIdx != -1 {
				key := part[:eqIdx]
				val := part[eqIdx+1:]
				step.ArgsMap[key] = val
			}
		}

	case "systemd":
		// For systemd, args format is "action unit"
		parts := strings.Fields(args)
		if len(parts) >= 1 {
			step.ArgsMap["action"] = parts[0]
		}
		if len(parts) >= 2 {
			step.ArgsMap["unit"] = parts[1]
		}

	default:
		// Unknown action, just store as command
		step.ArgsMap["command"] = args
	}

	return step, nil
}
