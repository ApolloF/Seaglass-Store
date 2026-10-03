package sources

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func elAmigosFixtureSource(t *testing.T) Source {
	t.Helper()
	return sourceFor(t, "elamigos")
}

func elAmigosFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "elamigos", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func parseElAmigosFixture(t *testing.T, page string, data []byte) ([]Entry, error) {
	t.Helper()
	return Parse(elAmigosFixtureSource(t), page, data)
}

func TestElAmigosCatalogDeduplicatesAndPreservesDetailURLs(t *testing.T) {
	entries, err := parseElAmigosFixture(t, "https://elamigos.site/", elAmigosFixture(t, "catalog.html"))
	if err != nil || len(entries) != 3 {
		t.Fatalf("entries=%+v err=%v", entries, err)
	}
	first := entries[0]
	if first.Title != "Control Resonant Deluxe Edition" || first.Version != "" || !first.SummaryOnly || !first.NeedsReview || first.UpdatedAt == nil || first.UpdatedAt.Format("2006-01-02") != "2026-10-02" || first.PublishedAt != nil {
		t.Fatalf("catalog claims=%+v", first)
	}
	if first.PageURL != "https://elamigos.site/data/Control_Resonant_Deluxe_Edition_MULTi15_-_ElAmigos.html" || first.ID != EntryID("elamigos", first.PageURL) {
		t.Fatalf("detail identity=%+v", first)
	}
	if entries[2].UpdatedAt != nil {
		t.Fatal("alphabetic index inherited a news batch date")
	}
	if len(Search(entries, "Red Dead")) != 1 {
		t.Fatal("local catalog search failed")
	}
}

func TestElAmigosBaseInstallerKeepsItsVersionAndSeparatePatchClaims(t *testing.T) {
	e, err := parseElAmigosFixture(t, "https://elamigos.site/data/Control_Resonant_Deluxe_Edition_MULTi15_-_ElAmigos.html", elAmigosFixture(t, "base-with-patches.html"))
	if err != nil || len(e) != 1 {
		t.Fatalf("%+v %v", e, err)
	}
	r := e[0]
	if r.Version != "1.3.3" || r.ReleaseKind != "release" || r.SummaryOnly || r.SizeBytes != 76863000000 || r.UpdatedAt == nil || r.UpdatedAt.Format("2006-01-02") != "2026-09-24" || r.PublishedAt != nil {
		t.Fatalf("base claims=%+v", r)
	}
	if !strings.Contains(r.LanguageClaim, "English") || !strings.Contains(r.LanguageClaim, "Audio:") || len(r.References) != 4 || len(r.Transports) != 0 {
		t.Fatalf("languages/transports=%+v", r)
	}
	warnings := strings.Join(r.Warnings, "\n")
	if !strings.Contains(warnings, "Separate patch claim: Control Resonant update 1.3.3 - 1.4.0") || !strings.Contains(warnings, "Separate patch claim: Control Resonant update 1.4.0 - 1.4.1") {
		t.Fatalf("patch claims lost: %s", warnings)
	}
	for _, ref := range r.References {
		if ref.Kind != "download" || ref.State != "manual-required" {
			t.Fatalf("unsupported host automated: %+v", ref)
		}
		if _, err := ResolverURL(ref); err == nil {
			t.Fatal("file-host container accepted by torrent resolver")
		}
	}
}

func TestElAmigosOlderReleaseIgnoresIntroPatchAndInstalledSizeClaims(t *testing.T) {
	e, err := parseElAmigosFixture(t, "https://elamigos.site/data/Red_Dead_Redemption_2_MULTi13__ElAmigos_-_KnPzu8CD.html", elAmigosFixture(t, "older-release.html"))
	if err != nil || len(e) != 1 || e[0].Version != "1311.23" || len(e[0].References) != 4 || e[0].SizeBytes != 105565000000 || e[0].InstalledSizeBytes != 0 {
		t.Fatalf("%+v %v", e, err)
	}
	for _, ref := range e[0].References {
		if strings.Contains(ref.URL, "D113AED626") {
			t.Fatal("intro patch presented as base installer")
		}
	}
}

func TestElAmigosPreviewHasNoAvailableReleaseReferences(t *testing.T) {
	e, err := parseElAmigosFixture(t, "https://elamigos.site/data/Example_-_ElAmigos.html", elAmigosFixture(t, "preview.html"))
	if err != nil || len(e) != 1 || e[0].ReleaseKind != "preview" || len(e[0].References) != 0 || len(e[0].Transports) != 0 {
		t.Fatalf("%+v %v", e, err)
	}
}

func TestElAmigosComingSoonInADescriptionDoesNotMakeAReleaseAPreview(t *testing.T) {
	data := `<h2>Example (2026), 2GB</h2><h3>ElAmigos release. Updated to version 1.2 (01.10.2026).</h3>` +
		`<p>A co-op campaign for four. Online mode coming soon; ranked play not yet available.</p>` +
		`<h2>DDOWNLOAD</h2><a href="https://filecrypt.cc/Container/BASE.html">base</a>`
	e, err := parseElAmigosFixture(t, "https://elamigos.site/data/Example.html", []byte(data))
	if err != nil || len(e) != 1 || e[0].ReleaseKind != "release" || e[0].Version != "1.2" || len(e[0].References) != 1 {
		t.Fatalf("%+v %v", e, err)
	}
	// The label in the heading or the release claim still marks a preview.
	for _, data := range []string{
		`<h2>Example (2027), coming soon</h2><h3>ElAmigos release.</h3><h2>DDOWNLOAD</h2><a href="https://filecrypt.cc/Container/X.html">x</a>`,
		`<h2>Example (2027)</h2><h3>ElAmigos release, preview only.</h3><h2>DDOWNLOAD</h2><a href="https://filecrypt.cc/Container/X.html">x</a>`,
	} {
		if e, err := parseElAmigosFixture(t, "https://elamigos.site/data/Example.html", []byte(data)); err != nil || e[0].ReleaseKind != "preview" || len(e[0].References) != 0 {
			t.Errorf("%s: %+v %v", data, e, err)
		}
	}
}

func TestElAmigosCatalogSkipsAFewUnreadableRows(t *testing.T) {
	var b strings.Builder
	for i := range 20 {
		fmt.Fprintf(&b, `<h3>Game %d ElAmigos <a href="/data/Game_%d.html">DOWNLOAD</a></h3>`, i, i)
	}
	odd := `<h3>Odd Link ElAmigos <a href="/data/sub/Odd.html?x=1">DOWNLOAD</a></h3>` +
		`<h3>!!! ElAmigos <a href="/data/No_Title.html">DOWNLOAD</a></h3>`
	e, err := parseElAmigosFixture(t, "https://elamigos.site/", []byte(b.String()+odd))
	if err != nil || len(e) != 20 {
		t.Fatalf("two odd rows among 22: %d entries, %v", len(e), err)
	}
	// More than a quarter unreadable: the layout changed.
	for range 6 {
		b.WriteString(odd)
	}
	if e, err := parseElAmigosFixture(t, "https://elamigos.site/", []byte(b.String())); err == nil {
		t.Fatalf("12 odd rows among 32 accepted: %d entries", len(e))
	}
}

func TestElAmigosPatchLinksDoNotBecomeStandaloneInstallerLinks(t *testing.T) {
	data := `<h2>Example (2026), 2GB</h2><h3>ElAmigos release. Updated to version 1.0 (01.10.2026).</h3><h2>DDOWNLOAD</h2><a href="https://filecrypt.cc/Container/BASE.html">base</a><h2>Example update 1.0 - 2.0 (02.10.2026), 3GB</h2><h2>RAPIDGATOR</h2><a href="https://filecrypt.cc/Container/PATCH.html">patch</a><h3>Languages: Patch language</h3>`
	e, err := parseElAmigosFixture(t, "https://elamigos.site/data/Example.html", []byte(data))
	if err != nil || len(e) != 1 || e[0].Version != "1.0" || e[0].SizeBytes != 2000000000 || len(e[0].References) != 1 || e[0].LanguageClaim != "" || strings.Contains(e[0].References[0].URL, "PATCH") {
		t.Fatalf("%+v %v", e, err)
	}
}

func TestElAmigosMissingOptionalMetadataStaysUnknown(t *testing.T) {
	data := `<h2>Example (2026)</h2><h3>ElAmigos release.</h3><h2>DDOWNLOAD</h2><a href="https://filecrypt.cc/Container/BASE.html">base</a>`
	e, err := parseElAmigosFixture(t, "https://elamigos.site/data/Example.html", []byte(data))
	if err != nil || len(e) != 1 || e[0].Version != "" || e[0].SizeBytes != 0 || e[0].LanguageClaim != "" || e[0].UpdatedAt != nil || len(e[0].References) != 1 {
		t.Fatalf("%+v %v", e, err)
	}
}

func TestElAmigosUpdateOnlyReleaseCannotClaimStandaloneAvailability(t *testing.T) {
	data := `<h2>Example update 1.0 - 2.0 (2026), 3GB</h2><h3>ElAmigos release.</h3><h2>DDOWNLOAD</h2><a href="https://filecrypt.cc/Container/PATCH.html">patch</a>`
	e, err := parseElAmigosFixture(t, "https://elamigos.site/data/Example.html", []byte(data))
	if err != nil || len(e) != 1 || e[0].ReleaseKind != "update" || len(e[0].Transports) != 0 {
		t.Fatalf("%+v %v", e, err)
	}
}

func TestElAmigosRejectsChangedLayoutsChallengesAndHostileURLs(t *testing.T) {
	for _, page := range []string{"https://elamigos.site.evil.example/data/Example.html", "https://elamigos.site@127.0.0.1/data/Example.html", "https://elamigos.site:444/data/Example.html", "http://elamigos.site/data/Example.html", "https://elamigos.site/data/Example.html#x", "https://elamigos.site/page/2/"} {
		if _, err := parseElAmigosFixture(t, page, elAmigosFixture(t, "base-release.html")); err == nil {
			t.Fatalf("unsafe/unsupported page accepted: %s", page)
		}
	}
	for _, data := range []string{`<h2>Example (2026), 2GB</h2>`, `<article>New layout</article>`, `<div class="cf-turnstile"></div>`, `<h3>Example ElAmigos <a href="https://evil.example/data/Example.html">DOWNLOAD</a></h3>`, `<h3>Example ElAmigos <a href="/data/Example.html?redirect=evil">DOWNLOAD</a></h3>`} {
		if _, err := parseElAmigosFixture(t, "https://elamigos.site/", []byte(data)); err == nil {
			t.Fatalf("changed/unsafe layout accepted: %s", data)
		}
	}
}

func TestElAmigosRejectsHostileReleaseReferences(t *testing.T) {
	var links strings.Builder
	for _, target := range []string{"javascript:alert(1)", "file:///C:/private", "https://127.0.0.1/a", "https://example.local/a", "https://user:pass@host.example/a", "https://filecrypt.cc:444/a", "https://filecrypt.cc/a#payload"} {
		fmt.Fprintf(&links, `<a href="%s">link</a>`, target)
	}
	data := `<h2>Example (2026), 2GB</h2><h3>ElAmigos release.</h3><h2>DDOWNLOAD</h2>` + links.String()
	e, err := parseElAmigosFixture(t, "https://elamigos.site/data/Example.html", []byte(data))
	if err != nil || len(e) != 1 || len(e[0].References) != 0 || len(e[0].Transports) != 0 {
		t.Fatalf("%+v %v", e, err)
	}
}

func TestElAmigosCatalogSupportsThousandsOfEntriesWithABound(t *testing.T) {
	var b strings.Builder
	for i := 0; i <= maxElAmigosEntries; i++ {
		fmt.Fprintf(&b, `<h3>Game %d ElAmigos <a href="/data/Game_%d.html">DOWNLOAD</a></h3>`, i, i)
		if i == 3499 {
			e, err := parseElAmigosFixture(t, "https://elamigos.site/", []byte(b.String()))
			if err != nil || len(e) != 3500 {
				t.Fatalf("large finite catalog: count=%d err=%v", len(e), err)
			}
		}
	}
	if _, err := parseElAmigosFixture(t, "https://elamigos.site/", []byte(b.String())); err == nil {
		t.Fatal("unbounded catalog accepted")
	}
}

func TestElAmigosUsesBoundedHTTPAndETagWithoutFollowingFileHosts(t *testing.T) {
	s := elAmigosFixtureSource(t)
	c, err := NewClient(s.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	calls := 0
	c.http.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		c.last = time.Time{}
		if req.URL.Hostname() != s.Host || !strings.HasSuffix(req.URL.Path, ".html") {
			t.Fatalf("unexpected request: %s", req.URL)
		}
		if calls == 2 {
			if req.Header.Get("If-None-Match") != `"one"` {
				t.Fatal("ETag lost")
			}
			return testResponse(304, ""), nil
		}
		resp := testResponse(200, string(elAmigosFixture(t, "base-release.html")))
		resp.Header.Set("ETag", `"one"`)
		return resp, nil
	})
	for i := 0; i < 2; i++ {
		doc, err := c.FetchDocument(context.Background(), "https://elamigos.site/data/Example.html")
		if err != nil {
			t.Fatal(err)
		}
		e, err := parseElAmigosFixture(t, doc.URL, doc.Body)
		if err != nil || len(e) != 1 || len(e[0].References) != 4 {
			t.Fatalf("%+v %v", e, err)
		}
	}
	if calls != 2 {
		t.Fatalf("unexpected network requests: %d", calls)
	}
}

func TestElAmigosRegistryDeclaresFiniteBrowserOnlyOptInProvider(t *testing.T) {
	p, ok := Lookup("elamigos")
	if !ok || !p.Discovery || p.DefaultOn || p.Torrents || p.Paged() || p.Feed != "" || p.Search != "" || len(p.Notes) == 0 {
		t.Fatalf("provider=%+v", p)
	}
	if raw, ok := p.ListingURL(1); !ok || raw != p.StartURL {
		t.Fatal("catalog URL differs from entry point")
	}
	if _, ok := p.ListingURL(2); ok {
		t.Fatal("finite catalog has a second page")
	}
	if _, err := p.SearchURL("Example"); err == nil {
		t.Fatal("invented server-side search")
	}
	if _, err := NewClient("elamigos", false); err != ErrDisabled {
		t.Fatalf("opt-in bypassed: %v", err)
	}
	for _, id := range DefaultIDs() {
		if id == "elamigos" {
			t.Fatal("new source enabled by default")
		}
	}
}

func TestElAmigosCollectorStopsAfterFiniteCatalogAndEnrichesMatchingDetail(t *testing.T) {
	c, err := NewClient("elamigos", true)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	calls := 0
	c.http.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		c.last = time.Time{}
		name := "catalog.html"
		if calls == 2 && strings.HasSuffix(req.URL.Path, ".html") {
			name = "base-with-patches.html"
		} else if calls != 1 {
			t.Fatalf("unexpected page or file-host request: %s", req.URL)
		}
		resp := testResponse(200, string(elAmigosFixture(t, name)))
		resp.Request = req
		return resp, nil
	})
	snapshot, err := Collect(context.Background(), c, c.source.StartURL, CollectOptions{Pages: 5, Details: 1, Query: "Control Resonant", Resolve: 3})
	if err != nil || calls != 2 || len(snapshot.Entries) != 1 || len(snapshot.Documents) != 2 || snapshot.Entries[0].SummaryOnly || snapshot.Entries[0].Version != "1.3.3" || len(snapshot.Entries[0].Transports) != 0 {
		t.Fatalf("calls=%d snapshot=%+v err=%v", calls, snapshot, err)
	}
	if snapshot.Entries[0].DocumentSHA256 != snapshot.Documents[1].SHA256 {
		t.Fatal("detail provenance lost")
	}
	if next, err := NextPage(c.source, c.source.StartURL, []byte(`<a rel="next" href="/page/2/">Next</a>`)); err != nil || next != "" {
		t.Fatalf("finite catalog followed pagination: %s %v", next, err)
	}
}

func FuzzElAmigosParse(f *testing.F) {
	for _, name := range []string{"catalog.html", "base-with-patches.html", "preview.html"} {
		data, err := os.ReadFile(filepath.Join("testdata", "elamigos", name))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data, name == "catalog.html")
	}
	f.Fuzz(func(t *testing.T, data []byte, catalog bool) {
		page := "https://elamigos.site/data/Example.html"
		if catalog {
			page = "https://elamigos.site/"
		}
		entries, err := Parse(elAmigosFixtureSource(t), page, data)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.TitleKey == "" || !e.NeedsReview || len(e.DocumentSHA256) != 64 || len(e.Transports) != 0 {
				t.Fatalf("invalid parsed entry: %+v", e)
			}
			for _, ref := range e.References {
				if _, err := referenceURL(ref.URL); err != nil || ref.State != "manual-required" {
					t.Fatalf("invalid parsed reference: %+v", ref)
				}
			}
		}
	})
}
