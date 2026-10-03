package discovery

import (
	"slices"
	"testing"
)

func game(title string, genres ...string) GameSummary {
	return GameSummary{Key: "title:" + title, Title: title, SourceBacked: true, Genres: genres}
}

func recTitles(rs []Recommendation) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Game.Title)
	}
	return out
}

func TestRecommendRanksByGenreOverlapAndWeight(t *testing.T) {
	games := []GameSummary{game("Slasher", "Action", "RPG"), game("Farm", "Simulation"), game("Brawler", "Action"), game("Quest", "RPG")}
	signals := []Signal{
		{Title: "Old Dungeon", Genres: []string{"RPG"}, Kind: BasisPlayed, Weight: 0.4},
		{Title: "Hollow Tide", Genres: []string{"Action", "Adventure"}, Kind: BasisPlayed, Weight: 1},
	}
	got, basis := Recommend(games, signals, nil, 10)
	if want := []string{"Slasher", "Brawler", "Quest"}; !slices.Equal(recTitles(got), want) {
		t.Fatalf("order: %v, want %v", recTitles(got), want)
	}
	if basis != BasisPlayed {
		t.Errorf("basis %q", basis)
	}
}

func TestRecommendOneSignalWithManyGenresDoesNotDominate(t *testing.T) {
	games := []GameSummary{game("Broad", "A", "B", "C", "D"), game("Narrow", "X")}
	signals := []Signal{
		{Title: "Wide", Genres: []string{"A", "B", "C", "D"}, Kind: BasisPlayed, Weight: 1},
		{Title: "Tight", Genres: []string{"X"}, Kind: BasisPlayed, Weight: 1},
	}
	got, _ := Recommend(games, signals, nil, 10)
	if got[0].Game.Title != "Broad" || len(got) != 2 {
		t.Fatalf("got %v", recTitles(got))
	}
	// Both fully overlap their signal, so each earns the same weight; ties follow title.
	if got[1].Game.Title != "Narrow" {
		t.Errorf("tie order: %v", recTitles(got))
	}
}

func TestRecommendLeavesOutInstalledExcludedAndWishlisted(t *testing.T) {
	installed, wished, excluded, open := game("Installed", "RPG"), game("Wished", "RPG"), game("Excluded", "RPG"), game("Open", "RPG")
	installed.Installed = &Installed{}
	wished.Wishlisted = true
	signals := []Signal{{Title: "Played", Genres: []string{"RPG"}, Kind: BasisPlayed, Weight: 1}}
	got, _ := Recommend([]GameSummary{installed, wished, excluded, open}, signals, func(g GameSummary) bool { return g.Title == "Excluded" }, 10)
	if !slices.Equal(recTitles(got), []string{"Open"}) {
		t.Fatalf("got %v", recTitles(got))
	}
}

func TestRecommendLeavesOutGamesWithoutASource(t *testing.T) {
	steamOnly := game("Steam only", "RPG")
	steamOnly.SourceBacked = false
	got, _ := Recommend([]GameSummary{steamOnly}, []Signal{{Title: "P", Genres: []string{"RPG"}, Kind: BasisPlayed, Weight: 1}}, nil, 10)
	if len(got) != 0 {
		t.Errorf("got %v", recTitles(got))
	}
}

func TestRecommendNamesUpToThreeSignalsAndTheSharedGenres(t *testing.T) {
	games := []GameSummary{game("Target", "Action", "RPG", "Indie")}
	signals := []Signal{
		{Title: "Four", Genres: []string{"Action"}, Kind: BasisPlayed, Weight: 0.1},
		{Title: "One", Genres: []string{"Action", "RPG"}, Kind: BasisPlayed, Weight: 1},
		{Title: "Two", Genres: []string{"action"}, Kind: BasisPlayed, Weight: 0.8},
		{Title: "Three", Genres: []string{"Indie"}, Kind: BasisPlayed, Weight: 0.5},
		{Title: "Unrelated", Genres: []string{"Racing"}, Kind: BasisPlayed, Weight: 5},
	}
	got, _ := Recommend(games, signals, nil, 10)
	r := got[0]
	if want := []string{"One", "Two", "Three"}; !slices.Equal(r.Because, want) {
		t.Errorf("because %v, want %v", r.Because, want)
	}
	if want := []string{"Action", "RPG", "Indie"}; !slices.Equal(r.Genres, want) {
		t.Errorf("genres %v, want %v", r.Genres, want)
	}
}

func TestRecommendBasisNamesTheSignalKinds(t *testing.T) {
	games := []GameSummary{game("G", "RPG")}
	played := Signal{Title: "P", Genres: []string{"RPG"}, Kind: BasisPlayed, Weight: 1}
	wished := Signal{Title: "W", Genres: []string{"RPG"}, Kind: BasisWishlist, Weight: 1}
	for name, c := range map[string]struct {
		signals []Signal
		want    string
	}{
		"played":   {[]Signal{played}, BasisPlayed},
		"wishlist": {[]Signal{wished}, BasisWishlist},
		"both":     {[]Signal{played, wished}, BasisBoth},
	} {
		if _, basis := Recommend(games, c.signals, nil, 10); basis != c.want {
			t.Errorf("%s: basis %q, want %q", name, basis, c.want)
		}
	}
}

func TestRecommendFallsBackToThePopularChartWithoutOverlap(t *testing.T) {
	a, b, c := game("Alpha", "RPG"), game("Beta", "RPG"), game("Gamma", "RPG")
	a.PopularRank, b.PopularRank = 5, 2
	signals := []Signal{{Title: "P", Genres: []string{"Racing"}, Kind: BasisPlayed, Weight: 1}}
	for _, sig := range [][]Signal{signals, nil, {{Title: "No genres", Kind: BasisPlayed, Weight: 1}}} {
		got, basis := Recommend([]GameSummary{a, b, c}, sig, nil, 10)
		if basis != BasisPopular || !slices.Equal(recTitles(got), []string{"Beta", "Alpha"}) {
			t.Fatalf("basis %q, games %v", basis, recTitles(got))
		}
		if len(got[0].Because) != 0 || len(got[0].Genres) != 0 {
			t.Errorf("popular recommendation explains itself: %+v", got[0])
		}
	}
}

func TestRecommendNothingWithoutAChartOrOverlap(t *testing.T) {
	got, basis := Recommend([]GameSummary{game("Alpha", "RPG")}, nil, nil, 10)
	if basis != BasisNone || got == nil || len(got) != 0 {
		t.Errorf("basis %q, games %v", basis, got)
	}
}

func TestRecommendOrderIsDeterministicAndLimited(t *testing.T) {
	old, fresh, unranked := game("Old", "RPG"), game("Fresh", "RPG"), game("Zed", "RPG")
	old.PublishedAt, fresh.PublishedAt = 10, 20
	ranked := game("Ranked", "RPG")
	ranked.PopularRank = 40
	signals := []Signal{{Title: "P", Genres: []string{"RPG"}, Kind: BasisPlayed, Weight: 1}}
	got, _ := Recommend([]GameSummary{unranked, old, fresh, ranked}, signals, nil, 3)
	if want := []string{"Ranked", "Fresh", "Old"}; !slices.Equal(recTitles(got), want) {
		t.Errorf("order %v, want %v", recTitles(got), want)
	}
}
