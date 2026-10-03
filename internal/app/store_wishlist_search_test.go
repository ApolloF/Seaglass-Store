package app

import (
	"context"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/store/discovery"
	"github.com/ApolloF/Seaglass/internal/store/sources"
	"github.com/ApolloF/Seaglass/internal/store/wishlist"
)

// failingFetcher answers some URLs with an error, the rest as next does.
type failingFetcher struct {
	next  discovery.Fetcher
	fail  map[string]error
	asked []string
}

func (f *failingFetcher) FetchDocument(ctx context.Context, raw string) (sources.Document, error) {
	f.asked = append(f.asked, raw)
	if err := f.fail[raw]; err != nil {
		return sources.Document{}, err
	}
	return f.next.FetchDocument(ctx, raw)
}

func TestARateLimitedIdleSearchDefersThatSourceUntilRetryAfter(t *testing.T) {
	c, s, dodi := searchCore(t)
	p, _ := sources.Lookup("dodi")
	lantern, _ := p.SearchURL("Lantern Season")
	f := &failingFetcher{next: dodi, fail: map[string]error{lantern: &sources.HTTPError{Status: 429, RetryAfter: "3600"}}}
	c.discovery.fetchers["dodi"] = f
	w := c.wishlist
	now := time.Now()
	if wait := w.searchNext(now); wait != wishlist.SearchSpacing {
		t.Errorf("waits %v after a search", wait)
	}
	if at := c.discovery.index().Crawl("dodi").RetryAt; !at.Equal(now.Add(time.Hour)) {
		t.Fatalf("DODI waits until %v, want an hour", at)
	}
	// Lantern Season wasn't answered by DODI: still queued, now behind
	// the other game.
	if key, _ := w.queue().Next(now.Add(wishlist.SearchSpacing)); key != "steam:3100100" || w.queue().Len() != 2 {
		t.Fatalf("next %q, %d queued", key, w.queue().Len())
	}
	asked := len(f.asked)
	for i := 1; i <= 3; i++ {
		w.searchNext(now.Add(time.Duration(i) * wishlist.SearchSpacing))
	}
	if res, _, _ := c.discovery.searchRemote("Lantern Season"); res[1].ID != "dodi" || res[1].State != discovery.StateError {
		t.Errorf("the person's search: %+v", res)
	}
	if len(f.asked) != asked {
		t.Fatalf("DODI was asked before Retry-After: %v", f.asked[asked:])
	}
	if w.queue().Len() != 2 {
		t.Errorf("games searched while DODI waits were taken off the queue: %d queued", w.queue().Len())
	}

	delete(f.fail, lantern)
	later := now.Add(time.Hour + time.Second)
	w.searchNext(later)
	w.searchNext(later.Add(wishlist.SearchSpacing))
	if len(f.asked) == asked {
		t.Fatal("DODI wasn't asked again after Retry-After")
	}
	if items := s.Wishlist(); !items[0].Game.SourceBacked || w.queue().Len() != 0 {
		t.Errorf("after the wait: %+v, %d queued", items[0], w.queue().Len())
	}
}

// Steam answers null for a delisted app: that game waits like a searched
// one instead of being asked about first, again and again.
func TestAGameSteamCannotNameDoesNotBlockTheQueue(t *testing.T) {
	net := &steamNet{answers: map[string]string{
		"GetWishlist":    wishlistAnswer("4200000", "3100100"),
		"appids=4200000": `null`,
		"appids=3100100": `{"3100100":{"success":true,"data":{"type":"game","name":"Hollow Tide II"}}}`,
	}}
	c, s := wishlistCore(t, net)
	if res, err := s.ImportSteamWishlist(testSteamID); err != nil || res.Searching != 2 {
		t.Fatalf("import: %+v %v", res, err)
	}
	w := c.wishlist
	now := time.Now()
	if key, _ := w.queue().Next(now); key != "steam:4200000" {
		t.Fatalf("first in line: %q", key)
	}
	if wait := w.searchNext(now); wait != wishlist.SearchSpacing {
		t.Errorf("waits %v", wait)
	}
	if key, _ := w.queue().Next(now.Add(wishlist.SearchSpacing)); key != "steam:3100100" || w.queue().Len() != 1 {
		t.Errorf("after the unreadable answer: next %q, %d queued", key, w.queue().Len())
	}
}
