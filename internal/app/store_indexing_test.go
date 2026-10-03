package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/store/discovery"
	"github.com/ApolloF/Seaglass/internal/store/sources"
)

// siteFake is a DODI-like site with n articles, ten to a listing page.
type siteFake struct {
	mu    sync.Mutex
	n     int
	hits  map[string]int
	order []string
	fail  map[string]error
	// before runs ahead of each request, as a person or a game would.
	before func(raw string)
}

func newSiteFake(n int) *siteFake {
	return &siteFake{n: n, hits: map[string]int{}, fail: map[string]error{}}
}

func (f *siteFake) FetchDocument(ctx context.Context, raw string) (sources.Document, error) {
	if f.before != nil {
		f.before(raw)
	}
	if err := ctx.Err(); err != nil {
		return sources.Document{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hits[raw]++
	f.order = append(f.order, raw)
	if err := f.fail[raw]; err != nil {
		delete(f.fail, raw)
		return sources.Document{}, err
	}
	page := 1
	if rest, ok := strings.CutPrefix(raw, "https://dodi-repacks.site/page/"); ok {
		fmt.Sscanf(strings.TrimSuffix(rest, "/"), "%d", &page)
	} else if raw != "https://dodi-repacks.site/" {
		return sources.Document{}, &sources.HTTPError{Status: 404}
	}
	from := (page - 1) * 10
	if from >= f.n {
		return sources.Document{}, &sources.HTTPError{Status: 404}
	}
	var b strings.Builder
	b.WriteString("<html><body>")
	body := `<p>Torrent: <a href="https://unknown-host.example/abc">Mirror</a></p>`
	for i := from; i < min(from+10, f.n); i++ {
		b.WriteString(article("dodi-repacks.site", fmt.Sprintf("game-%03d", i), fmt.Sprintf("Game %03d", i), "v1.0", 24*(i+1), body))
	}
	if from+10 < f.n {
		fmt.Fprintf(&b, `<a class="next page-numbers" href="https://dodi-repacks.site/page/%d/">Next</a>`, page+1)
	}
	b.WriteString("</body></html>")
	return sources.Document{URL: raw, Body: []byte(b.String()), ContentType: "text/html"}, nil
}

func (f *siteFake) count(raw string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[raw]
}

func (f *siteFake) requests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.order)
}

func dodiPage(n int) string {
	if n == 1 {
		return "https://dodi-repacks.site/"
	}
	return fmt.Sprintf("https://dodi-repacks.site/page/%d/", n)
}

// indexingCore indexes DODI only, from site, with no breather between
// batches.
func indexingCore(t *testing.T, site *siteFake) (*Core, *StoreService) {
	c := testStoreCore(t)
	if _, err := c.updateSettings(func(v *settings.Settings) { v.Store.Sources = []string{"dodi"} }); err != nil {
		t.Fatal(err)
	}
	c.discovery.breather = 0
	c.discovery.fetchers["dodi"] = site
	return c, NewStoreService(c)
}

func TestIndexingRunsBatchAfterBatchToTheCatalogsEnd(t *testing.T) {
	site := newSiteFake(123) // 13 pages: more than two batches
	c, s := indexingCore(t, site)
	c.discovery.run("dodi")
	cr := c.discovery.index().Crawl("dodi")
	if !cr.BackfillDone || c.discovery.index().Counts()["dodi"] != 123 {
		t.Fatalf("after one run: %+v, %d releases", cr, c.discovery.index().Counts()["dodi"])
	}
	for n := 1; n <= 13; n++ {
		if got := site.count(dodiPage(n)); got != 1 {
			t.Errorf("page %d asked %d times", n, got)
		}
	}
	st := s.DiscoveryStatus()
	if src := st.Sources[sourceIndex(st, "dodi")]; src.State != discovery.CrawlIdle || !src.BackfillDone || src.Releases != 123 {
		t.Errorf("status after the end: %+v", src)
	}
}

func sourceIndex(st discovery.Status, id string) int {
	for i, s := range st.Sources {
		if s.ID == id {
			return i
		}
	}
	return -1
}

func TestAFiniteCatalogIsIndexedOnceByTheScheduler(t *testing.T) {
	site := newSiteFake(8)
	c, _ := indexingCore(t, site)
	c.discovery.lookup = func(id string) (sources.Provider, bool) {
		p, ok := sources.Lookup(id)
		if id == "dodi" {
			p.ListingPage = "" // one page, nothing past it
		}
		return p, ok
	}
	c.discovery.run("dodi")
	if !c.discovery.index().Crawl("dodi").BackfillDone {
		t.Fatal("a one-page catalog isn't complete after its page")
	}
	asked := site.requests()
	c.discovery.run("dodi")
	c.discovery.run("dodi")
	if site.requests() != asked || site.count(dodiPage(2)) != 0 {
		t.Errorf("later runs asked again: %v", site.order)
	}
}

func TestPausingStopsBeforeTheNextPageAndResumingGoesOn(t *testing.T) {
	site := newSiteFake(80)
	c, s := indexingCore(t, site)
	site.before = func(raw string) {
		if raw == dodiPage(3) {
			if _, err := s.PauseIndexing(true); err != nil {
				t.Error(err)
			}
		}
	}
	c.discovery.run("dodi")
	site.before = nil
	if site.count(dodiPage(4)) != 0 {
		t.Fatal("a page was asked after the pause")
	}
	st := s.DiscoveryStatus()
	if !st.Paused || st.Sources[sourceIndex(st, "dodi")].State != discovery.CrawlPaused {
		t.Errorf("status while paused: %+v", st)
	}
	before := site.requests()
	c.discovery.tick()
	c.discovery.run("dodi")
	if site.requests() != before {
		t.Error("indexing ran while paused")
	}
	// Game pages and searches still work while paused.
	if res, _ := s.BrowseGames(discovery.BrowseQuery{Text: "game 001"}); res.Page.Total != 1 {
		t.Errorf("browsing while paused: %+v", res.Page)
	} else if _, err := s.GameDetails(res.Page.Games[0].Key); err != nil {
		t.Errorf("a game page while paused: %v", err)
	}
	if _, err := s.RefreshDiscovery(); err == nil {
		t.Error("a refresh ran while paused")
	}

	if st, err := s.PauseIndexing(false); err != nil || st.Paused {
		t.Fatalf("resume: %+v %v", st, err)
	}
	c.discovery.run("dodi")
	if !c.discovery.index().Crawl("dodi").BackfillDone || c.discovery.index().Counts()["dodi"] != 80 {
		t.Fatalf("after resuming: %+v", c.discovery.index().Crawl("dodi"))
	}
	// Page 3's request was cancelled by the pause before it went out.
	for n := 1; n <= 8; n++ {
		if got := site.count(dodiPage(n)); got != 1 {
			t.Errorf("page %d asked %d times", n, got)
		}
	}
}

func TestResumingWakesTheScheduler(t *testing.T) {
	c, s := indexingCore(t, newSiteFake(5))
	if _, err := s.PauseIndexing(true); err != nil {
		t.Fatal(err)
	}
	for len(c.discovery.kick) > 0 {
		<-c.discovery.kick
	}
	if _, err := s.PauseIndexing(false); err != nil {
		t.Fatal(err)
	}
	if len(c.discovery.kick) != 1 {
		t.Error("resuming didn't start indexing at once")
	}
}

func TestPauseIndexingNeedsTheStore(t *testing.T) {
	c, s := indexingCore(t, newSiteFake(5))
	if _, err := c.updateSettings(func(v *settings.Settings) { v.ExperimentalStore = false }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PauseIndexing(true); err == nil || c.Settings.Get().Store.IndexingPaused {
		t.Error("indexing was paused with the Store off")
	}
}

func TestAGameStartingHoldsIndexingUntilItEnds(t *testing.T) {
	site := newSiteFake(80)
	c, s := indexingCore(t, site)
	var mu sync.Mutex
	playing := false
	c.discovery.isPlaying = func() bool {
		mu.Lock()
		defer mu.Unlock()
		return playing
	}
	site.before = func(raw string) {
		if raw == dodiPage(3) {
			mu.Lock()
			playing = true
			mu.Unlock()
		}
	}
	c.discovery.run("dodi")
	site.before = nil
	if site.count(dodiPage(4)) != 0 {
		t.Fatal("indexing went on while a game ran")
	}
	st := s.DiscoveryStatus()
	if !st.Playing || st.Paused || st.Sources[sourceIndex(st, "dodi")].State != discovery.CrawlPaused {
		t.Errorf("status while playing: %+v", st)
	}
	mu.Lock()
	playing = false
	mu.Unlock()
	c.discovery.run("dodi")
	if !c.discovery.index().Crawl("dodi").BackfillDone {
		t.Error("indexing didn't go on after the game")
	}
}

func TestTurningTheSourceOffStopsItsBatch(t *testing.T) {
	site := newSiteFake(80)
	c, _ := indexingCore(t, site)
	site.before = func(raw string) {
		if raw == dodiPage(2) {
			if _, err := c.updateSettings(func(v *settings.Settings) { v.Store.Sources = []string{"fitgirl"} }); err != nil {
				t.Error(err)
			}
		}
	}
	c.discovery.run("dodi")
	if site.count(dodiPage(3)) != 0 {
		t.Errorf("DODI was asked after it was turned off: %v", site.order)
	}
}

func TestRetryAfterDefersTheNextRequestAndKeepsTheIndex(t *testing.T) {
	site := newSiteFake(40)
	c, s := indexingCore(t, site)
	site.fail[dodiPage(3)] = &sources.HTTPError{Status: 429, RetryAfter: "600"}
	c.discovery.run("dodi")
	cr := c.discovery.index().Crawl("dodi")
	if !time.Now().Before(cr.RetryAt) || cr.Error == "" {
		t.Fatalf("after a 429: %+v", cr)
	}
	if n := c.discovery.index().Counts()["dodi"]; n != 20 {
		t.Errorf("%d releases kept, want the first two pages' 20", n)
	}
	st := s.DiscoveryStatus()
	if src := st.Sources[sourceIndex(st, "dodi")]; src.State != discovery.CrawlBackoff || src.RetryAt == 0 || src.Error == "" {
		t.Errorf("status while backing off: %+v", src)
	}
	asked := site.requests()
	c.discovery.run("dodi")
	if site.requests() != asked {
		t.Error("a request went out before Retry-After")
	}
}

func TestRemoteSearchSkipsProvidersWithoutSiteSearch(t *testing.T) {
	site := newSiteFake(5)
	c, _ := indexingCore(t, site)
	c.discovery.lookup = func(id string) (sources.Provider, bool) {
		p, ok := sources.Lookup(id)
		p.Search = ""
		return p, ok
	}
	progress, _, _ := c.discovery.searchRemote("lantern")
	if progress[0].ID != "dodi" || progress[0].State != discovery.StateSkipped {
		t.Errorf("progress: %+v", progress)
	}
	if site.requests() != 0 {
		t.Errorf("a provider without site search was asked: %v", site.order)
	}
}
