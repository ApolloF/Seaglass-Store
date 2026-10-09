// Package update keeps Seaglass up to date from its GitHub releases.
//
// It asks GitHub for the newest release (prereleases don't count), downloads
// the installer (or, for a copy that wasn't installed, the bare exe) together
// with its published SHA-256, and checks the hash, and the Authenticode
// publisher when the running exe is signed, before the file is ever run.
// Downloads come only from the release assets of the feed that answered.
package update

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/edition"
)

// Asset names every release carries. They stay the same across versions, so
// github.com/…/releases/latest/download/<name> always points at the newest.
const (
	InstallerAsset = "Seaglass-setup.exe"
	ExeAsset       = "Seaglass.exe"
)

// Source is where the updater finds and fetches releases: one Feed, or
// Feeds tried in order.
type Source interface {
	Latest(ctx context.Context) (Release, error)
	Download(ctx context.Context, rel Release, name, dir string, progress func(done, total int64)) (string, string, error)
}

// Feed is where releases come from.
type Feed struct {
	LatestURL   string   // GitHub's "latest release" API endpoint
	AssetPrefix string   // every download URL must start with this
	Hosts       []string // hosts a download may be redirected to
	Client      *http.Client
	// Keys are the release keys; with any, only files listed in a signed
	// SHA256SUMS are accepted.
	Keys []ed25519.PublicKey
	// Product is the name signed into each release ("Seaglass" when empty).
	Product string
}

// Signed reports whether the feed's downloads are checked against signed
// releases, not only against checksums GitHub serves beside them.
func (f Feed) Signed() bool { return len(f.Keys) > 0 }

func (f Feed) product() string {
	if f.Product == "" {
		return defaultProduct
	}
	return f.Product
}

// githubFeed reads a GitHub repository's releases, signed with ReleaseKeys.
func githubFeed(repo string) Feed {
	return Feed{
		LatestURL:   "https://api.github.com/repos/" + repo + "/releases/latest",
		AssetPrefix: "https://github.com/" + repo + "/releases/download/",
		Hosts:       []string{"github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com"},
		Keys:        ReleaseKeys,
	}
}

// GitHub is this edition's release feed: the public releases-only
// repository, which stays reachable when the source repository is private.
var GitHub = githubFeed(edition.ReleasesRepo)

// Legacy is the source repository's own releases, where every release
// before the releases-only repository was published.
var Legacy = githubFeed(edition.Repo)

// Releases is what the updater reads. The releases-only repository answers
// first; the source repository stands in while the new one has nothing
// published yet or can't be reached, so the move can't strand an install.
// Both need the same release signature.
var Releases = Feeds{GitHub, Legacy}

// ReleasesPage is where people download Seaglass by hand.
const ReleasesPage = "https://github.com/" + edition.ReleasesRepo + "/releases/latest"

// Asset is one downloadable file of a release.
type Asset struct {
	Name   string
	URL    string
	Size   int64
	Digest string // SHA-256 GitHub computed on upload ("sha256:<hex>"), when it did
}

// Release is a published Seaglass release.
type Release struct {
	Tag       string // "v1.0.0"
	Notes     string // markdown, shortened
	Page      string // release page on github.com
	Published time.Time
	assets    map[string]Asset
	from      string // LatestURL of the feed that answered
}

// Asset returns the release's file with this name.
func (r Release) Asset(name string) (Asset, bool) {
	a, ok := r.assets[name]
	return a, ok
}

func (f Feed) client() *http.Client {
	c := &http.Client{Timeout: 10 * time.Minute}
	if f.Client != nil {
		*c = *f.Client
	}
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if !f.hostAllowed(req.URL) {
			return fmt.Errorf("download redirected to %s, which isn't allowed", req.URL.Hostname())
		}
		return nil
	}
	return c
}

func (f Feed) hostAllowed(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, a := range f.Hosts {
		if h == a {
			return true
		}
	}
	return false
}

// inRelease reports whether raw is a plain URL below the release prefix
// (no "..", escapes or spaces that could lead it elsewhere).
func (f Feed) inRelease(raw string) bool {
	if !strings.HasPrefix(raw, f.AssetPrefix) || strings.ContainsAny(raw, "\r\n \\%") {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && f.hostAllowed(u) && path.Clean(u.Path) == u.Path
}

const maxNotes = 4000

// Latest asks GitHub for the newest release.
func (f Feed) Latest(ctx context.Context) (Release, error) {
	if u, err := url.Parse(f.LatestURL); err != nil || u.Scheme != "https" || u.User != nil {
		return Release{}, errors.New("refusing to ask for releases over anything but HTTPS")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.LatestURL, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Seaglass (+https://github.com/ApolloF/Seaglass)")
	resp, err := f.client().Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Release{}, ErrNoRelease
	}
	if resp.StatusCode != http.StatusOK {
		return Release{}, errors.New("GitHub answered " + resp.Status)
	}
	var r struct {
		Tag        string    `json:"tag_name"`
		Body       string    `json:"body"`
		HTMLURL    string    `json:"html_url"`
		Draft      bool      `json:"draft"`
		Prerelease bool      `json:"prerelease"`
		Published  time.Time `json:"published_at"`
		Assets     []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Size   int64  `json:"size"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&r); err != nil {
		return Release{}, err
	}
	if r.Draft || r.Prerelease {
		return Release{}, ErrNoRelease
	}
	if _, ok := parse(r.Tag); !ok {
		return Release{}, errors.New("unexpected release tag " + strconv.Quote(r.Tag))
	}
	rel := Release{Tag: r.Tag, Notes: r.Body, Page: r.HTMLURL, Published: r.Published, assets: map[string]Asset{}, from: f.LatestURL}
	if len(rel.Notes) > maxNotes {
		rel.Notes = strings.ToValidUTF8(rel.Notes[:maxNotes], "") + "…" // drop a rune cut in half
	}
	if !strings.HasPrefix(rel.Page, "https://github.com/") {
		rel.Page = ReleasesPage
	}
	for _, a := range r.Assets {
		if f.inRelease(a.URL) {
			rel.assets[a.Name] = Asset{Name: a.Name, URL: a.URL, Size: a.Size, Digest: a.Digest}
		}
	}
	return rel, nil
}

// ErrNoRelease means GitHub has no (non-preview) release yet.
var ErrNoRelease = errors.New("no release published yet")

// Feeds are tried in order, and the first with a release answers. A later
// feed is asked only when the ones before it have no release or can't be
// reached, so whoever controls a later feed can't hold back the first.
type Feeds []Feed

// Latest returns the first feed's newest release. With none anywhere, it
// reports the first real failure rather than ErrNoRelease: a feed that
// can't be reached mustn't read as "up to date".
func (fs Feeds) Latest(ctx context.Context) (Release, error) {
	var failed error
	for _, f := range fs {
		rel, err := f.Latest(ctx)
		if err == nil {
			return rel, nil
		}
		if failed == nil && !errors.Is(err, ErrNoRelease) {
			failed = err
		}
	}
	if failed != nil {
		return Release{}, failed
	}
	return Release{}, ErrNoRelease
}

// Download fetches name from the feed rel came from, with that feed's
// checks.
func (fs Feeds) Download(ctx context.Context, rel Release, name, dir string, progress func(done, total int64)) (string, string, error) {
	for _, f := range fs {
		if rel.from != "" && f.LatestURL == rel.from {
			return f.Download(ctx, rel, name, dir, progress)
		}
	}
	return "", "", fmt.Errorf("%s didn't come from a known release feed", rel.Tag)
}

// Newer reports whether version tag a is newer than b ("v1.2.3", "1.2").
// Anything that isn't a version (like "dev") is never newer nor older.
func Newer(a, b string) bool {
	va, oka := parse(a)
	vb, okb := parse(b)
	if !oka || !okb {
		return false
	}
	for i := range va {
		if va[i] != vb[i] {
			return va[i] > vb[i]
		}
	}
	return false
}

// Valid reports whether v is a version Seaglass can compare.
func Valid(v string) bool {
	_, ok := parse(v)
	return ok
}

// parse reads a version as major, minor, patch and the edition's own
// release number (v1.9.0-store.2), which counts after the three.
func parse(v string) ([4]int, bool) {
	var out [4]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.Index(v, edition.Suffix); i >= 0 {
		n, err := strconv.Atoi(v[i+len(edition.Suffix):])
		if err != nil || n < 0 {
			return out, false
		}
		out[3], v = n, v[:i]
	}
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i] // pre-release or build suffix
	}
	parts := strings.Split(v, ".")
	if v == "" || len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
