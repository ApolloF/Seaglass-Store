package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/store/sources"
)

func TestASearchAskedToWaitDefersEveryRequestToThatSourceUntilRetryAfter(t *testing.T) {
	ix, dir := testIndex(t)
	f := newFakeSource(t, "fitgirl", 5)
	clk := &clock{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	search, err := f.src.SearchURL("Game")
	if err != nil {
		t.Fatal(err)
	}
	f.fail[search] = &sources.HTTPError{Status: 429, RetryAfter: "3600"}
	if _, err := SearchSource(context.Background(), ix, f.src, f, "Game", clk.now()); !Unanswered(err) {
		t.Fatalf("a 429 counts as an answer: %v", err)
	}
	ix = reopen(dir) // the wait outlives a restart
	if c := ix.Crawl("fitgirl"); !c.RetryAt.Equal(clk.now().Add(time.Hour)) || c.Failures != 1 {
		t.Fatalf("after the 429: %+v", c)
	}
	asked := len(f.order)
	clk.add(59 * time.Minute)
	if _, err := SearchSource(context.Background(), ix, f.src, f, "Game", clk.now()); !Unanswered(err) {
		t.Errorf("a search before Retry-After: %v", err)
	}
	if res := Pass(context.Background(), ix, f.src, f, true, clk.now, PassHooks{}); res.Pages != 0 {
		t.Errorf("a pass before Retry-After: %+v", res)
	}
	if len(f.order) != asked {
		t.Fatalf("asked before Retry-After: %v", f.order[asked:])
	}
	clk.add(time.Minute)
	// The fake source has no search page: a 404 is an answer, not a wait.
	if _, err := SearchSource(context.Background(), ix, f.src, f, "Game", clk.now()); err == nil || Unanswered(err) || len(f.order) != asked+1 {
		t.Errorf("after Retry-After: %v, %d requests", err, len(f.order)-asked)
	}
}

func TestAPassStopsWhenASearchMadeTheSourceAskToWait(t *testing.T) {
	ix, _ := testIndex(t)
	f := newFakeSource(t, "fitgirl", 95)
	clk := &clock{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	page3, page4 := "https://fitgirl-repacks.site/page/3/", "https://fitgirl-repacks.site/page/4/"
	f.before = func(raw string) {
		if raw == page3 {
			if err := backOff(ix, "fitgirl", &sources.HTTPError{Status: 429, RetryAfter: "600"}, clk.now()); err != nil {
				t.Error(err)
			}
		}
	}
	res := Pass(context.Background(), ix, f.src, f, true, clk.now, PassHooks{})
	if res.Stopped != StopWaiting || res.More || f.count(page4) != 0 {
		t.Fatalf("the pass went on: %+v, page 4 asked %d times", res, f.count(page4))
	}
	// Page 3 came back and is kept, but its answer doesn't lift the wait.
	if c := ix.Crawl("fitgirl"); c.NextPage != 4 || !c.RetryAt.Equal(clk.now().Add(10*time.Minute)) || ix.Counts()["fitgirl"] != 30 {
		t.Errorf("after the pass: %+v, %d records", c, ix.Counts()["fitgirl"])
	}
}
