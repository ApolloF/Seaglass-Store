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
// SearchAgain. A search that didn't reach every source asks only the
// missing ones on the game's next turn, and a game is given up after
// SearchMisses such searches: a source that stays down doesn't keep the
// queue, and the sources that answered, busy.
const (
	SearchesPerImport = 200
	SearchSpacing     = 2 * time.Minute
	SearchAgain       = 24 * time.Hour
	SearchMisses      = 3
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
	// Pending is what earlier searches of a queued game left open.
	Pending map[string]pendingSearch `json:"pending,omitempty"`
}

// pendingSearch is a queued game whose search missed a source.
type pendingSearch struct {
	Answered []string `json:"answered"` // sources that answered its search
	Misses   int      `json:"misses"`   // searches that missed a source
}

// OpenSearches reads the queue; a missing or unreadable file is an empty
// queue.
func OpenSearches(path string) *Searches {
	q := &Searches{path: path, f: searchesFile{Schema: Schema, Queue: []string{}, Searched: map[string]time.Time{}, Pending: map[string]pendingSearch{}}}
	if b, err := os.ReadFile(path); err == nil {
		var f searchesFile
		if json.Unmarshal(b, &f) == nil && f.Schema == Schema {
			if f.Queue == nil {
				f.Queue = []string{}
			}
			if f.Searched == nil {
				f.Searched = map[string]time.Time{}
			}
			if f.Pending == nil {
				f.Pending = map[string]pendingSearch{}
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

// Next is the next game to search: the first queued game for which
// ready, given the sources that already answered its search, says a
// search can get further now (nil ready: any game). With none, wait is how long until one may be searched:
// the rest of SearchSpacing, or 0 when no game is queued or ready.
func (q *Searches) Next(now time.Time, ready func(answered []string) bool) (key string, wait time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.f.Queue = slices.DeleteFunc(q.f.Queue, func(k string) bool { return q.recent(k, now) })
	if len(q.f.Queue) == 0 {
		return "", 0
	}
	if w := q.f.Last.Add(SearchSpacing).Sub(now); w > 0 {
		return "", w
	}
	for _, k := range q.f.Queue {
		if ready == nil || ready(q.f.Pending[k].Answered) {
			return k, 0
		}
	}
	return "", 0
}

// Answered are the sources that already answered a queued game's search.
func (q *Searches) Answered(key string) []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return slices.Clone(q.f.Pending[key].Answered)
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
	q.done(key, now)
	return q.save(now)
}

func (q *Searches) done(key string, now time.Time) {
	q.f.Queue = slices.DeleteFunc(q.f.Queue, func(k string) bool { return k == key })
	delete(q.f.Pending, key)
	q.f.Searched[key] = now
	q.f.Last = now
}

// Later records a search that didn't reach every source: answered are
// the sources that did, so they aren't asked again. key goes to the back
// of the queue, and the spacing applies before the next search. After
// SearchMisses such searches the game counts as searched.
func (q *Searches) Later(key string, answered []string, now time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !slices.Contains(q.f.Queue, key) {
		q.f.Last = now
		return q.save(now)
	}
	p := q.f.Pending[key]
	p.Misses++
	if p.Misses >= SearchMisses {
		q.done(key, now)
		return q.save(now)
	}
	for _, src := range answered {
		if !slices.Contains(p.Answered, src) {
			p.Answered = append(p.Answered, src)
		}
	}
	q.f.Pending[key] = p
	q.f.Queue = append(slices.DeleteFunc(q.f.Queue, func(k string) bool { return k == key }), key)
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
	delete(q.f.Pending, key)
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
