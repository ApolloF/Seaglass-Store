package enrich

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCompletionTakesTimesFromAnExactSearchMatch(t *testing.T) {
	c, fn, _ := newTestClient(t, hltbRouter(t, "hltb_search_portal2.json", ""))
	got := c.Completion(context.Background(), CompletionQuery{Title: "Portal 2"})
	want := Completion{HLTBID: 7231, Title: "Portal 2", Main: 515, MainExtras: 826, Completionist: 1376,
		URL: "https://howlongtobeat.com/game/7231", FetchedAt: got.FetchedAt, State: StateOK}
	if got != want {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
	if got.Corrected {
		t.Fatal("an automatic match is not a correction")
	}
	if got.FetchedAt == 0 {
		t.Fatal("FetchedAt not set")
	}

	init := fn.find("/api/search/site/init")
	search := fn.find("/api/search/site")
	if len(init) != 1 || len(search) != 2 { // "…/site" also matches the init path
		t.Fatalf("%d init, %d search-path requests", len(init), len(search))
	}
	post := search[1]
	if post.Method != "POST" || post.Header.Get("x-auth-token") != "test-token" || post.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("search request %+v", post)
	}
	if post.Header.Get("Referer") != "https://howlongtobeat.com/" || strings.Contains(post.Header.Get("User-Agent"), "Mozilla") {
		t.Fatalf("headers %v", post.Header)
	}
	var body struct {
		SearchType  string   `json:"searchType"`
		SearchTerms []string `json:"searchTerms"`
		SearchPage  int      `json:"searchPage"`
	}
	if err := json.Unmarshal([]byte(post.Body), &body); err != nil {
		t.Fatal(err)
	}
	if body.SearchType != "games" || len(body.SearchTerms) != 2 || body.SearchTerms[0] != "Portal" || body.SearchTerms[1] != "2" || body.SearchPage != 1 {
		t.Fatalf("body %s", post.Body)
	}
}

func TestSearchWordsLeavePunctuationOut(t *testing.T) {
	for in, want := range map[string]string{
		"Portal 2:":                   "Portal 2",
		"Half-Life 2":                 "Half Life 2",
		"Ratchet & Clank":             "Ratchet Clank",
		"Assassin's Creed® (2007)":    "Assassin s Creed",
		"  The Witcher™ 3: Wild Hunt": "The Witcher 3 Wild Hunt",
		"!!!":                         "",
	} {
		if got := strings.Join(searchTerms(in), " "); got != want {
			t.Errorf("searchTerms(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSecondCompletionCallComesFromTheCache(t *testing.T) {
	c, fn, clk := newTestClient(t, hltbRouter(t, "hltb_search_portal2.json", ""))
	ctx := context.Background()
	q := CompletionQuery{Title: "Portal 2"}
	c.Completion(ctx, q)
	n := fn.count()
	// A differently written title is the same game.
	for _, title := range []string{"Portal 2", "PORTAL® 2", "Portal 2 (2011)"} {
		clk.Advance(time.Hour)
		if got := c.Completion(ctx, CompletionQuery{Title: title, Year: 2011}); got.HLTBID != 7231 || got.State != StateOK {
			t.Fatalf("%q: %+v", title, got)
		}
	}
	if fn.count() != n {
		t.Fatalf("%d extra requests", fn.count()-n)
	}
	cached, ok := c.CachedCompletion(q)
	if !ok || cached.HLTBID != 7231 || cached.State != StateOK {
		t.Fatalf("cached %+v %v", cached, ok)
	}
	if _, ok := c.CachedCompletion(CompletionQuery{Title: "Something Else"}); ok {
		t.Fatal("nothing is cached for another game")
	}
	clk.Advance(CompletionTTL)
	if got, _ := c.CachedCompletion(q); got.State != StateStale {
		t.Fatalf("expired cache is labelled %q", got.State)
	}
}

func TestExpiredCompletionIsFetchedAgain(t *testing.T) {
	c, fn, clk := newTestClient(t, hltbRouter(t, "hltb_search_portal2.json", ""))
	ctx := context.Background()
	c.Completion(ctx, CompletionQuery{Title: "Portal 2"})
	n := fn.countPath("/api/search/site")
	clk.Advance(CompletionTTL + time.Second)
	if got := c.Completion(ctx, CompletionQuery{Title: "Portal 2"}); got.State != StateOK {
		t.Fatalf("%+v", got)
	}
	if fn.countPath("/api/search/site") <= n {
		t.Fatal("no new search after the seven days")
	}
}

func TestMatchTitleIsConservative(t *testing.T) {
	hits := []hltbHit{
		{ID: 1, Title: "Portal", Type: "game", Year: 2007},
		{ID: 2, Title: "Portal 2", Type: "game", Year: 2011},
		{ID: 3, Title: "Portal 2: Peer Review", Type: "dlc", Year: 2011},
		{ID: 4, Title: "Portal 2: Sixense Perceptual Pack", Type: "game", Year: 2013},
		{ID: 5, Title: "Portal 2: Confinement", Type: "mod", Year: 2024},
		{ID: 6, Title: "Portal Reloaded", Type: "mod", Year: 2022},
	}
	cases := []struct {
		name  string
		title string
		year  int
		want  int // 0: no match
	}{
		{"exact", "Portal 2", 0, 2},
		{"the first game is not its sequel", "Portal", 0, 1},
		{"case and trademark signs", "PORTAL® 2™", 0, 2},
		{"punctuation and spacing", "portal  2!", 0, 2},
		{"trailing year in parentheses", "Portal 2 (2011)", 0, 2},
		{"a DLC by its exact name", "Portal 2: Peer Review", 0, 3},
		{"a subtitle is not the base game", "Portal 2: Peer Review Edition", 0, 0},
		{"remaster is a different game", "Portal 2 Remastered", 0, 0},
		{"an edition is no match", "Portal 2 Deluxe Edition", 0, 0},
		{"sequel number differs", "Portal 3", 0, 0},
		{"roman numeral is not a digit", "Portal II", 0, 0},
		{"missing sequel number", "Portal 2 2", 0, 0},
		{"a prefix is no match", "Port", 0, 0},
		{"a longer title is no match", "Portal 2 Game of the Year", 0, 0},
		{"a mod never matches by name", "Portal Reloaded", 0, 0},
		{"empty title", "  ", 0, 0},
		{"punctuation only", "!!!", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := matchTitle(hits, tc.title, tc.year)
			if tc.want == 0 {
				if ok {
					t.Fatalf("matched %+v", got)
				}
				return
			}
			if !ok || got.ID != tc.want {
				t.Fatalf("got %+v %v, want %d", got, ok, tc.want)
			}
		})
	}
}

func TestMatchTitleUsesYearOnlyToTellEqualTitlesApart(t *testing.T) {
	hits := []hltbHit{
		{ID: 35859, Title: "Ratchet & Clank", Type: "game", Year: 2016},
		{ID: 7585, Title: "Ratchet & Clank", Type: "game", Year: 2002},
		{ID: 79776, Title: "Ratchet & Clank: Rift Apart", Type: "game", Year: 2021},
	}
	cases := []struct {
		name  string
		title string
		year  int
		want  int
	}{
		{"two matches and no year", "Ratchet & Clank", 0, 0},
		{"year picks the remake", "Ratchet & Clank", 2016, 35859},
		{"year picks the original", "Ratchet & Clank", 2002, 7585},
		{"and for &", "Ratchet and Clank", 2016, 35859},
		{"a year one off still picks one", "Ratchet & Clank", 2017, 35859},
		{"a year near neither is no match", "Ratchet & Clank", 2010, 0},
		{"a year between two is ambiguous", "Ratchet & Clank", 2009, 0},
		{"the title's own year is used", "Ratchet & Clank (2016)", 0, 35859},
		{"the given year wins over the title's", "Ratchet & Clank (2002)", 2016, 35859},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := matchTitle(hits, tc.title, tc.year)
			if (tc.want == 0) == ok || (ok && got.ID != tc.want) {
				t.Fatalf("got %+v %v, want %d", got, ok, tc.want)
			}
		})
	}
	// One exact title is accepted whatever the year: Steam's release year is
	// often not HowLongToBeat's first release.
	if got, ok := matchTitle(hits[:1], "Ratchet & Clank", 2024); !ok || got.ID != 35859 {
		t.Fatalf("single exact match with another year: %+v %v", got, ok)
	}
}

func TestCompletionPicksTheRemakeByYearThroughTheSearch(t *testing.T) {
	c, _, _ := newTestClient(t, hltbRouter(t, "hltb_search_ratchet.json", ""))
	ctx := context.Background()
	if got := c.Completion(ctx, CompletionQuery{Title: "Ratchet & Clank", Year: 2016}); got.HLTBID != 35859 || got.Main != 570 {
		t.Fatalf("%+v", got)
	}
	got := c.Completion(ctx, CompletionQuery{Title: "Ratchet & Clank"})
	if got.HLTBID != 0 || got.State != StateUnavailable || got.Main != 0 {
		t.Fatalf("ambiguous without a year must not match: %+v", got)
	}
}

func TestCompletionWithoutAMatchIsUnavailableWithASearchLink(t *testing.T) {
	c, fn, _ := newTestClient(t, hltbRouter(t, "hltb_search_portal2.json", ""))
	ctx := context.Background()
	got := c.Completion(ctx, CompletionQuery{Title: "Portal 2 Deluxe Edition"})
	if got.HLTBID != 0 || got.State != StateUnavailable || got.Main != 0 || got.MainExtras != 0 || got.Completionist != 0 || got.Error == "" {
		t.Fatalf("%+v", got)
	}
	u, err := url.Parse(got.URL)
	if err != nil || u.Host != "howlongtobeat.com" || u.Query().Get("q") != "Portal 2 Deluxe Edition" {
		t.Fatalf("link %q", got.URL)
	}
	n := fn.count()
	again := c.Completion(ctx, CompletionQuery{Title: "Portal 2 Deluxe Edition"})
	if again.State != StateUnavailable || fn.count() != n {
		t.Fatalf("a remembered no-match asks again: %+v, %d requests", again, fn.count()-n)
	}
	if cached, ok := c.CachedCompletion(CompletionQuery{Title: "Portal 2 Deluxe Edition"}); !ok || cached.State != StateUnavailable {
		t.Fatalf("%+v %v", cached, ok)
	}
}

func TestCompletionForAGameHowLongToBeatHasNoResultsFor(t *testing.T) {
	c, _, _ := newTestClient(t, hltbRouter(t, "hltb_search_empty.json", ""))
	got := c.Completion(context.Background(), CompletionQuery{Title: "Totally Unknown Game"})
	if got.State != StateUnavailable || got.HLTBID != 0 || !strings.Contains(got.URL, "Totally+Unknown+Game") {
		t.Fatalf("%+v", got)
	}
}

func TestCompletionWithoutATitleMakesNoRequest(t *testing.T) {
	c, fn, _ := newTestClient(t, hltbRouter(t, "hltb_search_portal2.json", ""))
	got := c.Completion(context.Background(), CompletionQuery{Title: " ™ "})
	if got.State != StateUnavailable || fn.count() != 0 {
		t.Fatalf("%+v after %d requests", got, fn.count())
	}
}

func TestChosenMatchIsReadDirectlyAndMarkedCorrected(t *testing.T) {
	c, fn, _ := newTestClient(t, hltbRouter(t, "hltb_search_ratchet.json", "hltb_game_7231.html"))
	got := c.Completion(context.Background(), CompletionQuery{Title: "Anything at all", HLTBID: 7231})
	want := Completion{HLTBID: 7231, Title: "Portal 2", Main: 515, MainExtras: 826, Completionist: 1376,
		URL: "https://howlongtobeat.com/game/7231", Corrected: true, FetchedAt: got.FetchedAt, State: StateOK}
	if got != want {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
	if fn.count() != 1 || fn.last().Path != "/game/7231" || fn.countPath("search") != 0 {
		t.Fatalf("expected one game-page request, got %d (%s)", fn.count(), fn.last().URL)
	}
	if cached, ok := c.CachedCompletion(CompletionQuery{HLTBID: 7231}); !ok || cached.HLTBID != 7231 || !cached.Corrected {
		t.Fatalf("%+v %v", cached, ok)
	}
}

func TestChosenMatchForAnUnknownGameIsUnavailable(t *testing.T) {
	c, _, _ := newTestClient(t, func(recorded) answer { return status(404) })
	got := c.Completion(context.Background(), CompletionQuery{HLTBID: 99999999})
	if got.State != StateUnavailable || got.HLTBID != 99999999 || !got.Corrected || got.URL != "https://howlongtobeat.com/game/99999999" {
		t.Fatalf("%+v", got)
	}
	// A game page that doesn't exist is not a failing host.
	if _, blocked := c.hosts[hostHLTB].blocked(c.now()); blocked {
		t.Fatal("a 404 paused the host")
	}
}

func TestGamePageForAnotherGameIsNotAccepted(t *testing.T) {
	c, _, _ := newTestClient(t, hltbRouter(t, "", "hltb_game_7231.html"))
	if got := c.Completion(context.Background(), CompletionQuery{HLTBID: 1234}); got.State != StateError || got.Main != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestChangedGamePageKeepsCachedTimesAsStale(t *testing.T) {
	page := "hltb_game_7231.html"
	c, fn, clk := newTestClient(t, func(r recorded) answer { return ok(fixture(t, page)) })
	ctx := context.Background()
	if got := c.Completion(ctx, CompletionQuery{HLTBID: 7231}); got.State != StateOK {
		t.Fatalf("%+v", got)
	}
	clk.Advance(CompletionTTL + time.Hour)
	page = "hltb_game_changed.html"
	got := c.Completion(ctx, CompletionQuery{HLTBID: 7231})
	if got.State != StateStale || got.Main != 515 || got.Error == "" || got.HLTBID != 7231 {
		t.Fatalf("%+v", got)
	}
	// A changed format pauses the host instead of asking on every call.
	n := fn.count()
	if again := c.Completion(ctx, CompletionQuery{HLTBID: 7231}); again.State != StateStale || fn.count() != n {
		t.Fatalf("%+v, %d requests", again, fn.count()-n)
	}
}

func TestChangedGamePageWithNothingCachedIsAnErrorWithALink(t *testing.T) {
	c, _, _ := newTestClient(t, func(recorded) answer { return ok(fixture(t, "hltb_game_changed.html")) })
	got := c.Completion(context.Background(), CompletionQuery{HLTBID: 7231})
	if got.State != StateError || got.Main != 0 || got.MainExtras != 0 || got.Completionist != 0 || got.URL != "https://howlongtobeat.com/game/7231" {
		t.Fatalf("%+v", got)
	}
}

func TestBlockedSearchKeepsCachedTimesStaleAndLeavesTheSiteAlone(t *testing.T) {
	blocked := false
	router := hltbRouter(t, "hltb_search_portal2.json", "")
	c, fn, clk := newTestClient(t, func(r recorded) answer {
		if blocked {
			return status(403, "Content-Type", "application/json")
		}
		return router(r)
	})
	ctx := context.Background()
	q := CompletionQuery{Title: "Portal 2"}
	c.Completion(ctx, q)
	clk.Advance(CompletionTTL + time.Hour)
	blocked = true
	got := c.Completion(ctx, q)
	if got.State != StateStale || got.Main != 515 || !strings.Contains(got.Error, "403") {
		t.Fatalf("%+v", got)
	}
	n := fn.count()
	c.Completion(ctx, q)
	c.Completion(ctx, CompletionQuery{Title: "Other Game"})
	if fn.count() != n {
		t.Fatalf("%d requests while the site is paused", fn.count()-n)
	}
	other := c.Completion(ctx, CompletionQuery{Title: "Other Game"})
	if other.State != StateUnavailable || other.URL == "" || other.Main != 0 {
		t.Fatalf("%+v", other)
	}
}

func TestRefusedSearchTokenIsRenewedOnce(t *testing.T) {
	var inits atomic.Int32
	search := fixture(t, "hltb_search_portal2.json")
	c, fn, _ := newTestClient(t, func(r recorded) answer {
		switch {
		case strings.HasSuffix(r.Path, "/init"):
			if inits.Add(1) == 1 {
				return ok([]byte(`{"token":"old"}`))
			}
			return ok([]byte(`{"token":"new"}`))
		case strings.HasSuffix(r.Path, "/api/search/site"):
			if r.Header.Get("x-auth-token") == "old" {
				return status(403)
			}
			return ok(search)
		}
		return status(404)
	})
	got := c.Completion(context.Background(), CompletionQuery{Title: "Portal 2"})
	if got.State != StateOK || got.HLTBID != 7231 {
		t.Fatalf("%+v", got)
	}
	if inits.Load() != 2 || fn.countPath("/api/search/site/init") != 2 {
		t.Fatalf("%d token requests", inits.Load())
	}
	// The renewed token is kept for the next search.
	c.Completion(context.Background(), CompletionQuery{Title: "Portal 2 Deluxe"})
	if inits.Load() != 2 {
		t.Fatalf("asked for a token again: %d", inits.Load())
	}
	if _, blocked := c.hosts[hostHLTB].blocked(c.now()); blocked {
		t.Fatal("one refused token paused the site")
	}
}

func TestSearchTokenIsRenewedWhenOld(t *testing.T) {
	c, fn, clk := newTestClient(t, hltbRouter(t, "hltb_search_portal2.json", ""))
	ctx := context.Background()
	c.Completion(ctx, CompletionQuery{Title: "Portal 2"})
	clk.Advance(hltbTokenTTL + time.Minute)
	c.Completion(ctx, CompletionQuery{Title: "Portal 2 Remake"})
	if got := fn.countPath("/api/search/site/init"); got != 2 {
		t.Fatalf("%d token requests", got)
	}
}

func TestChangedSearchFormatIsAnErrorNotAGuess(t *testing.T) {
	for name, body := range map[string]string{
		"html":          "<html>Just a moment...</html>",
		"no data":       `{"count":1}`,
		"wrong types":   `{"data":[{"game_id":"x","game_name":5}]}`,
		"data is array": `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			c, _, _ := newTestClient(t, func(r recorded) answer {
				if strings.HasSuffix(r.Path, "/init") {
					return ok(fixture(t, "hltb_init.json"))
				}
				return ok([]byte(body))
			})
			got := c.Completion(context.Background(), CompletionQuery{Title: "Portal 2"})
			if got.State != StateError || got.HLTBID != 0 || got.Main != 0 || got.URL == "" {
				t.Fatalf("%+v", got)
			}
		})
	}
}

func TestMissingSearchTokenIsAnError(t *testing.T) {
	c, _, _ := newTestClient(t, func(recorded) answer { return ok([]byte(`{"nope":1}`)) })
	if got := c.Completion(context.Background(), CompletionQuery{Title: "Portal 2"}); got.State != StateError {
		t.Fatalf("%+v", got)
	}
}

func TestCandidatesListSearchResultsToChooseFrom(t *testing.T) {
	c, fn, _ := newTestClient(t, hltbRouter(t, "hltb_search_portal2.json", ""))
	ctx := context.Background()
	list, err := c.Candidates(ctx, "Portal 2")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 || list[0] != (Candidate{HLTBID: 7231, Title: "Portal 2", Year: 2011, Type: "game", Main: 515, MainExtras: 826, Completionist: 1376, URL: "https://howlongtobeat.com/game/7231"}) {
		t.Fatalf("%+v", list)
	}
	// The kind of entry comes along, so DLC and mods can be told from the game.
	kinds := map[string]string{}
	for _, l := range list {
		kinds[l.Title] = l.Type
	}
	if kinds["Portal 2: Peer Review"] != "dlc" || kinds["Portal 2: Confinement"] != "mod" {
		t.Fatalf("kinds: %v", kinds)
	}
	n := fn.count()
	if again, err := c.Candidates(ctx, "portal 2"); err != nil || len(again) != 4 || fn.count() != n {
		t.Fatalf("a repeated search should come from the cache: %v, %d new requests", err, fn.count()-n)
	}
	if _, err := c.Candidates(ctx, "   "); err == nil {
		t.Fatal("an empty title is an error")
	}
}

func TestCandidatesNeverNilAndErrorWhenSiteFailsWithoutCache(t *testing.T) {
	c, _, _ := newTestClient(t, hltbRouter(t, "hltb_search_empty.json", ""))
	list, err := c.Candidates(context.Background(), "Nothing Here")
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("%v %v", list, err)
	}
	down, _, _ := newTestClient(t, func(recorded) answer { return status(500) })
	if list, err := down.Candidates(context.Background(), "Portal 2"); err == nil || list != nil {
		t.Fatalf("%v %v", list, err)
	}
}

func TestCandidatesFallBackToOldResultsWhenTheSiteFails(t *testing.T) {
	down := false
	router := hltbRouter(t, "hltb_search_portal2.json", "")
	c, _, clk := newTestClient(t, func(r recorded) answer {
		if down {
			return status(500)
		}
		return router(r)
	})
	ctx := context.Background()
	c.Candidates(ctx, "Portal 2")
	clk.Advance(hltbSearchTTL + time.Hour)
	down = true
	if list, err := c.Candidates(ctx, "Portal 2"); err != nil || len(list) != 4 {
		t.Fatalf("%v %v", list, err)
	}
}

func TestOldSearchResultsAreNeverMatchedAutomatically(t *testing.T) {
	down := false
	router := hltbRouter(t, "hltb_search_portal2.json", "")
	c, _, clk := newTestClient(t, func(r recorded) answer {
		if down {
			return status(500)
		}
		return router(r)
	})
	ctx := context.Background()
	c.Candidates(ctx, "Portal 2")
	clk.Advance(hltbSearchTTL + time.Hour)
	down = true
	got := c.Completion(ctx, CompletionQuery{Title: "Portal 2"})
	if got.State != StateError || got.HLTBID != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestMinutesRoundToTheNearestMinute(t *testing.T) {
	for in, want := range map[int]int{-5: 0, 0: 0, 29: 0, 30: 1, 89: 1, 90: 2, 30883: 515, 3600: 60} {
		if got := minutes(in); got != want {
			t.Errorf("minutes(%d) = %d, want %d", in, got, want)
		}
	}
}
