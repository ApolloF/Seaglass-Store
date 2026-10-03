package wishlist

import (
	"encoding/json"
	"os"
	"slices"
	"sync"
	"time"
)

// Imported games without a known source release are searched for on the
// source sites while Seaglass is idle. The bounds keep an import of
// thousands of games from flooding a source: at most SearchesPerImport
// games are queued per import, one game is searched every SearchSpacing
// (so at most 30 an hour), and a game isn't searched again within
// SearchAgain.
const (
	SearchesPerImport = 200
	SearchSpacing     = 2 * time.Minute
	SearchAgain       = 24 * time.Hour
)

// Searches is the idle search queue file. Safe for concurrent use.
type Searches struct {
	path string
	mu   sync.Mutex
	f    searchesFile
}

type searchesFile struct {
	Schema   int                  `json:"schema"`
	Queue    []string             `json:"queue"`    // wishlist keys, next first
	Searched map[string]time.Time `json:"searched"` // key → when it was last searched
	Last     time.Time            `json:"last"`     // the last search of any game
}

// OpenSearches reads the queue; a missing or unreadable file is an empty
// queue.
func OpenSearches(path string) *Searches {
	q := &Searches{path: path, f: searchesFile{Schema: Schema, Queue: []string{}, Searched: map[string]time.Time{}}}
	if b, err := os.ReadFile(path); err == nil {
		var f searchesFile
		if json.Unmarshal(b, &f) == nil && f.Schema == Schema {
			if f.Queue == nil {
				f.Queue = []string{}
			}
			if f.Searched == nil {
				f.Searched = map[string]time.Time{}
			}
			q.f = f
		}
	}
	return q
}

// Enqueue queues the keys not searched within SearchAgain, at most
// SearchesPerImport new ones and MaxEntries in all. It returns how many of
// keys are queued now.
func (q *Searches) Enqueue(keys []string, now time.Time) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	added, queued := 0, 0
	for _, k := range keys {
		switch {
		case slices.Contains(q.f.Queue, k):
			queued++
		case q.recent(k, now), added == SearchesPerImport, len(q.f.Queue) >= MaxEntries:
		default:
			q.f.Queue = append(q.f.Queue, k)
			added++
			queued++
		}
	}
	if added == 0 {
		return queued, nil
	}
	return queued, q.save(now)
}

func (q *Searches) recent(key string, now time.Time) bool {
	t, ok := q.f.Searched[key]
	return ok && now.Sub(t) < SearchAgain
}

// Next is the next game to search. With none, wait is how long until one
// may be searched: the rest of SearchSpacing, or 0 when nothing is queued.
func (q *Searches) Next(now time.Time) (key string, wait time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.f.Queue = slices.DeleteFunc(q.f.Queue, func(k string) bool { return q.recent(k, now) })
	if len(q.f.Queue) == 0 {
		return "", 0
	}
	if w := q.f.Last.Add(SearchSpacing).Sub(now); w > 0 {
		return "", w
	}
	return q.f.Queue[0], 0
}

// Len is how many games wait for a search.
func (q *Searches) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.f.Queue)
}

// Done records that key was searched and takes it off the queue.
func (q *Searches) Done(key string, now time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.f.Queue = slices.DeleteFunc(q.f.Queue, func(k string) bool { return k == key })
	q.f.Searched[key] = now
	q.f.Last = now
	return q.save(now)
}

// Drop takes key off the queue without a search: it isn't saved anymore,
// or it has a source release now.
func (q *Searches) Drop(key string, now time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := len(q.f.Queue)
	q.f.Queue = slices.DeleteFunc(q.f.Queue, func(k string) bool { return k == key })
	if len(q.f.Queue) == n {
		return nil
	}
	return q.save(now)
}

// save forgets searches older than SearchAgain, which no longer hold a
// game back, and writes the file.
func (q *Searches) save(now time.Time) error {
	for k, t := range q.f.Searched {
		if now.Sub(t) >= SearchAgain {
			delete(q.f.Searched, k)
		}
	}
	return writeJSON(q.path, q.f)
}
