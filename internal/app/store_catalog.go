package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/safety"
	"github.com/ApolloF/Seaglass/internal/scan"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/store/catalog"
	"github.com/ApolloF/Seaglass/internal/store/feed"
	"github.com/ApolloF/Seaglass/internal/store/jobs"
	"github.com/ApolloF/Seaglass/internal/store/sources"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// EventStoreCatalog says the catalog changed (its new number of games).
const EventStoreCatalog = "store:catalog"

func init() {
	application.RegisterEvent[int](EventStoreCatalog)
}

// feedStale is how old a feed may get before it's fetched again.
const feedStale = 6 * time.Hour

// FeedInfo is a feed as Settings shows it.
type FeedInfo struct {
	feed.Status
	Enabled bool `json:"enabled"`
}

// catalogState holds the catalog built from the feeds the person added.
type catalogState struct {
	c        *Core
	cache    feed.Cache
	fetchMu  sync.Mutex // one round of fetching at a time
	mu       sync.RWMutex
	entries  []catalog.Entry
	previews map[string]sources.Snapshot
}

func newCatalogState(c *Core) *catalogState {
	return &catalogState{c: c, cache: feed.Cache{Dir: platform.CacheDir("store", "feeds")}}
}

// loop builds the catalog from the cached feeds, then keeps them fresh.
func (cs *catalogState) loop(ctx context.Context) {
	if cs.c.Settings.Get().ExperimentalStore {
		cs.rebuild()
	}
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	for {
		if cs.c.Settings.Get().ExperimentalStore && cs.c.waitIdle(ctx) {
			cs.refresh(ctx, false)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// refresh fetches the enabled feeds (only stale ones unless all) and
// rebuilds the catalog when one changed.
func (cs *catalogState) refresh(ctx context.Context, all bool) {
	cs.fetchMu.Lock()
	defer cs.fetchMu.Unlock()
	changed := false
	for _, f := range cs.c.Settings.Get().Store.Feeds {
		before := cs.cache.Status(f.URL)
		if !f.Enabled || (!all && time.Since(time.Unix(before.Fetched, 0)) < feedStale) {
			continue
		}
		st := cs.cache.Refresh(ctx, f.URL)
		if st.Error != "" {
			logx.Printf("store: feed %s: %s", f.URL, st.Error)
		}
		changed = changed || st.ETag != before.ETag || st.Items != before.Items || st.Name != before.Name
	}
	if changed || cs.len() == 0 {
		cs.rebuild()
	}
}

// rebuild makes the catalog from the enabled feeds' cached copies.
func (cs *catalogState) rebuild() {
	sources := cs.reviewedSources()
	for _, f := range cs.c.Settings.Get().Store.Feeds {
		if !f.Enabled {
			continue
		}
		if fd, ok := cs.cache.Load(f.URL); ok {
			sources = append(sources, catalog.Source{URL: f.URL, Name: fd.Name, Feed: fd})
		}
	}
	ix := cs.c.Manifest.Index()
	entries := catalog.Build(sources, func(title string, appID int) (string, int) {
		m := ix.Identify(scan.Candidate{Title: title, SteamAppID: appID, TitleTrusted: true, Source: scan.Installer, AppIDFrom: "the feed"})
		// Looser title matches could put two games together.
		if m.SteamAppID == 0 || m.Confidence < 72 {
			return "", 0
		}
		return m.Title, m.SteamAppID
	})
	cs.mu.Lock()
	cs.entries = entries
	cs.mu.Unlock()
	if len(entries) > 0 && cs.c.art != nil {
		keys := make(map[string]bool, len(entries))
		for _, e := range entries {
			keys[e.Key] = true
		}
		cs.c.art.forget(keys)
	}
	cs.c.emit(EventStoreCatalog, len(entries))
}

func (cs *catalogState) len() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return len(cs.entries)
}

func (cs *catalogState) search(q catalog.Query) catalog.Page {
	cs.mu.RLock()
	p := catalog.Search(cs.entries, q)
	cs.mu.RUnlock()
	ann := cs.annotator()
	for i := range p.Entries {
		ann(&p.Entries[i])
	}
	return p
}

// annotator adds what's known about this PC to entries: the version to
// recommend, and whether the store installed the game.
func (cs *catalogState) annotator() func(*catalog.Entry) {
	cfg := cs.c.Settings.Get().Store
	p := catalog.Prefs{Language: cfg.Language, Trust: map[string]int{}, Blocked: map[string]int{}}
	for _, f := range cfg.Feeds {
		p.Trust[f.URL] = f.Trust
	}
	installed := map[string]catalog.Installed{}
	for _, j := range cs.c.store.jobs.All() {
		if j.Safety != nil && j.Safety.Verdict == safety.Blocked && j.FeedName != "" {
			p.Blocked[j.FeedName]++
		}
		if j.State == jobs.Installed && j.GameKey != "" {
			if old, ok := installed[j.GameKey]; !ok || newerInstalled(j.Version, old.Version) {
				installed[j.GameKey] = catalog.Installed{Download: j.ID, Version: j.Version, Dir: j.InstallDir}
			}
		}
	}
	return func(e *catalog.Entry) {
		r := catalog.Recommend(*e, p)
		e.Recommended = &r
		if in, ok := installed[e.Key]; ok {
			var eligible []catalog.Offer
			var indexes []int
			for i, offer := range e.Offers {
				if order, comparable := catalog.CompareReleases(offer.Version, in.Version); comparable && order > 0 {
					eligible = append(eligible, offer)
					indexes = append(indexes, i)
				}
			}
			in.Update = len(eligible) > 0
			if in.Update {
				newer := catalog.Recommend(catalog.Entry{Offers: eligible}, p)
				newer.Offer = indexes[newer.Offer]
				e.Recommended = &newer
			}
			e.Installed = &in
		}
	}
}

// updates are the installed catalog games with a newer version.
func (cs *catalogState) updates() []catalog.Entry {
	cs.mu.RLock()
	all := slices.Clone(cs.entries)
	cs.mu.RUnlock()
	ann := cs.annotator()
	out := []catalog.Entry{}
	for i := range all {
		ann(&all[i])
		if all[i].Installed != nil && all[i].Installed.Update {
			out = append(out, all[i])
		}
	}
	return out
}

func (cs *catalogState) languages() []string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return catalog.Languages(cs.entries)
}

func (cs *catalogState) entry(key string) (catalog.Entry, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	i := slices.IndexFunc(cs.entries, func(e catalog.Entry) bool { return e.Key == key })
	if i < 0 {
		return catalog.Entry{}, false
	}
	e := cs.entries[i]
	cs.annotator()(&e)
	return e, true
}

func (cs *catalogState) feeds() []FeedInfo {
	out := []FeedInfo{}
	for _, f := range cs.c.Settings.Get().Store.Feeds {
		out = append(out, FeedInfo{Status: cs.cache.Status(f.URL), Enabled: f.Enabled})
	}
	return out
}

// addFeed fetches a new feed and, when it's a feed Seaglass can read,
// adds it.
func (cs *catalogState) addFeed(ctx context.Context, url string) (settings.Settings, error) {
	url = strings.TrimSpace(url)
	if err := feed.CheckURL(url); err != nil {
		return cs.c.Settings.Get(), err
	}
	if slices.ContainsFunc(cs.c.Settings.Get().Store.Feeds, func(f settings.FeedSource) bool { return f.URL == url }) {
		return cs.c.Settings.Get(), errors.New("that feed is added already")
	}
	cs.fetchMu.Lock()
	st := cs.cache.Refresh(ctx, url)
	cs.fetchMu.Unlock()
	if st.Error != "" {
		cs.cache.Forget(url)
		return cs.c.Settings.Get(), fmt.Errorf("couldn't add the feed: %s", st.Error)
	}
	saved, err := cs.c.updateSettings(func(v *settings.Settings) {
		v.Store.Feeds = append(v.Store.Feeds, settings.FeedSource{URL: url, Enabled: true})
	})
	if err == nil {
		logx.Printf("store: feed added: %s (%d items)", url, st.Items)
	}
	return saved, err
}

func (cs *catalogState) removeFeed(url string) (settings.Settings, error) {
	saved, err := cs.c.updateSettings(func(v *settings.Settings) {
		v.Store.Feeds = slices.DeleteFunc(v.Store.Feeds, func(f settings.FeedSource) bool { return f.URL == url })
	})
	if err == nil {
		cs.cache.Forget(url)
	}
	return saved, err
}

func (cs *catalogState) setFeedEnabled(url string, on bool) (settings.Settings, error) {
	return cs.c.updateSettings(func(v *settings.Settings) {
		for i := range v.Store.Feeds {
			if v.Store.Feeds[i].URL == url {
				v.Store.Feeds[i].Enabled = on
			}
		}
	})
}

// updateSettings changes the settings the way the interface saves them,
// on top of whatever was saved last.
func (c *Core) updateSettings(fn func(*settings.Settings)) (settings.Settings, error) {
	c.setMu.Lock()
	defer c.setMu.Unlock()
	old := c.Settings.Get()
	v := c.Settings.Get()
	fn(&v)
	saved, err := c.saveSettings(v)
	if err == nil && c.profile != nil {
		c.profile.settingsSaved(old, saved)
	}
	return saved, err
}

func newerInstalled(a, b string) bool {
	order, known := catalog.CompareReleases(a, b)
	return known && order > 0
}
