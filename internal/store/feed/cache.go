package feed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Status is what's known about one feed's last fetch.
type Status struct {
	URL     string `json:"url"`
	Name    string `json:"name"` // from the feed; "" before its first good fetch
	Items   int    `json:"items"`
	Skipped int    `json:"skipped"` // items left out because they didn't pass the checks
	Fetched int64  `json:"fetched"` // unix seconds of the last good fetch
	Error   string `json:"error,omitempty"`
	ETag    string `json:"etag,omitempty"`
}

// Cache keeps fetched feeds on disk, so the catalog is there at start
// and a feed that's down keeps its last good copy.
type Cache struct {
	Dir    string
	Client *http.Client
}

// ID is a feed's file name in the cache.
func ID(url string) string {
	h := sha256.Sum256([]byte(url))
	return hex.EncodeToString(h[:8])
}

func (c Cache) body(url string) string   { return filepath.Join(c.Dir, ID(url)+".json") }
func (c Cache) status(url string) string { return filepath.Join(c.Dir, ID(url)+".status.json") }

// Status returns a feed's last fetch.
func (c Cache) Status(url string) Status {
	st := Status{URL: url}
	if b, err := os.ReadFile(c.status(url)); err == nil {
		_ = json.Unmarshal(b, &st)
		st.URL = url
	}
	return st
}

// Load returns a feed's last good copy.
func (c Cache) Load(url string) (Feed, bool) {
	b, err := os.ReadFile(c.body(url))
	if err != nil {
		return Feed{}, false
	}
	f, _, err := Parse(b)
	return f, err == nil
}

// Refresh fetches a feed (unless it's unchanged since the last fetch) and
// keeps it when it parses. A failure is noted in its status; the last good
// copy stays.
func (c Cache) Refresh(ctx context.Context, url string) Status {
	st := c.Status(url)
	f, skipped, notModified, etag, err := c.fetch(ctx, url, st.ETag)
	switch {
	case err != nil:
		st.Error = err.Error()
	case notModified:
		st.Error, st.Fetched = "", time.Now().Unix()
	default:
		st = Status{URL: url, Name: f.Name, Items: len(f.Items), Skipped: len(skipped), Fetched: time.Now().Unix(), ETag: etag}
	}
	if b, err := json.Marshal(st); err == nil {
		_ = writeFile(c.status(url), b)
	}
	return st
}

// Forget removes a feed from the cache.
func (c Cache) Forget(url string) {
	_ = os.Remove(c.body(url))
	_ = os.Remove(c.status(url))
}

func (c Cache) fetch(ctx context.Context, url, etag string) (f Feed, skipped []string, notModified bool, newTag string, err error) {
	if err := checkURL(url); err != nil {
		return f, nil, false, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return f, nil, false, "", err
	}
	req.Header.Set("Accept", "application/json")
	if etag != "" && c.hasBody(url) {
		req.Header.Set("If-None-Match", etag)
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return f, nil, false, "", fmt.Errorf("couldn't reach the feed: %w", err)
	}
	defer resp.Body.Close()
	// A redirect to plain HTTP elsewhere would undo the HTTPS check.
	if err := checkURL(resp.Request.URL.String()); err != nil {
		return f, nil, false, "", fmt.Errorf("the feed redirected to %s: %w", resp.Request.URL.Host, err)
	}
	switch {
	case resp.StatusCode == http.StatusNotModified:
		return f, nil, true, etag, nil
	case resp.StatusCode != http.StatusOK:
		return f, nil, false, "", fmt.Errorf("the feed answered %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, MaxBytes+1))
	if err != nil {
		return f, nil, false, "", fmt.Errorf("reading the feed: %w", err)
	}
	f, skipped, err = Parse(b)
	if err != nil {
		return f, nil, false, "", err
	}
	if err := writeFile(c.body(url), b); err != nil {
		return f, nil, false, "", err
	}
	return f, skipped, false, resp.Header.Get("ETag"), nil
}

func (c Cache) hasBody(url string) bool {
	_, err := os.Stat(c.body(url))
	return err == nil
}

func writeFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}
	return nil
}
