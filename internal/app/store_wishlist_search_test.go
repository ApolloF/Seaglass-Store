package app

import (
	"context"
	"errors"
	"slices"
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
	if key, _ := w.queue().Next(now.Add(wishlist.SearchSpacing), nil); key != "steam:3100100" || w.queue().Len() != 2 {
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
	if key, _ := w.queue().Next(now, nil); key != "steam:4200000" {
		t.Fatalf("first in line: %q", key)
	}
	if wait := w.searchNext(now); wait != wishlist.SearchSpacing {
		t.Errorf("waits %v", wait)
	}
	if key, _ := w.queue().Next(now.Add(wishlist.SearchSpacing), nil); key != "steam:3100100" || w.queue().Len() != 1 {
		t.Errorf("after the unreadable answer: next %q, %d queued", key, w.queue().Len())
	}
}

// FitGirl can't be reached for days: each game is asked of DODI once, of
// FitGirl a few times, and the queue empties.
func TestASourceThatStaysDownNeitherKeepsTheQueueNorRepeatsTheOthers(t *testing.T) {
	c, _, dodi := searchCore(t)
	fg, _ := sources.Lookup("fitgirl")
	fail := map[string]error{}
	for _, title := range []string{"Lantern Season", "Hollow Tide II"} {
		u, _ := fg.SearchURL(title)
		fail[u] = errors.New("connection reset by peer")
	}
	f := &failingFetcher{next: c.discovery.fetchers["fitgirl"], fail: fail}
	c.discovery.fetchers["fitgirl"] = f
	w := c.wishlist
	now := time.Now()
	// Turns half an hour apart outlive the hour searches are cached for.
	for i := range 20 {
		w.searchNext(now.Add(time.Duration(i) * 30 * time.Minute))
	}
	if n := w.queue().Len(); n != 0 {
		t.Fatalf("%d games still queued", n)
	}
	dp, _ := sources.Lookup("dodi")
	for _, title := range []string{"Lantern Season", "Hollow Tide II"} {
		u, _ := dp.SearchURL(title)
		if n := slices.Index(dodi.asked, u); n < 0 || slices.Contains(dodi.asked[n+1:], u) {
			t.Errorf("DODI asked for %s other than once: %v", title, dodi.asked)
		}
		fu, _ := fg.SearchURL(title)
		if n := len(slices.DeleteFunc(slices.Clone(f.asked), func(s string) bool { return s != fu })); n == 0 || n > wishlist.SearchMisses {
			t.Errorf("FitGirl asked %d times for %s", n, title)
		}
	}
}
