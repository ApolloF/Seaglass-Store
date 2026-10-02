package sources

import (
	"encoding/base32"
	"errors"
	"net"
	"strings"
	"testing"
)

const testHash = "0123456789abcdef0123456789abcdef01234567"
const testMagnet = "magnet:?xt=urn:btih:" + testHash

func sourceFor(t *testing.T, id string) Source {
	t.Helper()
	source, err := PrivateSource(id, true)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestSourcesRequireExplicitEnable(t *testing.T) {
	if _, err := PrivateSource("fitgirl", false); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	if _, err := NewClient("dodi", false); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	if _, err := PrivateSource("mirror", true); err == nil {
		t.Fatal("unknown source accepted")
	}
}

func TestSourceRejectsOtherHostsAndUnsafeURLs(t *testing.T) {
	s := sourceFor(t, "fitgirl")
	for _, raw := range []string{
		"http://fitgirl-repacks.site/", "https://fitgirl-repacks.site.evil.example/",
		"https://fitgirl-repacks.site@evil.example/", "https://user:pass@fitgirl-repacks.site/",
		"https://fitgirl-repacks.site:444/", "https://127.0.0.1/", "file:///setup.exe",
		"https://fitgirl-repacks.site/#download", "https://www.fitgirl-repacks.site/",
	} {
		if _, err := s.ValidateURL(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	if _, err := s.ValidateURL(s.StartURL); err != nil {
		t.Fatal(err)
	}
}

func TestPublicAddressesExcludeLocalAndTransitionRanges(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.1.2.3", "192.168.1.1", "169.254.169.254", "100.64.0.1", "0.1.2.3", "224.0.0.1", "::1", "fc00::1", "fe80::1", "::ffff:127.0.0.1", "64:ff9b::7f00:1", "2002:7f00:1::", "2001:db8::1"} {
		if publicIP(net.ParseIP(raw)) {
			t.Errorf("accepted %s", raw)
		}
	}
	for _, raw := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		if !publicIP(net.ParseIP(raw)) {
			t.Errorf("rejected %s", raw)
		}
	}
}

func TestMagnetNormalizesIdentityAndRemovesNetworkParameters(t *testing.T) {
	uri := testMagnet + "&dn=Example+Game&tr=http://127.0.0.1/announce&xs=https://evil.example/file&ws=https://evil.example/seed"
	got, err := Magnet(uri)
	if err != nil {
		t.Fatal(err)
	}
	if got.InfoHash != testHash || strings.Contains(got.URI, "127.0.0.1") || strings.Contains(got.URI, "evil") || !strings.Contains(got.URI, "Example+Game") {
		t.Fatalf("%+v", got)
	}
	base32Hash := base32.StdEncoding.EncodeToString([]byte{1, 35, 69, 103, 137, 171, 205, 239, 1, 35, 69, 103, 137, 171, 205, 239, 1, 35, 69, 103})
	got, err = Magnet("magnet:?xt=urn:btih:" + base32Hash)
	if err != nil || got.InfoHash != testHash {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestMagnetRejectsAmbiguousAndMalformedHashes(t *testing.T) {
	for _, raw := range []string{testMagnet + "&xt=urn:btih:" + testHash, "magnet:?xt=urn:btih:no", "magnet:?xt=urn:btmh:123", "magnet://host?xt=urn:btih:" + testHash, testMagnet + "&dn=%ZZ", testMagnet + "#fragment"} {
		if _, err := Magnet(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestFitGirlRSSSkipsNewsAndDeduplicatesTorrentIdentity(t *testing.T) {
	s := sourceFor(t, "fitgirl")
	rss := `<rss xmlns:content="http://purl.org/rss/1.0/modules/content/"><channel>
	<item><title>Upcoming Repacks</title><link>https://fitgirl-repacks.site/news/</link><description>Not a release</description></item>
	<item><title>Example Game – v1.2.3 + 2 DLCs</title><link>https://fitgirl-repacks.site/example/</link><pubDate>Fri, 02 Oct 2026 01:18:01 +0000</pubDate><content:encoded><![CDATA[
	<p>Languages: <strong>ENG/MULTI12</strong><br>Original Size: <strong>40 GB</strong><br>Repack Size: <strong>From 12.5 GB</strong></p>
	<a href="` + testMagnet + `">magnet</a><a href="magnet:?xt=urn:btih:` + strings.ToUpper(testHash) + `">mirror</a>
	<a href="https://1337x.to/torrent/1/example/">1337x torrent</a>
	]]></content:encoded></item></channel></rss>`
	entries, err := Parse(s, s.StartURL, []byte(rss))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("%+v", entries)
	}
	e := entries[0]
	if e.Title != "Example Game" || e.Version != "v1.2.3" || e.SizeBytes != 12500000000 || !e.SizeIsMinimum || e.InstalledSizeBytes != 40000000000 || e.LanguageClaim != "ENG/MULTI12" {
		t.Fatalf("%+v", e)
	}
	if len(e.Transports) != 1 || len(e.References) != 1 || e.PublishedAt == nil || !e.NeedsReview || len(e.DocumentSHA256) != 64 {
		t.Fatalf("%+v", e)
	}
}

func TestDODIHTMLKeepsUnresolvedLinksWithoutFollowingThem(t *testing.T) {
	s := sourceFor(t, "dodi")
	markup := `<article><h3 class="entry-title"><a href="/example/">1234- Example Game (v2.0 + All DLCs + MULTi4) [DODI Repack]</a></h3><div class="entry-content"><p><strong>Repack Size</strong>: From 3 GiB<br>Game Size: 10 GB</p><a href="https://short.example/abc">Torrent – Click Here</a><a href="javascript:alert(1)">Download</a><a href="https://127.0.0.1/file.torrent">Download</a><script>Repack Size: 1 GB</script></div></article><div id="comments"><a href="` + testMagnet + `">magnet</a></div>`
	entries, err := Parse(s, s.StartURL, []byte(markup))
	if err != nil {
		t.Fatal(err)
	}
	e := entries[0]
	if e.Title != "Example Game" || e.Version != "v2.0" || e.PageURL != "https://dodi-repacks.site/example/" || e.SizeBytes != 3*(1<<30) {
		t.Fatalf("%+v", e)
	}
	if len(e.Transports) != 0 || len(e.References) != 1 {
		t.Fatalf("%+v", e)
	}
}

func Test1337xListingAndDetailAreSeparateEvidence(t *testing.T) {
	s := sourceFor(t, "1337x")
	listing := `<table><tr><td class="name"><a href="/cat/Games/1/">Games</a><a href="/torrent/123/example/">Example Game v1.2</a></td></tr></table>`
	entries, err := Parse(s, s.StartURL, []byte(listing))
	if err != nil || len(entries) != 1 || len(entries[0].Transports) != 0 {
		t.Fatalf("%+v %v", entries, err)
	}
	detail := `<div class="torrent-detail-page"><h1>Example Game v1.2</h1><a href="` + testMagnet + `">Magnet Download</a></div>`
	parsed, err := Parse(s, entries[0].PageURL, []byte(detail))
	if err != nil || len(parsed) != 1 || len(parsed[0].Transports) != 1 || parsed[0].ID != entries[0].ID {
		t.Fatalf("%+v %v", parsed, err)
	}
}

func TestParserRejectsChallengesMalformedRSSAndLimits(t *testing.T) {
	s := sourceFor(t, "fitgirl")
	for _, data := range [][]byte{nil, []byte(`<html><title>Just a moment...</title></html>`), []byte(`<rss><channel><item><title>broken`), []byte(strings.Repeat("x", MaxDocumentBytes+1)), []byte(strings.Repeat("<br>", maxNodes+2))} {
		if _, err := Parse(s, s.StartURL, data); err == nil {
			t.Fatal("invalid document accepted")
		}
	}
	rss := `<rss><channel>` + strings.Repeat(`<item><title>News</title><link>https://fitgirl-repacks.site/news/</link></item>`, MaxEntries+1) + `</channel></rss>`
	if _, err := Parse(s, s.StartURL, []byte(rss)); err == nil {
		t.Fatal("item limit ignored")
	}
}

func TestSizeClaimsAreConservative(t *testing.T) {
	for _, tc := range []struct {
		input   string
		size    int64
		minimum bool
	}{
		{"12.5 GB", 12500000000, false}, {"From 3 GiB", 3 * (1 << 30), true}, {"3-8 GB", 0, false}, {"unknown", 0, false}, {"0 GB", 0, false}, {"9999999999 TB", 0, false},
	} {
		got, minimum := parseSize(tc.input)
		if got != tc.size || minimum != tc.minimum {
			t.Errorf("%s: %d %v", tc.input, got, minimum)
		}
	}
}

func TestReleaseTitlesAreRequiredAndBounded(t *testing.T) {
	s := sourceFor(t, "dodi")
	for _, title := range []string{"", strings.Repeat("x", 1025)} {
		data := `<article><h1 class="entry-title">` + title + `</h1><div class="entry-content">Repack Size: 1 GB</div></article>`
		if _, err := Parse(s, s.StartURL, []byte(data)); err == nil {
			t.Fatal("invalid title accepted")
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(`<article><h1 class="entry-title">Example</h1><div class="entry-content">Repack Size: 1 GB</div></article>`))
	f.Add([]byte(`<rss><channel/></rss>`))
	f.Add([]byte(`<script>magnet:?xt=urn:btih:broken</script>`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 65536 {
			t.Skip()
		}
		for _, id := range []string{"fitgirl", "dodi", "1337x"} {
			s := sourceFor(t, id)
			entries, err := Parse(s, s.StartURL, data)
			if err != nil {
				continue
			}
			if len(entries) > MaxEntries {
				t.Fatal("entry bound exceeded")
			}
			for _, entry := range entries {
				if !entry.NeedsReview {
					t.Fatal("untrusted evidence marked verified")
				}
			}
		}
	})
}
