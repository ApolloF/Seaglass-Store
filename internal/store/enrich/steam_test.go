package enrich

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPopularityMapsChartRanksByAppID(t *testing.T) {
	c, fn, _ := newTestClient(t, steamRouter(t))
	p := c.Popularity(context.Background())
	if p.State != StateOK || p.Error != "" {
		t.Fatalf("state %q error %q", p.State, p.Error)
	}
	if len(p.Ranks) != 5 || p.Ranks[730] != 1 || p.Ranks[570] != 2 || p.Ranks[578080] != 3 {
		t.Fatalf("ranks %v", p.Ranks)
	}
	if _, ok := p.Ranks[620]; ok {
		t.Fatal("a game off the chart has no rank")
	}
	if p.FetchedAt == 0 {
		t.Fatal("FetchedAt not set")
	}
	if r := fn.last(); r.Host != "api.steampowered.com" || r.Method != "GET" {
		t.Fatalf("asked %s %s", r.Method, r.URL)
	}
}

func TestPopularityEmptyChartDoesNotReplaceTheCachedOne(t *testing.T) {
	empty := []byte(`{"response":{"ranks":[]}}`)
	good := fixture(t, "chart.json")
	var body = good
	c, _, clk := newTestClient(t, func(recorded) answer { return ok(body) })
	if p := c.Popularity(context.Background()); p.State != StateOK {
		t.Fatalf("first: %q", p.State)
	}
	clk.Advance(PopularityTTL + 1)
	body = empty
	p := c.Popularity(context.Background())
	if p.State != StateStale || p.Ranks[730] != 1 || p.Error == "" {
		t.Fatalf("got %q %v %q, want the old chart marked stale", p.State, p.Ranks, p.Error)
	}
}

func TestCachedPopularityIsUnavailableUntilFetched(t *testing.T) {
	c, fn, _ := newTestClient(t, steamRouter(t))
	p := c.CachedPopularity()
	if p.State != StateUnavailable || p.Ranks == nil || len(p.Ranks) != 0 {
		t.Fatalf("before: %+v", p)
	}
	c.Popularity(context.Background())
	n := fn.count()
	p = c.CachedPopularity()
	if p.State != StateOK || p.Ranks[730] != 1 || fn.count() != n {
		t.Fatalf("after: %+v, requests %d -> %d", p, n, fn.count())
	}
}

func TestReviewSummaryReadsOverallAndRecentScores(t *testing.T) {
	c, fn, _ := newTestClient(t, steamRouter(t))
	s := c.ReviewSummary(context.Background(), 620)
	if s.State != StateOK {
		t.Fatalf("state %q %q", s.State, s.Error)
	}
	if s.Overall.Label != "Overwhelmingly Positive" || s.Overall.Total != 467426 || s.Overall.Percent != 98 {
		t.Fatalf("overall %+v", s.Overall)
	}
	// The histogram fixture is thirty real days; recent is their sum.
	if s.Recent.Total <= 0 || s.Recent.Percent < 90 || s.Recent.Percent > 100 {
		t.Fatalf("recent %+v", s.Recent)
	}
	if s.Recent.Label != "" {
		t.Fatalf("Steam gives no recent label, got %q", s.Recent.Label)
	}
	if s.URL != "https://steamcommunity.com/app/620/reviews/" || s.AppID != 620 {
		t.Fatalf("url %q app %d", s.URL, s.AppID)
	}
	if fn.countPath("/appreviews/620") != 1 || fn.countPath("appreviewhistogram/620") != 1 {
		t.Fatalf("requests %d", fn.count())
	}
	if q := fn.find("/appreviews/620")[0].Query; !strings.Contains(q, "language=all") || !strings.Contains(q, "num_per_page=0") {
		t.Fatalf("summary query %q", q)
	}
}

func TestReviewSummaryWithoutRecentKeepsTheOverallScore(t *testing.T) {
	for name, a := range map[string]answer{
		"empty last 30 days": ok(fixture(t, "histogram_empty.json")),
		"histogram refused":  status(500),
		"histogram garbled":  ok([]byte("<html>")),
	} {
		t.Run(name, func(t *testing.T) {
			base := steamRouter(t)
			c, _, _ := newTestClient(t, func(r recorded) answer {
				if strings.Contains(r.Path, "appreviewhistogram") {
					return a
				}
				return base(r)
			})
			s := c.ReviewSummary(context.Background(), 620)
			if s.State != StateOK || s.Overall.Total != 467426 {
				t.Fatalf("%q %+v", s.State, s.Overall)
			}
			if s.Recent.Total != 0 || s.Recent.Percent != 0 || s.Recent.Label != "" {
				t.Fatalf("recent must stay empty, got %+v", s.Recent)
			}
		})
	}
}

func TestReviewSummaryForAGameWithoutReviews(t *testing.T) {
	c, _, _ := newTestClient(t, func(r recorded) answer {
		if strings.Contains(r.Path, "appreviewhistogram") {
			return ok(fixture(t, "histogram_empty.json"))
		}
		return ok(fixture(t, "summary_none.json"))
	})
	s := c.ReviewSummary(context.Background(), 99999999)
	if s.State != StateOK || s.Overall.Total != 0 || s.Overall.Label != "No user reviews" || s.Overall.Percent != 0 {
		t.Fatalf("%q %+v", s.State, s.Overall)
	}
}

func TestReviewSummaryWithoutAnAppIDMakesNoRequest(t *testing.T) {
	c, fn, _ := newTestClient(t, steamRouter(t))
	if s := c.ReviewSummary(context.Background(), 0); s.State != StateUnavailable || fn.count() != 0 {
		t.Fatalf("%q, %d requests", s.State, fn.count())
	}
}

func TestReviewsFirstPageHasPlainTextAndAttribution(t *testing.T) {
	c, fn, _ := newTestClient(t, steamRouter(t))
	p := c.Reviews(context.Background(), ReviewQuery{AppID: 620, Filter: ReviewsHelpful, Language: "english"})
	if p.State != StateOK || len(p.Reviews) != 3 {
		t.Fatalf("%q %d reviews", p.State, len(p.Reviews))
	}
	r := p.Reviews[0]
	if r.ID != "5001" || r.Author != "Test Tester" || !r.Recommended || r.Helpful != 21 || r.Funny != 2 ||
		r.PlaytimeAtReview != 1155 || r.PlaytimeForever != 1500 || r.Posted != 1788893804 || r.Language != "english" {
		t.Fatalf("review %+v", r)
	}
	if r.URL != "https://steamcommunity.com/profiles/76561199000000001/recommended/620/" {
		t.Fatalf("url %q", r.URL)
	}
	want := "Great\n\nReally good. Very good.\n\n• puzzles\n• story\n\nSee the guide (https://example.com/guide) and (spoiler: GLaDOS is the bad guy)."
	if r.Text != want {
		t.Fatalf("text %q\nwant %q", r.Text, want)
	}
	if got := p.Reviews[1]; got.Author != "76561199000000002" || got.Recommended {
		t.Fatalf("no persona name should show the SteamID, got %+v", got)
	}
	if p.Cursor != "AoIFQFYqVMAAAAB/page2==" {
		t.Fatalf("cursor %q", p.Cursor)
	}
	if p.More {
		t.Fatal("a short page is the last one")
	}
	q := fn.find("/appreviews/620")[0].Query
	for _, want := range []string{"filter=all", "language=english", "cursor=%2A", "num_per_page=20", "json=1"} {
		if !strings.Contains(q, want) {
			t.Fatalf("query %q lacks %q", q, want)
		}
	}
}

func TestReviewsFollowTheCursorAndFilters(t *testing.T) {
	var queries []url.Values
	full := fixture(t, "reviews_full_page.json")
	c, _, _ := newTestClient(t, func(r recorded) answer {
		q, _ := url.ParseQuery(r.Query)
		queries = append(queries, q)
		if q.Get("cursor") == "*" {
			return ok(full)
		}
		return ok(fixture(t, "reviews_page2.json"))
	})
	ctx := context.Background()
	first := c.Reviews(ctx, ReviewQuery{AppID: 620, Filter: ReviewsRecent, Language: "german"})
	if len(first.Reviews) != 20 || !first.More || first.Cursor != "AoIFQFYqVMAAAAB/page3==" {
		t.Fatalf("first page: %d reviews, more %v, cursor %q", len(first.Reviews), first.More, first.Cursor)
	}
	next := c.Reviews(ctx, ReviewQuery{AppID: 620, Filter: ReviewsRecent, Language: "german", Cursor: first.Cursor})
	if len(next.Reviews) != 0 || next.More || next.State != StateOK {
		t.Fatalf("next page: %+v", next)
	}
	if len(queries) != 2 {
		t.Fatalf("%d requests", len(queries))
	}
	if queries[0].Get("filter") != "recent" || queries[0].Get("language") != "german" {
		t.Fatalf("filters not sent: %v", queries[0])
	}
	if queries[1].Get("cursor") != "AoIFQFYqVMAAAAB/page3==" {
		t.Fatalf("the cursor must be sent back exactly, got %q", queries[1].Get("cursor"))
	}
}

func TestReviewsPageWithoutNewCursorEndsTheList(t *testing.T) {
	full := fixture(t, "reviews_full_page.json")
	c, _, _ := newTestClient(t, func(recorded) answer { return ok(full) })
	// The fixture's cursor equals the one asked with: Steam repeats it at the end.
	p := c.Reviews(context.Background(), ReviewQuery{AppID: 620, Cursor: "AoIFQFYqVMAAAAB/page3=="})
	if p.More {
		t.Fatal("the same cursor again means there is no more")
	}
}

func TestReviewsEmptyAndStarCursorsShareOnePage(t *testing.T) {
	c, fn, _ := newTestClient(t, steamRouter(t))
	ctx := context.Background()
	c.Reviews(ctx, ReviewQuery{AppID: 620})
	c.Reviews(ctx, ReviewQuery{AppID: 620, Cursor: "*", Filter: ReviewsHelpful})
	c.Reviews(ctx, ReviewQuery{AppID: 620, Filter: "nonsense", Language: "Not A Language"})
	if fn.count() != 1 {
		t.Fatalf("%d requests; \"\" and \"*\", an unknown filter and a bad language all mean the first page", fn.count())
	}
	c.Reviews(ctx, ReviewQuery{AppID: 620, Filter: ReviewsRecent})
	if fn.count() != 2 {
		t.Fatalf("a different filter is a different page, requests %d", fn.count())
	}
}

func TestReviewsSendOnlyCleanedLanguageAndCursor(t *testing.T) {
	c, fn, _ := newTestClient(t, steamRouter(t))
	c.Reviews(context.Background(), ReviewQuery{AppID: 620, Language: "english&filter=evil", Cursor: "a b&x=1"})
	q, _ := url.ParseQuery(fn.last().Query)
	if q.Get("language") != "all" || q.Get("cursor") != "a b&x=1" || q.Get("x") != "" {
		t.Fatalf("query %v", q)
	}
}

func TestPlainTextTurnsBBCodeIntoReadableText(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"bold and italic": {"[b]bold[/b] and [i]italic[/i] [u]u[/u] [strike]s[/strike]", "bold and italic u s"},
		"heading":         {"[h1]Title[/h1]\nbody", "Title\n\nbody"},
		"link with text":  {"[url=https://a.example/x]link[/url]", "link (https://a.example/x)"},
		"bare link":       {"[url]https://a.example/[/url]", "https://a.example/"},
		"link to itself":  {"[url=https://a.example/]https://a.example/[/url]", "https://a.example/"},
		"script link":     {"[url=javascript:alert(1)]click[/url]", "click"},
		"spoiler":         {"[spoiler]secret[/spoiler]", "(spoiler: secret)"},
		"list":            {"[list][*]one[*]two[/list]", "• one\n• two"},
		"quote":           {"[quote=Bob]hi[/quote]", "Bob wrote: “hi”"},
		"anonymous quote": {"[quote]hi[/quote]", "“hi”"},
		"image":           {"[img]https://x.example/y.png[/img]text", "text"},
		"steam image":     {"{STEAM_CLAN_IMAGE}/3703047/abc.png text", "text"},
		"table":           {"[table][tr][td]a[/td][td]b[/td][/tr][/table]", "a | b |"},
		"noparse":         {"[noparse][b][/noparse]x", "x"},
		"unknown bracket": {"[Story 9/10] fine [b]really[/b]", "[Story 9/10] fine really"},
		"html tags":       {"<b>hi</b> <a href=\"x\">there</a>", "hi there"},
		"html script":     {"before<script>alert(1)</script>after", "beforeafter"},
		"less than":       {"I <3 this, 1 < 2", "I <3 this, 1 < 2"},
		"blank lines":     {"a\r\n\r\n\r\n\r\nb", "a\n\nb"},
		"control chars":   {"a\x00b\x07c", "abc"},
		"unclosed tag":    {"[url=https://a.example/x]no end", "no end"},
		"emoji":           {"ok 👍🏽 👨‍👩‍👧", "ok 👍🏽 👨‍👩‍👧"},
		"only formatting": {"[b][/b]", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := plainText(tc.in, 8000); got != tc.want {
				t.Fatalf("plainText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPlainTextCapsVeryLongReviewsOnWholeCharacters(t *testing.T) {
	got := plainText(strings.Repeat("é", 9000), maxReviewText)
	if !utf8.ValidString(got) || utf8.RuneCountInString(got) != maxReviewText+1 || !strings.HasSuffix(got, "…") {
		t.Fatalf("%d characters, valid %v", utf8.RuneCountInString(got), utf8.ValidString(got))
	}
	short := strings.Repeat("a", maxReviewText)
	if plainText(short, maxReviewText) != short {
		t.Fatal("text at the cap is left alone")
	}
}

func TestCriticReturnsTheScoreAndLinkSteamGives(t *testing.T) {
	c, fn, _ := newTestClient(t, steamRouter(t))
	cr := c.Critic(context.Background(), 620)
	if cr.State != StateOK || cr.Score != 95 || cr.AppID != 620 || cr.FetchedAt == 0 {
		t.Fatalf("%+v", cr)
	}
	if cr.URL != "https://www.metacritic.com/game/pc/portal-2?ftag=MCD-06-10aaa1f" {
		t.Fatalf("url %q", cr.URL)
	}
	r := fn.last()
	if r.Host != "store.steampowered.com" || !strings.Contains(r.Query, "appids=620") || !strings.Contains(r.Query, "filters=metacritic") {
		t.Fatalf("asked %s", r.URL)
	}
	if fn.countPath("metacritic") != 0 {
		t.Fatal("Metacritic itself is never fetched")
	}
}

func TestCriticWithoutAScoreIsOKAndEmpty(t *testing.T) {
	for name, body := range map[string]string{
		"no metacritic entry":  "appdetails_none.json",
		"app unknown to steam": "appdetails_unknown.json",
	} {
		t.Run(name, func(t *testing.T) {
			id := 730
			if name == "app unknown to steam" {
				id = 99999999
			}
			c, _, _ := newTestClient(t, func(recorded) answer { return ok(fixture(t, body)) })
			cr := c.Critic(context.Background(), id)
			if cr.State != StateOK || cr.Score != 0 || cr.URL != "" {
				t.Fatalf("%+v", cr)
			}
		})
	}
}

func TestCriticDropsALinkThatIsNotMetacritic(t *testing.T) {
	body := `{"620":{"success":true,"data":{"metacritic":{"score":80,"url":"https://evil.example/portal-2"}}}}`
	c, _, _ := newTestClient(t, func(recorded) answer { return ok([]byte(body)) })
	cr := c.Critic(context.Background(), 620)
	if cr.Score != 80 || cr.URL != "" {
		t.Fatalf("%+v", cr)
	}
}

func TestCriticAnswerForAnotherAppIsAFormatError(t *testing.T) {
	c, _, _ := newTestClient(t, func(recorded) answer { return ok(fixture(t, "appdetails_metacritic.json")) })
	if cr := c.Critic(context.Background(), 730); cr.State != StateError {
		t.Fatalf("%+v", cr)
	}
}
