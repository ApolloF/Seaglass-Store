package enrich

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// recorded is one request the fake network saw.
type recorded struct {
	Method string
	URL    string
	Host   string
	Path   string
	Query  string
	Header http.Header
	Body   string
	At     time.Time
}

// answer is what the fake network replies with.
type answer struct {
	status int
	body   []byte
	header http.Header
	err    error
	// chunked hides the length, as a server streaming its answer would.
	chunked bool
}

func ok(b []byte) answer { return answer{status: 200, body: b} }

func status(code int, h ...string) answer {
	a := answer{status: code, header: http.Header{}}
	for i := 0; i+1 < len(h); i += 2 {
		a.header.Set(h[i], h[i+1])
	}
	return a
}

// fakeNet replaces the network: it records every request and answers
// through its handler.
type fakeNet struct {
	mu      sync.Mutex
	reqs    []recorded
	handler func(recorded) answer
	clock   *clock
}

func (f *fakeNet) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	r := recorded{Method: req.Method, URL: req.URL.String(), Host: req.URL.Host, Path: req.URL.Path, Query: req.URL.RawQuery, Header: req.Header.Clone(), Body: string(body)}
	if f.clock != nil {
		r.At = f.clock.Now()
	}
	f.mu.Lock()
	f.reqs = append(f.reqs, r)
	f.mu.Unlock()
	a := f.handler(r)
	if a.err != nil {
		return nil, a.err
	}
	h := a.header
	if h == nil {
		h = http.Header{}
	}
	n := int64(len(a.body))
	if a.chunked {
		n = -1
	}
	return &http.Response{StatusCode: a.status, Status: http.StatusText(a.status), Header: h, Body: io.NopCloser(bytes.NewReader(a.body)), ContentLength: n, Request: req}, nil
}

func (f *fakeNet) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reqs)
}

// countPath counts requests whose path contains s.
func (f *fakeNet) countPath(s string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.reqs {
		if strings.Contains(r.Path, s) {
			n++
		}
	}
	return n
}

func (f *fakeNet) last() recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reqs[len(f.reqs)-1]
}

func (f *fakeNet) find(path string) []recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []recorded
	for _, r := range f.reqs {
		if strings.Contains(r.Path, path) {
			out = append(out, r)
		}
	}
	return out
}

// clock is a hand-wound clock.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock { return &clock{t: time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func (c *clock) sleep(_ context.Context, d time.Duration) error {
	c.Advance(d)
	return nil
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// newTestClient builds a Client on the fake network and a hand-wound
// clock. The spacing between requests is off, as with any Transport.
func newTestClient(t *testing.T, handler func(recorded) answer) (*Client, *fakeNet, *clock) {
	t.Helper()
	clk := newClock()
	fn := &fakeNet{handler: handler, clock: clk}
	c := New(Options{Dir: t.TempDir(), Transport: fn, Now: clk.Now})
	c.sleep = clk.sleep
	return c, fn, clk
}

// steamRouter answers the Steam fixtures by path.
func steamRouter(t *testing.T) func(recorded) answer {
	chart, summary, hist := fixture(t, "chart.json"), fixture(t, "summary.json"), fixture(t, "histogram.json")
	page1, page2 := fixture(t, "reviews_page1.json"), fixture(t, "reviews_page2.json")
	critic := fixture(t, "appdetails_metacritic.json")
	return func(r recorded) answer {
		switch {
		case strings.Contains(r.Path, "GetMostPlayedGames"):
			return ok(chart)
		case strings.Contains(r.Path, "appreviewhistogram"):
			return ok(hist)
		case strings.Contains(r.Path, "/appreviews/"):
			switch {
			case strings.Contains(r.Query, "num_per_page=0"):
				return ok(summary)
			case strings.Contains(r.Query, "cursor=AoIFQFYqVMAAAAB%2Fpage2%3D%3D"):
				return ok(page2)
			}
			return ok(page1)
		case strings.Contains(r.Path, "appdetails"):
			// Only Portal 2 has a score; any other game has none.
			if strings.Contains(r.Query, "appids=620") {
				return ok(critic)
			}
			id := strings.TrimPrefix(r.Query[strings.Index(r.Query, "appids="):], "appids=")
			return ok([]byte(`{"` + id + `":{"success":true,"data":[]}}`))
		}
		return status(404)
	}
}

// hltbRouter answers HowLongToBeat's token, search and game page.
func hltbRouter(t *testing.T, search, page string) func(recorded) answer {
	init := fixture(t, "hltb_init.json")
	var searchBody, pageBody []byte
	if search != "" {
		searchBody = fixture(t, search)
	}
	if page != "" {
		pageBody = fixture(t, page)
	}
	return func(r recorded) answer {
		switch {
		case strings.Contains(r.Path, "/api/search/site/init"):
			return ok(init)
		case strings.HasSuffix(r.Path, "/api/search/site"):
			return ok(searchBody)
		case strings.HasPrefix(r.Path, "/game/"):
			return ok(pageBody)
		}
		return status(404)
	}
}
