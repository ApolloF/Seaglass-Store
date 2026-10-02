package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Cache kinds. Each is one JSON file under Options.Dir.
const (
	kindPopularity = "popularity"
	kindSummary    = "summaries"
	kindReviews    = "reviews"
	kindCritic     = "critics"
	kindCompletion = "completions"
	kindSearch     = "hltb-search"
)

// maxEntries bounds each file; the oldest entries go first.
var maxEntries = map[string]int{
	kindPopularity: 1,
	kindSummary:    2000,
	kindReviews:    150,
	kindCritic:     2000,
	kindCompletion: 1000,
	kindSearch:     100,
}

const (
	cacheVersion = 1
	maxCacheFile = 32 << 20
	// A kind is written at most this often; Flush forces it.
	writeEvery = 2 * time.Second
)

type cacheEntry struct {
	At int64           `json:"at"` // unix seconds
	V  json.RawMessage `json:"v"`
}

type cacheFile struct {
	Version int                   `json:"version"`
	Entries map[string]cacheEntry `json:"entries"`
}

// load reads every kind's file. The disk cache is disposable: a missing,
// unreadable or corrupt file is skipped, never fatal.
func (c *Client) load() {
	if c.opts.Dir == "" {
		return
	}
	for kind := range maxEntries {
		f, err := os.Open(filepath.Join(c.opts.Dir, kind+".json"))
		if err != nil {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(f, maxCacheFile+1))
		f.Close()
		if err != nil || len(b) > maxCacheFile {
			continue
		}
		var cf cacheFile
		if json.Unmarshal(b, &cf) != nil || cf.Version != cacheVersion {
			continue
		}
		if cf.Entries != nil {
			c.entries[kind] = cf.Entries
		}
	}
}

// cacheGet decodes a cached value and when it was fetched.
func cacheGet[T any](c *Client, kind, key string) (v T, at time.Time, ok bool) {
	c.cmu.Lock()
	e, found := c.entries[kind][key]
	c.cmu.Unlock()
	if !found {
		return v, at, false
	}
	if json.Unmarshal(e.V, &v) != nil {
		var zero T
		return zero, at, false
	}
	return v, time.Unix(e.At, 0), true
}

func (c *Client) cachePut(kind, key string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.cmu.Lock()
	m := c.entries[kind]
	if m == nil {
		m = map[string]cacheEntry{}
		c.entries[kind] = m
	}
	m[key] = cacheEntry{At: c.now().Unix(), V: b}
	if limit := maxEntries[kind]; len(m) > limit {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return m[keys[i]].At < m[keys[j]].At })
		for _, k := range keys[:len(m)-limit] {
			delete(m, k)
		}
	}
	c.dirty[kind] = true
	c.cmu.Unlock()
	c.persist(kind, false)
}

// persist writes a dirty kind. Without force it holds back when it wrote
// a moment ago; the next write or Flush catches up.
func (c *Client) persist(kind string, force bool) error {
	if c.opts.Dir == "" {
		return nil
	}
	c.cmu.Lock()
	recent := !c.written[kind].IsZero() && c.now().Sub(c.written[kind]) < writeEvery
	if !c.dirty[kind] || (recent && !force) {
		c.cmu.Unlock()
		return nil
	}
	b, err := json.Marshal(cacheFile{Version: cacheVersion, Entries: c.entries[kind]})
	if err != nil {
		c.cmu.Unlock()
		return err
	}
	c.dirty[kind] = false
	c.written[kind] = c.now()
	c.cmu.Unlock()

	c.wmu.Lock()
	defer c.wmu.Unlock()
	if err := writeAtomic(filepath.Join(c.opts.Dir, kind+".json"), b); err != nil {
		c.cmu.Lock()
		c.dirty[kind] = true
		c.cmu.Unlock()
		return err
	}
	return nil
}

// Flush writes anything not yet on disk.
func (c *Client) Flush() error {
	var first error
	for kind := range maxEntries {
		if err := c.persist(kind, true); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// writeAtomic writes to a temp file and renames it into place, so a crash
// never leaves a half-written cache.
func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// flight is one fetch that several callers wait for.
type flight struct {
	done    chan struct{}
	val     any
	err     error
	waiters int
	cancel  context.CancelFunc
}

// share runs fn once for all concurrent callers with the same key. The
// fetch runs on its own context so one caller giving up doesn't fail the
// others; when the last caller gives up, the fetch is cancelled.
func (c *Client) share(ctx context.Context, key string, fn func(context.Context) (any, error)) (any, error) {
	c.fmu.Lock()
	f := c.flights[key]
	if f != nil && f.waiters > 0 {
		f.waiters++
	} else {
		fctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		f = &flight{done: make(chan struct{}), waiters: 1, cancel: cancel}
		c.flights[key] = f
		go func() {
			defer cancel()
			f.val, f.err = fn(fctx)
			c.fmu.Lock()
			if c.flights[key] == f {
				delete(c.flights, key)
			}
			c.fmu.Unlock()
			close(f.done)
		}()
	}
	c.fmu.Unlock()
	select {
	case <-f.done:
		return f.val, f.err
	case <-ctx.Done():
		c.fmu.Lock()
		f.waiters--
		if f.waiters == 0 {
			f.cancel()
		}
		c.fmu.Unlock()
		return nil, ctx.Err()
	}
}

// How a lookup ended.
type outcome int

const (
	fromCache  outcome = iota // fresh in the cache, no request made
	fetched                   // just fetched
	staleCache                // the fetch failed; this is the old answer
	failed                    // the fetch failed and nothing is cached
)

// resolve answers from the cache while it is fresh, otherwise fetches once
// for all callers; if that fails it falls back to the old cached answer.
func resolve[T any](c *Client, ctx context.Context, kind, key string, ttl time.Duration, fetch func(context.Context) (T, error)) (T, outcome, error) {
	old, at, have := cacheGet[T](c, kind, key)
	if have && c.now().Sub(at) < ttl {
		return old, fromCache, nil
	}
	v, err := c.share(ctx, kind+"\x00"+key, func(ctx context.Context) (any, error) {
		v, err := fetch(ctx)
		if err != nil {
			return nil, err
		}
		c.cachePut(kind, key, v)
		return v, nil
	})
	if err != nil {
		if have {
			return old, staleCache, err
		}
		var zero T
		return zero, failed, err
	}
	return v.(T), fetched, nil
}

// failState is the State for an answer with nothing to show: an error is
// something that went wrong, unavailable is something that can't be had.
func failState(err error) string {
	var back *errBackoff
	var st *statusError
	switch {
	case errors.As(err, &back), errors.Is(err, errNotFound):
		return StateUnavailable
	case errors.As(err, &st) && st.Status == 429:
		return StateUnavailable
	}
	return StateError
}
