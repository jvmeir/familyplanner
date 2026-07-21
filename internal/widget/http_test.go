package widget

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIPAllowedSSRF(t *testing.T) {
	// Blocked regardless of the loopback exemption.
	blocked := []string{
		"169.254.169.254", // cloud metadata (link-local)
		"169.254.1.1",     // link-local
		"0.0.0.0",         // unspecified
		"224.0.0.1",       // multicast
		"fe80::1",         // IPv6 link-local
		"::",              // IPv6 unspecified
	}
	for _, s := range blocked {
		require.Falsef(t, ipAllowed(net.ParseIP(s)), "%s must be blocked", s)
	}

	// Allowed: public + private LAN.
	allowed := []string{"1.1.1.1", "192.168.1.10", "10.0.0.5", "172.16.3.4", "8.8.8.8"}
	for _, s := range allowed {
		require.Truef(t, ipAllowed(net.ParseIP(s)), "%s must be allowed", s)
	}

	// Loopback is gated by the (test-only) exemption.
	defer func(v bool) { allowLoopback = v }(allowLoopback)
	allowLoopback = false
	require.False(t, ipAllowed(net.ParseIP("127.0.0.1")), "loopback blocked in prod")
	require.False(t, ipAllowed(net.ParseIP("::1")))
	allowLoopback = true
	require.True(t, ipAllowed(net.ParseIP("127.0.0.1")), "loopback allowed in tests")
}
