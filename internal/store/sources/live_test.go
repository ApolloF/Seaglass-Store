package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type liveObservation struct {
	Source            string `json:"source"`
	URL               string `json:"url"`
	Kind              string `json:"kind"`
	Bytes             int    `json:"bytes"`
	Entries           int    `json:"entries"`
	Transports        int    `json:"transports"`
	References        int    `json:"references"`
	TorrentReferences int    `json:"torrentReferences"`
	KnownSizes        int    `json:"knownSizes"`
	KnownLanguages    int    `json:"knownLanguages"`
	ElapsedMS         int64  `json:"elapsedMs"`
	Error             string `json:"error,omitempty"`
}

// TestLiveSources audits source metadata only. Enable explicitly; raw pages stay in a caller-selected scratch folder.
func TestLiveSources(t *testing.T) {
	if os.Getenv("WL_CATALOG_LIVE") != "1" {
		t.Skip("set WL_CATALOG_LIVE=1 for live source metadata requests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	var report []liveObservation
	for _, id := range []string{"fitgirl", "dodi"} {
		client, err := NewClient(id, true)
		if err != nil {
			t.Fatal(err)
		}
		source := client.source
		root := "https://" + source.Host
		urls := []string{root + "/", root + "/page/5/", root + "/page/20/", root + "/?s=witcher", root + "/?s=cyberpunk", root + "/?s=seaglass_no_such_game_73498723"}
		if id == "fitgirl" {
			urls = append([]string{source.StartURL}, urls...)
		}
		var candidates []Entry
		for _, raw := range urls {
			observation, entries := auditDocument(ctx, t, client, raw, "listing")
			report = append(report, observation)
			candidates = append(candidates, entries...)
		}
		// Recent, older and search discoveries are interleaved so ten detail checks cover several layouts/ages.
		candidates = unique(candidates)
		step := len(candidates) / 10
		if step < 1 {
			step = 1
		}
		count := 0
		for i := 0; i < len(candidates) && count < 10; i += step {
			count++
			observation, entries := auditDocument(ctx, t, client, candidates[i].PageURL, "detail")
			report = append(report, observation)
			if observation.Error == "" && (len(entries) != 1 || entries[0].SummaryOnly) {
				t.Errorf("detail did not yield one complete release: %s", candidates[i].PageURL)
			}
		}
		client.Close()
	}
	if folder := os.Getenv("WL_CATALOG_AUDIT_DIR"); folder != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, "live-audit.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func auditDocument(ctx context.Context, t *testing.T, client *Client, raw, kind string) (liveObservation, []Entry) {
	t.Helper()
	start := time.Now()
	observation := liveObservation{Source: client.source.ID, URL: raw, Kind: kind}
	doc, err := client.FetchDocument(ctx, raw)
	var entries []Entry
	if err == nil {
		entries, err = Parse(client.source, doc.URL, doc.Body)
	}
	observation.ElapsedMS = time.Since(start).Milliseconds()
	if err != nil {
		observation.Error = err.Error()
		t.Errorf("live %s: %v", raw, err)
		return observation, nil
	}
	observation.Bytes, observation.Entries = len(doc.Body), len(entries)
	for _, entry := range entries {
		observation.Transports += len(entry.Transports)
		observation.References += len(entry.References)
		if entry.SizeBytes > 0 {
			observation.KnownSizes++
		}
		if entry.LanguageClaim != "" {
			observation.KnownLanguages++
		}
		for _, ref := range entry.References {
			if ref.Kind == "torrent" {
				observation.TorrentReferences++
			}
		}
		if !entry.NeedsReview || len(entry.DocumentSHA256) != 64 || entry.TitleKey == "" {
			t.Errorf("invalid provenance/review state at %s", raw)
		}
		for _, transport := range entry.Transports {
			if transport.Kind == "magnet" {
				if _, err := Magnet(transport.URI); err != nil {
					t.Errorf("invalid live magnet at %s: %v", raw, err)
				}
			}
			if strings.HasPrefix(transport.URI, "http:") {
				t.Errorf("insecure transport at %s", raw)
			}
		}
	}
	if folder := os.Getenv("WL_CATALOG_AUDIT_DIR"); folder != "" {
		name := fmt.Sprintf("%s-%s-%s.html", client.source.ID, kind, documentEvidence(doc, 0).SHA256[:12])
		if err := os.WriteFile(filepath.Join(folder, name), doc.Body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("%s %s: %d entries, %d transports, %d torrent references", client.source.ID, raw, observation.Entries, observation.Transports, observation.TorrentReferences)
	return observation, entries
}

func TestLiveDODIHostChallenge(t *testing.T) {
	if os.Getenv("WL_CATALOG_HOSTS") != "1" {
		t.Skip("set WL_CATALOG_HOSTS=1 for an external torrent-host form check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	resolver, err := NewResolver(true)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	_, err = resolver.Resolve(ctx, Reference{URL: "https://www.up-4ever.net/wbfv0oi66dgl", Kind: "torrent"})
	var result *ResolutionError
	if !errors.As(err, &result) || result.State != "captcha-required" {
		t.Fatalf("expected observed Turnstile flow, got %v", err)
	}
	t.Log(result)
}

func TestCapturedSourceCorpus(t *testing.T) {
	folder := os.Getenv("WL_CATALOG_REPLAY_DIR")
	if folder == "" {
		t.Skip("set WL_CATALOG_REPLAY_DIR to captured source documents")
	}
	files, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	count, details, sizes, languages := 0, 0, 0, 0
	for _, file := range files {
		name := file.Name()
		id := ""
		if strings.HasPrefix(name, "fitgirl-") {
			id = "fitgirl"
		}
		if strings.HasPrefix(name, "dodi-") {
			id = "dodi"
		}
		if id == "" || !strings.HasSuffix(name, ".html") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(folder, name))
		if err != nil {
			t.Fatal(err)
		}
		source := sourceFor(t, id)
		entries, err := Parse(source, "https://"+source.Host+"/", data)
		if err != nil {
			t.Errorf("captured %s: %v", name, err)
			continue
		}
		count++
		if strings.Contains(name, "-detail-") && len(entries) == 1 {
			details++
			if entries[0].SizeBytes > 0 || len(entries[0].SizeOptionsBytes) > 0 {
				sizes++
			}
			if entries[0].LanguageClaim != "" {
				languages++
			}
		}
	}
	if count == 0 {
		t.Fatal("no captured source documents")
	}
	t.Logf("%d captured documents parsed; %d/%d detail size claims and %d/%d language claims retained", count, sizes, details, languages, details)
}
