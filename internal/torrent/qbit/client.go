// Package qbit drives qBittorrent (5.0 or newer) through its Web UI API
// as the store's download engine, running it as a hidden sidecar with a
// profile of its own.
package qbit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/torrent"
)

// Tag marks every torrent Seaglass adds, so it only ever touches its own.
const Tag = "seaglass"

// ErrAuth means qBittorrent refused the credentials.
var ErrAuth = errors.New("qBittorrent refused the login")

// Client talks to one qBittorrent Web UI. Safe for concurrent use.
type Client struct {
	base       string // http://127.0.0.1:port
	user, pass string
	http       *http.Client
	mu         sync.Mutex // one login at a time
}

var _ torrent.Engine = (*Client)(nil)

// NewClient returns a client for the Web UI at base (http://127.0.0.1:port).
func NewClient(base, user, pass string) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{base: strings.TrimRight(base, "/"), user: user, pass: pass, http: &http.Client{Jar: jar, Timeout: 30 * time.Second}}
}

// Login starts a session.
func (c *Client) Login(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	body, err := c.send(ctx, http.MethodPost, "auth/login", url.Values{"username": {c.user}, "password": {c.pass}})
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(body)) != "Ok." {
		return ErrAuth
	}
	return nil
}

// Version is qBittorrent's version, like "v5.1.4".
func (c *Client) Version(ctx context.Context) (string, error) {
	b, err := c.call(ctx, http.MethodGet, "app/version", nil)
	return strings.TrimSpace(string(b)), err
}

// Shutdown asks qBittorrent to quit.
func (c *Client) Shutdown(ctx context.Context) error {
	_, err := c.call(ctx, http.MethodPost, "app/shutdown", nil)
	return err
}

// Add adds a magnet link or the URL of a .torrent file.
func (c *Client) Add(ctx context.Context, source string, opts torrent.AddOptions) error {
	if strings.ContainsAny(source, "\r\n") {
		return errors.New("one torrent at a time")
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	tags := Tag
	if opts.Tag != "" {
		if strings.ContainsAny(opts.Tag, ", ") {
			return errors.New("a tag can't hold commas or spaces")
		}
		tags += "," + opts.Tag
	}
	for k, v := range map[string]string{"urls": source, "savepath": opts.SavePath, "tags": tags, "stopped": fmt.Sprint(opts.Paused)} {
		if v != "" {
			_ = w.WriteField(k, v)
		}
	}
	if err := w.Close(); err != nil {
		return err
	}
	b, err := c.retry(ctx, func() ([]byte, error) {
		return c.do(ctx, http.MethodPost, "torrents/add", bytes.NewReader(buf.Bytes()), w.FormDataContentType())
	})
	if err != nil {
		return err
	}
	if s := strings.TrimSpace(string(b)); s != "Ok." && s != "" {
		return fmt.Errorf("qBittorrent didn't take the torrent: %s", s)
	}
	return nil
}

type info struct {
	Hash       string  `json:"hash"`
	Name       string  `json:"name"`
	Tags       string  `json:"tags"`
	State      string  `json:"state"`
	Size       int64   `json:"size"`
	AmountLeft int64   `json:"amount_left"`
	Progress   float64 `json:"progress"`
	DLSpeed    int64   `json:"dlspeed"`
	UPSpeed    int64   `json:"upspeed"`
	NumSeeds   int     `json:"num_seeds"`
	NumLeechs  int     `json:"num_leechs"`
	ETA        int64   `json:"eta"`
	SavePath   string  `json:"save_path"`
	Ratio      float64 `json:"ratio"`
}

// List returns the torrents Seaglass added.
func (c *Client) List(ctx context.Context) ([]torrent.Torrent, error) {
	b, err := c.call(ctx, http.MethodGet, "torrents/info", url.Values{"tag": {Tag}})
	if err != nil {
		return nil, err
	}
	var in []info
	if err := json.Unmarshal(b, &in); err != nil {
		return nil, fmt.Errorf("qBittorrent's torrent list: %w", err)
	}
	out := make([]torrent.Torrent, 0, len(in))
	for _, t := range in {
		eta := t.ETA
		if eta >= 8640000 { // qBittorrent's "infinity"
			eta = 0
		}
		out = append(out, torrent.Torrent{
			Hash: t.Hash, Name: t.Name, Tag: ownTag(t.Tags), State: state(t.State, t.Progress),
			Size: t.Size, Done: max(t.Size-t.AmountLeft, 0), Progress: t.Progress,
			DownSpeed: t.DLSpeed, UpSpeed: t.UPSpeed, Seeds: t.NumSeeds, Peers: t.NumLeechs,
			ETA: eta, SavePath: t.SavePath, Ratio: t.Ratio,
		})
	}
	return out, nil
}

// ownTag is the tag Seaglass gave a torrent besides Tag.
func ownTag(tags string) string {
	for _, t := range strings.Split(tags, ",") {
		if t = strings.TrimSpace(t); t != "" && t != Tag {
			return t
		}
	}
	return ""
}

// state maps qBittorrent's states (4.x and 5.x names) onto Seaglass's.
func state(s string, progress float64) torrent.State {
	switch s {
	case "metaDL", "forcedMetaDL":
		return torrent.Metadata
	case "downloading", "forcedDL", "stalledDL", "allocating", "moving":
		return torrent.Downloading
	case "queuedDL":
		return torrent.Queued
	case "pausedDL", "stoppedDL":
		return torrent.Paused
	case "checkingDL", "checkingUP", "checkingResumeData":
		return torrent.Checking
	case "uploading", "forcedUP", "stalledUP", "queuedUP":
		return torrent.Seeding
	case "pausedUP", "stoppedUP":
		return torrent.Complete
	case "error", "missingFiles":
		return torrent.Failed
	}
	if progress >= 1 {
		return torrent.Complete
	}
	return torrent.Downloading
}

// Files lists a torrent's files.
func (c *Client) Files(ctx context.Context, hash string) ([]torrent.File, error) {
	b, err := c.call(ctx, http.MethodGet, "torrents/files", url.Values{"hash": {hash}})
	if err != nil {
		return nil, err
	}
	var in []struct {
		Index    *int    `json:"index"`
		Name     string  `json:"name"`
		Size     int64   `json:"size"`
		Progress float64 `json:"progress"`
		Priority int     `json:"priority"`
	}
	if err := json.Unmarshal(b, &in); err != nil {
		return nil, fmt.Errorf("qBittorrent's file list: %w", err)
	}
	out := make([]torrent.File, 0, len(in))
	for i, f := range in {
		ix := i
		if f.Index != nil {
			ix = *f.Index
		}
		out = append(out, torrent.File{Index: ix, Name: f.Name, Size: f.Size, Progress: f.Progress, Skip: f.Priority == 0})
	}
	return out, nil
}

// SetSkipped chooses whether files are downloaded.
func (c *Client) SetSkipped(ctx context.Context, hash string, indexes []int, skip bool) error {
	if len(indexes) == 0 {
		return nil
	}
	ids := make([]string, len(indexes))
	for i, x := range indexes {
		ids[i] = fmt.Sprint(x)
	}
	prio := "1"
	if skip {
		prio = "0"
	}
	_, err := c.call(ctx, http.MethodPost, "torrents/filePrio", url.Values{"hash": {hash}, "id": {strings.Join(ids, "|")}, "priority": {prio}})
	return err
}

// Pause stops torrents (qBittorrent 5 calls it stop; 4.x pause).
func (c *Client) Pause(ctx context.Context, hashes ...string) error {
	return c.either(ctx, "torrents/stop", "torrents/pause", hashes)
}

// Resume starts torrents again.
func (c *Client) Resume(ctx context.Context, hashes ...string) error {
	return c.either(ctx, "torrents/start", "torrents/resume", hashes)
}

func (c *Client) either(ctx context.Context, path, old string, hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}
	v := url.Values{"hashes": {strings.Join(hashes, "|")}}
	_, err := c.call(ctx, http.MethodPost, path, v)
	if errors.Is(err, errNotFound) {
		_, err = c.call(ctx, http.MethodPost, old, v)
	}
	return err
}

// Remove deletes torrents, with their downloaded files or without.
func (c *Client) Remove(ctx context.Context, deleteFiles bool, hashes ...string) error {
	if len(hashes) == 0 {
		return nil
	}
	_, err := c.call(ctx, http.MethodPost, "torrents/delete", url.Values{"hashes": {strings.Join(hashes, "|")}, "deleteFiles": {fmt.Sprint(deleteFiles)}})
	return err
}

// Apply sets qBittorrent's network preferences.
func (c *Client) Apply(ctx context.Context, n torrent.Network) error {
	b, err := json.Marshal(prefs(n.Normalize()))
	if err != nil {
		return err
	}
	_, err = c.call(ctx, http.MethodPost, "app/setPreferences", url.Values{"json": {string(b)}})
	return err
}

// Interfaces lists the network interfaces qBittorrent can bind to.
func (c *Client) Interfaces(ctx context.Context) ([]torrent.Interface, error) {
	b, err := c.call(ctx, http.MethodGet, "app/networkInterfaceList", nil)
	if err != nil {
		return nil, err
	}
	var in []struct{ Name, Value string }
	if err := json.Unmarshal(b, &in); err != nil {
		return nil, fmt.Errorf("qBittorrent's interface list: %w", err)
	}
	out := make([]torrent.Interface, 0, len(in))
	for _, i := range in {
		if strings.HasPrefix(i.Value, "loopback") {
			continue // binding to loopback would cut every peer off
		}
		out = append(out, torrent.Interface{ID: i.Value, Name: i.Name})
	}
	return out, nil
}

// Addresses lists an interface's addresses ("" for every interface).
func (c *Client) Addresses(ctx context.Context, iface string) ([]string, error) {
	b, err := c.call(ctx, http.MethodGet, "app/networkInterfaceAddressList", url.Values{"iface": {iface}})
	if err != nil {
		return nil, err
	}
	var out []string
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("qBittorrent's address list: %w", err)
	}
	return out, nil
}

var (
	errNotFound  = errors.New("not found")
	errForbidden = errors.New("forbidden")
)

// call sends a form request in a session.
func (c *Client) call(ctx context.Context, method, path string, v url.Values) ([]byte, error) {
	return c.retry(ctx, func() ([]byte, error) { return c.send(ctx, method, path, v) })
}

// retry runs req, logging in again once when the session expired (or
// there was none yet).
func (c *Client) retry(ctx context.Context, req func() ([]byte, error)) ([]byte, error) {
	b, err := req()
	if errors.Is(err, errForbidden) {
		if err := c.Login(ctx); err != nil {
			return nil, err
		}
		b, err = req()
	}
	return b, err
}

func (c *Client) send(ctx context.Context, method, path string, v url.Values) ([]byte, error) {
	if method == http.MethodGet {
		if len(v) > 0 {
			path += "?" + v.Encode()
		}
		return c.do(ctx, method, path, nil, "")
	}
	return c.do(ctx, method, path, strings.NewReader(v.Encode()), "application/x-www-form-urlencoded")
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader, ctype string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/api/v2/"+path, body)
	if err != nil {
		return nil, err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	// qBittorrent's CSRF protection wants these to match its own address.
	req.Header.Set("Referer", c.base)
	req.Header.Set("Origin", c.base)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized:
		return nil, errForbidden
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("qBittorrent %s: %w", path, errNotFound)
	case resp.StatusCode >= 300:
		return nil, fmt.Errorf("qBittorrent %s: %s %s", path, resp.Status, strings.TrimSpace(string(b)))
	}
	return b, nil
}
