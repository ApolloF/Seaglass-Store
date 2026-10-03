package discovery

import (
	"cmp"
	"slices"
	"strings"
)

// maxBecause is how many played or wishlisted games a recommendation names.
const maxBecause = 3

type scored struct {
	game    GameSummary
	score   float64
	because []contribution
}

// contribution is what one signal added to one game's score.
type contribution struct {
	signal Signal
	shared []string
	score  float64
}

// Recommend picks source-backed games that share genres with the signals.
// A signal's weight is split over its genres, so a game with many genres
// doesn't outweigh one with a few. Installed games (or any game exclude
// names) and wishlisted ones are never offered. Without a signal that
// overlaps any candidate, the chart's order among the same games is used
// (BasisPopular); without a chart there is nothing to recommend
// (BasisNone). The order is deterministic.
func Recommend(games []GameSummary, signals []Signal, exclude func(GameSummary) bool, limit int) ([]Recommendation, string) {
	if limit <= 0 {
		limit = ShelfSize
	}
	var candidates []GameSummary
	for _, g := range games {
		if g.SourceBacked && g.Installed == nil && !g.Wishlisted && (exclude == nil || !exclude(g)) {
			candidates = append(candidates, g)
		}
	}
	var hits []scored
	for _, g := range candidates {
		if s, ok := scoreGame(g, signals); ok {
			hits = append(hits, s)
		}
	}
	if len(hits) == 0 {
		return popularFallback(candidates, limit)
	}
	slices.SortFunc(hits, func(a, b scored) int {
		return cmp.Or(cmp.Compare(b.score, a.score), compareGames(a.game, b.game))
	})
	out := []Recommendation{}
	var played, wished bool
	for _, h := range hits[:min(len(hits), limit)] {
		r := Recommendation{Game: h.game, Because: []string{}, Genres: []string{}}
		slices.SortStableFunc(h.because, func(a, b contribution) int {
			return cmp.Or(cmp.Compare(b.score, a.score), strings.Compare(a.signal.Title, b.signal.Title))
		})
		for i, c := range h.because {
			if i < maxBecause {
				r.Because = append(r.Because, c.signal.Title)
			}
			for _, genre := range c.shared {
				if !slices.ContainsFunc(r.Genres, func(x string) bool { return strings.EqualFold(x, genre) }) {
					r.Genres = append(r.Genres, genre)
				}
			}
			switch c.signal.Kind {
			case BasisPlayed:
				played = true
			case BasisWishlist:
				wished = true
			}
		}
		out = append(out, r)
	}
	switch {
	case played && wished:
		return out, BasisBoth
	case wished:
		return out, BasisWishlist
	}
	return out, BasisPlayed
}

// scoreGame adds up what each signal with a shared genre contributes.
func scoreGame(g GameSummary, signals []Signal) (scored, bool) {
	s := scored{game: g}
	for _, sig := range signals {
		if len(sig.Genres) == 0 || sig.Weight <= 0 {
			continue
		}
		var shared []string
		for _, genre := range sig.Genres {
			if slices.ContainsFunc(g.Genres, func(x string) bool { return strings.EqualFold(x, genre) }) {
				shared = append(shared, genre)
			}
		}
		if len(shared) == 0 {
			continue
		}
		part := sig.Weight * float64(len(shared)) / float64(len(sig.Genres))
		s.score += part
		s.because = append(s.because, contribution{signal: sig, shared: shared, score: part})
	}
	return s, len(s.because) > 0
}

// popularFallback is the chart's order among the candidates.
func popularFallback(candidates []GameSummary, limit int) ([]Recommendation, string) {
	var ranked []GameSummary
	for _, g := range candidates {
		if g.PopularRank > 0 {
			ranked = append(ranked, g)
		}
	}
	if len(ranked) == 0 {
		return []Recommendation{}, BasisNone
	}
	slices.SortFunc(ranked, compareGames)
	out := []Recommendation{}
	for _, g := range ranked[:min(len(ranked), limit)] {
		out = append(out, Recommendation{Game: g, Because: []string{}, Genres: []string{}})
	}
	return out, BasisPopular
}

// compareGames orders by chart place (ranked first), then the newest
// source publication, then title.
func compareGames(a, b GameSummary) int {
	switch {
	case a.PopularRank > 0 && b.PopularRank > 0:
		if c := cmp.Compare(a.PopularRank, b.PopularRank); c != 0 {
			return c
		}
	case a.PopularRank > 0:
		return -1
	case b.PopularRank > 0:
		return 1
	}
	return cmp.Or(cmp.Compare(b.PublishedAt, a.PublishedAt), strings.Compare(a.Title, b.Title), strings.Compare(a.Key, b.Key))
}
