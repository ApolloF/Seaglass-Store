package sources

import (
	"context"
	"os"
	"testing"
	"time"
)

// Metadata only: no file-host requests, torrent peers or installers.
func TestLiveElAmigos(t *testing.T) {
	if os.Getenv("WL_STORE_LIVE") != "1" && os.Getenv("WL_ELAMIGOS_LIVE") != "1" {
		t.Skip("set WL_ELAMIGOS_LIVE=1 for four bounded public metadata requests")
	}
	s := elAmigosFixtureSource()
	c := newClient(s.ValidateURL)
	c.source = s
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pages := []string{
		s.StartURL,
		"https://elamigos.site/data/Control_Resonant_Deluxe_Edition_MULTi15_-_ElAmigos.html",
		"https://elamigos.site/data/Red_Dead_Redemption_2_MULTi13__ElAmigos_-_KnPzu8CD.html",
		"https://elamigos.site/data/Resonance_A_Plague_Tale_Legacy_MULTi17_-_ElAmigos.html",
	}
	for i, page := range pages {
		doc, err := c.FetchDocument(ctx, page)
		if err != nil {
			t.Fatal(err)
		}
		entries, err := parseElAmigosFixture(t, doc.URL, doc.Body)
		if err != nil || len(entries) == 0 {
			t.Fatalf("%s: count=%d err=%v", page, len(entries), err)
		}
		if i > 0 && (len(entries) != 1 || entries[0].SummaryOnly || entries[0].ReleaseKind != "release" || len(entries[0].References) == 0) {
			t.Fatalf("detail failed verification: %+v", entries)
		}
		sizes, languages, references := 0, 0, 0
		for _, entry := range entries {
			if !entry.NeedsReview || entry.SourceID != "elamigos" || len(entry.Transports) != 0 {
				t.Fatalf("unexpected trust/transport: %+v", entry)
			}
			if entry.SizeBytes > 0 {
				sizes++
			}
			if entry.LanguageClaim != "" {
				languages++
			}
			references += len(entry.References)
		}
		t.Logf("url=%s bytes=%d sha256=%s entries=%d sizes=%d languages=%d browserReferences=%d", doc.URL, len(doc.Body), documentEvidence(doc, 0).SHA256, len(entries), sizes, languages, references)
	}
}
