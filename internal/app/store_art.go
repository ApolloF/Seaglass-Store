package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/meta"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// EventStoreArt brings the interface a catalog game's art and description.
const EventStoreArt = "store:art"

// StoreArt is a catalog game's metadata, for the store's pages.
type StoreArt struct {
	Key  string        `json:"key"`
	Meta *library.Meta `json:"meta"` // nil: nothing was found
}

func init() {
	application.RegisterEvent[StoreArt](EventStoreArt)
}

// storeArtRetry is how long a game nothing was found for waits before
// it's looked up again.
const storeArtRetry = 7 * 24 * time.Hour

// artState fetches art and descriptions for catalog games, the ones the
// interface shows first, one at a time, and keeps them on disk.
type artState struct {
	c    *Core
	path string
	wake chan struct{}

	mu     sync.Mutex
	known  map[string]*library.Meta // nil value: looked up, nothing found
	tried  map[string]int64         // when a game with nothing found was looked up
	queue  []string
	queued map[string]bool
	dirty  bool
}

type artFile struct {
	Known map[string]*library.Meta `json:"known"`
	Tried map[string]int64         `json:"tried"`
}

func newArtState(c *Core) *artState {
	a := &artState{c: c, path: filepath.Join(platform.CacheDir("store"), "art.json"), wake: make(chan struct{}, 1),
		known: map[string]*library.Meta{}, tried: map[string]int64{}, queued: map[string]bool{}}
	if b, err := os.ReadFile(a.path); err == nil {
		var f artFile
		if json.Unmarshal(b, &f) == nil {
			if f.Known != nil {
				a.known = f.Known
			}
			if f.Tried != nil {
				a.tried = f.Tried
			}
		}
	}
	return a
}

// get returns what's known about these games and queues the rest, the
// first key first.
func (a *artState) get(keys []string) []StoreArt {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []StoreArt{}
	var missing []string
	for _, k := range keys {
		if m, ok := a.known[k]; ok && (m != nil || time.Since(time.Unix(a.tried[k], 0)) < storeArtRetry) {
			if m != nil {
				out = append(out, StoreArt{Key: k, Meta: m})
			}
			continue
		}
		if !a.queued[k] {
			missing = append(missing, k)
			a.queued[k] = true
		}
	}
	// Asked for now: ahead of what's still waiting from before.
	a.queue = append(missing, a.queue...)
	if len(missing) > 0 {
		select {
		case a.wake <- struct{}{}:
		default:
		}
	}
	return out
}

func (a *artState) next() (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.queue) == 0 {
		return "", false
	}
	k := a.queue[0]
	a.queue = a.queue[1:]
	delete(a.queued, k)
	return k, true
}

func (a *artState) loop(ctx context.Context) {
	for {
		k, ok := a.next()
		if !ok {
			a.save()
			select {
			case <-ctx.Done():
				return
			case <-a.wake:
			}
			continue
		}
		if !a.c.waitIdle(ctx) {
			return
		}
		e, ok := a.c.catalog.entry(k)
		if !ok {
			continue
		}
		m, err := a.c.meta.client.Fetch(ctx, meta.Request{Title: e.Title, SteamAppID: e.SteamAppID})
		if errors.Is(err, meta.ErrRateLimited) {
			logx.Printf("store art: rate limited, pausing a minute")
			a.get([]string{k})
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Minute):
			}
			continue
		}
		if ctx.Err() != nil {
			return
		}
		if err != nil || (m.Cover == "" && m.Hero == "" && m.Description == "") {
			m = nil
		}
		a.mu.Lock()
		a.known[k], a.dirty = m, true
		if m == nil {
			a.tried[k] = time.Now().Unix()
		}
		a.mu.Unlock()
		if m != nil {
			a.c.emit(EventStoreArt, StoreArt{Key: k, Meta: m})
		}
	}
}

func (a *artState) save() {
	a.mu.Lock()
	if !a.dirty {
		a.mu.Unlock()
		return
	}
	b, err := json.Marshal(artFile{Known: a.known, Tried: a.tried})
	a.dirty = false
	a.mu.Unlock()
	if err == nil {
		err = writeFileAtomic(a.path, b)
	}
	if err != nil {
		logx.Printf("store art: %v", err)
	}
}

// keep lists the stored images the store still uses, so pruning the
// library's unused art leaves them alone.
func (a *artState) keep(into map[string]bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, m := range a.known {
		if m != nil {
			for _, u := range []string{m.Cover, m.Hero, m.Backdrop, m.Tile, m.Logo, m.Icon} {
				if u != "" {
					into[u] = true
				}
			}
		}
	}
}

// forget drops games no longer in the catalog.
func (a *artState) forget(keep map[string]bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k := range a.known {
		if !keep[k] {
			delete(a.known, k)
			delete(a.tried, k)
			a.dirty = true
		}
	}
}

func writeFileAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
