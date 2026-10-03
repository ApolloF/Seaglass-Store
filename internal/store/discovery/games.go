package discovery

import (
	"cmp"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/scan"
	"github.com/ApolloF/Seaglass/internal/store/catalog"
	"github.com/ApolloF/Seaglass/internal/store/sources"
)

// Identify finds a trusted Steam identity for a release title: Steam's
// name and AppID, or 0 when none is trustworthy enough to merge on.
type Identify func(title string) (name string, appID int)

// Game is a group of source releases (and feed offers) for one game.
type Game struct {
	Key       string
	Title     string
	AppID     int
	How       string // Identity.How
	Corrected bool
	Records   []Record       // newest publication first
	Feed      *catalog.Entry // the person's feeds' offers for it, if any
	TitleKeys []string       // the normalized release titles grouped here

	sortTitle string
	search    string
	published int64
	updated   int64
}

// Group puts records and feed entries together by game key. Records merge
// only through a trusted Steam identity or an identical normalized title,
// so sequels, remasters and DLC stay apart.
func Group(records []Record, feeds []catalog.Entry, corrections map[string]IdentityCorrection, identify Identify) []*Game {
	byKey := map[string]*Game{}
	type ident struct {
		name  string
		appID int
		how   string
		fixed bool
	}
	known := map[string]ident{}
	for _, r := range records {
		tk := scan.Normalize(r.Entry.Title)
		if tk == "" {
			continue
		}
		id, ok := known[tk]
		if !ok {
			if c, has := corrections[tk]; has {
				id = ident{name: c.Name, appID: c.SteamAppID, how: "correction", fixed: true}
			} else if identify != nil {
				if name, appID := identify(r.Entry.Title); appID > 0 {
					id = ident{name: name, appID: appID, how: "game-database"}
				}
			}
			known[tk] = id
		}
		key := "title:" + tk
		if id.appID > 0 {
			key = "steam:" + strconv.Itoa(id.appID)
		}
		g := byKey[key]
		if g == nil {
			g = &Game{Key: key, AppID: id.appID, How: id.how, Corrected: id.fixed}
			if id.appID > 0 && id.name != "" {
				g.Title = id.name
			}
			byKey[key] = g
		}
		if id.fixed {
			g.Corrected, g.How = true, "correction"
			if id.appID == 0 {
				g.How = ""
			}
		}
		if !slices.Contains(g.TitleKeys, tk) {
			g.TitleKeys = append(g.TitleKeys, tk)
		}
		g.Records = append(g.Records, r)
	}
	for i := range feeds {
		f := feeds[i]
		g := byKey[f.Key]
		if g == nil {
			g = &Game{Key: f.Key, Title: f.Title, AppID: f.SteamAppID}
			if f.SteamAppID > 0 {
				g.How = "feed"
			}
			byKey[f.Key] = g
		}
		g.Feed = &f
		if g.Title == "" {
			g.Title = f.Title
		}
	}
	out := make([]*Game, 0, len(byKey))
	for _, g := range byKey {
		slices.SortStableFunc(g.Records, func(a, b Record) int { return cmp.Compare(published(b), published(a)) })
		if g.Title == "" && len(g.Records) > 0 {
			g.Title = g.Records[0].Entry.Title
		}
		for _, r := range g.Records {
			g.published = max(g.published, published(r))
			g.updated = max(g.updated, updated(r))
		}
		g.sortTitle = scan.SortTitle(g.Title)
		g.search = scan.Normalize(g.Title)
		for _, tk := range g.TitleKeys {
			if !strings.Contains(g.search, tk) {
				g.search += " " + tk
			}
		}
		out = append(out, g)
	}
	slices.SortFunc(out, func(a, b *Game) int {
		return cmp.Or(strings.Compare(a.sortTitle, b.sortTitle), strings.Compare(a.Key, b.Key))
	})
	return out
}

func published(r Record) int64 {
	if r.Entry.PublishedAt != nil {
		return r.Entry.PublishedAt.Unix()
	}
	return 0
}

// updated is when the release last changed: a claim change the index
// noticed, or the article's own modified time when it is clearly later
// than its publication.
func updated(r Record) int64 {
	var t int64
	if !r.Changed.IsZero() {
		t = r.Changed.Unix()
	}
	if r.Entry.UpdatedAt != nil && r.Entry.PublishedAt != nil && r.Entry.UpdatedAt.Sub(*r.Entry.PublishedAt) > time.Hour {
		t = max(t, r.Entry.UpdatedAt.Unix())
	}
	return t
}

// Summary is the game's card, before what the app adds (popularity,
// reviews, genres, installed, wishlist).
func (g *Game) Summary() GameSummary {
	s := GameSummary{Key: g.Key, Title: g.Title, SteamAppID: g.AppID, Sources: []string{}, Languages: []string{}, Genres: []string{},
		PublishedAt: g.published, UpdatedAt: g.updated}
	for _, r := range g.Records {
		if !slices.Contains(s.Sources, r.Entry.SourceID) {
			s.Sources = append(s.Sources, r.Entry.SourceID)
		}
		langs, _ := ParseLanguages(r.Entry.LanguageClaim)
		for _, l := range langs {
			if !slices.Contains(s.Languages, l) {
				s.Languages = append(s.Languages, l)
			}
		}
		if availability(r) == AvailInstallable {
			s.Installable = true
		}
	}
	s.Releases = len(g.Records)
	if len(g.Records) > 0 {
		newest := g.Records[0].Entry
		s.Version = newest.Version
		if !newest.SizeIsMinimum {
			s.SizeBytes = newest.SizeBytes
		}
	}
	if g.Feed != nil {
		s.Sources = append(s.Sources, SourceFeeds)
		s.Releases += len(g.Feed.Offers)
		s.Installable = s.Installable || len(g.Feed.Offers) > 0
		for _, l := range g.Feed.Languages {
			if !slices.Contains(s.Languages, l) {
				s.Languages = append(s.Languages, l)
			}
		}
		if len(g.Records) == 0 {
			s.Version, s.SizeBytes = g.Feed.Version, g.Feed.Size
		}
	}
	s.SourceBacked = s.Releases > 0
	return s
}

// Releases are the game's release choices, newest publication first, feed
// offers after the source releases.
func (g *Game) Releases(sourceName func(string) string) []Release {
	out := make([]Release, 0, len(g.Records))
	for _, r := range g.Records {
		out = append(out, ReleaseOf(r, sourceName(r.Entry.SourceID)))
	}
	if g.Feed != nil {
		for i, o := range g.Feed.Offers {
			out = append(out, Release{ID: "feed:" + strconv.Itoa(i), Origin: OriginFeed, Source: o.FeedURL, SourceName: o.FeedName, Title: g.Feed.Title,
				RawTitle: o.Title, Version: o.Version, SizeBytes: o.SizeBytes, InstalledSizeBytes: o.InstalledSizeBytes, Languages: nonNil(o.Languages),
				Kind: "release", Availability: AvailInstallable, Transports: 1, Unresolved: []string{}, Warnings: []string{}, FeedKey: g.Feed.Key, FeedOffer: i})
		}
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return slices.Clone(s)
}

// ReleaseOf turns a record into a release choice.
func ReleaseOf(r Record, sourceName string) Release {
	e := r.Entry
	langs, _ := ParseLanguages(e.LanguageClaim)
	rel := Release{ID: e.ID, Origin: OriginSource, Source: e.SourceID, SourceName: sourceName, Title: e.Title, RawTitle: e.RawTitle, Version: e.Version,
		PageURL: e.PageURL, PublishedAt: published(r), UpdatedAt: updated(r), SizeClaim: e.SizeClaim, SizeIsMinimum: e.SizeIsMinimum,
		InstalledSizeBytes: e.InstalledSizeBytes, Languages: langs, LanguageClaim: e.LanguageClaim, Kind: e.ReleaseKind,
		Availability: availability(r), Transports: len(validTransports(e)), Unresolved: unresolved(r), Warnings: nonNil(e.Warnings)}
	if !e.SizeIsMinimum && len(e.SizeOptionsBytes) == 0 {
		rel.SizeBytes = e.SizeBytes
	}
	if rel.Kind == "" {
		rel.Kind = "release"
	}
	if p, ok := sources.Lookup(e.SourceID); ok && !p.Torrents {
		rel.BrowserOnly = true
	}
	return rel
}

// validTransports are the torrent identities a download can use: magnets
// with a v1 info hash that still parse.
func validTransports(e sources.Entry) []int {
	var out []int
	for i, t := range e.Transports {
		if t.Kind != "magnet" || t.InfoHash == "" {
			continue
		}
		if m, err := sources.Magnet(t.URI); err == nil && m.InfoHash == t.InfoHash {
			out = append(out, i)
		}
	}
	return out
}

// Resolution states that only a browser can get past.
var manualStates = map[string]bool{"captcha-required": true, "manual-required": true, "access-blocked": true}

func availability(r Record) string {
	e := r.Entry
	switch {
	case r.Gone:
		return AvailGone
	case e.ReleaseKind == "update":
		return AvailUpdateOnly
	case e.ReleaseKind == "preview":
		return AvailPreview
	case len(validTransports(e)) > 0:
		return AvailInstallable
	case e.SummaryOnly:
		return AvailSummary
	}
	resolvable := false
	for _, ref := range e.References {
		if ref.Kind != "torrent" {
			continue
		}
		if _, err := sources.ResolverURL(ref); err == nil && !manualStates[ref.State] {
			resolvable = true
		}
	}
	if resolvable {
		return AvailUnresolved
	}
	return AvailManual
}

// unresolved explains, one line per mirror, why a release has no usable
// torrent yet.
func unresolved(r Record) []string {
	out := []string{}
	if len(validTransports(r.Entry)) > 0 {
		return out
	}
	if r.FetchError != "" {
		out = append(out, "The release page couldn't be read: "+r.FetchError)
	}
	if r.Entry.SummaryOnly {
		return append(out, "Only the search summary is known; the release page is read when you open it")
	}
	for _, ref := range r.Entry.References {
		if ref.Kind != "torrent" {
			continue
		}
		host := ref.URL
		if u, err := url.Parse(ref.URL); err == nil {
			host = u.Hostname()
		}
		switch {
		case ref.State == "" || ref.State == "resolved":
			if _, err := sources.ResolverURL(ref); err != nil {
				out = append(out, host+": only a browser can get this torrent")
			} else {
				out = append(out, host+": not resolved yet")
			}
		case ref.Reason != "":
			out = append(out, fmt.Sprintf("%s: %s (%s)", host, stateText(ref.State), ref.Reason))
		default:
			out = append(out, host+": "+stateText(ref.State))
		}
	}
	if len(out) == 0 {
		out = append(out, "The release page lists no torrent mirror Seaglass can use")
	}
	return out
}

func stateText(state string) string {
	switch state {
	case "captcha-required":
		return "the host asks for a browser challenge"
	case "rate-limited":
		return "the host asked to wait; try again later"
	case "access-blocked":
		return "the host refused access"
	case "manual-required":
		return "only a browser can get this torrent"
	case "host-error":
		return "the host reported an error"
	default:
		return "resolving failed"
	}
}
