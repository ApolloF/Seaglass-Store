package app

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/store/discovery"
	"github.com/ApolloF/Seaglass/internal/store/enrich"
)

// hltbFake answers HowLongToBeat's token, search and game page, counting
// what it was asked. Nothing leaves the test.
type hltbFake struct {
	search   string
	failing  atomic.Bool
	requests atomic.Int32
	age      atomic.Int64 // seconds the client's clock is ahead
}

func (f *hltbFake) RoundTrip(req *http.Request) (*http.Response, error) {
	f.requests.Add(1)
	reply := func(code int, body string) (*http.Response, error) {
		return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(bytes.NewBufferString(body)), ContentLength: int64(len(body)), Request: req}, nil
	}
	if f.failing.Load() {
		return reply(503, "")
	}
	switch p := req.URL.Path; {
	case strings.HasSuffix(p, "/api/search/site/init"):
		return reply(200, `{"token":"t"}`)
	case strings.HasSuffix(p, "/api/search/site"):
		return reply(200, f.search)
	case strings.HasPrefix(p, "/game/42"):
		return reply(200, `<script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"game":{"data":{"game":[{"game_id":42,"game_name":"Chosen One","comp_main":36000,"comp_plus":72000,"comp_100":108000}]}}}}}</script>`)
	}
	return reply(404, "")
}

const portalSearch = `{"data":[{"game_id":7231,"game_name":"Portal 2","game_type":"game","comp_main":30883,"comp_plus":49552,"comp_100":82580,"release_world":2011}]}`

// twoRemakes has two games with the wanted title and no year to tell them
// apart, so nothing settles the match.
const twoRemakes = `{"data":[
 {"game_id":1,"game_name":"Remake","game_type":"game","comp_main":3600,"release_world":2001},
 {"game_id":2,"game_name":"Remake","game_type":"game","comp_main":7200,"release_world":2002}]}`

// completionCore is a core with the Store turned off, a library and
// HowLongToBeat faked.
func completionCore(t *testing.T, fake *hltbFake) *Core {
	t.Helper()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	lib, err := library.Open(filepath.Join(dir, "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := &Core{Lib: lib, Settings: settings.Open(filepath.Join(dir, "settings.json")), ctx: ctx, cancel: cancel}
	c.discovery = newDiscoveryState(c)
	c.discovery.ix = discovery.OpenIndex(filepath.Join(dir, "discovery"), filepath.Join(dir, "store-identity.json"))
	c.enrich = &enrichState{c: c, client: enrich.New(enrich.Options{Dir: filepath.Join(dir, "enrich"), Transport: fake, Now: func() time.Time { return time.Now().Add(time.Duration(fake.age.Load()) * time.Second) }}), wake: make(chan struct{}, 1), queued: map[string]bool{}}
	return c
}

func addGame(t *testing.T, c *Core, f library.Found) int64 {
	t.Helper()
	if f.Key == "" {
		f.Key = `c:\games\` + strings.ToLower(f.Title)
	}
	f.Dir = f.Key
	c.Lib.ApplyScan([]library.Found{f}, time.Now())
	for _, g := range c.Lib.Games() {
		if g.Key == f.Key {
			return g.ID
		}
	}
	t.Fatal("game not added")
	return 0
}

func TestCompletionWorksWithTheStoreTurnedOff(t *testing.T) {
	c := completionCore(t, &hltbFake{search: portalSearch})
	if c.Settings.Get().ExperimentalStore {
		t.Fatal("the Store should be off for this test")
	}
	id := addGame(t, c, library.Found{Title: "Portal 2", Source: "epic", Confidence: 50})
	got, err := NewLibraryService(c).Completion(id, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Completion.State != enrich.StateOK || got.Completion.Main != 515 || got.Completion.MainExtras != 826 || got.Completion.Completionist != 1376 {
		t.Fatalf("times: %+v", got.Completion)
	}
}

func TestCompletionWithoutFetchAnswersFromTheCacheAndAsksNoOne(t *testing.T) {
	fake := &hltbFake{search: portalSearch}
	c := completionCore(t, fake)
	s := NewLibraryService(c)
	id := addGame(t, c, library.Found{Title: "Portal 2", Source: "gog"})
	got, _ := s.Completion(id, false)
	if got.Completion.State != enrich.StateLoading || fake.requests.Load() != 0 {
		t.Fatalf("not fetched yet: %+v after %d requests", got.Completion, fake.requests.Load())
	}
	if _, err := s.Completion(id, true); err != nil {
		t.Fatal(err)
	}
	asked := fake.requests.Load()
	got, _ = s.Completion(id, false)
	if got.Completion.State != enrich.StateOK || got.Completion.Main != 515 || fake.requests.Load() != asked {
		t.Fatalf("cached read: %+v, %d requests more", got.Completion, fake.requests.Load()-asked)
	}
}

func TestCompletionKeyIsSteamTrustedOrTitleNeverTheFolder(t *testing.T) {
	cases := []struct {
		name string
		g    library.Game
		want string
	}{
		{"a Steam game", library.Game{Source: "steam", SteamAppID: 620, Title: "Portal 2", Confidence: 100}, "steam:620"},
		{"a confirmed match", library.Game{Source: "folder", SteamAppID: 620, Confirmed: true, Confidence: 10, Title: "Portal 2"}, "steam:620"},
		{"a confident match", library.Game{Source: "folder", SteamAppID: 620, Confidence: 80, Title: "Portal 2"}, "steam:620"},
		{"an untrusted app id", library.Game{Source: "folder", SteamAppID: 620, Confidence: 79, Title: "Portal 2", Key: `d:\games\portal 2`}, "title:" + "portal2"},
		{"a metadata-only app", library.Game{Source: "epic", MetaAppID: 620, Confidence: 100, Title: "Portal 2"}, "title:portal2"},
		{"no identity", library.Game{Source: "folder", Title: "Portal 2", CustomTitle: "Portal Two", Key: `d:\games\p2`}, "title:portaltwo"},
	}
	for _, tc := range cases {
		got, _ := completionIdentity(tc.g)
		if got != tc.want {
			t.Errorf("%s: key %q, want %q", tc.name, got, tc.want)
		}
		if strings.Contains(got, `\`) || got == tc.g.Key {
			t.Errorf("%s: key %q is the folder's", tc.name, got)
		}
	}
}

func TestCompletionUsesSteamsNameForASteamIdentityElseTheDisplayTitle(t *testing.T) {
	_, title := completionIdentity(library.Game{Source: "steam", SteamAppID: 620, Title: "Portal 2", CustomTitle: "My Portal"})
	if title != "Portal 2" {
		t.Errorf("Steam game searched as %q", title)
	}
	_, title = completionIdentity(library.Game{Source: "folder", Title: "portal2", CustomTitle: "Portal 2"})
	if title != "Portal 2" {
		t.Errorf("folder game searched as %q", title)
	}
}

func TestCorrectionSetInTheLibraryIsReadUnderTheStoreKeyAndBack(t *testing.T) {
	c := completionCore(t, &hltbFake{search: portalSearch})
	s := NewLibraryService(c)
	id := addGame(t, c, library.Found{Title: "Portal 2", Source: "steam", SteamAppID: 620, Confidence: 100})

	got, err := s.SetCompletionMatch(id, 42)
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "steam:620" || got.Completion.HLTBID != 42 || !got.Completion.Corrected || got.Completion.Main != 600 {
		t.Fatalf("after the choice: %+v", got)
	}
	if c.discovery.index().CompletionMatch("steam:620") != 42 {
		t.Error("the Store's page for the same game doesn't see the choice")
	}

	if err := c.discovery.index().SetCompletionMatch("steam:620", 0); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.Completion(id, true); got.Completion.Corrected || got.Completion.HLTBID != 7231 {
		t.Errorf("a choice cleared in the Store is still used here: %+v", got.Completion)
	}
	if err := c.discovery.index().SetCompletionMatch("steam:620", 42); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.Completion(id, true); got.Completion.HLTBID != 42 {
		t.Errorf("a choice made in the Store isn't used here: %+v", got.Completion)
	}
}

func TestSettingTheMatchToZeroReturnsToTheAutomaticMatch(t *testing.T) {
	c := completionCore(t, &hltbFake{search: portalSearch})
	s := NewLibraryService(c)
	id := addGame(t, c, library.Found{Title: "Portal 2", Source: "gog"})
	if _, err := s.SetCompletionMatch(id, 42); err != nil {
		t.Fatal(err)
	}
	got, err := s.SetCompletionMatch(id, 0)
	if err != nil || got.Completion.HLTBID != 7231 || got.Completion.Corrected {
		t.Fatalf("automatic again: %+v, %v", got.Completion, err)
	}
	if _, err := s.SetCompletionMatch(id, -3); err == nil {
		t.Error("a negative HowLongToBeat id was accepted")
	}
}

func TestCompletionInputsAreBoundedAtTheBoundary(t *testing.T) {
	fake := &hltbFake{search: portalSearch}
	c := completionCore(t, fake)
	s := NewLibraryService(c)
	id := addGame(t, c, library.Found{Title: "Portal 2", Source: "gog"})
	if _, err := s.CompletionCandidates(id, strings.Repeat("a", maxCompletionQuery+1)); err == nil {
		t.Error("a search over 200 bytes was accepted")
	}
	if _, err := s.CompletionCandidates(id, "  "+strings.Repeat("a", maxCompletionQuery)+"  "); err != nil && strings.Contains(err.Error(), "too long") {
		t.Errorf("padding counted toward the length: %v", err)
	}
	for _, bad := range []int{maxHLTBID + 1, -1} {
		if _, err := s.SetCompletionMatch(id, bad); err == nil {
			t.Errorf("match %d was accepted", bad)
		}
	}
	if c.discovery.index().CompletionMatch("title:portal2") != 0 {
		t.Error("a refused match was saved")
	}
	if _, err := s.SetCompletionMatch(id, maxHLTBID); err != nil {
		t.Errorf("the largest match was refused: %v", err)
	}
}

func TestANewCompletionLookupCancelsTheOneBeforeIt(t *testing.T) {
	var l latestLookup
	first, release := l.begin(context.Background(), time.Minute)
	defer release()
	second, release2 := l.begin(context.Background(), time.Minute)
	defer release2()
	if first.Err() == nil {
		t.Error("the earlier game's lookup kept running")
	}
	if second.Err() != nil {
		t.Errorf("the newest lookup was cancelled: %v", second.Err())
	}
}

func TestAmbiguousTitleStaysUnmatchedWithASearchLink(t *testing.T) {
	c := completionCore(t, &hltbFake{search: twoRemakes})
	id := addGame(t, c, library.Found{Title: "Remake", Source: "folder"})
	got, err := NewLibraryService(c).Completion(id, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Completion.HLTBID != 0 || got.Completion.State != enrich.StateUnavailable || !strings.HasPrefix(got.Completion.URL, "https://howlongtobeat.com/?q=") {
		t.Fatalf("guessed: %+v", got.Completion)
	}
}

func TestFailedRefreshKeepsCachedTimesAsStaleAndNothingKnownHasOnlyALink(t *testing.T) {
	fake := &hltbFake{search: portalSearch}
	c := completionCore(t, fake)
	s := NewLibraryService(c)
	known := addGame(t, c, library.Found{Title: "Portal 2", Source: "gog"})
	other := addGame(t, c, library.Found{Title: "Unseen Game", Source: "gog"})
	if _, err := s.Completion(known, true); err != nil {
		t.Fatal(err)
	}

	fake.failing.Store(true)
	got, err := s.Completion(other, true)
	if err != nil || got.Completion.State != enrich.StateError || got.Completion.Main != 0 || got.Completion.URL == "" {
		t.Fatalf("nothing known: %+v, %v", got.Completion, err)
	}

	// Seven days on the cached times are old, and the site still fails.
	fake.age.Store(int64(enrich.CompletionTTL/time.Second) + 3600)
	got, err = s.Completion(known, true)
	if err != nil || got.Completion.State != enrich.StateStale || got.Completion.Main != 515 || got.Completion.Error == "" {
		t.Fatalf("old times, failed refresh: %+v, %v", got.Completion, err)
	}
}

func TestOpenCompletionLinkRejectsOtherHosts(t *testing.T) {
	for _, raw := range []string{"http://howlongtobeat.com/game/1", "https://evil.example/", "https://howlongtobeat.com.evil.example/", "https://user@howlongtobeat.com/game/1", "https://howlongtobeat.com:8443/", "https://store.steampowered.com/app/1", "file:///C:/Windows", "javascript:alert(1)"} {
		if _, err := completionLink(raw); err == nil {
			t.Errorf("%s was accepted", raw)
		}
		if err := (&LibraryService{}).OpenCompletionLink(raw); err == nil {
			t.Errorf("%s was opened", raw)
		}
	}
	for _, raw := range []string{"https://howlongtobeat.com/game/7231", "https://www.howlongtobeat.com/?q=portal+2"} {
		if _, err := completionLink(raw); err != nil {
			t.Errorf("%s: %v", raw, err)
		}
	}
}
