package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/store/jobs"
	"github.com/ApolloF/Seaglass/internal/torrent"
	"github.com/ApolloF/Seaglass/internal/torrent/qbit"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Events for the experimental store.
const (
	EventStoreJobs   = "store:jobs"   // the downloads, with their progress
	EventStoreEngine = "store:engine" // the download engine's status
)

func init() {
	application.RegisterEvent[[]jobs.Job](EventStoreJobs)
	application.RegisterEvent[EngineStatus](EventStoreEngine)
}

// ErrStoreDisabled means the experimental store is turned off.
var ErrStoreDisabled = errors.New("the store is turned off (Settings, Experimental)")

// proxySecret holds the download engine's proxy password.
const proxySecret = "store-proxy"

// EngineStatus is the download engine as the interface shows it.
type EngineStatus struct {
	Installed bool   `json:"installed"` // qBittorrent was found
	Exe       string `json:"exe"`
	Running   bool   `json:"running"`
	Version   string `json:"version,omitempty"`
	Error     string `json:"error,omitempty"`
	// InterfaceMissing: downloads are bound to an interface that's gone
	// (a VPN that disconnected), so nothing is sent or received.
	InterfaceMissing bool `json:"interfaceMissing"`
	GameRunning      bool `json:"gameRunning"` // downloads wait for the game to close
	Held             bool `json:"held"`        // paused from the tray until resumed
}

// storeState runs the experimental store's downloads: it starts the
// engine when there's something to download, hands it the queue, follows
// progress, and stops it again once it has been idle for a while.
type storeState struct {
	c    *Core
	jobs *jobs.Store
	pipe *pipeline
	kick chan struct{}

	mu       sync.Mutex
	side     *qbit.Sidecar
	status   EngineStatus
	hold     atomic.Bool          // every download waits (the tray's "Pause downloads")
	idle     time.Time            // since when nothing needed the engine
	added    map[string]time.Time // when a download was handed to the engine, which lists it a moment later
	ticks    int
	lastSave time.Time
}

// engineIdle is how long the engine keeps running with nothing to do.
const engineIdle = 2 * time.Minute

func newStoreState(c *Core) *storeState {
	st := &storeState{c: c, jobs: jobs.Open(filepath.Join(platform.AppDir(), "downloads.json")), kick: make(chan struct{}, 1)}
	st.pipe = newPipeline(st)
	return st
}

// downloadsDir is where downloads go.
func downloadsDir(s settings.StoreSettings) string {
	if s.Downloads != "" {
		return s.Downloads
	}
	base := platform.Downloads
	if base == "" {
		base = filepath.Join(platform.Profile, "Downloads")
	}
	return filepath.Join(base, "Seaglass")
}

func (st *storeState) qbittorrent() string {
	if p := st.c.Settings.Get().Store.QBittorrent; p != "" {
		return p
	}
	return qbit.Find()
}

func (st *storeState) loop(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-st.kick:
		}
		st.tick(ctx)
	}
}

// wake runs a tick soon, after the person changed something.
func (st *storeState) wake() {
	select {
	case st.kick <- struct{}{}:
	default:
	}
}

func (st *storeState) tick(ctx context.Context) {
	cfg := st.c.Settings.Get()
	all := st.jobs.All()
	if cfg.ExperimentalStore {
		st.pipe.advance(all)
	}
	need := false
	for _, j := range all {
		need = need || j.Active()
	}
	st.mu.Lock()
	running := st.side != nil && st.side.Running()
	if !need {
		if st.idle.IsZero() {
			st.idle = time.Now()
		}
		stop := running && (!cfg.ExperimentalStore || time.Since(st.idle) > engineIdle)
		st.mu.Unlock()
		if stop {
			st.stopEngine()
		}
		r := st.engineRunning()
		st.setStatus(func(s *EngineStatus) { s.Running, s.GameRunning = r, false })
		return
	}
	st.idle = time.Time{}
	st.mu.Unlock()
	if !cfg.ExperimentalStore {
		st.stopEngine()
		return
	}

	eng, err := st.engine(ctx)
	if err != nil {
		st.setStatus(func(s *EngineStatus) { s.Running, s.Error = false, err.Error() })
		return
	}
	ts, err := eng.List(ctx)
	if err != nil {
		st.setStatus(func(s *EngineStatus) { s.Error = err.Error() })
		return
	}
	byTag := map[string]*torrent.Torrent{}
	for i := range ts {
		byTag[ts[i].Tag] = &ts[i]
	}
	held := st.hold.Load()
	playing := cfg.Store.PauseWhilePlaying && st.c.Launch.Active()
	st.ticks++
	checkSpace := st.ticks%10 == 1
	now := time.Now()
	for _, j := range all {
		t := byTag[j.ID]
		st.steer(ctx, eng, j, t, playing || held, checkSpace)
		_, _ = st.jobs.Update(j.ID, func(j *jobs.Job) bool { return j.Sync(t, now) })
	}
	if st.ticks%10 == 1 {
		st.checkInterface(ctx, eng, cfg.Store.Network)
	}
	st.setStatus(func(s *EngineStatus) { s.Running, s.Error, s.GameRunning, s.Held = true, "", playing, held })
	if time.Since(st.lastSave) > 30*time.Second {
		st.lastSave = now
		if err := st.jobs.Save(); err != nil {
			logx.Printf("store: saving downloads: %v", err)
		}
	}
	st.c.emit(EventStoreJobs, st.jobs.All())
}

// steer makes the engine do what the person wants for one download.
func (st *storeState) steer(ctx context.Context, eng torrent.Engine, j jobs.Job, t *torrent.Torrent, playing, checkSpace bool) {
	wanted := j.State == jobs.Queued || j.State == jobs.Downloading
	switch {
	case t == nil && wanted:
		if at, ok := st.added[j.ID]; ok && time.Since(at) < 30*time.Second {
			return
		}
		if st.added == nil {
			st.added = map[string]time.Time{}
		}
		st.added[j.ID] = time.Now()
		if err := eng.Add(ctx, j.Source, torrent.AddOptions{SavePath: j.SavePath, Tag: j.ID, Paused: playing}); err != nil {
			st.fail(j.ID, err.Error())
		}
		return
	case t == nil:
		return
	}
	moving := t.State != torrent.Paused && t.State != torrent.Complete && t.State != torrent.Failed
	if checkSpace && wanted && t.Size > t.Done {
		if free, err := platform.FreeSpace(j.SavePath); err == nil && free < uint64(t.Size-t.Done)+256<<20 {
			_ = eng.Pause(ctx, t.Hash)
			st.fail(j.ID, fmt.Sprintf("Not enough free space: %s more is needed, %s is free.", bytesText(t.Size-t.Done), bytesText(int64(free))))
			return
		}
	}
	// Finished downloads keep seeding (up to the seed ratio), except while
	// a game runs.
	switch {
	case moving && (playing || j.State == jobs.Paused || j.State == jobs.Failed):
		_ = eng.Pause(ctx, t.Hash)
	case wanted && !playing && t.State == torrent.Paused:
		_ = eng.Resume(ctx, t.Hash)
	}
}

// removeTorrent takes a download's torrent out of the engine.
func (st *storeState) removeTorrent(j jobs.Job, deleteFiles bool) error {
	ctx, cancel := context.WithTimeout(st.c.ctx, 45*time.Second)
	defer cancel()
	eng, err := st.engine(ctx)
	if err != nil {
		return err
	}
	hash := j.Hash
	if hash == "" {
		ts, err := eng.List(ctx)
		if err != nil {
			return err
		}
		for _, t := range ts {
			if t.Tag == j.ID {
				hash = t.Hash
			}
		}
	}
	if hash == "" {
		return nil
	}
	return eng.Remove(ctx, deleteFiles, hash)
}

func (st *storeState) fail(id, msg string) {
	_, _ = st.jobs.Update(id, func(j *jobs.Job) bool {
		j.State, j.Error = jobs.Failed, msg
		return true
	})
}

// checkInterface notices when downloads are bound to an interface that's
// gone. qBittorrent then sends nothing at all, which is the point (a VPN
// kill-switch), but the person should know why downloads stand still.
func (st *storeState) checkInterface(ctx context.Context, eng torrent.Engine, n torrent.Network) {
	missing := false
	if n.Interface != "" {
		ifs, err := eng.Interfaces(ctx)
		if err != nil {
			return
		}
		missing = true
		for _, i := range ifs {
			missing = missing && i.ID != n.Interface
		}
	}
	st.setStatus(func(s *EngineStatus) { s.InterfaceMissing = missing })
}

// engine returns the running engine, starting it when needed.
func (st *storeState) engine(ctx context.Context) (torrent.Engine, error) {
	exe := st.qbittorrent()
	st.mu.Lock()
	if st.side != nil && !strings.EqualFold(st.side.Exe, exe) {
		old := st.side
		st.side = nil
		st.mu.Unlock()
		old.Stop()
		st.mu.Lock()
	}
	if st.side == nil {
		st.side = &qbit.Sidecar{Exe: exe, Profile: platform.CacheDir("store", "qbittorrent")}
	}
	side := st.side
	st.mu.Unlock()
	wasRunning := side.Running()
	c, err := side.Ensure(ctx, st.network())
	if err != nil {
		return nil, err
	}
	if !wasRunning {
		v, _ := c.Version(ctx)
		logx.Printf("store: qBittorrent %s started", v)
		st.setStatus(func(s *EngineStatus) { s.Version = v })
	}
	return c, nil
}

func (st *storeState) network() torrent.Network {
	n := st.c.Settings.Get().Store.Network
	n.ProxyPassword = platform.LoadSecret(proxySecret)
	return n
}

func (st *storeState) engineRunning() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.side != nil && st.side.Running()
}

func (st *storeState) stopEngine() {
	st.mu.Lock()
	side := st.side
	st.mu.Unlock()
	if side != nil && side.Running() {
		side.Stop()
		logx.Printf("store: qBittorrent stopped")
		st.setStatus(func(s *EngineStatus) { s.Running = false })
	}
}

// setHold pauses every download until resumed (or Seaglass restarts).
func (st *storeState) setHold(on bool) {
	st.hold.Store(on)
	if st.c.shell != nil {
		st.c.shell.syncTrayDownloads(st.c.Settings.Get().ExperimentalStore, on)
	}
	st.setStatus(func(s *EngineStatus) { s.Held = on })
	st.wake()
}

func (st *storeState) held() bool { return st.hold.Load() }

// settingsChanged applies changed store settings.
func (st *storeState) settingsChanged(old, saved settings.Settings) {
	if st.c.shell != nil && old.ExperimentalStore != saved.ExperimentalStore {
		st.c.shell.syncTrayDownloads(saved.ExperimentalStore, st.held())
	}
	if !saved.ExperimentalStore {
		go st.stopEngine()
		return
	}
	switch {
	case !old.ExperimentalStore:
		go st.c.catalog.refresh(st.c.ctx, false)
	case !reflect.DeepEqual(old.Store.Feeds, saved.Store.Feeds):
		go st.c.catalog.rebuild()
	}
	if saved.Store.Network != old.Store.Network && st.engineRunning() {
		go st.applyNetwork()
	}
	st.setStatus(func(*EngineStatus) {}) // qBittorrent's path may have changed
	st.wake()
}

func (st *storeState) applyNetwork() {
	ctx, cancel := context.WithTimeout(st.c.ctx, 15*time.Second)
	defer cancel()
	eng, err := st.engine(ctx)
	if err == nil {
		err = eng.Apply(ctx, st.network())
	}
	if err != nil {
		logx.Printf("store: network settings: %v", err)
		st.setStatus(func(s *EngineStatus) { s.Error = err.Error() })
	}
}

func (st *storeState) setStatus(fn func(*EngineStatus)) {
	exe := st.qbittorrent()
	st.mu.Lock()
	before := st.status
	fn(&st.status)
	st.status.Exe, st.status.Installed = exe, exe != "" && platform.IsFile(exe)
	s := st.status
	st.mu.Unlock()
	if s != before {
		st.c.emit(EventStoreEngine, s)
	}
}

func (st *storeState) getStatus() EngineStatus {
	st.setStatus(func(*EngineStatus) {})
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.status
}

func (st *storeState) close() {
	st.stopEngine()
	if err := st.jobs.Save(); err != nil {
		logx.Printf("store: saving downloads: %v", err)
	}
}

func bytesText(n int64) string {
	const gb, mb = 1 << 30, 1 << 20
	if n >= gb {
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	}
	return fmt.Sprintf("%d MB", (n+mb-1)/mb)
}

// downloadSource checks a magnet link or .torrent URL the person gave,
// and finds a title for it.
func downloadSource(src, title string) (string, string, error) {
	src = strings.TrimSpace(src)
	u, err := url.Parse(src)
	if err != nil || len(src) > 8192 {
		return "", "", errors.New("that isn't a magnet link or a link to a .torrent file")
	}
	switch u.Scheme {
	case "magnet":
		q := u.Query()
		if !strings.HasPrefix(strings.ToLower(q.Get("xt")), "urn:btih:") && !strings.HasPrefix(strings.ToLower(q.Get("xt")), "urn:btmh:") {
			return "", "", errors.New("that magnet link has no torrent hash")
		}
		if title == "" {
			title = q.Get("dn")
		}
	case "https", "http":
		if u.Host == "" {
			return "", "", errors.New("that link has no address")
		}
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(u.Path), ".torrent")
		}
	default:
		return "", "", errors.New("that isn't a magnet link or a link to a .torrent file")
	}
	title = strings.TrimSpace(title)
	if title == "" || title == "." || title == "/" {
		title = "Download"
	}
	if r := []rune(title); len(r) > 200 {
		title = string(r[:200])
	}
	return src, title, nil
}
