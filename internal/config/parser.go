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
	sectionRe = regexp.MustCompile(`^\[(\w+):([^\]]+)\]$`)
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
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("line %d: invalid concurrency value: %s", lineNum, value)
			}
			env.Concurrency = n
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

// parseStep parses a step value like "cmd:apt-get install -y nginx" or "systemd:restart nginx".
func parseStep(value string) (Step, error) {
	idx := strings.Index(value, ":")
	if idx == -1 {
		return Step{}, fmt.Errorf("invalid step format (missing ':'): %s", value)
	}

	action := value[:idx]
	args := value[idx+1:]

	return Step{
		Action: action,
		Args:   args,
	}, nil
}
