package sources

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func smallTorrent(name string) []byte {
	info := "d6:lengthi4e4:name" + pageNumber(len(name)) + ":" + name + "12:piece lengthi4e6:pieces20:" + strings.Repeat("x", 20) + "e"
	return []byte("d4:info" + info + "e")
}

func TestTorrentMetadataHashesOriginalInfoAndChecksLayout(t *testing.T) {
	transport, err := TorrentMetadata(smallTorrent("Example"))
	if err != nil || len(transport.InfoHash) != 40 || len(transport.MetadataSHA256) != 64 || transport.SizeBytes != 4 || transport.TorrentName != "Example" {
		t.Fatalf("%+v %v", transport, err)
	}
	if _, err := Magnet(transport.URI); err != nil {
		t.Fatal(err)
	}
}

func TestTorrentMetadataRejectsMalformedOrUnsafeInputs(t *testing.T) {
	for _, data := range [][]byte{
		nil, []byte("<html>captcha</html>"), append(smallTorrent("Example"), 'x'),
		[]byte(strings.Replace(string(smallTorrent("Example")), "i4e", "i04e", 1)),
		[]byte(strings.Replace(string(smallTorrent("Example")), "i4e", "i-4e", 1)),
		[]byte(strings.Replace(string(smallTorrent("Example")), "12:piece lengthi4e", "12:piece lengthi0e", 1)),
		[]byte(strings.Repeat("x", (2<<20)+1)),
		[]byte("d4:infod6:lengthi4e4:name7:Example12:piece lengthi4e6:pieces0:ee"),
		[]byte("d4:infod4:name7:Example6:lengthi4eee"),
		[]byte("d4:infod6:lengthi4e6:lengthi4eee"),
	} {
		if _, err := TorrentMetadata(data); err == nil {
			t.Fatalf("accepted %q", data[:min(len(data), 100)])
		}
	}
	for _, name := range []string{"..", "../game", `C:\game`, "CON", "nul.exe", "game.", "game ", "LPT1.txt", "x\x00y"} {
		if _, err := TorrentMetadata(smallTorrent(name)); err == nil {
			t.Errorf("accepted name %q", name)
		}
	}
}

func TestTorrentMetadataValidatesMultifilePaths(t *testing.T) {
	for _, path := range []string{"game.bin", "..", "CON.txt", `x\y`} {
		info := "d5:filesld6:lengthi4e4:pathl" + pageNumber(len(path)) + ":" + path + "eee4:name7:Example12:piece lengthi4e6:pieces20:" + strings.Repeat("x", 20) + "e"
		_, err := TorrentMetadata([]byte("d4:info" + info + "e"))
		if (path == "game.bin") != (err == nil) {
			t.Errorf("path=%s err=%v", path, err)
		}
	}
	info := "d5:filesld4:attr1:l6:lengthi4e4:pathl8:game.bineee4:name7:Example12:piece lengthi4e6:pieces20:" + strings.Repeat("x", 20) + "e"
	if _, err := TorrentMetadata([]byte("d4:info" + info + "e")); err == nil {
		t.Fatal("torrent symlink accepted")
	}
}

func TestResolverRequiresEnableAndReviewedTorrentSection(t *testing.T) {
	if _, err := NewResolver(false); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	for _, ref := range []Reference{
		{URL: "https://file-me.top/abcdefghijkl.html", Kind: "download"},
		{URL: "https://file-me.top.evil.example/abcdefghijkl.html", Kind: "torrent"},
		{URL: "https://file-me.top/login", Kind: "torrent"},
		{URL: "https://127.0.0.1/abcdefghijkl.html", Kind: "torrent"},
		{URL: "https://svr1.file-me.top/files/setup.exe", Kind: "torrent"},
	} {
		if _, err := ResolverURL(ref); err == nil {
			t.Errorf("accepted %+v", ref)
		}
	}
	got, err := ResolverURL(Reference{URL: "http://file-me.top/abcdefghijkl.html", Kind: "torrent"})
	if err != nil || got != "https://file-me.top/abcdefghijkl.html" {
		t.Fatalf("%s %v", got, err)
	}
}

func TestResolverFollowsOrdinaryFormAndValidatesTorrent(t *testing.T) {
	r, _ := NewResolver(true)
	defer r.Close()
	calls := 0
	r.client.http.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		r.client.last = time.Time{}
		switch calls {
		case 1:
			if req.Method != "GET" {
				t.Fatal(req.Method)
			}
			resp := testResponse(200, `<form method="POST" action=""><input name="op" value="download2"><input name="id" value="abcdefghijkl"><input name="rand" value="test-token"></form>`)
			resp.Header.Set("Set-Cookie", "file_session=session-test; Path=/; Secure; HttpOnly")
			return resp, nil
		case 2:
			cookie, err := req.Cookie("file_session")
			if err != nil || cookie.Value != "session-test" {
				t.Fatal("file-host session cookie lost")
			}
			data, _ := io.ReadAll(req.Body)
			form, _ := url.ParseQuery(string(data))
			if req.Method != "POST" || form.Get("rand") != "test-token" || form.Get("method_free") != "Free Download" {
				t.Fatalf("%s %v", req.Method, form)
			}
			resp := testResponse(302, "")
			resp.Header.Set("Location", "https://svr1.file-me.top/files/Example.torrent")
			return resp, nil
		case 3:
			if _, err := req.Cookie("file_session"); err == nil {
				t.Fatal("host-only session cookie leaked to CDN")
			}
			resp := testResponse(200, string(smallTorrent("Example")))
			resp.Header.Set("Content-Type", "application/x-bittorrent")
			resp.Request = req
			return resp, nil
		default:
			t.Fatal("unexpected request")
			return nil, nil
		}
	})
	transport, err := r.Resolve(context.Background(), Reference{URL: "http://file-me.top/abcdefghijkl.html", Kind: "torrent"})
	if err != nil || calls != 3 || transport.SizeBytes != 4 {
		t.Fatalf("%+v %v calls=%d", transport, err, calls)
	}
}

func TestResolverStopsAtActualCaptchaButIgnoresMarketingText(t *testing.T) {
	for _, markup := range []string{`<div class="cf-turnstile"></div>`, `<div class="g-recaptcha"></div>`, `<div class="h-captcha"></div>`} {
		r, _ := NewResolver(true)
		calls := 0
		r.client.http.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) { calls++; return testResponse(200, markup), nil })
		_, err := r.Resolve(context.Background(), Reference{URL: "https://www.up-4ever.net/abcdefghijkl", Kind: "torrent"})
		var result *ResolutionError
		if !errors.As(err, &result) || result.State != "captcha-required" || calls != 1 {
			t.Fatalf("%v calls=%d", err, calls)
		}
	}
	root, _ := document([]byte(`<p>Premium has no captchas</p>`))
	if challenge(root) {
		t.Fatal("marketing mistaken for challenge")
	}
}

func TestResolverRejectsUnreviewedRedirectsBeforeContact(t *testing.T) {
	r, _ := NewResolver(true)
	calls := 0
	r.client.http.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		resp := testResponse(302, "")
		resp.Header.Set("Location", "https://evil.example/setup.exe")
		return resp, nil
	})
	if _, err := r.Resolve(context.Background(), Reference{URL: "https://file-me.top/abcdefghijkl.html", Kind: "torrent"}); err == nil || calls != 1 {
		t.Fatalf("%v calls=%d", err, calls)
	}
}

func TestResolverSurfacesRateLimitsWithoutRetrying(t *testing.T) {
	for _, status := range []int{200, 429, 403} {
		r, _ := NewResolver(true)
		calls := 0
		r.client.http.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			resp := testResponse(status, `<div class="alert-danger">You have to wait 2 minutes</div>`)
			resp.Header.Set("Retry-After", "120")
			return resp, nil
		})
		_, err := r.Resolve(context.Background(), Reference{URL: "https://file-me.top/abcdefghijkl.html", Kind: "torrent"})
		var result *ResolutionError
		if !errors.As(err, &result) || calls != 1 {
			t.Fatalf("%v calls=%d", err, calls)
		}
		if status != 403 && result.State != "rate-limited" {
			t.Fatal(result)
		}
	}
}

func TestDownloadFormsRejectTokenRequestsActionsAndExcessiveWaits(t *testing.T) {
	base := `<form method="POST" action=""><input name="op" value="download2"><input name="id" value="abcdefghijkl">%s</form>`
	for _, extra := range []string{`<input name="password" value="">`, `<input name="op" value="download2">`, `<span id="seconds">999</span>`, `<input name="usr_login" value="user">`} {
		root, _ := document([]byte(strings.Replace(base, "%s", extra, 1)))
		if _, _, _, err := downloadForm("https://file-me.top/abcdefghijkl.html", root); err == nil {
			t.Fatal("unsafe form accepted")
		}
	}
	markup := strings.Replace(strings.Replace(base, `action=""`, `action="https://evil.example/abcdefghijkl.html"`, 1), "%s", "", 1)
	root, _ := document([]byte(markup))
	if _, _, _, err := downloadForm("https://file-me.top/abcdefghijkl.html", root); err == nil {
		t.Fatal("unsafe action")
	}
	root, _ = document([]byte(strings.Replace(base, "%s", `<span id="seconds">5</span>`, 1)))
	if _, _, wait, err := downloadForm("https://file-me.top/abcdefghijkl.html", root); err != nil || wait != 6*time.Second {
		t.Fatalf("%v %v", wait, err)
	}
}

func FuzzTorrentMetadata(f *testing.F) {
	f.Add(smallTorrent("Example"))
	f.Add([]byte("d4:infodee"))
	f.Add([]byte("<html/>"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 65536 {
			t.Skip()
		}
		transport, err := TorrentMetadata(data)
		if err == nil {
			if len(transport.InfoHash) != 40 || transport.SizeBytes <= 0 || !safeComponent(transport.TorrentName) {
				t.Fatalf("%+v", transport)
			}
		}
	})
}
