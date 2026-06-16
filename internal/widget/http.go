package widget

import (
	"net/http"
	"time"
)

// httpClient is the shared client for network-backed widgets (calendar/weather).
// Keep timeouts short so a slow feed can't stall the broker.
var httpClient = &http.Client{Timeout: 10 * time.Second}
