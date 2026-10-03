package discovery

import (
	"encoding/json"
	"errors"
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
	Schema  int        `json:"schema"`
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

	mu       sync.RWMutex
	records  map[string]map[string]*Record // source → entry ID → record
	crawl    map[string]*CrawlState
	identity identityFile
	dirty    map[string]bool // sources whose file needs saving
	version  int             // bumped on every change, for cached views
}

// OpenIndex loads the index from dir and the corrections from
// identityPath. Missing, unreadable or older-schema files start empty: the
// index is a cache and indexing again rebuilds it.
func OpenIndex(dir, identityPath string) *Index {
	ix := &Index{dir: dir, identityPath: identityPath, records: map[string]map[string]*Record{}, crawl: map[string]*CrawlState{}, dirty: map[string]bool{}}
	for _, src := range Sources {
		ix.records[src] = map[string]*Record{}
		ix.crawl[src] = &CrawlState{Source: src}
		b, err := os.ReadFile(ix.sourcePath(src))
		if err != nil {
			continue
		}
		var f sourceFile
		if json.Unmarshal(b, &f) != nil || f.Schema != IndexSchema {
			continue
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
	}
	for _, src := range Sources {
		ix.replayJournal(src)
	}
	if b, err := os.ReadFile(identityPath); err == nil {
		_ = json.Unmarshal(b, &ix.identity)
	}
	return ix
}

func (ix *Index) sourcePath(src string) string { return filepath.Join(ix.dir, src+".json") }

// Version changes whenever the index does.
func (ix *Index) Version() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.version
}

// Save writes the sources that changed, each atomically.
func (ix *Index) Save() error {
	ix.mu.Lock()
	var files = map[string][]byte{}
	for src := range ix.dirty {
		f := sourceFile{Schema: IndexSchema, Crawl: *ix.crawl[src], Records: make([]Record, 0, len(ix.records[src]))}
		for _, r := range ix.records[src] {
			f.Records = append(f.Records, *r)
		}
		// A stable order keeps the file diffable and the writes deterministic.
		slices.SortFunc(f.Records, func(a, b Record) int { return strings.Compare(a.Entry.ID, b.Entry.ID) })
		b, err := json.Marshal(f)
		if err != nil {
			ix.mu.Unlock()
			return err
		}
		files[src] = b
		ix.dropJournal(src)
	}
	ix.dirty = map[string]bool{}
	ix.mu.Unlock()
	var errs []error
	for src, b := range files {
		if err := writeAtomic(ix.sourcePath(src), b); err != nil {
			ix.mu.Lock()
			ix.dirty[src] = true
			ix.mu.Unlock()
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
			if !e.SummaryOnly {
				r.Detailed = now
			}
			m[e.ID] = r
			res.Added++
			res.IDs = append(res.IDs, e.ID)
			continue
		}
		old.LastSeen, old.Gone = now, false
		if e.SummaryOnly && !old.Entry.SummaryOnly {
			// A search summary knows less than the article already read;
			// keep the article's claims and only fill a missing date.
			if old.Entry.PublishedAt == nil {
				old.Entry.PublishedAt = e.PublishedAt
			}
			continue
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
	ix.mu.Lock()
	for _, c := range cs {
		ix.identity.Steam = slices.DeleteFunc(ix.identity.Steam, func(o IdentityCorrection) bool { return o.TitleKey == c.TitleKey })
		ix.identity.Steam = append(ix.identity.Steam, c)
	}
	ix.version++
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
	ix.mu.Lock()
	if ix.identity.Completion == nil {
		ix.identity.Completion = map[string]int{}
	}
	if id == 0 {
		delete(ix.identity.Completion, key)
	} else {
		ix.identity.Completion[key] = id
	}
	b, err := json.MarshalIndent(ix.identity, "", "  ")
	ix.mu.Unlock()
	if err != nil {
		return err
	}
	return writeAtomic(ix.identityPath, b)
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
