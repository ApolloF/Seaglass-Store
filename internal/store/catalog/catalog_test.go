package catalog

import (
	"testing"

	"github.com/ApolloF/Seaglass/internal/store/feed"
)

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"v1.10", "v1.9", 1},
		{"1.2", "1.2.0", -1},
		{"Build 15302", "Build 9876", 1},
		{"2026-05-01", "2026-04-30", 1},
		{"v1.2 + 3 DLCs", "v1.2", 0},
		{"v2.0 (Hotfix 3)", "v2.0", 0},
		{"", "v0.1", -1},
		{"latest", "", 0},
		{"v1.0.99999999999999999999999", "v1.0.5", 1},
	} {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := CompareVersions(c.b, c.a); got != -c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.b, c.a, got, -c.want)
		}
	}
}

func item(title, version, date string, size int64, langs ...string) feed.Item {
	return feed.Item{Title: title, Version: version, BuildDate: date, SizeBytes: size, Languages: langs, Magnet: "magnet:?xt=urn:btih:" + title}
}

func TestBuild(t *testing.T) {
	a := Source{URL: "https://a/feed.json", Name: "A", Feed: feed.Feed{Items: []feed.Item{
		item("The Alpha", "v1.9", "2026-01-01", 10, "English"),
		item("Beta", "", "", 5),
		item("Portal", "v2", "", 7),
	}}}
	b := Source{URL: "https://b/feed.json", Name: "B", Feed: feed.Feed{Items: []feed.Item{
		item("the alpha", "v1.10", "2026-02-01", 12, "english", "French"),
		item("PORTAL: Still Alive", "v1", "", 6),
	}}}
	// Both Portals are the same Steam game; the rest stay unknown.
	ident := func(title string, _ int) (string, int) {
		if len(title) >= 6 && (title[:6] == "Portal" || title[:6] == "PORTAL") {
			return "Portal", 400
		}
		return "", 0
	}
	got := Build([]Source{a, b}, ident)
	if len(got) != 3 {
		t.Fatalf("%d entries: %+v", len(got), got)
	}
	alpha, beta, portal := got[0], got[1], got[2]
	if alpha.Title != "The Alpha" || beta.Title != "Beta" || portal.Title != "Portal" {
		t.Fatalf("order by sort title: %q, %q, %q", alpha.Title, beta.Title, portal.Title)
	}
	if len(alpha.Offers) != 2 || alpha.Offers[0].FeedName != "B" || alpha.Version != "v1.10" || alpha.Size != 12 || alpha.Updated != "2026-02-01" {
		t.Errorf("alpha, newest offer first: %+v", alpha)
	}
	if len(alpha.Languages) != 2 {
		t.Errorf("languages from every offer, once each: %v", alpha.Languages)
	}
	if portal.Key != "steam:400" || len(portal.Offers) != 2 || portal.Version != "v2" {
		t.Errorf("portal: %+v", portal)
	}
	if beta.Key != "title:beta" || beta.Languages == nil {
		t.Errorf("beta: %+v", beta)
	}
}

func TestSearch(t *testing.T) {
	all := Build([]Source{{Name: "A", Feed: feed.Feed{Items: []feed.Item{
		item("Ember Crown", "1", "2026-03-01", 30, "English", "German"),
		item("Hollow Tide", "1", "2026-05-01", 10, "English"),
		item("Grim & Crown", "1", "2026-01-01", 20, "German"),
	}}}}, nil)
	p := Search(all, Query{Text: "crown"})
	if p.Total != 2 || p.Entries[0].Title != "Ember Crown" {
		t.Errorf("text: %+v", p)
	}
	if p := Search(all, Query{Language: "german", Sort: "size"}); p.Total != 2 || p.Entries[0].Title != "Grim & Crown" {
		t.Errorf("language, by size: %+v", p)
	}
	if p := Search(all, Query{Sort: "updated", Limit: 1, Offset: 1}); p.Total != 3 || len(p.Entries) != 1 || p.Entries[0].Title != "Ember Crown" {
		t.Errorf("updated, second page: %+v", p)
	}
	if p := Search(all, Query{Offset: 50}); p.Total != 3 || len(p.Entries) != 0 {
		t.Errorf("past the end: %+v", p)
	}
	if l := Languages(all); len(l) != 2 || l[0] != "English" {
		t.Errorf("languages: %v", l)
	}
}
