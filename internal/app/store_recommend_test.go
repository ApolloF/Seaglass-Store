package app

import (
	"errors"
	"net/http"
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

func TestAnnotatorReadsCompletionAndReleaseDateWithoutFetching(t *testing.T) {
	c, s := discoveryCore(t)
	if err := c.discovery.refresh(); err != nil {
		t.Fatal(err)
	}
	wire := &countingTransport{}
	c.enrich.client = enrich.New(enrich.Options{Dir: t.TempDir(), Transport: wire})
	home, err := s.StoreHome()
	if err != nil || len(home.New) == 0 {
		t.Fatalf("home: %d games, %v", len(home.New), err)
	}
	for _, g := range home.New {
		if g.CompletionMain != 0 || g.ReleaseDate != "" {
			t.Errorf("%s: completion %d, release %q while nothing is cached", g.Title, g.CompletionMain, g.ReleaseDate)
		}
	}
	if n := wire.requests.Load(); n != 0 {
		t.Errorf("%d requests while annotating cards", n)
	}
}
