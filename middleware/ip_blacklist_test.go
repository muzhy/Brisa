package middleware

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewIPBlacklist(t *testing.T) {
	t.Run("valid IPs and CIDRs", func(t *testing.T) {
		ips := []string{"1.2.3.4", "192.168.1.0/24", "::1"}
		blacklist, err := NewIPBlacklist(ips)
		require.NoError(t, err)
		assert.NotNil(t, blacklist)
		assert.Len(t, blacklist.blockedIPs, 2)
		assert.Len(t, blacklist.networks, 1)
	})

	t.Run("invalid IP address", func(t *testing.T) {
		ips := []string{"999.999.999.999"}
		blacklist, err := NewIPBlacklist(ips)
		require.Error(t, err)
		assert.Nil(t, blacklist)
		assert.Contains(t, err.Error(), "invalid IP address or CIDR block")
	})

	t.Run("invalid CIDR block", func(t *testing.T) {
		ips := []string{"192.168.1.0/33"}
		blacklist, err := NewIPBlacklist(ips)
		require.Error(t, err)
		assert.Nil(t, blacklist)
		assert.Contains(t, err.Error(), "invalid IP address or CIDR block")
	})
}

func TestIPBlacklist_IsBlocked(t *testing.T) {
	blacklist := []string{
		"1.2.3.4",            // Exact IPv4
		"192.168.1.0/24",     // IPv4 CIDR
		"2001:db8::1",        // Exact IPv6
		"2001:db8:abcd::/48", // IPv6 CIDR
	}

	ipBlacklist, err := NewIPBlacklist(blacklist)
	require.NoError(t, err)

	testCases := []struct {
		name        string
		clientIP    string
		wantBlocked bool
	}{
		{
			name:        "Blocked exact IPv4",
			clientIP:    "1.2.3.4",
			wantBlocked: true,
		},
		{
			name:        "Blocked IPv4 in CIDR",
			clientIP:    "192.168.1.100",
			wantBlocked: true,
		},
		{
			name:        "Allowed IPv4",
			clientIP:    "8.8.8.8",
			wantBlocked: false,
		},
		{
			name:        "Blocked exact IPv6",
			clientIP:    "2001:db8::1",
			wantBlocked: true,
		},
		{
			name:        "Blocked IPv6 in CIDR",
			clientIP:    "2001:db8:abcd:0001:0002:0003:0004:0005",
			wantBlocked: true,
		},
		{
			name:        "Allowed IPv6",
			clientIP:    "2606:4700:4700::1111",
			wantBlocked: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.clientIP)
			require.NotNil(t, ip)
			isBlocked := ipBlacklist.IsBlocked(ip)
			assert.Equal(t, tc.wantBlocked, isBlocked)
		})
	}
}
