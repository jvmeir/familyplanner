package oauth

import (
	"errors"
	"testing"

	"golang.org/x/oauth2"
)

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want ErrorKind
	}{
		{"nil", nil, ""},
		{"retrieve invalid_grant", &oauth2.RetrieveError{ErrorCode: "invalid_grant"}, ErrReconnect},
		{"retrieve other", &oauth2.RetrieveError{ErrorCode: "temporarily_unavailable"}, ErrTransient},
		{"string invalid_grant", errors.New("oauth2: cannot fetch token: invalid_grant"), ErrReconnect},
		{"network", errors.New("dial tcp: i/o timeout"), ErrTransient},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyError(tc.err); got != tc.want {
				t.Fatalf("ClassifyError(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
