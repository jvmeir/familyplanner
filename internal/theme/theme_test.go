package theme_test

import (
	"testing"

	"github.com/jvmeir/familyplanner/internal/theme"
	"github.com/stretchr/testify/require"
)

func TestResolveCascade(t *testing.T) {
	require.Equal(t, "donker", theme.Resolve("donker", "licht").ID, "view override wins")
	require.Equal(t, "licht", theme.Resolve("", "licht").ID, "falls back to global default")
	require.Equal(t, theme.DefaultID, theme.Resolve("nope", "alsonope").ID, "falls back to hard default")
}

func TestVarsCSS(t *testing.T) {
	css := theme.Presets["licht"].VarsCSS()
	require.Contains(t, css, "--bg:")
	require.Contains(t, css, "--accent:")
	require.Contains(t, css, ";")
}
