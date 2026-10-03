package discovery

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/store/sources"
)

const testMagnetHash = "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c"

// fakeSource serves a WordPress-like source: an RSS feed of the newest
// articles and listing pages of ten articles each, newest first.
type fakeSource struct {
	mu       sync.Mutex
	src      sources.Provider
	articles []fakeArticle // newest first
	fail     map[string]error
	hits     map[string]int
	order    []string
	// before runs ahead of each request, as a crash or a person would.
	before func(raw string)
}

type fakeArticle struct {
	slug, title, version string
	published            time.Time
	magnet               bool
}

func newFakeSource(t *testing.T, id string, n int) *fakeSource {
	src, ok := sources.Lookup(id)
	if !ok {
		t.Fatal("unknown source", id)
	}
	f := &fakeSource{src: src, fail: map[string]error{}, hits: map[string]int{}}
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		f.articles = append(f.articles, fakeArticle{slug: fmt.Sprintf("game-%03d", i), title: fmt.Sprintf("Game %03d", i), version: "v1.0", published: base.Add(-time.Duration(i) * 24 * time.Hour), magnet: id == "fitgirl"})
	}
	return f
}

// publish puts a new article on top, as a source does.
func (f *fakeSource) publish(a fakeArticle) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.articles = append([]fakeArticle{a}, f.articles...)
}

func (a fakeArticle) html(host string) string {
	body := `<p>Repack Size: 2.1 GB</p><p>Languages: English, German</p>`
	if a.magnet {
		body += `<a href="magnet:?xt=urn:btih:` + testMagnetHash + `&dn=` + a.slug + `">magnet</a>`
	} else {
		body += `<p>Torrent: <a href="https://file-me.top/abcdefabcdef.html">File-Me</a></p>`
	}
	return `<article class="category-lossless-repack"><h1 class="entry-title"><a href="https://` + host + `/` + a.slug + `/">` + a.title + ` – ` + a.version + `</a></h1>` +
		`<time class="published" datetime="` + a.published.Format(time.RFC3339) + `"></time><div class="entry-content">` + body + `</div></article>`
}

func (f *fakeSource) FetchDocument(ctx context.Context, raw string) (sources.Document, error) {
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
	host := f.src.Host
	if raw == f.src.StartURL && strings.Contains(raw, "/feed/") {
		var b strings.Builder
		b.WriteString(`<?xml version="1.0"?><rss xmlns:content="http://purl.org/rss/1.0/modules/content/"><channel>`)
		for _, a := range f.articles[:min(3, len(f.articles))] {
			fmt.Fprintf(&b, `<item><title>%s – %s</title><link>https://%s/%s/</link><pubDate>%s</pubDate><content:encoded><![CDATA[%s]]></content:encoded></item>`,
				a.title, a.version, host, a.slug, a.published.Format(time.RFC1123Z), strings.TrimSuffix(strings.TrimPrefix(a.html(host), ""), ""))
		}
		b.WriteString(`</channel></rss>`)
		return sources.Document{URL: raw, Body: []byte(b.String()), ContentType: "application/rss+xml"}, nil
	}
	for _, a := range f.articles {
		if raw == "https://"+host+"/"+a.slug+"/" {
			return sources.Document{URL: raw, Body: []byte("<html><body>" + a.html(host) + "</body></html>"), ContentType: "text/html"}, nil
		}
	}
	page := 1
	if rest, ok := strings.CutPrefix(raw, "https://"+host+"/page/"); ok {
		fmt.Sscanf(strings.TrimSuffix(rest, "/"), "%d", &page)
	} else if raw != "https://"+host+"/" {
		return sources.Document{}, &sources.HTTPError{Status: 404}
	}
	from := (page - 1) * 10
	if from >= len(f.articles) {
		return sources.Document{}, &sources.HTTPError{Status: 404}
	}
	var b strings.Builder
	b.WriteString("<html><body>")
	for _, a := range f.articles[from:min(from+10, len(f.articles))] {
		b.WriteString(a.html(host))
	}
	if from+10 < len(f.articles) {
		fmt.Fprintf(&b, `<a class="next page-numbers" href="https://%s/page/%d/">Next</a>`, host, page+1)
	}
	b.WriteString("</body></html>")
	return sources.Document{URL: raw, Body: []byte(b.String()), ContentType: "text/html"}, nil
}

func (f *fakeSource) count(raw string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[raw]
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }
func testIndex(t *testing.T) (*Index, string) {
	dir := t.TempDir()
	return OpenIndex(filepath.Join(dir, "discovery"), filepath.Join(dir, "store-identity.json")), dir
}

func TestFirstPassReadsNewestListingsThenOlderPages(t *testing.T) {
	ix, _ := testIndex(t)
	f := newFakeSource(t, "fitgirl", 95)
	clk := &clock{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	res := Pass(context.Background(), ix, f.src, f, true, clk.now, PassHooks{})
	if res.Err != nil || !res.Recent {
		t.Fatalf("first pass: %+v", res)
	}
	if res.Pages != PagesPerPass {
		t.Errorf("listing pages in one pass: %d, want %d", res.Pages, PagesPerPass)
	}
	if f.count(f.src.StartURL) != 1 {
		t.Error("the feed wasn't read for the newest releases")
	}
	c := ix.Crawl("fitgirl")
	if c.NextPage != 6 || c.BackfillDone || c.RecentAt.IsZero() {
		t.Errorf("crawl state after the first pass: %+v", c)
	}
	if n := ix.Counts()["fitgirl"]; n != 50 {
		t.Errorf("records: %d, want 50 (pages 1-5)", n)
	}
	newest, _ := ix.Record("fitgirl", sources.EntryID("fitgirl", "https://fitgirl-repacks.site/game-000/"))
	older, _ := ix.Record("fitgirl", sources.EntryID("fitgirl", "https://fitgirl-repacks.site/game-030/"))
	if newest.Backfill || !older.Backfill {
		t.Errorf("only older pages are backfill: newest %v, older %v", newest.Backfill, older.Backfill)
	}
	// Older pages are due again at once; the newest listings wait six hours.
	if !res.More {
		t.Error("the batch ended without saying older pages remain")
	}
	if recent, backfill := Due(ix.Crawl("fitgirl"), clk.now(), false); recent || !backfill {
		t.Errorf("right after the batch: recent %v backfill %v", recent, backfill)
	}
}

func TestBackfillResumesAfterRestartAndStopsAtConfirmedEnd(t *testing.T) {
	ix, dir := testIndex(t)
	f := newFakeSource(t, "dodi", 95)
	clk := &clock{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	Pass(context.Background(), ix, f.src, f, true, clk.now, PassHooks{})
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}
	ix = OpenIndex(filepath.Join(dir, "discovery"), filepath.Join(dir, "store-identity.json"))
	if c := ix.Crawl("dodi"); c.NextPage != 6 || ix.Counts()["dodi"] != 50 {
		t.Fatalf("after a restart: %+v, %d records", c, ix.Counts()["dodi"])
	}
	for i := 0; i < 5 && !ix.Crawl("dodi").BackfillDone; i++ {
		if res := Pass(context.Background(), ix, f.src, f, false, clk.now, PassHooks{}); res.Err != nil {
			t.Fatal(res.Err)
		}
	}
	c := ix.Crawl("dodi")
	if !c.BackfillDone || ix.Counts()["dodi"] != 95 {
		t.Fatalf("end not confirmed: %+v, %d records", c, ix.Counts()["dodi"])
	}
	if f.count("https://dodi-repacks.site/page/6/") != 1 {
		t.Error("page 6 was fetched again after a restart")
	}
	if _, backfill := Due(ix.Crawl("dodi"), clk.now(), false); backfill {
		t.Error("backfill continues past the confirmed end")
	}
}

func TestInterruptedPageIsRetriedAfterBackoff(t *testing.T) {
	ix, _ := testIndex(t)
	f := newFakeSource(t, "dodi", 95)
	clk := &clock{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	f.fail["https://dodi-repacks.site/page/4/"] = errors.New("connection reset")
	res := Pass(context.Background(), ix, f.src, f, true, clk.now, PassHooks{})
	if res.Err == nil {
		t.Fatal("the failure wasn't reported")
	}
	c := ix.Crawl("dodi")
	if c.NextPage != 4 || c.Failures != 1 || !c.RetryAt.Equal(clk.now().Add(time.Minute)) {
		t.Fatalf("after a failed page: %+v", c)
	}
	if n := ix.Counts()["dodi"]; n != 30 {
		t.Errorf("records before the failure were lost or doubled: %d", n)
	}
	clk.add(30 * time.Second)
	if recent, backfill := Due(ix.Crawl("dodi"), clk.now(), true); recent || backfill {
		t.Error("a refresh ran during backoff")
	}
	clk.add(time.Minute)
	if res := Pass(context.Background(), ix, f.src, f, false, clk.now, PassHooks{}); res.Err != nil {
		t.Fatal(res.Err)
	}
	if f.count("https://dodi-repacks.site/page/4/") != 2 || ix.Crawl("dodi").NextPage != 9 || ix.Crawl("dodi").Failures != 0 {
		t.Errorf("page 4 wasn't retried: %+v", ix.Crawl("dodi"))
	}
}

func TestRetryAfterAndBackoffAreHonoured(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	c := CrawlState{}
	Backoff(&c, &sources.HTTPError{Status: 429, RetryAfter: "120"}, now)
	if !c.RetryAt.Equal(now.Add(2*time.Minute)) || !strings.Contains(c.Error, "slow down") {
		t.Errorf("Retry-After seconds: %+v", c)
	}
	Backoff(&c, &sources.HTTPError{Status: 503, RetryAfter: now.Add(10 * time.Minute).Format(http.TimeFormat)}, now)
	if got := c.RetryAt.Sub(now); got < 9*time.Minute || got > 10*time.Minute {
		t.Errorf("Retry-After date: %v", got)
	}
	Backoff(&c, &sources.HTTPError{Status: 429, RetryAfter: "999999"}, now)
	if c.RetryAt.Sub(now) != retryMax {
		t.Errorf("Retry-After isn't capped: %v", c.RetryAt.Sub(now))
	}
	c = CrawlState{}
	var waits []time.Duration
	for i := 0; i < 9; i++ {
		Backoff(&c, errors.New("timeout"), now)
		waits = append(waits, c.RetryAt.Sub(now))
	}
	if waits[0] != time.Minute || waits[1] != 2*time.Minute || waits[2] != 4*time.Minute || waits[8] != backoffMax {
		t.Errorf("exponential backoff: %v", waits)
	}
}

func TestRecentRefreshCatchesUpWithoutDuplicates(t *testing.T) {
	ix, _ := testIndex(t)
	f := newFakeSource(t, "fitgirl", 40)
	clk := &clock{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	Pass(context.Background(), ix, f.src, f, true, clk.now, PassHooks{})
	before := ix.Counts()["fitgirl"]
	// Twelve new articles while Seaglass was closed: two pages' worth.
	for i := 0; i < 12; i++ {
		f.publish(fakeArticle{slug: fmt.Sprintf("new-%02d", i), title: fmt.Sprintf("New %02d", i), version: "v1", published: clk.now().Add(time.Duration(i) * time.Hour), magnet: true})
	}
	clk.add(RecentEvery)
	res := Pass(context.Background(), ix, f.src, f, false, clk.now, PassHooks{})
	if res.Err != nil || !res.Recent {
		t.Fatalf("refresh: %+v", res)
	}
	if got := ix.Counts()["fitgirl"]; got != max(before, 40)+12 && got != 52 {
		t.Errorf("records after catching up: %d", got)
	}
	r, ok := ix.Record("fitgirl", sources.EntryID("fitgirl", "https://fitgirl-repacks.site/new-00/"))
	if !ok || r.Backfill {
		t.Errorf("an article published while away isn't new: %+v", r)
	}
}

func TestArticleUpdatesChangeTheRecordButSummariesDoNotDowngradeIt(t *testing.T) {
	ix, _ := testIndex(t)
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	src, _ := sources.PrivateSource("fitgirl", true)
	page := "https://fitgirl-repacks.site/ember-crown/"
	full := sources.Entry{ID: "x", SourceID: "fitgirl", PageURL: page + "?utm=1", Title: "Ember Crown", TitleKey: "embercrown", Version: "v1.0", SizeClaim: "2 GB", ReleaseKind: "release",
		Transports: []sources.Transport{{Kind: "magnet", URI: "magnet:?xt=urn:btih:" + testMagnetHash, InfoHash: testMagnetHash}}}
	ix.Merge(src.ID, []sources.Entry{full}, OriginListing, false, now)
	ix.Merge(src.ID, []sources.Entry{full}, OriginRSS, false, now.Add(time.Hour))
	if ix.Counts()["fitgirl"] != 1 {
		t.Fatal("the same article from the feed and the listing made two records")
	}
	id := sources.EntryID("fitgirl", page)
	r, _ := ix.Record("fitgirl", id)
	if !r.Changed.IsZero() {
		t.Error("seeing the same article again counts as a change")
	}
	summary := full
	summary.SummaryOnly, summary.Version, summary.Transports = true, "", nil
	ix.Merge(src.ID, []sources.Entry{summary}, OriginSearch, false, now.Add(2*time.Hour))
	if r, _ = ix.Record("fitgirl", id); r.Entry.SummaryOnly || r.Entry.Version != "v1.0" || len(r.Entry.Transports) != 1 {
		t.Errorf("a search summary replaced the article: %+v", r.Entry)
	}
	updated := full
	updated.Version = "v1.1"
	res := ix.Merge(src.ID, []sources.Entry{updated}, OriginListing, false, now.Add(3*time.Hour))
	if r, _ = ix.Record("fitgirl", id); res.Changed != 1 || r.Changed.IsZero() || r.Entry.Version != "v1.1" || !r.FirstSeen.Equal(now) {
		t.Errorf("an updated article: %+v %+v", res, r)
	}
}

// An announcement's page can turn into the release while its summary row
// stays the same: it is read again once a day while still listed.
func TestAnAnnouncementIsReadAgainADayLater(t *testing.T) {
	ix, _ := testIndex(t)
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	page := "https://elamigos.site/data/Upcoming.html"
	row := sources.Entry{SourceID: "elamigos", PageURL: page, RawTitle: "Upcoming ElAmigos", Title: "Upcoming", TitleKey: "upcoming", ReleaseKind: "release", SummaryOnly: true}
	ix.Merge("elamigos", []sources.Entry{row}, OriginListing, true, now)
	detail := row
	detail.SummaryOnly, detail.ReleaseKind, detail.RawTitle = false, "preview", "Upcoming (2027)"
	ix.Merge("elamigos", []sources.Entry{detail}, OriginDetailFetch, true, now)
	id := sources.EntryID("elamigos", page)
	ix.Merge("elamigos", []sources.Entry{row}, OriginListing, true, now.Add(6*time.Hour))
	if r, _ := ix.Record("elamigos", id); NeedsDetail(r) {
		t.Fatal("read again six hours later")
	}
	ix.Merge("elamigos", []sources.Entry{row}, OriginListing, true, now.Add(previewRecheck))
	if r, _ := ix.Record("elamigos", id); !NeedsDetail(r) || r.Entry.ReleaseKind != "preview" {
		t.Errorf("a day later: needs reading %v, %+v", NeedsDetail(r), r.Entry)
	}
}

func TestResolvedTorrentsSurviveARefresh(t *testing.T) {
	ix, _ := testIndex(t)
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	e := sources.Entry{SourceID: "dodi", PageURL: "https://dodi-repacks.site/x/", Title: "X", TitleKey: "x", ReleaseKind: "release",
		References: []sources.Reference{{URL: "https://file-me.top/abcdefabcdef.html", Kind: "torrent"}}}
	ix.Merge("dodi", []sources.Entry{e}, OriginListing, false, now)
	id := sources.EntryID("dodi", e.PageURL)
	resolved := sources.Transport{Kind: "magnet", URI: "magnet:?xt=urn:btih:" + testMagnetHash, InfoHash: testMagnetHash, MetadataSHA256: strings.Repeat("b", 64)}
	ix.Update("dodi", id, func(r *Record) {
		r.Entry.Transports = append(r.Entry.Transports, resolved)
		r.Entry.References[0].State = "resolved"
	})
	res := ix.Merge("dodi", []sources.Entry{e}, OriginListing, false, now.Add(time.Hour))
	r, _ := ix.Record("dodi", id)
	if len(r.Entry.Transports) != 1 || r.Entry.References[0].State != "resolved" || res.Changed != 0 {
		t.Errorf("re-reading the article lost the resolved torrent: %+v %+v", r.Entry, res)
	}
}

func TestCancellingOrPlayingStopsAPassWithoutCountingAFailure(t *testing.T) {
	ix, _ := testIndex(t)
	f := newFakeSource(t, "dodi", 95)
	clk := &clock{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if res := Pass(ctx, ix, f.src, f, true, clk.now, PassHooks{}); res.Stopped != StopCanceled || res.Err != nil {
		t.Errorf("cancelled: %+v", res)
	}
	playing := false
	calls := 0
	paused := func() bool { calls++; return playing }
	playing = true
	if res := Pass(context.Background(), ix, f.src, f, true, clk.now, PassHooks{Paused: paused}); res.Stopped != StopPaused || res.Pages != 0 {
		t.Errorf("playing: %+v", res)
	}
	if c := ix.Crawl("dodi"); c.Failures != 0 || !c.RetryAt.IsZero() || len(f.order) != 0 {
		t.Errorf("a stop counted as a failure or fetched anyway: %+v %v", c, f.order)
	}
}

func TestMissingArticleIsMarkedUnavailableNotDeleted(t *testing.T) {
	ix, _ := testIndex(t)
	f := newFakeSource(t, "fitgirl", 12)
	clk := &clock{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	Pass(context.Background(), ix, f.src, f, true, clk.now, PassHooks{})
	id := sources.EntryID("fitgirl", "https://fitgirl-repacks.site/game-005/")
	f.mu.Lock()
	f.articles = append(f.articles[:5], f.articles[6:]...)
	f.mu.Unlock()
	if err := FetchDetail(context.Background(), ix, f.src.Source, f, id, clk.now()); err != nil {
		t.Fatal(err)
	}
	r, ok := ix.Record("fitgirl", id)
	if !ok || !r.Gone || availability(r) != AvailGone {
		t.Errorf("a vanished article: %v %+v", ok, r)
	}
}

func TestDetailFetchCompletesASearchSummary(t *testing.T) {
	ix, _ := testIndex(t)
	f := newFakeSource(t, "fitgirl", 3)
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	summary := sources.Entry{SourceID: "fitgirl", PageURL: "https://fitgirl-repacks.site/game-001/", Title: "Game 001", TitleKey: "game001", ReleaseKind: "release", SummaryOnly: true}
	ix.Merge("fitgirl", []sources.Entry{summary}, OriginSearch, true, now)
	id := sources.EntryID("fitgirl", summary.PageURL)
	if r, _ := ix.Record("fitgirl", id); availability(r) != AvailSummary {
		t.Fatalf("a summary: %s", availability(r))
	}
	if err := FetchDetail(context.Background(), ix, f.src.Source, f, id, now); err != nil {
		t.Fatal(err)
	}
	r, _ := ix.Record("fitgirl", id)
	if r.Entry.SummaryOnly || r.Detailed.IsZero() || availability(r) != AvailInstallable || !r.Backfill {
		t.Errorf("after the detail fetch: %+v (%s)", r, availability(r))
	}
}

func TestFiniteCatalogIsCompleteAfterItsOnlyPage(t *testing.T) {
	ix, _ := testIndex(t)
	f := newFakeSource(t, "dodi", 8)
	f.src.ListingPage, f.src.Search = "", ""
	clk := &clock{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	res := Pass(context.Background(), ix, f.src, f, true, clk.now, PassHooks{})
	if res.Err != nil || !res.Recent || res.Pages != 1 {
		t.Fatalf("pass over a one-page catalog: %+v", res)
	}
	if c := ix.Crawl("dodi"); !c.BackfillDone {
		t.Fatalf("a one-page catalog left backfill open: %+v", c)
	}
	clk.add(time.Hour)
	if res := Pass(context.Background(), ix, f.src, f, false, clk.now, PassHooks{}); res.Pages != 0 {
		t.Fatalf("a finished one-page catalog was fetched again within six hours: %+v", res)
	}
	for _, raw := range f.order {
		if strings.Contains(raw, "/page/") {
			t.Fatalf("requested a page that cannot exist: %s", raw)
		}
	}
	if n := ix.Counts()["dodi"]; n != 8 {
		t.Fatalf("records %d, want 8", n)
	}
}

// A finite catalog's one page holds its whole history. Indexed for the
// first time, all of it is history, so turning the source on never creates
// wishlist news; later, only rows dated within the last week can be news.
func TestFiniteCatalogKeepsOldAndUndatedRowsAsHistory(t *testing.T) {
	ix, _ := testIndex(t)
	f := newFakeSource(t, "dodi", 10)
	f.src.ListingPage, f.src.Search = "", ""
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	clk := &clock{now}
	res := Pass(context.Background(), ix, f.src, f, true, clk.now, PassHooks{})
	if res.Err != nil || len(res.Merged.IDs) != 10 {
		t.Fatalf("first pass: %+v", res)
	}
	for _, id := range res.Merged.IDs {
		if r, _ := ix.Record("dodi", id); !r.Backfill {
			t.Errorf("first pass: %s published %s is news", r.Entry.Title, r.Entry.PublishedAt)
		}
	}

	clk.add(RecentEvery)
	f.publish(fakeArticle{slug: "fresh", title: "Fresh", version: "v1.0", published: clk.now().Add(-48 * time.Hour)})
	f.publish(fakeArticle{slug: "old-addition", title: "Old Addition", version: "v1.0", published: clk.now().Add(-30 * 24 * time.Hour)})
	res = Pass(context.Background(), ix, f.src, f, false, clk.now, PassHooks{})
	if res.Err != nil || res.Merged.Added != 2 {
		t.Fatalf("later refresh: %+v", res)
	}
	fresh, _ := ix.Record("dodi", sources.EntryID("dodi", "https://dodi-repacks.site/fresh/"))
	old, _ := ix.Record("dodi", sources.EntryID("dodi", "https://dodi-repacks.site/old-addition/"))
	if fresh.Backfill || !old.Backfill {
		t.Errorf("later refresh: a row from two days ago is history %v, one from a month ago %v", fresh.Backfill, old.Backfill)
	}

	week := now.Add(-48 * time.Hour)
	undated := sources.Entry{Title: "Undated"}
	dated := sources.Entry{Title: "Updated", UpdatedAt: &week}
	if got := catalogHistory([]sources.Entry{undated, dated}, now, true); len(got) != 1 || got[0].Title != "Undated" {
		t.Errorf("history %+v, want the undated row only", got)
	}
}

// relistedRecord indexes a read article whose summary row then changed:
// it waits to be read again.
func relistedRecord(t *testing.T, ix *Index, f *fakeSource, now time.Time) string {
	t.Helper()
	page := "https://dodi-repacks.site/game-001/"
	row := sources.Entry{SourceID: "dodi", PageURL: page, RawTitle: "Game 001 – v1.0", Title: "Game 001", TitleKey: "game001", ReleaseKind: "release", SummaryOnly: true}
	ix.Merge("dodi", []sources.Entry{row}, OriginListing, false, now)
	id := sources.EntryID("dodi", page)
	if err := FetchDetail(context.Background(), ix, f.src.Source, f, id, now); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.articles[1].version = "v1.1"
	f.mu.Unlock()
	row.RawTitle = "Game 001 – v1.1"
	if res := ix.Merge("dodi", []sources.Entry{row}, OriginListing, false, now.Add(time.Hour)); res.Relisted != 1 {
		t.Fatalf("not relisted: %+v", res)
	}
	return id
}

func TestARelistedReleaseWhoseRereadHitsAServerErrorIsReadAgainLater(t *testing.T) {
	ix, _ := testIndex(t)
	f := newFakeSource(t, "dodi", 3)
	clk := &clock{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	id := relistedRecord(t, ix, f, clk.now())
	page := "https://dodi-repacks.site/game-001/"
	clk.add(2 * time.Hour)
	f.fail[page] = &sources.HTTPError{Status: http.StatusServiceUnavailable}
	if err := FetchDetail(context.Background(), ix, f.src.Source, f, id, clk.now()); err == nil {
		t.Fatal("the 503 wasn't reported")
	}
	r, _ := ix.Record("dodi", id)
	if r.FetchError != "" || !NeedsDetail(r) {
		t.Fatalf("a server error stopped the release from being read again: %+v", r)
	}
	// Before the source's wait ends it isn't asked; after, it is read.
	if err := FetchDetail(context.Background(), ix, f.src.Source, f, id, clk.now()); err == nil || f.count(page) != 2 {
		t.Fatalf("asked during the wait: %v, %d requests", err, f.count(page))
	}
	clk.add(time.Hour)
	if err := FetchDetail(context.Background(), ix, f.src.Source, f, id, clk.now()); err != nil {
		t.Fatal(err)
	}
	if r, _ := ix.Record("dodi", id); r.Detailed.IsZero() || r.Entry.Version != "v1.1" || NeedsDetail(r) {
		t.Errorf("after the wait: %+v", r)
	}
}

func TestATimedOutRereadLeavesTheReleaseWaitingToBeRead(t *testing.T) {
	ix, _ := testIndex(t)
	f := newFakeSource(t, "dodi", 3)
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	id := relistedRecord(t, ix, f, now)
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	if err := FetchDetail(ctx, ix, f.src.Source, f, id, now.Add(2*time.Hour)); err == nil {
		t.Fatal("the timeout wasn't reported")
	}
	if r, _ := ix.Record("dodi", id); r.FetchError != "" || !NeedsDetail(r) {
		t.Errorf("a timeout stopped the release from being read again: %+v", r)
	}
}

// A page that can't be parsed stays unread until the source's row for it
// changes again, so it isn't asked for over and over.
func TestAReleaseThatCouldNotBeReadIsReadAgainWhenItsRowChanges(t *testing.T) {
	ix, _ := testIndex(t)
	f := newFakeSource(t, "dodi", 3)
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	id := relistedRecord(t, ix, f, now)
	ix.Update("dodi", id, func(r *Record) { r.FetchError = "the release page lists no matching release" })
	row := sources.Entry{SourceID: "dodi", PageURL: "https://dodi-repacks.site/game-001/", RawTitle: "Game 001 – v1.1", Title: "Game 001", TitleKey: "game001", ReleaseKind: "release", SummaryOnly: true}
	ix.Merge("dodi", []sources.Entry{row}, OriginListing, false, now.Add(2*time.Hour))
	if r, _ := ix.Record("dodi", id); NeedsDetail(r) {
		t.Fatal("read again while its row stayed the same")
	}
	row.RawTitle = "Game 001 – v1.2"
	if res := ix.Merge("dodi", []sources.Entry{row}, OriginListing, false, now.Add(3*time.Hour)); res.Relisted != 1 {
		t.Errorf("not relisted: %+v", res)
	}
	if r, _ := ix.Record("dodi", id); !NeedsDetail(r) || r.FetchError != "" {
		t.Errorf("after its row changed: %+v", r)
	}
}

// fetchFunc is a Fetcher made of a function.
type fetchFunc func(ctx context.Context, raw string) (sources.Document, error)

func (f fetchFunc) FetchDocument(ctx context.Context, raw string) (sources.Document, error) {
	return f(ctx, raw)
}

// A release page and a search wait for DODI while a listing page is out;
// DODI answers that one with 429: neither is sent after it.
func TestRequestsQueuedBehindAWaitAnswerAreNotSent(t *testing.T) {
	ix, _ := testIndex(t)
	p, _ := sources.Lookup("dodi")
	listing, out, answer := "https://dodi-repacks.site/page/7/", make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	var sent []string
	paced := Paced(ix, p.Source, fetchFunc(func(ctx context.Context, raw string) (sources.Document, error) {
		mu.Lock()
		sent = append(sent, raw)
		mu.Unlock()
		if raw == listing {
			close(out)
			<-answer
			return sources.Document{}, &sources.HTTPError{Status: http.StatusTooManyRequests, RetryAfter: "3600"}
		}
		return sources.Document{URL: raw, Body: []byte("<html></html>"), ContentType: "text/html"}, nil
	}))
	first := make(chan error, 1)
	go func() {
		_, err := paced.FetchDocument(context.Background(), listing)
		first <- err
	}()
	<-out
	queued := make(chan error, 2)
	for _, raw := range []string{"https://dodi-repacks.site/game-001/", "https://dodi-repacks.site/?s=game"} {
		go func() {
			_, err := paced.FetchDocument(context.Background(), raw)
			queued <- err
		}()
	}
	close(answer)
	if err := <-first; !slowDown(err) {
		t.Fatalf("the listing page: %v", err)
	}
	for range 2 {
		if err := <-queued; !errors.As(err, new(waiting)) || !transient(err) {
			t.Errorf("a queued request: %v", err)
		}
	}
	if len(sent) != 1 {
		t.Errorf("sent after the 429: %v", sent[1:])
	}
	// A pass that meets such a wait stops without counting a failure or
	// shortening the wait its caller saves.
	res := Pass(context.Background(), ix, p, paced, true, time.Now, PassHooks{})
	if c := ix.Crawl("dodi"); res.Stopped != StopWaiting || res.Err != nil || c.Failures != 0 || !c.RetryAt.IsZero() {
		t.Errorf("the pass: %+v, %+v", res, c)
	}
}
