package feed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const hash = "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c"

const good = `{
  "schema": 1,
  "name": "  Test\tfeed ",
  "homepage": "http://example.com",
  "items": [
    {"title": "Alpha", "version": "v1.2", "buildDate": "2026-05-01", "sizeBytes": 1000,
     "magnet": "magnet:?xt=urn:btih:` + hash + `&dn=Alpha", "languages": ["English", " ", "Français"],
     "installerType": "INNO", "sha256": "` + "ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef0123456789" + `", "steamAppId": 620},
    {"title": "Beta", "torrentUrl": "https://example.org/beta.torrent"},
    {"title": "Gamma", "magnet": "magnet:?xt=urn:btih:MFRGGZDFMZTWQ2LKNNWG23TPOBYXE43U&dn=g"},
    {"title": "", "magnet": "magnet:?xt=urn:btih:` + hash + `"},
    {"title": "No source"},
    {"title": "Both", "magnet": "magnet:?xt=urn:btih:` + hash + `", "torrentUrl": "https://example.org/x.torrent"},
    {"title": "Bad hash", "magnet": "magnet:?xt=urn:btih:1234"},
    {"title": "Plain HTTP", "torrentUrl": "http://example.org/x.torrent"},
    {"title": "Mac", "magnet": "magnet:?xt=urn:btih:` + hash + `", "platform": "macos"},
    {"title": "Bad date", "magnet": "magnet:?xt=urn:btih:` + hash + `", "buildDate": "May 2026"},
    {"title": "Bad installer", "magnet": "magnet:?xt=urn:btih:` + hash + `", "installerType": "setup.exe"},
    {"title": "Huge", "magnet": "magnet:?xt=urn:btih:` + hash + `", "sizeBytes": 99999999999999999}
  ]
}`

func TestParse(t *testing.T) {
	f, skipped, err := Parse([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "Test feed" || f.Homepage != "" {
		t.Errorf("name %q, homepage %q (plain HTTP elsewhere is dropped)", f.Name, f.Homepage)
	}
	if len(f.Items) != 3 || len(skipped) != 9 {
		t.Fatalf("%d items, skipped %d: %v", len(f.Items), len(skipped), skipped)
	}
	a := f.Items[0]
	if a.InstallerType != "inno" || a.Platform != "windows" || len(a.Languages) != 2 || a.SHA256 != strings.ToLower(a.SHA256) || a.Source() != a.Magnet {
		t.Errorf("first item: %+v", a)
	}
	if f.Items[1].Source() != "https://example.org/beta.torrent" {
		t.Errorf("source of a torrent URL item: %q", f.Items[1].Source())
	}
	for _, want := range []string{"no title", "no magnet", "both", "info hash", "https", "Windows", "YYYY-MM-DD", "installer type", "size"} {
		found := false
		for _, s := range skipped {
			found = found || strings.Contains(s, want)
		}
		if !found {
			t.Errorf("no skipped item says %q: %v", want, skipped)
		}
	}
}

func TestParseRejectsWholeFeeds(t *testing.T) {
	for name, b := range map[string]string{
		"not JSON":     `nope`,
		"other schema": `{"schema":2,"name":"x","items":[]}`,
		"no name":      `{"schema":1,"name":" ","items":[]}`,
	} {
		if _, _, err := Parse([]byte(b)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestCheckURL(t *testing.T) {
	for u, ok := range map[string]bool{
		"https://example.org/feed.json":   true,
		"http://localhost:8080/feed.json": true,
		"http://127.0.0.1/feed.json":      true,
		"http://example.org/feed.json":    false,
		"https://user:pw@example.org/f":   false,
		"file:///C:/feed.json":            false,
		"example.org/feed.json":           false,
	} {
		if err := CheckURL(u); (err == nil) != ok {
			t.Errorf("%s: %v", u, err)
		}
	}
}

func TestCache(t *testing.T) {
	var hits atomic.Int32
	body := good
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if fail {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Write([]byte(body))
	}))
	defer srv.Close()
	c := Cache{Dir: t.TempDir()}
	ctx := context.Background()
	url := srv.URL + "/feed.json"

	st := c.Refresh(ctx, url)
	if st.Error != "" || st.Name != "Test feed" || st.Items != 3 || st.Skipped != 9 || st.Fetched == 0 {
		t.Fatalf("first fetch: %+v", st)
	}
	if st := c.Refresh(ctx, url); st.Error != "" || st.Items != 3 {
		t.Errorf("unchanged feed: %+v", st)
	}
	fail = true
	st = c.Refresh(ctx, url)
	if st.Error == "" || st.Items != 3 {
		t.Errorf("a feed that's down should keep its last copy: %+v", st)
	}
	if f, ok := c.Load(url); !ok || len(f.Items) != 3 {
		t.Errorf("last good copy: %v, %d items", ok, len(f.Items))
	}
	c.Forget(url)
	if _, ok := c.Load(url); ok || c.Status(url).Name != "" {
		t.Error("forgotten feed still cached")
	}
	if st := c.Refresh(ctx, "http://example.org/feed.json"); st.Error == "" || hits.Load() != 3 {
		t.Errorf("plain HTTP elsewhere was fetched: %+v", st)
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(good))
	f.Add([]byte(`{"schema":1,"name":"x","items":[{"title":"\u0000a","magnet":"magnet:?xt=urn:btih:` + hash + `"}]}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		feed, _, err := Parse(b)
		if err != nil {
			return
		}
		if feed.Name == "" {
			t.Error("a parsed feed without a name")
		}
		for _, it := range feed.Items {
			if it.Title == "" || it.Source() == "" || it.Platform != "windows" || strings.ContainsAny(it.Title, "\x00\n\r\t") {
				t.Errorf("item passed the checks: %+v", it)
			}
		}
	})
}
