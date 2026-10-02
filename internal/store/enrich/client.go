package enrich

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Options configure a Client.
type Options struct {
	// Dir holds the disk cache (%LOCALAPPDATA%\Seaglass\store\enrich).
	Dir string
	// Transport replaces the network for tests; nil uses the default,
	// which only reaches the allowlisted provider hosts over HTTPS.
	Transport http.RoundTripper
	// Now replaces the clock for tests; nil uses time.Now.
	Now func() time.Time
}

// ErrNotImplemented was returned by the stubs before the providers existed.
// Nothing returns it any more; it stays so existing references compile.
var ErrNotImplemented = errors.New("not implemented yet")

// Client answers enrichment requests from its cache and the providers.
// Safe for concurrent use. Its methods never return an error for a
// provider failure: the answer's State and Error say what happened, and a
// cached answer is kept (StateStale) rather than dropped.
type Client struct {
	opts  Options
	http  *http.Client
	now   func() time.Time
	sleep func(context.Context, time.Duration) error
	hosts map[string]*hostState

	cmu     sync.Mutex // guards entries, dirty, written
	entries map[string]map[string]cacheEntry
	dirty   map[string]bool
	written map[string]time.Time
	wmu     sync.Mutex // one disk write at a time

	fmu     sync.Mutex
	flights map[string]*flight

	tmu      sync.Mutex
	hltbTok  string
	hltbTokT time.Time
}

// Gaps between requests to one host. Steam's store API allows about 200
// requests per five minutes; internal/meta spaces them the same way.
const (
	gapSteamStore = 1500 * time.Millisecond
	gapSteamAPI   = time.Second
	gapHLTB       = 2 * time.Second
)

// New opens the cache in o.Dir.
func New(o Options) *Client {
	c := &Client{
		opts:    o,
		now:     o.Now,
		sleep:   realSleep,
		entries: map[string]map[string]cacheEntry{},
		dirty:   map[string]bool{},
		written: map[string]time.Time{},
		flights: map[string]*flight{},
	}
	if c.now == nil {
		c.now = time.Now
	}
	gaps := map[string]time.Duration{hostSteamStore: gapSteamStore, hostSteamAPI: gapSteamAPI, hostHLTB: gapHLTB}
	rt := o.Transport
	if rt == nil {
		rt = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
			MaxIdleConnsPerHost:   1,
		}
	} else {
		// Tests replay recorded answers; there is nothing to space out.
		for h := range gaps {
			gaps[h] = 0
		}
	}
	c.hosts = map[string]*hostState{}
	for h, g := range gaps {
		c.hosts[h] = newHostState(h, g)
	}
	c.http = &http.Client{
		Transport: rt,
		Timeout:   requestTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return errors.New("too many redirects")
			}
			if !allowed(req.URL) {
				return ErrNotAllowed
			}
			return nil
		},
	}
	c.load()
	return c
}

// Popularity returns Steam's most-played chart (cached for an hour).
func (c *Client) Popularity(ctx context.Context) Popularity {
	v, oc, err := resolve(c, ctx, kindPopularity, "chart", PopularityTTL, c.fetchChart)
	switch oc {
	case fromCache, fetched:
		v.State, v.Error = StateOK, ""
	case staleCache:
		v.State, v.Error = StateStale, err.Error()
	default:
		v = Popularity{State: failState(err), Error: err.Error()}
	}
	if v.Ranks == nil {
		v.Ranks = map[int]int{}
	}
	return v
}

// CachedPopularity returns the cached chart without asking Steam; State is
// StateUnavailable when there is none.
func (c *Client) CachedPopularity() Popularity {
	v, at, ok := cacheGet[Popularity](c, kindPopularity, "chart")
	if !ok {
		return Popularity{Ranks: map[int]int{}, State: StateUnavailable}
	}
	v.State, v.Error = c.cachedState(at, PopularityTTL), ""
	if v.Ranks == nil {
		v.Ranks = map[int]int{}
	}
	return v
}

// cachedState is how a cached answer is labelled when nothing is fetched.
func (c *Client) cachedState(at time.Time, ttl time.Duration) string {
	if c.now().Sub(at) < ttl {
		return StateOK
	}
	return StateStale
}

// ReviewSummary returns a game's Steam review scores (cached six hours).
func (c *Client) ReviewSummary(ctx context.Context, appID int) ReviewSummary {
	if appID <= 0 {
		return ReviewSummary{AppID: appID, State: StateUnavailable, Error: "no Steam AppID"}
	}
	v, oc, err := resolve(c, ctx, kindSummary, strconv.Itoa(appID), ReviewSummaryTTL, func(ctx context.Context) (ReviewSummary, error) {
		return c.fetchSummary(ctx, appID)
	})
	switch oc {
	case fromCache, fetched:
		v.State, v.Error = StateOK, ""
	case staleCache:
		v.State, v.Error = StateStale, err.Error()
	default:
		v = ReviewSummary{AppID: appID, URL: reviewsURL(appID), State: failState(err), Error: err.Error()}
	}
	return v
}

// CachedReviewSummary returns a cached summary without asking Steam.
func (c *Client) CachedReviewSummary(appID int) (ReviewSummary, bool) {
	v, at, ok := cacheGet[ReviewSummary](c, kindSummary, strconv.Itoa(appID))
	if !ok {
		return ReviewSummary{}, false
	}
	v.State, v.Error = c.cachedState(at, ReviewSummaryTTL), ""
	return v, true
}

// Reviews returns one page of a game's Steam reviews (cached an hour).
func (c *Client) Reviews(ctx context.Context, q ReviewQuery) ReviewPage {
	if q.AppID <= 0 {
		return ReviewPage{AppID: q.AppID, Reviews: []Review{}, State: StateUnavailable, Error: "no Steam AppID"}
	}
	q = cleanReviewQuery(q)
	v, oc, err := resolve(c, ctx, kindReviews, reviewKey(q), ReviewPageTTL, func(ctx context.Context) (ReviewPage, error) {
		return c.fetchReviews(ctx, q)
	})
	switch oc {
	case fromCache, fetched:
		v.State, v.Error = StateOK, ""
	case staleCache:
		v.State, v.Error = StateStale, err.Error()
	default:
		v = ReviewPage{AppID: q.AppID, State: failState(err), Error: err.Error()}
	}
	if v.Reviews == nil {
		v.Reviews = []Review{}
	}
	return v
}

// Critic returns the Metacritic score and link Steam gives (cached seven days).
func (c *Client) Critic(ctx context.Context, appID int) Critic {
	if appID <= 0 {
		return Critic{AppID: appID, State: StateUnavailable, Error: "no Steam AppID"}
	}
	v, oc, err := resolve(c, ctx, kindCritic, strconv.Itoa(appID), CriticTTL, func(ctx context.Context) (Critic, error) {
		return c.fetchCritic(ctx, appID)
	})
	switch oc {
	case fromCache, fetched:
		v.State, v.Error = StateOK, ""
	case staleCache:
		v.State, v.Error = StateStale, err.Error()
	default:
		v = Critic{AppID: appID, State: failState(err), Error: err.Error()}
	}
	return v
}
