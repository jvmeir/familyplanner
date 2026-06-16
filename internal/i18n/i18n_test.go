package i18n_test

import (
	"context"
	"testing"

	"github.com/jvmeir/familyplanner/internal/i18n"
	"github.com/stretchr/testify/require"
)

func TestDutchIsDefault(t *testing.T) {
	svc, err := i18n.New("nl")
	require.NoError(t, err)

	lc := svc.Loc()
	require.Equal(t, "Aanmelden", lc.T("login.title"))
	require.Equal(t, "Wachtwoord", lc.T("login.passphrase"))
}

func TestPluralization(t *testing.T) {
	svc, err := i18n.New("nl")
	require.NoError(t, err)
	lc := svc.Loc()

	require.Equal(t, "nog 1 dag", lc.T("countdown.days", map[string]any{"Count": 1}))
	require.Equal(t, "nog 5 dagen", lc.T("countdown.days", map[string]any{"Count": 5}))
}

func TestMissingKeyReturnsID(t *testing.T) {
	svc, err := i18n.New("nl")
	require.NoError(t, err)
	require.Equal(t, "does.not.exist", svc.Loc().T("does.not.exist"))
}

func TestContextHelper(t *testing.T) {
	svc, err := i18n.New("nl")
	require.NoError(t, err)
	ctx := i18n.WithLoc(context.Background(), svc.Loc())
	require.Equal(t, "Beheer", i18n.T(ctx, "admin.title"))

	// no localizer in context -> returns the id, never panics
	require.Equal(t, "admin.title", i18n.T(context.Background(), "admin.title"))
}
