package discovery

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/store/sources"
)

// Crawl pacing agreed in the plan.
const (
	RecentEvery  = 6 * time.Hour    // the newest listings are fetched again after this
	PassEvery    = 30 * time.Minute // background passes come at most this often
	PagesPerPass = 5                // listing pages per source per pass
	backoffBase  = time.Minute
	backoffMax   = time.Hour
	retryMax     = 6 * time.Hour // a longer Retry-After is capped: the next start asks again
)

// Fetcher fetches source documents: a sources.Client, which keeps one
// request in flight per source, two seconds apart.
type Fetcher interface {
	FetchDocument(ctx context.Context, raw string) (sources.Document, error)
}

// Due says what a source needs now. force is a manual refresh or the first
// pass after setup: it asks for the newest listings regardless of age, but
// never before a source's Retry-After or backoff ends.
func Due(c CrawlState, now time.Time, force bool) (recent, backfill bool) {
	if now.Before(c.RetryAt) {
		return false, false
	}
	recent = force || c.RecentAt.IsZero() || now.Sub(c.RecentAt) >= RecentEvery
	backfill = !c.BackfillDone && (c.PassAt.IsZero() || now.Sub(c.PassAt) >= PassEvery)
	return recent, backfill
}

// PassResult says what a pass did.
type PassResult struct {
	Merged  MergeResult
	Recent  bool // the newest listings were refreshed
	Pages   int  // listing pages fetched
	Stopped string
	Err     error
}

// Pass stops reasons.
const (
	StopPaused   = "paused"   // a game started
	StopCanceled = "canceled" // the Store or the source was turned off, or Seaglass is closing
)

// Pass runs one pass for a source: the newest listings when due, then
// older listing pages, PagesPerPass listing pages at most. paused is asked
// before every request; a game starting stops the pass where it is.
func Pass(ctx context.Context, ix *Index, src sources.Source, f Fetcher, force bool, now func() time.Time, paused func() bool) PassResult {
	var res PassResult
	c := ix.Crawl(src.ID)
	recent, backfill := Due(c, now(), force)
	if !recent && !backfill {
		return res
	}
	budget := PagesPerPass
	stop := func() bool {
		if ctx.Err() != nil {
			res.Stopped = StopCanceled
			return true
		}
		if paused != nil && paused() {
			res.Stopped = StopPaused
			return true
		}
		return false
	}
	merge := func(entries []sources.Entry, origin string, older bool) MergeResult {
		m := ix.Merge(src.ID, entries, origin, older, now())
		res.Merged.Added += m.Added
		res.Merged.Changed += m.Changed
		res.Merged.IDs = append(res.Merged.IDs, m.IDs...)
		return m
	}
	defer func() {
		ix.SetCrawl(src.ID, func(c *CrawlState) {
			if res.Stopped == "" || res.Pages > 0 {
				c.PassAt = now()
			}
		})
	}()

	if recent {
		if stop() {
			return res
		}
		// FitGirl's feed carries full articles for the newest releases.
		if src.ID == "fitgirl" {
			// A feed of announcements only is not a failure.
			entries, _, err := fetchParse(ctx, src, f, src.StartURL)
			if err != nil && !errors.Is(err, errNoReleases) {
				return failed(ix, src.ID, &res, err, now())
			}
			merge(entries, OriginRSS, false)
		}
		first := c.RecentAt.IsZero()
		for page := 1; budget > 0; page++ {
			if stop() {
				return res
			}
			entries, next, err := fetchParse(ctx, src, f, pageURL(src, page))
			budget--
			res.Pages++
			if err != nil && !errors.Is(err, errNoReleases) {
				return failed(ix, src.ID, &res, err, now())
			}
			m := merge(entries, OriginListing, false)
			// Catch up page by page after a long pause, until a page brings
			// nothing new; the very first refresh reads one page and leaves
			// the rest to backfill.
			if first || m.Added == 0 || next == "" {
				break
			}
		}
		res.Recent = true
		ix.SetCrawl(src.ID, func(c *CrawlState) {
			c.RecentAt, c.Failures, c.RetryAt, c.Error = now(), 0, time.Time{}, ""
			if c.NextPage < 2 {
				c.NextPage = 2
			}
		})
	}

	for budget > 0 {
		c = ix.Crawl(src.ID)
		if c.BackfillDone || !backfill {
			break
		}
		if stop() {
			return res
		}
		page := max(c.NextPage, 2)
		entries, next, err := fetchParse(ctx, src, f, pageURL(src, page))
		budget--
		res.Pages++
		var status *sources.HTTPError
		switch {
		case errors.As(err, &status) && (status.Status == http.StatusNotFound || status.Status == http.StatusGone):
			// Past the last page: a confirmed end.
			ix.SetCrawl(src.ID, func(c *CrawlState) { c.BackfillDone, c.Failures, c.Error = true, 0, "" })
			return res
		case errors.Is(err, errNoReleases) && next != "":
			// A page of announcements only; move on rather than stall here.
		case err != nil:
			return failed(ix, src.ID, &res, err, now())
		default:
			merge(entries, OriginListing, true)
		}
		ix.SetCrawl(src.ID, func(c *CrawlState) {
			c.Failures, c.RetryAt, c.Error = 0, time.Time{}, ""
			if next == "" {
				c.BackfillDone = true
			} else {
				c.NextPage = page + 1
			}
		})
		if next == "" {
			break
		}
	}
	return res
}

var errNoReleases = errors.New("the page lists no releases")

// fetchParse fetches and parses one source document; next is the page's
// own "next" link ("" at the end).
func fetchParse(ctx context.Context, src sources.Source, f Fetcher, raw string) ([]sources.Entry, string, error) {
	doc, err := f.FetchDocument(ctx, raw)
	if err != nil {
		return nil, "", err
	}
	next, _ := sources.NextPage(src, doc.URL, doc.Body)
	if strings.Contains(doc.URL, "/feed/") {
		next = "" // the feed's pages aren't the article listing
	}
	entries, err := sources.Parse(src, doc.URL, doc.Body)
	if err != nil {
		if strings.Contains(err.Error(), "no release articles found") || strings.Contains(err.Error(), "no release items found") {
			return nil, next, errNoReleases
		}
		return nil, next, err
	}
	return entries, next, nil
}

func pageURL(src sources.Source, page int) string {
	if page <= 1 {
		return "https://" + src.Host + "/"
	}
	return "https://" + src.Host + "/page/" + strconv.Itoa(page) + "/"
}

// failed records a failed request: Retry-After when the source sent one,
// else capped exponential backoff. Cancellation isn't a failure.
func failed(ix *Index, src string, res *PassResult, err error, now time.Time) PassResult {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		res.Stopped = StopCanceled
		return *res
	}
	res.Err = err
	ix.SetCrawl(src, func(c *CrawlState) { Backoff(c, err, now) })
	return *res
}

// Backoff updates a crawl state after a failed request.
func Backoff(c *CrawlState, err error, now time.Time) {
	c.Failures++
	c.Error = err.Error()
	wait := backoffBase << min(c.Failures-1, 10)
	wait = min(wait, backoffMax)
	var status *sources.HTTPError
	if errors.As(err, &status) {
		if d, ok := RetryAfter(status.RetryAfter, now); ok {
			wait = min(max(d, backoffBase), retryMax)
		}
		switch status.Status {
		case http.StatusTooManyRequests, http.StatusServiceUnavailable:
			c.Error = fmt.Sprintf("the source asked to slow down (HTTP %d)", status.Status)
		}
	}
	c.RetryAt = now.Add(wait)
}

// RetryAfter reads a Retry-After header: seconds or an HTTP date.
func RetryAfter(v string, now time.Time) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return time.Duration(n) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(t.Sub(now), 0), true
	}
	return 0, false
}

// FetchDetail reads a release's article and updates its record. A missing
// article marks the record unavailable instead of deleting it.
func FetchDetail(ctx context.Context, ix *Index, src sources.Source, f Fetcher, id string, now time.Time) error {
	r, ok := ix.Record(src.ID, id)
	if !ok {
		return errors.New("that release isn't listed anymore")
	}
	if now.Before(ix.Crawl(src.ID).RetryAt) {
		return fmt.Errorf("%s asked to wait; try again after %s", src.Name, ix.Crawl(src.ID).RetryAt.Local().Format("15:04"))
	}
	doc, err := f.FetchDocument(ctx, r.Entry.PageURL)
	var status *sources.HTTPError
	if errors.As(err, &status) && (status.Status == http.StatusNotFound || status.Status == http.StatusGone) {
		ix.Update(src.ID, id, func(r *Record) { r.Gone, r.FetchError = true, "the article is gone" })
		return nil
	}
	if err != nil {
		if errors.As(err, &status) && (status.Status == http.StatusTooManyRequests || status.Status == http.StatusServiceUnavailable) {
			ix.SetCrawl(src.ID, func(c *CrawlState) { Backoff(c, err, now) })
		}
		if !errors.Is(err, context.Canceled) {
			ix.Update(src.ID, id, func(r *Record) { r.FetchError = err.Error() })
		}
		return err
	}
	entries, err := sources.Parse(src, doc.URL, doc.Body)
	if err != nil {
		ix.Update(src.ID, id, func(r *Record) { r.FetchError = err.Error() })
		return err
	}
	want := CanonicalPage(r.Entry.PageURL)
	for _, e := range entries {
		if CanonicalPage(e.PageURL) == want || CanonicalPage(doc.URL) == CanonicalPage(e.PageURL) && len(entries) == 1 {
			e.PageURL = r.Entry.PageURL
			ix.Merge(src.ID, []sources.Entry{e}, OriginDetailFetch, r.Backfill, now)
			return nil
		}
	}
	ix.Update(src.ID, id, func(r *Record) { r.FetchError = "the release page lists no matching release" })
	return errors.New("the release page lists no matching release")
}

// Resolve tries the supported torrent-metadata resolver once for a
// release without a validated transport, and records the outcome.
func Resolve(ctx context.Context, ix *Index, src string, id string, resolver *sources.Resolver) {
	r, ok := ix.Record(src, id)
	if !ok || len(validTransports(r.Entry)) > 0 {
		return
	}
	entries := []sources.Entry{r.Entry}
	sources.ResolveEntries(ctx, resolver, entries, 1)
	e := entries[0]
	ix.Update(src, id, func(r *Record) {
		r.Entry.References = e.References
		for _, t := range e.Transports {
			if !containsHash(r.Entry.Transports, t.InfoHash) {
				r.Entry.Transports = append(r.Entry.Transports, t)
			}
		}
		r.Entry.Warnings = e.Warnings
	})
}

func containsHash(ts []sources.Transport, hash string) bool {
	for _, t := range ts {
		if t.InfoHash == hash {
			return true
		}
	}
	return false
}

// Attach adds a torrent the person chose for a release, after
// sources.TorrentMetadata validated it.
func Attach(ix *Index, src, id string, t sources.Transport) bool {
	return ix.Update(src, id, func(r *Record) {
		if !containsHash(r.Entry.Transports, t.InfoHash) {
			r.Entry.Transports = append(r.Entry.Transports, t)
		}
		const note = "Manual torrent attached: confirm that its name matches the selected game."
		if !strings.Contains(strings.Join(r.Entry.Warnings, "\n"), note) {
			r.Entry.Warnings = append(r.Entry.Warnings, note)
		}
	})
}
