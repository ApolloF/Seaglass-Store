package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ApolloF/Seaglass/internal/identify"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/store/catalog"
	"github.com/ApolloF/Seaglass/internal/store/feed"
	"github.com/ApolloF/Seaglass/internal/store/jobs"
)

// testStoreCore is a Core with only what the store needs, in a temp folder.
func testStoreCore(t *testing.T) *Core {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := &Core{Settings: settings.Open(filepath.Join(dir, "settings.json")), Manifest: identify.NewManager(filepath.Join(dir, "manifest")), ctx: ctx, cancel: cancel}
	c.store = &storeState{c: c, jobs: jobs.Open(filepath.Join(dir, "downloads.json")), kick: make(chan struct{}, 1)}
	c.store.pipe = newPipeline(c.store)
	c.catalog = &catalogState{c: c, cache: feed.Cache{Dir: filepath.Join(dir, "feeds")}}
	v := c.Settings.Get()
	v.ExperimentalStore = true
	if _, err := c.Settings.Set(v); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCatalogFromFeeds(t *testing.T) {
	const m = "magnet:?xt=urn:btih:dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a.json":
			w.Write([]byte(`{"schema":1,"name":"Feed A","items":[
				{"title":"Ember Crown","version":"v1.0","magnet":"` + m + `","sizeBytes":1000,"languages":["English"]},
				{"title":"Hollow Tide","version":"v2","magnet":"` + m + `"}]}`))
		case "/b.json":
			w.Write([]byte(`{"schema":1,"name":"Feed B","items":[
				{"title":"ember crown","version":"v1.1","magnet":"` + m + `","languages":["German"]}]}`))
		default:
			w.Write([]byte(`{"not":"a feed"}`))
		}
	}))
	defer srv.Close()
	c := testStoreCore(t)
	cs := c.catalog
	ctx := context.Background()

	if _, err := cs.addFeed(ctx, srv.URL+"/nope.json"); err == nil {
		t.Error("something that isn't a feed was added")
	}
	for _, p := range []string{"/a.json", "/b.json"} {
		if _, err := cs.addFeed(ctx, srv.URL+p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := cs.addFeed(ctx, srv.URL+"/a.json"); err == nil {
		t.Error("the same feed was added twice")
	}
	if got := c.Settings.Get().Store.Feeds; len(got) != 2 || !got[0].Enabled {
		t.Fatalf("feeds in settings: %+v", got)
	}
	cs.rebuild()
	page := cs.search(catalog.Query{Text: "ember"})
	if page.Total != 1 || len(page.Entries[0].Offers) != 2 || page.Entries[0].Version != "v1.1" || page.Entries[0].Offers[0].FeedName != "Feed B" {
		t.Fatalf("ember crown from both feeds, newest first: %+v", page)
	}

	key := page.Entries[0].Key
	svc := NewStoreService(c)
	gameDir := filepath.Join(t.TempDir(), "Ember Crown")
	j, err := svc.DownloadOffer(key, 1, InstallOptions{Dir: gameDir, Language: "English", Install: true})
	if err != nil {
		t.Fatal(err)
	}
	if j.Title != "Ember Crown v1.0" || j.Source != m || j.GameKey != key || j.FeedName != "Feed A" || j.State != jobs.Queued ||
		j.InstallDir != gameDir || j.Language != "English" || !j.AutoInstall {
		t.Errorf("queued offer: %+v", j)
	}
	if _, err := svc.DownloadOffer(key, 5, InstallOptions{}); err == nil {
		t.Error("an offer that doesn't exist was queued")
	}
	if _, err := svc.DownloadOffer(key, 1, InstallOptions{Dir: gameDir, Language: "Klingon"}); err == nil {
		t.Error("a language the version doesn't offer was accepted")
	}

	if _, err := cs.setFeedEnabled(srv.URL+"/b.json", false); err != nil {
		t.Fatal(err)
	}
	cs.rebuild()
	if e, _ := cs.entry(key); len(e.Offers) != 1 || e.Version != "v1.0" {
		t.Errorf("with feed B off: %+v", e)
	}
	if _, err := cs.removeFeed(srv.URL + "/a.json"); err != nil {
		t.Fatal(err)
	}
	cs.rebuild()
	if n := cs.len(); n != 0 {
		t.Errorf("%d games left with one feed removed and the other off", n)
	}
	if fs := cs.feeds(); len(fs) != 1 || fs[0].Enabled || fs[0].Name != "Feed B" {
		t.Errorf("feeds: %+v", fs)
	}
}
