// Package oauth builds OAuth2 configs for the known data-source provider types.
package oauth

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/microsoft"
)

// ErrorKind classifies a token-refresh failure for health reporting.
type ErrorKind string

const (
	// ErrReconnect means the refresh token was rejected (OAuth "invalid_grant"):
	// the data source must be reconnected interactively.
	ErrReconnect ErrorKind = "reconnect"
	// ErrTransient means a temporary failure (network, provider hiccup) that will
	// likely self-heal on the next refresh.
	ErrTransient ErrorKind = "error"
)

// ClassifyError inspects a FreshToken error. An OAuth "invalid_grant" response
// means the refresh token is dead (revoked/expired/consent withdrawn) and needs
// an interactive reconnect; everything else is treated as transient.
func ClassifyError(err error) ErrorKind {
	if err == nil {
		return ""
	}
	var re *oauth2.RetrieveError
	if errors.As(err, &re) && re.ErrorCode == "invalid_grant" {
		return ErrReconnect
	}
	if strings.Contains(strings.ToLower(err.Error()), "invalid_grant") {
		return ErrReconnect
	}
	return ErrTransient
}

type providerDef struct {
	endpoint oauth2.Endpoint
	scopes   []string
}

var providers = map[string]providerDef{
	"ms_graph": {
		endpoint: microsoft.AzureADEndpoint("common"),
		scopes:   []string{"offline_access", "Calendars.Read", "Tasks.Read"},
	},
	"google_photos": {
		endpoint: google.Endpoint,
		scopes:   []string{"https://www.googleapis.com/auth/photoslibrary.readonly"},
	},
	"onedrive": {
		endpoint: microsoft.AzureADEndpoint("common"),
		scopes:   []string{"offline_access", "Files.Read"},
	},
	"ms_todo": {
		endpoint: microsoft.AzureADEndpoint("common"),
		scopes:   []string{"offline_access", "Tasks.Read"},
	},
}

// AuthOptions returns provider-specific authorize options (e.g. Google needs
// access_type=offline + prompt=consent to return a refresh token).
func AuthOptions(dsType string) []oauth2.AuthCodeOption {
	opts := []oauth2.AuthCodeOption{oauth2.AccessTypeOffline}
	if dsType == "google_photos" {
		opts = append(opts, oauth2.ApprovalForce)
	}
	return opts
}

// Known reports whether a data-source type uses OAuth2.
func Known(dsType string) bool {
	_, ok := providers[dsType]
	return ok
}

// Config builds an *oauth2.Config for the given provider type. redirect may be
// empty for refresh-only use.
func Config(dsType, clientID, clientSecret, redirect string) (*oauth2.Config, bool) {
	p, ok := providers[dsType]
	if !ok {
		return nil, false
	}
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     p.endpoint,
		RedirectURL:  redirect,
		Scopes:       p.scopes,
	}, true
}

// FreshToken returns a valid token, refreshing tok if expired. The caller should
// persist the result if it differs from the stored token.
func FreshToken(ctx context.Context, dsType, clientID, clientSecret string, tok *oauth2.Token) (*oauth2.Token, error) {
	cfg, ok := Config(dsType, clientID, clientSecret, "")
	if !ok {
		return nil, errors.New("oauth: unknown provider")
	}
	return cfg.TokenSource(ctx, tok).Token()
}
