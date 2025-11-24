package middleware

import (
	"brisa"
	"fmt"
	"log"
	"net"
)

// IPBlacklistConfig holds the configuration for the IPBlacklist middleware.
type IPBlacklistConfig struct {
	// IPs contains a list of single IP addresses or CIDR blocks to block.
	// e.g., ["1.2.3.4", "192.168.0.0/24"]
	IPs []string
}

// NewIPBlacklist creates a new middleware for blocking IPs.
// It validates the configuration and returns an error if any IP/CIDR is invalid.
func NewIPBlacklistHandler(config IPBlacklistConfig) (brisa.Handler, error) {
	blockedIPs := make(map[string]struct{})
	blockedNets := make([]*net.IPNet, 0)

	for _, ipStr := range config.IPs {
		// Try to parse as CIDR first
		_, ipNet, err := net.ParseCIDR(ipStr)
		if err == nil {
			blockedNets = append(blockedNets, ipNet)
			continue
		}

		// If not a CIDR, try to parse as a single IP
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP address or CIDR block in blacklist: %s", ipStr)
		}
		blockedIPs[ip.String()] = struct{}{}
	}

	log.Printf("IP Blacklist middleware initialized with %d single IPs and %d networks", len(blockedIPs), len(blockedNets))

	// Return the actual middleware function (a closure)
	return func(ctx *brisa.Context) brisa.Action {
		clientIP := ctx.Session.GetClientIP().(*net.TCPAddr).IP

		if _, found := blockedIPs[clientIP.String()]; found {
			log.Printf("IP %s rejected by blacklist (exact match)", clientIP)
			return brisa.Reject
		}

		for _, network := range blockedNets {
			if network.Contains(clientIP) {
				log.Printf("IP %s rejected by blacklist (network match: %s)", clientIP, network)
				return brisa.Reject
			}
		}

		return brisa.Pass
	}, nil
}
