package wishlist

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/store/catalog"
)

// MaxEntries bounds the wishlist; MaxActivity a game's kept activity.
const (
	MaxEntries  = 2000
	MaxActivity = 50
)

// ErrFull means the wishlist has MaxEntries games.
var ErrFull = errors.New("the wishlist is full")

// Store is the wishlist file. Safe for concurrent use.
type Store struct {
	path string
	mu   sync.Mutex
	f    File
}

// Open reads wishlist.json; a missing or unreadable file is an empty
// wishlist.
func Open(path string) *Store {
	s := &Store{path: path, f: File{Schema: Schema, Entries: []Entry{}}}
	if b, err := os.ReadFile(path); err == nil {
		var f File
		if json.Unmarshal(b, &f) == nil && f.Schema == Schema {
			for i := range f.Entries {
				if f.Entries[i].Baseline == nil {
					f.Entries[i].Baseline = []string{}
				}
				if f.Entries[i].Activity == nil {
					f.Entries[i].Activity = []Activity{}
				}
			}
			if f.Entries == nil {
				f.Entries = []Entry{}
			}
			s.f = f
		}
	}
	return s
}

// List returns the saved games, most recently saved first.
func (s *Store) List() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Entries are kept in the order they were saved; walking them backwards
	// keeps games saved at the same instant newest first too.
	out := make([]Entry, 0, len(s.f.Entries))
	for i := len(s.f.Entries) - 1; i >= 0; i-- {
		out = append(out, clone(s.f.Entries[i]))
	}
	slices.SortStableFunc(out, func(a, b Entry) int { return b.AddedAt.Compare(a.AddedAt) })
	return out
}

// Get returns one saved game.
func (s *Store) Get(key string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.find(key); i >= 0 {
		return clone(s.f.Entries[i]), true
	}
	return Entry{}, false
}

func (s *Store) find(key string) int {
	return slices.IndexFunc(s.f.Entries, func(e Entry) bool { return e.Key == key })
}

func clone(e Entry) Entry {
	e.Baseline = slices.Clone(e.Baseline)
	e.Activity = slices.Clone(e.Activity)
	return e
}

// Add saves a game. The releases known now are its baseline: only later
// ones can become activity. Saving a saved game again changes nothing.
func (s *Store) Add(key, title string, appID int, known []Observation, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.find(key) >= 0 {
		return nil
	}
	if len(s.f.Entries) >= MaxEntries {
		return ErrFull
	}
	e := Entry{Key: key, Title: title, SteamAppID: appID, AddedAt: now, Baseline: []string{}, Activity: []Activity{}}
	for _, o := range known {
		e.Baseline = append(e.Baseline, o.ReleaseID)
		e.Version = newest(e.Version, o.Version)
	}
	s.f.Entries = append(s.f.Entries, e)
	return s.save()
}

// Remove forgets a saved game and its activity.
func (s *Store) Remove(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.find(key)
	if i < 0 {
		return nil
	}
	s.f.Entries = slices.Delete(s.f.Entries, i, i+1)
	return s.save()
}

// Acknowledge marks a saved game's activity read ("" marks every game's).
func (s *Store) Acknowledge(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for i := range s.f.Entries {
		if key != "" && s.f.Entries[i].Key != key {
			continue
		}
		for j := range s.f.Entries[i].Activity {
			if !s.f.Entries[i].Activity[j].Read {
				s.f.Entries[i].Activity[j].Read, changed = true, true
			}
		}
	}
	if !changed {
		return nil
	}
	return s.save()
}

// Rekey follows a game whose key changed (a Steam match correction).
func (s *Store) Rekey(old, key, title string, appID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.find(old)
	if i < 0 || old == key {
		return nil
	}
	if j := s.find(key); j >= 0 {
		// Already saved under the new key: keep one entry with both histories.
		s.f.Entries[j].Baseline = append(s.f.Entries[j].Baseline, s.f.Entries[i].Baseline...)
		s.f.Entries[j].Activity = append(s.f.Entries[j].Activity, s.f.Entries[i].Activity...)
		s.f.Entries = slices.Delete(s.f.Entries, i, i+1)
		return s.save()
	}
	e := &s.f.Entries[i]
	e.Key, e.SteamAppID = key, appID
	if title != "" {
		e.Title = title
	}
	return s.save()
}

// Observe compares a saved game's releases with its baseline and records
// activity: its first source release when it had none, and releases of a
// confirmed newer game version. Releases from backfill join the baseline
// silently. It reports whether anything changed.
func (s *Store) Observe(key string, releases []Observation, now time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.find(key)
	if i < 0 {
		return false, nil
	}
	e := &s.f.Entries[i]
	hadReleases := len(e.Baseline) > 0
	changed := false
	// Oldest first, so the first release of a game is the one announced.
	sorted := slices.Clone(releases)
	slices.SortStableFunc(sorted, func(a, b Observation) int { return a.PublishedAt.Compare(b.PublishedAt) })
	for _, o := range sorted {
		if o.ReleaseID == "" || slices.Contains(e.Baseline, o.ReleaseID) {
			continue
		}
		e.Baseline = append(e.Baseline, o.ReleaseID)
		changed = true
		kind := ""
		switch {
		case o.Backfill:
		case !hadReleases:
			kind = KindAvailable
		case isNewer(o.Version, e.Version):
			kind = KindNewer
		}
		e.Version = newest(e.Version, o.Version)
		hadReleases = true
		if kind == "" {
			continue
		}
		e.Activity = append(e.Activity, Activity{ID: activityID(e.Key, o.ReleaseID, kind), Kind: kind, ReleaseID: o.ReleaseID, Source: o.Source,
			SourceName: o.SourceName, Version: o.Version, At: now})
		if n := len(e.Activity); n > MaxActivity {
			e.Activity = slices.Clone(e.Activity[n-MaxActivity:])
		}
	}
	if !changed {
		return false, nil
	}
	return true, s.save()
}

// isNewer: a confirmed newer game version; incomparable claims never are.
func isNewer(v, than string) bool {
	if than == "" || v == "" {
		return false
	}
	order, ok := catalog.CompareReleases(v, than)
	return ok && order > 0
}

func newest(a, b string) string {
	if a == "" {
		return b
	}
	if isNewer(b, a) {
		return b
	}
	return a
}

func activityID(key, release, kind string) string {
	sum := sha256.Sum256([]byte(key + "\x00" + release + "\x00" + kind))
	return hex.EncodeToString(sum[:8])
}

// View returns an entry's activity for the interface, newest first, and
// how much is unread.
func View(e Entry) ([]ActivityView, int) {
	out := make([]ActivityView, 0, len(e.Activity))
	unread := 0
	for i := len(e.Activity) - 1; i >= 0; i-- {
		a := e.Activity[i]
		out = append(out, ActivityView{ID: a.ID, Kind: a.Kind, ReleaseID: a.ReleaseID, Source: a.Source, SourceName: a.SourceName, Version: a.Version, At: a.At.Unix(), Read: a.Read})
		if !a.Read {
			unread++
		}
	}
	return out, unread
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
