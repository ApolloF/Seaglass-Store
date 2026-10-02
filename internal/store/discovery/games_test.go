package discovery

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/store/catalog"
	"github.com/ApolloF/Seaglass/internal/store/feed"
	"github.com/ApolloF/Seaglass/internal/store/sources"
)

func rec(src, title, version string, ago time.Duration) Record {
	pub := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).Add(-ago)
	page := "https://" + map[string]string{"fitgirl": "fitgirl-repacks.site", "dodi": "dodi-repacks.site"}[src] + "/" + strings.ReplaceAll(strings.ToLower(title+version), " ", "-") + "/"
	return Record{Entry: sources.Entry{ID: sources.EntryID(src, page), SourceID: src, PageURL: page, Title: title, RawTitle: title + " " + version, Version: version, PublishedAt: &pub, ReleaseKind: "release",
		LanguageClaim: "English", Transports: []sources.Transport{{Kind: "magnet", URI: "magnet:?xt=urn:btih:" + testMagnetHash, InfoHash: testMagnetHash}}}}
}

func keys(gs []*Game) []string {
	var out []string
	for _, g := range gs {
		out = append(out, g.Key)
	}
	slices.Sort(out)
	return out
}

func TestIdenticalNormalizedTitlesMergeAcrossSources(t *testing.T) {
	gs := Group([]Record{rec("fitgirl", "Ember Crown", "v1.2", 0), rec("dodi", "EMBER CROWN", "v1.1", time.Hour), rec("dodi", "Ember: Crown", "v1.0", 2*time.Hour)}, nil, nil, nil)
	if len(gs) != 1 || len(gs[0].Records) != 3 || gs[0].Key != "title:embercrown" {
		t.Fatalf("groups: %v", keys(gs))
	}
	s := gs[0].Summary()
	if !slices.Equal(s.Sources, []string{"fitgirl", "dodi"}) || s.Version != "v1.2" || !s.Installable {
		t.Errorf("summary: %+v", s)
	}
}

func TestSequelsRemastersAndDLCStayApart(t *testing.T) {
	titles := []string{"Ashen Lanterns", "Ashen Lanterns 2", "Ashen Lanterns II", "Ashen Lanterns Remastered", "Ashen Lanterns: Deep Ember DLC", "Ashen Lanterns - Definitive Edition"}
	var rs []Record
	for i, title := range titles {
		rs = append(rs, rec("fitgirl", title, "v1", time.Duration(i)*time.Hour))
	}
	// The game database only knows the original; nothing loose merges.
	identify := func(title string) (string, int) {
		if title == "Ashen Lanterns" {
			return "Ashen Lanterns", 1938400
		}
		return "", 0
	}
	gs := Group(rs, nil, nil, identify)
	if len(gs) != len(titles) {
		t.Fatalf("%d titles made %d games: %v", len(titles), len(gs), keys(gs))
	}
}

func TestTrustedSteamIdentityMergesDifferentlyWrittenTitles(t *testing.T) {
	identify := func(title string) (string, int) {
		switch title {
		case "The Witcher 3: Wild Hunt", "Witcher 3 Wild Hunt GOTY":
			return "The Witcher 3: Wild Hunt", 292030
		}
		return "", 0
	}
	gs := Group([]Record{rec("fitgirl", "The Witcher 3: Wild Hunt", "v4.04", 0), rec("dodi", "Witcher 3 Wild Hunt GOTY", "v4.04", time.Hour)}, nil, nil, identify)
	if len(gs) != 1 || gs[0].Key != "steam:292030" || gs[0].Title != "The Witcher 3: Wild Hunt" || gs[0].How != "game-database" {
		t.Fatalf("groups: %+v", gs)
	}
}

func TestSteamCorrectionSurvivesRefreshesAndRestarts(t *testing.T) {
	dir := t.TempDir()
	ix := OpenIndex(filepath.Join(dir, "d"), filepath.Join(dir, "store-identity.json"))
	identify := func(title string) (string, int) { return "Wrong Game", 111 }
	if err := ix.SetSteamCorrections([]IdentityCorrection{{TitleKey: "embercrown", SteamAppID: 1245620, Name: "Ember Crown"}, {TitleKey: "quietorbit", SteamAppID: 0}}); err != nil {
		t.Fatal(err)
	}
	ix = OpenIndex(filepath.Join(dir, "d"), filepath.Join(dir, "store-identity.json"))
	gs := Group([]Record{rec("fitgirl", "Ember Crown", "v1.3", 0), rec("dodi", "Quiet Orbit", "v1", 0)}, nil, ix.SteamCorrections(), identify)
	got := map[string]*Game{}
	for _, g := range gs {
		got[g.Key] = g
	}
	if g := got["steam:1245620"]; g == nil || !g.Corrected || g.How != "correction" || g.Title != "Ember Crown" {
		t.Errorf("corrected match: %v", keys(gs))
	}
	if g := got["title:quietorbit"]; g == nil || !g.Corrected || g.AppID != 0 {
		t.Errorf("'not on Steam' kept the game database's guess: %v", keys(gs))
	}
}

func TestFeedOffersJoinTheSameGame(t *testing.T) {
	feeds := catalog.Build([]catalog.Source{{URL: "https://feeds.example/a.json", Name: "Indie", Feed: feed.Feed{Items: []feed.Item{{Title: "Brass Orchard", Version: "Build 2", Magnet: "magnet:?xt=urn:btih:" + testMagnetHash, Languages: []string{"Spanish"}}}}}}, nil)
	gs := Group([]Record{rec("fitgirl", "Brass Orchard", "Build 1", 0)}, feeds, nil, nil)
	if len(gs) != 1 || gs[0].Feed == nil {
		t.Fatalf("groups: %v", keys(gs))
	}
	s := gs[0].Summary()
	if !slices.Contains(s.Sources, SourceFeeds) || s.Releases != 2 || !slices.Contains(s.Languages, "Spanish") {
		t.Errorf("summary: %+v", s)
	}
	rels := gs[0].Releases(func(id string) string { return id })
	if len(rels) != 2 || rels[1].Origin != OriginFeed || rels[1].FeedKey != "title:brassorchard" || rels[1].FeedOffer != 0 {
		t.Errorf("releases: %+v", rels)
	}
}

func TestReleaseAvailabilityIsExplicit(t *testing.T) {
	r := rec("dodi", "Hollow Tide", "v2", 0)
	r.Entry.Transports = nil
	r.Entry.References = []sources.Reference{{URL: "https://file-me.top/abcdefabcdef.html", Kind: "torrent"}}
	if a := availability(r); a != AvailUnresolved {
		t.Errorf("an unresolved File-Me mirror: %s", a)
	}
	r.Entry.References[0].State, r.Entry.References[0].Reason = "captcha-required", "browser challenge"
	if a := availability(r); a != AvailManual {
		t.Errorf("a CAPTCHA mirror: %s", a)
	}
	if u := unresolved(r); len(u) != 1 || !strings.Contains(u[0], "file-me.top") || !strings.Contains(u[0], "browser challenge") {
		t.Errorf("reasons: %v", u)
	}
	r.Entry.References = []sources.Reference{{URL: "https://unknown-host.example/x", Kind: "torrent"}}
	if a := availability(r); a != AvailManual {
		t.Errorf("an unsupported host: %s", a)
	}
	r.Entry.ReleaseKind = "update"
	if a := availability(r); a != AvailUpdateOnly {
		t.Errorf("an update: %s", a)
	}
	r = rec("fitgirl", "X", "v1", 0)
	r.Entry.Transports[0].URI = "magnet:?xt=urn:btih:broken"
	if a := availability(r); a == AvailInstallable {
		t.Error("an invalid magnet counts as a validated transport")
	}
}

func TestLanguageClaimsAreReadWithoutGuessing(t *testing.T) {
	cases := []struct {
		claim    string
		want     []string
		complete bool
	}{
		{"RUS/ENG/MULTi10", []string{"Russian", "English"}, false},
		{"English, French, German, Spanish – Spain, Portuguese – Brazil, Simplified Chinese", []string{"English", "French", "German", "Spanish", "Brazilian Portuguese", "Chinese"}, true},
		{"Text: ENG; Audio: JAP", []string{"English", "Japanese"}, true},
		{"MULTi12", []string{}, false},
		{"", []string{}, false},
	}
	for _, c := range cases {
		got, complete := ParseLanguages(c.claim)
		if !slices.Equal(got, c.want) || complete != c.complete {
			t.Errorf("%q: %v %v, want %v %v", c.claim, got, complete, c.want, c.complete)
		}
	}
}
