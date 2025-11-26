package middleware

import (
	"brisa"
	"fmt"
	"net"

	"github.com/go-viper/mapstructure/v2"
)

// IPBlacklistConfig holds the configuration for the IPBlacklist middleware.
type IPBlacklistConfig struct {
	// IPs contains a list of single IP addresses or CIDR blocks to block.
	// e.g., ["1.2.3.4", "192.168.0.0/24"]
	IPs []string
}

func IPBlacklistFactory(config map[string]any) (brisa.Middleware, error) {
	var cfg IPBlacklistConfig
	if err := mapstructure.Decode(config, &cfg); err != nil {
		return brisa.Middleware{}, err
	}
	handler, err := NewIPBlacklistHandler(cfg)
	return brisa.Middleware{Handler: handler}, err
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

	// Return the actual middleware function (a closure)
	return func(ctx *brisa.Context) brisa.Action {
		clientIP := ctx.Session.GetClientIP().(*net.TCPAddr).IP

		if _, found := blockedIPs[clientIP.String()]; found {
			// log.Printf("IP %s rejected by blacklist (exact match)", clientIP)
			ctx.Logger().Info("IP rejected by blacklist (exact match)", "ip", clientIP)
			return brisa.Reject
		}

		for _, network := range blockedNets {
			if network.Contains(clientIP) {
				// log.Printf("IP %s rejected by blacklist (network match: %s)", clientIP, network)
				ctx.Logger().Info("IP rejected by blacklist (network match)", "ip", clientIP, "network", network)
				return brisa.Reject
			}
		}

		return brisa.Pass
	}, nil
}
