package widget

import (
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// httpClient is the shared client for network-backed widgets (calendar/weather/
// ticker/bring/graph). Keep timeouts short so a slow feed can't stall the broker.
//
// It guards against SSRF on the user-supplied feed URLs (iCal/RSS): the dialer's
// Control hook inspects the *resolved* IP of every connection and refuses
// loopback, link-local (incl. the 169.254.169.254 cloud-metadata endpoint),
// unspecified and multicast targets. Private LAN ranges (10/8, 172.16/12,
// 192.168/16) stay allowed on purpose — this app runs on a home LAN and users
// legitimately point widgets at other devices on their network.
var httpClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
			Control:   ssrfControl,
		}).DialContext,
		MaxIdleConns:          50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	},
}

// ssrfControl rejects connections to sensitive local addresses. It runs after
// DNS resolution with the concrete IP, so it also blunts DNS-rebinding attempts.
func ssrfControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("ssrf: could not parse address %q", address)
	}
	if !ipAllowed(ip) {
		return fmt.Errorf("ssrf: refusing to connect to %s", ip)
	}
	return nil
}

// allowLoopback is flipped on only in tests, whose httptest servers bind to
// 127.0.0.1. In production loopback stays blocked.
var allowLoopback = false

// ipAllowed reports whether an IP is an acceptable outbound target. Loopback,
// link-local, unspecified and multicast are blocked; everything else (public +
// private LAN) is allowed.
func ipAllowed(ip net.IP) bool {
	if ip.IsLoopback() {
		return allowLoopback
	}
	if ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	return true
}
