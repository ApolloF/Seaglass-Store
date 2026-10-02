package enrich

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFreshCacheHitMakesNoRequest(t *testing.T) {
	c, fn, clk := newTestClient(t, steamRouter(t))
	ctx := context.Background()
	c.Critic(ctx, 620)
	c.ReviewSummary(ctx, 620)
	c.Popularity(ctx)
	c.Reviews(ctx, ReviewQuery{AppID: 620})
	n := fn.count()
	clk.Advance(30 * time.Minute)
	for i := 0; i < 3; i++ {
		if cr := c.Critic(ctx, 620); cr.State != StateOK || cr.Score != 95 {
			t.Fatalf("critic %+v", cr)
		}
		if s := c.ReviewSummary(ctx, 620); s.State != StateOK {
			t.Fatalf("summary %+v", s)
		}
		if p := c.Popularity(ctx); p.State != StateOK {
			t.Fatalf("popularity %+v", p)
		}
		if p := c.Reviews(ctx, ReviewQuery{AppID: 620}); p.State != StateOK {
			t.Fatalf("reviews %+v", p)
		}
	}
	if fn.count() != n {
		t.Fatalf("%d requests after the cache was filled, want none", fn.count()-n)
	}
}

func TestCacheLifetimesFollowThePlan(t *testing.T) {
	c, fn, clk := newTestClient(t, steamRouter(t))
	ctx := context.Background()
	steps := []struct {
		name string
		ttl  time.Duration
		call func()
	}{
		{"popularity", PopularityTTL, func() { c.Popularity(ctx) }},
		{"review summary", ReviewSummaryTTL, func() { c.ReviewSummary(ctx, 620) }},
		{"review page", ReviewPageTTL, func() { c.Reviews(ctx, ReviewQuery{AppID: 620}) }},
		{"critic", CriticTTL, func() { c.Critic(ctx, 620) }},
	}
	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			s.call()
			n := fn.count()
			clk.Advance(s.ttl - time.Second)
			s.call()
			if fn.count() != n {
				t.Fatal("asked again before the lifetime ended")
			}
			clk.Advance(2 * time.Second)
			s.call()
			if fn.count() == n {
				t.Fatal("did not ask again after the lifetime ended")
			}
		})
	}
}

func TestFailedFetchReturnsTheCachedAnswerMarkedStale(t *testing.T) {
	fail := false
	router := steamRouter(t)
	c, _, clk := newTestClient(t, func(r recorded) answer {
		if fail {
			return status(500)
		}
		return router(r)
	})
	ctx := context.Background()
	c.Critic(ctx, 620)
	c.ReviewSummary(ctx, 620)
	c.Reviews(ctx, ReviewQuery{AppID: 620})
	clk.Advance(30 * 24 * time.Hour)
	fail = true
	if cr := c.Critic(ctx, 620); cr.State != StateStale || cr.Score != 95 || cr.Error == "" {
		t.Fatalf("critic %+v", cr)
	}
	if s := c.ReviewSummary(ctx, 620); s.State != StateStale || s.Overall.Total != 467426 || s.Error == "" {
		t.Fatalf("summary %+v", s)
	}
	if p := c.Reviews(ctx, ReviewQuery{AppID: 620}); p.State != StateStale || len(p.Reviews) != 3 || p.Error == "" {
		t.Fatalf("reviews %+v", p)
	}
}

func TestFailedFetchWithNothingCachedSaysUnavailableOrError(t *testing.T) {
	cases := map[string]struct {
		a    answer
		want string
	}{
		"server error":   {status(500), StateError},
		"network down":   {answer{err: errors.New("dial tcp: no route to host")}, StateError},
		"garbled answer": {ok([]byte("<html>maintenance</html>")), StateError},
		"rate limited":   {status(429), StateUnavailable},
		"page missing":   {status(404), StateUnavailable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// A failure pauses the host, so each question gets its own client.
			fresh := func() *Client {
				c, _, _ := newTestClient(t, func(recorded) answer { return tc.a })
				return c
			}
			ctx := context.Background()
			cr := fresh().Critic(ctx, 620)
			if cr.State != tc.want || cr.Error == "" || cr.Score != 0 || cr.AppID != 620 {
				t.Fatalf("critic %+v", cr)
			}
			s := fresh().ReviewSummary(ctx, 620)
			if s.State != tc.want || s.Error == "" || s.URL == "" {
				t.Fatalf("summary %+v", s)
			}
			p := fresh().Popularity(ctx)
			if p.State != tc.want || p.Ranks == nil || len(p.Ranks) != 0 {
				t.Fatalf("popularity %+v", p)
			}
			r := fresh().Reviews(ctx, ReviewQuery{AppID: 620})
			if r.State != tc.want || r.Reviews == nil || len(r.Reviews) != 0 {
				t.Fatalf("reviews %+v", r)
			}
		})
	}
}

func TestRetryAfterPausesTheHostUntilItPasses(t *testing.T) {
	throttled := true
	router := steamRouter(t)
	c, fn, clk := newTestClient(t, func(r recorded) answer {
		if throttled {
			return status(429, "Retry-After", "120")
		}
		return router(r)
	})
	ctx := context.Background()
	if cr := c.Critic(ctx, 620); cr.State != StateUnavailable {
		t.Fatalf("%+v", cr)
	}
	if fn.count() != 1 {
		t.Fatalf("%d requests", fn.count())
	}
	clk.Advance(119 * time.Second)
	throttled = false
	for _, id := range []int{620, 730} {
		cr := c.Critic(ctx, id)
		if cr.State != StateUnavailable || !strings.Contains(cr.Error, "paused") {
			t.Fatalf("during the pause: %+v", cr)
		}
	}
	if s := c.ReviewSummary(ctx, 620); s.State != StateUnavailable {
		t.Fatalf("every call on the same host waits: %+v", s)
	}
	if fn.count() != 1 {
		t.Fatalf("%d requests during the pause", fn.count())
	}
	// Another host is not affected.
	if p := c.Popularity(ctx); p.State != StateOK {
		t.Fatalf("popularity %+v", p)
	}
	clk.Advance(2 * time.Second)
	if cr := c.Critic(ctx, 620); cr.State != StateOK || cr.Score != 95 {
		t.Fatalf("after the pause: %+v", cr)
	}
}

func TestPauseKeepsAnsweringFromTheCacheAsStale(t *testing.T) {
	throttled := false
	router := steamRouter(t)
	c, fn, clk := newTestClient(t, func(r recorded) answer {
		if throttled {
			return status(503, "Retry-After", "600")
		}
		return router(r)
	})
	ctx := context.Background()
	c.Critic(ctx, 620)
	clk.Advance(CriticTTL + time.Hour)
	throttled = true
	if cr := c.Critic(ctx, 620); cr.State != StateStale || cr.Score != 95 {
		t.Fatalf("%+v", cr)
	}
	n := fn.count()
	clk.Advance(5 * time.Minute)
	if cr := c.Critic(ctx, 620); cr.State != StateStale || cr.Score != 95 || fn.count() != n {
		t.Fatalf("%+v, %d requests during the pause", cr, fn.count()-n)
	}
}

func TestRetryAfterAcceptsSecondsAndDates(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Duration{
		"":                                 0,
		"30":                               30 * time.Second,
		"-5":                               0,
		"soon":                             0,
		"Fri, 02 Oct 2026 12:05:00 GMT":    5 * time.Minute,
		"Fri, 02 Oct 2026 11:00:00 GMT":    0,
		http.TimeFormat:                    0,
		"Fri, 02 Oct 2026 12:00:10 GMT   ": 10 * time.Second,
	} {
		if got := parseRetryAfter(in, now); got != want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestFailuresBackOffExponentiallyUpToACap(t *testing.T) {
	h := newHostState("x", 0)
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	want := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 16 * time.Minute, 30 * time.Minute, 30 * time.Minute}
	for i, w := range want {
		h.mu.Lock()
		h.until = time.Time{}
		h.mu.Unlock()
		h.fail(now, 0)
		if until, _ := h.blocked(now); until.Sub(now) != w {
			t.Fatalf("failure %d: paused %v, want %v", i+1, until.Sub(now), w)
		}
	}
	h.succeed()
	if _, blocked := h.blocked(now); blocked {
		t.Fatal("a success ends the pause")
	}
	h.fail(now, 0)
	if until, _ := h.blocked(now); until.Sub(now) != time.Minute {
		t.Fatal("a success restarts the schedule")
	}
	h.fail(now, 3*time.Hour)
	if until, _ := h.blocked(now); until.Sub(now) != retryAfterMax {
		t.Fatalf("a server's wish is honoured up to %v, got %v", retryAfterMax, until.Sub(now))
	}
}

func TestRepeatedFailuresLengthenThePauseBetweenRequests(t *testing.T) {
	c, fn, clk := newTestClient(t, func(recorded) answer { return status(500) })
	ctx := context.Background()
	for i, pause := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute} {
		c.Critic(ctx, 620)
		if fn.count() != i+1 {
			t.Fatalf("round %d: %d requests", i, fn.count())
		}
		clk.Advance(pause - time.Second)
		if cr := c.Critic(ctx, 620); !strings.Contains(cr.Error, "paused") || fn.count() != i+1 {
			t.Fatalf("round %d: asked during the pause (%+v)", i, cr)
		}
		clk.Advance(2 * time.Second)
	}
}

func TestRequestsToOneHostAreSpacedApart(t *testing.T) {
	c, fn, clk := newTestClient(t, steamRouter(t))
	c.hosts[hostSteamStore].gap = gapSteamStore
	c.hosts[hostSteamAPI].gap = gapSteamAPI
	ctx := context.Background()
	start := clk.Now()
	c.Critic(ctx, 620)
	c.Critic(ctx, 730)
	c.Critic(ctx, 570)
	reqs := fn.find("appdetails")
	if len(reqs) != 3 {
		t.Fatalf("%d requests", len(reqs))
	}
	for i := 1; i < len(reqs); i++ {
		if d := reqs[i].At.Sub(reqs[i-1].At); d < gapSteamStore {
			t.Fatalf("requests %d and %d were %v apart, want at least %v", i-1, i, d, gapSteamStore)
		}
	}
	if !reqs[0].At.Equal(start) {
		t.Fatal("the first request has nothing to wait for")
	}
	// The API host has its own turn and gap.
	before := clk.Now()
	c.Popularity(ctx)
	if got := fn.find("GetMostPlayedGames")[0].At; !got.Equal(before) {
		t.Fatalf("another host waited %v", got.Sub(before))
	}
}

func TestHLTBRequestsAreSpacedTwoSecondsApart(t *testing.T) {
	c, fn, _ := newTestClient(t, hltbRouter(t, "hltb_search_portal2.json", "hltb_game_7231.html"))
	c.hosts[hostHLTB].gap = gapHLTB
	c.Completion(context.Background(), CompletionQuery{Title: "Portal 2"})
	if fn.count() != 2 {
		t.Fatalf("%d requests", fn.count())
	}
	if d := fn.last().At.Sub(fn.reqs[0].At); d < 2*time.Second {
		t.Fatalf("token and search were %v apart", d)
	}
}

func TestOnlyOneRequestPerHostIsInFlight(t *testing.T) {
	var inFlight, peak atomic.Int32
	router := steamRouter(t)
	c, _, _ := newTestClient(t, func(r recorded) answer {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
		inFlight.Add(-1)
		return router(r)
	})
	var wg sync.WaitGroup
	for _, id := range []int{1, 2, 3, 4, 5} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Critic(context.Background(), id)
		}()
	}
	wg.Wait()
	if peak.Load() != 1 {
		t.Fatalf("%d requests at once to one host", peak.Load())
	}
}

func TestConcurrentCallersShareOneRequest(t *testing.T) {
	release := make(chan struct{})
	router := steamRouter(t)
	c, fn, _ := newTestClient(t, func(r recorded) answer {
		<-release
		return router(r)
	})
	var wg sync.WaitGroup
	results := make([]Critic, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = c.Critic(context.Background(), 620)
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if fn.count() != 1 {
		t.Fatalf("%d requests for 8 callers", fn.count())
	}
	for i, cr := range results {
		if cr.State != StateOK || cr.Score != 95 {
			t.Fatalf("caller %d got %+v", i, cr)
		}
	}
}

func TestCancelledContextStopsTheWaitForTheNetwork(t *testing.T) {
	c, _, _ := newTestClient(t, steamRouter(t))
	slow := &blockingNet{release: make(chan struct{}), started: make(chan struct{}, 1)}
	c.http.Transport = slow
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Critic, 1)
	go func() { done <- c.Critic(ctx, 620) }()
	select {
	case <-slow.started:
	case <-time.After(3 * time.Second):
		t.Fatal("the request never started")
	}
	cancel()
	select {
	case cr := <-done:
		if cr.State != StateError || !strings.Contains(cr.Error, "canceled") {
			t.Fatalf("%+v", cr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the call did not return after its context was cancelled")
	}
	// A cancelled call is not a failing host.
	if _, blocked := c.hosts[hostSteamStore].blocked(c.now()); blocked {
		t.Fatal("cancelling paused the host")
	}
}

// blockingNet holds each request until its context ends.
type blockingNet struct {
	release chan struct{}
	started chan struct{}
}

func (b *blockingNet) RoundTrip(req *http.Request) (*http.Response, error) {
	select {
	case b.started <- struct{}{}:
	default:
	}
	select {
	case <-req.Context().Done():
		return nil, req.Context().Err()
	case <-b.release:
		return nil, errors.New("released")
	}
}

func TestCancelledContextStopsTheWaitForSpacing(t *testing.T) {
	c, _, _ := newTestClient(t, steamRouter(t))
	c.sleep = realSleep
	c.hosts[hostSteamStore].gap = time.Hour
	ctx := context.Background()
	c.Critic(ctx, 620)
	cctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	cr := c.Critic(cctx, 730)
	if cr.State != StateError || time.Since(start) > 3*time.Second {
		t.Fatalf("%+v after %v", cr, time.Since(start))
	}
}

func TestOnlyAllowlistedHTTPSHostsAreFetched(t *testing.T) {
	c, fn, _ := newTestClient(t, steamRouter(t))
	for _, raw := range []string{
		"https://example.com/x",
		"http://store.steampowered.com/api/appdetails",
		"https://store.steampowered.com.evil.example/",
		"https://evil.example/store.steampowered.com",
		"https://user@store.steampowered.com/",
		"https://www.metacritic.com/game/pc/portal-2",
		"ftp://store.steampowered.com/",
	} {
		_, err := c.fetch(context.Background(), request{url: raw})
		if !errors.Is(err, ErrNotAllowed) {
			t.Errorf("%s: error %v, want ErrNotAllowed", raw, err)
		}
	}
	if fn.count() != 0 {
		t.Fatalf("%d requests went out", fn.count())
	}
	for _, raw := range []string{
		"https://api.steampowered.com/x", "https://store.steampowered.com/x", "https://howlongtobeat.com/x",
	} {
		if _, err := c.fetch(context.Background(), request{url: raw}); errors.Is(err, ErrNotAllowed) {
			t.Errorf("%s should be allowed", raw)
		}
	}
}

func TestRedirectToAnotherHostIsRefused(t *testing.T) {
	c, fn, _ := newTestClient(t, func(r recorded) answer {
		if r.Host == "store.steampowered.com" {
			return status(302, "Location", "https://evil.example/steal")
		}
		return ok([]byte("secret"))
	})
	cr := c.Critic(context.Background(), 620)
	if cr.State != StateError || !strings.Contains(cr.Error, "host not allowed") {
		t.Fatalf("%+v", cr)
	}
	for _, r := range fn.reqs {
		if r.Host == "evil.example" {
			t.Fatal("the redirect was followed")
		}
	}
}

func TestRedirectToPlainHTTPIsRefused(t *testing.T) {
	c, fn, _ := newTestClient(t, func(r recorded) answer {
		return status(301, "Location", "http://store.steampowered.com/api/appdetails")
	})
	if cr := c.Critic(context.Background(), 620); cr.State != StateError || fn.count() != 1 {
		t.Fatalf("%+v after %d requests", cr, fn.count())
	}
}

func TestRedirectsWithinTheAllowlistAreFollowedButLimited(t *testing.T) {
	hops := 0
	c, _, _ := newTestClient(t, func(r recorded) answer {
		if r.Path == "/api/appdetails" && hops == 0 {
			hops++
			return status(302, "Location", "https://store.steampowered.com/api/appdetails2?filters=metacritic&appids=620")
		}
		if r.Path == "/api/appdetails2" {
			return ok(fixture(t, "appdetails_metacritic.json"))
		}
		return status(404)
	})
	if cr := c.Critic(context.Background(), 620); cr.State != StateOK || cr.Score != 95 {
		t.Fatalf("%+v", cr)
	}
	c2, fn2, _ := newTestClient(t, func(r recorded) answer {
		return status(302, "Location", "https://store.steampowered.com/loop")
	})
	if cr := c2.Critic(context.Background(), 620); cr.State != StateError || fn2.count() > maxRedirects+1 {
		t.Fatalf("%+v after %d requests", cr, fn2.count())
	}
}

func TestOversizedAnswerIsRejected(t *testing.T) {
	big := []byte(strings.Repeat("x", maxBody+1))
	for name, chunked := range map[string]bool{"length announced": false, "length unknown": true} {
		t.Run(name, func(t *testing.T) {
			c, _, _ := newTestClient(t, func(recorded) answer { return answer{status: 200, body: big, chunked: chunked} })
			cr := c.Critic(context.Background(), 620)
			if cr.State != StateError || !strings.Contains(cr.Error, "too large") {
				t.Fatalf("%+v", cr)
			}
		})
	}
	atCap := []byte(strings.Repeat(" ", maxBody-len(`{"620":{"success":false}}`)) + `{"620":{"success":false}}`)
	c, _, _ := newTestClient(t, func(recorded) answer { return ok(atCap) })
	if cr := c.Critic(context.Background(), 620); cr.State != StateOK {
		t.Fatalf("an answer of exactly the cap is fine: %+v", cr)
	}
}

func TestDiskCacheSurvivesANewClient(t *testing.T) {
	dir := t.TempDir()
	clk := newClock()
	fn := &fakeNet{handler: steamRouter(t), clock: clk}
	c1 := New(Options{Dir: dir, Transport: fn, Now: clk.Now})
	ctx := context.Background()
	c1.Popularity(ctx)
	c1.Critic(ctx, 620)
	c1.ReviewSummary(ctx, 620)
	c1.Reviews(ctx, ReviewQuery{AppID: 620})
	if err := c1.Flush(); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*"))
	for _, f := range files {
		if strings.HasSuffix(f, ".tmp") {
			t.Fatalf("temp file left behind: %s", f)
		}
	}
	if len(files) != 4 {
		t.Fatalf("files %v", files)
	}

	never := &fakeNet{handler: func(r recorded) answer { t.Errorf("unexpected request %s", r.URL); return status(500) }}
	c2 := New(Options{Dir: dir, Transport: never, Now: clk.Now})
	if p := c2.CachedPopularity(); p.State != StateOK || p.Ranks[730] != 1 {
		t.Fatalf("popularity %+v", p)
	}
	if cr := c2.Critic(ctx, 620); cr.State != StateOK || cr.Score != 95 {
		t.Fatalf("critic %+v", cr)
	}
	if s, ok := c2.CachedReviewSummary(620); !ok || s.State != StateOK || s.Overall.Total != 467426 {
		t.Fatalf("summary %+v %v", s, ok)
	}
	if p := c2.Reviews(ctx, ReviewQuery{AppID: 620}); p.State != StateOK || len(p.Reviews) != 3 {
		t.Fatalf("reviews %+v", p)
	}
	clk.Advance(2 * time.Hour)
	if p := c2.CachedPopularity(); p.State != StateStale {
		t.Fatalf("an expired cached chart is labelled stale, got %q", p.State)
	}
}

func TestFlushPersistsWhatTheThrottleHeldBack(t *testing.T) {
	c, _, _ := newTestClient(t, steamRouter(t))
	dir := c.opts.Dir
	ctx := context.Background()
	c.Critic(ctx, 620)
	c.Critic(ctx, 730) // within the write interval: held back
	read := func() string {
		b, _ := os.ReadFile(filepath.Join(dir, kindCritic+".json"))
		return string(b)
	}
	if strings.Contains(read(), `"730"`) {
		t.Fatal("the second write should have been held back")
	}
	if err := c.Flush(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(), `"730"`) {
		t.Fatalf("Flush did not write the pending entry: %s", read())
	}
}

func TestCorruptDiskCacheIsIgnored(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{kindPopularity, kindCritic, kindSummary, kindReviews, kindCompletion, kindSearch} {
		if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(`{"version":1,"entries":{"620":{"at":`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Right shape, wrong version; and right version with a broken value.
	os.WriteFile(filepath.Join(dir, kindCritic+".json"), []byte(`{"version":99,"entries":{"620":{"at":1,"v":{"score":1}}}}`), 0o644)
	os.WriteFile(filepath.Join(dir, kindSummary+".json"), []byte(`{"version":1,"entries":{"620":{"at":4102444800,"v":"not an object"}}}`), 0o644)
	clk := newClock()
	fn := &fakeNet{handler: steamRouter(t), clock: clk}
	c := New(Options{Dir: dir, Transport: fn, Now: clk.Now})
	ctx := context.Background()
	if cr := c.Critic(ctx, 620); cr.State != StateOK || cr.Score != 95 {
		t.Fatalf("critic %+v", cr)
	}
	if s := c.ReviewSummary(ctx, 620); s.State != StateOK || s.Overall.Total != 467426 {
		t.Fatalf("summary %+v", s)
	}
	if p := c.CachedPopularity(); p.State != StateUnavailable {
		t.Fatalf("popularity %+v", p)
	}
	if err := c.Flush(); err != nil {
		t.Fatalf("flush over corrupt files: %v", err)
	}
}

func TestUnwritableCacheDirDoesNotBreakAnswers(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "file")
	os.WriteFile(dir, []byte("not a directory"), 0o644)
	clk := newClock()
	fn := &fakeNet{handler: steamRouter(t), clock: clk}
	c := New(Options{Dir: filepath.Join(dir, "sub"), Transport: fn, Now: clk.Now})
	if cr := c.Critic(context.Background(), 620); cr.State != StateOK {
		t.Fatalf("%+v", cr)
	}
	if err := c.Flush(); err == nil {
		t.Fatal("Flush should report the write failure")
	}
}

func TestClientWithoutADirKeepsItsCacheInMemory(t *testing.T) {
	clk := newClock()
	fn := &fakeNet{handler: steamRouter(t), clock: clk}
	c := New(Options{Transport: fn, Now: clk.Now})
	ctx := context.Background()
	c.Critic(ctx, 620)
	c.Critic(ctx, 620)
	if fn.count() != 1 || c.Flush() != nil {
		t.Fatalf("%d requests, flush %v", fn.count(), c.Flush())
	}
}

func TestOldestEntriesGoWhenACacheKindIsFull(t *testing.T) {
	c, _, clk := newTestClient(t, steamRouter(t))
	for i := 0; i < maxEntries[kindReviews]+5; i++ {
		clk.Advance(time.Second)
		c.cachePut(kindReviews, "k"+string(rune('a'+i%26))+strings.Repeat("x", i), ReviewPage{AppID: i})
	}
	c.cmu.Lock()
	n := len(c.entries[kindReviews])
	c.cmu.Unlock()
	if n != maxEntries[kindReviews] {
		t.Fatalf("%d entries, want %d", n, maxEntries[kindReviews])
	}
	if _, _, ok := cacheGet[ReviewPage](c, kindReviews, "ka"); ok {
		t.Fatal("the oldest entry should be gone")
	}
}
