package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/store/discovery"
	"github.com/ApolloF/Seaglass/internal/store/feed"
	"github.com/ApolloF/Seaglass/internal/store/jobs"
	"github.com/ApolloF/Seaglass/internal/store/sources"
)

const discoveryMagnet = "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c"

// pagesFetcher serves fixed documents by URL; anything else is a 404.
type pagesFetcher struct {
	pages map[string]string
	asked []string
}

func (f *pagesFetcher) FetchDocument(ctx context.Context, raw string) (sources.Document, error) {
	f.asked = append(f.asked, raw)
	if err := ctx.Err(); err != nil {
		return sources.Document{}, err
	}
	body, ok := f.pages[raw]
	if !ok {
		return sources.Document{}, &sources.HTTPError{Status: 404}
	}
	kind := "text/html"
	if strings.HasPrefix(body, "<?xml") {
		kind = "application/rss+xml"
	}
	return sources.Document{URL: raw, Body: []byte(body), ContentType: kind}, nil
}

func article(host, slug, title, version string, ago int, body string) string {
	pub := time.Now().Add(-time.Duration(ago) * time.Hour).UTC().Format(time.RFC3339)
	return `<article class="category-lossless-repack"><h1 class="entry-title"><a href="https://` + host + `/` + slug + `/">` + title + ` – ` + version + `</a></h1>` +
		`<time class="published" datetime="` + pub + `"></time><div class="entry-content"><p>Repack Size: 2.1 GB</p>` + body + `</div></article>`
}

// discoveryCore is a Store core whose sources answer from fixed pages.
func discoveryCore(t *testing.T) (*Core, *StoreService) {
	c := testStoreCore(t)
	fg := `<p>Languages: English, German</p><p><a href="magnet:?xt=urn:btih:` + discoveryMagnet + `&dn=x">magnet</a></p>`
	dodiBody := `<p>Languages: MULTi9</p><p>Torrent: <a href="https://unknown-host.example/abc">Mirror</a></p>`
	c.discovery.fetchers["fitgirl"] = &pagesFetcher{pages: map[string]string{
		"https://fitgirl-repacks.site/feed/": `<?xml version="1.0"?><rss xmlns:content="http://purl.org/rss/1.0/modules/content/"><channel></channel></rss>`,
		"https://fitgirl-repacks.site/": "<html><body>" + article("fitgirl-repacks.site", "ember-crown", "Ember Crown", "v1.2", 1, fg) +
			article("fitgirl-repacks.site", "quiet-orbit", "Quiet Orbit", "v1.0", 30, fg) + "</body></html>",
	}}
	c.discovery.fetchers["dodi"] = &pagesFetcher{pages: map[string]string{
		"https://dodi-repacks.site/": "<html><body>" + article("dodi-repacks.site", "ember-crown", "Ember Crown", "v1.1", 5, dodiBody) +
			article("dodi-repacks.site", "hollow-tide", "Hollow Tide", "v2.0", 2, dodiBody) + "</body></html>",
	}}
	return c, NewStoreService(c)
}

func TestStoreWorksWithoutAFeedAfterDiscovery(t *testing.T) {
	c, s := discoveryCore(t)
	if len(c.Settings.Get().Store.Feeds) != 0 {
		t.Fatal("the test core has feeds")
	}
	if err := c.discovery.refresh(); err != nil {
		t.Fatal(err)
	}
	res, err := s.BrowseGames(discovery.BrowseQuery{})
	if err != nil || res.Page.Total != 3 {
		t.Fatalf("browse: %d games, %v", res.Page.Total, err)
	}
	home, err := s.StoreHome()
	if err != nil || len(home.New) != 3 || home.New[0].Title != "Ember Crown" {
		t.Fatalf("home: %+v %v", home.New, err)
	}
	if (home.PopularState != discovery.StateUnavailable && home.PopularState != discovery.StateLoading) || len(home.Popular) != 0 {
		t.Error("the Popular shelf shows something without a chart")
	}
	st := s.DiscoveryStatus()
	if !st.Enabled || st.Games != 3 || st.Releases != 4 || st.Sources[0].RecentAt == 0 {
		t.Errorf("status: %+v", st)
	}
	d, err := s.GameDetails("title:embercrown")
	if err != nil || len(d.Releases) != 2 || d.Releases[0].Source != "fitgirl" || d.Recommended != 0 {
		t.Fatalf("details: %+v %v", d, err)
	}
	if d.Releases[1].Availability != discovery.AvailManual || len(d.Releases[1].Unresolved) == 0 {
		t.Errorf("the DODI release should say only a browser can get it: %+v", d.Releases[1])
	}
	if n := len(c.store.jobs.All()); n != 0 {
		t.Errorf("browsing queued %d downloads", n)
	}
}

// An announcement and a release from a source without torrents never
// reach a download or a .torrent attachment.
func TestPrepareExplainsAnnouncementsAndBrowserOnlySources(t *testing.T) {
	c, s := discoveryCore(t)
	if err := c.discovery.refresh(); err != nil {
		t.Fatal(err)
	}
	d, _ := s.GameDetails("title:embercrown")
	base, ok := c.discovery.index().Record("dodi", d.Releases[1].ID)
	if !ok {
		t.Fatal("the DODI release is not indexed")
	}
	preview := base.Entry
	preview.PageURL, preview.ReleaseKind = "https://dodi-repacks.site/ember-crown-coming-soon/", "preview"
	browser := base.Entry
	browser.SourceID, browser.PageURL = "elamigos", "https://elamigos.site/data/Ember_Crown_MULTi9_-_ElAmigos.html"
	browser.References = []sources.Reference{{Kind: "download", URL: "https://ddownload.com/abc", State: "manual-required"}}
	// A valid magnet that reached a browser-only record must still be refused.
	magnet, _ := sources.Magnet("magnet:?xt=urn:btih:" + discoveryMagnet)
	browser.Transports = []sources.Transport{magnet}
	now := time.Now()
	c.discovery.index().Merge("dodi", []sources.Entry{preview}, discovery.OriginListing, false, now)
	c.discovery.index().Merge("elamigos", []sources.Entry{browser}, discovery.OriginListing, false, now)
	c.discovery.fetchers["elamigos"] = &pagesFetcher{pages: map[string]string{}}
	if _, err := s.SetupSources([]string{"fitgirl", "dodi", "elamigos"}); err != nil {
		t.Fatal(err)
	}
	d, _ = s.GameDetails("title:embercrown")
	for _, r := range d.Releases {
		if r.BrowserOnly != (r.Source == "elamigos") || (r.Kind == "preview") != (r.Availability == discovery.AvailPreview) {
			t.Errorf("release %s: kind %s, availability %s, browser only %v", r.Source, r.Kind, r.Availability, r.BrowserOnly)
		}
	}
	for _, want := range []struct{ src, page, state string }{
		{"dodi", preview.PageURL, "preview"},
		{"elamigos", browser.PageURL, "browser"},
	} {
		id := sources.EntryID(want.src, discovery.CanonicalPage(want.page))
		p, err := s.PrepareRelease("title:embercrown", id)
		if err != nil || p.Ready || p.State != want.state || p.Reason == "" {
			t.Errorf("%s: %+v %v", want.state, p, err)
		}
		r, _ := c.discovery.index().Record(want.src, id)
		if err := attachable(r); err == nil {
			t.Errorf("%s: a .torrent could be attached", want.state)
		}
		if _, err := s.DownloadRelease("title:embercrown", id, 0, InstallOptions{}); err == nil || err.Error() != attachable(r).Error() {
			t.Errorf("%s: queued a download or refused it for another reason: %v", want.state, err)
		}
	}
	if err := attachable(base); err != nil {
		t.Errorf("a torrent source's release refused a .torrent: %v", err)
	}
}

func TestPreparedReleaseQueuesThroughTheExistingChecks(t *testing.T) {
	c, s := discoveryCore(t)
	if err := c.discovery.refresh(); err != nil {
		t.Fatal(err)
	}
	d, _ := s.GameDetails("title:embercrown")
	fg, dodi := d.Releases[0], d.Releases[1]
	p, err := s.PrepareRelease("title:embercrown", dodi.ID)
	if err != nil || p.Ready || p.State != "unresolved" || len(p.Offers) != 0 || !strings.Contains(p.Reason, "browser") {
		t.Fatalf("an unresolved release: %+v %v", p, err)
	}
	if _, err := s.DownloadRelease("title:embercrown", dodi.ID, 0, InstallOptions{}); err == nil {
		t.Fatal("a release without a validated torrent was queued")
	}
	p, err = s.PrepareRelease("title:embercrown", fg.ID)
	if err != nil || !p.Ready || len(p.Offers) != 1 || p.Offers[0].InfoHash != discoveryMagnet {
		t.Fatalf("a magnet release: %+v %v", p, err)
	}
	if got := p.Offers[0].Languages; len(got) != 2 || got[0] != "English" {
		t.Errorf("languages of a complete claim: %v", got)
	}
	j, err := s.DownloadRelease("title:embercrown", fg.ID, p.Offers[0].Transport, InstallOptions{Dir: t.TempDir() + `\Ember Crown`, Install: true})
	if err != nil {
		t.Fatal(err)
	}
	if j.GameKey != "title:embercrown" || j.Language != "English" || j.Version != "v1.2" || j.FeedName != "FitGirl" || !strings.Contains(j.Source, discoveryMagnet) {
		t.Errorf("queued job: %+v", j)
	}
	// The existing update guard: an older release can't replace this one.
	if _, err := c.store.jobs.Update(j.ID, func(j *jobs.Job) bool { j.State = jobs.Installed; return true }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DownloadRelease("title:embercrown", fg.ID, 0, InstallOptions{Update: true}); err == nil || !strings.Contains(err.Error(), "not a confirmed newer") {
		t.Errorf("updating to the same version: %v", err)
	}
}

func TestAttachedTorrentMakesAReleaseReady(t *testing.T) {
	c, s := discoveryCore(t)
	if err := c.discovery.refresh(); err != nil {
		t.Fatal(err)
	}
	d, _ := s.GameDetails("title:hollowtide")
	id := d.Releases[0].ID
	transport, _ := sources.Magnet("magnet:?xt=urn:btih:" + discoveryMagnet)
	transport.MetadataSHA256, transport.SizeBytes, transport.TorrentName = strings.Repeat("c", 64), 3<<30, "Hollow Tide"
	if !discovery.Attach(c.discovery.index(), "dodi", id, transport) {
		t.Fatal("attach failed")
	}
	p, err := c.discovery.prepared("title:hollowtide", id)
	if err != nil || !p.Ready || p.Offers[0].SizeBytes != 3<<30 || len(p.Offers[0].Languages) != 0 {
		t.Fatalf("after attaching: %+v %v", p, err)
	}
	if !strings.Contains(strings.Join(p.Warnings, "\n"), "Manual torrent attached") {
		t.Error("the attachment isn't called out")
	}
	// A refresh re-reads the article and keeps the attachment.
	c.discovery.index().SetCrawl("dodi", func(cs *discovery.CrawlState) { cs.RecentAt = time.Time{} })
	if err := c.discovery.refresh(); err != nil {
		t.Fatal(err)
	}
	if p, _ := c.discovery.prepared("title:hollowtide", id); !p.Ready {
		t.Error("a refresh lost the attached torrent")
	}
}

func TestSteamCorrectionRekeysTheGame(t *testing.T) {
	c, s := discoveryCore(t)
	if err := c.discovery.refresh(); err != nil {
		t.Fatal(err)
	}
	d, err := s.SetSteamMatch("title:embercrown", 1245620, "Ember Crown")
	if err != nil || d.Summary.Key != "steam:1245620" || !d.Identity.Corrected || d.Identity.How != "correction" || len(d.Releases) != 2 {
		t.Fatalf("after the correction: %+v %v", d.Identity, err)
	}
	if _, err := s.GameDetails("title:embercrown"); err == nil {
		t.Error("the old key still finds the game")
	}
	d, err = s.SetSteamMatch("steam:1245620", 0, "")
	if err != nil || d.Summary.Key != "title:embercrown" || d.Identity.SteamAppID != 0 {
		t.Errorf("'not on Steam': %+v %v", d.Identity, err)
	}
}

func TestFeedsAndReviewedOffersStayInTheStore(t *testing.T) {
	c, s := discoveryCore(t)
	c.catalog.entries = nil
	f := feed.Feed{Schema: feed.Schema, Name: "Indie", Items: []feed.Item{{Title: "Brass Orchard", Version: "Build 2", Magnet: "magnet:?xt=urn:btih:" + discoveryMagnet}}}
	c.catalog.cache.Dir = t.TempDir()
	if err := saveTestFeed(c, "https://feeds.example/indie.json", f); err != nil {
		t.Fatal(err)
	}
	res, err := s.BrowseGames(discovery.BrowseQuery{Sources: []string{discovery.SourceFeeds}})
	if err != nil || res.Page.Total != 1 || res.Page.Games[0].Title != "Brass Orchard" {
		t.Fatalf("feed games: %+v %v", res.Page, err)
	}
	d, err := s.GameDetails(res.Page.Games[0].Key)
	if err != nil || len(d.Releases) != 1 || d.Releases[0].Origin != discovery.OriginFeed || d.Releases[0].FeedKey == "" {
		t.Errorf("feed release: %+v %v", d.Releases, err)
	}
}

func TestTurningSourcesOffCancelsIndexing(t *testing.T) {
	c, s := discoveryCore(t)
	ctx := c.discovery.ctx()
	if _, err := c.updateSettings(func(v *settings.Settings) { v.Store.PrivateSources = false }); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil {
		t.Error("turning source browsing off didn't cancel discovery's requests")
	}
	if st := s.DiscoveryStatus(); st.Enabled {
		t.Error("discovery still enabled")
	}
	if _, err := s.RefreshDiscovery(); err == nil {
		t.Error("refresh ran with sources off")
	}
	if res, _ := s.BrowseGames(discovery.BrowseQuery{}); res.Page.Total != 0 {
		t.Error("games of turned-off sources are still shown")
	}
}

// saveTestFeed puts a feed in the catalog as if it had been fetched.
func saveTestFeed(c *Core, url string, f feed.Feed) error {
	if _, err := c.updateSettings(func(v *settings.Settings) {
		v.Store.Feeds = append(v.Store.Feeds, settings.FeedSource{URL: url, Enabled: true})
	}); err != nil {
		return err
	}
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(c.catalog.cache.Dir, feed.ID(url)+".json"), b, 0o600); err != nil {
		return fmt.Errorf("saving the feed: %w", err)
	}
	c.catalog.rebuild()
	return nil
}

func TestWishlistFollowsGamesNotReleases(t *testing.T) {
	c, s := discoveryCore(t)
	if err := c.discovery.refresh(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddToWishlist("title:hollowtide", "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddToWishlist("steam:3100100", "Hollow Tide II", 3100100); err != nil {
		t.Fatal(err)
	}
	items := s.Wishlist()
	if len(items) != 2 || items[0].Game.SourceBacked || !items[1].Game.SourceBacked || items[1].Unread != 0 {
		t.Fatalf("saved: %+v", items)
	}
	// DODI publishes a newer version, in a new article.
	f := c.discovery.fetchers["dodi"].(*pagesFetcher)
	body := `<p>Torrent: <a href="https://unknown-host.example/abc">Mirror</a></p>`
	f.pages["https://dodi-repacks.site/"] = "<html><body>" + article("dodi-repacks.site", "hollow-tide-v21", "Hollow Tide", "v2.1", 0, body) + f.pages["https://dodi-repacks.site/"][len("<html><body>"):]
	if err := c.discovery.refresh(); err != nil {
		t.Fatal(err)
	}
	items = s.Wishlist()
	if items[1].Unread != 1 || items[1].Activity[0].Kind != "newer" || items[1].Activity[0].Version != "v2.1" || !items[1].Game.Activity {
		t.Fatalf("after a newer release: %+v", items[1])
	}
	if res, _ := s.BrowseGames(discovery.BrowseQuery{Text: "hollow"}); !res.Page.Games[0].Wishlisted || !res.Page.Games[0].Activity {
		t.Error("browse doesn't mark the saved game's activity")
	}
	if items, _ = s.AcknowledgeWishlist("title:hollowtide"); items[1].Unread != 0 {
		t.Error("acknowledged activity is still unread")
	}
	if n := len(c.store.jobs.All()); n != 0 {
		t.Errorf("wishlisting queued %d downloads", n)
	}
}

func TestRemoteSearchFillsGapsAndLeavesTheStoreResponsive(t *testing.T) {
	c, s := discoveryCore(t)
	f := c.discovery.fetchers["dodi"].(*pagesFetcher)
	f.pages["https://dodi-repacks.site/?s=lantern"] = `<html><body><article><h1 class="entry-title"><a href="https://dodi-repacks.site/lantern-season/">123- Lantern Season (v1.0.2) [DODI Repack]</a></h1>` +
		`<div class="entry-summary"><p>Excerpt</p></div><time class="published" datetime="2024-01-01T00:00:00Z"></time></article></body></html>`
	done := make(chan discovery.SearchResult)
	go func() {
		res, err := s.SearchGames(discovery.BrowseQuery{Text: "lantern"})
		if err != nil {
			t.Error(err)
		}
		done <- res
	}()
	var res discovery.SearchResult
	select {
	case res = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the search hung")
	}
	if res.Page.Total != 1 || res.Page.Games[0].Title != "Lantern Season" || !res.Complete {
		t.Fatalf("remote result: %+v", res)
	}
	states := map[string]string{}
	for _, p := range res.Remote {
		states[p.ID] = p.State
	}
	if states["dodi"] != discovery.StateOK || states["fitgirl"] != discovery.StateError {
		t.Errorf("provider states: %v (FitGirl's search page isn't served, so it fails without hiding DODI's)", states)
	}
	// Answered from the hour's cache the second time; and the Store still answers.
	res, _ = s.SearchGames(discovery.BrowseQuery{Text: "lantern"})
	for _, p := range res.Remote {
		if p.ID == "dodi" && !p.Cached {
			t.Error("the second search asked DODI again")
		}
	}
	if _, err := s.GameDetails(res.Page.Games[0].Key); err != nil {
		t.Fatal(err)
	}
	// A found summary from long ago is history, not news.
	r, _ := c.discovery.index().Record("dodi", sources.EntryID("dodi", "https://dodi-repacks.site/lantern-season/"))
	if !r.Backfill || !r.Entry.SummaryOnly {
		t.Errorf("search record: %+v", r)
	}
	if res, _ := s.SearchGames(discovery.BrowseQuery{Text: "l"}); res.Remote[0].State != discovery.StateSkipped {
		t.Error("a one-letter query went to the sources")
	}
}
