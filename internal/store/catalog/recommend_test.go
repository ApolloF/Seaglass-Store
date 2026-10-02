package catalog

import (
	"strings"
	"testing"

	"github.com/ApolloF/Seaglass/internal/store/feed"
)

func offer(feedURL, name, version string, size int64, langs ...string) Offer {
	return Offer{Item: feed.Item{Title: "G", Version: version, SizeBytes: size, Languages: langs}, FeedURL: feedURL, FeedName: name}
}

func TestRecommend(t *testing.T) {
	newest := offer("https://a", "A", "v2", 10, "English")
	older := offer("https://b", "B", "v1", 8, "English", "German")
	e := Entry{Offers: []Offer{newest, older}}

	if r := Recommend(e, Prefs{}); r.Offer != 0 || !strings.Contains(strings.Join(r.Why, "|"), "newest") {
		t.Errorf("no preferences: %+v", r)
	}
	r := Recommend(e, Prefs{Language: "german"})
	if r.Offer != 1 || !strings.Contains(strings.Join(r.Why, "|"), "doesn't have german") {
		t.Errorf("wants German: %+v", r)
	}
	if r := Recommend(e, Prefs{Blocked: map[string]int{"A": 2}}); r.Offer != 1 || !strings.Contains(strings.Join(r.Why, "|"), "blocked") {
		t.Errorf("A's downloads were blocked: %+v", r)
	}
	if r := Recommend(e, Prefs{Trust: map[string]int{"https://b": 2}}); r.Offer != 1 {
		t.Errorf("trusts B more: %+v", r)
	}
	if r := Recommend(e, Prefs{Trust: map[string]int{"https://b": 1}}); r.Offer != 0 {
		t.Errorf("a little more trust doesn't beat a newer version: %+v", r)
	}

	// Same version from two feeds: the smaller download.
	twin := offer("https://c", "C", "v2", 7)
	if r := Recommend(Entry{Offers: []Offer{newest, twin}}, Prefs{}); r.Offer != 1 {
		t.Errorf("same version, smaller: %+v", r)
	}
	checked := newest
	checked.SHA256, checked.InstallerType = strings.Repeat("a", 64), "inno"
	if r := Recommend(Entry{Offers: []Offer{checked}}, Prefs{}); r.Offer != 0 || len(r.Why) != 2 {
		t.Errorf("one offer with a checksum and a silent installer: %+v", r)
	}
	if r := Recommend(Entry{Offers: []Offer{older}}, Prefs{}); r.Why[0] != "The only version offered" {
		t.Errorf("one plain offer: %+v", r)
	}
	if r := Recommend(Entry{}, Prefs{}); r.Offer != -1 {
		t.Errorf("no offers: %+v", r)
	}
}
