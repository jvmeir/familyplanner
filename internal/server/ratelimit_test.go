package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRateLimiterBurstThenRefill(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	clock := func() time.Time { return now }
	l := newRateLimiter(2, time.Minute, clock) // 2 attempts/min

	require.True(t, l.allow("ip"), "1st within burst")
	require.True(t, l.allow("ip"), "2nd within burst")
	require.False(t, l.allow("ip"), "3rd exhausts the bucket")

	// A different key has its own bucket.
	require.True(t, l.allow("other"))

	// After half the window, ~one token has refilled.
	now = now.Add(30 * time.Second)
	require.True(t, l.allow("ip"), "refilled one token")
	require.False(t, l.allow("ip"), "and only one")
}
