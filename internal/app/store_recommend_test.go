package app

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/store/discovery"
	"github.com/ApolloF/Seaglass/internal/store/enrich"
	"github.com/ApolloF/Seaglass/internal/store/wishlist"
)

func playedGame(title string, ago time.Duration, now time.Time, genres ...string) library.Game {
	return library.Game{Title: title, LastPlayed: now.Add(-ago).Unix(), Meta: &library.Meta{Genres: genres}}
}

func TestRecommendSignalsWeighRecentPlayMoreAndSkipOldOrUnknownGenres(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	day := 24 * time.Hour
	games := []library.Game{
		playedGame("Old", 90*day, now, "RPG"),
		playedGame("Week", 7*day, now, "Action"),
		playedGame("Yesterday", day, now, "RPG"),
		playedGame("No genres", day, now),
		{Title: "Never played", Meta: &library.Meta{Genres: []string{"RPG"}}},
	}
	got := recommendSignals(games, nil, func(string) []string { return nil }, now)
	if len(got) != 2 || got[0].Title != "Yesterday" || got[1].Title != "Week" {
		t.Fatalf("signals: %+v", got)
	}
	if got[0].Weight <= got[1].Weight || got[0].Kind != discovery.BasisPlayed {
		t.Errorf("weights: %v then %v", got[0].Weight, got[1].Weight)
	}
}

func TestRecommendSignalsUseSteamPlaytimeDateAndStayAtTen(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	var games []library.Game
	for i := range 12 {
		g := playedGame("Game", time.Duration(i+1)*time.Hour, now, "RPG")
		g.LastPlayed, g.StoreLastPlayed = 0, now.Add(-time.Duration(i+1)*time.Hour).Unix()
		games = append(games, g)
	}
	if got := recommendSignals(games, nil, func(string) []string { return nil }, now); len(got) != maxPlayedSignals {
		t.Errorf("%d signals", len(got))
	}
}

func TestRecommendSignalsTakeWishlistGenresFromStoreMetadata(t *testing.T) {
	now := time.Now()
	wished := []wishlist.Entry{
		{Key: "steam:1", Title: "Older", AddedAt: now.Add(-48 * time.Hour)},
		{Key: "steam:2", Title: "Newer", AddedAt: now.Add(-time.Hour)},
		{Key: "steam:3", Title: "Unknown genres", AddedAt: now},
	}
	genres := func(key string) []string {
		if key == "steam:3" {
			return nil
		}
		return []string{"Strategy"}
	}
	got := recommendSignals(nil, wished, genres, now)
	if len(got) != 2 || got[0].Title != "Newer" || got[1].Title != "Older" || got[0].Weight <= got[1].Weight || got[0].Kind != discovery.BasisWishlist {
		t.Fatalf("signals: %+v", got)
	}
}

func TestInstalledInLibraryMatchesTrustedSteamIdentityAndNormalizedTitle(t *testing.T) {
	games := []library.Game{
		{Title: "Hollow Tide", Installed: true, SteamAppID: 10, Confidence: 95},
		{Title: "Guess", Installed: true, SteamAppID: 20, Confidence: 40},
		{Title: "Café: Deluxe", Installed: true},
		{Title: "Owned only", Installed: false, SteamAppID: 30, Confidence: 100},
	}
	in := installedInLibrary(games)
	for _, c := range []struct {
		g    discovery.GameSummary
		want bool
	}{
		{discovery.GameSummary{Key: "steam:10", Title: "Renamed"}, true},
		{discovery.GameSummary{Key: "steam:20", Title: "Other"}, false},
		{discovery.GameSummary{Key: "title:x", Title: "Hollow  Tide™"}, true},
		{discovery.GameSummary{Key: "title:cafe", Title: "cafe deluxe"}, false},
		{discovery.GameSummary{Key: "title:café", Title: "CAFÉ Deluxe"}, true},
		{discovery.GameSummary{Key: "steam:30", Title: "Owned only"}, false},
	} {
		if got := in(c.g); got != c.want {
			t.Errorf("%+v: %v, want %v", c.g, got, c.want)
		}
	}
}

// countingTransport counts requests other than the Steam chart, which the
// Popular shelf refreshes in the background on its own.
type countingTransport struct{ requests atomic.Int32 }

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if !strings.Contains(r.URL.Path, "GetMostPlayedGames") {
		c.requests.Add(1)
	}
	return nil, errors.New("no network in this test")
}

// waitForChartRefresh blocks until the background chart fetch that a Store
// home load starts has finished, so a test can swap the enrichment client
// without racing that goroutine's read of it.
func waitForChartRefresh(t *testing.T, e *enrichState) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		e.mu.Lock()
		running := e.chart
		e.mu.Unlock()
		if !running {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the chart refresh never finished")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestAnnotatorReadsCompletionAndReleaseDateWithoutFetching(t *testing.T) {
	c, s := discoveryCore(t)
	if err := c.discovery.refresh(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	wire := &countingTransport{}
	c.enrich.client = enrich.New(enrich.Options{Dir: dir, Transport: wire})
	c.art = &artState{known: map[string]*library.Meta{}}
	home, err := s.StoreHome()
	if err != nil || len(home.New) == 0 {
		t.Fatalf("home: %d games, %v", len(home.New), err)
	}
	for _, g := range home.New {
		if g.CompletionMain != 0 || g.ReleaseDate != "" || len(g.Genres) != 0 {
			t.Errorf("%s: completion %d, release %q, genres %v while nothing is cached", g.Title, g.CompletionMain, g.ReleaseDate, g.Genres)
		}
	}

	// Seed what a card may read: HowLongToBeat times in the disk cache and
	// the game's own metadata from Steam.
	hltb := &hltbFake{search: `{"data":[{"game_id":9,"game_name":"Ember Crown","game_type":"game","comp_main":36000,"comp_plus":72000,"comp_100":108000,"release_world":2024}]}`}
	seed := enrich.New(enrich.Options{Dir: dir, Transport: hltb})
	if got := seed.Completion(context.Background(), enrich.CompletionQuery{Title: "Ember Crown", Year: 2024}); got.Main != 600 {
		t.Fatalf("seeding the times: %+v", got)
	}
	if err := seed.Flush(); err != nil {
		t.Fatal(err)
	}
	waitForChartRefresh(t, c.enrich)
	c.enrich.client = enrich.New(enrich.Options{Dir: dir, Transport: wire})
	c.art.known["title:embercrown"] = &library.Meta{ReleaseYear: 2024, ReleaseDate: "12 Mar, 2024", Genres: []string{"Adventure"}}
	home, err = s.StoreHome()
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range home.New {
		if g.Key == "title:embercrown" {
			if g.CompletionMain != 600 || g.ReleaseDate != "12 Mar, 2024" || !slices.Equal(g.Genres, []string{"Adventure"}) {
				t.Errorf("a seeded game: completion %d, release %q, genres %v", g.CompletionMain, g.ReleaseDate, g.Genres)
			}
			continue
		}
		if g.CompletionMain != 0 || g.ReleaseDate != "" {
			t.Errorf("%s: completion %d, release %q, though a source date is all that is known", g.Title, g.CompletionMain, g.ReleaseDate)
		}
	}
	if n := wire.requests.Load(); n != 0 {
		t.Errorf("%d requests while annotating cards", n)
	}
}

func TestFeaturedLeavesOutGamesInstalledInTheLibrary(t *testing.T) {
	c, s := discoveryCore(t)
	if err := c.discovery.refresh(); err != nil {
		t.Fatal(err)
	}
	lib, err := library.Open(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	c.Lib = lib
	featured := func() []string {
		home, err := s.StoreHome()
		if err != nil {
			t.Fatal(err)
		}
		var titles []string
		for _, g := range home.Featured {
			titles = append(titles, g.Title)
		}
		return titles
	}
	if !slices.Contains(featured(), "Ember Crown") {
		t.Fatalf("Ember Crown isn't featured to begin with: %v", featured())
	}
	addGame(t, c, library.Found{Title: "Ember Crown", Source: "steam"})
	if got := featured(); slices.Contains(got, "Ember Crown") {
		t.Errorf("a game installed through another launcher is still featured: %v", got)
	}
}
