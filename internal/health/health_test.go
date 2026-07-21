package health

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

func TestAssessAllHealthy(t *testing.T) {
	sum := Assess(
		[]Source{{Name: "Outlook", IsOAuth: true, OAuthStatus: "connected", Health: "ok",
			AccessExpiry: now.Add(time.Hour).Format(time.RFC3339)}},
		[]Widget{{Name: "Klok", Status: "ok", FetchedAt: now.Add(-time.Minute).Format(sqlTime)}},
		now, time.Hour,
	)
	require.Equal(t, LevelOK, sum.Level)
	require.Equal(t, 0, sum.Count)
}

func TestAssessReconnectNeeded(t *testing.T) {
	// invalid_grant -> broker set health="reconnect".
	sum := Assess([]Source{{Name: "Outlook", IsOAuth: true, OAuthStatus: "connected", Health: "reconnect"}}, nil, now, time.Hour)
	require.Equal(t, LevelError, sum.Level)
	require.Len(t, sum.Issues, 1)
	require.Equal(t, "reconnect", sum.Issues[0].Kind)
	require.Contains(t, sum.Issues[0].Message, "opnieuw verbinden")
}

func TestAssessNeverConnected(t *testing.T) {
	// OAuth source created but never authorized (oauth_status != connected).
	sum := Assess([]Source{{Name: "OneDrive", IsOAuth: true}}, nil, now, time.Hour)
	require.Equal(t, LevelError, sum.Level)
	require.Equal(t, "reconnect", sum.Issues[0].Kind)
}

func TestAssessAccessExpiredIsHealthy(t *testing.T) {
	// A past access-token expiry is normal: the broker refreshes it from the
	// refresh token, so a connected + healthy source stays OK (no warning).
	sum := Assess([]Source{{Name: "Outlook", IsOAuth: true, OAuthStatus: "connected", Health: "ok",
		AccessExpiry: now.Add(-time.Minute).Format(time.RFC3339)}}, nil, now, time.Hour)
	require.Equal(t, LevelOK, sum.Level)
	require.Empty(t, sum.Issues)
}

func TestAssessFailedSyncAndStale(t *testing.T) {
	sum := Assess(nil, []Widget{
		{Name: "Agenda", Status: "error"},
		{Name: "Weer", Status: "ok", FetchedAt: now.Add(-2 * time.Hour).Format(sqlTime)},
	}, now, time.Hour)
	require.Equal(t, LevelWarn, sum.Level)
	require.Equal(t, 2, sum.Count)
	kinds := map[string]bool{}
	for _, is := range sum.Issues {
		kinds[is.Kind] = true
	}
	require.True(t, kinds["sync"], "failed sync surfaced")
	require.True(t, kinds["stale"], "stale data surfaced")
}

func TestAssessNonOAuthIgnored(t *testing.T) {
	// An iCal (non-OAuth) source has no auth health, even if unhealthy fields set.
	sum := Assess([]Source{{Name: "School.ics", IsOAuth: false, Health: "reconnect"}}, nil, now, time.Hour)
	require.Equal(t, LevelOK, sum.Level)
	require.Equal(t, 0, sum.Count)
}

func TestAssessErrorRanksAboveWarn(t *testing.T) {
	sum := Assess(
		[]Source{{Name: "Outlook", IsOAuth: true, OAuthStatus: "connected", Health: "reconnect"}},
		[]Widget{{Name: "Agenda", Status: "error"}},
		now, time.Hour,
	)
	require.Equal(t, LevelError, sum.Level)
	require.Equal(t, LevelError, sum.Issues[0].Level, "most-urgent issue first")
}
