package discovery

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/scan"
	"github.com/ApolloF/Seaglass/internal/store/sources"
)

// SearchTTL is how long a remote search answer is reused.
const SearchTTL = time.Hour

// MinSearch is the shortest query sent to a source site or Steam.
const MinSearch = 2

// SearchCache remembers remote search answers for an hour, so typing the
// same query again doesn't ask the sites again. Safe for concurrent use.
type SearchCache struct {
	mu sync.Mutex
	m  map[string]searchAnswer
}

type searchAnswer struct {
	at    time.Time
	found int
	steam []SteamHit
}

// SteamHit is one Steam search result.
type SteamHit struct {
	AppID int
	Name  string
}

func cacheKey(provider, text string) string { return provider + "\x00" + scan.Normalize(text) }

// Source returns a cached source search: how many releases it found.
func (c *SearchCache) Source(provider, text string, now time.Time) (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a, ok := c.m[cacheKey(provider, text)]
	if !ok || now.Sub(a.at) >= SearchTTL {
		return 0, false
	}
	return a.found, true
}

// Steam returns a cached Steam search.
func (c *SearchCache) Steam(text string, now time.Time) ([]SteamHit, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a, ok := c.m[cacheKey("steam", text)]
	if !ok || now.Sub(a.at) >= SearchTTL {
		return nil, false
	}
	return a.steam, true
}

// Put remembers an answer. Failures are not remembered: the next search
// tries again.
func (c *SearchCache) Put(provider, text string, found int, steam []SteamHit, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]searchAnswer{}
	}
	for k, a := range c.m {
		if now.Sub(a.at) >= SearchTTL {
			delete(c.m, k)
		}
	}
	c.m[cacheKey(provider, text)] = searchAnswer{at: now, found: found, steam: steam}
}

// SearchSource asks a source site's own search and indexes what it finds.
// Releases published more than a week ago count as history (backfill),
// so finding them never looks like news.
func SearchSource(ctx context.Context, ix *Index, src sources.Source, f Fetcher, text string, now time.Time) (int, error) {
	raw, err := src.SearchURL(strings.TrimSpace(text))
	if err != nil {
		return 0, err
	}
	doc, err := f.FetchDocument(ctx, raw)
	if err != nil {
		return 0, err
	}
	entries, err := sources.Parse(src, doc.URL, doc.Body)
	if err != nil {
		return 0, err
	}
	var recent, older []sources.Entry
	for _, e := range entries {
		if e.PublishedAt != nil && now.Sub(*e.PublishedAt) > 7*24*time.Hour {
			older = append(older, e)
		} else {
			recent = append(recent, e)
		}
	}
	ix.Merge(src.ID, recent, OriginSearch, false, now)
	ix.Merge(src.ID, older, OriginSearch, true, now)
	return len(entries), nil
}
