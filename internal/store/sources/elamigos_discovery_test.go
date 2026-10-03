package sources_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/store/discovery"
	"github.com/ApolloF/Seaglass/internal/store/sources"
)

type elAmigosFetcher struct {
	t       *testing.T
	p       sources.Provider
	hits    []string
	failure error
}

func (f *elAmigosFetcher) FetchDocument(ctx context.Context, raw string) (sources.Document, error) {
	f.t.Helper()
	if err := ctx.Err(); err != nil {
		return sources.Document{}, err
	}
	f.hits = append(f.hits, raw)
	if f.failure != nil {
		return sources.Document{}, f.failure
	}
	name := "catalog.html"
	if raw != f.p.Listing {
		if !strings.HasSuffix(raw, "/Control_Resonant_Deluxe_Edition_MULTi15_-_ElAmigos.html") {
			f.t.Fatalf("unexpected source/file-host URL: %s", raw)
		}
		name = "base-with-patches.html"
	}
	data, err := os.ReadFile(filepath.Join("testdata", "elamigos", name))
	return sources.Document{URL: raw, Body: data, ContentType: "text/html"}, err
}

func TestElAmigosDiscoveryIndexesFiniteCatalogAndRetainsDetailIdentityAcrossRestart(t *testing.T) {
	p, ok := sources.Lookup("elamigos")
	if !ok {
		t.Fatal("ElAmigos is absent from registry")
	}
	dir := t.TempDir()
	indexDir, identities := filepath.Join(dir, "local", "discovery"), filepath.Join(dir, "roaming", "identity.json")
	ix := discovery.OpenIndex(indexDir, identities)
	f := &elAmigosFetcher{t: t, p: p}
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	result := discovery.Pass(context.Background(), ix, p, f, true, func() time.Time { return now }, discovery.PassHooks{})
	if result.Err != nil || result.Pages != 1 || len(f.hits) != 1 || ix.Counts()[p.ID] != 3 || !ix.Crawl(p.ID).BackfillDone {
		t.Fatalf("result=%+v hits=%v state=%+v", result, f.hits, ix.Crawl(p.ID))
	}
	page := "https://elamigos.site/data/Control_Resonant_Deluxe_Edition_MULTi15_-_ElAmigos.html"
	id := sources.EntryID(p.ID, page)
	if err := discovery.FetchDetail(context.Background(), ix, p.Source, f, id, now); err != nil {
		t.Fatal(err)
	}
	r, ok := ix.Record(p.ID, id)
	if !ok || r.Entry.ID != id || r.Entry.PageURL != page || r.Entry.SummaryOnly || r.Entry.Version != "1.3.3" {
		t.Fatalf("detail identity/claims=%+v", r)
	}
	release := discovery.ReleaseOf(r, p.Name)
	if release.Availability != discovery.AvailManual || release.Transports != 0 {
		t.Fatalf("browser-only release is installable: %+v", release)
	}
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}
	ix = discovery.OpenIndex(indexDir, identities)
	if ix.Counts()[p.ID] != 3 || !ix.Crawl(p.ID).BackfillDone {
		t.Fatal("finite catalog progress was not retained")
	}
	if r, ok := ix.Record(p.ID, id); !ok || r.Entry.PageURL != page || r.Entry.Version != "1.3.3" {
		t.Fatalf(".html identity changed on restart: %+v", r)
	}
	result = discovery.Pass(context.Background(), ix, p, f, false, func() time.Time { return now.Add(time.Hour) }, discovery.PassHooks{})
	if result.Err != nil || result.Pages != 0 || len(f.hits) != 2 {
		t.Fatalf("completed catalog requested another page: %+v %v", result, f.hits)
	}
	// A failed refresh retains every cached record and completed catalog progress.
	f.failure = errors.New("provider offline")
	result = discovery.Pass(context.Background(), ix, p, f, true, func() time.Time { return now.Add(7 * time.Hour) }, discovery.PassHooks{})
	if result.Err == nil || ix.Counts()[p.ID] != 3 || !ix.Crawl(p.ID).BackfillDone {
		t.Fatalf("failure discarded the cache: %+v", result)
	}
}

func TestElAmigosDiscoveryUsesExactTitleOrTrustedIdentityForCrossProviderMatching(t *testing.T) {
	p, _ := sources.Lookup("elamigos")
	data, err := os.ReadFile(filepath.Join("testdata", "elamigos", "base-release.html"))
	if err != nil {
		t.Fatal(err)
	}
	es, err := sources.Parse(p.Source, "https://elamigos.site/data/Resonance_A_Plague_Tale_Legacy_MULTi17_-_ElAmigos.html", data)
	if err != nil {
		t.Fatal(err)
	}
	e := es[0]
	other := e
	other.SourceID, other.PageURL = "fitgirl", "https://fitgirl-repacks.site/example/"
	other.ID = sources.EntryID(other.SourceID, other.PageURL)
	groups := discovery.Group([]discovery.Record{{Entry: e}, {Entry: other}}, nil, nil, nil)
	if len(groups) != 1 || len(groups[0].Records) != 2 || groups[0].Summary().Installable {
		t.Fatalf("exact identity matching failed: %+v", groups)
	}
	other.Title = "Resonance A Plague Tale Legacy 2"
	groups = discovery.Group([]discovery.Record{{Entry: e}, {Entry: other}}, nil, nil, nil)
	if len(groups) != 2 {
		t.Fatal("sequel matched without trusted identity")
	}
	other.Title = "Resonance: A Plague Tale Legacy Deluxe"
	identify := func(title string) (string, int) { return "Resonance: A Plague Tale Legacy", 123456 }
	groups = discovery.Group([]discovery.Record{{Entry: e}, {Entry: other}}, nil, nil, identify)
	if len(groups) != 1 || groups[0].AppID != 123456 || len(groups[0].Records) != 2 {
		t.Fatal("trusted identity failed to group providers")
	}
}

// elAmigosPages serves ElAmigos documents by URL, as the test changes them.
type elAmigosPages map[string]string

func (p elAmigosPages) FetchDocument(ctx context.Context, raw string) (sources.Document, error) {
	body, ok := p[raw]
	if !ok {
		return sources.Document{}, &sources.HTTPError{Status: 404}
	}
	return sources.Document{URL: raw, Body: []byte(body), ContentType: "text/html"}, nil
}

func elAmigosFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "elamigos", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A release read once is read again when its catalog row changes: the
// row names another update, or the announcement's label is gone.
func TestElAmigosReleaseIsReadAgainWhenItsCatalogRowChanges(t *testing.T) {
	p, _ := sources.Lookup("elamigos")
	dir := t.TempDir()
	ix := discovery.OpenIndex(filepath.Join(dir, "discovery"), filepath.Join(dir, "identity.json"))
	control := "https://elamigos.site/data/Control_Resonant_Deluxe_Edition_MULTi15_-_ElAmigos.html"
	upcoming := "https://elamigos.site/data/Upcoming_Game_MULTi9_-_ElAmigos.html"
	catalog := elAmigosFile(t, "catalog.html")
	upcomingRow := `<h3>Upcoming Game ElAmigos (preview) <a href="data/Upcoming_Game_MULTi9_-_ElAmigos.html">DOWNLOAD</a></h3>`
	pages := elAmigosPages{
		p.Listing: strings.Replace(catalog, "<!-- Index start -->", "<!-- Index start -->"+upcomingRow, 1),
		control:   elAmigosFile(t, "base-with-patches.html"),
		upcoming:  strings.Replace(elAmigosFile(t, "preview.html"), "Example Upcoming Game", "Upcoming Game", 1),
	}
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	refresh := func() discovery.PassResult {
		t.Helper()
		res := discovery.Pass(context.Background(), ix, p, pages, true, func() time.Time { return now }, discovery.PassHooks{})
		if res.Err != nil {
			t.Fatal(res.Err)
		}
		return res
	}
	read := func(page string) discovery.Record {
		t.Helper()
		id := sources.EntryID(p.ID, page)
		if err := discovery.FetchDetail(context.Background(), ix, p.Source, pages, id, now); err != nil {
			t.Fatal(err)
		}
		r, _ := ix.Record(p.ID, id)
		return r
	}
	record := func(page string) discovery.Record {
		r, _ := ix.Record(p.ID, sources.EntryID(p.ID, page))
		return r
	}
	refresh()
	if r := read(control); r.Entry.Version != "1.3.3" || discovery.NeedsDetail(r) {
		t.Fatalf("after reading the release: %+v", r)
	}
	if r := read(upcoming); r.Entry.ReleaseKind != "preview" || discovery.NeedsDetail(r) {
		t.Fatalf("after reading the announcement: %+v", r)
	}

	// The same catalog six hours later: nothing to read again.
	now = now.Add(6 * time.Hour)
	refresh()
	if discovery.NeedsDetail(record(control)) || discovery.NeedsDetail(record(upcoming)) {
		t.Fatal("an unchanged catalog row asks for its release page again")
	}

	// A new batch lists Control with a newer update, and the announcement
	// is published as the game.
	now = now.Add(6 * time.Hour)
	batch := `<!-- Marker1-Start --><h1>03.10.2026</h1><h3>Control Resonant Deluxe Edition ElAmigos +[Update 1.4.2] <a href="data/Control_Resonant_Deluxe_Edition_MULTi15_-_ElAmigos.html">DOWNLOAD</a></h3>`
	pages[p.Listing] = strings.Replace(strings.Replace(pages[p.Listing], "<!-- Marker1-Start -->", batch, 1), " (preview)", "", 1)
	if res := refresh(); res.Merged.Relisted != 2 {
		t.Errorf("relisted %d, want 2", res.Merged.Relisted)
	}
	r := record(control)
	if !discovery.NeedsDetail(r) || r.Entry.Version != "1.3.3" || r.Entry.SummaryOnly {
		t.Fatalf("a changed row: needs reading %v, claims kept until then %+v", discovery.NeedsDetail(r), r.Entry)
	}
	if !discovery.NeedsDetail(record(upcoming)) {
		t.Fatal("the announcement whose label is gone isn't read again")
	}
	pages[control] = strings.Replace(pages[control], "Updated to version 1.3.3", "Updated to version 1.4.2", 1)
	pages[upcoming] = `<h2>Upcoming Game (2027), 3GB</h2><h3>ElAmigos release. Updated to version 1.0 (03.10.2026).</h3><h2>DDOWNLOAD</h2><a href="https://filecrypt.cc/Container/UPCOMING.html">base</a>`
	if r := read(control); r.Entry.Version != "1.4.2" || r.Changed.IsZero() || discovery.NeedsDetail(r) {
		t.Errorf("after reading it again: %+v", r)
	}
	if r := read(upcoming); r.Entry.ReleaseKind != "release" || r.Changed.IsZero() || len(r.Entry.References) != 1 {
		t.Errorf("the published announcement: %+v", r)
	}
}
