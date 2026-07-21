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

func TestNextPhotoNoRepeatWithinCycle(t *testing.T) {
	s := &Server{}
	urls := []string{"a", "b", "c"}

	// First full cycle: every photo appears exactly once (no repeats).
	seen := map[string]int{}
	for i := 0; i < len(urls); i++ {
		seen[s.nextPhoto(1, urls)]++
	}
	require.Len(t, seen, 3, "all three shown once before any repeat")
	for _, u := range urls {
		require.Equal(t, 1, seen[u], "photo %q shown exactly once per cycle", u)
	}

	// Second cycle also covers the whole album.
	seen2 := map[string]int{}
	for i := 0; i < len(urls); i++ {
		seen2[s.nextPhoto(1, urls)]++
	}
	require.Len(t, seen2, 3)

	// A single-photo album just returns that photo.
	require.Equal(t, "solo", s.nextPhoto(2, []string{"solo"}))
	require.Equal(t, "", s.nextPhoto(3, nil))
}
