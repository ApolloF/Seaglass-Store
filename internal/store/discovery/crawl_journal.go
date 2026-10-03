package discovery

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
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
// the journal into the source file: it sets the journal aside as held
// lines, new pages start a new journal, and the held lines are removed
// once the source file holding them is in place. Opening the index
// replays the held lines, then the journal. Lines are numbered, so lines
// the source file already holds are skipped.

// journalCompactAt is the journal size from which a pass folds it into
// the source file, so replaying it at start stays quick.
const journalCompactAt = 4 << 20

// journalLine is one saved page.
type journalLine struct {
	Schema  int        `json:"schema"`
	Seq     int64      `json:"seq,omitempty"`
	Crawl   CrawlState `json:"crawl"`
	Records []Record   `json:"records,omitempty"`
}

func (ix *Index) journalPath(src string) string { return filepath.Join(ix.dir, src+".journal") }

// heldPath holds a journal's lines while Save writes them to the source
// file.
func (ix *Index) heldPath(src string) string { return filepath.Join(ix.dir, src+".journal.folding") }

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
// while the line is written, so Save, which sets the journal aside under
// the same lock, holds every line its snapshot contains.
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
	ix.seq[src]++
	line := journalLine{Schema: IndexSchema, Seq: ix.seq[src], Crawl: *c}
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

// replayJournal applies a source's held lines and then its journal over
// its loaded file, skipping the lines up to folded, which the file holds.
// A line cut short by a crash ends the replay: that page is simply fetched
// again.
func (ix *Index) replayJournal(src string, folded int64) {
	if ix.replayFile(ix.heldPath(src), src, folded) {
		ix.replayFile(ix.journalPath(src), src, folded)
	}
}

// replayFile replays one journal file; false when a line was cut short,
// so later lines mustn't be applied over the gap.
func (ix *Index) replayFile(path, src string, folded int64) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	for _, raw := range bytes.Split(b, []byte("\n")) {
		if len(raw) == 0 {
			continue
		}
		var l journalLine
		if json.Unmarshal(raw, &l) != nil || l.Schema != IndexSchema {
			return false
		}
		ix.seq[src] = max(ix.seq[src], l.Seq)
		if l.Seq != 0 && l.Seq <= folded {
			continue
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
	return true
}

// holdJournal sets a source's journal aside before Save writes its lines
// to the source file; new pages then start a new journal. Lines still
// held after a failed save get these appended. ix.mu is held.
func (ix *Index) holdJournal(src string) error {
	path, held := ix.journalPath(src), ix.heldPath(src)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if _, err := os.Stat(held); errors.Is(err, fs.ErrNotExist) {
		return os.Rename(path, held)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(held, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Remove(path)
}
