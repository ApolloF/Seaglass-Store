package discovery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/store/sources"
)

// reopen reads the index from disk as a restart would, without anything
// the running index kept only in memory.
func reopen(dir string) *Index {
	return OpenIndex(filepath.Join(dir, "discovery"), filepath.Join(dir, "store-identity.json"))
}

func TestBatchesFollowEachOtherWithoutWaiting(t *testing.T) {
	ix, _ := testIndex(t)
	f := newFakeSource(t, "fitgirl", 95)
	clk := &clock{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	batches := 0
	for res := Pass(context.Background(), ix, f.src, f, true, clk.now, PassHooks{}); ; res = Pass(context.Background(), ix, f.src, f, false, clk.now, PassHooks{}) {
		if res.Err != nil {
			t.Fatal(res.Err)
		}
		batches++
		if !res.More {
			break
		}
		if batches > 10 {
			t.Fatal("backfill never ended")
		}
	}
	if c := ix.Crawl("fitgirl"); !c.BackfillDone || ix.Counts()["fitgirl"] != 95 {
		t.Fatalf("after back-to-back batches: %+v, %d records", c, ix.Counts()["fitgirl"])
	}
	if batches != 2 {
		t.Errorf("batches: %d, want 2 (pages 1-5, then 6-10 and the end)", batches)
	}
	for page := 2; page <= 10; page++ {
		if n := f.count(fmt.Sprintf("https://fitgirl-repacks.site/page/%d/", page)); n != 1 {
			t.Errorf("page %d fetched %d times", page, n)
		}
	}
}

func TestProgressIsSavedBeforeEachNextRequest(t *testing.T) {
	ix, dir := testIndex(t)
	f := newFakeSource(t, "dodi", 95)
	clk := &clock{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	checked := 0
	// Before every older page, what is on disk already holds every page
	// before it: a crash here loses nothing and skips nothing.
	f.before = func(raw string) {
		var page int
		if _, err := fmt.Sscanf(raw, "https://dodi-repacks.site/page/%d/", &page); err != nil {
			return
		}
		disk := reopen(dir)
		if c := disk.Crawl("dodi"); c.NextPage != page {
			t.Errorf("before page %d the saved position is %d", page, c.NextPage)
		}
		if n := disk.Counts()["dodi"]; n != (page-1)*10 {
			t.Errorf("before page %d, %d records are saved, want %d", page, n, (page-1)*10)
		}
		checked++
	}
	for res := Pass(context.Background(), ix, f.src, f, true, clk.now, PassHooks{}); res.More; res = Pass(context.Background(), ix, f.src, f, false, clk.now, PassHooks{}) {
	}
	if checked != 9 {
		t.Errorf("checked %d pages, want 9", checked)
	}
}

func TestRestartResumesAtTheSavedPageWithoutSkippingOrRepeating(t *testing.T) {
	ix, dir := testIndex(t)
	f := newFakeSource(t, "fitgirl", 95)
	clk := &clock{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	// Seaglass stops in the middle of a batch, without saving the index:
	// the request for page 4 never finishes.
	ctx, cancel := context.WithCancel(context.Background())
	f.before = func(raw string) {
		if raw == "https://fitgirl-repacks.site/page/4/" {
			cancel()
		}
	}
	if res := Pass(ctx, ix, f.src, f, true, clk.now, PassHooks{}); res.Stopped != StopCanceled || res.Err != nil {
		t.Fatalf("the interrupted batch: %+v", res)
	}
	f.before = nil
	ix = reopen(dir)
	if c := ix.Crawl("fitgirl"); c.NextPage != 4 || c.RecentAt.IsZero() || c.Failures != 0 || ix.Counts()["fitgirl"] != 30 {
		t.Fatalf("after the restart: %+v, %d records", c, ix.Counts()["fitgirl"])
	}
	for res := Pass(context.Background(), ix, f.src, f, false, clk.now, PassHooks{}); res.More; res = Pass(context.Background(), ix, f.src, f, false, clk.now, PassHooks{}) {
	}
	if c := ix.Crawl("fitgirl"); !c.BackfillDone || ix.Counts()["fitgirl"] != 95 {
		t.Fatalf("after resuming: %+v, %d records", c, ix.Counts()["fitgirl"])
	}
	for page := 2; page <= 10; page++ {
		if n := f.count(fmt.Sprintf("https://fitgirl-repacks.site/page/%d/", page)); n != 1 {
			t.Errorf("page %d fetched %d times", page, n)
		}
	}
	if n := f.count(f.src.Listing); n != 1 {
		t.Errorf("the newest listings were fetched %d times, want once", n)
	}
}

func TestACutShortJournalLineMeansThatPageIsFetchedAgain(t *testing.T) {
	ix, dir := testIndex(t)
	f := newFakeSource(t, "dodi", 95)
	clk := &clock{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	Pass(context.Background(), ix, f.src, f, true, clk.now, PassHooks{})
	path := filepath.Join(dir, "discovery", "dodi.journal")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// A crash in the middle of writing the last line (page 5).
	lines := strings.SplitAfter(strings.TrimSuffix(string(b), "\n"), "\n")
	last := lines[len(lines)-1]
	if err := os.WriteFile(path, []byte(strings.Join(lines[:len(lines)-1], "")+last[:len(last)/2]), 0o600); err != nil {
		t.Fatal(err)
	}
	if c := reopen(dir).Crawl("dodi"); c.NextPage != 5 {
		t.Errorf("after a cut-short line the next page is %d, want 5", c.NextPage)
	}
}

func TestSaveFoldsTheJournalIntoTheSourceFile(t *testing.T) {
	ix, dir := testIndex(t)
	f := newFakeSource(t, "dodi", 95)
	clk := &clock{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	Pass(context.Background(), ix, f.src, f, true, clk.now, PassHooks{})
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "discovery", "dodi.journal")); !os.IsNotExist(err) {
		t.Errorf("the journal is still there after saving: %v", err)
	}
	// A record changed after the save isn't undone by an older journal.
	id := sources.EntryID("dodi", "https://dodi-repacks.site/game-000/")
	ix.Update("dodi", id, func(r *Record) { r.FetchError = "kept" })
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}
	disk := reopen(dir)
	if r, _ := disk.Record("dodi", id); r.FetchError != "kept" || disk.Crawl("dodi").NextPage != 6 || disk.Counts()["dodi"] != 50 {
		t.Errorf("after reopening: %+v, %+v, %d records", r, disk.Crawl("dodi"), disk.Counts()["dodi"])
	}
}

func TestRetryAfterIsSavedAndDefersTheNextRequest(t *testing.T) {
	ix, dir := testIndex(t)
	f := newFakeSource(t, "fitgirl", 95)
	clk := &clock{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	f.fail["https://fitgirl-repacks.site/page/3/"] = &sources.HTTPError{Status: 429, RetryAfter: "120"}
	res := Pass(context.Background(), ix, f.src, f, true, clk.now, PassHooks{})
	if res.Err == nil || res.More {
		t.Fatalf("a 429: %+v", res)
	}
	ix = reopen(dir) // the wait outlives a restart
	c := ix.Crawl("fitgirl")
	if !c.RetryAt.Equal(clk.now().Add(2*time.Minute)) || c.NextPage != 3 || ix.Counts()["fitgirl"] != 20 {
		t.Fatalf("after a 429: %+v, %d records", c, ix.Counts()["fitgirl"])
	}
	asked := len(f.order)
	clk.add(time.Minute)
	if res := Pass(context.Background(), ix, f.src, f, true, clk.now, PassHooks{}); res.Pages != 0 || len(f.order) != asked {
		t.Errorf("asked again before Retry-After ended: %+v", res)
	}
	clk.add(time.Minute)
	if res := Pass(context.Background(), ix, f.src, f, false, clk.now, PassHooks{}); res.Err != nil || f.count("https://fitgirl-repacks.site/page/3/") != 2 {
		t.Errorf("page 3 wasn't asked again after the wait: %+v", res)
	}
}

func TestAFinishedFiniteCatalogIsNotAskedAgainByLaterBatches(t *testing.T) {
	ix, _ := testIndex(t)
	f := newFakeSource(t, "dodi", 8)
	f.src.ListingPage, f.src.Search = "", ""
	clk := &clock{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	if res := Pass(context.Background(), ix, f.src, f, true, clk.now, PassHooks{}); res.More || res.Pages != 1 {
		t.Fatalf("one-page catalog: %+v", res)
	}
	if recent, backfill := Due(ix.Crawl("dodi"), clk.now(), false); recent || backfill {
		t.Errorf("a finished one-page catalog is still due: recent %v backfill %v", recent, backfill)
	}
}

// Measured on 3 October 2026 (Go 1.27, Windows 11, Ryzen 7 7800X3D, NVMe)
// with 6,000 records, about a fully indexed FitGirl: rewriting the source
// file writes 7.5 MB and takes 26 ms per page (40 MB allocated); a journal
// line writes 13 KB and takes 0.3 ms.
//
//	go test -run XXX -bench PerPage -benchmem ./internal/store/discovery
func BenchmarkSaveWholeSourcePerPage(b *testing.B) {
	ix, _ := benchIndex(b, 6000)
	page := benchPage(10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ix.Merge("fitgirl", page, OriginListing, true, time.Now())
		if err := ix.Save(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJournalPerPage(b *testing.B) {
	ix, _ := benchIndex(b, 6000)
	page := benchPage(10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ix.journal("fitgirl", page, func(c *CrawlState) { c.NextPage++ }); err != nil {
			b.Fatal(err)
		}
	}
}

func benchIndex(b *testing.B, n int) (*Index, string) {
	dir := b.TempDir()
	ix := OpenIndex(filepath.Join(dir, "discovery"), filepath.Join(dir, "store-identity.json"))
	ix.Merge("fitgirl", benchPage(n), OriginListing, true, time.Now())
	if err := ix.Save(); err != nil {
		b.Fatal(err)
	}
	return ix, dir
}

// benchPage makes n releases about the size of real ones.
func benchPage(n int) []sources.Entry {
	pub := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	out := make([]sources.Entry, n)
	for i := range out {
		slug := fmt.Sprintf("bench-game-%05d", i)
		out[i] = sources.Entry{SourceID: "fitgirl", PageURL: "https://fitgirl-repacks.site/" + slug + "/", Title: "Bench Game " + slug, TitleKey: "benchgame" + slug,
			Version: "v1.2.3.45678 + 12 DLCs + Bonus Content", SizeClaim: "from 23.4 GB [Selective Download]", LanguageClaim: "ENG/RUS/MULTi12", ReleaseKind: "release", PublishedAt: &pub,
			Transports: []sources.Transport{{Kind: "magnet", URI: "magnet:?xt=urn:btih:" + testMagnetHash + "&dn=" + slug + "&tr=udp%3A%2F%2Ftracker.example%3A1337%2Fannounce", InfoHash: testMagnetHash}},
			References: []sources.Reference{
				{URL: "https://1337x.to/torrent/1234567/" + slug + "/", Kind: "torrent"},
				{URL: "https://paste.example/abcdef" + slug, Kind: "mirror"},
				{URL: "https://file-host.example/folder/" + slug + "-part1.rar", Kind: "direct"},
			},
			Warnings: []string{"Based on the GOG release. Some DLCs are optional and can be skipped while installing."}}
	}
	return out
}
