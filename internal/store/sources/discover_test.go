package sources

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSourceSearchEncodesTermsAndRejectsInvalidQueries(t *testing.T) {
	s := sourceFor(t, "fitgirl")
	got, err := s.SearchURL("Example & Game")
	if err != nil || got != "https://fitgirl-repacks.site/?s=Example+%26+Game" {
		t.Fatalf("%s %v", got, err)
	}
	for _, query := range []string{" ", strings.Repeat("x", 201)} {
		if _, err := s.SearchURL(query); err == nil {
			t.Fatal("invalid search accepted")
		}
	}
}

func TestSearchSummariesRetainIdentityForDetailEnrichment(t *testing.T) {
	for _, id := range []string{"fitgirl", "dodi"} {
		s := sourceFor(t, id)
		title := "Example Game – v1.2"
		if id == "dodi" {
			title = "123- Example Game (v1.2) [DODI Repack]"
		}
		markup := `<article class="category-lossless-repack"><h1 class="entry-title"><a href="/example/">` + title + `</a></h1><div class="entry-summary"><p>Excerpt only …</p></div><time class="published" datetime="2026-10-01T00:00:00Z"></time><time class="updated" datetime="2026-10-02T00:00:00Z"></time></article>`
		entries, err := Parse(s, "https://"+s.Host+"/?s=example", []byte(markup))
		if err != nil || len(entries) != 1 || !entries[0].SummaryOnly || entries[0].PublishedAt == nil || entries[0].UpdatedAt == nil || entries[0].Title != "Example Game" {
			t.Fatalf("%+v %v", entries, err)
		}
	}
}

func TestKnownEmptySearchIsNotAChallengeOrLayoutError(t *testing.T) {
	s := sourceFor(t, "fitgirl")
	data := `<body class="search search-no-results"><h1 class="page-title">Nothing Found</h1></body>`
	entries, err := Parse(s, "https://"+s.Host+"/?s=missing", []byte(data))
	if err != nil || len(entries) != 0 {
		t.Fatalf("%+v %v", entries, err)
	}
	if _, err := Parse(s, s.StartURL, []byte(strings.Replace(data, "Nothing Found", "Just a moment", 1))); err == nil {
		t.Fatal("unknown empty page accepted")
	}
	if _, err := Parse(s, s.StartURL, []byte(data+`<div class="cf-turnstile"></div>`)); err == nil {
		t.Fatal("challenge accepted")
	}
}

func TestPaginationStaysOnOriginAndPreservesFeedQuery(t *testing.T) {
	s := sourceFor(t, "fitgirl")
	next, err := NextPage(s, "https://"+s.Host+"/page/5/", []byte(`<a class="next page-numbers" href="/page/6/">Next</a>`))
	if err != nil || next != "https://"+s.Host+"/page/6/" {
		t.Fatalf("%s %v", next, err)
	}
	if _, err := NextPage(s, s.StartURL, []byte(`<a class="next" href="https://evil.example/">Next</a>`)); err != nil {
		t.Fatal("RSS endpoint should use its own pagination")
	}
	if _, err := NextPage(s, "https://"+s.Host+"/", []byte(`<a class="next" href="https://evil.example/">Next</a>`)); err == nil {
		t.Fatal("unsafe pagination accepted")
	}
	next, err = NextPage(s, s.StartURL+"?paged=2", nil)
	if err != nil || !strings.HasSuffix(next, "?paged=3") {
		t.Fatalf("%s %v", next, err)
	}
}

func TestReferencesPreserveHTTPButOnlyTorrentSectionsCanResolve(t *testing.T) {
	s := sourceFor(t, "dodi")
	markup := `<article><h1 class="entry-title">Example [DODI Repack]</h1><div class="entry-content"><p>Torrent – <a href="http://file-me.top/abcdefghijkl.html">Click Here</a></p><p>DataNodes – <a href="https://www.up-4ever.net/123456789012">Click Here</a></p></div></article>`
	entries, err := Parse(s, s.StartURL, []byte(markup))
	if err != nil {
		t.Fatal(err)
	}
	refs := entries[0].References
	if len(refs) != 2 || refs[0].Kind != "torrent" || refs[1].Kind != "download" || len(entries[0].Transports) != 0 {
		t.Fatalf("%+v", entries[0])
	}
	if _, err := ResolverURL(refs[1]); err == nil {
		t.Fatal("full game download reference accepted")
	}
}

func TestCollectorEnrichesSummariesAndPreservesEvidence(t *testing.T) {
	c, _ := NewClient("fitgirl", true)
	calls := 0
	c.http.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		c.last = time.Time{}
		markup := `<article class="category-lossless-repack"><h1 class="entry-title"><a href="/example/">Example v1.2</a></h1><div class="entry-summary">Excerpt</div></article>`
		if calls == 2 {
			markup = `<article><h1 class="entry-title">Example v1.2</h1><div class="entry-content"><p>Repack Size: 2 GB</p><a href="` + testMagnet + `">Magnet</a></div></article>`
		}
		resp := testResponse(200, markup)
		resp.Request = req
		return resp, nil
	})
	snapshot, err := Collect(context.Background(), c, "https://"+c.source.Host+"/?s=example", CollectOptions{Pages: 1, Details: 1})
	if err != nil || calls != 2 || len(snapshot.Documents) != 2 || snapshot.Entries[0].SummaryOnly || len(snapshot.Entries[0].Transports) != 1 {
		t.Fatalf("%+v %v calls=%d", snapshot, err, calls)
	}
	if snapshot.Entries[0].DocumentSHA256 != snapshot.Documents[1].SHA256 {
		t.Fatal("detail provenance lost")
	}
}

func TestCollectorStopsAtPaginationFailureAndLoop(t *testing.T) {
	for _, failure := range []bool{true, false} {
		c, _ := NewClient("dodi", true)
		calls := 0
		c.http.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			c.last = time.Time{}
			if calls == 2 && failure {
				return testResponse(429, ""), nil
			}
			href := "/page/2/"
			if !failure {
				href = "/"
			}
			resp := testResponse(200, `<article><h1 class="entry-title"><a href="/example/">Example [DODI Repack]</a></h1><div class="entry-content">Repack Size: 2 GB</div></article><a class="next" href="`+href+`">Next</a>`)
			resp.Request = req
			return resp, nil
		})
		snapshot, err := Collect(context.Background(), c, "https://"+c.source.Host+"/", CollectOptions{Pages: 3})
		if err != nil || len(snapshot.Entries) != 1 || len(snapshot.Warnings) != 1 {
			t.Fatalf("%+v %v", snapshot, err)
		}
	}
}

func TestDeepMarkupIsRejectedBeforeExpensiveTreeWalks(t *testing.T) {
	for _, data := range []string{strings.Repeat("<div>", maxNesting+1), strings.Repeat("<span>", maxNesting+1)} {
		if _, err := document([]byte(data)); err == nil {
			t.Fatal("deep markup accepted")
		}
	}
	if _, err := document([]byte(strings.Repeat("<br><img><input>", 1000))); err != nil {
		t.Fatal("void tags accumulated nesting", err)
	}
	if _, err := document([]byte(strings.Repeat(`<div><b class="different"></div>`, 65))); err == nil {
		t.Fatal("unclosed formatting reconstruction accepted")
	}
}
