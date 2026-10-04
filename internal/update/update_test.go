package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/ApolloF/Seaglass/internal/edition"
	"github.com/ApolloF/Seaglass/internal/platform"
)

func TestNewer(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{"v1.0.0", "v0.7.0", true},
		{"v0.10.0", "v0.9.9", true},
		{"v1.0", "v0.99.99", true},
		{"1.0.1", "v1.0.0", true},
		{"v1.0.0", "v1.0.0", false},
		{"v0.9.9", "v1.0.0", false},
		{"v1.1.0-beta", "v1.0.0", true},
		{"v1.0.0", "dev", false},
		{"dev", "v0.1.0", false},
		{"v1.2.3.4", "v1.0.0", false},
		{"", "v1.0.0", false},
		// The Store Edition's own releases count after the version.
		{"v1.9.0-store.2", "v1.9.0-store.1", true},
		{"v1.9.0-store.1", "v1.9.0", true},
		{"v1.10.0-store.1", "v1.9.0-store.7", true},
		{"v1.9.0-store.1", "v1.9.0-store.1", false},
		{"v1.9.0-store.x", "v1.9.0", false},
	} {
		if got := Newer(tt.a, tt.b); got != tt.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// fakeGitHub serves a latest-release answer and its assets over TLS. The
// feed's prefix and host allowlist point at it.
type fakeGitHub struct {
	srv     *httptest.Server
	release map[string]any
	files   map[string][]byte
	status  int          // answers /latest with this status when set
	asked   atomic.Int32 // how often /latest was asked
}

func newFake(t *testing.T) (*fakeGitHub, Feed) {
	f := &fakeGitHub{files: map[string][]byte{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		f.asked.Add(1)
		if f.status != 0 {
			w.WriteHeader(f.status)
			return
		}
		if f.release == nil {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(f.release)
	})
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/download/redirect-") {
			http.Redirect(w, r, "https://evil.example/x", http.StatusFound)
			return
		}
		b, ok := f.files[strings.TrimPrefix(r.URL.Path, "/download/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	})
	f.srv = httptest.NewTLSServer(mux)
	t.Cleanup(f.srv.Close)
	u, _ := url.Parse(f.srv.URL)
	return f, Feed{
		LatestURL:   f.srv.URL + "/latest",
		AssetPrefix: f.srv.URL + "/download/",
		Hosts:       []string{u.Hostname()},
		Client:      f.srv.Client(),
	}
}

func (f *fakeGitHub) publish(tag string, files map[string][]byte, extra ...map[string]any) {
	var assets []map[string]any
	for name, b := range files {
		f.files[name] = b
		assets = append(assets, map[string]any{"name": name, "size": len(b), "browser_download_url": f.srv.URL + "/download/" + name})
	}
	for _, e := range extra {
		assets = append(assets, e)
	}
	f.release = map[string]any{"tag_name": tag, "body": "notes", "html_url": "https://github.com/ApolloF/Seaglass/releases/tag/" + tag, "assets": assets}
}

func sumLine(b []byte, name string) []byte {
	h := sha256.Sum256(b)
	return []byte(hex.EncodeToString(h[:]) + "  " + name + "\n")
}

func TestLatestAndDownload(t *testing.T) {
	f, feed := newFake(t)
	ctx := context.Background()
	if _, err := feed.Latest(ctx); err != ErrNoRelease {
		t.Fatalf("no release: err = %v", err)
	}
	body := []byte("MZ pretend installer")
	f.publish("v1.2.0", map[string][]byte{
		InstallerAsset:             body,
		InstallerAsset + ".sha256": sumLine(body, InstallerAsset),
	}, map[string]any{"name": "elsewhere.exe", "size": 1, "browser_download_url": "https://example.com/elsewhere.exe"})
	rel, err := feed.Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "v1.2.0" || rel.Notes != "notes" {
		t.Errorf("release = %+v", rel)
	}
	if _, ok := rel.Asset("elsewhere.exe"); ok {
		t.Error("an asset outside the release prefix was accepted")
	}
	dir := t.TempDir()
	var last int64
	p, sum, err := feed.Download(ctx, rel, InstallerAsset, dir, func(done, total int64) { last = done })
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != string(body) {
		t.Errorf("downloaded %q", b)
	}
	if want, _ := FileSHA256(p); sum != want || last != int64(len(body)) {
		t.Errorf("sum %s want %s, progress %d", sum, want, last)
	}
	if filepath.Base(p) != "v1.2.0-"+InstallerAsset {
		t.Errorf("saved as %s", p)
	}
}

func TestDownloadRejectsBadChecksum(t *testing.T) {
	f, feed := newFake(t)
	body := []byte("MZ tampered")
	f.publish("v1.2.0", map[string][]byte{
		ExeAsset:             body,
		ExeAsset + ".sha256": sumLine([]byte("MZ original"), ExeAsset),
	})
	rel, err := feed.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, _, err := feed.Download(context.Background(), rel, ExeAsset, dir, nil); err == nil {
		t.Fatal("a file that doesn't match its checksum was accepted")
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("left behind %d file(s)", len(left))
	}
	// A checksum file for another file is refused too.
	f.publish("v1.2.0", map[string][]byte{ExeAsset: body, ExeAsset + ".sha256": sumLine(body, "other.exe")})
	rel, _ = feed.Latest(context.Background())
	if _, _, err := feed.Download(context.Background(), rel, ExeAsset, dir, nil); err == nil {
		t.Fatal("a checksum naming another file was accepted")
	}
}

func TestDownloadRefusesForeignRedirect(t *testing.T) {
	f, feed := newFake(t)
	f.publish("v1.2.0", nil,
		map[string]any{"name": ExeAsset, "size": 3, "browser_download_url": f.srv.URL + "/download/redirect-x"},
		map[string]any{"name": ExeAsset + ".sha256", "size": 3, "browser_download_url": f.srv.URL + "/download/redirect-y"},
		map[string]any{"name": "sneaky.exe", "size": 3, "browser_download_url": f.srv.URL + "/download/../elsewhere/x"})
	rel, err := feed.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rel.Asset("sneaky.exe"); ok {
		t.Error("an asset URL climbing out of the release prefix was accepted")
	}
	if _, _, err := feed.Download(context.Background(), rel, ExeAsset, t.TempDir(), nil); err == nil ||
		!strings.Contains(err.Error(), "isn't allowed") {
		t.Fatalf("redirect to another host: err = %v", err)
	}
}

func TestPrereleaseIgnored(t *testing.T) {
	f, feed := newFake(t)
	f.publish("v2.0.0-beta", nil)
	f.release["prerelease"] = true
	if _, err := feed.Latest(context.Background()); err != ErrNoRelease {
		t.Fatalf("prerelease: err = %v", err)
	}
}

func TestPendingRoundTrip(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "v1.2.0-"+ExeAsset)
	if err := os.WriteFile(file, []byte("MZ new"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, _ := FileSHA256(file)
	if err := SavePending(dir, Pending{Tag: "v1.2.0", File: file, SHA256: sum, Kind: KindExe}); err != nil {
		t.Fatal(err)
	}
	p, ok := LoadPending(dir)
	if !ok || p.Tag != "v1.2.0" {
		t.Fatalf("LoadPending = %+v, %v", p, ok)
	}
	// This test binary isn't signed, so only the hash counts.
	self, _ := os.Executable()
	if err := p.Check(self); err != nil {
		t.Errorf("Check: %v", err)
	}
	_ = os.WriteFile(file, []byte("MZ changed"), 0o644)
	if err := p.Check(self); err == nil {
		t.Error("a changed download passed")
	}
	// A pending file pointing outside its folder is ignored.
	_ = SavePending(dir, Pending{Tag: "v1.2.0", File: `C:\Windows\notepad.exe`, SHA256: sum, Kind: KindExe})
	if _, ok := LoadPending(dir); ok {
		t.Error("pending.json pointing outside the updates folder was accepted")
	}
	Clear(dir, "")
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("Clear left %d file(s)", len(left))
	}
}

func TestSwapExe(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "Seaglass.exe")
	next := filepath.Join(dir, "next.exe")
	_ = os.WriteFile(exe, []byte("old"), 0o755)
	_ = os.WriteFile(next, []byte("new"), 0o755)
	if KindFor(exe) != KindExe {
		t.Errorf("KindFor(writable folder) = %q", KindFor(exe))
	}
	if err := SwapExe(next, exe); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new" {
		t.Errorf("exe holds %q", b)
	}
	if b, _ := os.ReadFile(exe + ".old"); string(b) != "old" {
		t.Errorf("old exe holds %q", b)
	}
	CleanOld(exe)
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Error("CleanOld left the old exe")
	}
	_ = os.WriteFile(filepath.Join(dir, "uninstall.exe"), nil, 0o755)
	if KindFor(exe) != KindInstaller {
		t.Errorf("KindFor(installed) = %q", KindFor(exe))
	}
}

func TestCheckPublisher(t *testing.T) {
	gh := filepath.Join(platform.ProgramFiles, "GitHub CLI", "gh.exe")
	git := filepath.Join(platform.ProgramFiles, "Git", "cmd", "git.exe")
	if _, err := platform.Signer(gh); err != nil {
		t.Skip("no signed gh.exe to test with")
	}
	self, _ := os.Executable() // unsigned
	if err := CheckPublisher(gh, gh); err != nil {
		t.Errorf("same publisher refused: %v", err)
	}
	if err := CheckPublisher(self, gh); err == nil {
		t.Error("an unsigned update was accepted by a signed build")
	}
	if _, err := platform.Signer(git); err == nil {
		if err := CheckPublisher(git, gh); err == nil {
			t.Error("an update from another publisher was accepted")
		}
	}
	if err := CheckPublisher(gh, self); err != nil {
		t.Errorf("an unsigned build refused a signed update: %v", err)
	}
}

func TestSignedRelease(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	f, feed := newFake(t)
	feed.Keys = []ed25519.PublicKey{pub}
	body := []byte("MZ signed installer")
	h := sha256.Sum256(body)
	sums := FormatSums(map[string]string{InstallerAsset: hex.EncodeToString(h[:])})
	sign := func(tag string, s []byte) []byte { return EncodeSig(ed25519.Sign(priv, SignedMessage(tag, s))) }
	ctx := context.Background()
	try := func(tag string, files map[string][]byte) error {
		f.publish(tag, files)
		rel, err := feed.Latest(ctx)
		if err != nil {
			return err
		}
		_, _, err = feed.Download(ctx, rel, InstallerAsset, t.TempDir(), nil)
		return err
	}
	if err := try("v1.2.0", map[string][]byte{InstallerAsset: body, SumsAsset: sums, SigAsset: sign("v1.2.0", sums)}); err != nil {
		t.Fatalf("signed release refused: %v", err)
	}
	if err := try("v1.2.0", map[string][]byte{InstallerAsset: body, SumsAsset: sums}); err == nil {
		t.Error("a release without a signature was accepted")
	}
	// An old signed release republished under a newer tag.
	if err := try("v9.0.0", map[string][]byte{InstallerAsset: body, SumsAsset: sums, SigAsset: sign("v1.2.0", sums)}); err == nil {
		t.Error("a signature for another tag was accepted")
	}
	// The file swapped, with its hash in a list that's no longer signed.
	other := []byte("MZ evil")
	h2 := sha256.Sum256(other)
	sums2 := FormatSums(map[string]string{InstallerAsset: hex.EncodeToString(h2[:])})
	if err := try("v1.2.0", map[string][]byte{InstallerAsset: other, SumsAsset: sums2, SigAsset: sign("v1.2.0", sums)}); err == nil {
		t.Error("a changed SHA256SUMS was accepted")
	}
	// Signed by someone else.
	_, priv2, _ := ed25519.GenerateKey(nil)
	sig2 := EncodeSig(ed25519.Sign(priv2, SignedMessage("v1.2.0", sums)))
	if err := try("v1.2.0", map[string][]byte{InstallerAsset: body, SumsAsset: sums, SigAsset: sig2}); err == nil {
		t.Error("a signature by another key was accepted")
	}
	// Right list, wrong file.
	if err := try("v1.2.0", map[string][]byte{InstallerAsset: other, SumsAsset: sums, SigAsset: sign("v1.2.0", sums)}); err == nil {
		t.Error("a file not matching the signed list was accepted")
	}
}

// Releases without checksum files (like Syncer's) are checked against the
// SHA-256 GitHub computed on upload.
func TestDownloadUsesGitHubDigest(t *testing.T) {
	f, feed := newFake(t)
	ctx := context.Background()
	body := []byte("MZ pretend Syncer installer")
	h := sha256.Sum256(body)
	f.files["Syncer-setup.exe"] = body
	f.publish("v0.12.0", nil, map[string]any{
		"name": "Syncer-setup.exe", "size": len(body), "digest": "sha256:" + hex.EncodeToString(h[:]),
		"browser_download_url": f.srv.URL + "/download/Syncer-setup.exe",
	})
	rel, err := feed.Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := feed.Download(ctx, rel, "Syncer-setup.exe", t.TempDir(), nil); err != nil {
		t.Fatalf("download with a digest: %v", err)
	}
	f.publish("v0.12.1", nil, map[string]any{
		"name": "Syncer-setup.exe", "size": len(body), "digest": "sha256:" + strings.Repeat("0", 64),
		"browser_download_url": f.srv.URL + "/download/Syncer-setup.exe",
	})
	if rel, err = feed.Latest(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := feed.Download(ctx, rel, "Syncer-setup.exe", t.TempDir(), nil); err == nil {
		t.Error("a file that doesn't match its digest was accepted")
	}
}

func TestLongNotesStayValidUTF8(t *testing.T) {
	f, feed := newFake(t)
	f.publish("v2.0.0", nil)
	f.release["body"] = strings.Repeat("a", maxNotes-1) + "—more"
	rel, err := feed.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(rel.Notes) || !strings.HasSuffix(rel.Notes, "a…") {
		t.Errorf("notes end in %q", rel.Notes[len(rel.Notes)-8:])
	}
}

// signedFiles is a release of body as the installer, signed for tag.
func signedFiles(priv ed25519.PrivateKey, tag string, body []byte) map[string][]byte {
	h := sha256.Sum256(body)
	sums := FormatSums(map[string]string{InstallerAsset: hex.EncodeToString(h[:])})
	return map[string][]byte{InstallerAsset: body, SumsAsset: sums, SigAsset: EncodeSig(ed25519.Sign(priv, SignedMessage(tag, sums)))}
}

func TestFeedsPreferTheFirstFeed(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	nf, primary := newFake(t)
	of, fallback := newFake(t)
	primary.Keys, fallback.Keys = []ed25519.PublicKey{pub}, []ed25519.PublicKey{pub}
	nf.publish("v1.10.0-store.2", signedFiles(priv, "v1.10.0-store.2", []byte("MZ from the releases repo")))
	of.publish("v1.10.0-store.3", signedFiles(priv, "v1.10.0-store.3", []byte("MZ from the old repo")))
	feeds := Feeds{primary, fallback}
	ctx := context.Background()
	rel, err := feeds.Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Even a newer tag in the old repo doesn't win: it is asked only when
	// the releases repo has nothing.
	if rel.Tag != "v1.10.0-store.2" || of.asked.Load() != 0 {
		t.Fatalf("got %s, old repo asked %d time(s)", rel.Tag, of.asked.Load())
	}
	p, _, err := feeds.Download(ctx, rel, InstallerAsset, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "MZ from the releases repo" {
		t.Errorf("downloaded %q", b)
	}
}

func TestFeedsFallBack(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	for _, tt := range []struct {
		name   string
		status int // what the releases repo answers; 0 is "no release yet" (404)
	}{
		{"releases repo empty or missing", 0},
		{"releases repo failing", http.StatusBadGateway},
		{"rate limited", http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			nf, primary := newFake(t)
			of, fallback := newFake(t)
			primary.Keys, fallback.Keys = []ed25519.PublicKey{pub}, []ed25519.PublicKey{pub}
			nf.status = tt.status
			of.publish("v1.10.0-store.1", signedFiles(priv, "v1.10.0-store.1", []byte("MZ old repo")))
			feeds := Feeds{primary, fallback}
			ctx := context.Background()
			rel, err := feeds.Latest(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if rel.Tag != "v1.10.0-store.1" {
				t.Fatalf("tag %s", rel.Tag)
			}
			// The download goes to the feed the release came from, with its
			// signature check.
			p, _, err := feeds.Download(ctx, rel, InstallerAsset, t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if b, _ := os.ReadFile(p); string(b) != "MZ old repo" {
				t.Errorf("downloaded %q", b)
			}
		})
	}
}

func TestFeedsFallbackStillNeedsTheSignature(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	_, other, _ := ed25519.GenerateKey(nil)
	_, primary := newFake(t)
	of, fallback := newFake(t)
	primary.Keys, fallback.Keys = []ed25519.PublicKey{pub}, []ed25519.PublicKey{pub}
	of.publish("v9.0.0-store.1", signedFiles(other, "v9.0.0-store.1", []byte("MZ evil")))
	feeds := Feeds{primary, fallback}
	rel, err := feeds.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := feeds.Download(context.Background(), rel, InstallerAsset, t.TempDir(), nil); err == nil {
		t.Fatal("a fallback release signed with another key was accepted")
	}
}

func TestFeedsNothingAnywhere(t *testing.T) {
	nf, primary := newFake(t)
	of, fallback := newFake(t)
	ctx := context.Background()
	if _, err := (Feeds{primary, fallback}).Latest(ctx); err != ErrNoRelease {
		t.Fatalf("both empty: err = %v", err)
	}
	// The releases repo failing while the old one is private (404) is an
	// error, not "up to date".
	nf.status = http.StatusInternalServerError
	if _, err := (Feeds{primary, fallback}).Latest(ctx); err == nil || err == ErrNoRelease {
		t.Fatalf("releases repo down, old repo gone: err = %v", err)
	}
	nf.status, of.status = 0, http.StatusInternalServerError
	if _, err := (Feeds{primary, fallback}).Latest(ctx); err == nil || err == ErrNoRelease {
		t.Fatalf("releases repo empty, old repo down: err = %v", err)
	}
}

func TestFeedsRefuseAReleaseFromElsewhere(t *testing.T) {
	f, feed := newFake(t)
	_, other := newFake(t)
	body := []byte("MZ x")
	f.publish("v1.2.0", map[string][]byte{ExeAsset: body, ExeAsset + ".sha256": sumLine(body, ExeAsset)})
	rel, err := feed.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := (Feeds{other}).Download(context.Background(), rel, ExeAsset, t.TempDir(), nil); err == nil {
		t.Fatal("a release from a feed outside the list was downloaded")
	}
	if _, _, err := (Feeds{}).Download(context.Background(), Release{Tag: "v1.2.0"}, ExeAsset, t.TempDir(), nil); err == nil {
		t.Fatal("a release from no feed was downloaded")
	}
}

// The updater reads the public releases-only repository first and the
// source repository second, and both demand the release signature.
func TestReleasesFeedOrder(t *testing.T) {
	if len(Releases) != 2 {
		t.Fatalf("%d feeds", len(Releases))
	}
	want := []string{edition.ReleasesRepo, edition.Repo}
	for i, f := range Releases {
		if f.LatestURL != "https://api.github.com/repos/"+want[i]+"/releases/latest" ||
			f.AssetPrefix != "https://github.com/"+want[i]+"/releases/download/" {
			t.Errorf("feed %d reads %s / %s, want %s", i, f.LatestURL, f.AssetPrefix, want[i])
		}
		if len(f.Keys) == 0 {
			t.Errorf("feed %d doesn't check release signatures", i)
		}
	}
	if edition.ReleasesRepo == edition.Repo {
		t.Error("the releases repository is the source repository")
	}
	if !strings.Contains(ReleasesPage, edition.ReleasesRepo) {
		t.Errorf("ReleasesPage = %s", ReleasesPage)
	}
}
