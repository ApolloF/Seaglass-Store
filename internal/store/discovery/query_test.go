package discovery

import (
	"fmt"
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
	h := testView().Home(ranks, StateOK, Status{})
	if len(h.New) != ShelfSize || h.New[0].Title != "Game 000" || h.New[1].Title != "Game 001" {
		t.Errorf("new: %d, %s", len(h.New), h.New[0].Title)
	}
	if len(h.Popular) != 3 || h.Popular[0].PopularRank != 1 {
		t.Errorf("popular: %+v", h.Popular)
	}
	if len(h.Updated) != 1 || h.Updated[0].Title != "Game 077" {
		t.Errorf("updated: %+v", h.Updated)
	}
	if h := testView().Home(ranks, StateUnavailable, Status{}); len(h.Popular) != 0 || h.PopularState != StateUnavailable {
		t.Error("without a chart the Popular shelf showed something")
	}
}
