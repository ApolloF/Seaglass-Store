package discovery

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/ApolloF/Seaglass/internal/store/sources"
)

// Indexing progress is saved page by page in a journal next to each
// source's file: one line per page with the records it touched and the
// crawl position after it. Rewriting the whole source file instead would
// write its full size (about 1.3 KB per release: 7.5 MB and 26 ms for
// 6,000 releases, see BenchmarkSaveWholeSourcePerPage) for every ten
// releases read, against 13 KB and 0.3 ms for a journal line. Save folds
// the journal into the source file and starts a new one; opening the
// index replays it.

// journalCompactAt is the journal size from which a pass folds it into
// the source file, so replaying it at start stays quick.
const journalCompactAt = 4 << 20

// journalLine is one saved page.
type journalLine struct {
	Schema  int        `json:"schema"`
	Crawl   CrawlState `json:"crawl"`
	Records []Record   `json:"records,omitempty"`
}

func (ix *Index) journalPath(src string) string { return filepath.Join(ix.dir, src+".journal") }

// commitPage merges one page's entries, applies the crawl change (nil:
// none) and appends both to the source's journal, before the next request.
// It folds a large journal into the source file.
func (ix *Index) commitPage(src string, entries []sources.Entry, origin string, backfill bool, now time.Time, crawl func(*CrawlState)) (MergeResult, error) {
	m := ix.Merge(src, entries, origin, backfill, now)
	size, err := ix.journal(src, entries, crawl)
	if err != nil {
		// The journal can't be written: the whole file is the only way
		// left to keep the page.
		return m, ix.Save()
	}
	if size >= journalCompactAt {
		return m, ix.Save()
	}
	return m, nil
}

// journal applies the crawl change and appends the page. ix.mu is held
// while the line is written, so Save, which removes the journal under the
// same lock, never drops a line whose state it didn't write.
func (ix *Index) journal(src string, entries []sources.Entry, crawl func(*CrawlState)) (int64, error) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	c := ix.crawl[src]
	if c == nil {
		return 0, nil
	}
	if crawl != nil {
		crawl(c)
		ix.dirty[src] = true
		ix.version++
	}
	line := journalLine{Schema: IndexSchema, Crawl: *c}
	for _, e := range entries {
		if r := ix.records[src][sources.EntryID(src, CanonicalPage(e.PageURL))]; r != nil {
			line.Records = append(line.Records, cloneRecord(r))
		}
	}
	b, err := json.Marshal(line)
	if err != nil {
		return 0, err
	}
	path := ix.journalPath(src)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	_, err = f.Write(append(b, '\n'))
	st, serr := f.Stat()
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, err
	}
	if serr != nil {
		return 0, nil
	}
	return st.Size(), nil
}

// replayJournal applies a source's journal over its loaded file. A line
// cut short by a crash ends the replay: that page is simply fetched again.
func (ix *Index) replayJournal(src string) {
	b, err := os.ReadFile(ix.journalPath(src))
	if err != nil {
		return
	}
	for _, raw := range bytes.Split(b, []byte("\n")) {
		if len(raw) == 0 {
			continue
		}
		var l journalLine
		if json.Unmarshal(raw, &l) != nil || l.Schema != IndexSchema {
			break
		}
		l.Crawl.Source = src
		*ix.crawl[src] = l.Crawl
		for i := range l.Records {
			r := l.Records[i]
			if r.Entry.ID == "" || r.Entry.SourceID != src {
				continue
			}
			ix.records[src][r.Entry.ID] = &r
		}
		ix.dirty[src] = true
	}
}

// dropJournal forgets a journal whose state is about to be written to the
// source file. ix.mu is held.
func (ix *Index) dropJournal(src string) {
	_ = os.Remove(ix.journalPath(src))
}
