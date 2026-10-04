package sources

import (
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Provider describes a release source: where its catalog starts, how it is
// paged and searched, and what it can offer. The registry below is the one
// place a provider is declared; discovery, settings and the interface read
// it instead of naming sources. See docs/store-providers.md.
type Provider struct {
	Source
	// Discovery: the Store indexes this provider and lists it in source
	// setup. Providers without it serve other parts of the store (1337x
	// mirrors torrent files for other sources' releases).
	Discovery bool `json:"discovery"`
	// DefaultOn: a new Store user starts with this provider chosen. No
	// built-in provider is: the person turns sources on in Store settings,
	// so turning the Store on (or an update) never starts requests to a
	// site nobody picked.
	DefaultOn bool `json:"defaultOn"`
	// Listing is the first page of the newest-first release catalog.
	Listing string `json:"listing"`
	// ListingPage is listing page n for n >= 2, with "{n}" for the number.
	// "" means the catalog is one finite page and has no second page.
	ListingPage string `json:"listingPage,omitempty"`
	// Feed is an RSS feed that carries the newest articles in full; "" none.
	Feed string `json:"feed,omitempty"`
	// Search is the provider's own site search, with "{q}" for the
	// query-encoded text; "" means it has none and only the index is searched.
	Search string `json:"search,omitempty"`
	// Torrents: release pages may link torrents Seaglass can validate and
	// install. Without it every release opens in the browser.
	Torrents bool `json:"torrents"`
	// Notes are the provider's verified limitations, in plain sentences
	// the interface can show.
	Notes []string `json:"notes,omitempty"`
}

// providers in the order the interface lists them.
var providers = []Provider{
	{
		Source:    Source{ID: "fitgirl", Name: "FitGirl", Host: "fitgirl-repacks.site", StartURL: "https://fitgirl-repacks.site/feed/"},
		Discovery: true, Torrents: true,
		Listing:     "https://fitgirl-repacks.site/",
		ListingPage: "https://fitgirl-repacks.site/page/{n}/",
		Feed:        "https://fitgirl-repacks.site/feed/",
		Search:      "https://fitgirl-repacks.site/?s={q}",
	},
	{
		Source:    Source{ID: "dodi", Name: "DODI", Host: "dodi-repacks.site", StartURL: "https://dodi-repacks.site/"},
		Discovery: true, Torrents: true,
		Listing:     "https://dodi-repacks.site/",
		ListingPage: "https://dodi-repacks.site/page/{n}/",
		Search:      "https://dodi-repacks.site/?s={q}",
	},
	{
		Source:    Source{ID: "elamigos", Name: "ElAmigos", Host: "elamigos.site", StartURL: "https://elamigos.site/"},
		Discovery: true,
		Listing:   "https://elamigos.site/",
		Notes: []string{
			"One finite catalog; search uses indexed titles. Detail pages retain their .html URLs.",
			"File-host containers open in the browser. Automated downloads are unsupported.",
			"The installer version excludes separately listed patches. Source claims do not establish payload safety.",
		},
	},
	{
		Source:   Source{ID: "1337x", Name: "1337x", Host: "1337x.to", StartURL: "https://1337x.to/cat/Games/1/"},
		Torrents: true,
		Listing:  "https://1337x.to/cat/Games/1/",
	},
}

// Providers lists the providers discovery can index, in display order.
func Providers() []Provider {
	var out []Provider
	for _, p := range providers {
		if p.Discovery {
			out = append(out, clone(p))
		}
	}
	return out
}

// DiscoveryIDs lists the IDs of Providers.
func DiscoveryIDs() []string {
	var out []string
	for _, p := range providers {
		if p.Discovery {
			out = append(out, p.ID)
		}
	}
	return out
}

// DefaultIDs lists the discovery providers a new Store user starts with.
func DefaultIDs() []string {
	var out []string
	for _, p := range providers {
		if p.Discovery && p.DefaultOn {
			out = append(out, p.ID)
		}
	}
	return out
}

// Lookup finds any registered provider by ID.
func Lookup(id string) (Provider, bool) {
	for _, p := range providers {
		if p.ID == id {
			return clone(p), true
		}
	}
	return Provider{}, false
}

func clone(p Provider) Provider {
	p.Notes = slices.Clone(p.Notes)
	return p
}

// ListingURL is catalog page n (1 = newest). ok is false past the end of a
// finite catalog, so nobody requests a page that cannot exist.
func (p Provider) ListingURL(n int) (string, bool) {
	switch {
	case n < 1:
		return "", false
	case n == 1:
		return p.Listing, p.Listing != ""
	case p.ListingPage == "":
		return "", false
	}
	return strings.ReplaceAll(p.ListingPage, "{n}", strconv.Itoa(n)), true
}

// Paged reports whether the catalog continues past its first page.
func (p Provider) Paged() bool { return p.ListingPage != "" }

// SearchURL asks the provider's own site search, rather than filtering only
// what is indexed.
func (p Provider) SearchURL(query string) (string, error) {
	if p.Search == "" {
		return "", errors.New(p.Name + " has no site search")
	}
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 200 {
		return "", errors.New("search must contain 1..200 bytes")
	}
	return strings.ReplaceAll(p.Search, "{q}", url.QueryEscape(query)), nil
}
