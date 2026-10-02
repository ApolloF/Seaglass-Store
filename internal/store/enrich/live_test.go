package enrich

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveProviders asks the real providers a handful of questions about
// Portal 2 (about ten requests, spaced as in the app):
//
//	WL_STORE_LIVE=1 go test -run Live -v ./internal/store/enrich
func TestLiveProviders(t *testing.T) {
	if os.Getenv("WL_STORE_LIVE") == "" {
		t.Skip("set WL_STORE_LIVE=1 to ask the real providers")
	}
	const portal2 = 620
	c := New(Options{Dir: t.TempDir()})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pop := c.Popularity(ctx)
	t.Logf("chart: state %s, %d games, top: %v", pop.State, len(pop.Ranks), topRanks(pop.Ranks, 3))
	if pop.State != StateOK || len(pop.Ranks) < 50 {
		t.Errorf("chart %+v", pop)
	}

	sum := c.ReviewSummary(ctx, portal2)
	t.Logf("summary: state %s, overall %+v, recent %+v, %s", sum.State, sum.Overall, sum.Recent, sum.URL)
	if sum.State != StateOK || sum.Overall.Total < 1000 || sum.Overall.Label == "" {
		t.Errorf("summary %+v", sum)
	}
	if sum.Recent.Total == 0 {
		t.Logf("Steam gave no recent score")
	}

	page := c.Reviews(ctx, ReviewQuery{AppID: portal2, Filter: ReviewsHelpful, Language: "english"})
	t.Logf("reviews page 1: state %s, %d reviews, more %v, cursor %q", page.State, len(page.Reviews), page.More, page.Cursor)
	if page.State != StateOK || len(page.Reviews) == 0 {
		t.Fatalf("reviews %+v", page)
	}
	r := page.Reviews[0]
	t.Logf("first review: author %q recommended %v helpful %d funny %d playtime %d/%d min posted %d, %d chars, %s",
		r.Author, r.Recommended, r.Helpful, r.Funny, r.PlaytimeAtReview, r.PlaytimeForever, r.Posted, len(r.Text), r.URL)
	if r.Text == "" || r.URL == "" || r.Posted == 0 {
		t.Errorf("review %+v", r)
	}
	if page.More {
		next := c.Reviews(ctx, ReviewQuery{AppID: portal2, Filter: ReviewsHelpful, Language: "english", Cursor: page.Cursor})
		t.Logf("reviews page 2: state %s, %d reviews, first id %s (page 1 started with %s)", next.State, len(next.Reviews), firstID(next), r.ID)
		if next.State != StateOK || len(next.Reviews) == 0 || next.Reviews[0].ID == r.ID {
			t.Errorf("page 2 %+v", next)
		}
	}

	crit := c.Critic(ctx, portal2)
	t.Logf("critic: state %s, score %d, %s", crit.State, crit.Score, crit.URL)
	if crit.State != StateOK || crit.Score == 0 || crit.URL == "" {
		t.Errorf("critic %+v", crit)
	}

	hl := c.Completion(ctx, CompletionQuery{Title: "Portal 2", Year: 2011})
	t.Logf("HowLongToBeat search: state %s, id %d %q, main %d / +extras %d / 100%% %d min, %s", hl.State, hl.HLTBID, hl.Title, hl.Main, hl.MainExtras, hl.Completionist, hl.URL)
	if hl.State != StateOK || hl.HLTBID != 7231 || hl.Main == 0 {
		t.Errorf("completion %+v", hl)
	}
	byID := c.Completion(ctx, CompletionQuery{HLTBID: 7231})
	t.Logf("HowLongToBeat game page: state %s, id %d %q, main %d / +extras %d / 100%% %d min, corrected %v", byID.State, byID.HLTBID, byID.Title, byID.Main, byID.MainExtras, byID.Completionist, byID.Corrected)
	if byID.State != StateOK || byID.HLTBID != 7231 || !byID.Corrected || byID.Main == 0 {
		t.Errorf("completion by id %+v", byID)
	}
	if hl.Main != 0 && byID.Main != 0 && abs(hl.Main-byID.Main) > 5 {
		t.Errorf("search and game page disagree: %d vs %d minutes", hl.Main, byID.Main)
	}
	cands, err := c.Candidates(ctx, "Portal 2")
	t.Logf("candidates: %d, err %v (served from the search cache)", len(cands), err)
}

func topRanks(ranks map[int]int, n int) []int {
	out := make([]int, n)
	for id, r := range ranks {
		if r >= 1 && r <= n {
			out[r-1] = id
		}
	}
	return out
}

func firstID(p ReviewPage) string {
	if len(p.Reviews) == 0 {
		return ""
	}
	return p.Reviews[0].ID
}
