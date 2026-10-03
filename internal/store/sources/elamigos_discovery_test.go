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
	result := discovery.Pass(context.Background(), ix, p, f, true, func() time.Time { return now }, nil)
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
	result = discovery.Pass(context.Background(), ix, p, f, false, func() time.Time { return now.Add(time.Hour) }, nil)
	if result.Err != nil || result.Pages != 0 || len(f.hits) != 2 {
		t.Fatalf("completed catalog requested another page: %+v %v", result, f.hits)
	}
	// A failed refresh retains every cached record and completed catalog progress.
	f.failure = errors.New("provider offline")
	result = discovery.Pass(context.Background(), ix, p, f, true, func() time.Time { return now.Add(7 * time.Hour) }, nil)
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
