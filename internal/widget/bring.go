package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// bringBase is overridable in tests. Bring has no official public API; these are
// the community-known REST endpoints.
var bringBase = "https://api.getbring.com/rest/v2"

// bringLocaleURL is Bring's public catalog mapping the canonical (German) item
// names to a target locale — used to localize list items (which the API returns
// in the catalog's base German). Overridable in tests.
var bringLocaleURL = "https://web.getbring.com/locale/articles.nl-NL.json"

var (
	bringCatMu  sync.Mutex
	bringCatMap map[string]string // German name -> Dutch name
	bringCatAt  time.Time
)

// bringCatalog returns the German→Dutch item-name map, fetched once and cached
// for a day. On any failure it returns nil (items are then shown untranslated).
func bringCatalog(ctx context.Context) map[string]string {
	bringCatMu.Lock()
	defer bringCatMu.Unlock()
	if bringCatMap != nil && time.Since(bringCatAt) < 24*time.Hour {
		return bringCatMap
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bringLocaleURL, nil)
	if err != nil {
		return bringCatMap
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return bringCatMap
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return bringCatMap
	}
	var m map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil || len(m) == 0 {
		return bringCatMap
	}
	bringCatMap, bringCatAt = m, time.Now()
	return bringCatMap
}

// bringLocalize maps a catalog item name to Dutch (unchanged if not in the
// catalog, e.g. a freely-typed custom item).
func bringLocalize(cat map[string]string, name string) string {
	if t, ok := cat[name]; ok && t != "" {
		return t
	}
	return name
}

const bringAPIKey = "cof4Nc6D8saplXjE3h3HXqHH8m7VU2i1Gs0g85Sp"

type bringAuth struct {
	AccessToken   string `json:"access_token"`
	UUID          string `json:"uuid"`
	BringListUUID string `json:"bringListUUID"`
}

func bringHeaders(req *http.Request, auth *bringAuth) {
	req.Header.Set("X-BRING-API-KEY", bringAPIKey)
	req.Header.Set("X-BRING-CLIENT", "webApp")
	if auth != nil {
		req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	}
}

func bringLogin(ctx context.Context, email, password string) (bringAuth, error) {
	form := url.Values{"email": {email}, "password": {password}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, bringBase+"/bringauth", strings.NewReader(form.Encode()))
	if err != nil {
		return bringAuth{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	bringHeaders(req, nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return bringAuth{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return bringAuth{}, fmt.Errorf("bring: auth status %d", resp.StatusCode)
	}
	var a bringAuth
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		return bringAuth{}, err
	}
	return a, nil
}

func bringResolveList(ctx context.Context, auth bringAuth, name string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bringBase+"/bringusers/"+auth.UUID+"/lists", nil)
	if err != nil {
		return ""
	}
	bringHeaders(req, &auth)
	resp, err := httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var body struct {
		Lists []struct {
			ListUUID string `json:"listUuid"`
			Name     string `json:"name"`
		} `json:"lists"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ""
	}
	for _, l := range body.Lists {
		if strings.EqualFold(l.Name, name) {
			return l.ListUUID
		}
	}
	return ""
}

func bringItems(ctx context.Context, auth bringAuth, listUUID string) ([]string, error) {
	if listUUID == "" {
		listUUID = auth.BringListUUID
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bringBase+"/bringlists/"+listUUID, nil)
	if err != nil {
		return nil, err
	}
	bringHeaders(req, &auth)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bring: list status %d", resp.StatusCode)
	}
	var body struct {
		Purchase []struct {
			Name          string `json:"name"`
			Specification string `json:"specification"`
		} `json:"purchase"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	cat := bringCatalog(ctx) // German -> Dutch (nil on failure = no translation)
	items := make([]string, 0, len(body.Purchase))
	for _, p := range body.Purchase {
		name := bringLocalize(cat, p.Name)
		if p.Specification != "" {
			items = append(items, name+" ("+p.Specification+")")
		} else {
			items = append(items, name)
		}
	}
	return items, nil
}

// BringLists returns the user's lists (for the configure picker). The option ID
// is the list name, which bringFetch resolves.
func BringLists(ctx context.Context, email, password string) ([]ResourceOption, error) {
	auth, err := bringLogin(ctx, email, password)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bringBase+"/bringusers/"+auth.UUID+"/lists", nil)
	if err != nil {
		return nil, err
	}
	bringHeaders(req, &auth)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bring: lists status %d", resp.StatusCode)
	}
	var body struct {
		Lists []struct {
			ListUUID string `json:"listUuid"`
			Name     string `json:"name"`
		} `json:"lists"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]ResourceOption, 0, len(body.Lists))
	for _, l := range body.Lists {
		out = append(out, ResourceOption{ID: l.Name, Name: l.Name})
	}
	return out, nil
}

// bringFetch logs in and returns the items on the (named, or default) list.
func bringFetch(ctx context.Context, email, password, listName string) ([]string, error) {
	auth, err := bringLogin(ctx, email, password)
	if err != nil {
		return nil, err
	}
	listUUID := auth.BringListUUID
	if listName != "" {
		if u := bringResolveList(ctx, auth, listName); u != "" {
			listUUID = u
		}
	}
	return bringItems(ctx, auth, listUUID)
}
