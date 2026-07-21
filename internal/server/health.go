package server

import (
	"context"
	"time"

	"github.com/jvmeir/familyplanner/internal/health"
	"github.com/jvmeir/familyplanner/internal/oauth"
)

// staleAfter is how old a widget's last successful fetch may be before the kiosk
// flags it stale.
const staleAfter = time.Hour

// buildHealth aggregates data-source auth health + widget sync health into a
// display-ready summary for the kiosk badge + admin. Read-only: it never calls
// external services (it reads state the broker already recorded).
func (s *Server) buildHealth(ctx context.Context) health.Summary {
	var sources []health.Source
	if rows, err := s.store.ListDataSources(ctx); err == nil {
		for _, ds := range rows {
			sources = append(sources, health.Source{
				Name:         ds.Name,
				IsOAuth:      oauth.Known(ds.Type),
				OAuthStatus:  ds.OauthStatus,
				Health:       ds.Health,
				AccessExpiry: ds.AccessExpiry,
			})
		}
	}
	var widgets []health.Widget
	if rows, err := s.store.ListWidgetHealth(ctx); err == nil {
		for _, wr := range rows {
			widgets = append(widgets, health.Widget{
				Name:      wr.WidgetName,
				Status:    wr.Status,
				FetchedAt: wr.FetchedAt,
			})
		}
	}
	return health.Assess(sources, widgets, s.now(), staleAfter)
}
