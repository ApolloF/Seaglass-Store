package discovery

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func testView() *View {
	var rs []Record
	for i := 0; i < 130; i++ {
		r := rec("fitgirl", fmt.Sprintf("Game %03d", i), "v1", time.Duration(i)*time.Hour)
		if i%10 == 0 {
			r.Entry.LanguageClaim = "MULTi9" // unknown languages
		}
		if i == 77 {
			r.Changed = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		}
		rs = append(rs, r)
	}
	return NewView(Group(rs, nil, nil, nil))
}

// ranks puts three games on a made-up chart.
func ranks(s *GameSummary) {
	switch s.Title {
	case "Game 050":
		s.PopularRank = 3
	case "Game 120":
		s.PopularRank = 1
	case "Game 007":
		s.PopularRank = 9
	}
	if s.Title == "Game 010" {
		s.ReviewPercent, s.ReviewTotal = 95, 1000
	}
	if s.Title == "Game 011" {
		s.ReviewPercent, s.ReviewTotal = 60, 5
	}
}

func TestBrowsePagesInGroupsOfSixty(t *testing.T) {
	v := testView()
	p := v.Browse(BrowseQuery{}, nil)
	if len(p.Games) != DefaultLimit || p.Total != 130 {
		t.Fatalf("first page: %d of %d", len(p.Games), p.Total)
	}
	p2 := v.Browse(BrowseQuery{Offset: 120}, nil)
	if len(p2.Games) != 10 || p2.Games[0].Title != "Game 120" {
		t.Errorf("last page: %d, %s", len(p2.Games), p2.Games[0].Title)
	}
	if p := v.Browse(BrowseQuery{Limit: 1000}, nil); len(p.Games) != 130 {
		t.Errorf("limit: %d", len(p.Games))
	}
}

func TestPopularSortNeverMakesUpARank(t *testing.T) {
	p := testView().Browse(BrowseQuery{Sort: SortPopular, Limit: 200}, ranks)
	if p.Games[0].Title != "Game 120" || p.Games[1].Title != "Game 050" || p.Games[2].Title != "Game 007" {
		t.Fatalf("chart order: %s, %s, %s", p.Games[0].Title, p.Games[1].Title, p.Games[2].Title)
	}
	for _, g := range p.Games[3:] {
		if g.PopularRank != 0 {
			t.Fatalf("%s got rank %d", g.Title, g.PopularRank)
		}
	}
	if p.Games[3].Title != "Game 000" {
		t.Errorf("unranked games should follow in title order, got %s", p.Games[3].Title)
	}
}

func TestReviewSortPutsUnknownScoresLast(t *testing.T) {
	p := testView().Browse(BrowseQuery{Sort: SortReviews, Limit: 3}, ranks)
	if p.Games[0].Title != "Game 010" || p.Games[1].Title != "Game 011" || p.Games[2].ReviewTotal != 0 {
		t.Errorf("review order: %+v", p.Games)
	}
}

func TestUnknownLanguagesAreCountedNotInvented(t *testing.T) {
	p := testView().Browse(BrowseQuery{Language: "english", Limit: 200}, nil)
	if p.Total != 117 || p.Unknown != 13 {
		t.Errorf("total %d, unknown %d", p.Total, p.Unknown)
	}
	if p := testView().Browse(BrowseQuery{Genre: "RPG"}, nil); p.Total != 0 || p.Unknown != 130 {
		t.Errorf("genre unknown everywhere: total %d, unknown %d", p.Total, p.Unknown)
	}
}

func TestSearchMatchesEveryWord(t *testing.T) {
	v := testView()
	// Game 007, 070-079 and 107: every word is a part of the title.
	if p := v.Browse(BrowseQuery{Text: "game 07"}, nil); p.Total != 12 {
		t.Errorf("'game 07': %d", p.Total)
	}
	if p := v.Browse(BrowseQuery{Text: "nothing"}, nil); p.Total != 0 {
		t.Errorf("'nothing': %d", p.Total)
	}
}

func TestHomeShelvesOrderNewPopularAndUpdated(t *testing.T) {
	h := testView().Home(ranks, StateOK, Status{}, nil)
	if len(h.New) != ShelfSize || h.New[0].Title != "Game 000" || h.New[1].Title != "Game 001" {
		t.Errorf("new: %d, %s", len(h.New), h.New[0].Title)
	}
	if len(h.Popular) != 3 || h.Popular[0].PopularRank != 1 {
		t.Errorf("popular: %+v", h.Popular)
	}
	if len(h.Updated) != 1 || h.Updated[0].Title != "Game 077" {
		t.Errorf("updated: %+v", h.Updated)
	}
	if h := testView().Home(ranks, StateUnavailable, Status{}, nil); len(h.Popular) != 0 || h.PopularState != StateUnavailable {
		t.Error("without a chart the Popular shelf showed something")
	}
}

func TestHomeFeaturedLeavesOutInstalledAndPrefersInstallableAndPopular(t *testing.T) {
	installedOne := func(s *GameSummary) {
		ranks(s)
		s.Installable = s.Title != "Game 007"
		if s.Title == "Game 120" {
			s.Installed = &Installed{}
		}
	}
	h := testView().Home(installedOne, StateOK, Status{}, nil)
	if len(h.Featured) != featuredSize {
		t.Fatalf("featured: %d games", len(h.Featured))
	}
	if h.Featured[0].Title != "Game 050" || h.Featured[1].Title != "Game 000" {
		t.Errorf("order: %s, %s", h.Featured[0].Title, h.Featured[1].Title)
	}
	for _, g := range h.Featured {
		if g.Installed != nil || g.Title == "Game 007" {
			t.Errorf("featured %s: installed or not installable while others are", g.Title)
		}
	}
}

func TestHomeFeaturedEncodesAsEmptyListWithoutGames(t *testing.T) {
	h := NewView(nil).Home(nil, StateUnavailable, Status{}, nil)
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"featured":[]`) {
		t.Errorf("encoded home: %s", b)
	}
}

func TestHomeFeaturedLeavesOutGamesTheExcludeFunctionNames(t *testing.T) {
	inLibrary := func(s GameSummary) bool { return s.Title == "Game 000" || s.Title == "Game 050" }
	h := testView().Home(ranks, StateOK, Status{}, inLibrary)
	if len(h.Featured) != featuredSize {
		t.Fatalf("featured: %d games", len(h.Featured))
	}
	for _, g := range h.Featured {
		if inLibrary(g) {
			t.Errorf("featured %s is installed in the library", g.Title)
		}
	}
}

func TestSummaryTellsBrowserOnlyAndAnnouncedGamesApart(t *testing.T) {
	browser := rec("elamigos", "Browser Game", "v1", 0)
	preview := rec("fitgirl", "Preview Game", "", 0)
	preview.Entry.ReleaseKind = "preview"
	torrent := rec("fitgirl", "Mixed Game", "v1", 0)
	mixed := rec("elamigos", "Mixed Game", "v1", 0)
	cases := []struct {
		name                  string
		g                     *Game
		browserOnly, announced bool
	}{
		{"browser", &Game{Title: "Browser Game", Records: []Record{browser}}, true, false},
		{"preview", &Game{Title: "Preview Game", Records: []Record{preview}}, false, true},
		{"mixed", &Game{Title: "Mixed Game", Records: []Record{torrent, mixed}}, false, false},
		{"announcement beside a browser release", &Game{Title: "Both", Records: []Record{preview, browser}}, true, false},
	}
	for _, c := range cases {
		s := c.g.Summary()
		if s.BrowserOnly != c.browserOnly || s.Announced != c.announced {
			t.Errorf("%s: browserOnly %v announced %v", c.name, s.BrowserOnly, s.Announced)
		}
	}
}
