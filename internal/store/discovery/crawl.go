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

// Crawl pacing. Older listing pages are indexed in batches of
// PagesPerPass; while a source has older pages left, the next batch starts
// right after the last one. The per-source sources.Client keeps the pace:
// one request in flight, at least two seconds apart.
const (
	RecentEvery  = 6 * time.Hour // the newest listings are fetched again after this
	PagesPerPass = 5             // listing pages per source per batch
	backoffBase  = time.Minute
	backoffMax   = time.Hour
	retryMax     = 6 * time.Hour // a longer Retry-After is capped: the next start asks again
)

// Fetcher fetches source documents: a sources.Client, which keeps one
// request in flight per source, two seconds apart.
type Fetcher interface {
	FetchDocument(ctx context.Context, raw string) (sources.Document, error)
}

// Due says what a source needs now: the newest listings when they are six
// hours old, and older pages until a confirmed end. force is a manual
// refresh or the first pass after setup: it asks for the newest listings
// regardless of age. Nothing is due before a source's Retry-After or
// backoff ends.
func Due(c CrawlState, now time.Time, force bool) (recent, backfill bool) {
	if now.Before(c.RetryAt) {
		return false, false
	}
	recent = force || c.RecentAt.IsZero() || now.Sub(c.RecentAt) >= RecentEvery
	return recent, !c.BackfillDone
}

// PassResult says what a pass did.
type PassResult struct {
	Merged  MergeResult
	Recent  bool // the newest listings were refreshed
	Pages   int  // listing pages fetched
	Stopped string
	Err     error
	// More: the batch ended with older pages left, so the next one can
	// start at once.
	More bool
}

// Pass stops reasons.
const (
	StopPaused   = "paused"   // a game started, or the person paused indexing
	StopCanceled = "canceled" // the Store or the source was turned off, or Seaglass is closing
)

// PassHooks let the caller stop a pass and follow it page by page.
type PassHooks struct {
	// Paused is asked before every request; true stops the pass there.
	Paused func() bool
	// Page is told after each listing page was merged and saved; backfill
	// says it was an older page.
	Page func(backfill bool)
}

// Pass runs one batch for a source: the newest listings when due, then
// older listing pages, PagesPerPass listing pages at most. Each page's
// records and the crawl position are saved before the next request, so a
// restart resumes at the next page without skipping or losing one. A
// catalog of one finite page is complete after its first page.
func Pass(ctx context.Context, ix *Index, p sources.Provider, f Fetcher, force bool, now func() time.Time, h PassHooks) PassResult {
	var res PassResult
	src := p.Source
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
		if h.Paused != nil && h.Paused() {
			res.Stopped = StopPaused
			return true
		}
		return false
	}
	// commit saves a page and its crawl change before anything else is
	// asked; false when even that couldn't be saved.
	commit := func(entries []sources.Entry, origin string, older bool, crawl func(*CrawlState)) (MergeResult, bool) {
		m, err := ix.commitPage(src.ID, entries, origin, older, now(), crawl)
		res.Merged.Added += m.Added
		res.Merged.Changed += m.Changed
		res.Merged.IDs = append(res.Merged.IDs, m.IDs...)
		if err != nil {
			res.Err = fmt.Errorf("saving the index: %w", err)
			return m, false
		}
		if h.Page != nil {
			h.Page(older)
		}
		return m, true
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
		// A feed carries full articles for the newest releases.
		if p.Feed != "" {
			// A feed of announcements only is not a failure.
			entries, _, err := fetchParse(ctx, src, f, p.Feed)
			if err != nil && !errors.Is(err, errNoReleases) {
				return failed(ix, src.ID, &res, err, now())
			}
			if _, ok := commit(entries, OriginRSS, false, nil); !ok {
				return res
			}
		}
		first := c.RecentAt.IsZero()
		for page := 1; budget > 0; page++ {
			if stop() {
				return res
			}
			raw, ok := p.ListingURL(page)
			if !ok {
				break
			}
			entries, next, err := fetchParse(ctx, src, f, raw)
			budget--
			res.Pages++
			if err != nil && !errors.Is(err, errNoReleases) {
				return failed(ix, src.ID, &res, err, now())
			}
			if !p.Paged() {
				// A finite catalog lists its whole history on one page;
				// only its recently dated rows can be news.
				if _, ok := commit(catalogHistory(entries, now(), true), OriginListing, true, nil); !ok {
					return res
				}
				entries = catalogHistory(entries, now(), false)
			}
			m, ok := commit(entries, OriginListing, false, nil)
			if !ok {
				return res
			}
			// Catch up page by page after a long pause, until a page brings
			// nothing new; the very first refresh reads one page and leaves
			// the rest to backfill.
			if first || m.Added == 0 || next == "" {
				break
			}
		}
		res.Recent = true
		if _, ok := commit(nil, OriginListing, false, func(c *CrawlState) {
			c.RecentAt, c.Failures, c.RetryAt, c.Error = now(), 0, time.Time{}, ""
			if c.NextPage < 2 {
				c.NextPage = 2
			}
			if !p.Paged() {
				c.BackfillDone = true
			}
		}); !ok {
			return res
		}
	}

	for budget > 0 {
		c = ix.Crawl(src.ID)
		if c.BackfillDone {
			break
		}
		if stop() {
			return res
		}
		page := max(c.NextPage, 2)
		raw, ok := p.ListingURL(page)
		if !ok {
			commit(nil, OriginListing, true, func(c *CrawlState) { c.BackfillDone = true })
			break
		}
		entries, next, err := fetchParse(ctx, src, f, raw)
		budget--
		res.Pages++
		var status *sources.HTTPError
		switch {
		case errors.As(err, &status) && (status.Status == http.StatusNotFound || status.Status == http.StatusGone):
			// Past the last page: a confirmed end.
			commit(nil, OriginListing, true, func(c *CrawlState) { c.BackfillDone, c.Failures, c.Error = true, 0, "" })
			return res
		case errors.Is(err, errNoReleases) && next != "":
			// A page of announcements only; move on rather than stall here.
			entries = nil
		case err != nil:
			return failed(ix, src.ID, &res, err, now())
		}
		if _, ok := commit(entries, OriginListing, true, func(c *CrawlState) {
			c.Failures, c.RetryAt, c.Error = 0, time.Time{}, ""
			if next == "" {
				c.BackfillDone = true
			} else {
				c.NextPage = page + 1
			}
		}); !ok {
			return res
		}
		if next == "" {
			break
		}
	}
	res.More = !ix.Crawl(src.ID).BackfillDone
	return res
}

var errNoReleases = errors.New("the page lists no releases")

// catalogHistory picks a finite catalog's history (history true) or its
// news: rows without a date, as in an alphabetic index, and rows dated more
// than a week back are history, so turning the source on never presents its
// whole back catalog as wishlist news.
func catalogHistory(entries []sources.Entry, now time.Time, history bool) []sources.Entry {
	var out []sources.Entry
	for _, e := range entries {
		at := e.PublishedAt
		if at == nil {
			at = e.UpdatedAt
		}
		old := at == nil || now.Sub(*at) > 7*24*time.Hour
		if old == history {
			out = append(out, e)
		}
	}
	return out
}

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

// failed records a failed request: Retry-After when the source sent one,
// else capped exponential backoff. Cancellation isn't a failure.
func failed(ix *Index, src string, res *PassResult, err error, now time.Time) PassResult {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		res.Stopped = StopCanceled
		return *res
	}
	res.Err = err
	// The wait is saved at once: a restart honours it too.
	if _, jerr := ix.commitPage(src, nil, OriginListing, false, now, func(c *CrawlState) { Backoff(c, err, now) }); jerr != nil {
		res.Err = errors.Join(err, fmt.Errorf("saving the index: %w", jerr))
	}
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
