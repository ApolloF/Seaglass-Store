// Package jobs keeps the experimental store's downloads: what was asked
// for, how far it got, and what the person wants done with it next. It
// survives restarts; the torrent engine is told what to do from it.
package jobs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/torrent"
)

// State is where a download is.
type State string

const (
	Queued      State = "queued"      // waiting to be handed to the engine
	Downloading State = "downloading" // in the engine and wanted
	Paused      State = "paused"      // paused by the person
	Downloaded  State = "downloaded"  // every wanted file is on disk
	Failed      State = "failed"      // stopped with an error; can be retried
)

// Action is something the person asks of a download.
type Action string

const (
	Pause  Action = "pause"
	Resume Action = "resume" // also retries a failed download
	Remove Action = "remove"
)

// Can reports whether action makes sense for a download in state s.
func Can(s State, a Action) bool {
	switch a {
	case Pause:
		return s == Queued || s == Downloading
	case Resume:
		return s == Paused || s == Failed
	case Remove:
		return true
	}
	return false
}

// ErrNotFound means there's no such download.
var ErrNotFound = errors.New("no such download")

// Job is one download.
type Job struct {
	ID       string `json:"id"` // also the engine's tag for its torrent
	Title    string `json:"title"`
	Source   string `json:"source"` // magnet link or .torrent URL
	SavePath string `json:"savePath"`
	// From the catalog (empty for a link the person pasted).
	GameKey  string `json:"gameKey,omitempty"`
	Version  string `json:"version,omitempty"`
	FeedName string `json:"feedName,omitempty"`
	// How to install it, chosen before downloading.
	InstallDir  string `json:"installDir,omitempty"`  // the game's own folder
	Language    string `json:"language,omitempty"`    // as the feed names it; "" for the installer's default
	AutoInstall bool   `json:"autoInstall,omitempty"` // install as soon as it's downloaded and checked
	Hash     string `json:"hash,omitempty"` // once the engine has it
	Name     string `json:"name,omitempty"` // the torrent's own name: its folder (or file) in SavePath
	State    State  `json:"state"`
	Error    string `json:"error,omitempty"`
	// Progress, from the engine.
	Engine    torrent.State `json:"engine,omitempty"`
	Size      int64         `json:"size"`
	Done      int64         `json:"done"`
	DownSpeed int64         `json:"downSpeed"`
	UpSpeed   int64         `json:"upSpeed"`
	Seeds     int           `json:"seeds"`
	Peers     int           `json:"peers"`
	ETA       int64         `json:"eta"`
	Seeding   bool          `json:"seeding"`
	Created   int64         `json:"created"` // unix seconds
	Finished  int64         `json:"finished,omitempty"`
}

// Active reports whether the job needs the engine running.
func (j Job) Active() bool {
	return j.State == Queued || j.State == Downloading || j.Seeding
}

// Sync takes the engine's view of the job's torrent (nil: the engine
// doesn't have it) and reports whether anything worth saving changed.
func (j *Job) Sync(t *torrent.Torrent, now time.Time) bool {
	before := *j
	if t == nil {
		j.Engine, j.DownSpeed, j.UpSpeed, j.Seeds, j.Peers, j.ETA, j.Seeding = "", 0, 0, 0, 0, 0, false
		if j.State == Downloading {
			j.State = Queued // lost from the engine (its profile was reset): add it again
		}
		return j.State != before.State
	}
	j.Hash, j.Name, j.Engine, j.Size, j.Done = t.Hash, t.Name, t.State, t.Size, t.Done
	j.DownSpeed, j.UpSpeed, j.Seeds, j.Peers, j.ETA = t.DownSpeed, t.UpSpeed, t.Seeds, t.Peers, t.ETA
	j.Seeding = t.State == torrent.Seeding
	switch t.State {
	case torrent.Seeding, torrent.Complete:
		if j.State != Downloaded {
			j.State, j.Error, j.Finished = Downloaded, "", now.Unix()
		}
	case torrent.Failed:
		if j.State != Failed {
			j.State, j.Error = Failed, "The download stopped: its files can't be written or were moved."
		}
	default:
		if j.State == Queued {
			j.State = Downloading
		}
	}
	return j.State != before.State || j.Hash != before.Hash || j.Name != before.Name || j.Error != before.Error
}

// Store holds the downloads, saved as JSON. Safe for concurrent use.
type Store struct {
	path   string
	mu     sync.Mutex
	saveMu sync.Mutex // one write of the file at a time
	jobs   []Job
}

// Open loads the downloads at path. A damaged file is set aside and the
// backup from the last good open is used.
func Open(path string) *Store {
	s := &Store{path: path}
	for _, p := range []string{path, path + ".bak"} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var jobs []Job
		if err := json.Unmarshal(b, &jobs); err != nil {
			if p == path {
				_ = os.Rename(path, path+".broken-"+time.Now().Format("20060102-150405"))
			}
			continue
		}
		if p == path {
			_ = writeAtomic(path+".bak", b)
		}
		s.jobs = jobs
		break
	}
	return s
}

// All returns the downloads, oldest first.
func (s *Store) All() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.jobs)
}

// Get returns one download.
func (s *Store) Get(id string) (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.index(id); i >= 0 {
		return s.jobs[i], true
	}
	return Job{}, false
}

// Add queues a new download (its title, source, save path and catalog
// fields; the rest is filled in) and returns it.
func (s *Store) Add(j Job, now time.Time) (Job, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return Job{}, err
	}
	j = Job{ID: "sg-" + hex.EncodeToString(b), Title: j.Title, Source: j.Source, SavePath: j.SavePath,
		GameKey: j.GameKey, Version: j.Version, FeedName: j.FeedName,
		InstallDir: j.InstallDir, Language: j.Language, AutoInstall: j.AutoInstall, State: Queued, Created: now.Unix()}
	s.mu.Lock()
	s.jobs = append(s.jobs, j)
	s.mu.Unlock()
	return j, s.Save()
}

// Update changes a download in place (fn's change is kept when it
// reports true) and saves when it changed.
func (s *Store) Update(id string, fn func(*Job) bool) (Job, error) {
	s.mu.Lock()
	i := s.index(id)
	if i < 0 {
		s.mu.Unlock()
		return Job{}, ErrNotFound
	}
	changed := fn(&s.jobs[i])
	j := s.jobs[i]
	s.mu.Unlock()
	if changed {
		return j, s.Save()
	}
	return j, nil
}

// Delete forgets a download.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	i := s.index(id)
	if i >= 0 {
		s.jobs = slices.Delete(s.jobs, i, i+1)
	}
	s.mu.Unlock()
	if i < 0 {
		return ErrNotFound
	}
	return s.Save()
}

// Save writes the downloads to disk.
func (s *Store) Save() error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.mu.Lock()
	b, err := json.MarshalIndent(s.jobs, "", " ")
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return writeAtomic(s.path, b)
}

func (s *Store) index(id string) int {
	return slices.IndexFunc(s.jobs, func(j Job) bool { return j.ID == id })
}

func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
