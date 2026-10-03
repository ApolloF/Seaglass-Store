package discovery

import (
	"cmp"
	"slices"
	"strings"

	"github.com/ApolloF/Seaglass/internal/scan"
)

// DefaultLimit is a page of games; MaxLimit the most one request returns.
const (
	DefaultLimit = 60
	MaxLimit     = 200
	ShelfSize    = 20
)

// View is the grouped index the queries read. Build a new one when the
// index changes; a View itself never changes.
type View struct {
	Games []*Game
	byKey map[string]*Game
}

// NewView indexes games by key.
func NewView(games []*Game) *View {
	v := &View{Games: games, byKey: make(map[string]*Game, len(games))}
	for _, g := range games {
		v.byKey[g.Key] = g
	}
	return v
}

// Game finds a game by key.
func (v *View) Game(key string) (*Game, bool) {
	g, ok := v.byKey[key]
	return g, ok
}

// Annotate adds what the app knows about a game to its summary: chart
// rank, review score, genres, installed state, wishlist. Unknown stays
// unknown.
type Annotate func(*GameSummary)

type card struct {
	g *Game
	s GameSummary
}

func (v *View) cards(annotate Annotate) []card {
	out := make([]card, 0, len(v.Games))
	for _, g := range v.Games {
		s := g.Summary()
		if !s.SourceBacked {
			continue
		}
		if annotate != nil {
			annotate(&s)
		}
		out = append(out, card{g, s})
	}
	return out
}

// Browse filters, sorts and pages the games.
func (v *View) Browse(q BrowseQuery, annotate Annotate) BrowsePage {
	all := v.cards(annotate)
	page := BrowsePage{Games: []GameSummary{}, Languages: facet(all, func(s GameSummary) []string { return s.Languages }), Genres: facet(all, func(s GameSummary) []string { return s.Genres })}
	words := searchWords(q.Text)
	var hits []card
	for _, c := range all {
		if !matches(c.g.search, words) {
			continue
		}
		if len(q.Sources) > 0 && !slices.ContainsFunc(c.s.Sources, func(s string) bool { return slices.Contains(q.Sources, s) }) {
			continue
		}
		switch q.Availability {
		case "installable":
			if !c.s.Installable {
				continue
			}
		case "unresolved":
			if c.s.Installable {
				continue
			}
		}
		switch q.Installed {
		case "installed":
			if c.s.Installed == nil {
				continue
			}
		case "not-installed":
			if c.s.Installed != nil {
				continue
			}
		}
		if q.Language != "" {
			if len(c.s.Languages) == 0 {
				page.Unknown++
				continue
			}
			if !slices.ContainsFunc(c.s.Languages, func(l string) bool { return strings.EqualFold(l, q.Language) }) {
				continue
			}
		}
		if q.Genre != "" {
			if len(c.s.Genres) == 0 {
				page.Unknown++
				continue
			}
			if !slices.ContainsFunc(c.s.Genres, func(l string) bool { return strings.EqualFold(l, q.Genre) }) {
				continue
			}
		}
		hits = append(hits, c)
	}
	sortCards(hits, q.Sort)
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	limit = min(limit, MaxLimit)
	off := min(max(q.Offset, 0), len(hits))
	end := min(off+limit, len(hits))
	for _, c := range hits[off:end] {
		page.Games = append(page.Games, c.s)
	}
	page.Total = len(hits)
	return page
}

// searchWords are the normalized words of a query; every one must match.
func searchWords(text string) []string {
	var out []string
	for _, w := range strings.Fields(text) {
		if n := scan.Normalize(w); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func matches(search string, words []string) bool {
	for _, w := range words {
		if !strings.Contains(search, w) {
			return false
		}
	}
	return true
}

// Matches reports whether a title matches a query, as Browse decides.
func Matches(title, text string) bool { return matches(scan.Normalize(title), searchWords(text)) }

func sortCards(cs []card, by string) {
	title := func(a, b card) int {
		return cmp.Or(strings.Compare(a.g.sortTitle, b.g.sortTitle), strings.Compare(a.g.Key, b.g.Key))
	}
	switch by {
	case SortPublished:
		slices.SortStableFunc(cs, func(a, b card) int { return cmp.Or(cmp.Compare(b.s.PublishedAt, a.s.PublishedAt), title(a, b)) })
	case SortPopular:
		// Unranked games follow the chart in title order: no rank is made up.
		slices.SortStableFunc(cs, func(a, b card) int {
			ra, rb := a.s.PopularRank, b.s.PopularRank
			switch {
			case ra > 0 && rb > 0:
				return cmp.Or(cmp.Compare(ra, rb), title(a, b))
			case ra > 0:
				return -1
			case rb > 0:
				return 1
			}
			return title(a, b)
		})
	case SortReviews:
		slices.SortStableFunc(cs, func(a, b card) int {
			ka, kb := a.s.ReviewTotal > 0, b.s.ReviewTotal > 0
			switch {
			case ka && kb:
				return cmp.Or(cmp.Compare(b.s.ReviewPercent, a.s.ReviewPercent), cmp.Compare(b.s.ReviewTotal, a.s.ReviewTotal), title(a, b))
			case ka:
				return -1
			case kb:
				return 1
			}
			return title(a, b)
		})
	default:
		slices.SortStableFunc(cs, title)
	}
}

// facet lists a field's values, most common first.
func facet(cs []card, field func(GameSummary) []string) []string {
	count := map[string]int{}
	for _, c := range cs {
		for _, v := range field(c.s) {
			count[v]++
		}
	}
	out := make([]string, 0, len(count))
	for v := range count {
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b string) int { return cmp.Or(count[b]-count[a], strings.Compare(a, b)) })
	return out
}

// Home builds the front page's shelves. popularState is the chart's state:
// without a chart the Popular shelf stays empty rather than showing
// another ranking. exclude (may be nil) leaves games out of Featured, such
// as those installed outside the Store.
func (v *View) Home(annotate Annotate, popularState string, status Status, exclude func(GameSummary) bool) Home {
	all := v.cards(annotate)
	h := Home{New: []GameSummary{}, Popular: []GameSummary{}, Updated: []GameSummary{}, Wishlist: []GameSummary{}, Featured: []GameSummary{},
		Recommended: []Recommendation{}, PopularState: popularState, Status: status}
	byNew := slices.Clone(all)
	sortCards(byNew, SortPublished)
	for _, c := range byNew {
		if c.s.PublishedAt == 0 || len(h.New) == ShelfSize {
			break
		}
		h.New = append(h.New, c.s)
	}
	if popularState == StateOK || popularState == StateStale {
		ranked := slices.DeleteFunc(slices.Clone(all), func(c card) bool { return c.s.PopularRank == 0 })
		sortCards(ranked, SortPopular)
		for _, c := range ranked[:min(len(ranked), ShelfSize)] {
			h.Popular = append(h.Popular, c.s)
		}
	}
	changed := slices.DeleteFunc(slices.Clone(all), func(c card) bool { return c.s.UpdatedAt == 0 })
	slices.SortStableFunc(changed, func(a, b card) int { return cmp.Compare(b.s.UpdatedAt, a.s.UpdatedAt) })
	for _, c := range changed[:min(len(changed), ShelfSize)] {
		h.Updated = append(h.Updated, c.s)
	}
	for _, c := range all {
		if c.s.Activity {
			h.Wishlist = append(h.Wishlist, c.s)
		}
	}
	slices.SortStableFunc(h.Wishlist, func(a, b GameSummary) int { return cmp.Compare(b.PublishedAt, a.PublishedAt) })
	h.Featured = featured(all, exclude)
	return h
}

// Recommend recommends from the source-backed games as annotated; see
// Recommend.
func (v *View) Recommend(annotate Annotate, signals []Signal, exclude func(GameSummary) bool, limit int) ([]Recommendation, string) {
	all := v.cards(annotate)
	games := make([]GameSummary, 0, len(all))
	for _, c := range all {
		games = append(games, c.s)
	}
	return Recommend(games, signals, exclude, limit)
}

// featuredSize is how many games the top of the front page rotates through.
const featuredSize = 5

// featured picks games that can be installed first, then by chart place and
// recency; installed games are left out.
func featured(all []card, exclude func(GameSummary) bool) []GameSummary {
	open := []GameSummary{}
	for _, c := range all {
		if c.s.Installed == nil && (exclude == nil || !exclude(c.s)) {
			open = append(open, c.s)
		}
	}
	slices.SortFunc(open, func(a, b GameSummary) int {
		return cmp.Or(compareBool(b.Installable, a.Installable), compareGames(a, b))
	})
	return open[:min(len(open), featuredSize)]
}

// compareBool orders false before true.
func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case b:
		return -1
	}
	return 1
}
