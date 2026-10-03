package app

import (
	"slices"
	"strconv"
	"time"

	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/scan"
	"github.com/ApolloF/Seaglass/internal/store/discovery"
	"github.com/ApolloF/Seaglass/internal/store/wishlist"
)

const (
	// recentPlayWindow and maxPlayedSignals bound which played games steer
	// recommendations.
	recentPlayWindow = 60 * 24 * time.Hour
	maxPlayedSignals = 10
	// steamTrust is the confidence from which a library game's Steam
	// identity is trusted without the person confirming it.
	steamTrust = 85
	// minSignalWeight keeps the oldest play or wishlist entry in the mix.
	minSignalWeight = 0.2
)

// recommendations builds the recommender's input from the library and the
// wishlist, then recommends from the index.
func (c *Core) recommendations(view *discovery.View, annotate discovery.Annotate) ([]discovery.Recommendation, string) {
	var games []library.Game
	if c.Lib != nil {
		games = c.Lib.Games()
	}
	var wished []wishlist.Entry
	if c.wishlist != nil {
		wished = c.wishlist.store.List()
	}
	signals := recommendSignals(games, wished, c.art.genres, time.Now())
	return view.Recommend(annotate, signals, installedInLibrary(games), discovery.ShelfSize)
}

// recommendSignals are the recently played library games (newest first,
// at most ten) and the wishlisted games whose genres are known, each
// weighed by how recent it is.
func recommendSignals(games []library.Game, wished []wishlist.Entry, genres func(key string) []string, now time.Time) []discovery.Signal {
	type played struct {
		title  string
		genres []string
		at     time.Time
	}
	var recent []played
	for _, g := range games {
		at := time.Unix(max(g.LastPlayed, g.StoreLastPlayed), 0)
		if max(g.LastPlayed, g.StoreLastPlayed) == 0 || now.Sub(at) > recentPlayWindow || g.Meta == nil || len(g.Meta.Genres) == 0 {
			continue
		}
		recent = append(recent, played{title: libraryTitle(g), genres: g.Meta.Genres, at: at})
	}
	slices.SortFunc(recent, func(a, b played) int { return b.at.Compare(a.at) })
	out := []discovery.Signal{}
	for _, p := range recent[:min(len(recent), maxPlayedSignals)] {
		age := float64(now.Sub(p.at)) / float64(recentPlayWindow)
		out = append(out, discovery.Signal{Title: p.title, Genres: p.genres, Kind: discovery.BasisPlayed, Weight: max(minSignalWeight, 1-age)})
	}
	newest := slices.Clone(wished)
	slices.SortFunc(newest, func(a, b wishlist.Entry) int { return b.AddedAt.Compare(a.AddedAt) })
	for i, e := range newest {
		if g := genres(e.Key); len(g) > 0 {
			out = append(out, discovery.Signal{Title: e.Title, Genres: g, Kind: discovery.BasisWishlist, Weight: max(minSignalWeight, 1-0.1*float64(i))})
		}
	}
	return out
}

// installedInLibrary says whether a Store game is already installed in the
// library: by Steam identity when it is trusted, else by normalized title.
func installedInLibrary(games []library.Game) func(discovery.GameSummary) bool {
	keys, titles := map[string]bool{}, map[string]bool{}
	for _, g := range games {
		if !g.Installed {
			continue
		}
		if g.SteamAppID > 0 && (g.Confirmed || g.Confidence >= steamTrust) {
			keys["steam:"+strconv.Itoa(g.SteamAppID)] = true
		}
		for _, t := range []string{g.Title, g.CustomTitle} {
			if n := scan.Normalize(t); n != "" {
				titles[n] = true
			}
		}
	}
	return func(s discovery.GameSummary) bool {
		return keys[s.Key] || titles[scan.Normalize(s.Title)]
	}
}

func libraryTitle(g library.Game) string {
	if g.CustomTitle != "" {
		return g.CustomTitle
	}
	return g.Title
}
