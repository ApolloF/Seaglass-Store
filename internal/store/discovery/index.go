package discovery

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/store/sources"
)

// MaxRecords bounds one source's index. FitGirl and DODI publish a few
// thousand articles each; the margin leaves room for years more.
const MaxRecords = 50000

// sourceFile is one source's part of the index on disk.
type sourceFile struct {
	Schema int `json:"schema"`
	// Folded is the last journal line the file holds; replaying skips it
	// and the lines before it.
	Folded  int64      `json:"folded,omitempty"`
	Crawl   CrawlState `json:"crawl"`
	Records []Record   `json:"records"`
}

// identityFile is the person's corrections, kept in roaming app data.
type identityFile struct {
	Steam []IdentityCorrection `json:"steam"`
	// Completion maps a game key to the HowLongToBeat game the person
	// chose (0 is never stored: it means automatic).
	Completion map[string]int `json:"completion,omitempty"`
}

// Index is the persistent discovery index. Safe for concurrent use.
type Index struct {
	dir          string // %LOCALAPPDATA%\Seaglass\store\discovery
	identityPath string // %APPDATA%\Seaglass\store-identity.json

	// saveMu keeps one Save at a time, from its snapshot until its files
	// are in place, so an older snapshot never lands after a newer one.
	saveMu sync.Mutex
	// identityMu does the same for the corrections file.
	identityMu sync.Mutex

	mu       sync.RWMutex
	records  map[string]map[string]*Record // source → entry ID → record
	crawl    map[string]*CrawlState
	identity identityFile
	dirty    map[string]bool  // sources whose file needs saving
	seq      map[string]int64 // the last journal line written, per source
	version  int              // bumped on every change, for cached views
}

// OpenIndex loads the index from dir and the corrections from
// identityPath. Missing, unreadable or older-schema files start empty: the
// index is a cache and indexing again rebuilds it. A source file that is
// there but can't be used keeps the records its journal holds, but not
// the journal's crawl position: the pages before it were in the lost file,
// so the source is indexed again from its newest page.
func OpenIndex(dir, identityPath string) *Index {
	ix := &Index{dir: dir, identityPath: identityPath, records: map[string]map[string]*Record{}, crawl: map[string]*CrawlState{}, dirty: map[string]bool{}, seq: map[string]int64{}}
	removeTemps(dir, "*.json.*.tmp")
	removeTemps(filepath.Dir(identityPath), filepath.Base(identityPath)+".*.tmp")
	cutShort := false
	for _, src := range Sources {
		ix.records[src] = map[string]*Record{}
		ix.crawl[src] = &CrawlState{Source: src}
		folded, err := ix.loadSource(src)
		if !ix.replayJournal(src, folded) {
			cutShort = true
		}
		if err != nil {
			ix.restartCrawl(src)
		}
	}
	if b, err := os.ReadFile(identityPath); err == nil {
		_ = json.Unmarshal(b, &ix.identity)
	}
	if cutShort {
		// Pages appended after a cut-short line would be skipped on the
		// next start too: fold what was replayed into the source files now,
		// which removes the journal. A failed save keeps the lines held,
		// and the next save tries again.
		_ = ix.Save()
	}
	return ix
}

// loadSource reads a source's file and returns the last journal line it
// holds. A missing file is no error: the source wasn't indexed yet.
func (ix *Index) loadSource(src string) (int64, error) {
	b, err := os.ReadFile(ix.sourcePath(src))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var f sourceFile
	if err := json.Unmarshal(b, &f); err != nil {
		return 0, err
	}
	if f.Schema != IndexSchema {
		return 0, errors.New("the index file has another schema")
	}
	f.Crawl.Source = src
	ix.crawl[src] = &f.Crawl
	for i := range f.Records {
		r := f.Records[i]
		if r.Entry.ID == "" || r.Entry.SourceID != src {
			continue
		}
		ix.records[src][r.Entry.ID] = &r
	}
	ix.seq[src] = f.Folded
	return f.Folded, nil
}

// restartCrawl forgets how far a source was indexed and keeps only its
// wait: a Retry-After or a backoff still holds.
func (ix *Index) restartCrawl(src string) {
	c := ix.crawl[src]
	*c = CrawlState{Source: src, Failures: c.Failures, RetryAt: c.RetryAt, Error: c.Error}
}

// removeTemps deletes temporary files an interrupted save left behind;
// each save writes a file of its own, so nothing else replaces them.
func removeTemps(dir, pattern string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if ok, _ := filepath.Match(pattern, e.Name()); ok && !e.IsDir() {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func (ix *Index) sourcePath(src string) string { return filepath.Join(ix.dir, src+".json") }

// Version changes whenever the index does.
func (ix *Index) Version() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.version
}

// Save writes the sources that changed, each atomically. A source's
// journal is set aside under the same lock as the snapshot and removed
// only once the file holding its lines is in place, so a crash or a failed
// write in between loses no page.
func (ix *Index) Save() error {
	ix.saveMu.Lock()
	defer ix.saveMu.Unlock()
	var errs []error
	files := map[string][]byte{}
	ix.mu.Lock()
	for src := range ix.dirty {
		f := sourceFile{Schema: IndexSchema, Folded: ix.seq[src], Crawl: *ix.crawl[src], Records: make([]Record, 0, len(ix.records[src]))}
		for _, r := range ix.records[src] {
			f.Records = append(f.Records, *r)
		}
		// A stable order keeps the file diffable and the writes deterministic.
		slices.SortFunc(f.Records, func(a, b Record) int { return strings.Compare(a.Entry.ID, b.Entry.ID) })
		b, err := json.Marshal(f)
		if err == nil {
			err = ix.holdJournal(src)
		}
		if err != nil {
			errs = append(errs, err) // still dirty: the next save tries again
			continue
		}
		files[src] = b
		delete(ix.dirty, src)
	}
	ix.mu.Unlock()
	for src, b := range files {
		if err := writeAtomic(ix.sourcePath(src), b); err != nil {
			ix.mu.Lock()
			ix.dirty[src] = true
			ix.mu.Unlock()
			errs = append(errs, err)
			continue
		}
		// Should this fail, replaying skips the lines: the file holds them.
		_ = os.Remove(ix.heldPath(src))
	}
	return errors.Join(errs...)
}

// replaceFile moves a written temporary file into place; tests make it
// fail.
var replaceFile = os.Rename

// writeAtomic writes through a temporary file of its own in the same
// directory, so two writers never share one, and renames it into place.
func writeAtomic(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		// The data must be on disk before the rename is: after a power
		// loss, the rename alone can survive and leave an empty file.
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = replaceFile(f.Name(), path)
	}
	if err != nil {
		_ = os.Remove(f.Name())
	}
	return err
}

// Crawl returns a copy of a source's crawl state.
func (ix *Index) Crawl(src string) CrawlState {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	if c := ix.crawl[src]; c != nil {
		return *c
	}
	return CrawlState{Source: src}
}

// SetCrawl changes a source's crawl state.
func (ix *Index) SetCrawl(src string, fn func(*CrawlState)) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	c := ix.crawl[src]
	if c == nil {
		return
	}
	fn(c)
	ix.dirty[src] = true
	ix.version++
}

// Counts returns the number of records per source.
func (ix *Index) Counts() map[string]int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	out := map[string]int{}
	for src, m := range ix.records {
		out[src] = len(m)
	}
	return out
}

// Record returns a copy of one record.
func (ix *Index) Record(src, id string) (Record, bool) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	r, ok := ix.records[src][id]
	if !ok {
		return Record{}, false
	}
	return cloneRecord(r), true
}

// Records returns copies of the records of these sources.
func (ix *Index) Records(srcs []string) []Record {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	var out []Record
	for _, src := range srcs {
		for _, r := range ix.records[src] {
			out = append(out, cloneRecord(r))
		}
	}
	return out
}

func cloneRecord(r *Record) Record {
	c := *r
	c.Entry.Transports = slices.Clone(r.Entry.Transports)
	c.Entry.References = slices.Clone(r.Entry.References)
	c.Entry.Warnings = slices.Clone(r.Entry.Warnings)
	c.Entry.SizeOptionsBytes = slices.Clone(r.Entry.SizeOptionsBytes)
	return c
}

// MergeResult says what a merge changed.
type MergeResult struct {
	Added, Changed int
	IDs            []string // the records touched, new or changed
	// Relisted counts read articles whose summary changed: they wait to be
	// read again (NeedsDetail).
	Relisted int
}

// Merge adds or updates parsed entries of one source. origin is rss,
// listing, search or detail; backfill marks entries found on older
// listing pages, so they never count as news.
func (ix *Index) Merge(src string, entries []sources.Entry, origin string, backfill bool, now time.Time) MergeResult {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	var res MergeResult
	m := ix.records[src]
	if m == nil {
		return res
	}
	for _, e := range entries {
		if e.SourceID != src || e.TitleKey == "" {
			continue
		}
		e.PageURL = CanonicalPage(e.PageURL)
		e.ID = sources.EntryID(src, e.PageURL)
		old := m[e.ID]
		if old == nil {
			if len(m) >= MaxRecords {
				continue
			}
			r := &Record{Entry: e, FirstSeen: now, Backfill: backfill, LastSeen: now, Origin: origin}
			if e.SummaryOnly {
				r.Listed, r.ListedAt = e.RawTitle, listedAt(e)
			} else {
				r.Detailed = now
			}
			m[e.ID] = r
			res.Added++
			res.IDs = append(res.IDs, e.ID)
			continue
		}
		old.LastSeen, old.Gone = now, false
		if e.SummaryOnly && !old.Entry.SummaryOnly {
			// A summary knows less than the article already read; keep the
			// article's claims and only fill a missing date. When the
			// summary itself changed, the article is read again.
			if old.Entry.PublishedAt == nil {
				old.Entry.PublishedAt = e.PublishedAt
			}
			if relisted(old, e, now) {
				old.Detailed, old.FetchError = time.Time{}, ""
				res.Relisted++
			}
			old.Listed, old.ListedAt = e.RawTitle, listedAt(e)
			continue
		}
		if e.SummaryOnly {
			old.Listed, old.ListedAt = e.RawTitle, listedAt(e)
		}
		keepResolved(&e, old.Entry)
		changed := !old.Entry.SummaryOnly && claimsDiffer(old.Entry, e)
		if e.PublishedAt == nil {
			e.PublishedAt = old.Entry.PublishedAt
		}
		if old.Entry.SummaryOnly && !e.SummaryOnly {
			old.Detailed = now
			res.IDs = append(res.IDs, e.ID)
		}
		if origin == OriginDetailFetch {
			old.Detailed, old.FetchError = now, ""
		}
		if changed {
			old.Changed = now
			res.Changed++
			res.IDs = append(res.IDs, e.ID)
		}
		old.Entry = e
	}
	if len(entries) > 0 {
		ix.dirty[src] = true
		ix.version++
	}
	return res
}

// previewRecheck is how often an announcement's page is read again while
// its source keeps listing it: the page can turn into the release without
// the listing changing.
const previewRecheck = 24 * time.Hour

// relisted says a read article's summary row now claims something else: a
// new title (another update, a build, a preview label gone) or a newer
// dated batch. A row that only lost its date, as when a news batch leaves
// a catalog's front page, is no change. An article whose last reading
// failed for good (FetchError) is read again once its row changes too.
func relisted(r *Record, e sources.Entry, now time.Time) bool {
	if r.Detailed.IsZero() && r.FetchError == "" {
		return false // already waiting to be read
	}
	if r.Entry.ReleaseKind == "preview" && !r.Detailed.IsZero() && now.Sub(r.Detailed) >= previewRecheck {
		return true
	}
	if r.Listed == "" {
		return false // read before its summary was seen: nothing to compare
	}
	at := listedAt(e)
	return e.RawTitle != r.Listed || !at.IsZero() && !at.Equal(r.ListedAt)
}

// listedAt is the date a summary row is listed under.
func listedAt(e sources.Entry) time.Time {
	switch {
	case e.PublishedAt != nil:
		return *e.PublishedAt
	case e.UpdatedAt != nil:
		return *e.UpdatedAt
	}
	return time.Time{}
}

// NeedsDetail says a record's release page should be read: only its
// summary is known, or its summary changed since the page was read.
func NeedsDetail(r Record) bool {
	return !r.Gone && r.FetchError == "" && (r.Entry.SummaryOnly || r.Detailed.IsZero())
}

// Origins of records.
const (
	OriginRSS         = "rss"
	OriginListing     = "listing"
	OriginSearch      = "search"
	OriginDetailFetch = "detail"
)

// keepResolved carries validated torrent identities and resolution states
// over from the record: a re-parsed article doesn't know what the resolver
// or the person attached.
func keepResolved(e *sources.Entry, old sources.Entry) {
	for _, t := range old.Transports {
		if t.MetadataSHA256 == "" || t.InfoHash == "" {
			continue
		}
		if i := slices.IndexFunc(e.Transports, func(n sources.Transport) bool { return n.InfoHash == t.InfoHash }); i >= 0 {
			e.Transports[i] = t
		} else {
			e.Transports = append(e.Transports, t)
		}
	}
	for i := range e.References {
		for _, o := range old.References {
			if o.URL == e.References[i].URL && e.References[i].State == "" {
				e.References[i].State, e.References[i].Reason = o.State, o.Reason
			}
		}
	}
	for _, w := range old.Warnings {
		if strings.HasPrefix(w, "Manual torrent attached") && !slices.Contains(e.Warnings, w) {
			e.Warnings = append(e.Warnings, w)
		}
	}
}

// claimsDiffer reports a change in what a release claims to be. Timestamps
// alone don't count: a touched article is not a new release.
func claimsDiffer(a, b sources.Entry) bool {
	if a.Title != b.Title || a.Version != b.Version || a.SizeClaim != b.SizeClaim || a.ReleaseKind != b.ReleaseKind || a.LanguageClaim != b.LanguageClaim {
		return true
	}
	hashes := func(e sources.Entry) []string {
		var out []string
		for _, t := range e.Transports {
			if t.MetadataSHA256 == "" { // resolved ones are ours, not the article's
				out = append(out, t.InfoHash+t.URI)
			}
		}
		slices.Sort(out)
		return out
	}
	return !slices.Equal(hashes(a), hashes(b))
}

// Update changes one record (a detail fetch failed, a torrent was
// attached). false when it doesn't exist.
func (ix *Index) Update(src, id string, fn func(*Record)) bool {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	r := ix.records[src][id]
	if r == nil {
		return false
	}
	fn(r)
	ix.dirty[src] = true
	ix.version++
	return true
}

// SteamCorrections returns the person's Steam identity corrections by
// title key.
func (ix *Index) SteamCorrections() map[string]IdentityCorrection {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	out := make(map[string]IdentityCorrection, len(ix.identity.Steam))
	for _, c := range ix.identity.Steam {
		out[c.TitleKey] = c
	}
	return out
}

// SetSteamCorrections records corrections for these title keys and saves
// them at once (they are the person's choices, not cache).
func (ix *Index) SetSteamCorrections(cs []IdentityCorrection) error {
	return ix.saveIdentity(func() {
		for _, c := range cs {
			ix.identity.Steam = slices.DeleteFunc(ix.identity.Steam, func(o IdentityCorrection) bool { return o.TitleKey == c.TitleKey })
			ix.identity.Steam = append(ix.identity.Steam, c)
		}
		ix.version++
	})
}

// saveIdentity changes the corrections and writes them, one write at a
// time from the change until the file is in place, so the latest change
// is the one on disk.
func (ix *Index) saveIdentity(change func()) error {
	ix.identityMu.Lock()
	defer ix.identityMu.Unlock()
	ix.mu.Lock()
	change()
	b, err := json.MarshalIndent(ix.identity, "", "  ")
	ix.mu.Unlock()
	if err != nil {
		return err
	}
	return writeAtomic(ix.identityPath, b)
}

// CompletionMatch returns the HowLongToBeat game the person chose for a
// game key (0: automatic).
func (ix *Index) CompletionMatch(key string) int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.identity.Completion[key]
}

// SetCompletionMatch records the person's HowLongToBeat choice (0 clears it).
func (ix *Index) SetCompletionMatch(key string, id int) error {
	return ix.saveIdentity(func() {
		if ix.identity.Completion == nil {
			ix.identity.Completion = map[string]int{}
		}
		if id == 0 {
			delete(ix.identity.Completion, key)
		} else {
			ix.identity.Completion[key] = id
		}
	})
}

// CanonicalPage is the article URL records are keyed by: no query or
// fragment, a lowercase host and a trailing slash, so the listing, the
// feed and a search find the same record.
func CanonicalPage(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	scheme, rest, ok := strings.Cut(raw, "://")
	if !ok {
		return raw
	}
	host, path, _ := strings.Cut(rest, "/")
	path = "/" + path
	if !strings.HasSuffix(path, "/") && !strings.Contains(path[strings.LastIndex(path, "/"):], ".") {
		path += "/"
	}
	return strings.ToLower(scheme) + "://" + strings.ToLower(host) + path
}
