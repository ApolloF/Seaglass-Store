package app

import (
	"context"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/store/discovery"
	"github.com/ApolloF/Seaglass/internal/store/enrich"
)

// enrichState fetches Steam reviews, the chart, Metacritic and
// HowLongToBeat for the games the interface shows, never the whole index.
type enrichState struct {
	c      *Core
	client *enrich.Client
	wake   chan struct{}

	mu     sync.Mutex
	queue  []string
	queued map[string]bool
	chart  bool // a chart fetch is running
}

func newEnrichState(c *Core) *enrichState {
	return &enrichState{c: c, client: enrich.New(enrich.Options{Dir: platform.CacheDir("store", "enrich")}), wake: make(chan struct{}, 1), queued: map[string]bool{}}
}

// cached is what's known about a game without asking anyone.
func (e *enrichState) cached(key string, appID int) enrich.Enrichment {
	out := enrich.Enrichment{Key: key, SteamAppID: appID,
		Reviews: enrich.ReviewSummary{AppID: appID, State: enrich.StateLoading}, Critic: enrich.Critic{AppID: appID, State: enrich.StateLoading},
		Completion: enrich.Completion{State: enrich.StateLoading}}
	if appID > 0 {
		if r, ok := e.client.CachedReviewSummary(appID); ok {
			out.Reviews = r
		}
		out.PopularRank = e.client.CachedPopularity().Ranks[appID]
	} else {
		out.Reviews.State, out.Reviews.Error = enrich.StateUnavailable, "No Steam match: reviews need one"
		out.Critic.State = enrich.StateUnavailable
	}
	return out
}

// request returns cached card enrichment and queues the rest, the first
// key first; EventStoreEnrichment brings each one.
func (e *enrichState) request(keys []string) []enrich.Enrichment {
	out := []enrich.Enrichment{}
	var missing []string
	for _, k := range keys {
		_, appID, ok := e.c.storeGameRef(k)
		if !ok {
			continue
		}
		en := e.cached(k, appID)
		if en.Reviews.State == enrich.StateOK || appID == 0 {
			out = append(out, en)
			continue
		}
		missing = append(missing, k)
	}
	e.mu.Lock()
	var fresh []string
	for _, k := range missing {
		if !e.queued[k] {
			e.queued[k] = true
			fresh = append(fresh, k)
		}
	}
	e.queue = append(fresh, e.queue...)
	if len(e.queue) > 500 {
		for _, k := range e.queue[500:] {
			delete(e.queued, k)
		}
		e.queue = e.queue[:500]
	}
	e.mu.Unlock()
	if len(fresh) > 0 {
		select {
		case e.wake <- struct{}{}:
		default:
		}
	}
	return out
}

// loop fetches queued review summaries one at a time (the client spaces
// requests per host), waiting while a game runs.
func (e *enrichState) loop(ctx context.Context) {
	for {
		e.mu.Lock()
		var k string
		if len(e.queue) > 0 {
			k = e.queue[0]
			e.queue = e.queue[1:]
			delete(e.queued, k)
		}
		e.mu.Unlock()
		if k == "" {
			select {
			case <-ctx.Done():
				_ = e.client.Flush()
				return
			case <-e.wake:
			}
			continue
		}
		if !e.c.Settings.Get().ExperimentalStore || !e.c.waitIdle(ctx) {
			continue
		}
		_, appID, ok := e.c.storeGameRef(k)
		if !ok || appID == 0 {
			continue
		}
		en := e.cached(k, appID)
		en.Reviews = e.client.ReviewSummary(ctx, appID)
		e.c.emit(EventStoreEnrichment, en)
	}
}

// game fetches everything for a game's page, missing or old parts first.
func (e *enrichState) game(ctx context.Context, key string) (enrich.Enrichment, error) {
	title, appID, ok := e.c.storeGameRef(key)
	if !ok {
		return enrich.Enrichment{}, errNotIndexed
	}
	out := e.cached(key, appID)
	var wg sync.WaitGroup
	if appID > 0 {
		wg.Add(2)
		go func() { defer wg.Done(); out.Reviews = e.client.ReviewSummary(ctx, appID) }()
		go func() { defer wg.Done(); out.Critic = e.client.Critic(ctx, appID) }()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		out.Completion = e.client.Completion(ctx, enrich.CompletionQuery{Title: title, Year: e.year(key), HLTBID: e.c.discovery.index().CompletionMatch(key)})
	}()
	wg.Wait()
	e.refreshChart()
	out.PopularRank = e.client.CachedPopularity().Ranks[appID]
	if appID == 0 {
		out.PopularRank = 0
	}
	e.c.emit(EventStoreEnrichment, out)
	return out, nil
}

// year is a game's release year from its fetched metadata (0 unknown).
func (e *enrichState) year(key string) int {
	year, _ := e.release(key)
	return year
}

// release is a game's release year and date from its fetched metadata
// (zero and "" unknown): the game's own date, never a source's.
func (e *enrichState) release(key string) (int, string) {
	if e.c.art == nil {
		return 0, ""
	}
	e.c.art.mu.Lock()
	defer e.c.art.mu.Unlock()
	if m := e.c.art.known[key]; m != nil {
		return m.ReleaseYear, m.ReleaseDate
	}
	return 0, ""
}

// cachedCompletionMain is the Main Story time in minutes from the cache
// alone (0 unknown); a card never asks HowLongToBeat.
func (e *enrichState) cachedCompletionMain(key, title string, year int) int {
	q := enrich.CompletionQuery{Title: title, Year: year, HLTBID: e.c.discovery.index().CompletionMatch(key)}
	if c, ok := e.client.CachedCompletion(q); ok && c.HLTBID > 0 && (c.State == enrich.StateOK || c.State == enrich.StateStale) {
		return c.Main
	}
	return 0
}

// popularState is the chart's state for the Popular shelf: a missing or
// old chart is fetched in the background ("loading" until it arrives).
func (e *enrichState) popularState() string {
	p := e.client.CachedPopularity()
	if p.State != enrich.StateOK {
		if e.refreshChart() && p.State == enrich.StateUnavailable {
			return discovery.StateLoading
		}
	}
	return p.State
}

// refreshChart fetches the chart when it's missing or older than an hour;
// true when a fetch is running. The Store reloads its shelves after it.
func (e *enrichState) refreshChart() bool {
	if e.client.CachedPopularity().State == enrich.StateOK {
		return false
	}
	e.mu.Lock()
	if e.chart {
		e.mu.Unlock()
		return true
	}
	e.chart = true
	e.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(e.c.ctx, time.Minute)
		defer cancel()
		p := e.client.Popularity(ctx)
		e.mu.Lock()
		e.chart = false
		e.mu.Unlock()
		if p.State == enrich.StateOK {
			e.c.emit(EventStoreGames, discovery.Change{Keys: []string{}, All: true})
		}
	}()
	return true
}

// enrichAnnotator adds the chart rank, cached review scores, cached
// completion time and the game's own release date.
func (c *Core) enrichAnnotator() discovery.Annotate {
	if c.enrich == nil {
		return nil
	}
	ranks := c.enrich.client.CachedPopularity().Ranks
	return func(s *discovery.GameSummary) {
		year, date := c.enrich.release(s.Key)
		s.ReleaseDate = date
		s.CompletionMain = c.enrich.cachedCompletionMain(s.Key, s.Title, year)
		if s.SteamAppID <= 0 {
			return
		}
		s.PopularRank = ranks[s.SteamAppID]
		if r, ok := c.enrich.client.CachedReviewSummary(s.SteamAppID); ok && r.Overall.Total > 0 {
			s.ReviewPercent, s.ReviewTotal, s.ReviewLabel = r.Overall.Percent, r.Overall.Total, r.Overall.Label
		}
	}
}

// popularState is the state of Steam's chart for the Popular shelf.
func (c *Core) popularState() string {
	if c.enrich == nil {
		return discovery.StateUnavailable
	}
	return c.enrich.popularState()
}
