package netutil

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// IsPrivateNetwork checks if an IP address is in a private network range.
// Includes: localhost, LAN (RFC1918), and CGNAT (RFC6598, used by Tailscale).
func IsPrivateNetwork(ip net.IP) bool {
	// Localhost
	if ip.IsLoopback() {
		return true
	}

	// Private LAN ranges (RFC1918)
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	// CGNAT range (RFC6598) - used by Tailscale and similar
	cgnatRange := "100.64.0.0/10"
	privateRanges = append(privateRanges, cgnatRange)

	for _, cidr := range privateRanges {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// ValidateNATSURL checks if a NATS URL points to a private network.
// Returns error if URL is public and allowPublic is false.
func ValidateNATSURL(natsURL string, allowPublic bool) error {
	// Parse URL
	u, err := url.Parse(natsURL)
	if err != nil {
		return fmt.Errorf("invalid NATS URL: %v", err)
	}

	// Extract host (may include port)
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("NATS URL missing hostname")
	}

	// Resolve hostname to IP
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("failed to resolve NATS hostname %s: %v", host, err)
	}

	if len(ips) == 0 {
		return fmt.Errorf("no IP addresses found for NATS hostname %s", host)
	}

	// Check if any resolved IP is private
	hasPrivateIP := false
	for _, ip := range ips {
		if IsPrivateNetwork(ip) {
			hasPrivateIP = true
			break
		}
	}

	// If no private IPs found and public not allowed, reject
	if !hasPrivateIP && !allowPublic {
		return fmt.Errorf(
			"NATS URL %s resolves to public IP addresses (%v). "+
				"For security, only private networks (LAN/CGNAT) are allowed by default. "+
				"Use --allow-public to override",
			natsURL, ips[0])
	}

	return nil
}

// FormatAllowedNetworks returns a human-readable list of allowed network ranges.
func FormatAllowedNetworks() string {
	return strings.Join([]string{
		"127.0.0.0/8 (localhost)",
		"10.0.0.0/8 (private LAN)",
		"172.16.0.0/12 (private LAN)",
		"192.168.0.0/16 (private LAN)",
		"100.64.0.0/10 (CGNAT/Tailscale)",
	}, ", ")
}

// NormalizeNATSURL adds default scheme and port if missing.
// defaults: scheme=nats://, port=4222
func NormalizeNATSURL(server string) string {
	if server == "" {
		return ""
	}

	// Add default scheme if missing
	if !strings.Contains(server, "://") {
		server = "nats://" + server
	}

	// Check for port
	u, err := url.Parse(server)
	if err == nil {
		if u.Port() == "" {
			u.Host = net.JoinHostPort(u.Host, "4222")
			return u.String()
		}
	}

	return server
}
