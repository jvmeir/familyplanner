package config_test

import (
	"testing"
	"time"

	"github.com/jvmeir/familyplanner/internal/config"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("FP_ENV", "dev")
	t.Setenv("FP_ENCRYPTION_KEY", "") // force the dev fallback key

	c, err := config.Load()
	require.NoError(t, err)

	require.Equal(t, ":8080", c.Addr)
	require.Equal(t, "nl", c.DefaultLocale)
	require.Equal(t, "Europe/Brussels", c.TimeZone.String())
	require.Equal(t, 90*24*time.Hour, c.SessionTTL)
	require.Len(t, c.EncryptionKey, 32, "derived AES key must be 32 bytes")
}

func TestSessionDaysOverride(t *testing.T) {
	t.Setenv("FP_SESSION_DAYS", "30")
	c, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, 30*24*time.Hour, c.SessionTTL)
}

func TestProdRequiresEncryptionKey(t *testing.T) {
	t.Setenv("FP_ENV", "prod")
	t.Setenv("FP_ENCRYPTION_KEY", "")
	_, err := config.Load()
	require.Error(t, err)
}

func TestInvalidTimezone(t *testing.T) {
	t.Setenv("FP_TIMEZONE", "Mars/Olympus")
	_, err := config.Load()
	require.Error(t, err)
}
