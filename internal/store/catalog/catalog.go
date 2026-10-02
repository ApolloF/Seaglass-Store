// Package catalog turns the feeds the person added into one catalog of
// games: the same game from several feeds (or in several versions) is one
// entry with several offers, newest first.
package catalog

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/ApolloF/Seaglass/internal/scan"
	"github.com/ApolloF/Seaglass/internal/store/feed"
)

// Source is one feed's items.
type Source struct {
	URL  string
	Name string
	Feed feed.Feed
}

// Offer is one downloadable version of a game, from one feed.
type Offer struct {
	feed.Item
	FeedURL  string `json:"feedUrl"`
	FeedName string `json:"feedName"`
}

// Entry is one game in the catalog.
type Entry struct {
	Key        string   `json:"key"` // "steam:<id>" or "title:<normalized title>"
	Title      string   `json:"title"`
	SteamAppID int      `json:"steamAppId,omitempty"`
	Offers     []Offer  `json:"offers"`    // newest version first
	Version    string   `json:"version"`   // the newest offer's
	Updated    string   `json:"updated"`   // the newest build date, YYYY-MM-DD
	Size       int64    `json:"size"`      // the newest offer's download
	Languages  []string `json:"languages"` // of every offer together
	// Filled in for the interface when the entry is asked for.
	Recommended *Recommendation `json:"recommended,omitempty"`
	Installed   *Installed      `json:"installed,omitempty"`
	sortTitle   string
	search      string
}

// Installed is a catalog game the store installed.
type Installed struct {
	Download string `json:"download"` // the download (job) that installed it
	Version  string `json:"version"`
	Dir      string `json:"dir"`
	Update   bool   `json:"update"` // the catalog has a newer version
}

// Identify finds the game behind a feed item's title (and Steam AppID,
// when the feed has one): its proper name and Steam AppID, 0 when unknown.
type Identify func(title string, steamAppID int) (string, int)

// Build makes the catalog. identify may be nil.
func Build(sources []Source, identify Identify) []Entry {
	byKey := map[string]*Entry{}
	var order []string
	for _, s := range sources {
		for _, it := range s.Feed.Items {
			title, appID := it.Title, it.SteamAppID
			if identify != nil {
				if t, id := identify(it.Title, it.SteamAppID); t != "" {
					title, appID = t, id
				}
			}
			key := "title:" + scan.Normalize(title)
			if appID > 0 {
				key = "steam:" + strconv.Itoa(appID)
			}
			e := byKey[key]
			if e == nil {
				e = &Entry{Key: key, Title: title, SteamAppID: appID}
				byKey[key] = e
				order = append(order, key)
			}
			e.Offers = append(e.Offers, Offer{Item: it, FeedURL: s.URL, FeedName: s.Name})
		}
	}
	out := make([]Entry, 0, len(order))
	for _, k := range order {
		e := byKey[k]
		slices.SortStableFunc(e.Offers, func(a, b Offer) int {
			return cmp.Or(-CompareVersions(a.Version, b.Version), -strings.Compare(a.BuildDate, b.BuildDate))
		})
		best := e.Offers[0]
		e.Version, e.Size = best.Version, best.SizeBytes
		seen := map[string]bool{}
		for _, o := range e.Offers {
			e.Updated = max(e.Updated, o.BuildDate)
			for _, l := range o.Languages {
				if k := strings.ToLower(l); !seen[k] {
					seen[k] = true
					e.Languages = append(e.Languages, l)
				}
			}
		}
		if e.Languages == nil {
			e.Languages = []string{}
		}
		e.sortTitle = scan.SortTitle(e.Title)
		e.search = scan.Normalize(e.Title)
		out = append(out, *e)
	}
	slices.SortFunc(out, func(a, b Entry) int { return strings.Compare(a.sortTitle, b.sortTitle) })
	return out
}

// Query picks and orders entries.
type Query struct {
	Text     string `json:"text"`
	Language string `json:"language"` // "" for any
	Sort     string `json:"sort"`     // title, updated, size
	Offset   int    `json:"offset"`
	Limit    int    `json:"limit"` // at most 200
}

// Page is one page of entries.
type Page struct {
	Entries []Entry `json:"entries"`
	Total   int     `json:"total"` // matching entries, on every page together
}

// Search returns the entries that match q.
func Search(all []Entry, q Query) Page {
	text := scan.Normalize(q.Text)
	var hits []Entry
	for _, e := range all {
		if text != "" && !strings.Contains(e.search, text) {
			continue
		}
		if q.Language != "" && !slices.ContainsFunc(e.Languages, func(l string) bool { return strings.EqualFold(l, q.Language) }) {
			continue
		}
		hits = append(hits, e)
	}
	switch q.Sort {
	case "updated":
		slices.SortStableFunc(hits, func(a, b Entry) int { return strings.Compare(b.Updated, a.Updated) })
	case "size":
		slices.SortStableFunc(hits, func(a, b Entry) int { return cmp.Compare(a.Size, b.Size) })
	}
	limit := q.Limit
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	off := min(max(q.Offset, 0), len(hits))
	end := min(off+limit, len(hits))
	return Page{Entries: append([]Entry{}, hits[off:end]...), Total: len(hits)}
}

// Languages lists every language in the catalog, most offered first.
func Languages(all []Entry) []string {
	count := map[string]int{}
	name := map[string]string{}
	for _, e := range all {
		for _, l := range e.Languages {
			k := strings.ToLower(l)
			count[k]++
			if name[k] == "" {
				name[k] = l
			}
		}
	}
	out := make([]string, 0, len(count))
	for k := range count {
		out = append(out, k)
	}
	slices.SortFunc(out, func(a, b string) int { return cmp.Or(count[b]-count[a], strings.Compare(a, b)) })
	for i, k := range out {
		out[i] = name[k]
	}
	return out
}
