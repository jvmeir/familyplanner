package widget

import (
	"os"
	"testing"
)

// TestMain permits loopback connections for the whole package's tests, whose
// httptest servers bind to 127.0.0.1 (production keeps loopback SSRF-blocked).
func TestMain(m *testing.M) {
	allowLoopback = true
	os.Exit(m.Run())
}
