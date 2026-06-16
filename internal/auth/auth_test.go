package auth_test

import (
	"testing"

	"github.com/jvmeir/familyplanner/internal/auth"
	"github.com/stretchr/testify/require"
)

func TestHashAndVerify(t *testing.T) {
	hash, err := auth.HashPassphrase("correct horse")
	require.NoError(t, err)
	require.Contains(t, hash, "$argon2id$")

	ok, err := auth.VerifyPassphrase("correct horse", hash)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = auth.VerifyPassphrase("wrong", hash)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestVerifyRejectsGarbage(t *testing.T) {
	_, err := auth.VerifyPassphrase("x", "not-a-hash")
	require.Error(t, err)
}

func TestTokens(t *testing.T) {
	a, err := auth.NewToken()
	require.NoError(t, err)
	b, err := auth.NewToken()
	require.NoError(t, err)
	require.NotEqual(t, a, b, "tokens must be unique")

	require.Equal(t, auth.HashToken(a), auth.HashToken(a), "hash is deterministic")
	require.NotEqual(t, auth.HashToken(a), auth.HashToken(b))
}
