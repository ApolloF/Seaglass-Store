package enrich

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Hosts the providers talk to. Nothing else is ever fetched.
const (
	hostSteamAPI   = "api.steampowered.com"
	hostSteamStore = "store.steampowered.com"
	hostHLTB       = "howlongtobeat.com"
)

var allowedHosts = []string{hostSteamAPI, hostSteamStore, hostHLTB}

func allowed(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, a := range allowedHosts {
		if h == a {
			return true
		}
	}
	return false
}

// ErrNotAllowed means a URL points somewhere the providers don't fetch from.
var ErrNotAllowed = errors.New("host not allowed")

// errBackoff means a host failed recently and is being left alone for a while.
type errBackoff struct{ until time.Time }

func (e *errBackoff) Error() string {
	return "paused after earlier failures; trying again after " + e.until.Local().Format("15:04")
}

// statusError is a response that wasn't 200.
type statusError struct {
	Host       string
	Status     int
	RetryAfter time.Duration
}

func (e *statusError) Error() string {
	return fmt.Sprintf("%s answered HTTP %d", e.Host, e.Status)
}

// errNotFound means the provider has no such game.
var errNotFound = errors.New("not found")

// errFormat means a provider answered 200 with something we can't read,
// which usually means it changed its format.
type errFormat struct{ what string }

func (e *errFormat) Error() string { return "unexpected answer from the provider: " + e.what }

// Unreadable says a provider answered with something Seaglass can't read:
// asking again soon gets the same answer.
func Unreadable(err error) bool { return errors.As(err, new(*errFormat)) }

// Limits on what we read and how long we wait.
const (
	maxBody        = 4 << 20
	maxRedirects   = 3
	requestTimeout = 30 * time.Second

	backoffFirst = time.Minute
	backoffMax   = 30 * time.Minute
	// A server may ask for longer than our own cap; we honour up to this.
	retryAfterMax = time.Hour

	userAgent = "Seaglass (+https://github.com/ApolloF/Seaglass)"
)

// hostState keeps one host to one request at a time, spaces the requests
// out and, after failures, leaves the host alone for a growing while.
type hostState struct {
	name string
	gap  time.Duration
	sem  chan struct{}

	mu       sync.Mutex
	last     time.Time // when the last request ended
	failures int
	until    time.Time // no requests before this
}

func newHostState(name string, gap time.Duration) *hostState {
	return &hostState{name: name, gap: gap, sem: make(chan struct{}, 1)}
}

// blocked reports whether the host is in its pause.
func (h *hostState) blocked(now time.Time) (time.Time, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.until, now.Before(h.until)
}

// fail starts or extends the pause. A server's Retry-After wins over our
// own schedule.
func (h *hostState) fail(now time.Time, retryAfter time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failures++
	d := backoffFirst
	for i := 1; i < h.failures && d < backoffMax; i++ {
		d *= 2
	}
	if d > backoffMax {
		d = backoffMax
	}
	if retryAfter > 0 {
		d = min(retryAfter, retryAfterMax)
	}
	if until := now.Add(d); until.After(h.until) {
		h.until = until
	}
}

func (h *hostState) succeed() {
	h.mu.Lock()
	h.failures = 0
	h.until = time.Time{}
	h.mu.Unlock()
}

// request is one HTTP call to an allowlisted URL.
type request struct {
	method string
	url    string
	header map[string]string
	body   []byte
	// forbiddenOK: a 403 is the caller's to handle (an expired token) and
	// doesn't start a pause.
	forbiddenOK bool
}

func (c *Client) host(raw string) (*hostState, *url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, nil, err
	}
	if !allowed(u) {
		return nil, nil, fmt.Errorf("%w: %s", ErrNotAllowed, u.Hostname())
	}
	h := c.hosts[strings.ToLower(u.Hostname())]
	if h == nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrNotAllowed, u.Hostname())
	}
	return h, u, nil
}

// fetch makes one request, waiting its turn and spacing, and returns the
// body of a 200 answer (at most maxBody bytes).
func (c *Client) fetch(ctx context.Context, r request) ([]byte, error) {
	h, u, err := c.host(r.url)
	if err != nil {
		return nil, err
	}
	if until, ok := h.blocked(c.now()); ok {
		return nil, &errBackoff{until}
	}
	select {
	case h.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-h.sem }()
	// Another request may have failed while this one waited.
	if until, ok := h.blocked(c.now()); ok {
		return nil, &errBackoff{until}
	}
	h.mu.Lock()
	wait := h.last.Add(h.gap).Sub(c.now())
	h.mu.Unlock()
	if wait > 0 {
		if err := c.sleep(ctx, wait); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	body, err := c.send(ctx, u, r)
	now := c.now()
	h.mu.Lock()
	h.last = now
	h.mu.Unlock()

	switch e := err.(type) {
	case nil:
		h.succeed()
	case *statusError:
		switch {
		case e.Status == http.StatusNotFound:
			// The host is fine; it just doesn't know this one.
			h.succeed()
			return nil, errNotFound
		case e.Status == http.StatusForbidden && r.forbiddenOK:
		default:
			h.fail(now, e.RetryAfter)
		}
	default:
		if ctx.Err() == nil {
			h.fail(now, 0)
		}
	}
	return body, err
}

func (c *Client) send(ctx context.Context, u *url.URL, r request) ([]byte, error) {
	method := r.method
	if method == "" {
		method = http.MethodGet
	}
	var rd io.Reader
	if r.body != nil {
		rd = bytes.NewReader(r.body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	for k, v := range r.header {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &statusError{Host: u.Hostname(), Status: resp.StatusCode, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), c.now())}
	}
	if resp.ContentLength > maxBody {
		return nil, fmt.Errorf("%s: response too large", u.Hostname())
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxBody {
		return nil, fmt.Errorf("%s: response too large", u.Hostname())
	}
	return b, nil
}

// parseRetryAfter reads seconds or an HTTP date; 0 when absent or unusable.
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil {
		if n < 0 {
			return 0
		}
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}

// formatFailed records that a host answered but not in a form we can read.
// Hammering a host whose format changed helps nobody, so it pauses it too.
func (c *Client) formatFailed(raw string) {
	if h, _, err := c.host(raw); err == nil {
		h.fail(c.now(), 0)
	}
}

func realSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
