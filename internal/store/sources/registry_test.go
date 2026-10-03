package sources

import (
	"slices"
	"testing"
)

func TestRegistryListsDiscoveryProvidersInDisplayOrder(t *testing.T) {
	if got := DiscoveryIDs(); !slices.Equal(got[:2], []string{"fitgirl", "dodi"}) || slices.Contains(got, "1337x") {
		t.Fatalf("discovery providers %v", got)
	}
	for _, p := range Providers() {
		if !p.Discovery || p.Listing == "" || p.Name == "" {
			t.Fatalf("incomplete provider %+v", p)
		}
		if _, err := p.ValidateURL(p.Listing); err != nil {
			t.Fatalf("%s listing is off its origin: %v", p.ID, err)
		}
	}
	if _, ok := Lookup("1337x"); !ok {
		t.Fatal("1337x must stay registered for torrent mirrors")
	}
}

func TestNewProvidersStayOffUntilChosen(t *testing.T) {
	defaults := DefaultIDs()
	if !slices.Equal(defaults, []string{"fitgirl", "dodi"}) {
		t.Fatalf("defaults %v: a provider added later must not be chosen for anyone automatically", defaults)
	}
}

func TestListingURLsFollowEachProvidersPaging(t *testing.T) {
	p, _ := Lookup("dodi")
	if u, ok := p.ListingURL(1); !ok || u != "https://dodi-repacks.site/" {
		t.Fatal(u)
	}
	if u, ok := p.ListingURL(7); !ok || u != "https://dodi-repacks.site/page/7/" {
		t.Fatal(u)
	}
	if _, ok := p.ListingURL(0); ok {
		t.Fatal("page 0 accepted")
	}
	single := Provider{Source: Source{ID: "single", Name: "Single", Host: "single.example"}, Listing: "https://single.example/games.html"}
	if u, ok := single.ListingURL(1); !ok || u != "https://single.example/games.html" {
		t.Fatal(u)
	}
	if u, ok := single.ListingURL(2); ok || single.Paged() {
		t.Fatalf("a finite catalog has no page 2: %q", u)
	}
}

func TestSearchFollowsTheProviderCapability(t *testing.T) {
	p, _ := Lookup("dodi")
	if got, err := p.SearchURL("Half-Life 2"); err != nil || got != "https://dodi-repacks.site/?s=Half-Life+2" {
		t.Fatalf("%s %v", got, err)
	}
	x, _ := Lookup("1337x")
	if _, err := x.SearchURL("anything"); err == nil {
		t.Fatal("a provider without site search answered one")
	}
}

func TestLookupReturnsCopies(t *testing.T) {
	p, _ := Lookup("fitgirl")
	p.Notes = append(p.Notes, "changed")
	p.Name = "changed"
	if q, _ := Lookup("fitgirl"); q.Name != "FitGirl" || slices.Contains(q.Notes, "changed") {
		t.Fatal("registry was modified through a copy")
	}
}
