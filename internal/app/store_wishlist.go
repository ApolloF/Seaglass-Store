package app

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/store/discovery"
	"github.com/ApolloF/Seaglass/internal/store/wishlist"
)

// wishlistState keeps the Store's wishlist on this PC and turns new
// releases of saved games into in-store activity (no notifications).
type wishlistState struct {
	c     *Core
	store *wishlist.Store

	// The idle searches for imported games without a known source release.
	searchInit sync.Once
	searches   *wishlist.Searches
	searchKick chan struct{}
}

func newWishlistState(c *Core) *wishlistState {
	return &wishlistState{c: c, store: wishlist.Open(filepath.Join(platform.AppDir(), "wishlist.json"))}
}

// observations are a game's current source releases, as the wishlist sees
// them.
func (w *wishlistState) observations(d *discoveryState, key string) []wishlist.Observation {
	g, ok := d.currentView().Game(key)
	if !ok {
		return nil
	}
	var out []wishlist.Observation
	for _, r := range g.Records {
		if r.Entry.ReleaseKind == "preview" {
			// An announcement is not a release: it becomes news once the
			// source publishes the game itself.
			continue
		}
		o := wishlist.Observation{ReleaseID: r.Entry.ID, Source: r.Entry.SourceID, SourceName: d.sourceName(r.Entry.SourceID), Version: r.Entry.Version, Backfill: r.Backfill}
		if r.Entry.PublishedAt != nil {
			o.PublishedAt = *r.Entry.PublishedAt
		}
		out = append(out, o)
	}
	return out
}

// observe compares saved games' releases with their baselines, and asks
// for the articles of saved games known only from a search summary.
func (w *wishlistState) observe(d *discoveryState) {
	entries := w.store.List()
	changed := w.nameImported(entries)
	for _, e := range entries {
		obs := w.observations(d, e.Key)
		if ok, err := w.store.Observe(e.Key, obs, time.Now()); err != nil {
			logx.Printf("store wishlist: %v", err)
		} else if ok {
			changed = true
		}
		if g, ok := d.currentView().Game(e.Key); ok {
			for _, r := range g.Records {
				if r.Entry.SummaryOnly && !r.Gone && r.FetchError == "" {
					d.queueDetail(r.Entry.SourceID, r.Entry.ID, e.Key, false)
				}
			}
		}
	}
	if changed {
		w.changed()
	}
}

func (w *wishlistState) changed() {
	w.c.emit(EventStoreWishlist, w.items())
}

// items is the wishlist for the interface, most recently saved first.
func (w *wishlistState) items() []WishlistItem {
	d := w.c.discovery
	view := d.currentView()
	ann := d.annotator()
	out := []WishlistItem{}
	for _, e := range w.store.List() {
		act, unread := wishlist.View(e)
		it := WishlistItem{Key: e.Key, Title: e.Title, SteamAppID: e.SteamAppID, AddedAt: e.AddedAt.Unix(), Activity: act, Unread: unread, Origin: e.Origin}
		if g, ok := view.Game(e.Key); ok {
			it.Game = g.Summary()
			ann(&it.Game)
		} else {
			// A Steam game without a source release, or one whose source
			// is turned off right now.
			it.Game = discovery.GameSummary{Key: e.Key, Title: e.Title, SteamAppID: e.SteamAppID, Sources: []string{}, Languages: []string{}, Genres: []string{}}
			ann(&it.Game)
		}
		out = append(out, it)
	}
	return out
}

func (w *wishlistState) add(key, title string, appID int) error {
	title = strings.TrimSpace(title)
	if t, id, ok := w.c.discovery.gameRef(key); ok {
		if title == "" {
			title = t
		}
		if appID == 0 {
			appID = id
		}
	}
	if key == "" || title == "" {
		return errNotIndexed
	}
	if err := w.store.Add(key, title, appID, w.observations(w.c.discovery, key), time.Now()); err != nil {
		return err
	}
	w.changed()
	return nil
}

// title is a saved game's title ("" when it isn't saved).
func (w *wishlistState) title(key string) string {
	if e, ok := w.store.Get(key); ok {
		return e.Title
	}
	return ""
}

// keys are the saved games' keys.
func (w *wishlistState) keys() []string {
	var out []string
	for _, e := range w.store.List() {
		out = append(out, e.Key)
	}
	return out
}

// rekey follows a game whose key changed after a Steam correction.
func (w *wishlistState) rekey(old, key, name string, appID int) {
	if err := w.store.Rekey(old, key, name, appID); err != nil {
		logx.Printf("store wishlist: %v", err)
	}
	w.changed()
}

// wishlistAnnotator marks saved games and their unread activity.
func (c *Core) wishlistAnnotator() discovery.Annotate {
	if c.wishlist == nil {
		return nil
	}
	saved := map[string]bool{}
	for _, e := range c.wishlist.store.List() {
		_, unread := wishlist.View(e)
		saved[e.Key] = unread > 0
	}
	return func(s *discovery.GameSummary) {
		if unread, ok := saved[s.Key]; ok {
			s.Wishlisted, s.Activity = true, unread
		}
	}
}
