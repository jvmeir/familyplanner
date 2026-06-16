// Package i18n provides translation + locale-aware date formatting.
//
// Rule for the whole app: never hardcode user-facing text. Always reference a
// message ID and let the active locale resolve it. Dutch ("nl") is the default.
package i18n

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"time"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.json
var localeFS embed.FS

// Service holds the loaded message bundle.
type Service struct {
	bundle    *goi18n.Bundle
	defLocale string
}

// New loads every locale file under locales/ and sets the default language.
func New(defaultLocale string) (*Service, error) {
	tag, err := language.Parse(defaultLocale)
	if err != nil {
		tag = language.Dutch
		defaultLocale = "nl"
	}
	bundle := goi18n.NewBundle(tag)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)

	entries, err := fs.ReadDir(localeFS, "locales")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if _, err := bundle.LoadMessageFileFS(localeFS, "locales/"+e.Name()); err != nil {
			return nil, err
		}
	}
	return &Service{bundle: bundle, defLocale: defaultLocale}, nil
}

// Loc builds a localizer, preferring the given languages and falling back to the default.
func (s *Service) Loc(langs ...string) *Loc {
	langs = append(langs, s.defLocale)
	return &Loc{l: goi18n.NewLocalizer(s.bundle, langs...), Tag: langs[0]}
}

// Loc is a per-request localizer.
type Loc struct {
	l   *goi18n.Localizer
	Tag string
}

// T translates a message ID. Optional data is passed as template data; a "Count"
// key also drives pluralization. On a missing key it returns the ID itself so
// gaps are visible rather than silent.
func (lc *Loc) T(id string, data ...map[string]any) string {
	if lc == nil || lc.l == nil {
		return id
	}
	cfg := &goi18n.LocalizeConfig{MessageID: id}
	if len(data) > 0 && data[0] != nil {
		cfg.TemplateData = data[0]
		if c, ok := data[0]["Count"]; ok {
			cfg.PluralCount = c
		}
	}
	s, err := lc.l.Localize(cfg)
	if err != nil {
		return id
	}
	return s
}

type ctxKey struct{}

// WithLoc stores a localizer in the context (set by middleware, read by templates).
func WithLoc(ctx context.Context, lc *Loc) context.Context {
	return context.WithValue(ctx, ctxKey{}, lc)
}

// FromContext returns the localizer stored in ctx, or nil.
func FromContext(ctx context.Context) *Loc {
	lc, _ := ctx.Value(ctxKey{}).(*Loc)
	return lc
}

// T is the package-level helper templates call: i18n.T(ctx, "login.title").
func T(ctx context.Context, id string, data ...map[string]any) string {
	return FromContext(ctx).T(id, data...)
}

var nlMonths = [...]string{
	"januari", "februari", "maart", "april", "mei", "juni",
	"juli", "augustus", "september", "oktober", "november", "december",
}

// Date formats a time in Dutch long form, e.g. "30 mei 2026".
// (A full CLDR formatter per locale comes later; nl is the default for now.)
func Date(t time.Time) string {
	return itoa(t.Day()) + " " + nlMonths[t.Month()-1] + " " + itoa(t.Year())
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
