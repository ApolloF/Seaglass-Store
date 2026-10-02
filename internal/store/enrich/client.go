package enrich

import (
	"context"
	"errors"
	"net/http"
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

// ErrNotImplemented is returned until the providers are implemented
// (work package 3 of docs/store-discovery-plan.md).
var ErrNotImplemented = errors.New("not implemented yet")

// Client answers enrichment requests from its cache and the providers.
// Safe for concurrent use. Its methods never return an error for a
// provider failure: the answer's State and Error say what happened, and a
// cached answer is kept (StateStale) rather than dropped.
type Client struct {
	opts Options
}

// New opens the cache in o.Dir.
func New(o Options) *Client {
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Client{opts: o}
}

// Popularity returns Steam's most-played chart (cached for an hour).
func (c *Client) Popularity(ctx context.Context) Popularity {
	return Popularity{Ranks: map[int]int{}, State: StateUnavailable, Error: ErrNotImplemented.Error()}
}

// CachedPopularity returns the cached chart without asking Steam; State is
// StateUnavailable when there is none.
func (c *Client) CachedPopularity() Popularity {
	return Popularity{Ranks: map[int]int{}, State: StateUnavailable}
}

// ReviewSummary returns a game's Steam review scores (cached six hours).
func (c *Client) ReviewSummary(ctx context.Context, appID int) ReviewSummary {
	return ReviewSummary{AppID: appID, State: StateUnavailable, Error: ErrNotImplemented.Error()}
}

// CachedReviewSummary returns a cached summary without asking Steam.
func (c *Client) CachedReviewSummary(appID int) (ReviewSummary, bool) {
	return ReviewSummary{}, false
}

// Reviews returns one page of a game's Steam reviews (cached an hour).
func (c *Client) Reviews(ctx context.Context, q ReviewQuery) ReviewPage {
	return ReviewPage{AppID: q.AppID, Reviews: []Review{}, State: StateUnavailable, Error: ErrNotImplemented.Error()}
}

// Critic returns the Metacritic score and link Steam gives (cached seven days).
func (c *Client) Critic(ctx context.Context, appID int) Critic {
	return Critic{AppID: appID, State: StateUnavailable, Error: ErrNotImplemented.Error()}
}

// Completion returns HowLongToBeat times (cached seven days). With
// q.HLTBID set it reads that game; otherwise it searches q.Title and
// accepts only a conservative match (no sequel, remaster or DLC).
func (c *Client) Completion(ctx context.Context, q CompletionQuery) Completion {
	return Completion{State: StateUnavailable, Error: ErrNotImplemented.Error()}
}

// CachedCompletion returns cached times without asking HowLongToBeat.
func (c *Client) CachedCompletion(q CompletionQuery) (Completion, bool) {
	return Completion{}, false
}

// Candidates searches HowLongToBeat so the person can choose the match.
func (c *Client) Candidates(ctx context.Context, title string) ([]Candidate, error) {
	return nil, ErrNotImplemented
}

// Flush writes the cache to disk.
func (c *Client) Flush() error { return nil }
