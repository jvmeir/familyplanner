// Package health turns raw data-source auth state and widget-cache sync state
// into a small, display-ready summary for the kiosk badge and admin. It is pure
// (no DB/HTTP) so it is trivially testable; the server maps rows into its inputs.
package health

import (
	"sort"
	"time"
)

// Level is a severity, ordered ok < warn < error.
type Level string

const (
	LevelOK    Level = "ok"
	LevelWarn  Level = "warn"  // amber: expired access, failed sync, stale data
	LevelError Level = "error" // red: auth needs an interactive reconnect
)

func (l Level) rank() int {
	switch l {
	case LevelError:
		return 2
	case LevelWarn:
		return 1
	default:
		return 0
	}
}

// Source is the health-relevant state of a data source.
type Source struct {
	Name         string
	IsOAuth      bool
	OAuthStatus  string // "connected" once an OAuth source has been linked
	Health       string // "" | ok | reconnect | error (written by the broker)
	AccessExpiry string // RFC3339 access-token expiry, or ""
}

// Widget is the sync state of a widget's cache.
type Widget struct {
	Name      string
	Status    string // ok | stale | error | ""
	FetchedAt string // "2006-01-02 15:04:05" UTC, or ""
}

// Issue is one problem worth surfacing.
type Issue struct {
	Level   Level  `json:"level"`
	Kind    string `json:"kind"`    // reconnect | expired | sync | stale
	Subject string `json:"subject"` // source/widget name
	Message string `json:"message"` // Dutch, display-ready
}

// Summary is the aggregate: the worst level, a count, and the ranked issues.
type Summary struct {
	Level  Level   `json:"level"`
	Count  int     `json:"count"`
	Issues []Issue `json:"issues"`
}

const sqlTime = "2006-01-02 15:04:05"

// Assess evaluates sources + widgets as of now. staleAfter is how old a widget's
// last successful fetch may be before it's flagged stale.
func Assess(sources []Source, widgets []Widget, now time.Time, staleAfter time.Duration) Summary {
	now = now.UTC()
	var issues []Issue

	for _, s := range sources {
		if !s.IsOAuth {
			continue // only OAuth sources have auth to go inactive
		}
		switch {
		case s.Health == "reconnect" || (s.OAuthStatus != "connected" && s.Health != "ok"):
			issues = append(issues, Issue{LevelError, "reconnect", s.Name, s.Name + ": opnieuw verbinden"})
		case s.Health == "error":
			issues = append(issues, Issue{LevelWarn, "sync", s.Name, s.Name + ": synchronisatie mislukt"})
		default:
			if exp, err := time.Parse(time.RFC3339, s.AccessExpiry); err == nil && exp.Before(now) {
				issues = append(issues, Issue{LevelWarn, "expired", s.Name, s.Name + ": toegang verlopen"})
			}
		}
	}

	for _, w := range widgets {
		switch {
		case w.Status == "error" || w.Status == "stale":
			issues = append(issues, Issue{LevelWarn, "sync", w.Name, w.Name + ": synchronisatie mislukt"})
		case w.FetchedAt != "":
			if t, err := time.Parse(sqlTime, w.FetchedAt); err == nil && now.Sub(t) > staleAfter {
				issues = append(issues, Issue{LevelWarn, "stale", w.Name, w.Name + ": verouderd"})
			}
		}
	}

	// Worst first, so a badge showing one line shows the most urgent.
	sort.SliceStable(issues, func(i, j int) bool {
		return issues[i].Level.rank() > issues[j].Level.rank()
	})

	level := LevelOK
	for _, is := range issues {
		if is.Level.rank() > level.rank() {
			level = is.Level
		}
	}
	return Summary{Level: level, Count: len(issues), Issues: issues}
}
