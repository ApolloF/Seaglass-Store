package app

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/scan"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/store/discovery"
	"github.com/ApolloF/Seaglass/internal/store/enrich"
	"github.com/ApolloF/Seaglass/internal/store/sources"
	"github.com/ApolloF/Seaglass/internal/store/wishlist"
)

const testSteamID = "76561197960287930"

// steamNet answers Steam's requests by URL substring.
type steamNet struct {
	mu      sync.Mutex
	answers map[string]string
	asked   []string
}

func (n *steamNet) RoundTrip(req *http.Request) (*http.Response, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	u := req.URL.String()
	n.asked = append(n.asked, u)
	for part, body := range n.answers {
		if strings.Contains(u, part) {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
		}
	}
	return nil, errors.New("offline in tests")
}

func (n *steamNet) requests() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.asked)
}

// wishlistCore is a discovery core whose Steam answers from net. Ember
// Crown is Steam app 1245620.
func wishlistCore(t *testing.T, net *steamNet) (*Core, *StoreService) {
	c, s := discoveryCore(t)
	c.enrich.client = enrich.New(enrich.Options{Dir: filepath.Join(t.TempDir(), "enrich"), Transport: net})
	if err := c.discovery.refresh(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetSteamMatch("title:embercrown", 1245620, "Ember Crown"); err != nil {
		t.Fatal(err)
	}
	return c, s
}

func wishlistAnswer(ids ...string) string {
	var items []string
	for _, id := range ids {
		items = append(items, `{"appid":`+id+`,"priority":0,"date_added":1700000000}`)
	}
	return `{"response":{"items":[` + strings.Join(items, ",") + `]}}`
}

func TestImportingASteamWishlistMergesByAppID(t *testing.T) {
	net := &steamNet{answers: map[string]string{"GetWishlist": wishlistAnswer("1245620", "4200000", "3100100")}}
	_, s := wishlistCore(t, net)
	if _, err := s.AddToWishlist("steam:3100100", "Hollow Tide II", 3100100); err != nil {
		t.Fatal(err)
	}
	res, err := s.ImportSteamWishlist(" " + testSteamID + " ")
	if err != nil {
		t.Fatal(err)
	}
	if res.SteamID != testSteamID || res.Fetched != 3 || res.Added != 2 || res.Existing != 1 || res.Available != 1 || res.Searching != 2 {
		t.Errorf("counts: %+v", res)
	}
	if len(res.Items) != 3 {
		t.Fatalf("items: %+v", res.Items)
	}
	ember, unknown, saved := res.Items[0], res.Items[1], res.Items[2]
	if ember.Key != "steam:1245620" || ember.Title != "Ember Crown" || ember.Origin != wishlist.OriginSteam || !ember.Game.SourceBacked || ember.Unread != 0 {
		t.Errorf("a game with releases, listed first as on Steam, its releases not news: %+v", ember)
	}
	if unknown.Key != "steam:4200000" || unknown.Title != "Steam app 4200000" || unknown.Origin != wishlist.OriginSteam || unknown.Game.SourceBacked {
		t.Errorf("a game without a known name or release is kept: %+v", unknown)
	}
	if saved.Key != "steam:3100100" || saved.Origin != wishlist.OriginManual || saved.Title != "Hollow Tide II" {
		t.Errorf("the game saved before stays as it was: %+v", saved)
	}

	// Steam drops a game: importing again removes nothing.
	net.answers["GetWishlist"] = wishlistAnswer("1245620")
	res, err = s.ImportSteamWishlist(testSteamID)
	if err != nil || res.Added != 0 || res.Existing != 1 || len(s.Wishlist()) != 3 {
		t.Errorf("second import: %+v %v", res, err)
	}
	if _, err := s.RemoveFromWishlist("steam:4200000"); err != nil || len(s.Wishlist()) != 2 {
		t.Errorf("removing an imported game: %v", err)
	}
}

func TestAFullWishlistKeepsSteamsFirstGamesAndSaysHowManyWereLeftOut(t *testing.T) {
	c, _ := wishlistCore(t, &steamNet{})
	fill := make([]wishlist.Imported, wishlist.MaxEntries-2)
	for i := range fill {
		fill[i] = wishlist.Imported{Key: "title:filler" + strconv.Itoa(i), Title: "Filler " + strconv.Itoa(i)}
	}
	if _, _, err := c.wishlist.store.Import(fill, time.Now()); err != nil {
		t.Fatal(err)
	}
	res, err := c.wishlist.importSteam([]int{9001, 9002, 9003, 9004, 9005})
	if err != nil || res.Added != 2 || res.Skipped != 3 || res.Fetched != 5 {
		t.Fatalf("counts: %+v %v", res, err)
	}
	for _, key := range []string{"steam:9001", "steam:9002"} {
		if _, ok := c.wishlist.store.Get(key); !ok {
			t.Errorf("%s, high on Steam's wishlist, wasn't kept", key)
		}
	}
	if _, ok := c.wishlist.store.Get("steam:9005"); ok {
		t.Error("the last game on Steam's wishlist was kept over earlier ones")
	}
}

func TestAnAnnouncementAloneIsNotAReleaseForAnImportedGame(t *testing.T) {
	released := sources.Entry{ReleaseKind: "release"}
	preview := sources.Entry{ReleaseKind: "preview"}
	rec := func(e sources.Entry) discovery.Record { return discovery.Record{Entry: e} }
	for name, c := range map[string]struct {
		records []discovery.Record
		want    bool
	}{
		"none":              {nil, false},
		"only a preview":    {[]discovery.Record{rec(preview)}, false},
		"a release":         {[]discovery.Record{rec(released)}, true},
		"preview and a real":{[]discovery.Record{rec(preview), rec(released)}, true},
	} {
		if got := hasRelease(&discovery.Game{Records: c.records}); got != c.want {
			t.Errorf("%s: %v", name, got)
		}
	}
}

func TestAPrivateWishlistChangesNothing(t *testing.T) {
	net := &steamNet{answers: map[string]string{"GetWishlist": `{"response":{}}`}}
	_, s := wishlistCore(t, net)
	if _, err := s.ImportSteamWishlist(testSteamID); !errors.Is(err, enrich.ErrWishlistPrivate) {
		t.Errorf("private profile: %v", err)
	}
	if len(s.Wishlist()) != 0 {
		t.Error("a private wishlist saved games")
	}
}

func TestImportRefusesBadSteamIDsAndAStoreThatIsOff(t *testing.T) {
	net := &steamNet{answers: map[string]string{"GetWishlist": wishlistAnswer("1245620")}}
	c, s := wishlistCore(t, net)
	for _, id := range []string{"", "1245620", "86561197960287930", "7656119796028793x"} {
		if _, err := s.ImportSteamWishlist(id); !errors.Is(err, enrich.ErrSteamID) {
			t.Errorf("%q: %v", id, err)
		}
	}
	if _, err := c.updateSettings(func(v *settings.Settings) { v.ExperimentalStore = false }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportSteamWishlist(testSteamID); err == nil {
		t.Error("imported with the Store off")
	}
	if net.requests() != 0 {
		t.Errorf("Steam was asked: %v", net.asked)
	}
}

func TestTheSteamAccountOnThisPCPrefillsTheImport(t *testing.T) {
	_, s := discoveryCore(t)
	defer func(f func() (string, error)) { steamAccountID = f }(steamAccountID)
	steamAccountID = func() (string, error) { return testSteamID, nil }
	if a := s.SteamWishlistAccount(); a.SteamID != testSteamID || !a.Detected || a.Error != "" {
		t.Errorf("detected: %+v", a)
	}
	steamAccountID = func() (string, error) { return "", errors.New("no Steam account has signed in on this PC") }
	if a := s.SteamWishlistAccount(); a.SteamID != "" || a.Detected || a.Error != "No Steam account has signed in on this PC." {
		t.Errorf("none: %+v", a)
	}
}

// searchCore has imported 4200000 (Lantern Season, which only DODI's
// search finds) and 3100100 (which no source has).
func searchCore(t *testing.T) (*Core, *StoreService, *pagesFetcher) {
	net := &steamNet{answers: map[string]string{
		"GetWishlist":    wishlistAnswer("4200000", "3100100"),
		"appids=4200000": `{"4200000":{"success":true,"data":{"type":"game","name":"Lantern Season"}}}`,
		"appids=3100100": `{"3100100":{"success":true,"data":{"type":"game","name":"Hollow Tide II"}}}`,
	}}
	c, s := wishlistCore(t, net)
	if err := c.discovery.index().SetSteamCorrections([]discovery.IdentityCorrection{{TitleKey: scan.Normalize("Lantern Season"), SteamAppID: 4200000, Name: "Lantern Season", At: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	dodi := c.discovery.fetchers["dodi"].(*pagesFetcher)
	p, _ := sources.Lookup("dodi")
	u, _ := p.SearchURL("Lantern Season")
	dodi.pages[u] = `<html><body><article><h1 class="entry-title"><a href="https://dodi-repacks.site/lantern-season/">123- Lantern Season (v1.0.2) [DODI Repack]</a></h1>` +
		`<div class="entry-summary"><p>Excerpt</p></div><time class="published" datetime="2024-01-01T00:00:00Z"></time></article></body></html>`
	res, err := s.ImportSteamWishlist(testSteamID)
	if err != nil || res.Searching != 2 {
		t.Fatalf("import: %+v %v", res, err)
	}
	return c, s, dodi
}

func TestIdleSearchesFindReleasesForImportedGames(t *testing.T) {
	c, s, dodi := searchCore(t)
	w := c.wishlist
	now := time.Now()
	if wait := w.searchNext(now); wait != wishlist.SearchSpacing {
		t.Errorf("waits %v after a search", wait)
	}
	items := s.Wishlist()
	lantern := items[0]
	if lantern.Key != "steam:4200000" || lantern.Title != "Lantern Season" || !lantern.Game.SourceBacked || lantern.Unread != 0 {
		t.Fatalf("after its search: %+v", lantern)
	}
	asked := len(dodi.asked)
	if wait := w.searchNext(now.Add(time.Minute)); wait <= 0 || len(dodi.asked) != asked {
		t.Errorf("searched again within the spacing (wait %v)", wait)
	}
	w.searchNext(now.Add(wishlist.SearchSpacing))
	if w.queue().Len() != 0 || len(dodi.asked) == asked {
		t.Errorf("the second game wasn't searched: %d queued", w.queue().Len())
	}
	// Not again within a day, even when imported again.
	if res, err := s.ImportSteamWishlist(testSteamID); err != nil || res.Searching != 0 || res.Available != 1 {
		t.Errorf("import a day later: %+v %v", res, err)
	}
}

func TestIdleSearchesWaitWhilePausedPlayingOrOff(t *testing.T) {
	cases := map[string]func(c *Core){
		"paused": func(c *Core) {
			c.updateSettings(func(v *settings.Settings) { v.Store.IndexingPaused = true })
		},
		"playing": func(c *Core) { c.discovery.isPlaying = func() bool { return true } },
		"sources off": func(c *Core) {
			c.updateSettings(func(v *settings.Settings) { v.Store.PrivateSources = false })
		},
	}
	for name, set := range cases {
		t.Run(name, func(t *testing.T) {
			c, _, dodi := searchCore(t)
			set(c)
			asked := len(dodi.asked)
			if wait := c.wishlist.searchNext(time.Now()); wait != searchIdle {
				t.Errorf("waits %v", wait)
			}
			if len(dodi.asked) != asked || c.wishlist.queue().Len() != 2 {
				t.Errorf("searched anyway: %d queued", c.wishlist.queue().Len())
			}
		})
	}
}
