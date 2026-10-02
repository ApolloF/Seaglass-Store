// Package feed reads the experimental store's catalogs: JSON feeds at URLs
// the person adds (docs/store-feed.md). Seaglass ships none of its own.
// Everything in a feed is checked before use; items that don't pass are
// left out and counted, so one bad item doesn't cost a whole feed.
package feed

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Schema is the feed format version Seaglass reads.
const Schema = 1

// Limits on what a feed can hold.
const (
	MaxBytes = 32 << 20
	MaxItems = 50000
)

// Feed is one catalog.
type Feed struct {
	Schema   int    `json:"schema"`
	Name     string `json:"name"`
	Homepage string `json:"homepage,omitempty"`
	Items    []Item `json:"items"`
}

// Item is one downloadable version of a game.
type Item struct {
	Title              string   `json:"title"`
	Version            string   `json:"version,omitempty"`
	BuildDate          string   `json:"buildDate,omitempty"` // YYYY-MM-DD
	SizeBytes          int64    `json:"sizeBytes,omitempty"` // the download
	InstalledSizeBytes int64    `json:"installedSizeBytes,omitempty"`
	Magnet             string   `json:"magnet,omitempty"`     // a magnet link, or…
	TorrentURL         string   `json:"torrentUrl,omitempty"` // …the URL of a .torrent file
	Languages          []string `json:"languages,omitempty"`
	Platform           string   `json:"platform,omitempty"`      // windows
	InstallerType      string   `json:"installerType,omitempty"` // inno, nsis, msi, archive, portable
	SHA256             string   `json:"sha256,omitempty"`        // of the installer, checked after downloading
	SteamAppID         int      `json:"steamAppId,omitempty"`    // for art and descriptions
	Notes              string   `json:"notes,omitempty"`
}

// Source is the item's magnet link or .torrent URL.
func (it Item) Source() string {
	if it.Magnet != "" {
		return it.Magnet
	}
	return it.TorrentURL
}

// Parse reads and checks a feed. Items that don't pass are left out;
// skipped says why, one line each (the first few).
func Parse(b []byte) (f Feed, skipped []string, err error) {
	if len(b) > MaxBytes {
		return Feed{}, nil, fmt.Errorf("the feed is larger than %d MB", MaxBytes>>20)
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return Feed{}, nil, fmt.Errorf("the feed isn't valid JSON: %w", err)
	}
	if f.Schema != Schema {
		return Feed{}, nil, fmt.Errorf("the feed is schema %d; Seaglass reads schema %d", f.Schema, Schema)
	}
	if f.Name = clean(f.Name, 80); f.Name == "" {
		return Feed{}, nil, errors.New("the feed has no name")
	}
	if f.Homepage != "" && checkURL(f.Homepage) != nil {
		f.Homepage = ""
	}
	if len(f.Items) > MaxItems {
		return Feed{}, nil, fmt.Errorf("the feed has more than %d items", MaxItems)
	}
	items := f.Items[:0]
	for i, it := range f.Items {
		it, err := checkItem(it)
		if err != nil {
			if len(skipped) < 20 {
				skipped = append(skipped, fmt.Sprintf("item %d (%s): %v", i+1, clean(it.Title, 60), err))
			} else if len(skipped) == 20 {
				skipped = append(skipped, "…")
			}
			continue
		}
		items = append(items, it)
	}
	f.Items = items
	return f, skipped, nil
}

var (
	reHex40    = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	reBase32   = regexp.MustCompile(`^[A-Za-z2-7]{32}$`)
	reSHA256   = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
	installers = map[string]bool{"": true, "inno": true, "nsis": true, "msi": true, "archive": true, "portable": true}
)

func checkItem(it Item) (Item, error) {
	if it.Title = clean(it.Title, 200); it.Title == "" {
		return it, errors.New("no title")
	}
	it.Version, it.Notes = clean(it.Version, 80), clean(it.Notes, 2000)
	switch {
	case it.Magnet != "" && it.TorrentURL != "":
		return it, errors.New("both a magnet link and a torrent URL")
	case it.Magnet != "":
		if err := checkMagnet(it.Magnet); err != nil {
			return it, err
		}
	case it.TorrentURL != "":
		if err := checkURL(it.TorrentURL); err != nil {
			return it, fmt.Errorf("torrent URL: %w", err)
		}
	default:
		return it, errors.New("no magnet link or torrent URL")
	}
	if it.BuildDate != "" {
		if _, err := time.Parse(time.DateOnly, it.BuildDate); err != nil {
			return it, errors.New("buildDate isn't YYYY-MM-DD")
		}
	}
	if it.SizeBytes < 0 || it.SizeBytes > 1<<44 || it.InstalledSizeBytes < 0 || it.InstalledSizeBytes > 1<<44 {
		return it, errors.New("impossible size")
	}
	if it.SteamAppID < 0 {
		return it, errors.New("negative Steam AppID")
	}
	switch it.Platform = strings.ToLower(it.Platform); it.Platform {
	case "", "windows":
		it.Platform = "windows"
	default:
		return it, fmt.Errorf("platform %q isn't Windows", it.Platform)
	}
	if it.InstallerType = strings.ToLower(it.InstallerType); !installers[it.InstallerType] {
		return it, fmt.Errorf("unknown installer type %q", it.InstallerType)
	}
	if it.SHA256 != "" && !reSHA256.MatchString(it.SHA256) {
		return it, errors.New("sha256 isn't 64 hex digits")
	}
	it.SHA256 = strings.ToLower(it.SHA256)
	if len(it.Languages) > 60 {
		return it, errors.New("more than 60 languages")
	}
	langs := it.Languages[:0:0]
	for _, l := range it.Languages {
		if l = clean(l, 40); l != "" {
			langs = append(langs, l)
		}
	}
	it.Languages = langs
	return it, nil
}

// checkMagnet accepts a magnet link with a BitTorrent v1 info hash (hex
// or base32) or a v2 multihash.
func checkMagnet(m string) error {
	u, err := url.Parse(m)
	if err != nil || u.Scheme != "magnet" || len(m) > 8192 {
		return errors.New("not a magnet link")
	}
	for _, xt := range u.Query()["xt"] {
		x := strings.ToLower(xt)
		if h, ok := strings.CutPrefix(x, "urn:btih:"); ok && (reHex40.MatchString(h) || reBase32.MatchString(h)) {
			return nil
		}
		if h, ok := strings.CutPrefix(x, "urn:btmh:"); ok {
			if _, err := hex.DecodeString(h); err == nil && len(h) == 68 {
				return nil
			}
		}
	}
	return errors.New("the magnet link has no valid info hash")
}

// CheckURL accepts the address of a feed or a .torrent file: HTTPS, or
// plain HTTP on this PC only (a feed served locally for testing).
func CheckURL(raw string) error { return checkURL(raw) }

func checkURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || len(raw) > 2048 || u.User != nil {
		return errors.New("not a valid web address")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if h := u.Hostname(); h == "localhost" || h == "127.0.0.1" || h == "::1" {
			return nil
		}
		return errors.New("only https:// addresses (or http:// on this PC)")
	}
	return errors.New("only https:// addresses (or http:// on this PC)")
}

// clean trims s, drops control characters and cuts it to n characters.
func clean(s string, n int) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		s = strings.TrimSpace(string(r[:n]))
	}
	return s
}
