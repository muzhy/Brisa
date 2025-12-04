package middleware

import (
	"fmt"
	"net"

	"github.com/muzhy/brisa"
)

type IPBlacklist struct {
	blockedIPs map[string]struct{}
	networks   []*net.IPNet
}

// NewIPBlacklist creates a new IPBlacklist instance.
// It parses a list of IP addresses and CIDR blocks, returning an error if any are invalid.
func NewIPBlacklist(ips []string) (*IPBlacklist, error) {
	bl := &IPBlacklist{
		blockedIPs: make(map[string]struct{}),
		networks:   make([]*net.IPNet, 0),
	}

	for _, ipStr := range ips {
		// Try to parse as CIDR first
		_, ipNet, err := net.ParseCIDR(ipStr)
		if err == nil {
			bl.networks = append(bl.networks, ipNet)
			continue
		}

		// If not a CIDR, try to parse as a single IP
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP address or CIDR block in blacklist: %s", ipStr)
		}
		bl.blockedIPs[ip.String()] = struct{}{}
	}

	return bl, nil
}

// IsBlocked checks if a given IP address is in the blacklist.
func (bl *IPBlacklist) IsBlocked(ip net.IP) bool {
	if _, found := bl.blockedIPs[ip.String()]; found {
		return true
	}

	for _, network := range bl.networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// NewIPBlacklistHandler creates a new middleware handler for blocking IPs.
// It validates the IPs and returns an error if any IP/CIDR is invalid.
func NewIPBlacklistHandler(IPs []string) (brisa.Handler, error) {
	blacklist, err := NewIPBlacklist(IPs)
	if err != nil {
		return nil, err
	}

	// Return the actual middleware function (a closure)
	return func(ctx *brisa.Context) brisa.Action {
		clientIP := ctx.Session.GetClientIP().(*net.TCPAddr).IP

		if blacklist.IsBlocked(clientIP) {
			ctx.Logger.Info("IP rejected by blacklist", "ip", clientIP)
			return brisa.Reject
		}
		return brisa.Pass
	}, nil
}
