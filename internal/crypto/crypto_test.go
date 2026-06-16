package crypto_test

import (
	"crypto/sha256"
	"testing"

	"github.com/jvmeir/familyplanner/internal/crypto"
	"github.com/stretchr/testify/require"
)

func key(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

func TestSealOpenRoundTrip(t *testing.T) {
	k := key("secret")
	plaintext := []byte("mom's refresh token")

	token, err := crypto.Seal(plaintext, k)
	require.NoError(t, err)
	require.NotContains(t, token, "refresh", "ciphertext must not leak plaintext")

	got, err := crypto.Open(token, k)
	require.NoError(t, err)
	require.Equal(t, plaintext, got)
}

func TestOpenWithWrongKeyFails(t *testing.T) {
	token, err := crypto.Seal([]byte("data"), key("right"))
	require.NoError(t, err)

	_, err = crypto.Open(token, key("wrong"))
	require.Error(t, err)
}

func TestOpenDetectsTampering(t *testing.T) {
	k := key("secret")
	token, err := crypto.Seal([]byte("data"), k)
	require.NoError(t, err)

	// flip a character in the middle of the token
	b := []byte(token)
	b[len(b)/2] ^= 0x01
	_, err = crypto.Open(string(b), k)
	require.Error(t, err)
}
