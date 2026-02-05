package sysinfo

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"syscall"

	"github.com/drax2gma/stapply/internal/protocol"
)

// GatherFacts collects system information.
func GatherFacts(agentID string) (*protocol.DiscoverResponse, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("get hostname: %w", err)
	}

	memTotal, memFree, err := getMemoryInfo()
	if err != nil {
		// Log error but continue with zero values
		fmt.Fprintf(os.Stderr, "Warning: failed to get memory info: %v\n", err)
	}

	diskUsage, err := getDiskUsage("/")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to get disk usage: %v\n", err)
	}

	ips, err := getIPAddresses()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to get IP addresses: %v\n", err)
	}

	return &protocol.DiscoverResponse{
		AgentID:       agentID,
		Hostname:      hostname,
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		CPUCount:      runtime.NumCPU(),
		MemoryTotal:   memTotal,
		MemoryFree:    memFree,
		DiskUsageRoot: diskUsage,
		IPAddresses:   ips,
	}, nil
}

func getMemoryInfo() (total, free uint64, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		key := parts[0]
		val := parts[1] // In kB

		// Parse value
		var v uint64
		// Simple parsing, assuming valid /proc/meminfo format
		fmt.Sscanf(val, "%d", &v)
		v *= 1024 // Convert to bytes

		switch key {
		case "MemTotal:":
			total = v
		case "MemAvailable:":
			free = v
		}
	}
	return total, free, scanner.Err()
}

func getDiskUsage(path string) (int, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}

	// Blocks * BlockSize = Total bytes
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free

	if total == 0 {
		return 0, nil
	}

	return int((used * 100) / total), nil
}

func getIPAddresses() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var ips []string
	for _, i := range ifaces {
		// Skip loopback and down interfaces
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := i.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			// Append both IPv4 and IPv6
			ips = append(ips, ip.String())
		}
	}
	return ips, nil
}
