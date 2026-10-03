package app

import (
	"context"
	"path/filepath"
	"time"

	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/store/discovery"
	"github.com/ApolloF/Seaglass/internal/store/wishlist"
)

// searchIdle is how long the idle wishlist searches wait when there is
// nothing they may do; a resume, a game ending or an import wakes them
// sooner.
const searchIdle = 15 * time.Minute

// queue is the idle search queue, opened on first use beside the
// wishlist.
func (w *wishlistState) queue() *wishlist.Searches {
	w.searchInit.Do(func() {
		w.searches = wishlist.OpenSearches(filepath.Join(filepath.Dir(w.store.Path()), "wishlist-searches.json"))
		w.searchKick = make(chan struct{}, 1)
	})
	return w.searches
}

// wakeSearches makes the idle searches look at their queue now.
func (w *wishlistState) wakeSearches() {
	w.queue()
	select {
	case w.searchKick <- struct{}{}:
	default:
	}
}

// searchLoop searches the source sites for queued wishlist games, one
// game at a time, while indexing could run.
func (w *wishlistState) searchLoop(ctx context.Context) {
	w.queue()
	for {
		t := time.NewTimer(w.searchNext(time.Now()))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-w.searchKick:
		case <-t.C:
		}
		t.Stop()
	}
}

// canSearch: a chosen source has its own site search, so queued games
// will be searched once indexing may run.
func (w *wishlistState) canSearch() bool {
	for _, src := range w.c.discovery.enabled() {
		if w.c.discovery.searchable(src) {
			return true
		}
	}
	return false
}

// searchSources are the chosen sources that may be searched now: with a
// site search, not halted and not backing off.
func (w *wishlistState) searchSources(now time.Time) []string {
	d := w.c.discovery
	var out []string
	for _, src := range d.enabled() {
		if d.searchable(src) && !d.halted(src) && !now.Before(d.index().Crawl(src).RetryAt) {
			out = append(out, src)
		}
	}
	return out
}

// searchNext searches for the next queued game, if one may be searched
// now, and says how long to wait before looking again.
func (w *wishlistState) searchNext(now time.Time) time.Duration {
	d := w.c.discovery
	srcs := w.searchSources(now)
	if len(srcs) == 0 {
		return searchIdle
	}
	q := w.queue()
	for {
		key, wait := q.Next(now)
		if key == "" {
			if wait > 0 {
				return wait
			}
			return searchIdle
		}
		e, ok := w.store.Get(key)
		if !ok || w.available(key) {
			w.logErr(q.Drop(key, now))
			continue
		}
		ctx := d.indexCtx()
		title, err := w.searchTitle(ctx, e)
		if err != nil {
			// Steam can't name the game right now; it stays queued.
			logx.Printf("store wishlist: naming %s: %v", key, err)
			return searchIdle
		}
		if title == "" {
			// Nothing to search with: Steam doesn't know the game.
			w.logErr(q.Done(key, now))
			continue
		}
		added := false
		for _, src := range srcs {
			if d.halted(src) || ctx.Err() != nil {
				return searchIdle // stays queued for when indexing may run again
			}
			f, s, err := d.fetcher(src)
			if err != nil {
				logx.Printf("store wishlist: %s: %v", src, err)
				continue
			}
			before := d.index().Counts()[src]
			n, err := discovery.SearchSource(ctx, d.index(), s, f, title, time.Now())
			if err != nil {
				if ctx.Err() != nil {
					return searchIdle
				}
				logx.Printf("store wishlist: searching %s for %q: %v", src, title, err)
				continue
			}
			d.search.Put(src, title, n, nil, time.Now())
			added = added || d.index().Counts()[src] > before
		}
		w.logErr(q.Done(key, now))
		if added {
			d.save()
			d.indexChanged(nil, "")
		}
		_, wait = q.Next(now)
		return wait
	}
}

// available: the game has a known source release.
func (w *wishlistState) available(key string) bool {
	g, ok := w.c.discovery.currentView().Game(key)
	return ok && len(g.Records) > 0
}

// searchTitle is the name to search the sites with. An imported game
// still named by its placeholder is named from what Seaglass knows, or
// by asking Steam; "" when Steam doesn't know it.
func (w *wishlistState) searchTitle(ctx context.Context, e wishlist.Entry) (string, error) {
	if e.SteamAppID == 0 || e.Title != wishlist.Placeholder(e.SteamAppID) {
		return e.Title, nil
	}
	name := w.knownName(e.SteamAppID)
	if name == "" && w.c.enrich != nil {
		var err error
		if name, err = w.c.enrich.client.SteamAppName(ctx, e.SteamAppID); err != nil {
			return "", err
		}
	}
	if name == "" {
		return "", nil
	}
	if ok, err := w.store.Name(e.Key, name); err != nil {
		logx.Printf("store wishlist: %v", err)
	} else if ok {
		w.changed()
	}
	return name, nil
}

func (w *wishlistState) logErr(err error) {
	if err != nil {
		logx.Printf("store wishlist: %v", err)
	}
}
