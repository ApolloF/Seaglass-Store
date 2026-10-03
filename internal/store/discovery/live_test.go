package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/store/sources"
)

// TestLiveDiscovery runs one real pass per source and one source search,
// bounded to the pass budget (about a dozen requests per source, two
// seconds apart). Metadata only: nothing is resolved or downloaded.
//
//	WL_STORE_LIVE=1 go test -run LiveDiscovery -v ./internal/store/discovery
func TestLiveDiscovery(t *testing.T) {
	if os.Getenv("WL_STORE_LIVE") != "1" {
		t.Skip("set WL_STORE_LIVE=1 for live source requests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	dir := t.TempDir()
	ix := OpenIndex(filepath.Join(dir, "d"), filepath.Join(dir, "id.json"))
	for _, id := range Sources {
		client, err := sources.NewClient(id, true)
		if err != nil {
			t.Fatal(err)
		}
		src, _ := sources.Lookup(id)
		start := time.Now()
		res := Pass(ctx, ix, src, client, true, time.Now, PassHooks{})
		c := ix.Crawl(id)
		t.Logf("%s: pass %v, %d pages, %d new, err %v; next page %d, records %d", id, time.Since(start).Round(time.Second), res.Pages, res.Merged.Added, res.Err, c.NextPage, ix.Counts()[id])
		if res.Err != nil || res.Merged.Added == 0 {
			t.Errorf("%s: nothing indexed", id)
		}
		n, err := SearchSource(ctx, ix, src, client, "witcher", time.Now())
		t.Logf("%s: search 'witcher': %d results, err %v", id, n, err)
		client.Close()
	}
	v := NewView(Group(ix.Records(Sources), nil, nil, nil))
	installable, unresolved := 0, 0
	for _, g := range v.Games {
		if g.Summary().Installable {
			installable++
		} else {
			unresolved++
		}
	}
	h := v.Home(nil, StateUnavailable, Status{})
	t.Logf("games %d (installable %d, unresolved %d); newest: %v", len(v.Games), installable, unresolved, titles(h.New, 5))
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}
	if back := OpenIndex(filepath.Join(dir, "d"), filepath.Join(dir, "id.json")); back.Counts()["fitgirl"] != ix.Counts()["fitgirl"] {
		t.Error("the saved index doesn't load back")
	}
}

func titles(gs []GameSummary, n int) []string {
	var out []string
	for _, g := range gs[:min(n, len(gs))] {
		out = append(out, g.Title)
	}
	return out
}
